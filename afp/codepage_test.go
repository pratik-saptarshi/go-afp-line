package afp

import (
	"errors"
	"testing"
)

// cprun is one text run tagged with its code page, the way MCF/GCSGID
// tagging associates runs with a CPGID during reduction.
type cprun struct {
	cpgid uint16
	data  []byte
}

func decodeRuns(t *testing.T, runs []cprun) []string {
	t.Helper()
	out := make([]string, 0, len(runs))
	for _, r := range runs {
		cp, err := LookupCodePage(r.cpgid)
		if err != nil {
			t.Fatalf("LookupCodePage(%d): unexpected error: %v", r.cpgid, err)
		}
		out = append(out, cp.Decode(r.data))
	}
	return out
}

// TestCodePage_DualFixture decodes one byte stream whose runs are tagged with
// different code pages. Bytes 0x4A and 0x4F decode differently in cp037 vs
// cp500, so decoding a run with the wrong page visibly fails:
//
//	0x4A: cp037 -> U+00A2 '¢'    cp500 -> U+005B '['
//	0x4F: cp037 -> U+007C '|'    cp500 -> U+0021 '!'
func TestCodePage_DualFixture(t *testing.T) {
	hello := []byte{0xC8, 0x85, 0x93, 0x93, 0x96} // "Hello" — identical in both pages
	runs := []cprun{
		{cpgid: 37, data: append(append([]byte{}, hello...), 0x4A)},
		{cpgid: 500, data: append(append([]byte{}, hello...), 0x4A)},
		{cpgid: 37, data: []byte{0x4F}},
		{cpgid: 500, data: []byte{0x4F}},
	}
	want := []string{"Hello¢", "Hello[", "|", "!"}

	got := decodeRuns(t, runs)
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("run %d (cp %d): got %q, want %q", i, runs[i].cpgid, got[i], want[i])
		}
	}
}

// TestCodePage_RegistryCoversBothPages asserts both v1 pages are registered.
func TestCodePage_RegistryCoversBothPages(t *testing.T) {
	for _, id := range []uint16{37, 500} {
		cp, err := LookupCodePage(id)
		if err != nil {
			t.Fatalf("LookupCodePage(%d): unexpected error: %v", id, err)
		}
		if cp.CPGID != id {
			t.Errorf("LookupCodePage(%d): CPGID = %d", id, cp.CPGID)
		}
		// EBCDIC space 0x40 must decode to ASCII space in both pages.
		if got := cp.Decode([]byte{0x40}); got != " " {
			t.Errorf("LookupCodePage(%d): 0x40 decoded as %q, want %q", id, got, " ")
		}
	}
}

// TestLookupCodePage_Unknown asserts an unregistered code page fails loudly
// with a typed DecodeError — the exclusion path, never a silent fallback.
func TestLookupCodePage_Unknown(t *testing.T) {
	_, err := LookupCodePage(1141) // a real CPGID, but outside the v1 subset
	if err == nil {
		t.Fatal("LookupCodePage(1141): expected error, got nil")
	}
	var de DecodeError
	if !errors.As(err, &de) {
		t.Fatalf("LookupCodePage(1141): error is %T, want DecodeError", err)
	}
}
