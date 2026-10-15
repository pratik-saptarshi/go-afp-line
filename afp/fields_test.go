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
	// Architectural max: flen=40000 > 32767; full bytes present ->
	// the iterator rejects it fail-fast, before introducer validation.
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

// peakHeapReader wraps r and records the maximum HeapAlloc observed *while*
// reads are in flight. Sampling during iteration is what makes the ceiling
// meaningful: a buffering implementation retains every field, so its live
// set (which no GC can reclaim) trips the ceiling mid-iteration, while a
// buffer released when the iterator returns would be reclaimed by a
// post-iteration GC and escape a before/after measurement.
type peakHeapReader struct {
	r     io.Reader
	peak  uint64
	reads int
}

func (p *peakHeapReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.reads++
	if p.reads%4096 == 0 {
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		if ms.HeapAlloc > p.peak {
			p.peak = ms.HeapAlloc
		}
	}
	return n, err
}

// TestFields_BoundedMemory: 2M minimal 8-byte naked fields = 16 MiB of
// logical field data. The streaming iterator retains one field at a time
// (live set ~KBs); the old io.ReadAll + [][]byte shape would retain ~64
// MiB (2M 24-byte slice headers + 16 MiB data) live mid-iteration, tripping
// the 16 MiB ceiling.
func TestFields_BoundedMemory(t *testing.T) {
	field := []byte{0x00, 0x08, 0xD3, 0xA8, 0xAD, 0x00, 0x00, 0x00} // minimal BNG
	const nFields = 2_000_000
	const ceiling = 16 << 20

	runtime.GC() // start clean so samples reflect the iteration
	pr := &peakHeapReader{r: newLazyFieldReader(field, nFields)}
	count := 0
	for f, err := range Fields(pr) {
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
	if pr.peak > ceiling {
		t.Errorf("peak heap %d bytes while streaming %d fields, want < %d", pr.peak, nFields, ceiling)
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

	runtime.GC() // start clean so samples reflect the iteration
	pr := &peakHeapReader{r: newLazyFieldReader(field, nFields)}
	if err := RoundTrip(pr); err != nil {
		t.Fatalf("RoundTrip(lazy stream): unexpected error: %v", err)
	}
	if pr.peak > ceiling {
		t.Errorf("peak heap %d bytes while streaming %d fields, want < %d", pr.peak, nFields, ceiling)
	}
}

// errWithBytesReader models readers (e.g. checksum-validating) that return
// a non-EOF error together with bytes, which io.Reader permits: the first
// Read returns len(p) bytes and err, subsequent Reads return EOF.
type errWithBytesReader struct {
	data []byte
	err  error
	done bool
}

func (r *errWithBytesReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, io.EOF
	}
	r.done = true
	return copy(p, r.data), r.err
}

// TestFields_ErrorWithPrefixBytes: an error arriving with the framing
// prefix bytes must surface (wrapped, not a DecodeError) instead of being
// swallowed by the prefix read. io.ReadFull discards an error that arrives
// with a full buffer; the old io.ReadAll path surfaced it.
func TestFields_ErrorWithPrefixBytes(t *testing.T) {
	field := []byte{0x00, 0x08, 0xD3, 0xA8, 0xAD, 0x00, 0x00, 0x00} // minimal BNG
	boom := errors.New("boom")
	var nFields int
	var gotErr error
	for f, err := range Fields(&errWithBytesReader{data: field, err: boom}) {
		if err != nil {
			gotErr = err
			continue
		}
		nFields++
		_ = f
	}
	if nFields != 0 {
		t.Errorf("Fields yielded %d fields before the read error, want 0", nFields)
	}
	if gotErr == nil || !errors.Is(gotErr, boom) {
		t.Fatalf("Fields error = %v, want wrapped boom", gotErr)
	}
	var derr DecodeError
	if errors.As(gotErr, &derr) {
		t.Errorf("Fields error = DecodeError %+v, want a plain read error", derr)
	}
}

// eofWithBytesReader returns all remaining bytes together with io.EOF on
// every Read: legal per the io.Reader contract (n > 0 with EOF) and common
// in practice. A complete field delivered this way must be accepted, not
// reported as truncated.
type eofWithBytesReader struct {
	data []byte
}

func (r *eofWithBytesReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, io.EOF
}

// TestFields_EOFWithFinalBodyBytes: EOF arriving with the field's final
// body bytes is not truncation — the complete field is yielded and the
// stream then ends cleanly.
func TestFields_EOFWithFinalBodyBytes(t *testing.T) {
	field := []byte{0x00, 0x08, 0xD3, 0xA8, 0xAD, 0x00, 0x00, 0x00} // minimal BNG
	var fields [][]byte
	var gotErr error
	for f, err := range Fields(&eofWithBytesReader{data: append([]byte{}, field...)}) {
		if err != nil {
			gotErr = err
			continue
		}
		fields = append(fields, f.Data)
	}
	if gotErr != nil {
		t.Fatalf("Fields error = %v, want clean end after a complete field", gotErr)
	}
	if len(fields) != 1 || !bytes.Equal(fields[0], field) {
		t.Fatalf("Fields yielded %d fields, want the 1 complete field", len(fields))
	}
}

// scriptedErrReader serves clean full reads until limit bytes, then returns
// err together with the next chunk's bytes: the bytes must be processed
// first (a complete field is yielded) and the error surfaces afterwards,
// per the io.Reader contract.
type scriptedErrReader struct {
	data  []byte
	off   int
	limit int
	err   error
}

func (r *scriptedErrReader) Read(p []byte) (int, error) {
	if r.off >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.off:])
	r.off += n
	if r.off >= r.limit {
		return n, r.err
	}
	return n, nil
}

// TestFields_ErrorAfterCompleteField: when a non-EOF error arrives with a
// chunk that completes a field, the field is yielded first and the error
// surfaces on the next step — bytes are processed before the error is
// considered. Without the staging, the error would be silently dropped and
// iteration would end cleanly.
func TestFields_ErrorAfterCompleteField(t *testing.T) {
	field := []byte{0x00, 0x08, 0xD3, 0xA8, 0xAD, 0x00, 0x00, 0x00} // minimal BNG
	stream := append(append([]byte{}, field...), field...)
	boom := errors.New("boom")
	// Clean reads through the first field (8 bytes); the error arrives
	// with the second field's bytes.
	r := &scriptedErrReader{data: stream, limit: 8, err: boom}

	var fields [][]byte
	var gotErr error
	for f, err := range Fields(r) {
		if err != nil {
			gotErr = err
			continue
		}
		fields = append(fields, f.Data)
	}
	if len(fields) != 1 || !bytes.Equal(fields[0], field) {
		t.Fatalf("Fields yielded %d fields, want the 1 complete field before the error", len(fields))
	}
	if gotErr == nil || !errors.Is(gotErr, boom) {
		t.Fatalf("Fields error = %v, want wrapped boom after the complete field", gotErr)
	}
}

// TestFields_ArchitecturalMaxFailFast pins the fail-fast contract for a
// claimed length above the MO:DCA architectural maximum (32767): the
// iterator rejects it at the field's start offset before allocating or
// reading the body — even when the stream is shorter than the claim, the
// error is the length-bound violation, not truncation.
func TestFields_ArchitecturalMaxFailFast(t *testing.T) {
	// Claims 40000 bytes; only the 2-byte length prefix is present.
	stream := []byte{0x9C, 0x40}
	_, err := collectFields(t, bytes.NewReader(stream))
	if err == nil {
		t.Fatal("Fields accepted a length above the architectural maximum")
	}
	var derr DecodeError
	if !errors.As(err, &derr) {
		t.Fatalf("error = %T (%v), want DecodeError", err, err)
	}
	if derr.Kind != KindStructure {
		t.Errorf("Kind = %q, want %q", derr.Kind, KindStructure)
	}
	if derr.Offset != 0 {
		t.Errorf("Offset = %d, want 0 (field start)", derr.Offset)
	}
	if derr.FieldID != "" {
		t.Errorf("FieldID = %q, want empty (rejected before introducer decode)", derr.FieldID)
	}
	wantReason := "field length 40000 exceeds MO:DCA maximum 32767"
	if derr.Reason != wantReason {
		t.Errorf("Reason = %q, want %q", derr.Reason, wantReason)
	}
}

// TestFields_WrappedOverMaxLength pins the fail-fast contract for a
// 5A-wrapped record claiming more than the architectural maximum: the
// bound violation is reported even when the record is truncated short,
// and framing is not misclassified as naked (which would report
// truncation instead).
func TestFields_WrappedOverMaxLength(t *testing.T) {
	// 5A marker, length 40000 (0x9C40), BDT introducer; body absent.
	stream := []byte{0x5A, 0x9C, 0x40, 0xD3, 0xA8, 0xA8, 0x00, 0x00}
	_, err := collectFields(t, bytes.NewReader(stream))
	if err == nil {
		t.Fatal("Fields accepted a wrapped length above the architectural maximum")
	}
	var derr DecodeError
	if !errors.As(err, &derr) {
		t.Fatalf("error = %T (%v), want DecodeError", err, err)
	}
	if derr.Kind != KindStructure {
		t.Errorf("Kind = %q, want %q", derr.Kind, KindStructure)
	}
	if derr.Offset != 0 {
		t.Errorf("Offset = %d, want 0 (field start)", derr.Offset)
	}
	if derr.FieldID != "" {
		t.Errorf("FieldID = %q, want empty (rejected before introducer decode)", derr.FieldID)
	}
	wantReason := "field length 40000 exceeds MO:DCA maximum 32767"
	if derr.Reason != wantReason {
		t.Errorf("Reason = %q, want %q", derr.Reason, wantReason)
	}
}
