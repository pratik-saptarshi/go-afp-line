package afp

import (
	"bytes"
	"testing"
)

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
	tr, n, err := DecodeTriplet(commentHello)
	if err != nil {
		t.Fatalf("DecodeTriplet(Comment) error: %v", err)
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
	_, _, err := DecodeTriplet([]byte{0x03, 0xFF, 0x00})
	derr, ok := err.(DecodeError)
	if !ok {
		t.Fatalf("DecodeTriplet(unknown ID) error = %T (%v), want DecodeError", err, err)
	}
	if derr.Offset != 0 {
		t.Errorf("Offset = %d, want 0", derr.Offset)
	}
}

func TestDecodeTriplet_Truncated(t *testing.T) {
	// Tlength claims 5, only 3 bytes present.
	if _, _, err := DecodeTriplet([]byte{0x05, 0x65, 0xC8}); err == nil {
		t.Errorf("DecodeTriplet(truncated) = nil error, want DecodeError")
	}
}

func TestDecodeTriplet_BadLength(t *testing.T) {
	// Tlength=2 is below the architectural minimum of 3.
	if _, _, err := DecodeTriplet([]byte{0x02, 0x65}); err == nil {
		t.Errorf("DecodeTriplet(Tlength=2) = nil error, want DecodeError")
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
