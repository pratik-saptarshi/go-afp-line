package afp

import (
	"errors"
	"fmt"
	"strings"
)

// Field is a decoded MO:DCA structured field.
//
// TypeID returns the 3-byte structured field identifier as uppercase hex,
// e.g. "D3A8A8". Encode re-serializes the field; decode followed by Encode
// is byte-identical for every Field implementation.
type Field interface {
	TypeID() string
	Encode() []byte
}

// BDT is a Begin Document structured field (D3A8A8).
type BDT struct {
	Intro    Introducer
	DocName  string // CHAR[8], EBCDIC-037 decoded, trailing padding trimmed
	Reserved [2]byte
	Triplets Triplets // parsed triplet run; re-encoded byte-identically
}

// TypeID implements Field.
func (f *BDT) TypeID() string { return f.Intro.TypeIDString() }

// Encode implements Field.
func (f *BDT) Encode() []byte {
	b := f.Intro.Encode()
	b = append(b, encodeCHAR8(f.DocName)...)
	b = append(b, f.Reserved[0], f.Reserved[1])
	b = append(b, f.Triplets.Encode()...)
	return b
}

// EDT is an End Document structured field (D3A9A8).
type EDT struct {
	Intro    Introducer
	DocName  string
	Reserved [2]byte
	Triplets Triplets
}

// TypeID implements Field.
func (f *EDT) TypeID() string { return f.Intro.TypeIDString() }

// Encode implements Field.
func (f *EDT) Encode() []byte {
	b := f.Intro.Encode()
	b = append(b, encodeCHAR8(f.DocName)...)
	b = append(b, f.Reserved[0], f.Reserved[1])
	b = append(b, f.Triplets.Encode()...)
	return b
}

// BPG is a Begin Page structured field (D3A8AF).
type BPG struct {
	Intro       Introducer
	PageName    string // CHAR[8] when present, EBCDIC-037 decoded
	HasPageName bool   // true when the 8-byte name was present on the wire,
	// even if it decodes to empty (all-spaces); required for
	// byte-identical re-encoding
	Triplets Triplets // parsed triplet run; re-encoded byte-identically
}

// TypeID implements Field.
func (f *BPG) TypeID() string { return f.Intro.TypeIDString() }

// Encode implements Field.
func (f *BPG) Encode() []byte {
	b := f.Intro.Encode()
	if f.HasPageName {
		b = append(b, encodeCHAR8(f.PageName)...)
	}
	b = append(b, f.Triplets.Encode()...)
	return b
}

// EPG is an End Page structured field (D3A9AF).
type EPG struct {
	Intro       Introducer
	PageName    string
	HasPageName bool // see BPG
	Triplets    Triplets
}

// TypeID implements Field.
func (f *EPG) TypeID() string { return f.Intro.TypeIDString() }

// Encode implements Field.
func (f *EPG) Encode() []byte {
	b := f.Intro.Encode()
	if f.HasPageName {
		b = append(b, encodeCHAR8(f.PageName)...)
	}
	b = append(b, f.Triplets.Encode()...)
	return b
}

// RawField is an opaque passthrough for a well-formed field that has no
// typed decoder yet. Excluded is true when the field is known-excluded by
// the subset contract (out-of-subset: IOCA, GOCA, BCOCA, FOCA, CMOCA,
// overlays/medium); false when the field is contracted in-subset but its
// typed decoder arrives in a later task (e.g. PTX in Task 6). The raw bytes
// are always preserved so re-encoding stays byte-identical.
type RawField struct {
	Intro    Introducer
	Data     []byte // field data after the introducer, verbatim
	Excluded bool
}

// TypeID implements Field.
func (f *RawField) TypeID() string { return f.Intro.TypeIDString() }

// Encode implements Field.
func (f *RawField) Encode() []byte {
	b := f.Intro.Encode()
	b = append(b, f.Data...)
	return b
}

// fieldKind classifies a registered structured field type.
type fieldKind int

const (
	// kindTyped has a typed decoder in this task.
	kindTyped fieldKind = iota
	// kindContracted is in-subset per the contract; its typed decoder
	// arrives in a later task. Decodes to *RawField with Excluded=false.
	kindContracted
	// kindExcluded is known-excluded per the contract. Decodes to
	// *RawField with Excluded=true; never an error.
	kindExcluded
)

// fieldSpec is one row of the table-driven registry.
type fieldSpec struct {
	name   string
	kind   fieldKind
	decode func(Introducer, []byte) (Field, error) // set for kindTyped
}

// fieldRegistry maps the 3-byte SFID to its spec. IDs were verified against
// the MODCA-Reference-09 text and dual-corroborated across openafp (CC0) and
// parse-afp (CC0) per the subset contract's conflict rule.
var fieldRegistry = buildRegistry()

func buildRegistry() map[[3]byte]fieldSpec {
	reg := map[[3]byte]fieldSpec{}
	add := func(id [3]byte, name string, kind fieldKind,
		decode func(Introducer, []byte) (Field, error)) {
		reg[id] = fieldSpec{name: name, kind: kind, decode: decode}
	}
	// Typed in this task (contract §3).
	add([3]byte{0xD3, 0xA8, 0xA8}, "BDT", kindTyped, decodeDocField("BDT", newBDT))
	add([3]byte{0xD3, 0xA9, 0xA8}, "EDT", kindTyped, decodeDocField("EDT", newEDT))
	add([3]byte{0xD3, 0xA8, 0xAF}, "BPG", kindTyped, decodePageField("BPG", newBPG))
	add([3]byte{0xD3, 0xA9, 0xAF}, "EPG", kindTyped, decodePageField("EPG", newEPG))
	// Contracted in-subset; typed decoders arrive in later tasks.
	for _, c := range [][3]byte{
		{0xD3, 0xA8, 0xAD}, // BNG Begin Named Page Group
		{0xD3, 0xA9, 0xAD}, // ENG End Named Page Group
		{0xD3, 0xA8, 0xC9}, // BAG Begin Active Environment Group
		{0xD3, 0xA9, 0xC9}, // EAG End Active Environment Group
		{0xD3, 0xA8, 0x9B}, // BPT Begin Presentation Text Object
		{0xD3, 0xA9, 0x9B}, // EPT End Presentation Text Object
		{0xD3, 0xEE, 0x9B}, // PTX Presentation Text Data (Task 6)
		{0xD3, 0xAB, 0x8A}, // MCF Map Coded Font
		{0xD3, 0xB1, 0x9B}, // PTD Presentation Text Descriptor
	} {
		reg[c] = fieldSpec{kind: kindContracted}
	}
	// Known-excluded per the contract §9 closed exclusion list.
	for _, c := range [][3]byte{
		// IOCA (excluded)
		{0xD3, 0xA8, 0xFB}, // BIM
		{0xD3, 0xA9, 0xFB}, // EIM
		{0xD3, 0xEE, 0xFB}, // IPD
		{0xD3, 0xA6, 0xFB}, // IDD
		{0xD3, 0xA6, 0x7B}, // IID
		{0xD3, 0xA7, 0x7B}, // IOC
		{0xD3, 0xAC, 0x7B}, // ICP
		{0xD3, 0xEE, 0x7B}, // IRD
		// GOCA (excluded)
		{0xD3, 0xA8, 0xBB}, // BGR
		{0xD3, 0xA9, 0xBB}, // EGR
		{0xD3, 0xEE, 0xBB}, // GAD
		{0xD3, 0xA6, 0xBB}, // GDD
		// BCOCA (excluded)
		{0xD3, 0xA8, 0xEB}, // BBC
		{0xD3, 0xA9, 0xEB}, // EBC
		{0xD3, 0xEE, 0xEB}, // BDA
		{0xD3, 0xA6, 0xEB}, // BDD
		{0xD3, 0xAB, 0xEB}, // MBC
		// FOCA (excluded)
		{0xD3, 0xA8, 0x8A}, // BCF
		{0xD3, 0xA9, 0x8A}, // ECF
		{0xD3, 0xA8, 0x89}, // BFN
		{0xD3, 0xA9, 0x89}, // EFN
		{0xD3, 0xA8, 0x87}, // BCP
		{0xD3, 0xA9, 0x87}, // ECP
		{0xD3, 0xA7, 0x8A}, // CFC
		{0xD3, 0x8C, 0x8A}, // CFI
		{0xD3, 0xA7, 0x87}, // CPC
		{0xD3, 0xA6, 0x87}, // CPD
		{0xD3, 0x8C, 0x87}, // CPI
		{0xD3, 0xA7, 0x89}, // FNC
		{0xD3, 0xA6, 0x89}, // FND
		{0xD3, 0xEE, 0x89}, // FNG
		{0xD3, 0x8C, 0x89}, // FNI
		{0xD3, 0xA2, 0x89}, // FNM
		{0xD3, 0xAB, 0x89}, // FNN
		{0xD3, 0xAE, 0x89}, // FNO
		{0xD3, 0xAC, 0x89}, // FNP
		{0xD3, 0xB1, 0x8A}, // MCF1 Map Coded Font Format 1
		// CMOCA / color (excluded)
		{0xD3, 0xA8, 0x77}, // BCA
		{0xD3, 0xA9, 0x77}, // ECA
		{0xD3, 0xAB, 0x77}, // MCA
		{0xD3, 0xB0, 0x77}, // CAT
		// Overlays / medium (excluded)
		{0xD3, 0xA8, 0xDF}, // BMO
		{0xD3, 0xA9, 0xDF}, // EMO
		{0xD3, 0xA8, 0xCC}, // BMM
		{0xD3, 0xA9, 0xCC}, // EMM
		{0xD3, 0xB1, 0xDF}, // MMO
		{0xD3, 0xAB, 0xD8}, // MPO
		{0xD3, 0xAB, 0xCC}, // IMM
		{0xD3, 0xA2, 0x88}, // MCC
		{0xD3, 0xA6, 0x88}, // MDD
		{0xD3, 0xA7, 0x88}, // MMC
		{0xD3, 0xA8, 0xC6}, // BRG
		{0xD3, 0xA9, 0xC6}, // ERG
		{0xD3, 0xA8, 0xD9}, // BSG
		{0xD3, 0xA9, 0xD9}, // ESG
		{0xD3, 0xA8, 0xC4}, // BDG
		{0xD3, 0xA9, 0xC4}, // EDG
		{0xD3, 0xA8, 0xC7}, // BOG
		{0xD3, 0xA9, 0xC7}, // EOG
		// Misc skip (excluded, surfaced as diagnostics, never silent)
		{0xD3, 0xEE, 0xEE}, // NOP
		{0xD3, 0xA0, 0x90}, // TLE
		{0xD3, 0xAF, 0x5F}, // IPS
		{0xD3, 0xB1, 0x5F}, // MPS
	} {
		reg[c] = fieldSpec{kind: kindExcluded}
	}
	return reg
}

// DecodeField decodes exactly one structured field from the start of b.
// b must hold precisely one field's bytes (as produced by SplitFields);
// trailing bytes are rejected. The introducer is decoded with
// DecodeIntroducer, then the type ID is dispatched through the registry:
//
//   - in-subset with a typed decoder  -> *BDT, *EDT, *BPG, *EPG
//   - in-subset, decoder in later task -> *RawField with Excluded=false
//   - known-excluded                   -> *RawField with Excluded=true
//   - unknown type                     -> DecodeError (abort)
func DecodeField(b []byte) (Field, error) {
	in, err := DecodeIntroducer(b)
	if err != nil {
		return nil, err
	}
	if len(b) != in.Length {
		return nil, DecodeError{
			FieldID: in.TypeIDString(),
			Offset:  0,
			Reason: fmt.Sprintf("field byte count %d does not match SFLength %d",
				len(b), in.Length),
		}
	}
	spec, ok := fieldRegistry[in.TypeID]
	if !ok {
		return nil, DecodeError{
			FieldID: in.TypeIDString(),
			Offset:  0,
			Reason:  "unknown structured field type: aborting (not in contracted subset)",
		}
	}
	data := b[in.HeaderLength():]
	switch spec.kind {
	case kindTyped:
		return spec.decode(in, data)
	case kindExcluded:
		return &RawField{Intro: in, Data: cloneBytes(data), Excluded: true}, nil
	default: // kindContracted
		return &RawField{Intro: in, Data: cloneBytes(data), Excluded: false}, nil
	}
}

func cloneBytes(b []byte) []byte {
	cp := make([]byte, len(b))
	copy(cp, b)
	return cp
}

// parseTriplets decodes a field's triplet run, attaching the field's ID and
// re-basing the error offset to the stream position of the triplet run.
func parseTriplets(in Introducer, runOffset int, data []byte) (Triplets, error) {
	ts, err := DecodeTriplets(data)
	if err != nil {
		var derr DecodeError
		if errors.As(err, &derr) {
			derr.FieldID = in.TypeIDString()
			derr.Offset += runOffset
			return nil, derr
		}
		return nil, err
	}
	return ts, nil
}

// decodeDocField decodes BDT/EDT: DocName CHAR[8] (0-7), reserved (8-9),
// triplets (10-n).
func decodeDocField(name string, mk func(Introducer, string, [2]byte, Triplets) Field) func(Introducer, []byte) (Field, error) {
	return func(in Introducer, data []byte) (Field, error) {
		if len(data) < 10 {
			return nil, DecodeError{
				FieldID: in.TypeIDString(),
				Offset:  in.HeaderLength(),
				Reason: fmt.Sprintf("%s: need 10 data bytes (DocName+reserved), have %d",
					name, len(data)),
			}
		}
		var reserved [2]byte
		copy(reserved[:], data[8:10])
		ts, err := parseTriplets(in, in.HeaderLength()+10, data[10:])
		if err != nil {
			return nil, err
		}
		return mk(in, decodeCHAR(data[0:8]), reserved, ts), nil
	}
}

func newBDT(in Introducer, name string, reserved [2]byte, triplets Triplets) Field {
	return &BDT{Intro: in, DocName: name, Reserved: reserved, Triplets: triplets}
}

func newEDT(in Introducer, name string, reserved [2]byte, triplets Triplets) Field {
	return &EDT{Intro: in, DocName: name, Reserved: reserved, Triplets: triplets}
}

// decodePageField decodes BPG/EPG: PageName CHAR[8] (0-7, optional),
// triplets (8-n).
func decodePageField(name string, mk func(Introducer, string, bool, Triplets) Field) func(Introducer, []byte) (Field, error) {
	return func(in Introducer, data []byte) (Field, error) {
		switch {
		case len(data) == 0:
			return mk(in, "", false, nil), nil
		case len(data) >= 8:
			ts, err := parseTriplets(in, in.HeaderLength()+8, data[8:])
			if err != nil {
				return nil, err
			}
			return mk(in, decodeCHAR(data[0:8]), true, ts), nil
		default:
			return nil, DecodeError{
				FieldID: in.TypeIDString(),
				Offset:  in.HeaderLength(),
				Reason: fmt.Sprintf("%s: need 0 or >=8 data bytes (PageName), have %d",
					name, len(data)),
			}
		}
	}
}

func newBPG(in Introducer, name string, hasName bool, triplets Triplets) Field {
	return &BPG{Intro: in, PageName: name, HasPageName: hasName, Triplets: triplets}
}

func newEPG(in Introducer, name string, hasName bool, triplets Triplets) Field {
	return &EPG{Intro: in, PageName: name, HasPageName: hasName, Triplets: triplets}
}

// decodeCHAR decodes an EBCDIC-037 CHAR parameter to a string, trimming
// trailing padding (EBCDIC space, 0x40). The active code page machinery
// (X'01 triplet, MCF) arrives in Tasks 3/5; v1 decodes CHAR params as 037,
// which is in the contracted subset.
func decodeCHAR(b []byte) string {
	var sb strings.Builder
	sb.Grow(len(b))
	for _, c := range b {
		sb.WriteRune(ebcdic037[c])
	}
	return strings.TrimRight(sb.String(), " ")
}

// encodeCHAR8 encodes a name back to 8 EBCDIC-037 bytes, space-padded.
// Every rune produced by decodeCHAR reverse-maps; anything else becomes
// '?' (0x6F) rather than silently corrupting the stream.
func encodeCHAR8(s string) []byte {
	out := make([]byte, 8)
	for i := range out {
		out[i] = 0x40 // EBCDIC space padding
	}
	i := 0
	for _, r := range s {
		if i >= 8 {
			break
		}
		out[i] = asciiToEbcdic037(r)
		i++
	}
	return out
}

// asciiToEbcdic037 maps a rune back to EBCDIC-037; unmappable runes become
// '?' (0x6F).
func asciiToEbcdic037(r rune) byte {
	if r < 128 {
		if b := ebcdic037Reverse[r]; b != 0 || r == 0 {
			return b
		}
		// NUL (0x00) reverse-maps ambiguously; fall through to '?'.
	}
	return 0x6F
}

// ebcdic037Reverse is built at init from ebcdic037 for the ASCII range.
var ebcdic037Reverse [128]byte

func init() {
	for b, r := range ebcdic037 {
		if r < 128 && r != 0 {
			ebcdic037Reverse[r] = byte(b)
		}
	}
}

// ebcdic037 maps each EBCDIC-037 byte to its Unicode code point.
// Generated from Python's cp037 codec; spot-checked against the openafp
// Ebc2Asc table (CC0) in docs/_evidence/afp-research/openafp.
// Task 5 (codepage.go) will promote this into the code-page registry.
var ebcdic037 = [256]rune{
	0x0000, 0x0001, 0x0002, 0x0003, 0x009C, 0x0009, 0x0086, 0x007F,
	0x0097, 0x008D, 0x008E, 0x000B, 0x000C, 0x000D, 0x000E, 0x000F,
	0x0010, 0x0011, 0x0012, 0x0013, 0x009D, 0x0085, 0x0008, 0x0087,
	0x0018, 0x0019, 0x0092, 0x008F, 0x001C, 0x001D, 0x001E, 0x001F,
	0x0080, 0x0081, 0x0082, 0x0083, 0x0084, 0x000A, 0x0017, 0x001B,
	0x0088, 0x0089, 0x008A, 0x008B, 0x008C, 0x0005, 0x0006, 0x0007,
	0x0090, 0x0091, 0x0016, 0x0093, 0x0094, 0x0095, 0x0096, 0x0004,
	0x0098, 0x0099, 0x009A, 0x009B, 0x0014, 0x0015, 0x009E, 0x001A,
	0x0020, 0x00A0, 0x00E2, 0x00E4, 0x00E0, 0x00E1, 0x00E3, 0x00E5,
	0x00E7, 0x00F1, 0x00A2, 0x002E, 0x003C, 0x0028, 0x002B, 0x007C,
	0x0026, 0x00E9, 0x00EA, 0x00EB, 0x00E8, 0x00ED, 0x00EE, 0x00EF,
	0x00EC, 0x00DF, 0x0021, 0x0024, 0x002A, 0x0029, 0x003B, 0x00AC,
	0x002D, 0x002F, 0x00C2, 0x00C4, 0x00C0, 0x00C1, 0x00C3, 0x00C5,
	0x00C7, 0x00D1, 0x00A6, 0x002C, 0x0025, 0x005F, 0x003E, 0x003F,
	0x00F8, 0x00C9, 0x00CA, 0x00CB, 0x00C8, 0x00CD, 0x00CE, 0x00CF,
	0x00CC, 0x0060, 0x003A, 0x0023, 0x0040, 0x0027, 0x003D, 0x0022,
	0x00D8, 0x0061, 0x0062, 0x0063, 0x0064, 0x0065, 0x0066, 0x0067,
	0x0068, 0x0069, 0x00AB, 0x00BB, 0x00F0, 0x00FD, 0x00FE, 0x00B1,
	0x00B0, 0x006A, 0x006B, 0x006C, 0x006D, 0x006E, 0x006F, 0x0070,
	0x0071, 0x0072, 0x00AA, 0x00BA, 0x00E6, 0x00B8, 0x00C6, 0x00A4,
	0x00B5, 0x007E, 0x0073, 0x0074, 0x0075, 0x0076, 0x0077, 0x0078,
	0x0079, 0x007A, 0x00A1, 0x00BF, 0x00D0, 0x00DD, 0x00DE, 0x00AE,
	0x005E, 0x00A3, 0x00A5, 0x00B7, 0x00A9, 0x00A7, 0x00B6, 0x00BC,
	0x00BD, 0x00BE, 0x005B, 0x005D, 0x00AF, 0x00A8, 0x00B4, 0x00D7,
	0x007B, 0x0041, 0x0042, 0x0043, 0x0044, 0x0045, 0x0046, 0x0047,
	0x0048, 0x0049, 0x00AD, 0x00F4, 0x00F6, 0x00F2, 0x00F3, 0x00F5,
	0x007D, 0x004A, 0x004B, 0x004C, 0x004D, 0x004E, 0x004F, 0x0050,
	0x0051, 0x0052, 0x00B9, 0x00FB, 0x00FC, 0x00F9, 0x00FA, 0x00FF,
	0x005C, 0x00F7, 0x0053, 0x0054, 0x0055, 0x0056, 0x0057, 0x0058,
	0x0059, 0x005A, 0x00B2, 0x00D4, 0x00D6, 0x00D2, 0x00D3, 0x00D5,
	0x0030, 0x0031, 0x0032, 0x0033, 0x0034, 0x0035, 0x0036, 0x0037,
	0x0038, 0x0039, 0x00B3, 0x00DB, 0x00DC, 0x00D9, 0x00DA, 0x009F,
}
