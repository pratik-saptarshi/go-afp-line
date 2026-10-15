package afp

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// bdtIntroducer is the 8-byte SFI of a 17-byte BDT field:
// SFLength=17 (big-endian, includes the introducer), TypeID=D3A8A8, Flags=0x00.
var bdtIntroducer = []byte{0x00, 0x11, 0xD3, 0xA8, 0xA8, 0x00, 0x00, 0x00}

func TestDecodeIntroducer_BDT(t *testing.T) {
	in, err := decodeIntroducer(bdtIntroducer)
	if err != nil {
		t.Fatalf("decodeIntroducer: unexpected error: %v", err)
	}
	if in.Length != 17 {
		t.Errorf("Length = %d, want 17", in.Length)
	}
	if got := in.TypeIDString(); got != "D3A8A8" {
		t.Errorf("TypeID = %q, want %q", got, "D3A8A8")
	}
	if in.Flags != 0x00 {
		t.Errorf("Flags = %#02x, want 0x00", in.Flags)
	}
	if in.HasExtension() {
		t.Errorf("HasExtension = true, want false")
	}
}

func TestDecodeIntroducer_Truncated(t *testing.T) {
	_, err := decodeIntroducer([]byte{0x00, 0x11, 0xD3})
	var derr DecodeError
	if !errors.As(err, &derr) {
		t.Fatalf("expected DecodeError, got %T (%v)", err, err)
	}
	if derr.Offset != 0 {
		t.Errorf("DecodeError.Offset = %d, want 0", derr.Offset)
	}
}

func TestIntroducer_RoundTrip(t *testing.T) {
	vectors := [][]byte{
		bdtIntroducer,
		// Extension bit set (bit 0 of flags = 0x80 per IBM bit numbering):
		// SFLength=18, ExtLength byte=0x02, one extension data byte.
		{0x00, 0x12, 0xD3, 0xA8, 0xA8, 0x80, 0x00, 0x00, 0x02, 0x00},
	}
	for _, b := range vectors {
		in, err := decodeIntroducer(b)
		if err != nil {
			t.Fatalf("decodeIntroducer(% x): unexpected error: %v", b, err)
		}
		if got := in.Encode(); !bytes.Equal(got, b) {
			t.Errorf("Encode(Decode(% x)) = % x, want % x", b, got, b)
		}
	}
	// The extension vector must actually have parsed an extension.
	in, _ := decodeIntroducer(vectors[1])
	if !in.HasExtension() {
		t.Errorf("HasExtension = false for flags 0x80, want true")
	}
	if len(in.Extension) != 2 {
		t.Errorf("len(Extension) = %d, want 2", len(in.Extension))
	}
}

// Two synthetic fields: a 17-byte BDT and a 16-byte EDT.
func splitterFields() (f1, f2 []byte) {
	f1 = []byte{0x00, 0x11, 0xD3, 0xA8, 0xA8, 0x00, 0x00, 0x00,
		0xC8, 0xC5, 0xD3, 0xD3, 0xD6, 0xE6, 0xD3, 0xC4, 0x40} // DocName "HELLOWLD" + pad
	f2 = []byte{0x00, 0x10, 0xD3, 0xA9, 0xA8, 0x00, 0x00, 0x00,
		0xC8, 0xC5, 0xD3, 0xD3, 0xD6, 0xE6, 0xD3, 0xC4} // DocName "HELLOWLD"
	return f1, f2
}

func TestSplitter_WrappedAndNaked(t *testing.T) {
	f1, f2 := splitterFields()
	var naked, wrapped []byte
	naked = append(naked, f1...)
	naked = append(naked, f2...)
	// In 5A-wrapped records the 2-byte record length doubles as the SFLength,
	// so the wrapped form is the field bytes prefixed with a 5A byte.
	wrapped = append(wrapped, 0x5A)
	wrapped = append(wrapped, f1...)
	wrapped = append(wrapped, 0x5A)
	wrapped = append(wrapped, f2...)

	gotNaked, err := SplitFields(bytes.NewReader(naked))
	if err != nil {
		t.Fatalf("SplitFields(naked): unexpected error: %v", err)
	}
	gotWrapped, err := SplitFields(bytes.NewReader(wrapped))
	if err != nil {
		t.Fatalf("SplitFields(wrapped): unexpected error: %v", err)
	}
	want := [][]byte{f1, f2}
	for name, got := range map[string][][]byte{"naked": gotNaked, "wrapped": gotWrapped} {
		if len(got) != 2 {
			t.Fatalf("SplitFields(%s): got %d fields, want 2", name, len(got))
		}
		for i := range want {
			if !bytes.Equal(got[i], want[i]) {
				t.Errorf("SplitFields(%s)[%d] = % x, want % x", name, i, got[i], want[i])
			}
		}
	}
}

func TestDecodeIntroducer_LyingLength(t *testing.T) {
	// Claims 0xFFFFFF bytes but only 8 are present. Any implementation that
	// allocates or indexes by the claimed length would panic or OOM here;
	// returning DecodeError on this input proves the claim is never trusted.
	b := []byte{0xFF, 0xFF, 0xFF, 0xD3, 0xA8, 0xA8, 0x00, 0x00}
	_, err := decodeIntroducer(b)
	var derr DecodeError
	if !errors.As(err, &derr) {
		t.Fatalf("expected DecodeError, got %T (%v)", err, err)
	}
}

func TestSplitFields_LyingLength(t *testing.T) {
	// Naked field claims 0xFFFF bytes; only 8 follow. Must fail, not OOM.
	b := []byte{0xFF, 0xFF, 0xD3, 0xA8, 0xA8, 0x00, 0x00, 0x00}
	_, err := SplitFields(bytes.NewReader(b))
	var derr DecodeError
	if !errors.As(err, &derr) {
		t.Fatalf("expected DecodeError, got %T (%v)", err, err)
	}
}

func TestSplitFields_Truncated(t *testing.T) {
	f1, _ := splitterFields()
	// Cut the second field's length short mid-stream.
	b := append(append([]byte{}, f1...), 0x00, 0x11, 0xD3)
	_, err := SplitFields(bytes.NewReader(b))
	var derr DecodeError
	if !errors.As(err, &derr) {
		t.Fatalf("expected DecodeError, got %T (%v)", err, err)
	}
}

func TestSplitFields_Empty(t *testing.T) {
	fields, err := SplitFields(bytes.NewReader(nil))
	if err != nil {
		t.Fatalf("SplitFields(empty): unexpected error: %v", err)
	}
	if len(fields) != 0 {
		t.Errorf("SplitFields(empty) = %d fields, want 0", len(fields))
	}
}

func TestMaxFieldSize(t *testing.T) {
	if MaxFieldSize != 1<<20 {
		t.Errorf("MaxFieldSize = %d, want %d", MaxFieldSize, 1<<20)
	}
}

func FuzzDecodeIntroducer(f *testing.F) {
	f.Add(bdtIntroducer)
	f.Add([]byte{})
	f.Add([]byte{0x00})
	f.Add([]byte{0x00, 0x12, 0xD3, 0xA8, 0xA8, 0x80, 0x00, 0x00, 0x02, 0x00})
	f.Fuzz(func(t *testing.T, b []byte) {
		in, err := decodeIntroducer(b)
		if err != nil {
			// Every failure must be a typed DecodeError, never a panic
			// and never an untyped error.
			var derr DecodeError
			if !errors.As(err, &derr) {
				t.Fatalf("non-DecodeError returned: %T (%v)", err, err)
			}
			return
		}
		// Valid output invariants: Encode reproduces the header bytes,
		// and Length matches the length field.
		enc := in.Encode()
		if len(enc) > len(b) {
			t.Fatalf("Encode() longer than input: %d > %d", len(enc), len(b))
		}
		if !bytes.Equal(enc, b[:len(enc)]) {
			t.Fatalf("Encode() mismatch: got % x, input prefix % x", enc, b[:len(enc)])
		}
		if in.Length < 8 || in.Length > 32767 {
			t.Fatalf("Length out of range: %d", in.Length)
		}
	})
}

// TestSplitFields_CorruptRecordMarker: a 5A-wrapped stream whose record marker
// byte is corrupt (not 0x5A) must fail loudly with KindStructure naming the
// record's start offset — the byte must be verified before stripping, never
// silently skipped.
func TestSplitFields_CorruptRecordMarker(t *testing.T) {
	f1, f2 := splitterFields()
	var wrapped []byte
	wrapped = append(wrapped, 0x5A)
	wrapped = append(wrapped, f1...)
	wrapped = append(wrapped, 0x00) // corrupt marker: expected 0x5A
	wrapped = append(wrapped, f2...)

	_, err := SplitFields(bytes.NewReader(wrapped))
	var derr DecodeError
	if !errors.As(err, &derr) {
		t.Fatalf("expected DecodeError, got %T (%v)", err, err)
	}
	if derr.Kind != KindStructure {
		t.Errorf("Kind = %q, want %q", derr.Kind, KindStructure)
	}
	wantOffset := 1 + len(f1) // start of the corrupt record
	if derr.Offset != wantOffset {
		t.Errorf("Offset = %d, want %d (start of the corrupt record)", derr.Offset, wantOffset)
	}
	if !strings.Contains(derr.Reason, "expected 5A record marker") {
		t.Errorf("Reason = %q, want it to name the 5A record marker", derr.Reason)
	}
}

// TestSplitFields_ValidMarkersStillParse: the marker check must not disturb
// well-formed wrapped or naked streams — both parse as before.
func TestSplitFields_ValidMarkersStillParse(t *testing.T) {
	f1, f2 := splitterFields()
	var wrapped []byte
	wrapped = append(wrapped, 0x5A)
	wrapped = append(wrapped, f1...)
	wrapped = append(wrapped, 0x5A)
	wrapped = append(wrapped, f2...)
	var naked []byte
	naked = append(naked, f1...)
	naked = append(naked, f2...)

	for name, data := range map[string][]byte{"wrapped": wrapped, "naked": naked} {
		fields, err := SplitFields(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("SplitFields(%s): unexpected error: %v", name, err)
		}
		if len(fields) != 2 || !bytes.Equal(fields[0], f1) || !bytes.Equal(fields[1], f2) {
			t.Errorf("SplitFields(%s): fields mismatch", name)
		}
	}
}

// TestIntroducerEncode_RangePanics: Encode must fail loudly (not wrap) on
// wire-unrepresentable Length, mirroring decodeIntroducer's 8..32767 range.
func TestIntroducerEncode_RangePanics(t *testing.T) {
	base := Introducer{TypeID: [3]byte{0xD3, 0xA8, 0xA8}}
	for _, length := range []int{0, 7, 32768, 70000} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("Introducer{Length: %d}.Encode() did not panic", length)
				}
			}()
			in := base
			in.Length = length
			_ = in.Encode()
		}()
	}
	// Boundary values still encode.
	for _, length := range []int{8, 32767} {
		in := base
		in.Length = length
		if got := in.Encode(); len(got) < 2 || int(got[0])<<8|int(got[1]) != length {
			t.Errorf("Introducer{Length: %d}.Encode() = %x, want length prefix %d", length, got, length)
		}
	}
}
