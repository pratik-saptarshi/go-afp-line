package afp

import (
	"bytes"
	"errors"
	"testing"
)

// SFI flag fixtures: the PadFlag (bit 4, 0x08) and SegFlag (bit 2, 0x20)
// behavior. BGR (D3A8BB) is known-excluded and decodes to *RawField, so
// Data assertions observe decodeField's shared path verbatim; PTX
// (D3EE9B) is contracted in-subset and serves the segmentation tests at
// the Fields iterator level.

// paddedBGR builds a BGR field with the given flags and data bytes.
func paddedBGR(flags byte, data []byte) []byte {
	b := []byte{0x00, 0x00, 0xD3, 0xA8, 0xBB, flags, 0x00, 0x00}
	b = append(b, data...)
	b[0] = byte(len(b) >> 8)
	b[1] = byte(len(b))
	return b
}

func decodeErr(t *testing.T, err error) DecodeError {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var derr DecodeError
	if !errors.As(err, &derr) {
		t.Fatalf("expected DecodeError, got %T: %v", err, err)
	}
	return derr
}

// Fix 1: PadFlag set, 1-byte form — 2 trailing pad bytes, the last byte
// (0x02) is the padding length. RawField.Data must exclude the padding.
func TestDecodeField_StripsOneByteFormPadding(t *testing.T) {
	data := []byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0x02}
	f, err := DecodeField(paddedBGR(0x08, data))
	if err != nil {
		t.Fatalf("DecodeField: unexpected error: %v", err)
	}
	rf, ok := f.(*RawField)
	if !ok {
		t.Fatalf("DecodeField: got %T, want *RawField", f)
	}
	if want := []byte{0xAA, 0xBB, 0xCC, 0xDD}; !bytes.Equal(rf.Data, want) {
		t.Errorf("RawField.Data = % x, want % x (padding not stripped)", rf.Data, want)
	}
}

// Fix 1: PadFlag set, 3-byte form — padding ends [hi, lo, 0x00] with
// (hi<<8)|lo == padding length. 260 bytes of padding around a 4-byte
// payload.
func TestDecodeField_StripsThreeByteFormPadding(t *testing.T) {
	payload := []byte{0xAA, 0xBB, 0xCC, 0xDD}
	const padLen = 260 // 0x0104
	data := append(append([]byte{}, payload...), bytes.Repeat([]byte{0x55}, padLen-3)...)
	data = append(data, 0x01, 0x04, 0x00)
	f, err := DecodeField(paddedBGR(0x08, data))
	if err != nil {
		t.Fatalf("DecodeField: unexpected error: %v", err)
	}
	rf := f.(*RawField)
	if !bytes.Equal(rf.Data, payload) {
		t.Errorf("RawField.Data len = %d, want %d (3-byte padding not stripped)", len(rf.Data), len(payload))
	}
}

// Fix 1: PadFlag set, last byte claims more padding than the data holds —
// loud KindStructure, never silent acceptance.
func TestDecodeField_PadLenExceedsDataErrors(t *testing.T) {
	_, err := DecodeField(paddedBGR(0x08, []byte{0xAA, 0x05}))
	derr := decodeErr(t, err)
	if derr.Kind != KindStructure {
		t.Errorf("error kind = %q, want %q", derr.Kind, KindStructure)
	}
	if derr.FieldID != "D3A8BB" {
		t.Errorf("error FieldID = %q, want D3A8BB", derr.FieldID)
	}
}

// Fix 1: PadFlag set on a field with no data bytes — loud KindStructure,
// not an index-out-of-range panic.
func TestDecodeField_PadFlagEmptyDataErrors(t *testing.T) {
	_, err := DecodeField(paddedBGR(0x08, nil))
	derr := decodeErr(t, err)
	if derr.Kind != KindStructure {
		t.Errorf("error kind = %q, want %q", derr.Kind, KindStructure)
	}
	if derr.FieldID != "D3A8BB" {
		t.Errorf("error FieldID = %q, want D3A8BB", derr.FieldID)
	}
}

// Fix 1: 3-byte form claiming a 1–2 byte padding must use the 1-byte
// form — loud KindStructure.
func TestDecodeField_ThreeByteFormShortLengthErrors(t *testing.T) {
	_, err := DecodeField(paddedBGR(0x08, []byte{0xAA, 0xBB, 0x00, 0x02, 0x00}))
	derr := decodeErr(t, err)
	if derr.Kind != KindStructure {
		t.Errorf("error kind = %q, want %q", derr.Kind, KindStructure)
	}
}

// Fix 1: negative control — no PadFlag, trailing 0x02 is payload, not a
// padding-length byte. Data must pass through untouched.
func TestDecodeField_NoPadFlagSmallTailByteUntouched(t *testing.T) {
	data := []byte{0xAA, 0xBB, 0x02}
	f, err := DecodeField(paddedBGR(0x00, data))
	if err != nil {
		t.Fatalf("DecodeField: unexpected error: %v", err)
	}
	if rf := f.(*RawField); !bytes.Equal(rf.Data, data) {
		t.Errorf("RawField.Data = % x, want % x (flag-less tail must be untouched)", rf.Data, data)
	}
}

// segField builds a PTX (D3EE9B) field with the given flags and data.
func segField(flags byte, data []byte) []byte {
	b := []byte{0x00, 0x00, 0xD3, 0xEE, 0x9B, flags, 0x00, 0x00}
	b = append(b, data...)
	b[0] = byte(len(b) >> 8)
	b[1] = byte(len(b))
	return b
}

// edtField builds a small EDT (D3A9A8) field: a different type ID used to
// probe the interspersed-field rule.
func edtField(flags byte, data []byte) []byte {
	b := []byte{0x00, 0x00, 0xD3, 0xA9, 0xA8, flags, 0x00, 0x00}
	b = append(b, data...)
	b[0] = byte(len(b) >> 8)
	b[1] = byte(len(b))
	return b
}

// Fix 2: two SegFlag=1 fragments plus a SegFlag=0 terminator, split at
// byte boundaries that do not align to triplet boundaries, must yield a
// single logical field: concatenated data, SegFlag and PadFlag cleared,
// Length = header + concatenated data, Offset = first fragment's offset.
func TestFields_ReassemblesSegmentedSequence(t *testing.T) {
	frag1 := segField(0x20, []byte{0x01, 0x02, 0x03})
	frag2 := segField(0x20, []byte{0x04, 0x05})
	term := segField(0x00, []byte{0x06, 0x07})
	stream := append(append(append([]byte{}, frag1...), frag2...), term...)

	fields, err := collectFields(t, bytes.NewReader(stream))
	if err != nil {
		t.Fatalf("Fields: unexpected error: %v", err)
	}
	if len(fields) != 1 {
		t.Fatalf("Fields: got %d fields, want 1 reassembled logical field", len(fields))
	}
	want := []byte{0x00, 0x0F, 0xD3, 0xEE, 0x9B, 0x00, 0x00, 0x00,
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07}
	if !bytes.Equal(fields[0].Data, want) {
		t.Errorf("Fields[0].Data = % x, want % x", fields[0].Data, want)
	}
	if fields[0].Offset != 0 {
		t.Errorf("Fields[0].Offset = %d, want 0 (first fragment's offset)", fields[0].Offset)
	}
}

// Fix 2: a different-type field between segmented fragments is forbidden —
// loud KindStructure, and the partial sequence is never yielded.
func TestFields_SegmentedInterspersedFieldErrors(t *testing.T) {
	frag1 := segField(0x20, []byte{0x01, 0x02})
	intruder := edtField(0x00, []byte{0xAA})
	term := segField(0x00, []byte{0x03})
	stream := append(append(append([]byte{}, frag1...), intruder...), term...)

	fields, err := collectFields(t, bytes.NewReader(stream))
	if err == nil {
		t.Fatalf("Fields: expected error, got %d fields", len(fields))
	}
	derr := decodeErr(t, err)
	if derr.Kind != KindStructure {
		t.Errorf("error kind = %q, want %q", derr.Kind, KindStructure)
	}
	if len(fields) != 0 {
		t.Errorf("Fields yielded %d fields before the interspersed-field error, want 0", len(fields))
	}
}

// Fix 2: stream ends while a segmented sequence is open — loud
// KindStructure, never a silently dropped tail.
func TestFields_SegmentedTruncatedAtEOFErrors(t *testing.T) {
	frag1 := segField(0x20, []byte{0x01, 0x02})

	fields, err := collectFields(t, bytes.NewReader(frag1))
	if err == nil {
		t.Fatalf("Fields: expected error, got %d fields", len(fields))
	}
	derr := decodeErr(t, err)
	if derr.Kind != KindStructure {
		t.Errorf("error kind = %q, want %q", derr.Kind, KindStructure)
	}
	if len(fields) != 0 {
		t.Errorf("Fields yielded %d fields before the truncation error, want 0", len(fields))
	}
}

// Fix 2: regression pin — a stream with no SegFlag set passes through
// byte-identical, offsets included.
func TestFields_NonSegmentedPassthroughByteIdentical(t *testing.T) {
	plain := segField(0x00, []byte{0x01, 0x02, 0x03})
	stream := append(append([]byte{}, plain...), edtField(0x00, []byte{0xAA})...)

	fields, err := collectFields(t, bytes.NewReader(stream))
	if err != nil {
		t.Fatalf("Fields: unexpected error: %v", err)
	}
	split, err := SplitFields(bytes.NewReader(stream))
	if err != nil {
		t.Fatalf("SplitFields: unexpected error: %v", err)
	}
	if len(fields) != len(split) {
		t.Fatalf("Fields yielded %d fields, SplitFields %d", len(fields), len(split))
	}
	for i := range split {
		if !bytes.Equal(fields[i].Data, split[i]) {
			t.Errorf("Fields[%d].Data = % x, want SplitFields bytes % x", i, fields[i].Data, split[i])
		}
	}
	if fields[1].Offset != len(plain) {
		t.Errorf("Fields[1].Offset = %d, want %d", fields[1].Offset, len(plain))
	}
}

// Fix 2: contract pin — SplitFields keeps yielding physical fragments,
// never reassembled logical fields.
func TestSplitFields_YieldsPhysicalFragments(t *testing.T) {
	frag1 := segField(0x20, []byte{0x01, 0x02, 0x03})
	frag2 := segField(0x20, []byte{0x04, 0x05})
	term := segField(0x00, []byte{0x06, 0x07})
	stream := append(append(append([]byte{}, frag1...), frag2...), term...)

	split, err := SplitFields(bytes.NewReader(stream))
	if err != nil {
		t.Fatalf("SplitFields: unexpected error: %v", err)
	}
	want := [][]byte{frag1, frag2, term}
	if len(split) != len(want) {
		t.Fatalf("SplitFields: got %d fragments, want %d", len(split), len(want))
	}
	for i := range want {
		if !bytes.Equal(split[i], want[i]) {
			t.Errorf("SplitFields[%d] = % x, want % x", i, split[i], want[i])
		}
	}
}

// Fix 2: padding is stripped per physical fragment, with that fragment's
// own introducer, before concatenation. The synthetic logical field is
// padding-free with PadFlag cleared.
func TestFields_SegmentedStripsPerFragmentPadding(t *testing.T) {
	frag1 := segField(0x20|0x08, []byte{0x01, 0xEE, 0x02}) // PadFlag: 2 pad bytes
	term := segField(0x00, []byte{0x02, 0x03})
	stream := append(append([]byte{}, frag1...), term...)

	fields, err := collectFields(t, bytes.NewReader(stream))
	if err != nil {
		t.Fatalf("Fields: unexpected error: %v", err)
	}
	if len(fields) != 1 {
		t.Fatalf("Fields: got %d fields, want 1", len(fields))
	}
	want := []byte{0x00, 0x0B, 0xD3, 0xEE, 0x9B, 0x00, 0x00, 0x00,
		0x01, 0x02, 0x03}
	if !bytes.Equal(fields[0].Data, want) {
		t.Errorf("Fields[0].Data = % x, want % x", fields[0].Data, want)
	}
}
