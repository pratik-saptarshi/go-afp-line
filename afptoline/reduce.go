// Package afptoline reduces AFP (MO:DCA) print streams to line-data documents.
package afptoline

import (
	"errors"
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

// DefaultMaxTextBytes bounds the total decoded text of one reduction, so a
// control sequence that amplifies (e.g. RPS: one 8-byte sequence can expand
// to 65535 bytes) cannot exhaust memory on a hostile stream.
const DefaultMaxTextBytes = 1 << 20

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
//
// The total decoded text is bounded by DefaultMaxTextBytes (see
// ReduceWithBudget): an RPS-heavy stream fails with afp.DecodeError rather
// than expanding without limit.
func Reduce(r io.Reader) (line.Document, []line.Exclusion, error) {
	return ReduceWithBudget(r, DefaultMaxTextBytes)
}

// ReduceWithBudget is Reduce with an explicit bound on the total decoded
// text bytes of the document. A non-positive budget is rejected immediately,
// before any input is read. The decode uses code page 37, exactly as Reduce.
func ReduceWithBudget(r io.Reader, maxTextBytes int) (line.Document, []line.Exclusion, error) {
	cp, err := afp.LookupCodePage(37)
	if err != nil {
		return line.Document{}, nil, err
	}
	return reduceWithBudgetAndCodePage(r, maxTextBytes, cp)
}

// ReduceWithCodePage is Reduce with the EBCDIC code page used for PTOCA
// text decode supplied explicitly by the caller, exposing a page the
// default path cannot reach (ARCH-3: code page 500). Selecting the *correct*
// code page from stream metadata (MCF/FQN) is out of scope — this only
// exposes the knob; the caller names the page. The default behavior is
// unchanged: reducing with the code page from afp.LookupCodePage(37)
// behaves exactly like Reduce.
func ReduceWithCodePage(r io.Reader, cp afp.CodePage) (line.Document, []line.Exclusion, error) {
	return reduceWithBudgetAndCodePage(r, DefaultMaxTextBytes, cp)
}

// reduceWithBudgetAndCodePage is the shared implementation behind Reduce,
// ReduceWithBudget and ReduceWithCodePage.
func reduceWithBudgetAndCodePage(r io.Reader, maxTextBytes int, cp afp.CodePage) (line.Document, []line.Exclusion, error) {
	if maxTextBytes <= 0 {
		return line.Document{}, nil, fmt.Errorf("afptoline: non-positive text budget %d", maxTextBytes)
	}
	st := &reduceState{cp: cp, textLeft: maxTextBytes}
	// Track the wire offset just past the last visited record, for
	// finish() errors ("end of stream with ... open"): the old code
	// advanced st.off past each field including the 5A record marker. The
	// per-record marker (1 on wrapped streams, 0 on naked) is the gap
	// between consecutive field offsets — streams are homogeneous, so one
	// gap suffices. With fewer than two fields no gap is observable and 0
	// is assumed, which is exact except for a single-field wrapped stream
	// (its leading marker is invisible in the offsets).
	prevDataEnd, marker := 0, 0
	seen := false
	for sf, err := range afp.Fields(r) {
		if err != nil {
			return line.Document{}, nil, err
		}
		if seen {
			marker = sf.Offset - prevDataEnd
		}
		// st.off is the true wire offset of the field under visit, so
		// errors raised during visit carry the field's start offset,
		// matching DecodeError.Offset's documented meaning ("where
		// decoding failed"). Each Fields entry is the complete encoded
		// field (starting at its introducer); sf.Offset already counts
		// the stripped 5A record marker on wrapped streams.
		st.off = sf.Offset
		f, err := afp.DecodeField(sf.Data)
		if err != nil {
			return line.Document{}, st.excls, err
		}
		if err := st.visit(f); err != nil {
			return line.Document{}, st.excls, err
		}
		prevDataEnd = sf.Offset + len(sf.Data)
		seen = true
	}
	// After the loop st.off is the end-of-stream wire offset, so finish()
	// errors ("end of stream with ... open") report where the stream
	// ended, exactly as the old accumulate-and-add-one logic did.
	st.off = prevDataEnd + marker
	if err := st.finish(); err != nil {
		return line.Document{}, st.excls, err
	}
	doc := line.Document{Pages: st.pages}
	// Coordinate bounds are a reducer-level validation: the document is
	// well-formed but a run violates the contracted coordinate range, so
	// surface it as a typed DecodeError (KindValidation, matching the
	// sibling validation failures in visitPTX) rather than the untyped
	// line.ValidationError.
	if err := doc.Validate(); err != nil {
		var verr line.ValidationError
		if errors.As(err, &verr) {
			return line.Document{}, st.excls, afp.DecodeError{Kind: afp.KindValidation, Reason: verr.Error()}
		}
		return line.Document{}, st.excls, err
	}
	return doc, st.excls, nil
}

// reduceState carries the structural walk: document/page/group nesting.
type reduceState struct {
	cp       afp.CodePage
	pages    []line.Page
	page     *line.Page // open page under construction, nil when outside a page
	excls    []line.Exclusion
	inDoc    bool
	groups   []groupKind // open groups, innermost last; rejects misnesting
	textLeft int         // decoded text bytes still within budget
	off      int         // cumulative stream offset through the field currently being visited
}

// groupKind identifies a group delimiter on the pairing stack. BPG rides
// the stack alongside BNG/BAG/BPT so an end field always closes the
// innermost open group — flat counters accepted cross-closing (e.g. ENG
// while a page opened inside the named group was still open).
type groupKind int

const (
	grpBPG groupKind = iota
	grpBNG
	grpBAG
	grpBPT
)

// openGroup pushes a group delimiter opened by the field under visit.
func (st *reduceState) openGroup(g groupKind) {
	st.groups = append(st.groups, g)
}

// groupTop reports the innermost open group; ok is false when none is open.
func (st *reduceState) groupTop() (top groupKind, ok bool) {
	if len(st.groups) == 0 {
		return 0, false
	}
	return st.groups[len(st.groups)-1], true
}

// hasOpenGroup reports whether a group of kind g is open anywhere on the
// stack. Opens are non-nesting per kind in v1 (BNG/BAG/BPT each reject a
// second open), so this is the "already open" check for begin fields.
func (st *reduceState) hasOpenGroup(g groupKind) bool {
	for _, open := range st.groups {
		if open == g {
			return true
		}
	}
	return false
}

// closeGroup pops the innermost open group. Callers verify the top matches
// the expected kind before popping, so misnesting fails loudly instead of
// cross-closing.
func (st *reduceState) closeGroup() {
	st.groups = st.groups[:len(st.groups)-1]
}

// fail builds the reducer's DecodeError. The kind classifies the failure;
// the true stream offset (st.off, advanced past all preceding fields, so
// during a visit it is the current field's start offset) is attached here
// so no call site can forget it.
func (st *reduceState) fail(id string, kind afp.ErrorKind, reason string) error {
	return afp.DecodeError{FieldID: id, Offset: st.off, Kind: kind, Reason: reason}
}

func (st *reduceState) visit(f afp.Field) error {
	switch t := f.(type) {
	case *afp.BDT:
		if st.inDoc {
			return st.fail(t.TypeID(), afp.KindStructure, "nested BDT: document already open")
		}
		st.inDoc = true
		return nil
	case *afp.EDT:
		if !st.inDoc {
			return st.fail(t.TypeID(), afp.KindStructure, "EDT without BDT")
		}
		if st.page != nil {
			return st.fail(t.TypeID(), afp.KindStructure, "EDT with open page: missing EPG")
		}
		if len(st.groups) != 0 {
			return st.fail(t.TypeID(), afp.KindStructure, "EDT with unclosed group")
		}
		st.inDoc = false
		return nil
	case *afp.BPG:
		if !st.inDoc {
			return st.fail(t.TypeID(), afp.KindStructure, "BPG outside document")
		}
		if st.page != nil {
			return st.fail(t.TypeID(), afp.KindStructure, "nested BPG: page already open")
		}
		w, h := resolvePageGeometry()
		st.page = &line.Page{Number: len(st.pages) + 1, Width: w, Height: h}
		st.openGroup(grpBPG)
		return nil
	case *afp.EPG:
		if st.page == nil {
			return st.fail(t.TypeID(), afp.KindStructure, "EPG without BPG")
		}
		// Pairing: EPG closes the page, so the innermost open group must
		// be the page itself — anything else is a group crossing EPG.
		// (page != nil implies grpBPG is on the stack: BPG pushes on open
		// and only EPG pops, so the empty-stack arm is unreachable, but
		// it fails loudly rather than panicking on closeGroup.)
		top, ok := st.groupTop()
		switch {
		case ok && top == grpBAG:
			return st.fail(t.TypeID(), afp.KindStructure, "EPG with open BAG group")
		case ok && top == grpBPT:
			return st.fail(t.TypeID(), afp.KindStructure, "EPG with open BPT group")
		case ok && top == grpBPG:
			// expected: pop below
		default:
			return st.fail(t.TypeID(), afp.KindStructure, "EPG with misnested group")
		}
		st.closeGroup() // grpBPG
		st.pages = append(st.pages, *st.page)
		st.page = nil
		return nil
	case *afp.RawField:
		return st.visitRaw(t)
	default:
		return st.fail(f.TypeID(), afp.KindValidation, "unhandled field type in reduction walk")
	}
}

func (st *reduceState) visitRaw(t *afp.RawField) error {
	id := t.TypeID()
	if id == ptxTypeID {
		return st.visitPTX(t)
	}
	if t.Excluded {
		st.excls = append(st.excls, exclusionFor(t, st.cp))
		return nil
	}
	// Contracted in-subset, no v1 content semantics: pairing-validate the
	// group delimiters, accept (and note) the environment fields.
	switch id {
	case "D3A8AD": // BNG
		if !st.inDoc || st.page != nil || st.hasOpenGroup(grpBNG) {
			return st.fail(id, afp.KindStructure, "BNG in illegal position")
		}
		st.openGroup(grpBNG)
	case "D3A9AD": // ENG
		// Pairing: ENG closes the named group, so it must be innermost.
		// A page opened inside the group (top == grpBPG) is the ARCH3-1
		// misnesting: ENG would strand the open page.
		top, ok := st.groupTop()
		switch {
		case ok && top == grpBPG:
			return st.fail(id, afp.KindStructure, "ENG with open page: missing EPG")
		case ok && top == grpBNG:
			// expected: pop below
		default:
			return st.fail(id, afp.KindStructure, "ENG without BNG")
		}
		st.closeGroup() // grpBNG
	case "D3A8C9": // BAG
		if !st.inDoc || st.page == nil || st.hasOpenGroup(grpBAG) {
			return st.fail(id, afp.KindStructure, "BAG in illegal position")
		}
		st.openGroup(grpBAG)
	case "D3A9C9": // EAG
		// Pairing: EAG closes the environment group, so it must be
		// innermost — a BPT opened inside it is still open.
		top, ok := st.groupTop()
		switch {
		case ok && top == grpBPT:
			return st.fail(id, afp.KindStructure, "EAG with open BPT group")
		case ok && top == grpBAG:
			// expected: pop below
		default:
			return st.fail(id, afp.KindStructure, "EAG without BAG")
		}
		st.closeGroup() // grpBAG
	case "D3A89B": // BPT
		if st.page == nil || st.hasOpenGroup(grpBPT) {
			return st.fail(id, afp.KindStructure, "BPT outside page or nested")
		}
		st.openGroup(grpBPT)
	case "D3A99B": // EPT
		// Pairing: EPT closes the text object, so it must be innermost —
		// a BAG opened inside it is still open.
		top, ok := st.groupTop()
		switch {
		case ok && top == grpBAG:
			return st.fail(id, afp.KindStructure, "EPT with open BAG group")
		case ok && top == grpBPT:
			// expected: pop below
		default:
			return st.fail(id, afp.KindStructure, "EPT without BPT")
		}
		st.closeGroup() // grpBPT
	case "D3AB8A", // MCF: v1 notes the mapping; fixtures carry none
		"D3B19B": // PTD: v1 notes the descriptor; fixtures carry no AEG
		if !st.inDoc {
			return st.fail(id, afp.KindStructure, "environment field outside document")
		}
	default:
		return st.fail(id, afp.KindValidation, "contracted field without v1 reduction semantics")
	}
	return nil
}

func (st *reduceState) visitPTX(t *afp.RawField) error {
	if st.page == nil {
		return st.fail(t.TypeID(), afp.KindStructure, "PTX outside page")
	}
	runs, err := decodePTOCA(t, st.cp, st.textLeft, st.advanceFunc())
	if err != nil {
		var derr afp.DecodeError
		if errors.As(err, &derr) {
			if derr.FieldID == "" || derr.Kind == afp.KindBudget {
				// Budget and advance errors carry no decoder-local offset
				// to preserve (zero, or st.off already via fontIDForLID);
				// attribute them to the offending PTX.
				return afp.DecodeError{FieldID: t.TypeID(), Offset: st.off, Kind: derr.Kind, Reason: derr.Reason}
			}
			// The decoder's own errors (FieldID "D3EE9B") carry
			// decoder-local offsets relative to the PTX payload; rebase
			// them to stream-relative. (Threading the introducer length
			// into the rebase is CODEX-F01, deferred.)
			derr.Offset += st.off
			return derr
		}
		return err
	}
	// Reject C1 controls per policy: they have no WinAnsi encoding, so they
	// must never reach linetopdf. The check runs on the decoded Unicode rune,
	// independent of code page, immediately after decode — before budget
	// accounting or appending to the page.
	for _, run := range runs {
		for _, c := range run.Text {
			if c >= 0x80 && c <= 0x9F {
				return afp.DecodeError{
					FieldID: t.TypeID(),
					Offset:  st.off,
					Kind:    afp.KindEncoding,
					Reason:  fmt.Sprintf("C1 control character U+%04X has no WinAnsi encoding; rejected per policy, not mapped", c),
				}
			}
		}
	}
	// Bound amplification: control sequences such as RPS can expand a small
	// payload into many bytes. The check runs against the remaining document
	// budget per PTX, so an attacker cannot bypass it with many small fields.
	var n int
	for _, run := range runs {
		n += len(run.Text)
	}
	if n > st.textLeft {
		return afp.DecodeError{FieldID: t.TypeID(), Offset: st.off, Kind: afp.KindBudget, Reason: budgetExceededReason}
	}
	st.textLeft -= n
	for _, run := range runs {
		fontID, err := fontIDForLID(run.FontID, st.off)
		if err != nil {
			return err
		}
		st.page.Runs = append(st.page.Runs, line.TextRun{
			Text:     run.Text,
			X:        run.X,
			Y:        flipY(run.Y, st.page.Height),
			FontSize: defaultFontSize,
			FontID:   fontID,
		})
	}
	return nil
}

// advanceFunc returns the reducer's inline-advance closure for the PTOCA
// decoder: SCFL local ID → pinned font metrics → width in points at the
// default size. Unmapped fonts and unknown registry IDs fail loudly;
// v1 never guesses metrics.
func (st *reduceState) advanceFunc() advanceFunc {
	return func(lid byte, text string) (float64, error) {
		id, err := fontIDForLID(lid, st.off)
		if err != nil {
			return 0, err
		}
		m, ok := line.LookupFont(id)
		if !ok {
			return 0, afp.DecodeError{
				Kind:   afp.KindValidation,
				Reason: fmt.Sprintf("pinned font %q has no metrics", id),
			}
		}
		return m.Width(text, defaultFontSize), nil
	}
}

// fontIDForLID maps a PTOCA SCFL local font ID to a pinned line font ID.
// LID 0 is the default font. Any other LID is a loud error: without an MCF
// mapping we would be guessing metrics, and v1 never guesses.
func fontIDForLID(lid byte, offset int) (string, error) {
	if lid == 0 {
		return defaultFontID, nil
	}
	return "", afp.DecodeError{
		Offset: offset,
		Kind:   afp.KindValidation,
		Reason: fmt.Sprintf("SCFL local font ID %d has no pinned metrics mapping (MCF unmapped)", lid),
	}
}

// exclusionFor builds the loud diagnostic for a known-excluded field.
// IMM is special-cased: the diagnostic names the referenced medium map so the
// FormDef/PageDef reference is visible, not just the field type.
func exclusionFor(f *afp.RawField, cp afp.CodePage) line.Exclusion {
	id := f.TypeID()
	if id == immTypeID {
		name := mediumMapName(f.Data, cp)
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
// (blank-padded) with the caller's code page for the loud FormDef-reference
// diagnostic.
func mediumMapName(data []byte, cp afp.CodePage) string {
	if len(data) < 8 {
		return ""
	}
	return strings.TrimRight(cp.Decode(data[:8]), " \x00")
}

// finish validates end-of-stream structure: no unclosed document, page, or group.
func (st *reduceState) finish() error {
	if st.page != nil {
		return st.fail("", afp.KindStructure, "end of stream with open page: missing EPG")
	}
	if len(st.groups) != 0 {
		return st.fail("", afp.KindStructure, "end of stream with unclosed group")
	}
	if st.inDoc {
		return st.fail("", afp.KindStructure, "end of stream with open document: missing EDT")
	}
	return nil
}
