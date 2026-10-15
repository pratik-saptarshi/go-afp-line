package afptoline

import (
	"encoding/binary"
	"fmt"
	"math"
	"unicode/utf8"

	"github.com/pratik-saptarshi/go-afp-line/afp"
)

// lUnitsPerPoint converts PTOCA L-units to points. The PTOCA reference assumes
// a measurement unit of 1/1440 inch for coordinate ranges (see AMB/DIR
// definitions); 1440/72 = 20 L-units per point. A PTD-declared unit would
// override this; v1 assumes the reference default. See docs/subset-contract.md §7.
const lUnitsPerPoint = 20.0

// advanceFunc returns the inline advance width in points for text set in
// the given SCFL local font ID. It is unexported: only the reducer supplies
// one (wired to the pinned font registry), keeping the decoder
// font-agnostic. A returned width must be finite and non-negative; flush()
// rejects anything else loudly.
type advanceFunc func(fontLID byte, text string) (float64, error)

// Run is a decoded PTOCA text run: decoded text with its presentation position
// in points, in PTOCA object space (X rightward from the inline origin, Y
// downward from the baseline origin). Task 8 maps this into PDF user space
// (which flips Y using page geometry the state machine does not have).
//
// FontID is the SCFL local font ID active when the run starts (0 = default).
// It is an extension of the plan's Run shape: Task 8 needs it to resolve
// line.TextRun font metrics.
type Run struct {
	Text   string
	X, Y   float64
	FontID byte
}

// ptocaState is the PTOCA presentation state: current position plus the modal
// parameters set by control sequences.
type ptocaState struct {
	ic, bc   float64 // current presentation position, L-units
	imargin  float64 // SIM inline margin, L-units
	incr     float64 // SBI baseline increment, L-units
	sia      int16   // SIA intercharacter adjustment, L-units; folded into flush() advancement
	svi      uint16  // SVI variable-space character increment (tracked)
	fontID   byte    // SCFL local font identifier
	color    []byte  // STC raw color specification (tracked)
	iornt    uint16  // STO I-axis orientation
	bornt    uint16  // STO B-axis orientation
	suppress []byte  // BSU LID stack; non-empty means text is suppressed

	cp  afp.CodePage
	cur []byte // pending text bytes for the run being accumulated
	// adv, when non-nil, supplies the inline advance width in points for
	// emitted text; flush() advances st.ic by it (plus SIA). Nil disables
	// advancement (DecodePTOCA's legacy behavior). Unexported: only the
	// reducer supplies one, so the decoder stays font-agnostic.
	adv advanceFunc
	// remaining caps additional text() bytes; negative means unlimited.
	// It is the fail-fast security boundary against amplification (RPS):
	// the budget is checked before any segment is appended or any repeat
	// buffer allocated, so a hostile field cannot force a large transient
	// allocation.
	remaining int
	// curX, curY, curFont record where the pending run started (points, PTOCA
	// object space). Set by markStart on the first byte of a run.
	curX, curY float64
	curFont    byte

	runs []Run
}

// runPos returns the current position in points.
func (st *ptocaState) pos() (x, y float64) {
	return st.ic / lUnitsPerPoint, st.bc / lUnitsPerPoint
}

// budgetExceededReason is the DecodeError Reason when decoded text exceeds
// the configured budget. The reducer matches budget errors on
// DecodeError.Kind (afp.KindBudget) to attach the offending field's identity
// and stream offset.
const budgetExceededReason = "decoded text budget exceeded"

// text accumulates graphic-character bytes into the pending run. Bytes inside
// a suppression region are dropped (never presented, never budgeted, nothing
// allocated). Otherwise the fail-fast budget is enforced before appending:
// a segment that alone exceeds the remaining budget is rejected without
// touching the pending run.
func (st *ptocaState) text(b []byte) error {
	if len(st.suppress) > 0 {
		return nil
	}
	if st.remaining >= 0 && len(b) > st.remaining {
		return afp.DecodeError{Kind: afp.KindBudget, Reason: budgetExceededReason}
	}
	st.cur = append(st.cur, b...)
	st.remaining -= len(b)
	return nil
}

// flush ends the pending run, keeping the start position recorded by
// markStart. Moves, font changes, and suppression boundaries flush so each
// run has exactly one position and font. When st.adv is non-nil, flush also
// advances the inline position past the emitted text: the width comes from
// st.adv (points, converted to L-units) plus the SIA intercharacter
// adjustment per intercharacter gap. Absolute moves after a flush override
// the advanced position; that is the PTOCA-correct behavior.
func (st *ptocaState) flush() error {
	if len(st.cur) == 0 {
		return nil
	}
	text := st.cp.Decode(st.cur)
	st.runs = append(st.runs, Run{
		Text:   text,
		X:      st.curX,
		Y:      st.curY,
		FontID: st.curFont,
	})
	st.cur = nil
	if st.adv == nil {
		return nil
	}
	w, err := st.adv(st.curFont, text)
	if err != nil {
		return err
	}
	if math.IsNaN(w) || math.IsInf(w, 0) || w < 0 {
		return afp.DecodeError{
			Kind:   afp.KindValidation,
			Reason: fmt.Sprintf("advance width %v out of domain (want finite, non-negative)", w),
		}
	}
	// SIA applies per intercharacter gap: N runes → N-1 gaps, none for a
	// single character. sia is int16 L-units, signed; negative tightens.
	// v1 limitation: gaps adjacent to spaces count too. Per the PTOCA
	// reference, SIA inserts only between characters forming a word, not
	// between words — a run like "hello world" over-advances by 2×sia per
	// space. Impact is nil at sia=0 (the initialized value); inword gating
	// is future work. A mid-run SIA change likewise applies the final value
	// to all gaps of the merged run (SIA does not split runs).
	gaps := max(utf8.RuneCountInString(text)-1, 0)
	st.ic += w*lUnitsPerPoint + float64(st.sia)*float64(gaps)
	return nil
}

// curX, curY, curFont record where the pending run started.
func (st *ptocaState) markStart() {
	if len(st.cur) == 0 {
		st.curX, st.curY = st.pos()
		st.curFont = st.fontID
	}
}

// DecodePTOCA decodes the PTOCA data of a PTX structured field into text runs.
//
// f must be a *afp.RawField with TypeID D3EE9B (PTX arrives untyped from the
// registry; its Data is the field payload after the introducer). cp decodes
// text bytes. Unknown control sequences, truncated sequences, and unmatched
// ESU fail loudly with afp.DecodeError; nothing is silently skipped.
//
// v1 limitations (deliberate): SVI variable-space increments are tracked but
// not applied to advancement (see flush); DIR/DBR rules and STC color are
// parsed but not emitted as runs.
func DecodePTOCA(f afp.Field, cp afp.CodePage) ([]Run, error) {
	return decodePTOCA(f, cp, -1, nil)
}

// decodePTOCA is DecodePTOCA with an explicit cap on decoded text bytes and
// an optional inline-advance function. maxBytes < 0 means unlimited;
// otherwise the fail-fast budget in ptocaState.remaining aborts the decode
// before any segment or repeat buffer can exceed it. adv == nil disables
// advancement (DecodePTOCA's behavior); the reducer passes a metrics-backed
// closure so runs after text start at the advanced inline position.
func decodePTOCA(f afp.Field, cp afp.CodePage, maxBytes int, adv advanceFunc) ([]Run, error) {
	rf, ok := f.(*afp.RawField)
	if !ok {
		return nil, afp.DecodeError{Kind: afp.KindValidation, Reason: fmt.Sprintf("DecodePTOCA needs *afp.RawField, got %T", f)}
	}
	if id := rf.Intro.TypeIDString(); id != "D3EE9B" {
		return nil, afp.DecodeError{FieldID: id, Kind: afp.KindValidation, Reason: "DecodePTOCA needs a PTX field (D3EE9B)"}
	}
	st := &ptocaState{cp: cp, remaining: maxBytes, adv: adv}
	data := rf.Data
	pos := 0
	for pos < len(data) {
		if data[pos] == 0x2B && pos+1 < len(data) && data[pos+1] == 0xD3 {
			var chained bool
			var err error
			pos, chained, err = st.execUnchained(data, pos)
			if err != nil {
				return nil, err
			}
			for chained {
				if pos >= len(data) {
					break // chain terminated by end of object
				}
				pos, chained, err = st.execChained(data, pos)
				if err != nil {
					return nil, err
				}
			}
			continue
		}
		// Raw graphic-character bytes until the next 2B D3 escape.
		end := pos
		for end < len(data) {
			if data[end] == 0x2B && end+1 < len(data) && data[end+1] == 0xD3 {
				break
			}
			end++
		}
		st.markStart()
		if err := st.text(data[pos:end]); err != nil {
			return nil, err
		}
		pos = end
	}
	if err := st.flush(); err != nil {
		return nil, err
	}
	return st.runs, nil
}

// execUnchained parses one unchained control sequence at data[pos] (2B D3
// prefix). It returns the new position and whether the sequence was chained.
func (st *ptocaState) execUnchained(data []byte, pos int) (int, bool, error) {
	// data[pos]==0x2B && data[pos+1]==0xD3 established by caller.
	if pos+3 >= len(data) {
		return 0, false, afp.DecodeError{FieldID: "D3EE9B", Offset: pos, Kind: afp.KindStructure, Reason: "truncated control sequence introducer"}
	}
	length := int(data[pos+2])
	if length < 2 {
		return 0, false, afp.DecodeError{FieldID: "D3EE9B", Offset: pos, Kind: afp.KindStructure, Reason: fmt.Sprintf("invalid control sequence length %d", length)}
	}
	if pos+2+length > len(data) {
		return 0, false, afp.DecodeError{FieldID: "D3EE9B", Offset: pos, Kind: afp.KindStructure, Reason: "control sequence extends past PTX data"}
	}
	chained, err := st.exec(data[pos+3], data[pos+4:pos+2+length], pos)
	if err != nil {
		return 0, false, err
	}
	return pos + 2 + length, chained, nil
}

// execChained parses one chained control sequence at data[pos] (Length, Type
// only — no 2B D3 prefix).
func (st *ptocaState) execChained(data []byte, pos int) (int, bool, error) {
	if pos+1 >= len(data) {
		return 0, false, afp.DecodeError{FieldID: "D3EE9B", Offset: pos, Kind: afp.KindStructure, Reason: "truncated chained control sequence introducer"}
	}
	length := int(data[pos])
	if length < 2 {
		return 0, false, afp.DecodeError{FieldID: "D3EE9B", Offset: pos, Kind: afp.KindStructure, Reason: fmt.Sprintf("invalid chained control sequence length %d", length)}
	}
	if pos+length > len(data) {
		return 0, false, afp.DecodeError{FieldID: "D3EE9B", Offset: pos, Kind: afp.KindStructure, Reason: "chained control sequence extends past PTX data"}
	}
	chained, err := st.exec(data[pos+1], data[pos+2:pos+length], pos)
	if err != nil {
		return 0, false, err
	}
	return pos + length, chained, nil
}

// exec dispatches one control sequence by function type. The chaining bit
// (bit 0) is stripped before table lookup; an odd type means another chained
// sequence follows.
func (st *ptocaState) exec(ftype byte, params []byte, offset int) (bool, error) {
	chained := ftype&0x01 == 0x01
	base := ftype & 0xFE
	h, ok := ptocaHandlers[base]
	if !ok {
		return false, afp.DecodeError{
			FieldID: "D3EE9B",
			Offset:  offset,
			Kind:    afp.KindValidation,
			Reason:  fmt.Sprintf("unknown PTOCA control sequence 0x%02X", base),
		}
	}
	if err := h(st, params, offset); err != nil {
		return false, err
	}
	return chained, nil
}

// handler validates params and applies one control sequence.
type handler func(st *ptocaState, params []byte, offset int) error

// ptocaHandlers is the table-driven dispatch for the 19 in-scope control
// sequences (subset-contract.md §5). Codes are unchained bases.
var ptocaHandlers = map[byte]handler{
	0xD2: doMove(func(st *ptocaState, v int16) { st.bc = float64(v) }),      // AMB absolute baseline
	0xC6: doMove(func(st *ptocaState, v int16) { st.ic = float64(v) }),      // AMI absolute inline
	0xD4: doMove(func(st *ptocaState, v int16) { st.bc += float64(v) }),     // RMB relative baseline
	0xC8: doMove(func(st *ptocaState, v int16) { st.ic += float64(v) }),     // RMI relative inline
	0xD0: doU16(func(st *ptocaState, v uint16) { st.incr = float64(v) }),    // SBI baseline increment
	0xC0: doU16(func(st *ptocaState, v uint16) { st.imargin = float64(v) }), // SIM inline margin
	0xC2: doI16(func(st *ptocaState, v int16) { st.sia = v }),               // SIA intercharacter adjustment
	0xC4: doU16(func(st *ptocaState, v uint16) { st.svi = v }),              // SVI variable-space increment
	0x74: doSTC,
	0xF2: doBSU,
	0xF4: doESU,
	0xF0: doSCFL,
	0xF6: doSTO,
	0xDA: doTRN,
	0xEE: doRPS,
	0xE4: doRule, // DIR
	0xE6: doRule, // DBR
	0xD8: doBLN,
	0xF8: doNOP,
}

// doMove builds a handler for 2-byte signed-displacement move sequences.
// Moves flush the pending run: the run keeps its start position.
func doMove(apply func(st *ptocaState, v int16)) handler {
	return func(st *ptocaState, params []byte, offset int) error {
		if len(params) != 2 {
			return seqLenError(offset, 2, len(params))
		}
		if err := st.flush(); err != nil {
			return err
		}
		apply(st, int16(binary.BigEndian.Uint16(params)))
		return nil
	}
}

// doU16 builds a handler for 2-byte unsigned modal parameters.
func doU16(set func(st *ptocaState, v uint16)) handler {
	return func(st *ptocaState, params []byte, offset int) error {
		if len(params) != 2 {
			return seqLenError(offset, 2, len(params))
		}
		set(st, binary.BigEndian.Uint16(params))
		return nil
	}
}

// doI16 builds a handler for 2-byte signed modal parameters.
func doI16(set func(st *ptocaState, v int16)) handler {
	return func(st *ptocaState, params []byte, offset int) error {
		if len(params) != 2 {
			return seqLenError(offset, 2, len(params))
		}
		set(st, int16(binary.BigEndian.Uint16(params)))
		return nil
	}
}

func seqLenError(offset, want, got int) error {
	return afp.DecodeError{
		FieldID: "D3EE9B",
		Offset:  offset,
		Kind:    afp.KindStructure,
		Reason:  fmt.Sprintf("control sequence wants %d param bytes, got %d", want, got),
	}
}

// doSTC records the raw color specification (variable length).
func doSTC(st *ptocaState, params []byte, _ int) error {
	st.color = append(st.color[:0], params...)
	return nil
}

// doBSU pushes the suppression LID. Text between BSU and the matching ESU is
// not presented, but control sequences still execute (handled by gating only
// text accumulation, not state updates).
func doBSU(st *ptocaState, params []byte, offset int) error {
	if len(params) != 1 {
		return seqLenError(offset, 1, len(params))
	}
	if err := st.flush(); err != nil {
		return err
	}
	st.suppress = append(st.suppress, params[0])
	return nil
}

// doESU pops the matching suppression LID. An ESU with no matching open BSU
// is a loud error, not a silent skip.
func doESU(st *ptocaState, params []byte, offset int) error {
	if len(params) != 1 {
		return seqLenError(offset, 1, len(params))
	}
	lid := params[0]
	for i := len(st.suppress) - 1; i >= 0; i-- {
		if st.suppress[i] == lid {
			st.suppress = append(st.suppress[:i], st.suppress[i+1:]...)
			return nil
		}
	}
	return afp.DecodeError{
		FieldID: "D3EE9B",
		Offset:  offset,
		Kind:    afp.KindStructure,
		Reason:  fmt.Sprintf("ESU with no matching BSU (LID 0x%02X)", lid),
	}
}

// doSCFL switches the active coded font by local ID. Font changes split runs.
func doSCFL(st *ptocaState, params []byte, offset int) error {
	if len(params) != 1 {
		return seqLenError(offset, 1, len(params))
	}
	if err := st.flush(); err != nil {
		return err
	}
	st.fontID = params[0]
	return nil
}

// doSTO records I/B axis orientation (2+2 bytes).
func doSTO(st *ptocaState, params []byte, offset int) error {
	if len(params) != 4 {
		return seqLenError(offset, 4, len(params))
	}
	st.iornt = binary.BigEndian.Uint16(params[0:2])
	st.bornt = binary.BigEndian.Uint16(params[2:4])
	return nil
}

// doTRN appends literal text bytes. No scan for embedded escapes: the bytes
// were already delimited by Length.
func doTRN(st *ptocaState, params []byte, _ int) error {
	st.markStart()
	return st.text(params)
}

// doRPS processes Repeat String: RLENGTH(2B) then RPTDATA bytes. The RPTDATA
// bytes are repeated until RLENGTH bytes of graphic characters are presented;
// when RLENGTH equals the RPTDATA length this is identical to Transparent
// Data (per the PTOCA reference). Inline advances past the repeated text on
// flush, like any other run.
func doRPS(st *ptocaState, params []byte, offset int) error {
	if len(params) < 2 {
		return seqLenError(offset, 2, len(params))
	}
	n := int(binary.BigEndian.Uint16(params[0:2]))
	data := params[2:]
	if n == 0 {
		return nil
	}
	if len(data) == 0 {
		return afp.DecodeError{
			FieldID: "D3EE9B",
			Offset:  offset,
			Kind:    afp.KindStructure,
			Reason:  fmt.Sprintf("RPS RLENGTH %d exceeds available %d bytes", n, len(data)),
		}
	}
	// Suppressed bytes are dropped without consuming budget (see text()), so
	// a suppressed RPS neither allocates the repeat buffer nor checks the
	// budget against it.
	if len(st.suppress) > 0 {
		return nil
	}
	// Fail fast before allocating the repeat buffer: an RLENGTH that alone
	// exceeds the remaining budget can never be satisfied, so reject it
	// without the up-to-64 KiB transient.
	if st.remaining >= 0 && n > st.remaining {
		return afp.DecodeError{Kind: afp.KindBudget, Reason: budgetExceededReason}
	}
	st.markStart()
	// RLENGTH is a uint16, so the repeat buffer is bounded at 64 KiB, and
	// the pre-check above guarantees it fits the remaining budget.
	repeated := make([]byte, 0, n)
	for len(repeated) < n {
		repeated = append(repeated, data...)
	}
	return st.text(repeated[:n])
}

// doRule validates DIR/DBR (length 4: RLENGTH only; length 7: + 3-byte
// RWIDTH). Rules do not move the presentation position.
func doRule(st *ptocaState, params []byte, offset int) error {
	if len(params) != 2 && len(params) != 5 {
		return afp.DecodeError{
			FieldID: "D3EE9B",
			Offset:  offset,
			Kind:    afp.KindStructure,
			Reason:  fmt.Sprintf("rule sequence wants 2 or 5 param bytes, got %d", len(params)),
		}
	}
	return nil
}

// doBLN begins a new line: inline returns to the margin, baseline advances by
// the SBI increment. Bcnew = Bc + INCRMENT; Icnew = Imargin.
func doBLN(st *ptocaState, params []byte, offset int) error {
	if len(params) != 0 {
		return seqLenError(offset, 0, len(params))
	}
	if err := st.flush(); err != nil {
		return err
	}
	st.ic = st.imargin
	st.bc += st.incr
	return nil
}

// doNOP ignores the sequence (any length).
func doNOP(_ *ptocaState, _ []byte, _ int) error { return nil }
