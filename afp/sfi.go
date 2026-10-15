package afp

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// MaxFieldSize bounds the claimed length of a single structured field.
// It is enforced before any allocation based on the claim, so a lying
// length field can never cause an oversized allocation.
const MaxFieldSize = 1 << 20 // 1 MiB

// sfiLen is the length of a structured field introducer without extension.
const sfiLen = 8

// maxSFLength is the architectural maximum structured-field length per the
// MO:DCA reference (32,767 including the introducer).
const maxSFLength = 32767

// SFI flag masks. IBM numbers bits from the most significant bit, so
// "bit 0" is 0x80, "bit 2" is 0x20 and "bit 4" is 0x08.
const (
	flagExt = 0x80 // bit 0: SFI extension present
	flagSeg = 0x20 // bit 2: data segmented across consecutive fields
	flagPad = 0x08 // bit 4: padding present
)

// Introducer is a decoded MO:DCA structured field introducer (SFI).
//
// Layout (big-endian): SFLength (2B, total field length including the
// introducer), SFTypeID (3B, D3 TT CC), Flags (1B), Reserved (2B),
// optional extension (1B ExtLength + data) when the extension bit is set.
type Introducer struct {
	// Length is SFLength: the total structured-field length in bytes,
	// including the introducer itself. Range 8..32767.
	Length int
	// TypeID is the 3-byte structured field identifier (D3 TT CC).
	TypeID [3]byte
	// Flags is the raw SFI flag byte.
	Flags byte
	// Reserved holds the 2 reserved bytes verbatim. The architecture says
	// they should be zero, but they are preserved (not validated) so that
	// decode followed by encode stays byte-identical.
	Reserved [2]byte
	// Extension holds the SFI extension bytes including the ExtLength byte,
	// or nil when no extension is present.
	Extension []byte
}

// TypeIDString returns the type ID as uppercase hex, e.g. "D3A8A8".
func (in Introducer) TypeIDString() string {
	return fmt.Sprintf("%02X%02X%02X", in.TypeID[0], in.TypeID[1], in.TypeID[2])
}

// HeaderLength returns the introducer length in bytes: 8 plus extension.
func (in Introducer) HeaderLength() int {
	return sfiLen + len(in.Extension)
}

// HasExtension reports whether the SFI extension bit is set.
func (in Introducer) HasExtension() bool { return in.Flags&flagExt != 0 }

// IsSegmented reports whether the segmentation bit is set.
func (in Introducer) IsSegmented() bool { return in.Flags&flagSeg != 0 }

// HasPadding reports whether the padding bit is set.
func (in Introducer) HasPadding() bool { return in.Flags&flagPad != 0 }

// Encode serializes the introducer back to its wire bytes.
// Decode followed by Encode is byte-identical for the introducer.
func (in Introducer) Encode() []byte {
	b := make([]byte, 0, sfiLen+len(in.Extension))
	b = append(b, byte(in.Length>>8), byte(in.Length))
	b = append(b, in.TypeID[0], in.TypeID[1], in.TypeID[2], in.Flags, in.Reserved[0], in.Reserved[1])
	b = append(b, in.Extension...)
	return b
}

// DecodeIntroducer parses the structured field introducer at the start of b.
// b must hold at least the introducer; it may hold more (the field data is
// not consumed). A claimed length is never trusted for allocation: lengths
// above MaxFieldSize or the architectural maximum are rejected before any
// allocation could occur.
func DecodeIntroducer(b []byte) (Introducer, error) {
	if len(b) < sfiLen {
		return Introducer{}, DecodeError{
			Kind:   KindStructure,
			Offset: 0,
			Reason: fmt.Sprintf("truncated introducer: need %d bytes, have %d", sfiLen, len(b)),
		}
	}
	length := int(binary.BigEndian.Uint16(b[0:2]))
	typeID := [3]byte{b[2], b[3], b[4]}
	fieldID := fmt.Sprintf("%02X%02X%02X", typeID[0], typeID[1], typeID[2])
	if length < sfiLen {
		return Introducer{}, DecodeError{
			Kind:    KindStructure,
			FieldID: fieldID,
			Offset:  0,
			Reason:  fmt.Sprintf("invalid SFLength %d: minimum %d", length, sfiLen),
		}
	}
	if length > MaxFieldSize {
		return Introducer{}, DecodeError{
			Kind:    KindStructure,
			FieldID: fieldID,
			Offset:  0,
			Reason:  fmt.Sprintf("SFLength %d exceeds MaxFieldSize %d", length, MaxFieldSize),
		}
	}
	if length > maxSFLength {
		return Introducer{}, DecodeError{
			Kind:    KindStructure,
			FieldID: fieldID,
			Offset:  0,
			Reason:  fmt.Sprintf("SFLength %d exceeds MO:DCA maximum %d", length, maxSFLength),
		}
	}
	in := Introducer{Length: length, TypeID: typeID, Flags: b[5], Reserved: [2]byte{b[6], b[7]}}
	if b[5]&flagExt == 0 {
		return in, nil
	}
	if len(b) < sfiLen+1 {
		return Introducer{}, DecodeError{
			Kind:    KindStructure,
			FieldID: fieldID,
			Offset:  sfiLen,
			Reason:  "truncated SFI extension: missing ExtLength byte",
		}
	}
	extLen := int(b[sfiLen])
	if extLen < 1 {
		return Introducer{}, DecodeError{
			Kind:    KindStructure,
			FieldID: fieldID,
			Offset:  sfiLen,
			Reason:  fmt.Sprintf("invalid ExtLength %d: range 1..255", extLen),
		}
	}
	if len(b) < sfiLen+extLen {
		return Introducer{}, DecodeError{
			Kind:    KindStructure,
			FieldID: fieldID,
			Offset:  sfiLen,
			Reason:  fmt.Sprintf("truncated SFI extension: need %d bytes, have %d", extLen, len(b)-sfiLen),
		}
	}
	in.Extension = append([]byte(nil), b[sfiLen:sfiLen+extLen]...)
	return in, nil
}

// IsWrapped reports whether data uses 5A-wrapped records: a leading 0x5A
// byte, a plausible record length, and the D3 class code where the SFID
// belongs. Real streams are homogeneous; the framing is detected once from
// the first record so a naked field with a 0x5Axx length is not misread.
// SplitFields applies this same check before stripping the one-byte framing
// marker from each field; callers that need true wire offsets use it to
// account for the stripped byte.
func IsWrapped(data []byte) bool {
	if len(data) < 6 || data[0] != 0x5A {
		return false
	}
	l := int(binary.BigEndian.Uint16(data[1:3]))
	return l >= sfiLen && l <= len(data)-1 && data[3] == 0xD3
}

// SplitFields reads a MO:DCA stream and returns each structured field's raw
// bytes (starting at its introducer). It accepts both naked streams and
// 5A-wrapped variable-blocked records; in wrapped records the 2-byte record
// length doubles as the field's SFLength, so stripping the 5A byte yields
// the field. Every field is validated with DecodeIntroducer before its
// bytes are copied, and no allocation is sized by an unvalidated claim.
func SplitFields(r io.Reader) ([][]byte, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("afp: SplitFields: %w", err)
	}
	wrapped := IsWrapped(data)
	var fields [][]byte
	off := 0
	for off < len(data) {
		fieldStart := off
		if wrapped {
			off++ // strip the 5A marker; the record length is the SFLength
		}
		if len(data)-off < 2 {
			return nil, DecodeError{Offset: fieldStart, Kind: KindStructure, Reason: "truncated field length"}
		}
		flen := int(binary.BigEndian.Uint16(data[off : off+2]))
		if flen < sfiLen {
			return nil, DecodeError{
				Kind:   KindStructure,
				Offset: fieldStart,
				Reason: fmt.Sprintf("invalid field length %d: minimum %d", flen, sfiLen),
			}
		}
		if flen > MaxFieldSize {
			return nil, DecodeError{
				Kind:   KindStructure,
				Offset: fieldStart,
				Reason: fmt.Sprintf("field length %d exceeds MaxFieldSize %d", flen, MaxFieldSize),
			}
		}
		if len(data)-off < flen {
			return nil, DecodeError{
				Kind:   KindStructure,
				Offset: fieldStart,
				Reason: fmt.Sprintf("truncated field: need %d bytes, have %d", flen, len(data)-off),
			}
		}
		field := data[off : off+flen]
		if derr := checkIntroducer(field, fieldStart); derr != nil {
			return nil, derr
		}
		cp := make([]byte, flen)
		copy(cp, field)
		fields = append(fields, cp)
		off += flen
	}
	return fields, nil
}

// checkIntroducer validates a field's introducer, re-basing any error
// offset to the stream position where the field starts.
func checkIntroducer(field []byte, fieldStart int) error {
	if _, err := DecodeIntroducer(field); err != nil {
		var derr DecodeError
		if errors.As(err, &derr) {
			derr.Offset += fieldStart
			return derr
		}
		return err
	}
	return nil
}
