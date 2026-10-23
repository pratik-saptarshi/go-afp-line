package afptoline

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/pratik-saptarshi/go-afp-line/afp"
	"github.com/pratik-saptarshi/go-afp-line/line"
)

// Reducer-level tests for MCF-driven SCFL local font ID resolution (Task 2
// of the MCF font-mapping plan). Fixture construction mirrors the helpers
// in afp/mcf_test.go: one repeating group is an RGLength-prefixed run of
// X'24'/X'02' triplets.

// EBCDIC-037 encodings of the coded font names used in these tests.
var (
	mcfFixMono10     = []byte{0xC6, 0xC9, 0xE7, 0x60, 0xD4, 0xD6, 0xD5, 0xD6, 0x60, 0xF1, 0xF0}                         // "FIX-MONO-10"
	mcfFixProp10     = []byte{0xC6, 0xC9, 0xE7, 0x60, 0xD7, 0xD9, 0xD6, 0xD7, 0x60, 0xF1, 0xF0}                         // "FIX-PROP-10"
	mcfNopeNotPinned = []byte{0xD5, 0xD6, 0xD7, 0xC5, 0x60, 0xD5, 0xD6, 0xE3, 0x60, 0xD7, 0xC9, 0xD5, 0xD5, 0xC5, 0xC4} // "NOPE-NOT-PINNED"
)

// mcfGroup builds one MCF repeating group: RGLength (UBIN[2], big-endian)
// followed by the supplied triplet bytes.
func mcfGroup(triplets ...[]byte) []byte {
	var g []byte
	for _, t := range triplets {
		g = append(g, t...)
	}
	out := make([]byte, 2, 2+len(g))
	out[0] = byte((len(g) + 2) >> 8)
	out[1] = byte(len(g) + 2)
	return append(out, g...)
}

// tripletX24 builds an X'24' (Resource Local ID) triplet: ResType byte and
// LID byte.
func tripletX24(resType, lid byte) []byte {
	return []byte{0x04, 0x24, resType, lid}
}

// tripletX02 builds an X'02' (Fully Qualified Name) triplet with the given
// FQNType and raw EBCDIC name bytes.
func tripletX02(fqnType byte, name []byte) []byte {
	out := []byte{byte(len(name) + 4), 0x02, fqnType, 0x00}
	return append(out, name...)
}

// mcfReduceStream builds a legal BDT → [MCF] → BPG → BPT → PTX(SCFL lid,
// then TRN "A") → EPT → EPG → EDT stream. A nil mcfData omits the MCF field.
func mcfReduceStream(mcfData []byte, lid byte) []byte {
	var mcfDatas [][]byte
	if mcfData != nil {
		mcfDatas = [][]byte{mcfData}
	}
	return mcfDocFields(mcfDatas, lid)
}

// mcfDocFields builds one document: BDT → MCF* → BPG → BPT → PTX(SCFL lid,
// then TRN "A") → EPT → EPG → EDT, with one MCF field per mcfDatas entry in
// document order (needed to pin later-MCF-replaces-earlier and the BDT
// reset across documents).
func mcfDocFields(mcfDatas [][]byte, lid byte) []byte {
	tMCF := [3]byte{0xD3, 0xAB, 0x8A}
	fields := [][]byte{
		rawField(tBDT, append(ebcdicSpaces(8), 0x00, 0x00)),
	}
	for _, mcfData := range mcfDatas {
		fields = append(fields, rawField(tMCF, mcfData))
	}
	fields = append(fields,
		rawField(tBPG, ebcdicSpaces(8)),
		rawField(tBPT, nil),
		rawField(tPTX, append(seq(0xF0, lid), trn(0xC1)...)),
		rawField(tEPT, nil),
		rawField(tEPG, ebcdicSpaces(8)),
		rawField(tEDT, append(ebcdicSpaces(8), 0x00, 0x00)),
	)
	var buf bytes.Buffer
	for _, f := range fields {
		buf.Write(f)
	}
	return buf.Bytes()
}

// assertReduceErrContains requires a typed afp.DecodeError whose reason
// contains the expected substring.
func assertReduceErrContains(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("Reduce succeeded, want error containing %q", want)
	}
	var derr afp.DecodeError
	if !errors.As(err, &derr) {
		t.Fatalf("error %T (%v), want afp.DecodeError", err, err)
	}
	if !strings.Contains(derr.Reason, want) {
		t.Errorf("Reason = %q, want it to contain %q", derr.Reason, want)
	}
}

// TestReduce_MCFMappedFont: an MCF mapping LID 1 → pinned "FIX-MONO-10"
// resolves the PTX's SCFL LID 1 to that font and the reduction succeeds.
func TestReduce_MCFMappedFont(t *testing.T) {
	mcf := mcfGroup(tripletX24(0x05, 0x01), tripletX02(0x8E, mcfFixMono10))
	doc, _, err := Reduce(bytes.NewReader(mcfReduceStream(mcf, 0x01)))
	if err != nil {
		t.Fatalf("Reduce: %v", err)
	}
	if len(doc.Pages) != 1 {
		t.Fatalf("got %d pages, want 1", len(doc.Pages))
	}
	runs := doc.Pages[0].Runs
	if len(runs) != 1 {
		t.Fatalf("got %d runs, want 1", len(runs))
	}
	if runs[0].FontID != "FIX-MONO-10" {
		t.Errorf("FontID = %q, want %q", runs[0].FontID, "FIX-MONO-10")
	}
	if runs[0].Text != "A" {
		t.Errorf("Text = %q, want %q", runs[0].Text, "A")
	}
}

// TestReduce_MCFUnmappedLID: SCFL LID 1 with no MCF anywhere in the stream
// fails loudly — the reducer must not guess metrics.
func TestReduce_MCFUnmappedLID(t *testing.T) {
	_, _, err := Reduce(bytes.NewReader(mcfReduceStream(nil, 0x01)))
	assertReduceErrContains(t, err, "SCFL local font ID 1 has no MCF mapping (no MCF in stream)")
}

// TestReduce_MCFUnpinnedFont: the MCF maps LID 1 to a coded font name with
// no pinned metrics — the failure names the coded font, not the LID alone.
func TestReduce_MCFUnpinnedFont(t *testing.T) {
	mcf := mcfGroup(tripletX24(0x05, 0x01), tripletX02(0x8E, mcfNopeNotPinned))
	_, _, err := Reduce(bytes.NewReader(mcfReduceStream(mcf, 0x01)))
	assertReduceErrContains(t, err, `coded font "NOPE-NOT-PINNED" (MCF local font ID 1) has no pinned metrics`)
}

// TestReduce_MCFUnsupportedForm: the MCF group for LID 2 carries an X'24'
// but no X'8E' coded-font-name reference — the LID is unmappable, loudly.
func TestReduce_MCFUnsupportedForm(t *testing.T) {
	mcf := mcfGroup(tripletX24(0x05, 0x02), tripletX02(0x84, mcfFixMono10))
	_, _, err := Reduce(bytes.NewReader(mcfReduceStream(mcf, 0x02)))
	assertReduceErrContains(t, err, "uses unsupported font identification")
}

// TestReduce_MCFSecondMappingWins: two MCF fields in one document carry
// different mappings for the same LID — the later MCF replaces the earlier
// map (reduce.go, D3AB8A case), so the PTX's SCFL LID 1 resolves to the
// second MCF's coded font. A keep-first implementation (`if st.mcf == nil`)
// pins the first mapping and must fail this test.
func TestReduce_MCFSecondMappingWins(t *testing.T) {
	first := mcfGroup(tripletX24(0x05, 0x01), tripletX02(0x8E, mcfFixProp10))
	second := mcfGroup(tripletX24(0x05, 0x01), tripletX02(0x8E, mcfFixMono10))
	doc, _, err := Reduce(bytes.NewReader(mcfDocFields([][]byte{first, second}, 0x01)))
	if err != nil {
		t.Fatalf("Reduce: %v", err)
	}
	runs := doc.Pages[0].Runs
	if len(runs) != 1 {
		t.Fatalf("got %d runs, want 1", len(runs))
	}
	if runs[0].FontID != "FIX-MONO-10" {
		t.Errorf("FontID = %q, want %q (the second MCF wins)", runs[0].FontID, "FIX-MONO-10")
	}
}

// TestReduce_MCFResetAtBDT: the MCF mapping resets at BDT — a second
// document in the same stream without an MCF must not inherit the first
// document's map. SCFL LID≠0 with no MCF in the current document fails with
// the no-MCF diagnostic, proving the map did not leak across documents.
func TestReduce_MCFResetAtBDT(t *testing.T) {
	mapped := mcfGroup(tripletX24(0x05, 0x01), tripletX02(0x8E, mcfFixMono10))
	stream := append(mcfDocFields([][]byte{mapped}, 0x01), mcfDocFields(nil, 0x01)...)
	_, _, err := Reduce(bytes.NewReader(stream))
	assertReduceErrContains(t, err, "SCFL local font ID 1 has no MCF mapping (no MCF in stream)")
}

// TestReduce_MCFParseErrorRebased: a malformed MCF field must report a
// stream-relative offset (like visitPTX does) and attribute the error to the
// MCF field — not the payload-relative offset the parser produced.
func TestReduce_MCFParseErrorRebased(t *testing.T) {
	// Malformed: RGLength 1 (< 2) → parser error at payload offset 0.
	// Stream layout: BDT field (8-byte introducer + 10 payload = 18 bytes),
	// then the MCF field (8-byte introducer). Expected offset: 18 + 8 + 0.
	doc, _, err := Reduce(bytes.NewReader(mcfReduceStream([]byte{0x00, 0x01}, 0x01)))
	_ = doc
	if err == nil {
		t.Fatal("Reduce succeeded, want MCF structure error")
	}
	var derr afp.DecodeError
	if !errors.As(err, &derr) {
		t.Fatalf("error %T (%v), want afp.DecodeError", err, err)
	}
	if derr.FieldID != "D3AB8A" {
		t.Errorf("FieldID = %q, want %q", derr.FieldID, "D3AB8A")
	}
	if derr.Offset != 26 {
		t.Errorf("Offset = %d, want 26 (stream-relative: BDT 18 + MCF introducer 8 + payload 0)", derr.Offset)
	}
}

// TestFontIDForLID_DefaultVerified: LID 0 bypasses the MCF map, but the
// returned default font ID must still resolve in the pinned registry —
// advanceFunc's LookupFont call relies on fontIDForLID's verified-ID
// invariant, and a future registry edit must not silently produce zero
// widths. (The registry-miss branch itself is untestable without test
// hooks on the package-level table; the mapped-LID miss path exercises
// the same loud-error shape via TestReduce_MCFUnpinnedFont.)
func TestFontIDForLID_DefaultVerified(t *testing.T) {
	id, err := fontIDForLID(0, nil, 0)
	if err != nil {
		t.Fatalf("fontIDForLID(0): unexpected error: %v", err)
	}
	if id != "FIX-MONO-10" {
		t.Errorf("id = %q, want %q", id, "FIX-MONO-10")
	}
	if _, ok := line.LookupFont(id); !ok {
		t.Errorf("LookupFont(%q) missed: LID 0 must return a pinned font", id)
	}
}
