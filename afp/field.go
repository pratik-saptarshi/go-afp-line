package afp

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// Field is a decoded MO:DCA structured field.
//
// TypeID returns the 3-byte structured field identifier as uppercase hex,
// e.g. "D3A8A8".
//
// Encode re-serializes the field; decode followed by Encode is byte-identical
// for every Field implementation.
//
// Encode failure policy (fail-loud, one policy for every Encode method in
// this package): the introducer length is always recomputed from the encoded
// body — the stored Introducer.Length is never trusted, so mutating a
// decoded field re-encodes with a correct length prefix. Encode panics,
// rather than silently truncating, when the field state cannot be
// represented on the wire: a triplet whose data exceeds the 1-byte Tlength
// field, or a total length beyond the MO:DCA 32,767-byte maximum.
// DecodeField and DecodeTriplet validate all of these at construction, so a
// panic only fires on post-decode mutation of a decoded field — a
// programming error. Decode-side failures stay typed DecodeError returns;
// panics are encode-side only.
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

// encodeField assembles a field's wire bytes from its introducer and an
// already-encoded body. The introducer length is recomputed from the body —
// never trusted from stored state — so a field mutated after decoding
// re-encodes with a correct SFLength prefix. A total length beyond the
// MO:DCA architectural maximum cannot be represented on the wire: it panics
// loudly per the Field encode failure policy instead of emitting a bogus
// length.
func encodeField(in Introducer, body []byte) []byte {
	in.Length = in.HeaderLength() + len(body)
	if in.Length > maxSFLength {
		panic(fmt.Sprintf("afp: field %s: encoded length %d exceeds the MO:DCA maximum %d",
			in.TypeIDString(), in.Length, maxSFLength))
	}
	head := in.Encode()
	out := make([]byte, 0, len(head)+len(body))
	out = append(out, head...)
	return append(out, body...)
}

// TypeID implements Field.
func (f *BDT) TypeID() string { return f.Intro.TypeIDString() }

// Encode implements Field.
func (f *BDT) Encode() []byte {
	body := make([]byte, 0, 10)
	body = append(body, encodeCHAR8(f.DocName)...)
	body = append(body, f.Reserved[0], f.Reserved[1])
	body = append(body, f.Triplets.Encode()...)
	return encodeField(f.Intro, body)
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
	body := make([]byte, 0, 10)
	body = append(body, encodeCHAR8(f.DocName)...)
	body = append(body, f.Reserved[0], f.Reserved[1])
	body = append(body, f.Triplets.Encode()...)
	return encodeField(f.Intro, body)
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
	body := make([]byte, 0, 8)
	if f.HasPageName {
		body = append(body, encodeCHAR8(f.PageName)...)
	}
	body = append(body, f.Triplets.Encode()...)
	return encodeField(f.Intro, body)
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
	body := make([]byte, 0, 8)
	if f.HasPageName {
		body = append(body, encodeCHAR8(f.PageName)...)
	}
	body = append(body, f.Triplets.Encode()...)
	return encodeField(f.Intro, body)
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
	return encodeField(f.Intro, f.Data)
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
	for _, e := range []struct {
		id   [3]byte
		name string
	}{
		{[3]byte{0xD3, 0xA8, 0xAD}, "BNG"},
		{[3]byte{0xD3, 0xA9, 0xAD}, "ENG"},
		{[3]byte{0xD3, 0xA8, 0xC9}, "BAG"},
		{[3]byte{0xD3, 0xA9, 0xC9}, "EAG"},
		{[3]byte{0xD3, 0xA8, 0x9B}, "BPT"},
		{[3]byte{0xD3, 0xA9, 0x9B}, "EPT"},
		{[3]byte{0xD3, 0xEE, 0x9B}, "PTX"},
		{[3]byte{0xD3, 0xAB, 0x8A}, "MCF"},
		{[3]byte{0xD3, 0xB1, 0x9B}, "PTD"},
	} {
		reg[e.id] = fieldSpec{name: e.name, kind: kindContracted}
	}
	// Known-excluded per the contract §9 closed exclusion list.
	for _, e := range []struct {
		id   [3]byte
		name string
	}{
		// IOCA (excluded)
		{[3]byte{0xD3, 0xA8, 0xFB}, "BIM"},
		{[3]byte{0xD3, 0xA9, 0xFB}, "EIM"},
		{[3]byte{0xD3, 0xEE, 0xFB}, "IPD"},
		{[3]byte{0xD3, 0xA6, 0xFB}, "IDD"},
		{[3]byte{0xD3, 0xA6, 0x7B}, "IID"},
		{[3]byte{0xD3, 0xA7, 0x7B}, "IOC"},
		{[3]byte{0xD3, 0xAC, 0x7B}, "ICP"},
		{[3]byte{0xD3, 0xEE, 0x7B}, "IRD"},
		// GOCA (excluded)
		{[3]byte{0xD3, 0xA8, 0xBB}, "BGR"},
		{[3]byte{0xD3, 0xA9, 0xBB}, "EGR"},
		{[3]byte{0xD3, 0xEE, 0xBB}, "GAD"},
		{[3]byte{0xD3, 0xA6, 0xBB}, "GDD"},
		// BCOCA (excluded)
		{[3]byte{0xD3, 0xA8, 0xEB}, "BBC"},
		{[3]byte{0xD3, 0xA9, 0xEB}, "EBC"},
		{[3]byte{0xD3, 0xEE, 0xEB}, "BDA"},
		{[3]byte{0xD3, 0xA6, 0xEB}, "BDD"},
		{[3]byte{0xD3, 0xAB, 0xEB}, "MBC"},
		// FOCA (excluded)
		{[3]byte{0xD3, 0xA8, 0x8A}, "BCF"},
		{[3]byte{0xD3, 0xA9, 0x8A}, "ECF"},
		{[3]byte{0xD3, 0xA8, 0x89}, "BFN"},
		{[3]byte{0xD3, 0xA9, 0x89}, "EFN"},
		{[3]byte{0xD3, 0xA8, 0x87}, "BCP"},
		{[3]byte{0xD3, 0xA9, 0x87}, "ECP"},
		{[3]byte{0xD3, 0xA7, 0x8A}, "CFC"},
		{[3]byte{0xD3, 0x8C, 0x8A}, "CFI"},
		{[3]byte{0xD3, 0xA7, 0x87}, "CPC"},
		{[3]byte{0xD3, 0xA6, 0x87}, "CPD"},
		{[3]byte{0xD3, 0x8C, 0x87}, "CPI"},
		{[3]byte{0xD3, 0xA7, 0x89}, "FNC"},
		{[3]byte{0xD3, 0xA6, 0x89}, "FND"},
		{[3]byte{0xD3, 0xEE, 0x89}, "FNG"},
		{[3]byte{0xD3, 0x8C, 0x89}, "FNI"},
		{[3]byte{0xD3, 0xA2, 0x89}, "FNM"},
		{[3]byte{0xD3, 0xAB, 0x89}, "FNN"},
		{[3]byte{0xD3, 0xAE, 0x89}, "FNO"},
		{[3]byte{0xD3, 0xAC, 0x89}, "FNP"},
		{[3]byte{0xD3, 0xB1, 0x8A}, "MCF1"},
		// CMOCA / color (excluded)
		{[3]byte{0xD3, 0xA8, 0x77}, "BCA"},
		{[3]byte{0xD3, 0xA9, 0x77}, "ECA"},
		{[3]byte{0xD3, 0xAB, 0x77}, "MCA"},
		{[3]byte{0xD3, 0xB0, 0x77}, "CAT"},
		// Overlays / medium (excluded)
		{[3]byte{0xD3, 0xA8, 0xDF}, "BMO"},
		{[3]byte{0xD3, 0xA9, 0xDF}, "EMO"},
		{[3]byte{0xD3, 0xA8, 0xCC}, "BMM"},
		{[3]byte{0xD3, 0xA9, 0xCC}, "EMM"},
		{[3]byte{0xD3, 0xB1, 0xDF}, "MMO"},
		{[3]byte{0xD3, 0xAB, 0xD8}, "MPO"},
		{[3]byte{0xD3, 0xAB, 0xCC}, "IMM"},
		{[3]byte{0xD3, 0xA2, 0x88}, "MCC"},
		{[3]byte{0xD3, 0xA6, 0x88}, "MDD"},
		{[3]byte{0xD3, 0xA7, 0x88}, "MMC"},
		{[3]byte{0xD3, 0xA8, 0xC6}, "BRG"},
		{[3]byte{0xD3, 0xA9, 0xC6}, "ERG"},
		{[3]byte{0xD3, 0xA8, 0xD9}, "BSG"},
		{[3]byte{0xD3, 0xA9, 0xD9}, "ESG"},
		{[3]byte{0xD3, 0xA8, 0xC4}, "BDG"},
		{[3]byte{0xD3, 0xA9, 0xC4}, "EDG"},
		{[3]byte{0xD3, 0xA8, 0xC7}, "BOG"},
		{[3]byte{0xD3, 0xA9, 0xC7}, "EOG"},
		// Misc skip (excluded, surfaced as diagnostics, never silent)
		{[3]byte{0xD3, 0xEE, 0xEE}, "NOP"},
		{[3]byte{0xD3, 0xA0, 0x90}, "TLE"},
		{[3]byte{0xD3, 0xAF, 0x5F}, "IPS"},
		{[3]byte{0xD3, 0xB1, 0x5F}, "MPS"},
	} {
		reg[e.id] = fieldSpec{name: e.name, kind: kindExcluded}
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

// Acronym returns the short field name (e.g. "BDT", "PTX", "BGR") registered
// for a 3-byte structured-field type ID in uppercase hex (e.g. "D3A8BB").
// The second return value is false when the ID is malformed, unregistered, or
// registered without an acronym (in-subset fields whose typed decoder arrives
// in a later task). Exclusion diagnostics use this to name the field.
func Acronym(typeID string) (string, bool) {
	b, err := hex.DecodeString(typeID)
	if err != nil || len(b) != 3 {
		return "", false
	}
	var id [3]byte
	copy(id[:], b)
	spec, ok := fieldRegistry[id]
	if !ok || spec.name == "" {
		return "", false
	}
	return spec.name, true
}

func DecodeField(b []byte) (Field, error) {
	in, err := DecodeIntroducer(b)
	if err != nil {
		return nil, err
	}
	if len(b) != in.Length {
		return nil, DecodeError{
			FieldID: in.TypeIDString(),
			Offset:  0,
			Kind:    KindStructure,
			Reason: fmt.Sprintf("field byte count %d does not match SFLength %d",
				len(b), in.Length),
		}
	}
	spec, ok := fieldRegistry[in.TypeID]
	if !ok {
		return nil, DecodeError{
			FieldID: in.TypeIDString(),
			Offset:  0,
			Kind:    KindValidation,
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
				Kind:    KindStructure,
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
				Kind:    KindStructure,
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

// ebcdic037 is the EBCDIC-037 decode table, promoted to the code-page
// registry in codepage.go (Task 5). Kept as an alias so the CHAR
// parameter helpers below keep working unchanged.
var ebcdic037 = cp037Table
