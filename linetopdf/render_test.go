package linetopdf

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/pratik-saptarshi/go-afp-line/afptoline"
	"github.com/pratik-saptarshi/go-afp-line/line"
)

// courierCapHeight is Courier's cap-height factor (cap height / em), measured
// by the independent reader (poppler) in the Task 0 writer spike: for 10pt
// Courier at baseline Y on an H-pt page, yMin == H - Y - 0.629*size exactly.
// Our code never uses this constant: it is a property of the standard-14
// Courier font outlines, so poppler's measurement is a true third-party
// observation of where our Tm placement landed. The v1 fixture text is all
// caps, so ink top == cap height.
const courierCapHeight = 0.629

// positionTolerance is the subset contract's geometry tolerance.
const positionTolerance = 1.0

type bboxWord struct {
	XMin float64 `xml:"xMin,attr"`
	YMin float64 `xml:"yMin,attr"`
	XMax float64 `xml:"xMax,attr"`
	YMax float64 `xml:"yMax,attr"`
	Text string  `xml:",chardata"`
}

type bboxPage struct {
	Width  float64    `xml:"width,attr"`
	Height float64    `xml:"height,attr"`
	Words  []bboxWord `xml:"word"`
}

type bboxDoc struct {
	// pdftotext -bbox wraps pages as html > body > doc > page.
	Pages []bboxPage `xml:"body>doc>page"`
}

// renderFixtureToPDF reduces statement-001.afp and renders it to a temp PDF
// file, returning the document and the PDF path. The PDF is read back with
// pdftotext (poppler) — the independent reader, not pdfcpu.
func renderFixtureToPDF(t *testing.T) (line.Document, string) {
	t.Helper()
	if _, err := exec.LookPath("pdftotext"); err != nil {
		if os.Getenv("CI") == "true" {
			t.Fatal("pdftotext (poppler) not available: independent reader required")
		}
		t.Skip("pdftotext (poppler) not available: independent reader required")
	}
	f, err := os.Open("../afptoline/fixtures/statement-001.afp")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()
	doc, excl, err := afptoline.Reduce(f)
	if err != nil {
		t.Fatalf("reduce: %v", err)
	}
	if len(excl) != 0 {
		t.Fatalf("expected no exclusions, got %d", len(excl))
	}
	var buf bytes.Buffer
	if err := Render(doc, &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	pdfPath := filepath.Join(t.TempDir(), "out.pdf")
	if err := os.WriteFile(pdfPath, buf.Bytes(), 0644); err != nil {
		t.Fatalf("write pdf: %v", err)
	}
	return doc, pdfPath
}

// readWords runs pdftotext -bbox on pdfPath and returns the words per page.
func readWords(t *testing.T, pdfPath string) []bboxPage {
	t.Helper()
	xmlPath := filepath.Join(t.TempDir(), "out.xml")
	cmd := exec.Command("pdftotext", "-bbox", pdfPath, xmlPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("pdftotext -bbox: %v\n%s", err, out)
	}
	data, err := os.ReadFile(xmlPath)
	if err != nil {
		t.Fatalf("read bbox xml: %v", err)
	}
	var doc bboxDoc
	if err := xml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse bbox xml: %v", err)
	}
	return doc.Pages
}

// wordsForRun returns the words belonging to run, clustered by vertical
// proximity to the run's expected ink top, sorted left to right.
func wordsForRun(t *testing.T, page bboxPage, pageH float64, r line.TextRun) []bboxWord {
	t.Helper()
	expectedYMin := pageH - r.Y - courierCapHeight*r.FontSize
	var ws []bboxWord
	for _, w := range page.Words {
		if math.Abs(w.YMin-expectedYMin) < r.FontSize {
			ws = append(ws, w)
		}
	}
	if len(ws) == 0 {
		t.Fatalf("no words found near expected yMin %.2f for run %q", expectedYMin, r.Text)
	}
	sort.Slice(ws, func(i, j int) bool { return ws[i].XMin < ws[j].XMin })
	return ws
}

func joinWords(ws []bboxWord) string {
	parts := make([]string, len(ws))
	for i, w := range ws {
		parts[i] = strings.TrimSpace(w.Text)
	}
	return strings.Join(parts, " ")
}

func TestRender_Positions(t *testing.T) {
	doc, pdfPath := renderFixtureToPDF(t)
	pages := readWords(t, pdfPath)
	if len(pages) != len(doc.Pages) {
		t.Fatalf("page count: pdf has %d, doc has %d", len(pages), len(doc.Pages))
	}
	for i, p := range doc.Pages {
		bp := pages[i]
		for _, r := range p.Runs {
			ws := wordsForRun(t, bp, p.Height, r)
			if got := joinWords(ws); got != r.Text {
				t.Errorf("page %d: text mismatch: got %q want %q", p.Number, got, r.Text)
			}
			if d := math.Abs(ws[0].XMin - r.X); d > positionTolerance {
				t.Errorf("page %d run %q: xMin %.3f, want X %.3f (delta %.3f > %.1fpt)",
					p.Number, r.Text, ws[0].XMin, r.X, d, positionTolerance)
			}
			expectedYMin := p.Height - r.Y - courierCapHeight*r.FontSize
			if d := math.Abs(ws[0].YMin - expectedYMin); d > positionTolerance {
				t.Errorf("page %d run %q: yMin %.3f, want %.3f (delta %.3f > %.1fpt)",
					p.Number, r.Text, ws[0].YMin, expectedYMin, d, positionTolerance)
			}
		}
	}
}

func TestRender_WidthMatchesMetrics(t *testing.T) {
	doc, pdfPath := renderFixtureToPDF(t)
	pages := readWords(t, pdfPath)
	for i, p := range doc.Pages {
		bp := pages[i]
		for _, r := range p.Runs {
			fm, ok := line.LookupFont(r.FontID)
			if !ok {
				t.Fatalf("no pinned metrics for font %q", r.FontID)
			}
			// Expected width from the pinned table. The PDF itself only
			// carries a /Courier reference: poppler measures ink extents
			// from real Courier outlines, so this compares two independent
			// sources — meaningful, not circular.
			expected := fm.Width(r.Text, r.FontSize)
			ws := wordsForRun(t, bp, p.Height, r)
			measured := ws[len(ws)-1].XMax - ws[0].XMin
			if d := math.Abs(measured - expected); d > positionTolerance {
				t.Errorf("page %d run %q: placed width %.3f, metrics width %.3f (delta %.3f > %.1fpt)",
					p.Number, r.Text, measured, expected, d, positionTolerance)
			}
		}
	}
}

func TestRender_Untagged(t *testing.T) {
	doc, _ := renderFixtureToPDF(t)
	var buf bytes.Buffer
	if err := Render(doc, &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	raw := buf.Bytes()
	if bytes.Contains(raw, []byte("/StructTreeRoot")) {
		t.Error("output contains /StructTreeRoot: must be untagged")
	}
	if !bytes.Contains(raw, []byte("untagged; not accessible")) {
		t.Error(`producer metadata must state "untagged; not accessible"`)
	}
}

func TestRender_WinAnsiEncoding(t *testing.T) {
	doc := line.Document{Pages: []line.Page{{
		Number: 1, Width: 612, Height: 792,
		Runs: []line.TextRun{{Text: "café", X: 0, Y: 700, FontSize: 10, FontID: "FIX-MONO-10"}},
	}}}
	var buf bytes.Buffer
	if err := Render(doc, &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	raw := buf.Bytes()
	// Standard-14 fonts use WinAnsi: é must be the single byte 0xE9,
	// not the UTF-8 pair 0xC3 0xA9 (which reads as "Ã©").
	if !bytes.Contains(raw, []byte("(caf\xe9)")) {
		t.Error(`content stream does not contain WinAnsi-encoded "(caf\xe9)"`)
	}
	if bytes.Contains(raw, []byte("\xc3\xa9")) {
		t.Error("content stream contains raw UTF-8 bytes for é")
	}
}

func TestEncodeText_Unmappable(t *testing.T) {
	// U+1D11E MUSICAL SYMBOL G CLEF has no Windows-1252 mapping.
	if _, err := encodeText("\U0001D11E"); err == nil {
		t.Fatal("expected error for unmappable character, got nil")
	} else if !strings.Contains(err.Error(), "\U0001D11E") {
		t.Errorf("error should name the offending character, got: %v", err)
	}
}

func TestEncodeText_WinAnsiSpecials(t *testing.T) {
	// 0x80–0x9F are not C1 controls in Windows-1252: spot-check the
	// Euro sign and smart quotes against their assigned bytes.
	got, err := encodeText("€\u2019\u201C")
	if err != nil {
		t.Fatalf("encodeText: %v", err)
	}
	want := "\x80\x92\x93"
	if got != want {
		t.Errorf("encodeText(%q) = %q, want %q", "€'\"", got, want)
	}
}

func TestRender_UnknownFontID(t *testing.T) {
	doc := line.Document{Pages: []line.Page{{
		Number: 1, Width: 612, Height: 792,
		Runs: []line.TextRun{{Text: "X", X: 0, Y: 700, FontSize: 10, FontID: "NO-SUCH-FONT"}},
	}}}
	var buf bytes.Buffer
	err := Render(doc, &buf)
	if err == nil {
		t.Fatal("expected loud failure for unmapped font ID, got nil")
	}
	if !strings.Contains(err.Error(), "NO-SUCH-FONT") {
		t.Errorf("error should name the font ID, got: %v", err)
	}
	fmt.Println("unknown font error:", err)
}
