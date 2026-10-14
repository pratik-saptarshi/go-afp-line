// Writer spike: pdfcpu vs gofpdf positioned-text comparison.
//
// Coordinate mapping (documented):
//
//	PDF user space: origin at BOTTOM-LEFT, y increases upward, units = points.
//	Page size: US Letter = 612 x 792 pt.
//
// Intended placements, expressed as the TEXT-MATRIX ORIGIN (start of the
// text baseline, i.e. where the first glyph's origin sits) in PDF user space:
//
//	"HELLO"  Helvetica 12pt  at (100, 92)    -> 700 pt below the top edge
//	"WORLD"  Courier   10pt  at (300, 292)   -> 500 pt below the top edge
//
// Candidate-specific mapping:
//
//	gofpdf: origin is TOP-LEFT, y grows downward. pdf.Text(x, yTop, s) puts the
//	        baseline at yTop measured from the top. So yTop = 792 - yBaseline.
//	        HELLO -> Text(100, 700); WORLD -> Text(300, 500).
//	pdfcpu: this spike writes the page content stream directly, so the
//	        operators use PDF user space verbatim:
//	          BT /F0 12 Tf 1 0 0 1 100 92 Tm (HELLO) Tj ET
//	        Tm sets the text matrix; its translation (100,92) IS the baseline
//	        origin in PDF user space. Fonts are registered as page resources
//	        via pdfcpu's model/create packages (core font dicts, same mechanism
//	        pdfcpu itself uses for positioned text in watermarks/annotations).
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/jung-kurt/gofpdf"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/create"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

const (
	pageW, pageH = 612.0, 792.0 // US Letter in points

	helloText, helloFont      = "HELLO", "Helvetica"
	helloSize, helloX, helloY = 12.0, 100.0, 92.0 // baseline origin, PDF user space

	worldText, worldFont      = "WORLD", "Courier"
	worldSize, worldX, worldY = 10.0, 300.0, 292.0 // baseline origin, PDF user space
)

func writeGofpdf(out string) error {
	pdf := gofpdf.New("P", "pt", "Letter", "")
	pdf.SetMargins(0, 0, 0)
	pdf.SetAutoPageBreak(false, 0)
	pdf.AddPage()
	pdf.SetFont(helloFont, "", helloSize)
	// gofpdf y is measured from the TOP; baseline lands at yTop.
	pdf.Text(helloX, pageH-helloY, helloText)
	pdf.SetFont(worldFont, "", worldSize)
	pdf.Text(worldX, pageH-worldY, worldText)
	return pdf.OutputFileAndClose(out)
}

func writePdfcpu(out string) error {
	ctx := context.Background()
	conf := model.NewDefaultConfiguration()

	xRefCtx, err := pdfcpu.CreateContextWithXRefTable(conf, types.PaperSize["Letter"])
	if err != nil {
		return fmt.Errorf("create context: %w", err)
	}

	mb := types.RectForDim(pageW, pageH)
	p := model.NewPage(mb, mb)

	// Register core fonts as page resources; EnsureKey assigns /F0, /F1, ...
	fHello := p.Fm.EnsureKey(helloFont)
	fWorld := p.Fm.EnsureKey(worldFont)

	// Content stream in raw PDF user space. Tm's translation IS the baseline
	// origin (PDF spec: text space -> user space via the text matrix).
	fmt.Fprintf(p.Buf, "BT /%s %.1f Tf 1 0 0 1 %.1f %.1f Tm (%s) Tj ET\n",
		fHello, helloSize, helloX, helloY, helloText)
	fmt.Fprintf(p.Buf, "BT /%s %.1f Tf 1 0 0 1 %.1f %.1f Tm (%s) Tj ET\n",
		fWorld, worldSize, worldX, worldY, worldText)

	// Global font map: zero-valued FontResources; create.ensureFontIndRef
	// resolves them into core font dicts via pdffont.EnsureFontDict.
	fontMap := model.FontMap{helloFont: {}, worldFont: {}}

	if _, _, err := create.UpdatePageTree(ctx, xRefCtx, []*model.Page{&p}, fontMap); err != nil {
		return fmt.Errorf("update page tree: %w", err)
	}
	if err := api.CreatePDFFile(ctx, xRefCtx.XRefTable, out, conf); err != nil {
		return fmt.Errorf("write pdf: %w", err)
	}
	return nil
}

func main() {
	dir := os.Args[1]
	if err := writeGofpdf(dir + "/gofpdf.pdf"); err != nil {
		fmt.Fprintln(os.Stderr, "gofpdf:", err)
		os.Exit(1)
	}
	fmt.Println("wrote", dir+"/gofpdf.pdf")
	if err := writePdfcpu(dir + "/pdfcpu.pdf"); err != nil {
		fmt.Fprintln(os.Stderr, "pdfcpu:", err)
		os.Exit(1)
	}
	fmt.Println("wrote", dir+"/pdfcpu.pdf")

	fmt.Printf("INTENDED hello baseline origin: (%.1f, %.1f) pt BL-origin, %s %.0fpt\n", helloX, helloY, helloFont, helloSize)
	fmt.Printf("INTENDED world baseline origin: (%.1f, %.1f) pt BL-origin, %s %.0fpt\n", worldX, worldY, worldFont, worldSize)
}
