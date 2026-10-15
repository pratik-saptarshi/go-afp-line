package afp

import (
	"bytes"
	"errors"
	"io"
	"runtime"
	"testing"
)

// fieldsTestFields builds two synthetic fields (a 17-byte BDT and a 16-byte
// EDT) plus their 5A-wrapped form, mirroring splitterFields in sfi_test.go.
func fieldsTestStreams() (f1, f2, naked, wrapped []byte) {
	f1 = []byte{0x00, 0x11, 0xD3, 0xA8, 0xA8, 0x00, 0x00, 0x00,
		0xC8, 0xC5, 0xD3, 0xD3, 0xD6, 0xE6, 0xD3, 0xC4, 0x40}
	f2 = []byte{0x00, 0x10, 0xD3, 0xA9, 0xA8, 0x00, 0x00, 0x00,
		0xC8, 0xC5, 0xD3, 0xD3, 0xD6, 0xE6, 0xD3, 0xC4}
	naked = append(append([]byte{}, f1...), f2...)
	wrapped = append(wrapped, 0x5A)
	wrapped = append(wrapped, f1...)
	wrapped = append(wrapped, 0x5A)
	wrapped = append(wrapped, f2...)
	return f1, f2, naked, wrapped
}

// collectFields drains a Fields sequence into slices for comparison.
func collectFields(t *testing.T, r io.Reader) ([]StreamField, error) {
	t.Helper()
	var out []StreamField
	var termErr error
	for f, err := range Fields(r) {
		if err != nil {
			termErr = err
			break
		}
		out = append(out, f)
	}
	return out, termErr
}

func TestFields_EquivalentToSplitFields(t *testing.T) {
	f1, f2, naked, wrapped := fieldsTestStreams()
	for name, stream := range map[string][]byte{"naked": naked, "wrapped": wrapped} {
		fields, err := collectFields(t, bytes.NewReader(stream))
		if err != nil {
			t.Fatalf("Fields(%s): unexpected error: %v", name, err)
		}
		if len(fields) != 2 {
			t.Fatalf("Fields(%s): got %d fields, want 2", name, len(fields))
		}
		wantData := [][]byte{f1, f2}
		for i, f := range fields {
			if !bytes.Equal(f.Data, wantData[i]) {
				t.Errorf("Fields(%s)[%d].Data = % x, want % x", name, i, f.Data, wantData[i])
			}
		}
		// Offsets are true wire offsets: on wrapped streams the 5A
		// marker counts, so field 2 starts at 1+len(f1).
		wantOff := []int{0, len(f1)}
		if name == "wrapped" {
			wantOff[1] = 1 + len(f1)
		}
		for i, f := range fields {
			if f.Offset != wantOff[i] {
				t.Errorf("Fields(%s)[%d].Offset = %d, want %d", name, i, f.Offset, wantOff[i])
			}
		}
		// Cross-check against SplitFields: same bytes, same order.
		split, err := SplitFields(bytes.NewReader(stream))
		if err != nil {
			t.Fatalf("SplitFields(%s): unexpected error: %v", name, err)
		}
		for i := range split {
			if !bytes.Equal(fields[i].Data, split[i]) {
				t.Errorf("Fields(%s)[%d] != SplitFields(%s)[%d]", name, i, name, i)
			}
		}
	}
}

func TestFields_EmptyInput(t *testing.T) {
	fields, err := collectFields(t, bytes.NewReader(nil))
	if err != nil {
		t.Fatalf("Fields(empty): unexpected error: %v", err)
	}
	if len(fields) != 0 {
		t.Fatalf("Fields(empty): got %d fields, want 0", len(fields))
	}
}

// TestFields_ErrorEquivalence: for every structural corruption case, Fields
// must yield exactly the DecodeError SplitFields returns — same Kind,
// Reason, and Offset.
func TestFields_ErrorEquivalence(t *testing.T) {
	f1, f2, _, _ := fieldsTestStreams()
	bad5A := append(append([]byte{0x5A}, f1...), 0x00) // corrupt marker on field 2
	bad5A = append(bad5A, f2...)
	shortLen := append(append([]byte{}, f1...), 0x00) // 1 byte where a length belongs
	tinyLen := append([]byte{0x00, 0x07, 0xD3, 0xA8, 0xA8, 0x00, 0x00, 0x00}, f1...)
	// Truncated: claims 0x11 but only 8 bytes follow.
	trunc := []byte{0x00, 0x11, 0xD3, 0xA8, 0xA8, 0x00, 0x00, 0x00}
	// Bad introducer: flen=0x40 (64) passes bounds, full bytes present, but
	// length 64 with flags 0x80 set and no extension bytes -> introducer error.
	badIntro := []byte{0x00, 0x40, 0xD3, 0xA8, 0xA8, 0x80, 0x00, 0x00}
	badIntro = append(badIntro, make([]byte, 64-8)...)
	// Architectural max: flen=40000 > 32767 but < MaxFieldSize; full bytes
	// present -> truncation passes, introducer validation fails.
	archMax := []byte{0x9C, 0x40, 0xD3, 0xA8, 0xA8, 0x00, 0x00, 0x00}
	archMax = append(archMax, make([]byte, 40000-8)...)

	cases := map[string][]byte{
		"bad 5A marker":          bad5A,
		"truncated field length": shortLen,
		"field length below min": tinyLen,
		"truncated field":        trunc,
		"bad introducer":         badIntro,
		"over architectural max": archMax,
	}
	for name, stream := range cases {
		_, splitErr := SplitFields(bytes.NewReader(stream))
		if splitErr == nil {
			t.Fatalf("%s: SplitFields accepted corrupt input", name)
		}
		var want DecodeError
		if !errors.As(splitErr, &want) {
			t.Fatalf("%s: SplitFields error = %T (%v), want DecodeError", name, splitErr, splitErr)
		}
		_, fieldsErr := collectFields(t, bytes.NewReader(stream))
		if fieldsErr == nil {
			t.Fatalf("%s: Fields accepted corrupt input", name)
		}
		var got DecodeError
		if !errors.As(fieldsErr, &got) {
			t.Fatalf("%s: Fields error = %T (%v), want DecodeError", name, fieldsErr, fieldsErr)
		}
		if got.Kind != want.Kind || got.Reason != want.Reason || got.Offset != want.Offset || got.FieldID != want.FieldID {
			t.Errorf("%s:\n Fields error = %+v\n SplitFields  = %+v", name, got, want)
		}
	}
}

// lazyFieldReader generates nFields copies of field on demand, never
// materializing the stream: the peak retained state is one field. It
// serves exactly nFields*len(field) bytes, then io.EOF.
type lazyFieldReader struct {
	field []byte
	left  int // bytes still to serve
	off   int // offset within field for the current copy
}

func newLazyFieldReader(field []byte, nFields int) *lazyFieldReader {
	return &lazyFieldReader{field: field, left: nFields * len(field)}
}

func (r *lazyFieldReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if r.left <= 0 {
		return 0, io.EOF
	}
	if r.off >= len(r.field) {
		r.off = 0
	}
	n := len(r.field) - r.off
	if n > len(p) {
		n = len(p)
	}
	if n > r.left {
		n = r.left
	}
	copy(p, r.field[r.off:r.off+n])
	r.off += n
	r.left -= n
	return n, nil
}

func heapGrowth(t *testing.T, before runtime.MemStats) uint64 {
	t.Helper()
	var after runtime.MemStats
	runtime.GC()
	runtime.GC()
	runtime.ReadMemStats(&after)
	if after.HeapAlloc < before.HeapAlloc {
		return 0
	}
	return after.HeapAlloc - before.HeapAlloc
}

// TestFields_BoundedMemory: 2M minimal 8-byte naked fields = 16 MiB of
// logical field data. The old io.ReadAll + [][]byte shape would retain
// ~64 MiB (2M 24-byte slice headers + 16 MiB data), so a 16 MiB ceiling
// fails such a regression while the streaming iterator stays near zero.
func TestFields_BoundedMemory(t *testing.T) {
	field := []byte{0x00, 0x08, 0xD3, 0xA8, 0xAD, 0x00, 0x00, 0x00} // minimal BNG
	const nFields = 2_000_000
	const ceiling = 16 << 20

	var before runtime.MemStats
	runtime.GC()
	runtime.GC()
	runtime.ReadMemStats(&before)

	count := 0
	for f, err := range Fields(newLazyFieldReader(field, nFields)) {
		if err != nil {
			t.Fatalf("Fields: unexpected error at field %d: %v", count, err)
		}
		if !bytes.Equal(f.Data, field) {
			t.Fatalf("field %d = % x, want % x", count, f.Data, field)
		}
		if f.Offset != count*len(field) {
			t.Fatalf("field %d offset = %d, want %d", count, f.Offset, count*len(field))
		}
		count++
	}
	if count != nFields {
		t.Fatalf("iterated %d fields, want %d", count, nFields)
	}
	if growth := heapGrowth(t, before); growth > ceiling {
		t.Errorf("heap grew by %d bytes over %d streamed fields, want < %d", growth, nFields, ceiling)
	}
}

// TestRoundTrip_BoundedMemory: the same lazy stream through RoundTrip —
// the whole input path, including the fidelity gate, must stream. BNG is
// contracted in-subset, so it decodes to RawField and re-encodes
// byte-identically.
func TestRoundTrip_BoundedMemory(t *testing.T) {
	field := []byte{0x00, 0x08, 0xD3, 0xA8, 0xAD, 0x00, 0x00, 0x00} // minimal BNG
	const nFields = 2_000_000
	const ceiling = 16 << 20

	var before runtime.MemStats
	runtime.GC()
	runtime.GC()
	runtime.ReadMemStats(&before)

	if err := RoundTrip(newLazyFieldReader(field, nFields)); err != nil {
		t.Fatalf("RoundTrip(lazy stream): unexpected error: %v", err)
	}
	if growth := heapGrowth(t, before); growth > ceiling {
		t.Errorf("heap grew by %d bytes over %d streamed fields, want < %d", growth, nFields, ceiling)
	}
}
