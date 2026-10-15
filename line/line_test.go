package line

import (
	"math"
	"strings"
	"testing"
)

func validRun() TextRun {
	return TextRun{Text: "Hello", X: 72, Y: 720, FontSize: 10, FontID: "FIX-MONO-10"}
}

func validPage() Page {
	return Page{Number: 1, Width: 612, Height: 792, Runs: []TextRun{validRun()}}
}

func TestDocument_Validate(t *testing.T) {
	t.Run("valid document passes", func(t *testing.T) {
		doc := Document{Pages: []Page{validPage(), {Number: 2, Width: 612, Height: 792}}}
		if err := doc.Validate(); err != nil {
			t.Fatalf("Validate() = %v, want nil", err)
		}
	})

	t.Run("empty document is valid", func(t *testing.T) {
		if err := (Document{}).Validate(); err != nil {
			t.Fatalf("Validate() = %v, want nil", err)
		}
	})

	t.Run("propagates page error with location", func(t *testing.T) {
		doc := Document{Pages: []Page{validPage(), {Number: 0, Width: 612, Height: 792}}}
		err := doc.Validate()
		if err == nil {
			t.Fatal("Validate() = nil, want error for page number 0")
		}
		if !strings.Contains(err.Error(), "pages[1]") {
			t.Fatalf("error %q does not locate pages[1]", err)
		}
	})

	t.Run("rejects run with negative font size", func(t *testing.T) {
		run := validRun()
		run.FontSize = -10
		doc := Document{Pages: []Page{{Number: 1, Width: 612, Height: 792, Runs: []TextRun{run}}}}
		if err := doc.Validate(); err == nil {
			t.Fatal("Validate() = nil, want error for negative font size")
		}
	})

	t.Run("rejects run with zero font size", func(t *testing.T) {
		run := validRun()
		run.FontSize = 0
		doc := Document{Pages: []Page{{Number: 1, Width: 612, Height: 792, Runs: []TextRun{run}}}}
		if err := doc.Validate(); err == nil {
			t.Fatal("Validate() = nil, want error for zero font size")
		}
	})

	t.Run("rejects run with NaN coordinate", func(t *testing.T) {
		run := validRun()
		run.X = math.NaN()
		doc := Document{Pages: []Page{{Number: 1, Width: 612, Height: 792, Runs: []TextRun{run}}}}
		if err := doc.Validate(); err == nil {
			t.Fatal("Validate() = nil, want error for NaN X")
		}
	})

	t.Run("rejects run with infinite coordinate", func(t *testing.T) {
		run := validRun()
		run.Y = math.Inf(1)
		doc := Document{Pages: []Page{{Number: 1, Width: 612, Height: 792, Runs: []TextRun{run}}}}
		if err := doc.Validate(); err == nil {
			t.Fatal("Validate() = nil, want error for +Inf Y")
		}
	})

	t.Run("rejects run with empty font ID", func(t *testing.T) {
		run := validRun()
		run.FontID = ""
		doc := Document{Pages: []Page{{Number: 1, Width: 612, Height: 792, Runs: []TextRun{run}}}}
		if err := doc.Validate(); err == nil {
			t.Fatal("Validate() = nil, want error for empty FontID")
		}
	})

	t.Run("allows negative coordinates (off-page placement is legal)", func(t *testing.T) {
		run := validRun()
		run.X, run.Y = -5, -5
		doc := Document{Pages: []Page{{Number: 1, Width: 612, Height: 792, Runs: []TextRun{run}}}}
		if err := doc.Validate(); err != nil {
			t.Fatalf("Validate() = %v, want nil for off-page coordinates", err)
		}
	})
}

func TestPage_Validate(t *testing.T) {
	t.Run("rejects non-positive dimensions", func(t *testing.T) {
		for _, dims := range [][2]float64{{0, 792}, {612, 0}, {-612, 792}} {
			p := Page{Number: 1, Width: dims[0], Height: dims[1]}
			if err := p.Validate(); err == nil {
				t.Fatalf("Validate() = nil for dimensions %v, want error", dims)
			}
		}
	})

	t.Run("rejects page number zero", func(t *testing.T) {
		p := Page{Number: 0, Width: 612, Height: 792}
		if err := p.Validate(); err == nil {
			t.Fatal("Validate() = nil for page number 0, want error")
		}
	})

	t.Run("rejects +Inf dimensions and font size", func(t *testing.T) {
		inf := math.Inf(1)
		if err := (Page{Number: 1, Width: inf, Height: 792}).Validate(); err == nil {
			t.Fatal("Validate() = nil for +Inf width, want error")
		}
		if err := (Page{Number: 1, Width: 612, Height: inf}).Validate(); err == nil {
			t.Fatal("Validate() = nil for +Inf height, want error")
		}
		r := TextRun{Text: "x", X: 72, Y: 720, FontSize: inf, FontID: "FIX-MONO-10"}
		if err := r.Validate(); err == nil {
			t.Fatal("Validate() = nil for +Inf font size, want error")
		}
	})

	t.Run("rejects NaN and -Inf dimensions and font size", func(t *testing.T) {
		for _, v := range []float64{math.NaN(), math.Inf(-1)} {
			if err := (Page{Number: 1, Width: v, Height: 792}).Validate(); err == nil {
				t.Errorf("Validate() = nil for width %v, want error", v)
			}
			if err := (Page{Number: 1, Width: 612, Height: v}).Validate(); err == nil {
				t.Errorf("Validate() = nil for height %v, want error", v)
			}
			r := TextRun{Text: "x", X: 72, Y: 720, FontSize: v, FontID: "FIX-MONO-10"}
			if err := r.Validate(); err == nil {
				t.Errorf("Validate() = nil for font size %v, want error", v)
			}
		}
	})

	t.Run("locates failing run", func(t *testing.T) {
		p := validPage()
		p.Runs = append(p.Runs, TextRun{Text: "bad", X: 10, Y: 10, FontSize: -1, FontID: "FIX-MONO-10"})
		err := p.Validate()
		if err == nil {
			t.Fatal("Validate() = nil, want error")
		}
		if !strings.Contains(err.Error(), "runs[1]") {
			t.Fatalf("error %q does not locate runs[1]", err)
		}
	})
}

func TestFontMetrics_Lookup(t *testing.T) {
	t.Run("monospace fixture font returns declared widths", func(t *testing.T) {
		m, ok := LookupFont("FIX-MONO-10")
		if !ok {
			t.Fatal("LookupFont(FIX-MONO-10) not found")
		}
		// Monospace 600/1000 em at 10pt: "AB" -> 2 * 0.6 * 10 = 12pt.
		if w := m.Width("AB", 10); w != 12 {
			t.Fatalf("Width(AB, 10) = %v, want 12", w)
		}
		if w := m.Width("", 10); w != 0 {
			t.Fatalf("Width(\"\", 10) = %v, want 0", w)
		}
	})

	t.Run("proportional fixture font uses per-glyph advances", func(t *testing.T) {
		m, ok := LookupFont("FIX-PROP-10")
		if !ok {
			t.Fatal("LookupFont(FIX-PROP-10) not found")
		}
		// Declared: 'A' = 700, ' ' = 250 (units of a 1000 upm font) at 10pt.
		want := (700.0 + 250.0) / 1000 * 10
		if w := m.Width("A ", 10); w != want {
			t.Fatalf("Width(\"A \", 10) = %v, want %v", w, want)
		}
	})

	t.Run("unknown font is not found", func(t *testing.T) {
		if _, ok := LookupFont("NOPE-99"); ok {
			t.Fatal("LookupFont(NOPE-99) found, want not found")
		}
	})

	t.Run("missing glyph falls back to default advance", func(t *testing.T) {
		m, ok := LookupFont("FIX-PROP-10")
		if !ok {
			t.Fatal("LookupFont(FIX-PROP-10) not found")
		}
		// '\u2603' is not in the declared table; default advance applies.
		if w := m.Width("\u2603", 10); w <= 0 {
			t.Fatalf("Width(snowman, 10) = %v, want positive default advance", w)
		}
	})
}

func TestExclusion_Diagnostic(t *testing.T) {
	t.Run("diagnostic names field type and reason", func(t *testing.T) {
		e := Exclusion{FieldType: "GOCA", Reason: "vector graphics are outside the v1 contracted subset"}
		d := e.Diagnostic()
		if !strings.Contains(d, "GOCA") {
			t.Fatalf("diagnostic %q does not name the field type", d)
		}
		if !strings.Contains(d, "outside the v1 contracted subset") {
			t.Fatalf("diagnostic %q does not carry the reason", d)
		}
	})

	t.Run("IOCA exclusion is classified per the contract", func(t *testing.T) {
		e, ok := ClassifyExclusion("BIM")
		if !ok {
			t.Fatal("ClassifyExclusion(BIM) not found; IOCA must be classified as excluded")
		}
		if !strings.Contains(strings.ToUpper(e.Diagnostic()), "IOCA") {
			t.Fatalf("diagnostic %q does not name IOCA", e.Diagnostic())
		}
	})

	t.Run("closed list covers the contract categories", func(t *testing.T) {
		for _, acronym := range []string{"BGR", "BBC", "BCF", "BCA", "BMO", "BOC", "NOP", "TLE"} {
			if _, ok := ClassifyExclusion(acronym); !ok {
				t.Fatalf("ClassifyExclusion(%s) not found; contract §9 requires it", acronym)
			}
		}
	})

	t.Run("in-subset field is not an exclusion", func(t *testing.T) {
		if _, ok := ClassifyExclusion("PTX"); ok {
			t.Fatal("ClassifyExclusion(PTX) found; PTX is in-subset, not an exclusion")
		}
	})
}

// TestTextRunValidate_RejectsC1: C1 control characters (U+0080–U+009F) have
// no WinAnsi encoding, so a run containing one is rejected per policy —
// never mapped, never silently dropped. The check runs on the decoded
// Unicode rune, independent of code page. Boundary runes U+007F and U+00A0
// are not C1 and still pass.
func TestTextRunValidate_RejectsC1(t *testing.T) {
	for _, text := range []string{
		"a\u0080b", // EBCDIC-037 byte 0x20 decodes here; low boundary
		"\u0085",   // NEL, mid-range
		"\u009F",   // high boundary
	} {
		run := validRun()
		run.Text = text
		if err := run.Validate(); err == nil {
			t.Errorf("Validate(%q) = nil, want C1 rejection", text)
		}
	}
	for _, text := range []string{
		"Hello",
		"a\u007Fb",  // DEL is not C1
		"a\u00A0b",  // NBSP is not C1
		"caf\u00E9", // WinAnsi-mappable
		"\u20AC",    // € is WinAnsi-encodable, not C1
	} {
		run := validRun()
		run.Text = text
		if err := run.Validate(); err != nil {
			t.Errorf("Validate(%q) = %v, want nil (not C1)", text, err)
		}
	}
}
