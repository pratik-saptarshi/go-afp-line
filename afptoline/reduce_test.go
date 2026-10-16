package afptoline

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/pratik-saptarshi/go-afp-line/afp"
	"github.com/pratik-saptarshi/go-afp-line/line"
)

// fixture opens a checked-in AFP fixture by name.
func fixture(t *testing.T, name string) *os.File {
	t.Helper()
	f, err := os.Open("fixtures/" + name)
	if err != nil {
		t.Fatalf("open fixture %s: %v", name, err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

// wantRun is one expected text run: text plus baseline origin in PDF user space.
type wantRun struct {
	text   string
	x, y   float64
	fontID string
	size   float64
}

func checkRuns(t *testing.T, pageNo int, got []line.TextRun, want []wantRun) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("page %d: got %d runs, want %d", pageNo, len(got), len(want))
	}
	for i, w := range want {
		r := got[i]
		if r.Text != w.text {
			t.Errorf("page %d run %d: text %q, want %q (byte-exact)", pageNo, i, r.Text, w.text)
		}
		if d := r.X - w.x; d < -1 || d > 1 {
			t.Errorf("page %d run %d: X=%v, want %v (±1pt)", pageNo, i, r.X, w.x)
		}
		if d := r.Y - w.y; d < -1 || d > 1 {
			t.Errorf("page %d run %d: Y=%v, want %v (±1pt)", pageNo, i, r.Y, w.y)
		}
		if r.FontID != w.fontID {
			t.Errorf("page %d run %d: FontID %q, want %q", pageNo, i, r.FontID, w.fontID)
		}
		if r.FontSize != w.size {
			t.Errorf("page %d run %d: FontSize %v, want %v", pageNo, i, r.FontSize, w.size)
		}
	}
}

// TestReduce_Statement: the 2-page synthetic statement reduces to a 2-page
// document with the expected runs at expected positions (Task-0 tolerance ±1pt,
// text byte-exact). PTOCA Y-down (36pt, 72pt) flips to PDF Y-up on Letter
// (792-36=756, 792-72=720).
func TestReduce_Statement(t *testing.T) {
	doc, excls, err := Reduce(fixture(t, "statement-001.afp"))
	if err != nil {
		t.Fatalf("Reduce: %v", err)
	}
	if len(excls) != 0 {
		t.Fatalf("got %d exclusions, want 0: %v", len(excls), excls)
	}
	if len(doc.Pages) != 2 {
		t.Fatalf("got %d pages, want 2", len(doc.Pages))
	}
	for i, p := range doc.Pages {
		if p.Number != i+1 {
			t.Errorf("page index %d has Number %d", i, p.Number)
		}
		if p.Width != 612 || p.Height != 792 {
			t.Errorf("page %d: size (%v,%v), want (612,792)", p.Number, p.Width, p.Height)
		}
	}
	checkRuns(t, 1, doc.Pages[0].Runs, []wantRun{
		{"STATEMENT LINE ONE", 0, 756, "FIX-MONO-10", 10},
		// AMB moves the baseline only; the second line starts at the
		// advanced inline position: 18 chars * 6pt = 108pt.
		{"STATEMENT LINE TWO", 108, 720, "FIX-MONO-10", 10},
	})
	checkRuns(t, 2, doc.Pages[1].Runs, []wantRun{
		{"STATEMENT LINE THREE", 0, 756, "FIX-MONO-10", 10},
		// 20 chars * 6pt = 120pt advanced past the first line.
		{"STATEMENT LINE FOUR", 120, 720, "FIX-MONO-10", 10},
	})
	if err := doc.Validate(); err != nil {
		t.Errorf("doc.Validate: %v", err)
	}
}

// TestReduce_Exclusions: the GOCA-bearing stream reduces with exactly one
// exclusion whose diagnostic names the field (BGR/GOCA) — loud, not silent,
// and not an abort.
func TestReduce_Exclusions(t *testing.T) {
	doc, excls, err := Reduce(fixture(t, "exclusion-goca.afp"))
	if err != nil {
		t.Fatalf("Reduce: %v", err)
	}
	if len(doc.Pages) != 1 {
		t.Fatalf("got %d pages, want 1", len(doc.Pages))
	}
	checkRuns(t, 1, doc.Pages[0].Runs, []wantRun{
		{"PAGE TEXT", 0, 756, "FIX-MONO-10", 10},
	})
	if len(excls) != 1 {
		t.Fatalf("got %d exclusions, want exactly 1: %v", len(excls), excls)
	}
	d := excls[0].Diagnostic()
	if !strings.Contains(d, "BGR") || !strings.Contains(d, "GOCA") {
		t.Errorf("diagnostic %q does not name BGR/GOCA", d)
	}
}

// TestReduce_Conservation: every PTOCA text byte is accounted for — the run
// sequence across the document matches the stream order exactly: nothing
// dropped, duplicated, or reordered (ordering asserted, not just geometry).
func TestReduce_Conservation(t *testing.T) {
	doc, _, err := Reduce(fixture(t, "statement-001.afp"))
	if err != nil {
		t.Fatalf("Reduce: %v", err)
	}
	var got []string
	total := 0
	for _, p := range doc.Pages {
		for _, r := range p.Runs {
			got = append(got, r.Text)
			total += len(r.Text)
		}
	}
	want := []string{
		"STATEMENT LINE ONE",
		"STATEMENT LINE TWO",
		"STATEMENT LINE THREE",
		"STATEMENT LINE FOUR",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d runs in document order, want %d: %q", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("run %d: got %q, want %q (order/content mismatch)", i, got[i], want[i])
		}
	}
	// 18 + 18 + 20 + 19 text bytes across the four runs.
	if total != 75 {
		t.Errorf("placed text bytes = %d, want 75 (every PTOCA text byte accounted for)", total)
	}
}

// TestReduce_MediumRef: the IMM medium-map reference yields a loud diagnostic
// naming the referenced map ("FORMDEF1") — an exclusion, not an abort — and
// the rest of the document still reduces.
func TestReduce_MediumRef(t *testing.T) {
	doc, excls, err := Reduce(fixture(t, "medium-ref.afp"))
	if err != nil {
		t.Fatalf("Reduce: %v (IMM must be a loud exclusion, not an abort)", err)
	}
	if len(doc.Pages) != 1 {
		t.Fatalf("got %d pages, want 1", len(doc.Pages))
	}
	checkRuns(t, 1, doc.Pages[0].Runs, []wantRun{
		{"PAGE TEXT", 0, 756, "FIX-MONO-10", 10},
	})
	if len(excls) != 1 {
		t.Fatalf("got %d exclusions, want exactly 1: %v", len(excls), excls)
	}
	d := excls[0].Diagnostic()
	if !strings.Contains(d, "IMM") || !strings.Contains(d, "FORMDEF1") {
		t.Errorf("diagnostic %q does not name the IMM FORMDEF1 reference", d)
	}
}

// rawField assembles one naked-framed structured field for pairing tests:
// 8-byte SFI (SFLength BE incl. introducer | D3 TT CC | flags | X'0000') + data.
func rawField(typ [3]byte, data []byte) []byte {
	n := 8 + len(data)
	out := make([]byte, 0, n)
	out = append(out, byte(n>>8), byte(n), typ[0], typ[1], typ[2], 0x00, 0x00, 0x00)
	return append(out, data...)
}

var (
	tBDT = [3]byte{0xD3, 0xA8, 0xA8}
	tEDT = [3]byte{0xD3, 0xA9, 0xA8}
	tBPG = [3]byte{0xD3, 0xA8, 0xAF}
	tEPG = [3]byte{0xD3, 0xA9, 0xAF}
	tPTX = [3]byte{0xD3, 0xEE, 0x9B}
	tBAG = [3]byte{0xD3, 0xA8, 0xC9}
	tEAG = [3]byte{0xD3, 0xA9, 0xC9}
	tBPT = [3]byte{0xD3, 0xA8, 0x9B}
	tEPT = [3]byte{0xD3, 0xA9, 0x9B}
)

// ebcdicSpaces returns n EBCDIC-037 space (0x40) bytes for CHAR parameters.
func ebcdicSpaces(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = 0x40
	}
	return b
}

// TestReduce_PairingErrors: structural mismatches fail loudly with a typed
// afp.DecodeError — never a partial document, never a panic.
func TestReduce_PairingErrors(t *testing.T) {
	bdt := rawField(tBDT, append(ebcdicSpaces(8), 0x00, 0x00))
	bpg := rawField(tBPG, ebcdicSpaces(8))
	epg := rawField(tEPG, ebcdicSpaces(8))
	edt := rawField(tEDT, append(ebcdicSpaces(8), 0x00, 0x00))
	ptx := rawField(tPTX, nil)
	tBNG := [3]byte{0xD3, 0xA8, 0xAD}
	bng := rawField(tBNG, ebcdicSpaces(8))

	cases := []struct {
		name        string
		stream      [][]byte
		wantKind    afp.ErrorKind
		wantReason  string
		wantFieldID string
	}{
		{"nested BDT", [][]byte{bdt, bdt}, afp.KindStructure, "nested BDT: document already open", "D3A8A8"},
		{"EDT without BDT", [][]byte{edt}, afp.KindStructure, "EDT without BDT", "D3A9A8"},
		{"BPG before BDT", [][]byte{bpg}, afp.KindStructure, "BPG outside document", "D3A8AF"},
		{"EPG without BPG", [][]byte{bdt, epg}, afp.KindStructure, "EPG without BPG", "D3A9AF"},
		{"nested BPG", [][]byte{bdt, bpg, bpg}, afp.KindStructure, "nested BPG: page already open", "D3A8AF"},
		{"PTX outside page", [][]byte{bdt, ptx}, afp.KindStructure, "PTX outside page", "D3EE9B"},
		{"EDT with open page", [][]byte{bdt, bpg, edt}, afp.KindStructure, "EDT with open page: missing EPG", "D3A9A8"},
		{"unclosed document", [][]byte{bdt, bpg, epg}, afp.KindStructure, "end of stream with open document: missing EDT", ""},
		{"unclosed BNG", [][]byte{bdt, bng, edt}, afp.KindStructure, "EDT with unclosed group", "D3A9A8"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var buf bytes.Buffer
			for _, f := range c.stream {
				buf.Write(f)
			}
			_, _, err := Reduce(&buf)
			if err == nil {
				t.Fatalf("Reduce succeeded, want afp.DecodeError")
			}
			var derr afp.DecodeError
			if !errors.As(err, &derr) {
				t.Fatalf("error %T (%v), want afp.DecodeError", err, err)
			}
			if derr.Kind != c.wantKind {
				t.Errorf("Kind = %q, want %q", derr.Kind, c.wantKind)
			}
			if !strings.Contains(derr.Reason, c.wantReason) {
				t.Errorf("Reason %q does not contain %q", derr.Reason, c.wantReason)
			}
			if derr.FieldID != c.wantFieldID {
				t.Errorf("FieldID = %q, want %q", derr.FieldID, c.wantFieldID)
			}
		})
	}
}

// TestReduce_BAGInsidePage: BAG/EAG inside an open page is the contract walk
// (docs/subset-contract.md: BPG → BAG … EAG → … EPG) and must reduce
// successfully — CQ-1 regression test.
func TestReduce_BAGInsidePage(t *testing.T) {
	bdt := rawField(tBDT, append(ebcdicSpaces(8), 0x00, 0x00))
	bpg := rawField(tBPG, ebcdicSpaces(8))
	bag := rawField(tBAG, nil)
	eag := rawField(tEAG, nil)
	epg := rawField(tEPG, ebcdicSpaces(8))
	edt := rawField(tEDT, append(ebcdicSpaces(8), 0x00, 0x00))

	var buf bytes.Buffer
	for _, f := range [][]byte{bdt, bpg, bag, eag, epg, edt} {
		buf.Write(f)
	}
	doc, _, err := Reduce(&buf)
	if err != nil {
		t.Fatalf("Reduce: %v", err)
	}
	if len(doc.Pages) != 1 {
		t.Fatalf("got %d pages, want 1", len(doc.Pages))
	}
}

// TestReduce_BAGMustNotCrossEPG: a BAG group open when EPG arrives must
// fail at EPG — Codex P2 on PR #1 (BDT, BPG, BAG, EPG, EAG, EDT was
// accepted, breaking the begin/end pairing guarantee).
func TestReduce_BAGMustNotCrossEPG(t *testing.T) {
	bdt := rawField(tBDT, append(ebcdicSpaces(8), 0x00, 0x00))
	bpg := rawField(tBPG, ebcdicSpaces(8))
	bag := rawField(tBAG, nil)
	eag := rawField(tEAG, nil)
	epg := rawField(tEPG, ebcdicSpaces(8))
	edt := rawField(tEDT, append(ebcdicSpaces(8), 0x00, 0x00))

	// Case 1: BAG group crossing EPG — must fail at EPG.
	var buf bytes.Buffer
	for _, f := range [][]byte{bdt, bpg, bag, epg, eag, edt} {
		buf.Write(f)
	}
	if _, _, err := Reduce(&buf); err == nil {
		t.Fatalf("Reduce accepted BAG group crossing EPG")
	} else if !strings.Contains(err.Error(), "EPG") {
		t.Fatalf("error %q does not name EPG", err)
	}

	// Case 2: BAG never closed — must fail at EPG.
	buf.Reset()
	for _, f := range [][]byte{bdt, bpg, bag, epg, edt} {
		buf.Write(f)
	}
	if _, _, err := Reduce(&buf); err == nil {
		t.Fatalf("Reduce accepted stream with unclosed BAG")
	}
}

// TestReduce_BPTMustNotCrossEPG: a BPT object open when EPG arrives must
// fail at EPG — the BPT/EPG pairing hole mirrors the BAG/EPG hole fixed by
// TestReduce_BAGMustNotCrossEPG (BDT, BPG, BPT, EPG, EPT, EDT was accepted,
// breaking the begin/end pairing guarantee).
func TestReduce_BPTMustNotCrossEPG(t *testing.T) {
	bdt := rawField(tBDT, append(ebcdicSpaces(8), 0x00, 0x00))
	bpg := rawField(tBPG, ebcdicSpaces(8))
	bpt := rawField(tBPT, nil)
	ept := rawField(tEPT, nil)
	epg := rawField(tEPG, ebcdicSpaces(8))
	edt := rawField(tEDT, append(ebcdicSpaces(8), 0x00, 0x00))

	// Case 1: BPT object crossing EPG — must fail at EPG.
	var buf bytes.Buffer
	for _, f := range [][]byte{bdt, bpg, bpt, epg, ept, edt} {
		buf.Write(f)
	}
	if _, _, err := Reduce(&buf); err == nil {
		t.Fatalf("Reduce accepted BPT object crossing EPG")
	} else if !strings.Contains(err.Error(), "EPG") {
		t.Fatalf("error %q does not name EPG", err)
	}

	// Case 2: BPT never closed — must fail at EPG.
	buf.Reset()
	for _, f := range [][]byte{bdt, bpg, bpt, epg, edt} {
		buf.Write(f)
	}
	if _, _, err := Reduce(&buf); err == nil {
		t.Fatalf("Reduce accepted stream with unclosed BPT")
	} else if !strings.Contains(err.Error(), "EPG") {
		t.Fatalf("error %q does not name EPG", err)
	}
}

// TestReduce_RejectsMisnestedGroups: a group end must close the innermost
// open group (ARCH3-1). Flat counters accepted these misnested streams;
// the pairing stack rejects each at the offending end field with a typed
// afp.DecodeError.
func TestReduce_RejectsMisnestedGroups(t *testing.T) {
	bdt := rawField(tBDT, append(ebcdicSpaces(8), 0x00, 0x00))
	bpg := rawField(tBPG, ebcdicSpaces(8))
	epg := rawField(tEPG, ebcdicSpaces(8))
	edt := rawField(tEDT, append(ebcdicSpaces(8), 0x00, 0x00))
	tBNG := [3]byte{0xD3, 0xA8, 0xAD}
	tENG := [3]byte{0xD3, 0xA9, 0xAD}
	bng := rawField(tBNG, ebcdicSpaces(8))
	eng := rawField(tENG, ebcdicSpaces(8))
	bag := rawField(tBAG, nil)
	eag := rawField(tEAG, nil)
	bpt := rawField(tBPT, nil)
	ept := rawField(tEPT, nil)
	ptx := rawField(tPTX, trn(0xC1))

	cases := []struct {
		name       string
		stream     [][]byte
		wantReason string // substring; must name the offending end field
		wantField  string
	}{
		// ENG closes the named group while a page opened inside it is
		// still open: the innermost open group is the page, not the BNG.
		{"ENG across open page", [][]byte{bdt, bng, bpg, eng, epg, edt}, "ENG", "D3A9AD"},
		// EAG closes the environment group while a text object opened
		// inside it is still open.
		{"EAG across open BPT", [][]byte{bdt, bpg, bag, bpt, eag, ept, epg, edt}, "EAG", "D3A9C9"},
		// EPT closes the text object while an environment group opened
		// inside it is still open.
		{"EPT across open BAG", [][]byte{bdt, bpg, bpt, ptx, bag, ept, eag, epg, edt}, "EPT", "D3A99B"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			for _, f := range tc.stream {
				buf.Write(f)
			}
			_, _, err := Reduce(&buf)
			if err == nil {
				t.Fatalf("Reduce accepted misnested stream %q", tc.name)
			}
			var derr afp.DecodeError
			if !errors.As(err, &derr) {
				t.Fatalf("error %T (%v), want afp.DecodeError", err, err)
			}
			if derr.Kind != afp.KindStructure {
				t.Errorf("Kind = %q, want %q", derr.Kind, afp.KindStructure)
			}
			if !strings.Contains(derr.Reason, tc.wantReason) {
				t.Errorf("Reason = %q, want it to name %q", derr.Reason, tc.wantReason)
			}
			if derr.FieldID != tc.wantField {
				t.Errorf("FieldID = %q, want %q", derr.FieldID, tc.wantField)
			}
		})
	}
}

// errReader fails on Read: it proves ReduceWithBudget rejects a non-positive
// budget before touching the input at all.
type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("must not read") }

// TestReduceWithBudget_RPSAmplification: an RPS-heavy PTX must not expand
// past the decoded-text budget (amplification DoS). RLENGTH is a uint16, so
// one sequence expands to at most 65535 bytes; four RLENGTH=2000 sequences
// here decode to 8000 UTF-8 bytes against a 4096 budget.
func TestReduceWithBudget_RPSAmplification(t *testing.T) {
	bdt := rawField(tBDT, append(ebcdicSpaces(8), 0x00, 0x00))
	bpg := rawField(tBPG, ebcdicSpaces(8))
	var payload []byte
	for i := 0; i < 4; i++ {
		// RPS: RLENGTH(2B) = 2000, RPTDATA = "A" (EBCDIC 0xC1).
		payload = append(payload, seq(0xEE, 0x07, 0xD0, 0xC1)...)
	}
	ptx := rawField(tPTX, payload)
	epg := rawField(tEPG, ebcdicSpaces(8))
	edt := rawField(tEDT, append(ebcdicSpaces(8), 0x00, 0x00))

	var buf bytes.Buffer
	for _, f := range [][]byte{bdt, bpg, ptx, epg, edt} {
		buf.Write(f)
	}
	// The error offset is the offending PTX's start offset, matching
	// DecodeError.Offset's documented meaning ("where decoding failed"):
	// offset and FieldID identify the same field.
	wantOffset := len(bdt) + len(bpg)

	_, _, err := ReduceWithBudget(&buf, 4096)
	if err == nil {
		t.Fatalf("ReduceWithBudget accepted an RPS stream expanding to 8000 bytes under a 4096 budget")
	}
	if !strings.Contains(err.Error(), "decoded text budget exceeded") {
		t.Fatalf("error = %q, want it to contain %q", err.Error(), "decoded text budget exceeded")
	}
	var derr afp.DecodeError
	if !errors.As(err, &derr) {
		t.Fatalf("error %T (%v), want afp.DecodeError", err, err)
	}
	if derr.FieldID != "D3EE9B" {
		t.Errorf("FieldID = %q, want D3EE9B", derr.FieldID)
	}
	if derr.Offset != wantOffset {
		t.Errorf("Offset = %d, want %d (offending PTX's start offset)", derr.Offset, wantOffset)
	}
}

// TestReduceWithBudget_NonPositiveBudget: a zero or negative budget is an
// immediate error, before any input is read.
func TestReduceWithBudget_NonPositiveBudget(t *testing.T) {
	for _, budget := range []int{0, -1} {
		_, _, err := ReduceWithBudget(errReader{}, budget)
		if err == nil {
			t.Fatalf("ReduceWithBudget(r, %d): got nil error", budget)
		}
		if !strings.Contains(err.Error(), "non-positive text budget") {
			t.Fatalf("ReduceWithBudget(r, %d): error = %q, want %q", budget, err.Error(), "non-positive text budget")
		}
	}
}

// TestReduceWithBudget_UnderBudget: a small valid document under the budget
// reduces exactly as before.
func TestReduceWithBudget_UnderBudget(t *testing.T) {
	bdt := rawField(tBDT, append(ebcdicSpaces(8), 0x00, 0x00))
	bpg := rawField(tBPG, ebcdicSpaces(8))
	ptx := rawField(tPTX, trn(0xC1))
	epg := rawField(tEPG, ebcdicSpaces(8))
	edt := rawField(tEDT, append(ebcdicSpaces(8), 0x00, 0x00))

	var buf bytes.Buffer
	for _, f := range [][]byte{bdt, bpg, ptx, epg, edt} {
		buf.Write(f)
	}
	doc, _, err := ReduceWithBudget(&buf, 4096)
	if err != nil {
		t.Fatalf("ReduceWithBudget: %v", err)
	}
	if len(doc.Pages) != 1 || len(doc.Pages[0].Runs) != 1 {
		t.Fatalf("doc has %d pages / %d runs, want 1 page with 1 run", len(doc.Pages), len(doc.Pages[0].Runs))
	}
	if got := doc.Pages[0].Runs[0].Text; got != "A" {
		t.Errorf("text = %q, want %q", got, "A")
	}
}

// TestReduceWithBudget_CumulativeAcrossPTX: the budget is document-scoped.
// Two PTX fields that are each under the budget on their own still fail when
// their combined decoded text exceeds it; the error names the second PTX's
// offset.
func TestReduceWithBudget_CumulativeAcrossPTX(t *testing.T) {
	bdt := rawField(tBDT, append(ebcdicSpaces(8), 0x00, 0x00))
	bpg := rawField(tBPG, ebcdicSpaces(8))
	// RPS: RLENGTH(2B) = 3000, RPTDATA = "A".
	ptx1 := rawField(tPTX, seq(0xEE, 0x0B, 0xB8, 0xC1))
	ptx2 := rawField(tPTX, seq(0xEE, 0x0B, 0xB8, 0xC1))
	epg := rawField(tEPG, ebcdicSpaces(8))
	edt := rawField(tEDT, append(ebcdicSpaces(8), 0x00, 0x00))

	var buf bytes.Buffer
	for _, f := range [][]byte{bdt, bpg, ptx1, ptx2, epg, edt} {
		buf.Write(f)
	}
	// The budget fails on ptx2, so the error offset is ptx2's start.
	wantOffset := len(bdt) + len(bpg) + len(ptx1)

	_, _, err := ReduceWithBudget(&buf, 4096)
	if err == nil {
		t.Fatalf("ReduceWithBudget accepted two PTX fields decoding to 6000 bytes under a 4096 budget")
	}
	var derr afp.DecodeError
	if !errors.As(err, &derr) || derr.Offset != wantOffset {
		t.Fatalf("error = %v (%T), want afp.DecodeError at offset %d", err, err, wantOffset)
	}
	if derr.FieldID != "D3EE9B" {
		t.Errorf("FieldID = %q, want D3EE9B", derr.FieldID)
	}
	if derr.Reason != "decoded text budget exceeded" {
		t.Errorf("Reason = %q, want %q", derr.Reason, "decoded text budget exceeded")
	}
}

// TestReduceWithBudget_DefaultBudget: the default budget is 1 MiB, as the
// panel contract fixes.
func TestReduceWithBudget_DefaultBudget(t *testing.T) {
	if DefaultMaxTextBytes != 1<<20 {
		t.Errorf("DefaultMaxTextBytes = %d, want %d", DefaultMaxTextBytes, 1<<20)
	}
}

// TestReduceWithBudget_ExactBudget: a document decoding to exactly the
// budget passes; budget+1 fails. ASCII TRN payloads, so source bytes ==
// decoded bytes and the boundary is exact.
func TestReduceWithBudget_ExactBudget(t *testing.T) {
	build := func(n int) []byte {
		bdt := rawField(tBDT, append(ebcdicSpaces(8), 0x00, 0x00))
		bpg := rawField(tBPG, ebcdicSpaces(8))
		as := make([]byte, n)
		for i := range as {
			as[i] = 0xC1 // EBCDIC "A"
		}
		ptx := rawField(tPTX, trn(as...))
		epg := rawField(tEPG, ebcdicSpaces(8))
		edt := rawField(tEDT, append(ebcdicSpaces(8), 0x00, 0x00))
		var buf bytes.Buffer
		for _, f := range [][]byte{bdt, bpg, ptx, epg, edt} {
			buf.Write(f)
		}
		return buf.Bytes()
	}
	if _, _, err := ReduceWithBudget(bytes.NewReader(build(64)), 64); err != nil {
		t.Fatalf("ReduceWithBudget at exactly the budget: %v", err)
	}
	_, _, err := ReduceWithBudget(bytes.NewReader(build(65)), 64)
	if err == nil {
		t.Fatal("ReduceWithBudget accepted budget+1 decoded bytes")
	}
	var derr afp.DecodeError
	if !errors.As(err, &derr) || derr.Reason != budgetExceededReason {
		t.Fatalf("error = %v (%T), want afp.DecodeError with budget reason", err, err)
	}
}

// TestReduceWithBudget_WrappedOffset: on 5A-wrapped streams the budget
// error's offset is the offending PTX's true wire start offset — SplitFields
// strips the 5A record marker, so each field contributes its bytes plus one
// framing byte.
func TestReduceWithBudget_WrappedOffset(t *testing.T) {
	bdt := rawField(tBDT, append(ebcdicSpaces(8), 0x00, 0x00))
	bpg := rawField(tBPG, ebcdicSpaces(8))
	// RPS: RLENGTH = 2000, RPTDATA = "A" — expands past a 1024 budget.
	ptx := rawField(tPTX, seq(0xEE, 0x07, 0xD0, 0xC1))
	epg := rawField(tEPG, ebcdicSpaces(8))
	edt := rawField(tEDT, append(ebcdicSpaces(8), 0x00, 0x00))

	var buf bytes.Buffer
	for _, f := range [][]byte{bdt, bpg, ptx, epg, edt} {
		buf.WriteByte(0x5A)
		buf.Write(f)
	}
	wantOffset := (len(bdt) + 1) + (len(bpg) + 1)

	_, _, err := ReduceWithBudget(&buf, 1024)
	if err == nil {
		t.Fatal("ReduceWithBudget accepted an RPS stream expanding past the budget on a 5A-wrapped stream")
	}
	var derr afp.DecodeError
	if !errors.As(err, &derr) {
		t.Fatalf("error %T (%v), want afp.DecodeError", err, err)
	}
	if derr.FieldID != "D3EE9B" {
		t.Errorf("FieldID = %q, want D3EE9B", derr.FieldID)
	}
	if derr.Offset != wantOffset {
		t.Errorf("Offset = %d, want %d (true wire start offset of the offending PTX)", derr.Offset, wantOffset)
	}
	if derr.Reason != budgetExceededReason {
		t.Errorf("Reason = %q, want %q", derr.Reason, budgetExceededReason)
	}
}

// TestReduce_RejectsC1Controls: a PTX whose EBCDIC bytes decode to C1 control
// characters must fail the reduction immediately — in visitPTX, before any
// text is appended — with an afp.DecodeError naming the PTX field and its
// stream offset. A PTX whose bytes decode to valid WinAnsi-mapped text still
// reduces fine.
func TestReduce_RejectsC1Controls(t *testing.T) {
	bdt := rawField(tBDT, append(ebcdicSpaces(8), 0x00, 0x00))
	bpg := rawField(tBPG, ebcdicSpaces(8))
	epg := rawField(tEPG, ebcdicSpaces(8))
	edt := rawField(tEDT, append(ebcdicSpaces(8), 0x00, 0x00))

	// 0x20 in code page 037 decodes to U+0080, the low end of the C1 range.
	c1PTX := rawField(tPTX, trn(0xC1, 0x20, 0xC2))
	var buf bytes.Buffer
	for _, f := range [][]byte{bdt, bpg, c1PTX, epg, edt} {
		buf.Write(f)
	}
	wantOffset := len(bdt) + len(bpg)

	_, _, err := Reduce(&buf)
	if err == nil {
		t.Fatal("Reduce accepted a PTX decoding to U+0080")
	}
	var derr afp.DecodeError
	if !errors.As(err, &derr) {
		t.Fatalf("error %T (%v), want afp.DecodeError", err, err)
	}
	if derr.FieldID != "D3EE9B" {
		t.Errorf("FieldID = %q, want D3EE9B", derr.FieldID)
	}
	if derr.Offset != wantOffset {
		t.Errorf("Offset = %d, want %d (field-start offset of the offending PTX)", derr.Offset, wantOffset)
	}

	// The same C1 PTX also fails under ReduceWithBudget.
	buf.Reset()
	for _, f := range [][]byte{bdt, bpg, c1PTX, epg, edt} {
		buf.Write(f)
	}
	if _, _, err := ReduceWithBudget(&buf, DefaultMaxTextBytes); err == nil {
		t.Error("ReduceWithBudget accepted a PTX decoding to U+0080")
	}

	// 0xC1/0xC2 decode to "AB" — WinAnsi-mappable, valid — so an equivalent
	// PTX reduces cleanly.
	buf.Reset()
	okPTX := rawField(tPTX, trn(0xC1, 0xC2))
	for _, f := range [][]byte{bdt, bpg, okPTX, epg, edt} {
		buf.Write(f)
	}
	doc, _, err := Reduce(&buf)
	if err != nil {
		t.Fatalf("Reduce of valid WinAnsi-mapped bytes = %v, want nil", err)
	}
	if len(doc.Pages) != 1 || len(doc.Pages[0].Runs) != 1 {
		t.Fatalf("got %d pages/%v runs, want 1 page with 1 run", len(doc.Pages), doc.Pages)
	}
	if got := doc.Pages[0].Runs[0].Text; got != "AB" {
		t.Errorf("run text = %q, want %q", got, "AB")
	}
}

// TestReduceWithCodePage_CP500: EBCDIC byte 0x4A decodes to U+00A2 (¢) in
// code page 037 but U+005B ([) in code page 500, so the same PTX yields
// different text under the two pages. ReduceWithCodePage must honor the
// given page; Reduce must stay on 37 (default behavior byte-identical).
func TestReduceWithCodePage_CP500(t *testing.T) {
	build := func() []byte {
		bdt := rawField(tBDT, append(ebcdicSpaces(8), 0x00, 0x00))
		bpg := rawField(tBPG, ebcdicSpaces(8))
		ptx := rawField(tPTX, trn(0x4A))
		epg := rawField(tEPG, ebcdicSpaces(8))
		edt := rawField(tEDT, append(ebcdicSpaces(8), 0x00, 0x00))
		var buf bytes.Buffer
		for _, f := range [][]byte{bdt, bpg, ptx, epg, edt} {
			buf.Write(f)
		}
		return buf.Bytes()
	}
	cp500, err := afp.LookupCodePage(500)
	if err != nil {
		t.Fatalf("LookupCodePage(500): %v", err)
	}
	doc500, _, err := ReduceWithCodePage(bytes.NewReader(build()), cp500)
	if err != nil {
		t.Fatalf("ReduceWithCodePage(500): %v", err)
	}
	if got := doc500.Pages[0].Runs[0].Text; got != "[" {
		t.Errorf("ReduceWithCodePage(500): text = %q, want %q", got, "[")
	}
	// Reduce on the same input must decode as code page 37, unchanged.
	doc37, _, err := Reduce(bytes.NewReader(build()))
	if err != nil {
		t.Fatalf("Reduce: %v", err)
	}
	if got := doc37.Pages[0].Runs[0].Text; got != "¢" {
		t.Errorf("Reduce: text = %q, want %q (code page 37)", got, "¢")
	}
}

// TestReduceWithCodePage_IMMNameDecoding: the IMM exclusion diagnostic decodes
// MMPName with the caller-selected code page, not hardcoded 37.
func TestReduceWithCodePage_IMMNameDecoding(t *testing.T) {
	tIMM := [3]byte{0xD3, 0xAB, 0xCC}
	build := func() []byte {
		bdt := rawField(tBDT, append(ebcdicSpaces(8), 0x00, 0x00))
		bpg := rawField(tBPG, ebcdicSpaces(8))
		name := append([]byte{0x4A}, ebcdicSpaces(7)...)
		imm := rawField(tIMM, name)
		epg := rawField(tEPG, ebcdicSpaces(8))
		edt := rawField(tEDT, append(ebcdicSpaces(8), 0x00, 0x00))
		var buf bytes.Buffer
		for _, f := range [][]byte{bdt, bpg, imm, epg, edt} {
			buf.Write(f)
		}
		return buf.Bytes()
	}
	cp500, err := afp.LookupCodePage(500)
	if err != nil {
		t.Fatalf("LookupCodePage(500): %v", err)
	}
	_, excls, err := ReduceWithCodePage(bytes.NewReader(build()), cp500)
	if err != nil {
		t.Fatalf("ReduceWithCodePage(500): %v", err)
	}
	if len(excls) != 1 {
		t.Fatalf("got %d exclusions, want 1 (IMM)", len(excls))
	}
	if d := excls[0].Diagnostic(); !strings.Contains(d, "[") {
		t.Errorf("diagnostic %q does not contain cp500-decoded name %q", d, "[")
	}
}

// TestReduceWithBudget_RPSBoundaryEdges: RPS edge cases pinned end-to-end
// through ReduceWithBudget. The white-box decoder equivalents of
// RLENGTH==0 (no-op) and RLENGTH==len(RPTDATA) (transparent-equivalent) are
// already covered by TestDecodePTOCA_RPS_ZeroLengthNoOp and
// TestDecodePTOCA_RPS_Repeat; these rows pin the reduce-level behavior plus
// the amplification boundary, which A3's exact-budget test exercises only
// with raw text (never an RPS sequence).
func TestReduceWithBudget_RPSBoundaryEdges(t *testing.T) {
	bdt := rawField(tBDT, append(ebcdicSpaces(8), 0x00, 0x00))
	bpg := rawField(tBPG, ebcdicSpaces(8))
	epg := rawField(tEPG, ebcdicSpaces(8))
	edt := rawField(tEDT, append(ebcdicSpaces(8), 0x00, 0x00))
	build := func(rlength uint16, rptdata ...byte) []byte {
		params := append([]byte{byte(rlength >> 8), byte(rlength)}, rptdata...)
		ptx := rawField(tPTX, seq(0xEE, params...))
		var buf bytes.Buffer
		for _, f := range [][]byte{bdt, bpg, ptx, epg, edt} {
			buf.Write(f)
		}
		return buf.Bytes()
	}

	cases := []struct {
		name      string
		rlength   uint16
		rptdata   []byte
		budget    int
		wantText  string // expected single-run text on success
		wantFails bool
	}{
		{"rlength zero emits nothing", 0, nil, 64, "", false},
		{"rlength equal to data length is transparent", 3, []byte{0xC1, 0xC2, 0xC3}, 64, "ABC", false},
		{"rlength at the budget passes", 64, []byte{0xC1}, 64, strings.Repeat("A", 64), false},
		{"rlength one past the budget fails", 65, []byte{0xC1}, 64, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc, _, err := ReduceWithBudget(bytes.NewReader(build(tc.rlength, tc.rptdata...)), tc.budget)
			if tc.wantFails {
				if err == nil {
					t.Fatalf("ReduceWithBudget accepted RLENGTH=%d under a %d budget", tc.rlength, tc.budget)
				}
				var derr afp.DecodeError
				if !errors.As(err, &derr) {
					t.Fatalf("error %T (%v), want afp.DecodeError", err, err)
				}
				if derr.Kind != afp.KindBudget {
					t.Errorf("Kind = %q, want %q", derr.Kind, afp.KindBudget)
				}
				if derr.Reason != budgetExceededReason {
					t.Errorf("Reason = %q, want %q", derr.Reason, budgetExceededReason)
				}
				if derr.FieldID != "D3EE9B" {
					t.Errorf("FieldID = %q, want D3EE9B (offending PTX)", derr.FieldID)
				}
				if want := len(bdt) + len(bpg); derr.Offset != want {
					t.Errorf("Offset = %d, want %d (offending PTX's start offset)", derr.Offset, want)
				}
				return
			}
			if err != nil {
				t.Fatalf("ReduceWithBudget: %v", err)
			}
			if len(doc.Pages) != 1 {
				t.Fatalf("got %d pages, want 1", len(doc.Pages))
			}
			runs := doc.Pages[0].Runs
			if tc.wantText == "" {
				if len(runs) != 0 {
					t.Fatalf("got %d runs, want 0", len(runs))
				}
				return
			}
			if len(runs) != 1 || runs[0].Text != tc.wantText {
				t.Fatalf("runs = %+v, want single run %q", runs, tc.wantText)
			}
		})
	}
}

// TestReduce_BAGEdgeCases: BAG/EAG group edge cases through Reduce. An empty
// group (BAG immediately followed by EAG) is the legal no-op; a BAG opened
// inside an open BAG group must fail — BAG nesting is not permitted, so the
// second BAG is rejected with the illegal-position error. A PTX inside a BAG
// group reduces normally: groups do not gate content, only pairing.
func TestReduce_BAGEdgeCases(t *testing.T) {
	bdt := rawField(tBDT, append(ebcdicSpaces(8), 0x00, 0x00))
	bpg := rawField(tBPG, ebcdicSpaces(8))
	bag := rawField(tBAG, nil)
	eag := rawField(tEAG, nil)
	ptx := rawField(tPTX, trn(0xC1))
	epg := rawField(tEPG, ebcdicSpaces(8))
	edt := rawField(tEDT, append(ebcdicSpaces(8), 0x00, 0x00))

	cases := []struct {
		name       string
		stream     [][]byte
		wantRuns   int // runs expected on the single page; -1 means failure
		wantReason string
	}{
		{"empty BAG/EAG group is valid", [][]byte{bdt, bpg, bag, eag, epg, edt}, 0, ""},
		{"BAG group containing a PTX reduces", [][]byte{bdt, bpg, bag, ptx, eag, epg, edt}, 1, ""},
		{"BAG nested to depth 2 fails", [][]byte{bdt, bpg, bag, bag, eag, eag, epg, edt}, -1, "BAG in illegal position"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			for _, f := range tc.stream {
				buf.Write(f)
			}
			doc, excls, err := Reduce(&buf)
			if tc.wantRuns < 0 {
				if err == nil {
					t.Fatalf("Reduce accepted %s", tc.name)
				}
				var derr afp.DecodeError
				if !errors.As(err, &derr) {
					t.Fatalf("error %T (%v), want afp.DecodeError", err, err)
				}
				if derr.Kind != afp.KindStructure {
					t.Errorf("Kind = %q, want %q", derr.Kind, afp.KindStructure)
				}
				if !strings.Contains(derr.Reason, tc.wantReason) {
					t.Errorf("Reason = %q, want it to contain %q", derr.Reason, tc.wantReason)
				}
				if derr.FieldID != "D3A8C9" {
					t.Errorf("FieldID = %q, want D3A8C9 (the nested BAG)", derr.FieldID)
				}
				return
			}
			if err != nil {
				t.Fatalf("Reduce: %v", err)
			}
			if len(excls) != 0 {
				t.Fatalf("got %d exclusions, want 0: %v", len(excls), excls)
			}
			if len(doc.Pages) != 1 {
				t.Fatalf("got %d pages, want 1", len(doc.Pages))
			}
			if got := len(doc.Pages[0].Runs); got != tc.wantRuns {
				t.Fatalf("got %d runs, want %d", got, tc.wantRuns)
			}
			if tc.wantRuns == 1 && doc.Pages[0].Runs[0].Text != "A" {
				t.Errorf("run text = %q, want %q", doc.Pages[0].Runs[0].Text, "A")
			}
		})
	}
}

// TestReduceWithBudget_ReadError: a failing reader surfaces as the "afp:
// Fields" wrapper — the first error return in the reduce pipeline, and it
// is not an afp.DecodeError (no field was decoded).
func TestReduceWithBudget_ReadError(t *testing.T) {
	_, _, err := ReduceWithBudget(errReader{}, 4096)
	if err == nil {
		t.Fatal("ReduceWithBudget accepted a failing reader")
	}
	var derr afp.DecodeError
	if errors.As(err, &derr) {
		t.Fatalf("error = %v: a read failure is not a DecodeError", err)
	}
	if !strings.Contains(err.Error(), "afp: Fields:") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "afp: Fields:")
	}
}

// TestReduce_Corrupt5AMarker: a corrupt record marker on a later 5A-wrapped
// field fails the reduction through SplitFields (C1's loud check), with the
// stream offset of the damaged record — not the PTX offset machinery.
func TestReduce_Corrupt5AMarker(t *testing.T) {
	bdt := rawField(tBDT, append(ebcdicSpaces(8), 0x00, 0x00))
	bpg := rawField(tBPG, ebcdicSpaces(8))
	epg := rawField(tEPG, ebcdicSpaces(8))

	var buf bytes.Buffer
	buf.WriteByte(0x5A)
	buf.Write(bdt)
	buf.WriteByte(0x5A)
	buf.Write(bpg)
	buf.WriteByte(0x00) // corrupt marker: stream damage, not a skippable byte
	buf.Write(epg)

	_, _, err := Reduce(&buf)
	if err == nil {
		t.Fatal("Reduce accepted a 5A-wrapped stream with a corrupt record marker")
	}
	var derr afp.DecodeError
	if !errors.As(err, &derr) {
		t.Fatalf("error %T (%v), want afp.DecodeError", err, err)
	}
	if derr.Kind != afp.KindStructure {
		t.Errorf("Kind = %q, want %q", derr.Kind, afp.KindStructure)
	}
	if !strings.Contains(derr.Reason, "expected 5A record marker") {
		t.Errorf("Reason = %q, want it to contain %q", derr.Reason, "expected 5A record marker")
	}
	if want := (1 + len(bdt)) + (1 + len(bpg)); derr.Offset != want {
		t.Errorf("Offset = %d, want %d (start of the damaged record)", derr.Offset, want)
	}
}

// TestReduce_FinishOffset_WrappedStream: end of stream with an open page on
// a 5A-wrapped stream must report the true wire end, counting every record
// marker — the streaming reducer derives the per-record marker from the
// gap between consecutive field offsets.
func TestReduce_FinishOffset_WrappedStream(t *testing.T) {
	bdt := rawField(tBDT, append(ebcdicSpaces(8), 0x00, 0x00))
	bpg := rawField(tBPG, ebcdicSpaces(8))

	var buf bytes.Buffer
	buf.WriteByte(0x5A)
	buf.Write(bdt)
	buf.WriteByte(0x5A)
	buf.Write(bpg)
	// No EPG: the stream ends with the page open.

	_, _, err := Reduce(&buf)
	if err == nil {
		t.Fatal("Reduce accepted a stream ending with an open page")
	}
	var derr afp.DecodeError
	if !errors.As(err, &derr) {
		t.Fatalf("error %T (%v), want afp.DecodeError", err, err)
	}
	if want := (1 + len(bdt)) + (1 + len(bpg)); derr.Offset != want {
		t.Errorf("Offset = %d, want %d (end-of-stream wire offset)", derr.Offset, want)
	}
}

// TestReduce_ReducerFailSites: every st.fail site in the reducer, pinned with
// the expected Kind and naming. Sites already covered elsewhere (cited here,
// not duplicated):
//   - nested BDT / EDT without BDT / EDT with open page / BPG outside
//     document / nested BPG / EPG without BPG / PTX outside page / open
//     document at finish: TestReduce_PairingErrors
//   - EPG with open BAG: TestReduce_BAGMustNotCrossEPG
//   - EPG with open BPT: TestReduce_BPTMustNotCrossEPG
//   - BAG nested in page: TestReduce_BAGEdgeCases
//   - PTX decode budget error (wrapped with PTX identity): TestReduceWithBudget_RPSAmplification
//   - PTX C1 rejection: TestReduce_RejectsC1Controls
//   - visitPTX post-decode budget cap (n > st.textLeft): unreachable — the
//     decoder's fail-fast aborts on the same bytes first; kept as
//     defense-in-depth, documented in reduce.go.
//   - DecodeField length-mismatch check: unreachable via Reduce —
//     SplitFields always emits exactly the declared field bytes, so no
//     Reduce-level test can reach it.
func TestReduce_ReducerFailSites(t *testing.T) {
	tBNG := [3]byte{0xD3, 0xA8, 0xAD}
	tENG := [3]byte{0xD3, 0xA9, 0xAD}
	tMCF := [3]byte{0xD3, 0xAB, 0x8A}
	tPTD := [3]byte{0xD3, 0xB1, 0x9B}
	bdt := rawField(tBDT, append(ebcdicSpaces(8), 0x00, 0x00))
	edt := rawField(tEDT, append(ebcdicSpaces(8), 0x00, 0x00))
	bpg := rawField(tBPG, ebcdicSpaces(8))
	epg := rawField(tEPG, ebcdicSpaces(8))
	bng := rawField(tBNG, ebcdicSpaces(8))
	eng := rawField(tENG, ebcdicSpaces(8))
	bag := rawField(tBAG, nil)
	eag := rawField(tEAG, nil)
	bpt := rawField(tBPT, nil)
	ept := rawField(tEPT, nil)
	mcf := rawField(tMCF, ebcdicSpaces(8))
	ptd := rawField(tPTD, ebcdicSpaces(8))
	// PTX whose SCFL selects local font ID 5: no pinned metrics mapping
	// exists, so fontIDForLID fails the reduction loudly.
	ptxSCFL := rawField(tPTX, append(seq(0xF0, 0x05), trn(0xC1)...))
	// PTX whose control sequence extends past the PTX data: the decoder
	// error passes through visitPTX unwrapped (only budget errors are
	// re-attributed).
	ptxTrunc := rawField(tPTX, []byte{0x2B, 0xD3, 0xFF, 0xD2, 0x00})

	cases := []struct {
		name       string
		stream     [][]byte
		wantKind   afp.ErrorKind
		wantReason string // substring
		wantID     string // expected FieldID ("" when the error sets none)
	}{
		{"EDT with unclosed BNG", [][]byte{bdt, bng, edt}, afp.KindStructure, "EDT with unclosed group", "D3A9A8"},
		{"BNG outside document", [][]byte{bng}, afp.KindStructure, "BNG in illegal position", "D3A8AD"},
		{"BNG nested", [][]byte{bdt, bng, bng}, afp.KindStructure, "BNG in illegal position", "D3A8AD"},
		{"BNG on open page", [][]byte{bdt, bpg, bng}, afp.KindStructure, "BNG in illegal position", "D3A8AD"},
		{"ENG without BNG", [][]byte{bdt, eng}, afp.KindStructure, "ENG without BNG", "D3A9AD"},
		{"BAG outside document", [][]byte{bag}, afp.KindStructure, "BAG in illegal position", "D3A8C9"},
		{"BAG outside page", [][]byte{bdt, bag}, afp.KindStructure, "BAG in illegal position", "D3A8C9"},
		{"EAG without BAG", [][]byte{bdt, eag}, afp.KindStructure, "EAG without BAG", "D3A9C9"},
		{"BPT outside page", [][]byte{bdt, bpt}, afp.KindStructure, "BPT outside page or nested", "D3A89B"},
		{"BPT nested", [][]byte{bdt, bpg, bpt, bpt}, afp.KindStructure, "BPT outside page or nested", "D3A89B"},
		{"EPT without BPT", [][]byte{bdt, ept}, afp.KindStructure, "EPT without BPT", "D3A99B"},
		{"MCF outside document", [][]byte{mcf}, afp.KindStructure, "environment field outside document", "D3AB8A"},
		{"PTD outside document", [][]byte{ptd}, afp.KindStructure, "environment field outside document", "D3B19B"},
		{"SCFL nonzero LID", [][]byte{bdt, bpg, ptxSCFL, epg, edt}, afp.KindValidation, "SCFL local font ID 5", "D3EE9B"},
		{"PTX decoder error passes through", [][]byte{bdt, bpg, ptxTrunc, epg, edt}, afp.KindStructure, "control sequence extends past PTX data", "D3EE9B"},
		{"finish with open page", [][]byte{bdt, bpg}, afp.KindStructure, "end of stream with open page: missing EPG", ""},
		{"finish with unclosed group", [][]byte{bdt, bng}, afp.KindStructure, "end of stream with unclosed group", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			for _, f := range tc.stream {
				buf.Write(f)
			}
			_, _, err := Reduce(&buf)
			if err == nil {
				t.Fatalf("Reduce accepted %s", tc.name)
			}
			var derr afp.DecodeError
			if !errors.As(err, &derr) {
				t.Fatalf("error %T (%v), want afp.DecodeError", err, err)
			}
			if derr.Kind != tc.wantKind {
				t.Errorf("Kind = %q, want %q", derr.Kind, tc.wantKind)
			}
			if !strings.Contains(derr.Reason, tc.wantReason) {
				t.Errorf("Reason = %q, want it to contain %q", derr.Reason, tc.wantReason)
			}
			if derr.FieldID != tc.wantID {
				t.Errorf("FieldID = %q, want %q", derr.FieldID, tc.wantID)
			}
		})
	}
}

// TestReduce_BNGENGAndMCFValid: the valid counterparts of the fail sites
// above — balanced BNG/ENG brackets the document and an in-document MCF is
// accepted (v1: noted, parameters uninterpreted), not an abort.
func TestReduce_BNGENGAndMCFValid(t *testing.T) {
	bdt := rawField(tBDT, append(ebcdicSpaces(8), 0x00, 0x00))
	edt := rawField(tEDT, append(ebcdicSpaces(8), 0x00, 0x00))
	bng := rawField([3]byte{0xD3, 0xA8, 0xAD}, ebcdicSpaces(8))
	eng := rawField([3]byte{0xD3, 0xA9, 0xAD}, ebcdicSpaces(8))
	mcf := rawField([3]byte{0xD3, 0xAB, 0x8A}, ebcdicSpaces(8))

	var buf bytes.Buffer
	for _, f := range [][]byte{bdt, bng, mcf, eng, edt} {
		buf.Write(f)
	}
	doc, excls, err := Reduce(&buf)
	if err != nil {
		t.Fatalf("Reduce: %v", err)
	}
	if len(doc.Pages) != 0 {
		t.Fatalf("got %d pages, want 0", len(doc.Pages))
	}
	if len(excls) != 0 {
		t.Fatalf("got %d exclusions, want 0: %v", len(excls), excls)
	}
}

// bogusField implements afp.Field with a concrete type the reducer's visit
// switch never names: it reaches the defensive default branch.
type bogusField struct{}

func (bogusField) TypeID() string { return "D30000" }
func (bogusField) Encode() []byte { return nil }

// TestReduce_VisitUnhandledFieldType: the visit switch's defensive default
// fails loudly (KindValidation) instead of silently ignoring the field. It
// is unreachable through DecodeField — every typed field the registry
// produces is handled — so it is pinned by direct visit.
func TestReduce_VisitUnhandledFieldType(t *testing.T) {
	st := &reduceState{}
	err := st.visit(bogusField{})
	if err == nil {
		t.Fatal("visit accepted an unhandled field type")
	}
	var derr afp.DecodeError
	if !errors.As(err, &derr) {
		t.Fatalf("error %T (%v), want afp.DecodeError", err, err)
	}
	if derr.Kind != afp.KindValidation {
		t.Errorf("Kind = %q, want %q", derr.Kind, afp.KindValidation)
	}
	if !strings.Contains(derr.Reason, "unhandled field type in reduction walk") {
		t.Errorf("Reason = %q, want it to contain %q", derr.Reason, "unhandled field type in reduction walk")
	}
	if derr.FieldID != "D30000" {
		t.Errorf("FieldID = %q, want D30000", derr.FieldID)
	}
}

// TestReduce_VisitRawUnknownContractedField: the visitRaw switch's defensive
// default. A non-excluded RawField whose type ID no case handles must fail
// loudly. Unreachable through DecodeField (all contracted IDs are cased),
// so pinned by direct visit.
func TestReduce_VisitRawUnknownContractedField(t *testing.T) {
	f := &afp.RawField{
		Intro: afp.Introducer{Length: 8, TypeID: [3]byte{0xD3, 0x00, 0x00}},
	}
	st := &reduceState{inDoc: true}
	if err := st.visit(f); err == nil {
		t.Fatal("visit accepted a contracted field without v1 reduction semantics")
	} else {
		var derr afp.DecodeError
		if !errors.As(err, &derr) {
			t.Fatalf("error %T (%v), want afp.DecodeError", err, err)
		}
		if derr.Kind != afp.KindValidation {
			t.Errorf("Kind = %q, want %q", derr.Kind, afp.KindValidation)
		}
		if !strings.Contains(derr.Reason, "contracted field without v1 reduction semantics") {
			t.Errorf("Reason = %q, want it to contain %q", derr.Reason, "contracted field without v1 reduction semantics")
		}
		if derr.FieldID != "D30000" {
			t.Errorf("FieldID = %q, want D30000", derr.FieldID)
		}
	}
}
