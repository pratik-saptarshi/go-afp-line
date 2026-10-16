package afptoline

import (
	"errors"
	"strings"
	"testing"

	"github.com/pratik-saptarshi/go-afp-line/afp"
)

// cp037 for test expectations.
func testCP(t *testing.T) afp.CodePage {
	t.Helper()
	cp, err := afp.LookupCodePage(37)
	if err != nil {
		t.Fatalf("LookupCodePage(37): %v", err)
	}
	return cp
}

// ptxField wraps a PTX payload in a RawField as produced by afp.DecodeField.
func ptxField(payload []byte) afp.Field {
	return &afp.RawField{
		Intro: afp.Introducer{
			Length: 8 + len(payload),
			TypeID: [3]byte{0xD3, 0xEE, 0x9B},
		},
		Data: payload,
	}
}

// seq builds an unchained control sequence: 2B D3 <len> <type> <params...>.
// length counts itself and the type byte.
func seq(ftype byte, params ...byte) []byte {
	out := []byte{0x2B, 0xD3, byte(2 + len(params)), ftype}
	return append(out, params...)
}

// seq16 builds an unchained control sequence with a 2-byte big-endian param.
func seq16(ftype byte, v uint16) []byte {
	return seq(ftype, byte(v>>8), byte(v))
}

// trn wraps EBCDIC bytes in a Transparent Data sequence.
func trn(data ...byte) []byte { return seq(0xDA, data...) }

// decode is a test helper: decode payload, fail on unexpected error.
func decode(t *testing.T, payload []byte) []Run {
	t.Helper()
	runs, err := DecodePTOCA(ptxField(payload), testCP(t))
	if err != nil {
		t.Fatalf("DecodePTOCA: %v", err)
	}
	return runs
}

func approx(t *testing.T, what string, got, want float64) {
	t.Helper()
	if d := got - want; d < -1.0 || d > 1.0 {
		t.Errorf("%s = %v, want %v (±1pt)", what, got, want)
	}
}

func TestDecodePTOCA_AMB(t *testing.T) {
	// AMB 1440 L-units = 72pt down the baseline axis.
	runs := decode(t, append(seq(0xD2, 0x05, 0xA0), trn(0xC1)...))
	if len(runs) != 1 {
		t.Fatalf("got %d runs, want 1", len(runs))
	}
	if runs[0].Text != "A" {
		t.Errorf("text = %q, want %q", runs[0].Text, "A")
	}
	approx(t, "X", runs[0].X, 0)
	approx(t, "Y", runs[0].Y, 72)
}

func TestDecodePTOCA_AMI(t *testing.T) {
	// AMI 2880 L-units = 144pt along the inline axis.
	runs := decode(t, append(seq(0xC6, 0x0B, 0x40), trn(0xC1)...))
	if len(runs) != 1 {
		t.Fatalf("got %d runs, want 1", len(runs))
	}
	approx(t, "X", runs[0].X, 144)
	approx(t, "Y", runs[0].Y, 0)
}

func TestDecodePTOCA_RMB(t *testing.T) {
	p := append(seq(0xD2, 0x05, 0xA0), seq(0xD4, 0x02, 0xD0)...) // AMB 1440, RMB +720
	p = append(p, trn(0xC1)...)
	runs := decode(t, p)
	if len(runs) != 1 {
		t.Fatalf("got %d runs, want 1", len(runs))
	}
	approx(t, "Y", runs[0].Y, 108) // (1440+720)/20
}

func TestDecodePTOCA_RMB_Negative(t *testing.T) {
	p := append(seq(0xD2, 0x05, 0xA0), seq(0xD4, 0xFD, 0x30)...) // AMB 1440, RMB -720
	p = append(p, trn(0xC1)...)
	runs := decode(t, p)
	approx(t, "Y", runs[0].Y, 36) // (1440-720)/20
}

func TestDecodePTOCA_RMI(t *testing.T) {
	p := append(seq(0xC6, 0x05, 0xA0), seq(0xC8, 0x02, 0xD0)...) // AMI 1440, RMI +720
	p = append(p, trn(0xC1)...)
	runs := decode(t, p)
	approx(t, "X", runs[0].X, 108)
}

func TestDecodePTOCA_SIA_Parses(t *testing.T) {
	// Intercharacter adjustment is tracked, not applied (no font metrics in
	// v1 — Task 8 applies advances with the pinned metrics table).
	p := append(seq(0xC2, 0xFF, 0xF6), trn(0xC1, 0xC2)...) // SIA -10
	runs := decode(t, p)
	if len(runs) != 1 || runs[0].Text != "AB" {
		t.Fatalf("runs = %+v, want one run %q", runs, "AB")
	}
	approx(t, "X", runs[0].X, 0)
}

func TestDecodePTOCA_SCFL_FontSwitch(t *testing.T) {
	p := append(trn(0xC1), seq(0xF0, 0x05)...) // "A", SCFL LID 5
	p = append(p, trn(0xC2)...)                // "B"
	runs := decode(t, p)
	if len(runs) != 2 {
		t.Fatalf("got %d runs, want 2 (font change splits runs)", len(runs))
	}
	if runs[0].FontID != 0 || runs[0].Text != "A" {
		t.Errorf("run0 = %+v, want {A, font 0}", runs[0])
	}
	if runs[1].FontID != 5 || runs[1].Text != "B" {
		t.Errorf("run1 = %+v, want {B, font 5}", runs[1])
	}
}

func TestDecodePTOCA_BSU_ESU_Suppresses(t *testing.T) {
	p := append(trn(0xC1), seq(0xF2, 0x01)...) // "A", BSU LID 1
	p = append(p, seq(0xC6, 0x0B, 0x40)...)    // AMI 1440 *inside* suppression
	p = append(p, trn(0xC2)...)                // "B" suppressed
	p = append(p, seq(0xF4, 0x01)...)          // ESU LID 1
	p = append(p, trn(0xC3)...)                // "C"
	runs := decode(t, p)
	if len(runs) != 2 {
		t.Fatalf("got %d runs, want 2 (A and C; B suppressed)", len(runs))
	}
	if runs[0].Text != "A" || runs[1].Text != "C" {
		t.Errorf("texts = %q, %q; want A, C", runs[0].Text, runs[1].Text)
	}
	// Control sequences inside suppression still execute: AMI moved to 144pt.
	approx(t, "C.X", runs[1].X, 144)
}

func TestDecodePTOCA_BSU_Unterminated(t *testing.T) {
	// Suppression runs to end of object: only "A" is emitted.
	p := append(trn(0xC1), seq(0xF2, 0x01)...)
	p = append(p, trn(0xC2)...)
	runs := decode(t, p)
	if len(runs) != 1 || runs[0].Text != "A" {
		t.Fatalf("runs = %+v, want single run A", runs)
	}
}

func TestDecodePTOCA_ESU_Unmatched(t *testing.T) {
	_, err := DecodePTOCA(ptxField(seq(0xF4, 0x02)), testCP(t))
	if err == nil {
		t.Fatal("expected error for unmatched ESU, got nil")
	}
	var derr afp.DecodeError
	if !errors.As(err, &derr) {
		t.Fatalf("error type = %T, want afp.DecodeError", err)
	}
	if derr.Kind != afp.KindStructure {
		t.Errorf("Kind = %q, want %q", derr.Kind, afp.KindStructure)
	}
	if derr.FieldID != "D3EE9B" {
		t.Errorf("FieldID = %q, want D3EE9B", derr.FieldID)
	}
	if !strings.Contains(derr.Reason, "ESU with no matching BSU (LID 0x02)") {
		t.Errorf("Reason = %q, want ESU/LID naming", derr.Reason)
	}
}

func TestDecodePTOCA_UnknownSequence(t *testing.T) {
	_, err := DecodePTOCA(ptxField(seq(0x98, 0x00, 0x00)), testCP(t))
	if err == nil {
		t.Fatal("expected error for unknown control sequence, got nil")
	}
	var derr afp.DecodeError
	if !errors.As(err, &derr) {
		t.Fatalf("error type = %T, want afp.DecodeError", err)
	}
	if !strings.Contains(derr.Reason, "98") {
		t.Errorf("reason %q does not name the sequence code", derr.Reason)
	}
}

func TestDecodePTOCA_Chained(t *testing.T) {
	// 2B D3 | 04 D3(AMB chained) 05A0 | 04 C7(AMI chained) 0B40 | 03 DB(TRN chained) C1
	p := []byte{0x2B, 0xD3, 0x04, 0xD3, 0x05, 0xA0, 0x04, 0xC7, 0x0B, 0x40, 0x03, 0xDB, 0xC1}
	runs := decode(t, p)
	if len(runs) != 1 {
		t.Fatalf("got %d runs, want 1", len(runs))
	}
	if runs[0].Text != "A" {
		t.Errorf("text = %q, want A", runs[0].Text)
	}
	approx(t, "X", runs[0].X, 144) // AMI 2880
	approx(t, "Y", runs[0].Y, 72)  // AMB 1440
}

func TestDecodePTOCA_RawText(t *testing.T) {
	// "Hello" in EBCDIC 037 with no control sequences at all.
	runs := decode(t, []byte{0xC8, 0x85, 0x93, 0x93, 0x96})
	if len(runs) != 1 {
		t.Fatalf("got %d runs, want 1", len(runs))
	}
	if runs[0].Text != "Hello" {
		t.Errorf("text = %q, want Hello", runs[0].Text)
	}
	approx(t, "X", runs[0].X, 0)
	approx(t, "Y", runs[0].Y, 0)
}

func TestDecodePTOCA_RawTextSplitByMove(t *testing.T) {
	// "AB", AMI 2880, "C" -> two runs at the two positions.
	p := append([]byte{0xC1, 0xC2}, seq(0xC6, 0x0B, 0x40)...)
	p = append(p, 0xC3)
	runs := decode(t, p)
	if len(runs) != 2 {
		t.Fatalf("got %d runs, want 2", len(runs))
	}
	if runs[0].Text != "AB" || runs[1].Text != "C" {
		t.Errorf("texts = %q, %q", runs[0].Text, runs[1].Text)
	}
	approx(t, "run1.X", runs[1].X, 144)
}

func TestDecodePTOCA_TRN_NoScan(t *testing.T) {
	// TRN data containing 2B D3 must NOT be parsed as a control sequence.
	p := []byte{0x2B, 0xD3, 0x05, 0xDA, 0x2B, 0xD3, 0x41}
	runs := decode(t, p)
	if len(runs) != 1 {
		t.Fatalf("got %d runs, want 1 (TRN data is opaque)", len(runs))
	}
	want := testCP(t).Decode([]byte{0x2B, 0xD3, 0x41})
	if runs[0].Text != want {
		t.Errorf("text = %q, want %q", runs[0].Text, want)
	}
}

func TestDecodePTOCA_RPS(t *testing.T) {
	// RPS: RLENGTH(2B) = 3, RPTDATA = "ABC".
	p := []byte{0x2B, 0xD3, 0x07, 0xEE, 0x00, 0x03, 0xC1, 0xC2, 0xC3}
	runs := decode(t, p)
	if len(runs) != 1 || runs[0].Text != "ABC" {
		t.Fatalf("runs = %+v, want single run ABC", runs)
	}
}

func TestDecodePTOCA_RPS_Repeat(t *testing.T) {
	// RPS repeats RPTDATA until RLENGTH bytes are presented (spec: repeat
	// until the presented byte count equals the specified number of bytes).
	cases := []struct {
		name    string
		rlength uint16
		data    []byte
		want    string
	}{
		{"one byte repeated to five", 5, []byte{0xC1}, "AAAAA"},
		{"two bytes repeated to four", 4, []byte{0xC1, 0xC2}, "ABAB"},
		{"three bytes repeated to seven", 7, []byte{0xC1, 0xC2, 0xC3}, "ABCABCA"},
		{"rlength equal to data length acts as TRN", 3, []byte{0xC1, 0xC2, 0xC3}, "ABC"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			params := append([]byte{byte(tc.rlength >> 8), byte(tc.rlength)}, tc.data...)
			runs := decode(t, seq(0xEE, params...))
			if len(runs) != 1 || runs[0].Text != tc.want {
				t.Fatalf("runs = %+v, want single run %q", runs, tc.want)
			}
		})
	}
}

func TestDecodePTOCA_RPS_ZeroLengthNoOp(t *testing.T) {
	// RLENGTH=0 emits nothing and is not an error.
	runs := decode(t, seq(0xEE, 0x00, 0x00))
	if len(runs) != 0 {
		t.Fatalf("got %d runs, want 0 (RLENGTH=0 is a no-op)", len(runs))
	}
}

func TestDecodePTOCA_RPS_EmptyDataErrors(t *testing.T) {
	// Empty RPTDATA with positive RLENGTH cannot be repeated: fail loudly.
	p := seq(0xEE, 0x00, 0x05)
	_, err := DecodePTOCA(ptxField(p), testCP(t))
	var derr afp.DecodeError
	if !errors.As(err, &derr) {
		t.Fatalf("err = %v (%T), want afp.DecodeError", err, err)
	}
}

func TestDecodePTOCA_DIR_DBR_NoPositionChange(t *testing.T) {
	// DIR with width (len 7) and DBR without (len 4): neither moves position.
	p := append(seq(0xE4, 0x05, 0xA0, 0x00, 0x10, 0x80), seq(0xE6, 0x05, 0xA0)...)
	p = append(p, trn(0xC1)...)
	runs := decode(t, p)
	if len(runs) != 1 {
		t.Fatalf("got %d runs, want 1", len(runs))
	}
	approx(t, "X", runs[0].X, 0)
	approx(t, "Y", runs[0].Y, 0)
}

func TestDecodePTOCA_BLN(t *testing.T) {
	// SBI 1440, SIM 720, BLN -> X=margin 36pt, Y=baseline+incr 72pt.
	p := append(seq16(0xD0, 1440), seq16(0xC0, 720)...)
	p = append(p, seq(0xD8)...)
	p = append(p, trn(0xC1)...)
	runs := decode(t, p)
	if len(runs) != 1 {
		t.Fatalf("got %d runs, want 1", len(runs))
	}
	approx(t, "X", runs[0].X, 36)
	approx(t, "Y", runs[0].Y, 72)
}

func TestDecodePTOCA_STC_STO_NOP_SVI(t *testing.T) {
	p := append(seq(0x74, 0x01, 0x02, 0x03), seq(0xF6, 0x00, 0x00, 0x23, 0xF6)...)
	p = append(p, seq(0xF8, 0xAA, 0xBB)...)
	p = append(p, seq16(0xC4, 10)...)
	p = append(p, trn(0xC1)...)
	runs := decode(t, p)
	if len(runs) != 1 || runs[0].Text != "A" {
		t.Fatalf("runs = %+v, want single run A", runs)
	}
	approx(t, "X", runs[0].X, 0)
	approx(t, "Y", runs[0].Y, 0)
}

func TestDecodePTOCA_Truncated(t *testing.T) {
	cases := []struct {
		name       string
		payload    []byte
		wantKind   afp.ErrorKind
		wantReason string
	}{
		{"cut sequence", []byte{0x2B, 0xD3, 0x05, 0xD2, 0x05}, afp.KindStructure, "control sequence extends past PTX data"},
		{"lone escape", []byte{0x2B, 0xD3}, afp.KindStructure, "truncated control sequence introducer"},
		{"cut chained", []byte{0x2B, 0xD3, 0x03, 0xD3, 0x05}, afp.KindStructure, "control sequence wants 2 param bytes, got 1"},
		{"length overrun", []byte{0x2B, 0xD3, 0xFF, 0xD2, 0x00}, afp.KindStructure, "control sequence extends past PTX data"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodePTOCA(ptxField(tc.payload), testCP(t))
			if err == nil {
				t.Fatalf("%s: expected error, got nil", tc.name)
			}
			var derr afp.DecodeError
			if !errors.As(err, &derr) {
				t.Fatalf("%s: error type = %T, want afp.DecodeError", tc.name, err)
			}
			if derr.Kind != tc.wantKind {
				t.Errorf("%s: Kind = %q, want %q", tc.name, derr.Kind, tc.wantKind)
			}
			if derr.Reason != tc.wantReason {
				t.Errorf("%s: Reason = %q, want %q", tc.name, derr.Reason, tc.wantReason)
			}
		})
	}
}

func TestDecodePTOCA_NonPTX(t *testing.T) {
	bdt := &afp.RawField{
		Intro: afp.Introducer{Length: 16, TypeID: [3]byte{0xD3, 0xA8, 0xA8}},
		Data:  make([]byte, 8),
	}
	_, err := DecodePTOCA(bdt, testCP(t))
	if err == nil {
		t.Error("expected error for non-PTX field, got nil")
	} else {
		var derr afp.DecodeError
		if !errors.As(err, &derr) {
			t.Fatalf("error type = %T, want afp.DecodeError", err)
		}
		if derr.Kind != afp.KindValidation {
			t.Errorf("Kind = %q, want %q", derr.Kind, afp.KindValidation)
		}
		if derr.FieldID != "D3A8A8" {
			t.Errorf("FieldID = %q, want D3A8A8", derr.FieldID)
		}
		if derr.Reason != "DecodePTOCA needs a PTX field (D3EE9B)" {
			t.Errorf("Reason = %q", derr.Reason)
		}
	}
	_, err = DecodePTOCA(&afp.BDT{}, testCP(t))
	if err == nil {
		t.Error("expected error for non-RawField, got nil")
	} else {
		var derr afp.DecodeError
		if !errors.As(err, &derr) {
			t.Fatalf("error type = %T, want afp.DecodeError", err)
		}
		if derr.Kind != afp.KindValidation {
			t.Errorf("Kind = %q, want %q", derr.Kind, afp.KindValidation)
		}
		if !strings.Contains(derr.Reason, "DecodePTOCA needs *afp.RawField") {
			t.Errorf("Reason = %q", derr.Reason)
		}
	}
}

func TestDecodePTOCA_Empty(t *testing.T) {
	runs := decode(t, nil)
	if len(runs) != 0 {
		t.Errorf("got %d runs, want 0", len(runs))
	}
}

// TestDecodePTOCA_AllSequencesParse checks every in-scope control sequence
// from the subset contract §5 parses without error at its canonical length.
func TestDecodePTOCA_AllSequencesParse(t *testing.T) {
	vectors := map[string][]byte{
		"AMB": seq(0xD2, 0x05, 0xA0), "AMI": seq(0xC6, 0x05, 0xA0),
		"RMB": seq(0xD4, 0x05, 0xA0), "RMI": seq(0xC8, 0x05, 0xA0),
		"SBI": seq(0xD0, 0x05, 0xA0), "SIM": seq(0xC0, 0x05, 0xA0),
		"SIA": seq(0xC2, 0x05, 0xA0), "SVI": seq(0xC4, 0x05, 0xA0),
		"STC": seq(0x74, 0x01, 0x02), "BSU": seq(0xF2, 0x01),
		"ESU":  append(seq(0xF2, 0x01), seq(0xF4, 0x01)...), // BSU then matched ESU (unmatched ESU errors)
		"SCFL": seq(0xF0, 0x05), "STO": seq(0xF6, 0x00, 0x00, 0x23, 0xF6),
		"TRN": trn(0xC1), "RPS": {0x2B, 0xD3, 0x07, 0xEE, 0x00, 0x03, 0xC1, 0xC2, 0xC3},
		"DIR": seq(0xE4, 0x05, 0xA0, 0x00, 0x10, 0x80), "DBR": seq(0xE6, 0x05, 0xA0),
		"BLN": seq(0xD8), "NOP": seq(0xF8, 0xAA),
	}
	for name, p := range vectors {
		if _, err := DecodePTOCA(ptxField(p), testCP(t)); err != nil {
			t.Errorf("%s: unexpected error: %v", name, err)
		}
	}
}

// TestPtocaStateText_BudgetExceeded: text() enforces the fail-fast budget
// before appending. An 11-byte segment against remaining=10 returns the
// budget error and leaves the pending run untouched (nothing allocated).
func TestPtocaStateText_BudgetExceeded(t *testing.T) {
	st := &ptocaState{remaining: 10}
	err := st.text([]byte("0123456789A"))
	if err == nil {
		t.Fatal("text accepted 11 bytes with remaining=10")
	}
	var derr afp.DecodeError
	if !errors.As(err, &derr) || derr.Reason != budgetExceededReason {
		t.Fatalf("error = %v (%T), want afp.DecodeError with budget reason", err, err)
	}
	if derr.Kind != afp.KindBudget {
		t.Fatalf("Kind = %q, want %q", derr.Kind, afp.KindBudget)
	}
	if len(st.cur) != 0 {
		t.Fatalf("st.cur grew to %d bytes despite the budget error", len(st.cur))
	}
	if st.remaining != 10 {
		t.Fatalf("remaining = %d, want 10 (unchanged on rejection)", st.remaining)
	}
}

// TestPtocaStateText_SuppressedBypassesBudget: bytes inside a suppression
// region are dropped without consuming budget.
func TestPtocaStateText_SuppressedBypassesBudget(t *testing.T) {
	st := &ptocaState{remaining: 0, suppress: []byte{0x01}}
	if err := st.text([]byte("0123456789A")); err != nil {
		t.Fatalf("suppressed text hit the budget: %v", err)
	}
	if len(st.cur) != 0 {
		t.Fatalf("st.cur grew to %d bytes inside a suppression region", len(st.cur))
	}
}

// TestDoRPS_SuppressedBypassesBudget: an RPS inside a suppression region
// neither allocates the repeat buffer nor checks the budget — the bytes are
// dropped, mirroring TestPtocaStateText_SuppressedBypassesBudget.
func TestDoRPS_SuppressedBypassesBudget(t *testing.T) {
	// RPS params: RLENGTH = 65535, RPTDATA = "A" — would exceed any budget.
	params := []byte{0xFF, 0xFF, 0xC1}
	st := &ptocaState{remaining: 0, suppress: []byte{0x01}}
	if err := doRPS(st, params, 0); err != nil {
		t.Fatalf("suppressed RPS hit the budget: %v", err)
	}
	if len(st.cur) != 0 {
		t.Fatalf("st.cur grew to %d bytes inside a suppression region", len(st.cur))
	}
}

// TestDoRPS_BudgetPrecheck: doRPS fails fast before allocating the repeat
// buffer when RLENGTH alone exceeds the remaining budget. RLENGTH=65535
// against remaining=4096 must error with no large cur growth. The pre-check
// itself is pinned by allocation count: without it, each call allocates the
// 64 KiB repeat buffer (1 alloc/call); with it, zero.
func TestDoRPS_BudgetPrecheck(t *testing.T) {
	// RPS params: RLENGTH = 65535, RPTDATA = "A" (EBCDIC 0xC1).
	params := []byte{0xFF, 0xFF, 0xC1}
	st := &ptocaState{remaining: 4096}
	err := doRPS(st, params, 0)
	if err == nil {
		t.Fatal("doRPS accepted RLENGTH=65535 with remaining=4096")
	}
	var derr afp.DecodeError
	if !errors.As(err, &derr) || derr.Reason != budgetExceededReason {
		t.Fatalf("error = %v (%T), want afp.DecodeError with budget reason", err, err)
	}
	if derr.Kind != afp.KindBudget {
		t.Fatalf("Kind = %q, want %q", derr.Kind, afp.KindBudget)
	}
	if len(st.cur) > 4096 {
		t.Fatalf("st.cur grew to %d bytes: the repeat buffer was allocated before the budget check", len(st.cur))
	}
	if allocs := testing.AllocsPerRun(50, func() {
		s := &ptocaState{remaining: 4096}
		_ = doRPS(s, params, 0)
	}); allocs >= 1 {
		t.Fatalf("doRPS averaged %.1f allocations per call: the 64 KiB repeat buffer was allocated before the budget check", allocs)
	}
}

// TestDecodePTOCA_ErrorPaths: every error return in the PTOCA decoder,
// pinned with the expected Kind, offset, and naming. Decoder errors already
// covered elsewhere (and cited here rather than duplicated):
//   - non-RawField / non-PTX field: TestDecodePTOCA_NonPTX
//   - truncated unchained introducer and length overrun: TestDecodePTOCA_Truncated
//   - unmatched ESU: TestDecodePTOCA_ESU_Unmatched
//   - RPS/text fail-fast budget: TestDoRPS_BudgetPrecheck, TestPtocaStateText_BudgetExceeded
func TestDecodePTOCA_ErrorPaths(t *testing.T) {
	chained := func(tail ...byte) []byte {
		// 2B D3, len 3, ftype 0xF3 (BSU, chained bit set), LID 0x05: a valid
		// chained opener, then the chained-sequence bytes under test.
		return append(seq(0xF3, 0x05), tail...)
	}
	cases := []struct {
		name       string
		payload    []byte
		wantKind   afp.ErrorKind
		wantOffset int
		wantReason string // substring
	}{
		{"invalid unchained length", []byte{0x2B, 0xD3, 0x01, 0xD2, 0x05, 0xA0}, afp.KindStructure, 0, "invalid control sequence length 1"},
		{"truncated chained introducer", chained(0x05), afp.KindStructure, 5, "truncated chained control sequence introducer"},
		{"invalid chained length", chained(0x01, 0x00), afp.KindStructure, 5, "invalid chained control sequence length 1"},
		{"chained extends past data", chained(0xFF, 0x00, 0x00), afp.KindStructure, 5, "chained control sequence extends past PTX data"},
		{"unknown sequence", seq(0xE0, 0x01), afp.KindValidation, 0, "unknown PTOCA control sequence 0xE0"},
		{"AMB wrong param length", seq(0xD2, 0x05), afp.KindStructure, 0, "control sequence wants 2 param bytes, got 1"},
		{"SBI wrong param length", seq(0xD0, 0x05), afp.KindStructure, 0, "control sequence wants 2 param bytes, got 1"},
		{"SIA wrong param length", seq(0xC2, 0x05), afp.KindStructure, 0, "control sequence wants 2 param bytes, got 1"},
		{"BSU wrong param length", seq(0xF2, 0x01, 0x02), afp.KindStructure, 0, "control sequence wants 1 param bytes, got 2"},
		{"ESU wrong param length", seq(0xF4, 0x01, 0x02), afp.KindStructure, 0, "control sequence wants 1 param bytes, got 2"},
		{"SCFL wrong param length", seq(0xF0, 0x01, 0x02), afp.KindStructure, 0, "control sequence wants 1 param bytes, got 2"},
		{"STO wrong param length", seq(0xF6, 0x00, 0x00), afp.KindStructure, 0, "control sequence wants 4 param bytes, got 2"},
		{"BLN wrong param length", seq(0xD8, 0x00), afp.KindStructure, 0, "control sequence wants 0 param bytes, got 1"},
		{"RPS missing RLENGTH", seq(0xEE), afp.KindStructure, 0, "control sequence wants 2 param bytes, got 0"},
		{"RPS empty RPTDATA", seq(0xEE, 0x00, 0x05), afp.KindStructure, 0, "RPS RLENGTH 5 exceeds available 0 bytes"},
		{"DIR wrong param length", seq(0xE4, 0x05), afp.KindStructure, 0, "rule sequence wants 2 or 5 param bytes, got 1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodePTOCA(ptxField(tc.payload), testCP(t))
			if err == nil {
				t.Fatalf("DecodePTOCA accepted %q", tc.payload)
			}
			var derr afp.DecodeError
			if !errors.As(err, &derr) {
				t.Fatalf("error %T (%v), want afp.DecodeError", err, err)
			}
			if derr.FieldID != "D3EE9B" {
				t.Errorf("FieldID = %q, want D3EE9B", derr.FieldID)
			}
			if derr.Kind != tc.wantKind {
				t.Errorf("Kind = %q, want %q", derr.Kind, tc.wantKind)
			}
			if derr.Offset != tc.wantOffset {
				t.Errorf("Offset = %d, want %d", derr.Offset, tc.wantOffset)
			}
			if !strings.Contains(derr.Reason, tc.wantReason) {
				t.Errorf("Reason = %q, want it to contain %q", derr.Reason, tc.wantReason)
			}
		})
	}
}

// TestDecodePTOCA_RPSBudgetIsCumulative (RB3): the in-decoder text budget
// must be cumulative across control sequences, not per-sequence. Three RPS
// sequences each with RLENGTH == maxBytes pass doRPS's per-sequence
// pre-check, but together they exceed the budget; the second sequence must
// fail fast inside the decoder with KindBudget. This guards the
// st.remaining decrement in text(): with it mutated away, the decode
// succeeds (3000 bytes against a 1000-byte budget) and only a later,
// post-allocation check could reject it.
func TestDecodePTOCA_RPSBudgetIsCumulative(t *testing.T) {
	const budget = 1000
	var payload []byte
	for i := 0; i < 3; i++ {
		// RPS: RLENGTH(2B) = 1000, RPTDATA = "A" (EBCDIC 0xC1).
		payload = append(payload, seq(0xEE, 0x03, 0xE8, 0xC1)...)
	}
	_, err := decodePTOCA(ptxField(payload), testCP(t), budget, nil)
	if err == nil {
		t.Fatal("decodePTOCA accepted 3x1000 RPS bytes against a 1000-byte budget")
	}
	var derr afp.DecodeError
	if !errors.As(err, &derr) {
		t.Fatalf("error %T (%v), want afp.DecodeError", err, err)
	}
	if derr.Kind != afp.KindBudget {
		t.Errorf("Kind = %q, want %q (decoder fail-fast, not a later check)", derr.Kind, afp.KindBudget)
	}
}

// TestDecodePTOCA_RPSShortParam: an RPS control sequence with a single
// parameter byte (RLENGTH needs 2) must fail loudly via seqLenError
// (KindStructure), not panic or misdecode. Guards the len(params) < 2
// check in doRPS.
func TestDecodePTOCA_RPSShortParam(t *testing.T) {
	payload := seq(0xEE, 0xFF) // RPS with 1 param byte; RLENGTH needs 2.
	_, err := decodePTOCA(ptxField(payload), testCP(t), DefaultMaxTextBytes, nil)
	if err == nil {
		t.Fatal("decodePTOCA accepted a 1-byte RPS parameter")
	}
	var derr afp.DecodeError
	if !errors.As(err, &derr) {
		t.Fatalf("error %T (%v), want afp.DecodeError", err, err)
	}
	if derr.Kind != afp.KindStructure {
		t.Errorf("Kind = %q, want %q", derr.Kind, afp.KindStructure)
	}
	if derr.FieldID != "D3EE9B" {
		t.Errorf("FieldID = %q, want D3EE9B", derr.FieldID)
	}
}
