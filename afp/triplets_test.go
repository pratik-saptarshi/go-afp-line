package afp

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

// assertPanics fails the test unless f panics. Encode-side failures are
// programmer errors (post-decode mutation into wire-unrepresentable state),
// so the contract fails loudly with a panic rather than returning an error.
func assertPanics(t *testing.T, what string, f func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Errorf("%s: expected panic, none occurred", what)
			return
		}
		if s, ok := r.(string); ok && !strings.HasPrefix(s, "afp: ") {
			t.Errorf("%s: panic %q does not carry the afp: prefix", what, s)
		}
	}()
	f()
}

// Comment triplet X'65' carrying "HELLO" in EBCDIC-037:
// Tlength=7 (includes itself), Tid=0x65, 5 content bytes.
var commentHello = []byte{0x07, 0x65, 0xC8, 0xC5, 0xD3, 0xD3, 0xD6}

// BDT with a Comment triplet ("HI"): SFLength = 8+8+2+4 = 22.
var bdtWithComment = []byte{
	0x00, 0x16, 0xD3, 0xA8, 0xA8, 0x00, 0x00, 0x00,
	0xC8, 0xC5, 0xD3, 0xD3, 0xD6, 0xE6, 0xD3, 0xC4, // "HELLOWLD"
	0x00, 0x00,
	0x04, 0x65, 0xC8, 0xC9, // Comment "HI"
}

func TestDecodeTriplet_Comment(t *testing.T) {
	tr, n, err := decodeTriplet(commentHello)
	if err != nil {
		t.Fatalf("decodeTriplet(Comment) error: %v", err)
	}
	if n != len(commentHello) {
		t.Errorf("consumed = %d, want %d", n, len(commentHello))
	}
	if tr.ID != 0x65 {
		t.Errorf("ID = %02X, want 65", tr.ID)
	}
	if got := tr.Text(); got != "HELLO" {
		t.Errorf("Text() = %q, want %q", got, "HELLO")
	}
}

func TestDecodeTriplet_UnknownID(t *testing.T) {
	// Tid 0xFF is not in the registered set: typed error, not silent skip.
	_, _, err := decodeTriplet([]byte{0x03, 0xFF, 0x00})
	derr, ok := err.(DecodeError)
	if !ok {
		t.Fatalf("decodeTriplet(unknown ID) error = %T (%v), want DecodeError", err, err)
	}
	if derr.Offset != 0 {
		t.Errorf("Offset = %d, want 0", derr.Offset)
	}
}

func TestDecodeTriplet_Truncated(t *testing.T) {
	// Tlength claims 5, only 3 bytes present.
	if _, _, err := decodeTriplet([]byte{0x05, 0x65, 0xC8}); err == nil {
		t.Errorf("decodeTriplet(truncated) = nil error, want DecodeError")
	}
}

func TestDecodeTriplet_BadLength(t *testing.T) {
	// Tlength=2 is below the architectural minimum of 3.
	if _, _, err := decodeTriplet([]byte{0x02, 0x65}); err == nil {
		t.Errorf("decodeTriplet(Tlength=2) = nil error, want DecodeError")
	}
}

func TestTriplet_BDT_RoundTrip(t *testing.T) {
	// Triplet-bearing BDT must round-trip byte-identically THROUGH the
	// field path (DecodeField -> Encode), with triplets parsed, not raw.
	f, err := DecodeField(bdtWithComment)
	if err != nil {
		t.Fatalf("DecodeField(BDT+triplet) error: %v", err)
	}
	bdt, ok := f.(*BDT)
	if !ok {
		t.Fatalf("type = %T, want *BDT", f)
	}
	if len(bdt.Triplets) != 1 {
		t.Fatalf("len(Triplets) = %d, want 1", len(bdt.Triplets))
	}
	if bdt.Triplets[0].ID != 0x65 {
		t.Errorf("Triplets[0].ID = %02X, want 65", bdt.Triplets[0].ID)
	}
	if got := bdt.Triplets[0].Text(); got != "HI" {
		t.Errorf("Triplets[0].Text() = %q, want %q", got, "HI")
	}
	if enc := f.Encode(); !bytes.Equal(enc, bdtWithComment) {
		t.Errorf("Encode() = %X, want %X", enc, bdtWithComment)
	}
}

func TestTriplet_UnknownIDInField(t *testing.T) {
	// Unknown triplet ID inside a field's triplet run: the field decode
	// fails loudly with the field's ID attached.
	bad := []byte{
		0x00, 0x15, 0xD3, 0xA8, 0xA8, 0x00, 0x00, 0x00,
		0xC8, 0xC5, 0xD3, 0xD3, 0xD6, 0xE6, 0xD3, 0xC4,
		0x00, 0x00,
		0x03, 0xFF, 0x00,
	}
	_, err := DecodeField(bad)
	derr, ok := err.(DecodeError)
	if !ok {
		t.Fatalf("DecodeField(BDT+unknown triplet) error = %T (%v), want DecodeError", err, err)
	}
	if derr.FieldID != "D3A8A8" {
		t.Errorf("FieldID = %q, want D3A8A8", derr.FieldID)
	}
}

func TestRoundTrip_HelloWorld(t *testing.T) {
	// The strategy's hello-world document: BDT + EDT, byte-identical
	// through split -> decode (incl. triplets) -> re-encode.
	doc := append(append([]byte{}, bdtHello...), edtHello...)
	if err := RoundTrip(bytes.NewReader(doc)); err != nil {
		t.Fatalf("RoundTrip(hello-world) error: %v", err)
	}
}

func TestRoundTrip_Truncated(t *testing.T) {
	doc := append(append([]byte{}, bdtHello...), edtHello...)
	if err := RoundTrip(bytes.NewReader(doc[:len(doc)-5])); err == nil {
		t.Errorf("RoundTrip(truncated) = nil error, want error")
	}
}

func TestRoundTrip_UnknownField(t *testing.T) {
	doc := append(append([]byte{}, bdtHello...), unknownField...)
	if err := RoundTrip(bytes.NewReader(doc)); err == nil {
		t.Errorf("RoundTrip(unknown field) = nil error, want DecodeError")
	}
}

func TestTriplet_EncodeOverflowPanics(t *testing.T) {
	// len(Data)+2 = 256 exceeds the 1-byte Tlength field. The old code
	// emitted byte(256) = 0x00 silently; the contract panics loudly.
	tr := Triplet{ID: 0x65, Data: make([]byte, 254)}
	assertPanics(t, "Triplet.Encode with len(Data)+2 > 255", func() {
		tr.Encode()
	})
}

func TestTriplet_EncodeAboveArchitecturalMaxPanics(t *testing.T) {
	// len(Data)+2 = 255 fits the 1-byte field but exceeds the architectural
	// maximum (254) that decodeTriplet enforces: Encode must refuse to emit
	// a triplet its own decoder rejects.
	tr := Triplet{ID: 0x65, Data: bytes.Repeat([]byte{0xC8}, 253)}
	assertPanics(t, "Triplet.Encode with len(Data)+2 = 255 > maxTripletLen", func() {
		tr.Encode()
	})
}

func TestTriplet_EncodeBelowArchitecturalMinPanics(t *testing.T) {
	// len(Data)+2 = 2 is below the architectural minimum (3) that
	// decodeTriplet enforces: clearing Data on a decoded triplet must fail
	// loudly rather than emit a Tlength the decoder rejects.
	tr := Triplet{ID: 0x65}
	assertPanics(t, "Triplet.Encode with len(Data)+2 = 2 < minTripletLen", func() {
		tr.Encode()
	})
}

func TestTriplet_EncodeMaxTlength(t *testing.T) {
	// Boundary: len(Data)+2 = 254 is the architectural maximum Tlength.
	tr := Triplet{ID: 0x65, Data: bytes.Repeat([]byte{0xC8}, 252)}
	enc := tr.Encode()
	if len(enc) != 254 {
		t.Fatalf("len(Encode()) = %d, want 254", len(enc))
	}
	if enc[0] != 0xFE {
		t.Errorf("Tlength byte = 0x%02X, want 0xFE", enc[0])
	}
	if enc[1] != 0x65 {
		t.Errorf("Tid byte = 0x%02X, want 0x65", enc[1])
	}
}

func TestBDT_EncodeRecomputesLengthAfterTripletMutation(t *testing.T) {
	// Decode a BDT, then mutate its triplets post-decode. Encode must
	// emit a correct SFLength prefix recomputed from the new body, not
	// the stale stored Length (old code emitted 0x0016 for a 26-byte
	// field), and the re-encoded bytes must round-trip stably.
	f, err := DecodeField(bdtWithComment)
	if err != nil {
		t.Fatalf("DecodeField(BDT+triplet) error: %v", err)
	}
	bdt, ok := f.(*BDT)
	if !ok {
		t.Fatalf("type = %T, want *BDT", f)
	}
	bdt.Triplets = append(bdt.Triplets, Triplet{ID: 0x65, Data: []byte{0xC8, 0xC9}})
	enc := bdt.Encode()
	if prefix := int(binary.BigEndian.Uint16(enc[0:2])); prefix != len(enc) {
		t.Errorf("SFLength prefix = %d, want encoded length %d (stale Length)", prefix, len(enc))
	}
	f2, err := DecodeField(enc)
	if err != nil {
		t.Fatalf("DecodeField(re-encoded BDT) error: %v", err)
	}
	if enc2 := f2.Encode(); !bytes.Equal(enc2, enc) {
		t.Errorf("re-encode mismatch:\nfirst  % X\nsecond % X", enc, enc2)
	}
}
