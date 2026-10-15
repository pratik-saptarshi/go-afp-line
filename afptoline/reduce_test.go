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
