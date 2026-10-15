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
		{"STATEMENT LINE TWO", 0, 720, "FIX-MONO-10", 10},
	})
	checkRuns(t, 2, doc.Pages[1].Runs, []wantRun{
		{"STATEMENT LINE THREE", 0, 756, "FIX-MONO-10", 10},
		{"STATEMENT LINE FOUR", 0, 720, "FIX-MONO-10", 10},
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
	bag := rawField(tBAG, nil)

	cases := []struct {
		name   string
		stream [][]byte
	}{
		{"nested BDT", [][]byte{bdt, bdt}},
		{"EDT without BDT", [][]byte{edt}},
		{"BPG before BDT", [][]byte{bpg}},
		{"EPG without BPG", [][]byte{bdt, epg}},
		{"nested BPG", [][]byte{bdt, bpg, bpg}},
		{"PTX outside page", [][]byte{bdt, ptx}},
		{"EDT with open page", [][]byte{bdt, bpg, edt}},
		{"unclosed document", [][]byte{bdt, bpg, epg}},
		{"unclosed BAG", [][]byte{bdt, bag, edt}},
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
	wantOffset := len(bdt) + len(bpg) + len(c1PTX)

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
		t.Errorf("Offset = %d, want %d (cumulative offset through the offending PTX)", derr.Offset, wantOffset)
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
