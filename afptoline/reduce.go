// Package afptoline reduces AFP (MO:DCA) print streams to line-data documents.
package afptoline

import (
	"fmt"
	"io"
	"strings"

	"github.com/pratik-saptarshi/go-afp-line/afp"
	"github.com/pratik-saptarshi/go-afp-line/line"
)

// Reduction defaults (v1, per subset-contract §§7–8):
//
//   - defaultFontID: the pinned fixture font for PTOCA runs when no SCFL font
//     switch selected another coded font. Fixtures declare no MCF/SCFL, so
//     every run carries LID 0.
//   - defaultFontSize: point size absent a PTD descriptor. Fixtures carry no
//     AEG/PTD, so the pinned 10pt fixture size applies.
const (
	defaultFontID   = "FIX-MONO-10"
	defaultFontSize = 10.0
)

// ptxTypeID is the Presentation Text Data structured field.
const ptxTypeID = "D3EE9B"

// immTypeID is Invoke Medium Map: the FormDef/PageDef reference point.
const immTypeID = "D3ABCC"

// Reduce walks a MO:DCA stream and produces a line.Document plus loud
// exclusions for every out-of-subset construct encountered.
//
// Structure drive: BDT/EDT delimit the document, BPG/EPG delimit pages.
// Begin/end pairing is validated — a mismatch aborts with afp.DecodeError,
// never a partial document. PTX fields are decoded via DecodePTOCA (Task 6)
// and their runs placed in PDF user space (Y flipped via medium.go).
//
// Contracted-but-untyped structural fields (BNG/ENG, BAG/EAG, BPT/EPT) are
// pairing-validated; their parameters are uninterpreted in v1. MCF/PTD are
// accepted and noted (v1: parameters uninterpreted). Excluded fields become
// line.Exclusion diagnostics via exclusionFor; truly unknown field types abort
// per the decoder's abort-vs-diagnose policy.
func Reduce(r io.Reader) (line.Document, []line.Exclusion, error) {
	raws, err := afp.SplitFields(r)
	if err != nil {
		return line.Document{}, nil, err
	}
	cp, err := afp.LookupCodePage(37)
	if err != nil {
		return line.Document{}, nil, err
	}

	st := &reduceState{cp: cp}
	for _, raw := range raws {
		f, err := afp.DecodeField(raw)
		if err != nil {
			return line.Document{}, st.excls, err
		}
		if err := st.visit(f); err != nil {
			return line.Document{}, st.excls, err
		}
	}
	if err := st.finish(); err != nil {
		return line.Document{}, st.excls, err
	}
	doc := line.Document{Pages: st.pages}
	if err := doc.Validate(); err != nil {
		return line.Document{}, st.excls, err
	}
	return doc, st.excls, nil
}

// reduceState carries the structural walk: document/page/group nesting.
type reduceState struct {
	cp    afp.CodePage
	pages []line.Page
	page  *line.Page // open page under construction, nil when outside a page
	excls []line.Exclusion
	inDoc bool
	bng   int // named page group depth
	bag   int // active environment group depth
	bpt   int // presentation text object depth
}

func (st *reduceState) fail(id string, reason string) error {
	return afp.DecodeError{FieldID: id, Reason: reason}
}

func (st *reduceState) visit(f afp.Field) error {
	switch t := f.(type) {
	case *afp.BDT:
		if st.inDoc {
			return st.fail(t.TypeID(), "nested BDT: document already open")
		}
		st.inDoc = true
		return nil
	case *afp.EDT:
		if !st.inDoc {
			return st.fail(t.TypeID(), "EDT without BDT")
		}
		if st.page != nil {
			return st.fail(t.TypeID(), "EDT with open page: missing EPG")
		}
		if st.bng != 0 || st.bag != 0 || st.bpt != 0 {
			return st.fail(t.TypeID(), "EDT with unclosed group")
		}
		st.inDoc = false
		return nil
	case *afp.BPG:
		if !st.inDoc {
			return st.fail(t.TypeID(), "BPG outside document")
		}
		if st.page != nil {
			return st.fail(t.TypeID(), "nested BPG: page already open")
		}
		w, h := ResolvePageGeometry()
		st.page = &line.Page{Number: len(st.pages) + 1, Width: w, Height: h}
		return nil
	case *afp.EPG:
		if st.page == nil {
			return st.fail(t.TypeID(), "EPG without BPG")
		}
		if st.bag != 0 {
			return st.fail(t.TypeID(), "EPG with open BAG group")
		}
		st.pages = append(st.pages, *st.page)
		st.page = nil
		return nil
	case *afp.RawField:
		return st.visitRaw(t)
	default:
		return st.fail(f.TypeID(), "unhandled field type in reduction walk")
	}
}

func (st *reduceState) visitRaw(t *afp.RawField) error {
	id := t.TypeID()
	if id == ptxTypeID {
		return st.visitPTX(t)
	}
	if t.Excluded {
		st.excls = append(st.excls, exclusionFor(t))
		return nil
	}
	// Contracted in-subset, no v1 content semantics: pairing-validate the
	// group delimiters, accept (and note) the environment fields.
	switch id {
	case "D3A8AD": // BNG
		if !st.inDoc || st.page != nil || st.bng != 0 {
			return st.fail(id, "BNG in illegal position")
		}
		st.bng++
	case "D3A9AD": // ENG
		if st.bng != 1 {
			return st.fail(id, "ENG without BNG")
		}
		st.bng--
	case "D3A8C9": // BAG
		if !st.inDoc || st.page == nil || st.bag != 0 {
			return st.fail(id, "BAG in illegal position")
		}
		st.bag++
	case "D3A9C9": // EAG
		if st.bag != 1 {
			return st.fail(id, "EAG without BAG")
		}
		st.bag--
	case "D3A89B": // BPT
		if st.page == nil || st.bpt != 0 {
			return st.fail(id, "BPT outside page or nested")
		}
		st.bpt++
	case "D3A99B": // EPT
		if st.bpt != 1 {
			return st.fail(id, "EPT without BPT")
		}
		st.bpt--
	case "D3AB8A", // MCF: v1 notes the mapping; fixtures carry none
		"D3B19B": // PTD: v1 notes the descriptor; fixtures carry no AEG
		if !st.inDoc {
			return st.fail(id, "environment field outside document")
		}
	default:
		return st.fail(id, "contracted field without v1 reduction semantics")
	}
	return nil
}

func (st *reduceState) visitPTX(t *afp.RawField) error {
	if st.page == nil {
		return st.fail(t.TypeID(), "PTX outside page")
	}
	runs, err := DecodePTOCA(t, st.cp)
	if err != nil {
		return err
	}
	for _, run := range runs {
		fontID, err := fontIDForLID(run.FontID)
		if err != nil {
			return err
		}
		st.page.Runs = append(st.page.Runs, line.TextRun{
			Text:     run.Text,
			X:        run.X,
			Y:        FlipY(run.Y, st.page.Height),
			FontSize: defaultFontSize,
			FontID:   fontID,
		})
	}
	return nil
}

// fontIDForLID maps a PTOCA SCFL local font ID to a pinned line font ID.
// LID 0 is the default font. Any other LID is a loud error: without an MCF
// mapping we would be guessing metrics, and v1 never guesses.
func fontIDForLID(lid byte) (string, error) {
	if lid == 0 {
		return defaultFontID, nil
	}
	return "", afp.DecodeError{
		Reason: fmt.Sprintf("SCFL local font ID %d has no pinned metrics mapping (MCF unmapped)", lid),
	}
}

// exclusionFor builds the loud diagnostic for a known-excluded field.
// IMM is special-cased: the diagnostic names the referenced medium map so the
// FormDef/PageDef reference is visible, not just the field type.
func exclusionFor(f *afp.RawField) line.Exclusion {
	id := f.TypeID()
	if id == immTypeID {
		name := mediumMapName(f.Data)
		return line.Exclusion{
			FieldType: "IMM",
			Reason:    fmt.Sprintf("references medium map %q: FormDef/PageDef medium definitions are outside the v1 contracted subset", name),
		}
	}
	if acronym, ok := afp.Acronym(id); ok {
		if e, ok := line.ClassifyExclusion(acronym); ok {
			return e
		}
	}
	return line.Exclusion{
		FieldType: id,
		Reason:    "out-of-subset structured field",
	}
}

// mediumMapName decodes the MMPName CHAR[8] at the start of IMM field data
// (EBCDIC-037, blank-padded) for the loud FormDef-reference diagnostic.
func mediumMapName(data []byte) string {
	if len(data) < 8 {
		return ""
	}
	cp, err := afp.LookupCodePage(37)
	if err != nil {
		return ""
	}
	return strings.TrimRight(cp.Decode(data[:8]), " \x00")
}

// finish validates end-of-stream structure: no unclosed document, page, or group.
func (st *reduceState) finish() error {
	if st.page != nil {
		return st.fail("", "end of stream with open page: missing EPG")
	}
	if st.bng != 0 || st.bag != 0 || st.bpt != 0 {
		return st.fail("", "end of stream with unclosed group")
	}
	if st.inDoc {
		return st.fail("", "end of stream with open document: missing EDT")
	}
	return nil
}
