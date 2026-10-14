package afp

import (
	"bytes"
	"testing"
)

// Test vectors use real EBCDIC-037 bytes. "HELLOWLD" =
// C8 C5 D3 D3 D6 E6 D3 C4, "PAGE0001" = D7 C1 C7 C5 F0 F0 F0 F1.
// The structured field introducer is 8 bytes: 2B length (big-endian,
// including the introducer) + 3B type ID + 1B flags + 2B reserved.

var (
	// BDT, SFLength=18: introducer + DocName(8) + reserved(2).
	bdtHello = []byte{
		0x00, 0x12, 0xD3, 0xA8, 0xA8, 0x00, 0x00, 0x00,
		0xC8, 0xC5, 0xD3, 0xD3, 0xD6, 0xE6, 0xD3, 0xC4, // "HELLOWLD"
		0x00, 0x00,
	}
	// EDT, SFLength=18.
	edtHello = []byte{
		0x00, 0x12, 0xD3, 0xA9, 0xA8, 0x00, 0x00, 0x00,
		0xC8, 0xC5, 0xD3, 0xD3, 0xD6, 0xE6, 0xD3, 0xC4, // "HELLOWLD"
		0x00, 0x00,
	}
	// BPG, SFLength=16: introducer + PageName(8).
	bpgPage = []byte{
		0x00, 0x10, 0xD3, 0xA8, 0xAF, 0x00, 0x00, 0x00,
		0xD7, 0xC1, 0xC7, 0xC5, 0xF0, 0xF0, 0xF0, 0xF1, // "PAGE0001"
	}
	// EPG, SFLength=16.
	epgPage = []byte{
		0x00, 0x10, 0xD3, 0xA9, 0xAF, 0x00, 0x00, 0x00,
		0xD7, 0xC1, 0xC7, 0xC5, 0xF0, 0xF0, 0xF0, 0xF1, // "PAGE0001"
	}
	// BPG with no page name: introducer only, SFLength=8.
	bpgNoName = []byte{0x00, 0x08, 0xD3, 0xA8, 0xAF, 0x00, 0x00, 0x00}
	// Unknown type D3FFFF.
	unknownField = []byte{0x00, 0x08, 0xD3, 0xFF, 0xFF, 0x00, 0x00, 0x00}
	// BGR (Begin Graphics, D3A8BB): known-excluded per the subset contract.
	bgrExcluded = []byte{0x00, 0x08, 0xD3, 0xA8, 0xBB, 0x00, 0x00, 0x00}
	// PTX (D3EE9B): contracted in-subset, typed decoder arrives in Task 6.
	ptxContracted = []byte{0x00, 0x08, 0xD3, 0xEE, 0x9B, 0x00, 0x00, 0x00}
	// BDT claiming 12 bytes but carrying only 4 data bytes: truncated data.
	bdtTruncatedData = []byte{
		0x00, 0x0C, 0xD3, 0xA8, 0xA8, 0x00, 0x00, 0x00,
		0xC8, 0xC5, 0xD3, 0xD3,
	}
)

func TestDecodeField_BDT(t *testing.T) {
	f, err := DecodeField(bdtHello)
	if err != nil {
		t.Fatalf("DecodeField(BDT) error: %v", err)
	}
	bdt, ok := f.(*BDT)
	if !ok {
		t.Fatalf("DecodeField(BDT) type = %T, want *BDT", f)
	}
	if bdt.DocName != "HELLOWLD" {
		t.Errorf("DocName = %q, want %q", bdt.DocName, "HELLOWLD")
	}
	if f.TypeID() != "D3A8A8" {
		t.Errorf("TypeID() = %q, want %q", f.TypeID(), "D3A8A8")
	}
}

func TestDecodeField_EDT(t *testing.T) {
	f, err := DecodeField(edtHello)
	if err != nil {
		t.Fatalf("DecodeField(EDT) error: %v", err)
	}
	edt, ok := f.(*EDT)
	if !ok {
		t.Fatalf("DecodeField(EDT) type = %T, want *EDT", f)
	}
	if edt.DocName != "HELLOWLD" {
		t.Errorf("DocName = %q, want %q", edt.DocName, "HELLOWLD")
	}
}

func TestDecodeField_BPG(t *testing.T) {
	f, err := DecodeField(bpgPage)
	if err != nil {
		t.Fatalf("DecodeField(BPG) error: %v", err)
	}
	bpg, ok := f.(*BPG)
	if !ok {
		t.Fatalf("DecodeField(BPG) type = %T, want *BPG", f)
	}
	if bpg.PageName != "PAGE0001" {
		t.Errorf("PageName = %q, want %q", bpg.PageName, "PAGE0001")
	}
}

func TestDecodeField_EPG(t *testing.T) {
	f, err := DecodeField(epgPage)
	if err != nil {
		t.Fatalf("DecodeField(EPG) error: %v", err)
	}
	if _, ok := f.(*EPG); !ok {
		t.Fatalf("DecodeField(EPG) type = %T, want *EPG", f)
	}
}

func TestDecodeField_BPG_NoName(t *testing.T) {
	f, err := DecodeField(bpgNoName)
	if err != nil {
		t.Fatalf("DecodeField(BPG, no name) error: %v", err)
	}
	bpg, ok := f.(*BPG)
	if !ok {
		t.Fatalf("type = %T, want *BPG", f)
	}
	if bpg.PageName != "" {
		t.Errorf("PageName = %q, want empty", bpg.PageName)
	}
}

func TestDecodeField_BPG_BlankName(t *testing.T) {
	// All-spaces PageName: decodes to "" but must re-encode byte-identically.
	blank := []byte{
		0x00, 0x10, 0xD3, 0xA8, 0xAF, 0x00, 0x00, 0x00,
		0x40, 0x40, 0x40, 0x40, 0x40, 0x40, 0x40, 0x40,
	}
	f, err := DecodeField(blank)
	if err != nil {
		t.Fatalf("DecodeField(BPG, blank name) error: %v", err)
	}
	bpg, ok := f.(*BPG)
	if !ok {
		t.Fatalf("type = %T, want *BPG", f)
	}
	if bpg.PageName != "" {
		t.Errorf("PageName = %q, want empty", bpg.PageName)
	}
	if !bpg.HasPageName {
		t.Errorf("HasPageName = false, want true: the 8-byte name was on the wire")
	}
	if got := f.Encode(); !bytes.Equal(got, blank) {
		t.Errorf("Encode() = %X, want %X", got, blank)
	}
}

func TestDecodeField_UnknownType(t *testing.T) {
	_, err := DecodeField(unknownField)
	derr, ok := err.(DecodeError)
	if !ok {
		t.Fatalf("DecodeField(unknown) error = %T (%v), want DecodeError", err, err)
	}
	if derr.FieldID != "D3FFFF" {
		t.Errorf("FieldID = %q, want %q", derr.FieldID, "D3FFFF")
	}
}

func TestDecodeField_ExcludedGOCA(t *testing.T) {
	f, err := DecodeField(bgrExcluded)
	if err != nil {
		t.Fatalf("DecodeField(BGR) error: %v (excluded fields must not error)", err)
	}
	raw, ok := f.(*RawField)
	if !ok {
		t.Fatalf("DecodeField(BGR) type = %T, want *RawField", f)
	}
	if !raw.Excluded {
		t.Errorf("RawField.Excluded = false, want true for a known-excluded field")
	}
	if !bytes.Equal(raw.Encode(), bgrExcluded) {
		t.Errorf("RawField.Encode() = %X, want input bytes %X", raw.Encode(), bgrExcluded)
	}
}

func TestDecodeField_ContractedUntyped(t *testing.T) {
	f, err := DecodeField(ptxContracted)
	if err != nil {
		t.Fatalf("DecodeField(PTX) error: %v", err)
	}
	raw, ok := f.(*RawField)
	if !ok {
		t.Fatalf("DecodeField(PTX) type = %T, want *RawField", f)
	}
	if raw.Excluded {
		t.Errorf("RawField.Excluded = true, want false: PTX is contracted, not excluded")
	}
}

func TestDecodeField_TruncatedData(t *testing.T) {
	if _, err := DecodeField(bdtTruncatedData); err == nil {
		t.Errorf("DecodeField(truncated BDT data) = nil error, want DecodeError")
	}
}

func TestField_RoundTrip(t *testing.T) {
	vectors := [][]byte{
		bdtHello, edtHello, bpgPage, epgPage, bpgNoName,
		bgrExcluded, ptxContracted,
	}
	for i, v := range vectors {
		f, err := DecodeField(v)
		if err != nil {
			t.Fatalf("vector %d: DecodeField error: %v", i, err)
		}
		if got := f.Encode(); !bytes.Equal(got, v) {
			t.Errorf("vector %d (%s): Encode() = %X, want %X", i, f.TypeID(), got, v)
		}
	}
}

func FuzzDecodeField(f *testing.F) {
	f.Add(bdtHello)
	f.Add(bpgPage)
	f.Add(bgrExcluded)
	f.Add(unknownField)
	f.Add([]byte{})
	f.Add([]byte{0x00, 0x08, 0xD3})
	f.Fuzz(func(t *testing.T, b []byte) {
		fld, err := DecodeField(b)
		if err != nil {
			return // loud, typed failure: the contract for malformed input
		}
		enc := fld.Encode()
		fld2, err := DecodeField(enc)
		if err != nil {
			t.Fatalf("re-decode of Encode() output failed: %v", err)
		}
		if fld2.TypeID() != fld.TypeID() {
			t.Fatalf("round-trip TypeID changed: %q -> %q", fld.TypeID(), fld2.TypeID())
		}
	})
}
