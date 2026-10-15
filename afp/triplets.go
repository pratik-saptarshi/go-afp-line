package afp

import (
	"bytes"
	"errors"
	"fmt"
	"io"
)

// Triplet is a decoded MO:DCA triplet: a 1-byte ID followed by contents.
// The wire form is Tlength(1B, total length including itself, range 3-254)
// + Tid(1B) + contents. Contents are preserved verbatim so re-encoding is
// byte-identical; interpretation of specific triplets (beyond the v1
// Comment text accessor) arrives with consumers.
type Triplet struct {
	ID   byte
	Data []byte // triplet contents, verbatim
}

// Triplets is an ordered run of triplets as they appear in a field.
type Triplets []Triplet

// minTripletLen and maxTripletLen are the architectural Tlength bounds per
// the MO:DCA reference: the length includes the Tlength byte itself.
const (
	minTripletLen = 3
	maxTripletLen = 254
)

// knownTripletIDs is the registered triplet set: the 45 dual-corroborated
// IDs from the subset contract §4. Triplet 0x21 is deliberately absent
// (afplib/openafp conflict; excluded pending an S550-1058 check). An
// unregistered Tid is a typed decode error, never a silent skip.
var knownTripletIDs = map[byte]string{
	0x01: "Coded Graphic Character Set Global ID",
	0x02: "Fully Qualified Name",
	0x04: "Mapping Option",
	0x10: "Object Classification",
	0x18: "MO:DCA Interchange Set",
	0x1D: "Text Orientation",
	0x1F: "Font Descriptor Specification",
	0x24: "Resource Local ID",
	0x25: "Resource Section Number",
	0x26: "Character Rotation",
	0x2D: "Object Byte Offset",
	0x36: "Attribute Value",
	0x43: "Descriptor Position",
	0x45: "Media Eject Control",
	0x47: "Resource Usage Attribute",
	0x4B: "Object Area Measurement Units",
	0x4C: "Object Area Size",
	0x4E: "Color Specification",
	0x50: "Encoding Scheme ID",
	0x56: "Medium Map Page Number",
	0x57: "Object Byte Extent",
	0x58: "Object Structured Field Offset",
	0x59: "Object Structured Field Extent",
	0x5A: "Object Offset",
	0x5D: "Font Horizontal Scale Factor",
	0x5E: "Object Count",
	0x62: "Local Date and Time Stamp",
	0x64: "Object Origin Identifier",
	0x65: "Comment",
	0x68: "Medium Orientation",
	0x6C: "Resource Object Include",
	0x6D: "Extension Font",
	0x70: "Presentation Space Reset Mixing",
	0x71: "Presentation Space Mixing Rules",
	0x72: "Universal Date and Time Stamp",
	0x74: "Toner Saver",
	0x75: "Color Fidelity",
	0x78: "Font Fidelity",
	0x79: "Metric Adjustment",
	0x80: "Attribute Qualifier",
	0x81: "Page Position Information",
	0x83: "Presentation Control",
	0x84: "Font Resolution and Metric Technology",
	0x85: "Finishing Operation",
	0x87: "Media Fidelity",
}

// commentTripletID is the Comment triplet, the only triplet v1 interprets.
const commentTripletID = 0x65

// DecodeTriplet decodes a single triplet at the start of b, returning the
// triplet and the number of bytes consumed. Tlength is validated against the
// architectural range before any slice is taken; unknown triplet IDs are a
// typed DecodeError (offset relative to the start of b).
func DecodeTriplet(b []byte) (Triplet, int, error) {
	if len(b) < 2 {
		return Triplet{}, 0, DecodeError{
			Offset: 0,
			Kind:   KindStructure,
			Reason: fmt.Sprintf("truncated triplet: need 2 header bytes, have %d", len(b)),
		}
	}
	tlen := int(b[0])
	if tlen < minTripletLen || tlen > maxTripletLen {
		return Triplet{}, 0, DecodeError{
			Offset: 0,
			Kind:   KindStructure,
			Reason: fmt.Sprintf("invalid Tlength %d: range %d..%d", tlen, minTripletLen, maxTripletLen),
		}
	}
	if len(b) < tlen {
		return Triplet{}, 0, DecodeError{
			Offset: 0,
			Kind:   KindStructure,
			Reason: fmt.Sprintf("truncated triplet: Tlength %d, have %d bytes", tlen, len(b)),
		}
	}
	tid := b[1]
	if _, ok := knownTripletIDs[tid]; !ok {
		return Triplet{}, 0, DecodeError{
			Offset: 0,
			Kind:   KindValidation,
			Reason: fmt.Sprintf("unknown triplet ID X'%02X: not in contracted subset", tid),
		}
	}
	data := make([]byte, tlen-2)
	copy(data, b[2:tlen])
	return Triplet{ID: tid, Data: data}, tlen, nil
}

// DecodeTriplets decodes a whole triplet run (e.g. the trailing bytes of a
// BDT). An empty run decodes to nil with no error. Offsets in returned
// DecodeErrors are relative to the start of b; callers re-base them.
func DecodeTriplets(b []byte) (Triplets, error) {
	var out Triplets
	off := 0
	for off < len(b) {
		tr, n, err := DecodeTriplet(b[off:])
		if err != nil {
			var derr DecodeError
			if errors.As(err, &derr) {
				derr.Offset += off
				return nil, derr
			}
			return nil, err
		}
		out = append(out, tr)
		off += n
	}
	return out, nil
}

// Encode serializes the triplet back to its wire bytes.
//
// The Tlength byte is recomputed from the data on every call. A triplet
// whose data does not fit the architectural maximum (len(Data)+2 > 254)
// cannot be decoded back — DecodeTriplet rejects Tlength > 254 — so Encode
// panics loudly rather than emitting a triplet its own decoder rejects.
// This only fires when a decoded triplet is mutated into an unrepresentable
// state: a programming error.
func (t Triplet) Encode() []byte {
	n := len(t.Data) + 2
	if n < minTripletLen || n > maxTripletLen {
		panic(fmt.Sprintf("afp: Triplet.Encode: Tlength %d outside the architectural range %d..%d (triplet X'%02X carries %d data bytes)",
			n, minTripletLen, maxTripletLen, t.ID, len(t.Data)))
	}
	b := make([]byte, 0, n)
	b = append(b, byte(n), t.ID)
	b = append(b, t.Data...)
	return b
}

// Encode serializes the triplet run back to its wire bytes.
func (ts Triplets) Encode() []byte {
	var b []byte
	for _, t := range ts {
		b = append(b, t.Encode()...)
	}
	return b
}

// Text decodes the triplet contents as an EBCDIC-037 character string,
// trimming trailing padding. In v1 this interprets the Comment triplet
// (X'65'); multi-code-page handling arrives with the Task 5 registry.
func (t Triplet) Text() string {
	return decodeCHAR(t.Data)
}

// RoundTrip is the Tier 1 fidelity gate: it splits the stream into fields,
// decodes each field through the registry (including triplet paths), and
// re-encodes, requiring byte-identical output per field. Any deviation —
// unknown field, unknown triplet, re-encoding mismatch — returns an error.
// It works for both naked and 5A-wrapped streams because Fields
// normalizes framing before comparison, and it streams: fields are
// decoded as they arrive rather than after the whole input materializes.
func RoundTrip(r io.Reader) error {
	i := 0
	for sf, err := range Fields(r) {
		if err != nil {
			return fmt.Errorf("afp: RoundTrip: split: %w", err)
		}
		fb := sf.Data
		f, err := DecodeField(fb)
		if err != nil {
			return fmt.Errorf("afp: RoundTrip: field %d: %w", i, err)
		}
		if enc := f.Encode(); !bytes.Equal(enc, fb) {
			return DecodeError{
				FieldID: f.TypeID(),
				Offset:  i,
				Kind:    KindStructure,
				Reason:  "re-encode differs from input: round-trip gate failed",
			}
		}
		i++
	}
	return nil
}
