package afp

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"iter"
)

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
	// Mirror decodeIntroducer's 8..32767 range: a wire-unrepresentable
	// Length can only come from post-decode caller mutation (programmer
	// error), so fail loudly rather than wrapping the length bytes.
	if in.Length < sfiLen || in.Length > maxSFLength {
		panic(fmt.Sprintf("afp: cannot encode introducer with length %d (want %d..%d)", in.Length, sfiLen, maxSFLength))
	}
	b := make([]byte, 0, sfiLen+len(in.Extension))
	b = append(b, byte(in.Length>>8), byte(in.Length))
	b = append(b, in.TypeID[0], in.TypeID[1], in.TypeID[2], in.Flags, in.Reserved[0], in.Reserved[1])
	b = append(b, in.Extension...)
	return b
}

// decodeIntroducer parses the structured field introducer at the start of b.
// b must hold at least the introducer; it may hold more (the field data is
// not consumed). A claimed length is never trusted for allocation: lengths
// above the architectural maximum are rejected before any allocation could
// occur.
func decodeIntroducer(b []byte) (Introducer, error) {
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

// isWrapped reports whether data uses 5A-wrapped records: a leading 0x5A
// byte, a plausible record length, and the D3 class code where the SFID
// belongs. Real streams are homogeneous; the framing is detected once from
// the first record so a naked field with a 0x5Axx length is not misread.
// SplitFields applies this same check before stripping the one-byte framing
// marker from each field; callers that need true wire offsets use it to
// account for the stripped byte.
func isWrapped(data []byte) bool {
	if len(data) < 6 || data[0] != 0x5A {
		return false
	}
	l := int(binary.BigEndian.Uint16(data[1:3]))
	return l >= sfiLen && l <= len(data)-1 && data[3] == 0xD3
}

// StreamField is one structured field yielded by Fields.
type StreamField struct {
	// Data holds the complete field bytes starting at the introducer
	// (the 5A record marker is stripped on wrapped streams). The buffer
	// is freshly allocated per field and owned by the caller; the
	// iterator never reuses it. len(Data) always equals the validated
	// SFLength.
	Data []byte
	// Offset is the wire offset of the field's first wire byte: the 5A
	// marker on wrapped streams, the introducer otherwise.
	Offset int
}

// Fields returns an iterator over the structured fields of a MO:DCA
// stream, read incrementally: at most one field (plus a small framing
// prefix) is retained at a time. It accepts naked and 5A-wrapped streams,
// exactly like SplitFields, and yields the same fields with the same wire
// offsets and the same error contract (Kind/Reason/Offset per field).
//
// The sequence is single-use: it reads from r as it is iterated, so
// iterating it twice is undefined. If the consumer stops early (break or
// return), the iterator stops reading promptly and holds no resources:
// there are no goroutines or channels, so nothing can leak.
func Fields(r io.Reader) iter.Seq2[StreamField, error] {
	return fieldsOp(r, "Fields")
}

// pendingErrReader wraps an io.Reader so a non-EOF error returned alongside
// bytes is never silently lost. The io.Reader contract requires callers to
// process n > 0 bytes before considering err, but io.ReadFull discards an
// error that arrives with a full buffer — silently accepting input the old
// io.ReadAll path would have rejected (a reader may legally return a full
// buffer together with a non-EOF error, e.g. a checksum-validating reader
// delivering final bytes with its checksum error). This wrapper stages such
// an error and replays it (with n == 0) on the next Read, after the bytes
// have been processed. io.EOF is never staged: it is part of normal framing
// and ReadFull already handles (n > 0, EOF) by continuing.
type pendingErrReader struct {
	r       io.Reader
	pending error
}

func (e *pendingErrReader) Read(p []byte) (int, error) {
	if e.pending != nil {
		err := e.pending
		e.pending = nil
		return 0, err
	}
	n, err := e.r.Read(p)
	if n > 0 && err != nil && err != io.EOF {
		e.pending = err
		return n, nil
	}
	return n, err
}

// fieldsOp is the shared iterator core behind Fields and SplitFields; op
// names the operation in read-error wrappers.
func fieldsOp(r io.Reader, op string) iter.Seq2[StreamField, error] {
	return func(yield func(StreamField, error) bool) {
		// Stage errors that arrive with bytes (see pendingErrReader) so
		// every read site below — prefix, marker, length, body — surfaces
		// them after the accompanying bytes are processed.
		r = &pendingErrReader{r: r}
		fail := func(err error) bool {
			return yield(StreamField{}, err)
		}
		readErr := func(err error) bool {
			return fail(fmt.Errorf("afp: %s: %w", op, err))
		}
		// Framing peek: read a prefix once, then apply the existing
		// isWrapped predicate to it. The prefix is 1+l bytes (l is the
		// big-endian uint16 at [1:3]) when byte 0 is 0x5A and at least
		// 4 bytes are available, else up to 6 bytes: isWrapped needs
		// l <= len-1 with l >= 8, which a fixed 6-byte peek can never
		// satisfy. The peek only ever holds bytes the stream actually
		// contains. Streams are homogeneous; the decision is
		// stream-global.
		prefix, err := readPrefix(r)
		if err != nil {
			readErr(err)
			return
		}
		wrapped := isWrapped(prefix)
		if !wrapped && len(prefix) >= 4 && prefix[0] == 0x5A &&
			prefix[2] != 0xD3 && prefix[3] == 0xD3 &&
			int(binary.BigEndian.Uint16(prefix[1:3])) > maxSFLength {
			// Bound-aware framing fallback: isWrapped needs the full
			// record, which readPrefix deliberately does not fetch when
			// the claimed length exceeds the architectural maximum.
			// A 5A-prefixed record with D3 at [3] (and not at [2], which
			// would start a naked type triple) is wrapped with an
			// impossible length; treat it as wrapped so the length bound
			// check below rejects it instead of misclassifying the
			// framing and reporting truncation.
			// Note: prefix[2] == 0xD3 is deliberately excluded. That
			// input is genuinely ambiguous — it is also a plausible
			// naked field (length 0x5AXX, type triple at [2:5]) — and
			// preferring the valid interpretation avoids false
			// rejections. A short such stream reports truncation, which
			// is defensible for both readings.
			wrapped = true
		}
		// The prefix bytes are stream bytes; continue reading after them.
		stream := io.MultiReader(bytes.NewReader(prefix), r)
		off := 0
		var one, two [2]byte
		for {
			fieldStart := off
			if wrapped {
				// Verify the 5A record marker before stripping it: a
				// corrupt marker is stream damage, not a byte to skip
				// silently.
				if _, err := io.ReadFull(stream, one[:1]); err != nil {
					if err == io.EOF {
						return // clean end: nothing after the last field
					}
					readErr(err)
					return
				}
				if one[0] != 0x5A {
					fail(DecodeError{Kind: KindStructure, Offset: fieldStart, Reason: "expected 5A record marker"})
					return
				}
				off++ // strip the 5A marker; the record length is the SFLength
			}
			if _, err := io.ReadFull(stream, two[:]); err != nil {
				if err == io.EOF && !wrapped {
					return // clean end exactly at a field boundary
				}
				if err == io.EOF || err == io.ErrUnexpectedEOF {
					fail(DecodeError{Offset: fieldStart, Kind: KindStructure, Reason: "truncated field length"})
					return
				}
				readErr(err)
				return
			}
			flen := int(binary.BigEndian.Uint16(two[:]))
			if flen < sfiLen {
				fail(DecodeError{
					Kind:   KindStructure,
					Offset: fieldStart,
					Reason: fmt.Sprintf("invalid field length %d: minimum %d", flen, sfiLen),
				})
				return
			}
			if flen > maxSFLength {
				fail(DecodeError{
					Kind:   KindStructure,
					Offset: fieldStart,
					Reason: fmt.Sprintf("field length %d exceeds MO:DCA maximum %d", flen, maxSFLength),
				})
				return
			}
			// One ReadFull-equivalent of the whole field: truncation is
			// discovered here, before introducer validation — today's
			// order. Allocated only after the maxSFLength bound check,
			// so the claim is never trusted for sizing.
			field := make([]byte, flen)
			copy(field, two[:])
			// have counts from the introducer, excluding the 5A marker:
			// today's post-strip len(data)-off.
			have := 2
			for have < flen {
				m, rerr := stream.Read(field[have:flen])
				have += m
				if rerr == io.EOF {
					if have < flen {
						fail(DecodeError{
							Kind:   KindStructure,
							Offset: fieldStart,
							Reason: fmt.Sprintf("truncated field: need %d bytes, have %d", flen, have),
						})
						return
					}
					// EOF arrived with the final body bytes: the field is
					// complete (a legal io.Reader may return n > 0 with
					// EOF). Fall through to introducer validation.
					break
				}
				if rerr != nil {
					readErr(rerr)
					return
				}
			}
			if derr := checkIntroducer(field, fieldStart); derr != nil {
				fail(derr)
				return
			}
			off += flen
			if !yield(StreamField{Data: field, Offset: fieldStart}, nil) {
				return // consumer stopped early: stop reading promptly
			}
		}
	}
}

// readPrefix reads the framing-detection prefix: up to 1+l bytes where l
// is the big-endian uint16 at [1:3] when byte 0 is 0x5A and at least 4
// bytes are available, else up to 6 bytes. A claimed length above the
// architectural maximum is never trusted for sizing the probe: the head
// is returned as-is and the caller applies a bound-aware framing
// fallback. A short stream yields a short prefix (whatever the stream
// holds); a read error aborts the prefix.
func readPrefix(r io.Reader) ([]byte, error) {
	var first [4]byte
	n, err := io.ReadFull(r, first[:])
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return nil, err
	}
	head := first[:n]
	want := 6
	if n >= 4 && head[0] == 0x5A {
		if l := int(binary.BigEndian.Uint16(head[1:3])); l > maxSFLength {
			return head, nil
		} else {
			want = 1 + l
		}
	}
	if len(head) >= want {
		return head, nil
	}
	rest := make([]byte, want-len(head))
	m, err := io.ReadFull(r, rest)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return nil, err
	}
	return append(head, rest[:m]...), nil
}

// SplitFields reads a MO:DCA stream and returns each structured field's raw
// bytes (starting at its introducer). It accepts both naked streams and
// 5A-wrapped variable-blocked records; in wrapped records the 2-byte record
// length doubles as the field's SFLength, so stripping the 5A byte yields
// the field. Every field is validated with decodeIntroducer before its
// bytes are copied, and no allocation is sized by an unvalidated claim.
//
// SplitFields is a thin collecting wrapper over the streaming Fields
// iterator, so it keeps this exact contract — including the
// "afp: SplitFields: %w" read-error text. On huge inputs it materializes
// every field; Fields is the streaming path.
func SplitFields(r io.Reader) ([][]byte, error) {
	var fields [][]byte
	for f, err := range fieldsOp(r, "SplitFields") {
		if err != nil {
			return nil, err
		}
		fields = append(fields, f.Data)
	}
	return fields, nil
}

// checkIntroducer validates a field's introducer, re-basing any error
// offset to the stream position where the field starts.
func checkIntroducer(field []byte, fieldStart int) error {
	if _, err := decodeIntroducer(field); err != nil {
		var derr DecodeError
		if errors.As(err, &derr) {
			derr.Offset += fieldStart
			return derr
		}
		return err
	}
	return nil
}
