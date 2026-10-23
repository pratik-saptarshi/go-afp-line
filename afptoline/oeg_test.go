package afptoline

import (
	"bytes"
	"errors"
	"testing"

	"github.com/pratik-saptarshi/go-afp-line/afp"
)

// tBOG/tEOG are the Begin/End Object Environment Group structured-field
// type IDs. OEGs wrap objects (out-of-subset content in v1); the reducer
// tracks their boundaries only as a scope flag for MCF merging.
var (
	tBOG = [3]byte{0xD3, 0xA8, 0xC7}
	tEOG = [3]byte{0xD3, 0xA9, 0xC7}
)

// TestReduce_OEGBoundariesRecordedInCensus: BOG/EOG stay excluded fields —
// the interception records the scope flag and then falls through to the
// existing exclusion path, so each boundary appears exactly once in the
// returned exclusions census (neither dropped nor double-counted).
func TestReduce_OEGBoundariesRecordedInCensus(t *testing.T) {
	bdt := rawField(tBDT, append(ebcdicSpaces(8), 0x00, 0x00))
	bog := rawField(tBOG, nil)
	eog := rawField(tEOG, nil)
	edt := rawField(tEDT, append(ebcdicSpaces(8), 0x00, 0x00))

	var buf bytes.Buffer
	for _, f := range [][]byte{bdt, bog, eog, edt} {
		buf.Write(f)
	}
	_, excls, err := Reduce(&buf)
	if err != nil {
		t.Fatalf("Reduce: %v", err)
	}
	counts := map[string]int{}
	for _, e := range excls {
		counts[e.FieldType]++
	}
	if counts["BOG"] != 1 {
		t.Errorf("BOG census entries = %d, want 1", counts["BOG"])
	}
	if counts["EOG"] != 1 {
		t.Errorf("EOG census entries = %d, want 1", counts["EOG"])
	}
}

// TestReduce_BOGInsideOpenPage: real-world files wrap page-resident objects
// in OEGs, so a BOG/EOG pair inside an open page must not error and must
// not disturb the page. Regression guard for the interception.
func TestReduce_BOGInsideOpenPage(t *testing.T) {
	bdt := rawField(tBDT, append(ebcdicSpaces(8), 0x00, 0x00))
	bpg := rawField(tBPG, ebcdicSpaces(8))
	bog := rawField(tBOG, nil)
	eog := rawField(tEOG, nil)
	epg := rawField(tEPG, ebcdicSpaces(8))
	edt := rawField(tEDT, append(ebcdicSpaces(8), 0x00, 0x00))

	var buf bytes.Buffer
	for _, f := range [][]byte{bdt, bpg, bog, eog, epg, edt} {
		buf.Write(f)
	}
	doc, excls, err := Reduce(&buf)
	if err != nil {
		t.Fatalf("Reduce: %v", err)
	}
	if len(doc.Pages) != 1 {
		t.Errorf("got %d pages, want 1", len(doc.Pages))
	}
	counts := map[string]int{}
	for _, e := range excls {
		counts[e.FieldType]++
	}
	if counts["BOG"] != 1 || counts["EOG"] != 1 {
		t.Errorf("census: BOG=%d EOG=%d, want 1 each", counts["BOG"], counts["EOG"])
	}
}

// TestReduce_BDTClearsOpenOEG: an unclosed BOG must not leak its scope past
// a document boundary — BDT clears the guard. Verified through MCF merge
// behavior in the new document: two MCFs mapping the same LID must hit the
// uniform duplicate-LID abort (proving inOEG was cleared at the second
// BDT; if it leaked, the MCFs would be scope-discarded instead of merged
// once the Task 2 guard consults the flag).
func TestReduce_BDTClearsOpenOEG(t *testing.T) {
	bdt := rawField(tBDT, append(ebcdicSpaces(8), 0x00, 0x00))
	bog := rawField(tBOG, nil)
	edt := rawField(tEDT, append(ebcdicSpaces(8), 0x00, 0x00))
	bpg := rawField(tBPG, ebcdicSpaces(8))
	mcfData := mcfGroup(tripletX24(0x05, 0x01), tripletX02(0x8E, mcfFixMono10))
	mcf := rawField(tMCF, mcfData)
	epg := rawField(tEPG, ebcdicSpaces(8))

	var buf bytes.Buffer
	for _, f := range [][]byte{bdt, bog, edt, bdt, bpg, mcf, mcf, epg, edt} {
		buf.Write(f)
	}
	_, _, err := Reduce(&buf)
	if err == nil {
		t.Fatal("Reduce succeeded, want duplicate-LID abort (guard cleared at BDT)")
	}
	var derr afp.DecodeError
	if !errors.As(err, &derr) {
		t.Fatalf("error %T (%v), want afp.DecodeError", err, err)
	}
	if derr.Kind != afp.KindStructure {
		t.Errorf("Kind = %q, want %q", derr.Kind, afp.KindStructure)
	}
}
