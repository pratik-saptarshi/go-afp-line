package afp

import (
	"encoding/binary"
	"fmt"
	"strings"
)

// MCFMapping is the parsed result of an MCF (Map Coded Font) field's
// repeating groups. Each repeating group maps one local font ID (LID) to a
// resource name; in the contracted subset only groups carrying a coded-font
// name FQN (X'02' with FQNType X'8E' *and* FQNFmt X'00', the
// character-string format per MO:DCA FQN semantics) are mappable. An X'8E'
// with any other FQNFmt (e.g. X'10' OID) carries no character-string name
// and is treated like a nameless group. Groups that cannot be mapped (X'24'
// present, no X'8E'/X'00' name) have their LID recorded in Unsupported
// instead, in group order. (MO:DCA Reference S550-1058, MCF structured
// field; field and triplet semantics paraphrased here, not quoted.)
type MCFMapping struct {
	// ByLID maps a local font ID to the coded font name from its X'8E' /
	// X'00' Fully Qualified Name triplet, EBCDIC-037 decoded.
	ByLID map[byte]string
	// Unsupported lists the LIDs of repeating groups that carry an X'24'
	// local ID but no X'8E'/X'00' coded-font name — either a different
	// FQNType or an X'8E' with a non-character-string FQNFmt — in group
	// order.
	Unsupported []byte
}

// mcf triplet and FQN-type constants used by the MCF parser.
const (
	mcfTripletResourceLocalID byte = 0x24 // Resource Local ID
	mcfTripletFQN             byte = 0x02 // Fully Qualified Name
	mcfResTypeFont            byte = 0x05 // Resource Local ID ResType: font
	mcfFQNTypeCodedFontName   byte = 0x8E // FQNType: coded font name
	mcfFQNFormatCharString    byte = 0x00 // FQNFmt: character string (MO:DCA FQN semantics)
)

// ParseMCFMapping parses the repeating groups of an MCF field's data (the
// bytes after the structured-field introducer).
//
// Each group starts with a 2-byte big-endian RGLength (inclusive of itself)
// followed by a run of MO:DCA triplets. RGLength bounds violations, malformed
// X'24'/X'02' triplets, duplicate LIDs, repeated FQNTypes within a group, and
// unexpected ResType values are DecodeError{Kind: KindStructure} — this parser
// never guesses. An unexpected FQNType (anything but X'8E'), or an X'8E'
// with a non-character-string FQNFmt (anything but X'00'), is not an error:
// the group simply carries no coded-font name. A group without an X'24' triplet
// uses LID 0x00 (the architected default, applied per group); only a group
// carrying an X'24' but contributing no usable coded-font name — no X'8E'
// FQN at all, or an X'8E' whose FQNFmt is not a character string — is
// recorded in Unsupported. Empty input
// yields a zero MCFMapping with non-nil empty collections and no error.
func ParseMCFMapping(data []byte) (MCFMapping, error) {
	m := MCFMapping{ByLID: map[byte]string{}, Unsupported: []byte{}}
	seen := map[byte]bool{}
	pos := 0
	for pos < len(data) {
		if pos+2 > len(data) {
			return MCFMapping{}, DecodeError{
				Offset: pos,
				Kind:   KindStructure,
				Reason: fmt.Sprintf("MCF: truncated repeating-group header: need 2 bytes, have %d", len(data)-pos),
			}
		}
		rgLen := int(binary.BigEndian.Uint16(data[pos : pos+2]))
		if rgLen < 2 || pos+rgLen > len(data) {
			return MCFMapping{}, DecodeError{
				Offset: pos,
				Kind:   KindStructure,
				Reason: fmt.Sprintf("MCF: invalid RGLength %d at offset %d (data length %d)", rgLen, pos, len(data)),
			}
		}
		group := data[pos+2 : pos+rgLen]
		triplets, err := decodeTriplets(group)
		if err != nil {
			if derr, ok := err.(DecodeError); ok {
				derr.Offset += pos + 2
				return MCFMapping{}, derr
			}
			return MCFMapping{}, err
		}
		lid, name, hasName, hasX24, err := parseMCFGroup(triplets, pos+2)
		if err != nil {
			return MCFMapping{}, err
		}
		if seen[lid] {
			// MO:DCA p. 232 permits the same X'24' value across contiguous
			// groups for double-byte font sections; v1 does not implement
			// double-byte fonts, so the rejection is kept and the
			// diagnostic says so loudly (previously-reducing files with
			// such MCFs now abort here rather than mis-resolving later).
			return MCFMapping{}, DecodeError{
				Offset: pos + 2,
				Kind:   KindStructure,
				Reason: fmt.Sprintf("duplicate MCF local font ID %d; v1 does not support the MO:DCA p. 232 double-byte font exception", lid),
			}
		}
		seen[lid] = true
		if hasName {
			m.ByLID[lid] = name
		} else if hasX24 {
			// A nameless group without an X'24' carries no LID of its
			// own (it only borrows the architected default), so it is
			// not recorded as Unsupported.
			m.Unsupported = append(m.Unsupported, lid)
		}
		pos += rgLen
	}
	return m, nil
}

// parseMCFGroup interprets one repeating group's triplets. It returns the
// group's local font ID (0x00 when no X'24' triplet is present), hasX24
// reporting whether an X'24' triplet was present, and, when the group
// carries an X'8E'/X'00' coded-font name FQN (FQNType X'8E', FQNFmt X'00'
// character-string format), the decoded name with hasName set.
// base is the group's byte offset within the field data, used to
// re-base error offsets.
func parseMCFGroup(triplets Triplets, base int) (lid byte, name string, hasName, hasX24 bool, err error) {
	var seenX24 bool
	seenFQNType := map[byte]bool{}
	toff := 0 // running triplet offset within the group body
	for _, tr := range triplets {
		off := base + toff
		toff += len(tr.Encode())
		switch tr.ID {
		case mcfTripletResourceLocalID:
			if seenX24 {
				return 0, "", false, false, DecodeError{
					Offset: off,
					Kind:   KindStructure,
					Reason: "MCF: duplicate X'24' triplet in one repeating group",
				}
			}
			seenX24 = true
			if len(tr.Data) != 2 || tr.Data[0] != mcfResTypeFont {
				return 0, "", false, false, DecodeError{
					Offset: off,
					Kind:   KindStructure,
					Reason: fmt.Sprintf("MCF: X'24' needs ResType X'05' plus one LID byte, have %d data bytes starting X'%02X'",
						len(tr.Data), firstByte(tr.Data)),
				}
			}
			lid = tr.Data[1]
		case mcfTripletFQN:
			// Uniqueness is per FQNType, not per group: MO:DCA (MCF
			// syntax) lists X'85' (code page name) + X'86' (charset name)
			// as a mandatory minimum identification pair — two X'02'
			// triplets with different FQNTyp es in one group is the
			// standard real-world shape. A repeated FQNType is malformed.
			fqnType := firstByte(tr.Data)
			if seenFQNType[fqnType] {
				return 0, "", false, false, DecodeError{
					Offset: off,
					Kind:   KindStructure,
					Reason: fmt.Sprintf("duplicate FQN type %02X in one repeating group", fqnType),
				}
			}
			seenFQNType[fqnType] = true
			if len(tr.Data) < 2 {
				return 0, "", false, false, DecodeError{
					Offset: off,
					Kind:   KindStructure,
					Reason: fmt.Sprintf("MCF: X'02' needs FQNType/FQNFmt (2 bytes), have %d data bytes", len(tr.Data)),
				}
			}
			if tr.Data[0] == mcfFQNTypeCodedFontName && tr.Data[1] == mcfFQNFormatCharString {
				name = decodeEBCDICName(tr.Data[2:])
				hasName = true
			}
		}
	}
	return lid, name, hasName, seenX24, nil
}

// firstByte returns the first byte of b, or 0 for an empty slice. It exists
// only to keep diagnostic formatting total.
func firstByte(b []byte) byte {
	if len(b) == 0 {
		return 0
	}
	return b[0]
}

// decodeEBCDICName decodes an EBCDIC-037 byte string to text using the
// package's 037 table. Unlike decodeCHAR it does not trim trailing padding:
// an FQN value is a variable-length string, not a padded CHAR[8] parameter,
// and trimming would silently alter the mapped name.
func decodeEBCDICName(b []byte) string {
	var sb strings.Builder
	sb.Grow(len(b))
	for _, c := range b {
		sb.WriteRune(ebcdic037[c])
	}
	return sb.String()
}
