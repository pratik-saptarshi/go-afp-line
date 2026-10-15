// Package line defines the intermediate line-data model: the contract between
// AFP reduction (afptoline) and PDF rendering (linetopdf).
//
// Coordinates are in points in PDF user space: origin at the bottom-left of
// the page, Y increasing upward.
package line

import (
	"fmt"
	"math"
)

// MaxPageDimension bounds page size in points: 0 < Width, Height <= MaxPageDimension.
// 14,400 points (200 inches) is the practical ceiling for PDF page sizes;
// anything larger indicates a malformed or hostile input stream.
const MaxPageDimension = 14400

// MaxCoordinate bounds the absolute value of run coordinates in points:
// |X|, |Y| <= MaxCoordinate. Negatives remain legal (off-page placement).
const MaxCoordinate = 14400

// Document is an ordered sequence of pages produced by reduction.
type Document struct {
	Pages []Page
}

// Validate checks the structural invariants of the document and every page.
// An empty document (zero pages) is valid.
func (d Document) Validate() error {
	for i, p := range d.Pages {
		if err := p.Validate(); err != nil {
			return ValidationError{Where: fmt.Sprintf("document.pages[%d]", i), Reason: err.Error()}
		}
	}
	return nil
}

// Page is a single output page: its size and its positioned text runs.
type Page struct {
	Number int // 1-based page number
	Width  float64
	Height float64 // page size in points; both must be in (0, MaxPageDimension]
	Runs   []TextRun
}

// Validate checks the page dimensions, number, and every text run.
func (p Page) Validate() error {
	if p.Number < 1 {
		return ValidationError{Where: "page", Reason: fmt.Sprintf("number %d < 1", p.Number)}
	}
	if math.IsNaN(p.Width) || math.IsInf(p.Width, 0) || p.Width <= 0 || p.Width > MaxPageDimension {
		return ValidationError{Where: "page", Reason: fmt.Sprintf("width %v outside (0, %v]", p.Width, MaxPageDimension)}
	}
	if math.IsNaN(p.Height) || math.IsInf(p.Height, 0) || p.Height <= 0 || p.Height > MaxPageDimension {
		return ValidationError{Where: "page", Reason: fmt.Sprintf("height %v outside (0, %v]", p.Height, MaxPageDimension)}
	}
	for i, r := range p.Runs {
		if err := r.Validate(); err != nil {
			return ValidationError{Where: fmt.Sprintf("page.runs[%d]", i), Reason: err.Error()}
		}
	}
	return nil
}

// TextRun is a run of decoded text placed at a baseline origin.
type TextRun struct {
	Text     string
	X, Y     float64 // baseline origin in points (PDF user space); negatives legal (off-page)
	FontSize float64 // in points; must be positive
	FontID   string  // key into the font-metrics registry; must be non-empty
}

// Validate checks the text run invariants.
func (r TextRun) Validate() error {
	if !(r.FontSize > 0) || math.IsNaN(r.FontSize) || math.IsInf(r.FontSize, 0) {
		return ValidationError{Where: "run", Reason: fmt.Sprintf("non-positive font size %v", r.FontSize)}
	}
	if math.IsNaN(r.X) || math.IsInf(r.X, 0) || math.Abs(r.X) > MaxCoordinate {
		return ValidationError{Where: "run", Reason: fmt.Sprintf("X %v outside [-%v, %v]", r.X, MaxCoordinate, MaxCoordinate)}
	}
	if math.IsNaN(r.Y) || math.IsInf(r.Y, 0) || math.Abs(r.Y) > MaxCoordinate {
		return ValidationError{Where: "run", Reason: fmt.Sprintf("Y %v outside [-%v, %v]", r.Y, MaxCoordinate, MaxCoordinate)}
	}
	if r.FontID == "" {
		return ValidationError{Where: "run", Reason: "empty font ID"}
	}
	// C1 controls have no WinAnsi encoding; reject per policy, not mapped.
	// The check runs on the decoded Unicode rune, independent of code page:
	// a byte stream's 0x80–0x9F range may be printable (e.g. Windows-1252),
	// but a decoded rune in U+0080–U+009F is always a C1 control.
	for _, c := range r.Text {
		if c >= 0x80 && c <= 0x9F {
			return ValidationError{Where: "run", Reason: fmt.Sprintf("C1 control character U+%04X has no WinAnsi encoding", c)}
		}
	}
	return nil
}

// ValidationError describes a structural violation in the line-data model,
// located by a path such as "document.pages[2].runs[0]".
type ValidationError struct {
	Where  string
	Reason string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("line: invalid %s: %s", e.Where, e.Reason)
}

// FontMetrics holds the glyph advances for one font, in font units.
type FontMetrics struct {
	UnitsPerEm     int
	Advances       map[rune]float64 // advance width per glyph, in font units
	DefaultAdvance float64          // used for glyphs absent from Advances
}

// Width returns the advance width of s in points at the given point size.
func (m FontMetrics) Width(s string, size float64) float64 {
	var total float64
	for _, r := range s {
		adv, ok := m.Advances[r]
		if !ok {
			adv = m.DefaultAdvance
		}
		total += adv
	}
	if m.UnitsPerEm <= 0 {
		return 0
	}
	return total / float64(m.UnitsPerEm) * size
}

// PinnedFonts is the synthetic, pinned metrics table for fixture fonts,
// per the subset contract's font-resolution story (v1). Real AFP font
// resources (FOCA) are excluded; fixtures declare their metrics here.
var PinnedFonts = map[string]FontMetrics{
	// FIX-MONO-10: synthetic monospace, 600/1000 em for every glyph.
	"FIX-MONO-10": {
		UnitsPerEm:     1000,
		Advances:       map[rune]float64{},
		DefaultAdvance: 600,
	},
	// FIX-PROP-10: synthetic proportional with a few declared advances.
	"FIX-PROP-10": {
		UnitsPerEm: 1000,
		Advances: map[rune]float64{
			' ': 250,
			'A': 700,
			'e': 550,
			'H': 720,
			'l': 280,
			'o': 560,
		},
		DefaultAdvance: 500,
	},
}

// LookupFont returns the pinned metrics for a fixture font ID.
func LookupFont(id string) (FontMetrics, bool) {
	m, ok := PinnedFonts[id]
	return m, ok
}
