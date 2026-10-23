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
// BDT; if it leaked, the MCFs would be scope-discarded instead of merged).
// The MCFs sit between the second BDT and any BPG on purpose: no BPG
// intervenes, so the abort can only prove the BDT reset — the BPG reset
// cannot mask a missing BDT reset here.
func TestReduce_BDTClearsOpenOEG(t *testing.T) {
	bdt := rawField(tBDT, append(ebcdicSpaces(8), 0x00, 0x00))
	bog := rawField(tBOG, nil)
	edt := rawField(tEDT, append(ebcdicSpaces(8), 0x00, 0x00))
	mcfData := mcfGroup(tripletX24(0x05, 0x01), tripletX02(0x8E, mcfFixMono10))
	mcf := rawField(tMCF, mcfData)

	var buf bytes.Buffer
	for _, f := range [][]byte{bdt, bog, edt, bdt, mcf, mcf, edt} {
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

// TestReduce_OEGMCFDiscardedNoDuplicateAbort: an MCF inside an OEG is
// parsed (fail-closed) but its mapping is discarded — it must not hit the
// uniform duplicate-LID abort against a page MCF mapping the same LID.
// Without the scope guard the OEG MCF merges into the page map and this
// stream aborts at the OEG MCF (RED).
func TestReduce_OEGMCFDiscardedNoDuplicateAbort(t *testing.T) {
	bdt := rawField(tBDT, append(ebcdicSpaces(8), 0x00, 0x00))
	bpg := rawField(tBPG, ebcdicSpaces(8))
	pageMCF := rawField(tMCF, mcfGroup(tripletX24(0x05, 0x01), tripletX02(0x8E, mcfFixMono10)))
	bog := rawField(tBOG, nil)
	oegMCF := rawField(tMCF, mcfGroup(tripletX24(0x05, 0x01), tripletX02(0x8E, mcfFixProp10)))
	eog := rawField(tEOG, nil)
	epg := rawField(tEPG, ebcdicSpaces(8))
	edt := rawField(tEDT, append(ebcdicSpaces(8), 0x00, 0x00))

	var buf bytes.Buffer
	for _, f := range [][]byte{bdt, bpg, pageMCF, bog, oegMCF, eog, epg, edt} {
		buf.Write(f)
	}
	if _, _, err := Reduce(&buf); err != nil {
		t.Fatalf("Reduce: %v, want no error (OEG MCF mapping discarded)", err)
	}
}

// TestReduce_OEGMCFDiscardedPageMapIntact: after the OEG MCF is discarded,
// the page map must hold exactly the first page MCF's mapping. A second
// page-level MCF mapping the same LID must hit the uniform duplicate-LID
// abort — and the diagnostic must be attributed to that page MCF, not the
// OEG MCF. Without the guard the abort fires one field earlier (RED).
func TestReduce_OEGMCFDiscardedPageMapIntact(t *testing.T) {
	bdt := rawField(tBDT, append(ebcdicSpaces(8), 0x00, 0x00))
	bpg := rawField(tBPG, ebcdicSpaces(8))
	pageMCF := rawField(tMCF, mcfGroup(tripletX24(0x05, 0x01), tripletX02(0x8E, mcfFixMono10)))
	bog := rawField(tBOG, nil)
	oegMCF := rawField(tMCF, mcfGroup(tripletX24(0x05, 0x01), tripletX02(0x8E, mcfFixProp10)))
	eog := rawField(tEOG, nil)
	pageMCF2 := rawField(tMCF, mcfGroup(tripletX24(0x05, 0x01), tripletX02(0x8E, mcfFixProp10)))
	epg := rawField(tEPG, ebcdicSpaces(8))
	edt := rawField(tEDT, append(ebcdicSpaces(8), 0x00, 0x00))

	fields := [][]byte{bdt, bpg, pageMCF, bog, oegMCF, eog, pageMCF2, epg, edt}
	var buf bytes.Buffer
	for _, f := range fields {
		buf.Write(f)
	}
	_, _, err := Reduce(&buf)
	if err == nil {
		t.Fatal("Reduce succeeded, want duplicate-LID abort at the second page MCF")
	}
	var derr afp.DecodeError
	if !errors.As(err, &derr) {
		t.Fatalf("error %T (%v), want afp.DecodeError", err, err)
	}
	if derr.Kind != afp.KindStructure {
		t.Errorf("Kind = %q, want %q", derr.Kind, afp.KindStructure)
	}
	// mergeMCFMappings attributes the duplicate to the MCF under visit:
	// its stream-relative field start plus the 8-byte introducer.
	wantOff := 8
	for _, f := range fields[:6] {
		wantOff += len(f)
	}
	if derr.Offset != wantOff {
		t.Errorf("abort offset = %d, want %d (second page MCF, not the OEG MCF)", derr.Offset, wantOff)
	}
}

// TestReduce_OEGMCFMalformedStaysLoud: the guard parses before it
// discards — a malformed MCF inside an OEG still aborts loudly with
// KindStructure instead of being silently skipped. This test pins the
// parse-before-discard ordering: it passes only because the guard sits
// after the ParseMCFMapping call, never before it.
func TestReduce_OEGMCFMalformedStaysLoud(t *testing.T) {
	bdt := rawField(tBDT, append(ebcdicSpaces(8), 0x00, 0x00))
	bpg := rawField(tBPG, ebcdicSpaces(8))
	bog := rawField(tBOG, nil)
	badMCF := rawField(tMCF, []byte{0x00, 0x06}) // RGLength 6 < 7: malformed
	eog := rawField(tEOG, nil)
	epg := rawField(tEPG, ebcdicSpaces(8))
	edt := rawField(tEDT, append(ebcdicSpaces(8), 0x00, 0x00))

	var buf bytes.Buffer
	for _, f := range [][]byte{bdt, bpg, bog, badMCF, eog, epg, edt} {
		buf.Write(f)
	}
	_, _, err := Reduce(&buf)
	if err == nil {
		t.Fatal("Reduce succeeded, want loud KindStructure for malformed OEG MCF")
	}
	var derr afp.DecodeError
	if !errors.As(err, &derr) {
		t.Fatalf("error %T (%v), want afp.DecodeError", err, err)
	}
	if derr.Kind != afp.KindStructure {
		t.Errorf("Kind = %q, want %q", derr.Kind, afp.KindStructure)
	}
}

// TestReduce_UnclosedBOGMCFDiscarded: an unclosed BOG is conservatively
// fail-closed — subsequent MCFs are parsed but discarded, never merged into
// the page map. Two MCFs mapping the same LID after the unclosed BOG must
// not hit the duplicate-LID abort (without the guard the second MCF aborts;
// RED).
func TestReduce_UnclosedBOGMCFDiscarded(t *testing.T) {
	bdt := rawField(tBDT, append(ebcdicSpaces(8), 0x00, 0x00))
	bpg := rawField(tBPG, ebcdicSpaces(8))
	bog := rawField(tBOG, nil)
	mcf1 := rawField(tMCF, mcfGroup(tripletX24(0x05, 0x01), tripletX02(0x8E, mcfFixMono10)))
	mcf2 := rawField(tMCF, mcfGroup(tripletX24(0x05, 0x01), tripletX02(0x8E, mcfFixProp10)))
	epg := rawField(tEPG, ebcdicSpaces(8))
	edt := rawField(tEDT, append(ebcdicSpaces(8), 0x00, 0x00))

	var buf bytes.Buffer
	for _, f := range [][]byte{bdt, bpg, bog, mcf1, mcf2, epg, edt} {
		buf.Write(f)
	}
	if _, _, err := Reduce(&buf); err != nil {
		t.Fatalf("Reduce: %v, want no error (unclosed-OEG MCFs discarded)", err)
	}
}

// TestReduce_MCFAfterEOGMergesNormally: once the OEG closes the guard is
// off and MCFs merge into the page map normally — two page-level MCFs
// mapping the same LID hit the uniform duplicate-LID abort. This is a
// behavior-preservation check, green with or without the guard.
func TestReduce_MCFAfterEOGMergesNormally(t *testing.T) {
	bdt := rawField(tBDT, append(ebcdicSpaces(8), 0x00, 0x00))
	bpg := rawField(tBPG, ebcdicSpaces(8))
	bog := rawField(tBOG, nil)
	eog := rawField(tEOG, nil)
	mcf1 := rawField(tMCF, mcfGroup(tripletX24(0x05, 0x01), tripletX02(0x8E, mcfFixMono10)))
	mcf2 := rawField(tMCF, mcfGroup(tripletX24(0x05, 0x01), tripletX02(0x8E, mcfFixProp10)))
	epg := rawField(tEPG, ebcdicSpaces(8))
	edt := rawField(tEDT, append(ebcdicSpaces(8), 0x00, 0x00))

	var buf bytes.Buffer
	for _, f := range [][]byte{bdt, bpg, bog, eog, mcf1, mcf2, epg, edt} {
		buf.Write(f)
	}
	_, _, err := Reduce(&buf)
	if err == nil {
		t.Fatal("Reduce succeeded, want duplicate-LID abort for the second post-EOG page MCF")
	}
	var derr afp.DecodeError
	if !errors.As(err, &derr) {
		t.Fatalf("error %T (%v), want afp.DecodeError", err, err)
	}
	if derr.Kind != afp.KindStructure {
		t.Errorf("Kind = %q, want %q", derr.Kind, afp.KindStructure)
	}
}
