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

// winAnsiSpecial maps the Unicode code points assigned to Windows-1252 bytes
// 0x80–0x9F. Bytes 0x00–0x7F and 0xA0–0xFF are the identity mapping (ASCII and
// U+00A0–U+00FF); bytes 0x81, 0x8D, 0x8F, 0x90 and 0x9D are undefined in
// Windows-1252 and therefore absent here.
var winAnsiSpecial = map[rune]byte{
	'\u20AC': 0x80, // €
	'\u201A': 0x82, // ‚
	'\u0192': 0x83, // ƒ
	'\u201E': 0x84, // „
	'\u2026': 0x85, // …
	'\u2020': 0x86, // †
	'\u2021': 0x87, // ‡
	'\u02C6': 0x88, // ˆ
	'\u2030': 0x89, // ‰
	'\u0160': 0x8A, // Š
	'\u2039': 0x8B, // ‹
	'\u0152': 0x8C, // Œ
	'\u017D': 0x8E, // Ž
	'\u2018': 0x91, // '
	'\u2019': 0x92, // '
	'\u201C': 0x93, // "
	'\u201D': 0x94, // "
	'\u2022': 0x95, // •
	'\u2013': 0x96, // –
	'\u2014': 0x97, // —
	'\u02DC': 0x98, // ˜
	'\u2122': 0x99, // ™
	'\u0161': 0x9A, // š
	'\u203A': 0x9B, // ›
	'\u0153': 0x9C, // œ
	'\u017E': 0x9E, // ž
	'\u0178': 0x9F, // Ÿ
}

// winAnsiByte maps r to its single Windows-1252 byte, reporting whether such
// a mapping exists. Note the C1 range U+0080–U+009F has no mapping: those byte
// values belong to the printable characters in winAnsiSpecial.
func winAnsiByte(r rune) (byte, bool) {
	switch {
	case r < 0x80:
		return byte(r), true
	case r >= 0xA0 && r <= 0xFF:
		return byte(r), true
	default:
		b, ok := winAnsiSpecial[r]
		return b, ok
	}
}

// encodeText encodes s to single-byte WinAnsi (Windows-1252) for use as a PDF
// literal string, escaping PDF delimiters and control bytes. Standard-14
// fonts (Courier here) use WinAnsi encoding, so UTF-8 bytes must not pass
// through raw — "café" would otherwise read as "cafÃ©". Characters with no
// WinAnsi mapping fail loudly; there is no silent substitution.
func encodeText(s string) (string, error) {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		wb, ok := winAnsiByte(r)
		if !ok {
			return "", fmt.Errorf("character %q (U+%04X) has no WinAnsi mapping", r, r)
		}
		switch {
		case wb == '\\' || wb == '(' || wb == ')':
			b.WriteByte('\\')
			b.WriteByte(wb)
		case wb < 0x20 || wb == 0x7F:
			fmt.Fprintf(&b, "\\%03o", wb)
		default:
			b.WriteByte(wb)
		}
	}
	return b.String(), nil
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
			text, err := encodeText(r.Text)
			if err != nil {
				return fmt.Errorf("linetopdf: page %d run %q: %w", pg.Number, r.Text, err)
			}
			fmt.Fprintf(p.Buf, "BT /%s %.3f Tf 1 0 0 1 %.3f %.3f Tm (%s) Tj ET\n",
				fKey, r.FontSize, r.X, r.Y, text)
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
