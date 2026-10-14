// Package linetopdf renders a line.Document to untagged PDF.
//
// Writer: pdfcpu v0.16.2 (Task 0 spike verdict). Text runs are placed with raw
// Tm/Tj content-stream operators referencing standard-14 fonts; the fixture
// font FIX-MONO-10 maps to Courier, whose 600/1000em monospace advances are
// identical to the declared metrics by construction, so the mapping is
// metrically exact.
//
// Font-embedding spike (Task 9 follow-up, 2026-10-13): pdfcpu can embed user
// TTFs — verified mechanically (DejaVuSansMono embedded as a CID TrueType
// subset, emb=yes per pdffonts). However the path requires pdfcpu's global
// font-install machinery (font.UserFontDir + generated .gob metrics) and a
// real TTF's true metrics, which would contradict the declared synthetic
// fixture metrics. v1 therefore does not depend on it; revisit when real AFP
// font resources with real metrics arrive.
package linetopdf

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/create"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"github.com/pratik-saptarshi/go-afp-line/line"
)

// producer labels the output and carries the required disclaimer: this output
// is untagged and therefore not accessible.
const producer = "go-afp-line linetopdf (untagged; not accessible)"

// pdfFontFor maps a fixture font ID to a PDF standard-14 font name. The
// mapping is only valid when the PDF font's real advances are identical to
// the declared metrics; otherwise Render fails loudly instead of silently
// misplacing text.
func pdfFontFor(fontID string) (string, error) {
	switch fontID {
	case "FIX-MONO-10":
		// Courier is 600/1000em monospace by definition: metrically exact.
		return "Courier", nil
	default:
		return "", fmt.Errorf("linetopdf: no PDF font mapping for font ID %q: declared metrics have no metrically-exact standard-14 counterpart", fontID)
	}
}

// escapeText escapes s for use as a PDF literal string.
func escapeText(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "(", `\(`)
	s = strings.ReplaceAll(s, ")", `\)`)
	return s
}

// Render writes doc as untagged PDF to w: one page per line.Page at its
// declared Width/Height, with each text run placed at its baseline origin
// via the text matrix. Coordinates are PDF user space (bottom-left origin,
// points), matching the line model.
func Render(doc line.Document, w io.Writer) error {
	if err := doc.Validate(); err != nil {
		return fmt.Errorf("linetopdf: %w", err)
	}
	ctx := context.Background()
	conf := model.NewDefaultConfiguration()

	xRefCtx, err := pdfcpu.CreateContextWithXRefTable(conf, types.PaperSize["Letter"])
	if err != nil {
		return fmt.Errorf("linetopdf: create context: %w", err)
	}

	fontMap := model.FontMap{}
	pages := make([]*model.Page, 0, len(doc.Pages))
	for _, pg := range doc.Pages {
		mb := types.RectForDim(pg.Width, pg.Height)
		p := model.NewPage(mb, mb)
		for _, r := range pg.Runs {
			fontName, err := pdfFontFor(r.FontID)
			if err != nil {
				return err
			}
			fKey := p.Fm.EnsureKey(fontName)
			if _, ok := fontMap[fontName]; !ok {
				fontMap[fontName] = model.FontResource{}
			}
			fmt.Fprintf(p.Buf, "BT /%s %.3f Tf 1 0 0 1 %.3f %.3f Tm (%s) Tj ET\n",
				fKey, r.FontSize, r.X, r.Y, escapeText(r.Text))
		}
		pages = append(pages, &p)
	}

	if _, _, err := create.UpdatePageTree(ctx, xRefCtx, pages, fontMap); err != nil {
		return fmt.Errorf("linetopdf: build page tree: %w", err)
	}

	pdfCtx := xRefCtx
	// Label the output as untagged / not accessible, and preserve the info
	// dict so pdfcpu does not overwrite Producer with its own version string.
	infoDict := types.NewDict()
	infoDict.InsertString("Producer", producer)
	ir, err := pdfCtx.IndRefForNewObject(infoDict)
	if err != nil {
		return fmt.Errorf("linetopdf: info dict: %w", err)
	}
	pdfCtx.Info = ir
	pdfCtx.PreserveInfoDict = true

	if err := api.WriteContext(ctx, pdfCtx, w); err != nil {
		return fmt.Errorf("linetopdf: write: %w", err)
	}
	return nil
}
