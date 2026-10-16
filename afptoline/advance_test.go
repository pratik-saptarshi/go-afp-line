package afptoline

import (
	"errors"
	"math"
	"testing"

	"github.com/pratik-saptarshi/go-afp-line/afp"
	"github.com/pratik-saptarshi/go-afp-line/line"
)

// stubAdv returns 6pt per byte, ignoring the font. It stands in for the
// reducer's metrics-backed closure in decoder-level tests.
func stubAdv(fontLID byte, text string) (float64, error) {
	return float64(len(text)) * 6.0, nil
}

func decodeWithAdv(t *testing.T, payload []byte, adv advanceFunc) []Run {
	t.Helper()
	runs, err := decodePTOCA(ptxField(payload), testCP(t), -1, adv)
	if err != nil {
		t.Fatalf("decodePTOCA: %v", err)
	}
	return runs
}

// exact asserts a position to ±0.01 pt. SIA deltas are 0.25 pt, so the
// ±1 pt approx() helper cannot pin them — every SIA assertion uses exact.
func exact(t *testing.T, what string, got, want float64) {
	t.Helper()
	if d := got - want; d < -0.01 || d > 0.01 {
		t.Errorf("%s = %v, want %v (±0.01pt)", what, got, want)
	}
}

// EBCDIC-037 bytes: "Hi" = C8 89, "there" = A3 88 85 99 85, "X" = E7.
func hiTherePayload() []byte {
	p := append(trn(0xC8, 0x89), seq(0xF0, 0x01)...)       // "Hi", SCFL 1 (splits runs)
	return append(p, trn(0xA3, 0x88, 0x85, 0x99, 0x85)...) // "there"
}

func TestAdvance_TwoRunsSecondStartsAfterFirst(t *testing.T) {
	runs := decodeWithAdv(t, hiTherePayload(), stubAdv)
	if len(runs) != 2 {
		t.Fatalf("got %d runs, want 2", len(runs))
	}
	if runs[0].Text != "Hi" || runs[1].Text != "there" {
		t.Fatalf("texts = %q, %q", runs[0].Text, runs[1].Text)
	}
	approx(t, "run1 X", runs[0].X, 0)
	// "Hi" advances 2 chars * 6pt = 12pt.
	approx(t, "run2 X", runs[1].X, 12)
}

func TestAdvance_NilKeepsLegacyPositions(t *testing.T) {
	// DecodePTOCA passes nil: byte-for-byte today's behavior.
	runs := decode(t, hiTherePayload())
	if len(runs) != 2 {
		t.Fatalf("got %d runs, want 2", len(runs))
	}
	approx(t, "run1 X", runs[0].X, 0)
	approx(t, "run2 X", runs[1].X, 0)
}

func TestAdvance_AppliesSIA(t *testing.T) {
	// SIA = 5 L-units; "Hi" has 2 runes → 1 internal gap.
	// Run 2 "X" starts at (240 + 5) + 5 = 250 L-units = 12.5pt: the 240+5 is
	// run 1's width plus its internal H→i gap, and the second +5 is the
	// inter-run i→X gap ("Hi" and "X" form one word across the SCFL).
	p := append(seq(0xC2, 0x00, 0x05), trn(0xC8, 0x89)...)
	p = append(p, seq(0xF0, 0x01)...)
	p = append(p, trn(0xE7)...)
	runs := decodeWithAdv(t, p, stubAdv)
	if len(runs) != 2 {
		t.Fatalf("got %d runs, want 2", len(runs))
	}
	exact(t, "run2 X", runs[1].X, 12.5)
}

func TestAdvance_SIASingleCharHasNoGap(t *testing.T) {
	// SIA = 5 but "X" is one rune → 0 internal gaps. Run 2 "X" starts at
	// (120 + 5) L-units = 6.25pt: 120 is run 1's width, and the +5 is the
	// inter-run X→X gap (both word characters across the SCFL).
	p := append(seq(0xC2, 0x00, 0x05), trn(0xE7)...)
	p = append(p, seq(0xF0, 0x01)...)
	p = append(p, trn(0xE7)...)
	runs := decodeWithAdv(t, p, stubAdv)
	if len(runs) != 2 {
		t.Fatalf("got %d runs, want 2", len(runs))
	}
	exact(t, "run2 X", runs[1].X, 6.25)
}

func TestAdvance_NegativeSIATightens(t *testing.T) {
	// SIA = -10 L-units (0xFFF6); "Hi": run 2 "X" starts at
	// (240 - 10) - 10 = 220 L-units = 11.0pt (internal H→i gap plus the
	// inter-run i→X gap, both tightening).
	p := append(seq(0xC2, 0xFF, 0xF6), trn(0xC8, 0x89)...)
	p = append(p, seq(0xF0, 0x01)...)
	p = append(p, trn(0xE7)...)
	runs := decodeWithAdv(t, p, stubAdv)
	if len(runs) != 2 {
		t.Fatalf("got %d runs, want 2", len(runs))
	}
	exact(t, "run2 X", runs[1].X, 11)
}

func TestAdvance_RejectsBadWidth(t *testing.T) {
	for _, w := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), -1} {
		adv := func(byte, string) (float64, error) { return w, nil }
		_, err := decodePTOCA(ptxField(trn(0xC1)), testCP(t), -1, adv)
		var derr afp.DecodeError
		if !errors.As(err, &derr) {
			t.Errorf("width %v: err = %v, want afp.DecodeError", w, err)
			continue
		}
		if derr.Kind != afp.KindValidation {
			t.Errorf("width %v: Kind = %v, want KindValidation", w, derr.Kind)
		}
	}
}

func TestAdvance_PropagatesClosureError(t *testing.T) {
	want := errors.New("metrics unavailable")
	adv := func(byte, string) (float64, error) { return 0, want }
	_, err := decodePTOCA(ptxField(trn(0xC1)), testCP(t), -1, adv)
	if !errors.Is(err, want) {
		t.Errorf("err = %v, want wrapped %v", err, want)
	}
}

func TestAdvance_SIAAcrossSCFLRunSplit(t *testing.T) {
	// Codex P2: SIA 5, "A", SCFL 0, "B" — A and B form one word across the
	// font change, so the A→B sia gap shifts B's presentation position:
	// B starts at (120 + 5) L-units = 6.25pt.
	p := append(seq(0xC2, 0x00, 0x05), trn(0xC1)...)
	p = append(p, seq(0xF0, 0x00)...)
	p = append(p, trn(0xC2)...)
	runs := decodeWithAdv(t, p, stubAdv)
	if len(runs) != 2 {
		t.Fatalf("got %d runs, want 2", len(runs))
	}
	exact(t, "run2 X", runs[1].X, 6.25)
}

func TestAdvance_SIANoGapAcrossLeadingSpace(t *testing.T) {
	// SIA 5, "A", SCFL 0, " B": the space ends the word — no inter-run gap.
	p := append(seq(0xC2, 0x00, 0x05), trn(0xC1)...)
	p = append(p, seq(0xF0, 0x00)...)
	p = append(p, trn(0x40, 0xC2)...)
	runs := decodeWithAdv(t, p, stubAdv)
	if len(runs) != 2 {
		t.Fatalf("got %d runs, want 2", len(runs))
	}
	exact(t, "run2 X", runs[1].X, 6)
}

func TestAdvance_SIANoGapAcrossMove(t *testing.T) {
	// SIA 5, "A", RMI 0, "B": even a zero move breaks the word for SIA.
	p := append(seq(0xC2, 0x00, 0x05), trn(0xC1)...)
	p = append(p, seq(0xC8, 0x00, 0x00)...)
	p = append(p, trn(0xC2)...)
	runs := decodeWithAdv(t, p, stubAdv)
	if len(runs) != 2 {
		t.Fatalf("got %d runs, want 2", len(runs))
	}
	exact(t, "run2 X", runs[1].X, 6)
}

func TestAdvance_SIANoGapAroundSpaces(t *testing.T) {
	// SIA 5, "A B" (one run), SCFL 0, "C": no sia around the space, but the
	// B→C inter-run gap applies, so C starts at (360 + 5) L-units = 18.25pt.
	p := append(seq(0xC2, 0x00, 0x05), trn(0xC1, 0x40, 0xC2)...)
	p = append(p, seq(0xF0, 0x00)...)
	p = append(p, trn(0xC3)...)
	runs := decodeWithAdv(t, p, stubAdv)
	if len(runs) != 2 {
		t.Fatalf("got %d runs, want 2", len(runs))
	}
	exact(t, "run2 X", runs[1].X, 18.25)
}

func TestAdvance_SIAChainThreeRuns(t *testing.T) {
	// SIA 5, "A" SCFL "B" SCFL "C": three consecutive advancing runs.
	// Run 1 "A" at 0; flush advances st.ic by 120 L-units (6pt), 0 gaps.
	// Run 2 "B" at 6.25pt (120 + 5 inter-run gap); flush advances st.ic to
	// 245 L-units. Run 3 "C" at 12.5pt. A st.ic "=" mutant (instead of "+=")
	// would put run 3 at 6.25pt, which single-advance tests cannot catch.
	p := append(seq(0xC2, 0x00, 0x05), trn(0xC1)...)
	p = append(p, seq(0xF0, 0x00)...)
	p = append(p, trn(0xC2)...)
	p = append(p, seq(0xF0, 0x00)...)
	p = append(p, trn(0xC3)...)
	runs := decodeWithAdv(t, p, stubAdv)
	if len(runs) != 3 {
		t.Fatalf("got %d runs, want 3", len(runs))
	}
	exact(t, "run1 X", runs[0].X, 0)
	exact(t, "run2 X", runs[1].X, 6.25)
	exact(t, "run3 X", runs[2].X, 12.5)
}

func TestAdvance_OverstrikeRMIBackToZero(t *testing.T) {
	// "Hi", RMI -240 L-units (back over the just-advanced width), "Hi".
	// A reducer post-pass would place run 2 at 12pt; the state machine
	// puts it at 0. This test kills that alternative.
	p := append(trn(0xC8, 0x89), seq(0xC8, 0xFF, 0x10)...)
	p = append(p, trn(0xC8, 0x89)...)
	runs := decodeWithAdv(t, p, stubAdv)
	if len(runs) != 2 {
		t.Fatalf("got %d runs, want 2", len(runs))
	}
	approx(t, "run1 X", runs[0].X, 0)
	approx(t, "run2 X", runs[1].X, 0)
}

func TestAdvance_BLNResetsToMargin(t *testing.T) {
	// "Hi", BLN, "there": the new line starts at the margin, not at 12pt.
	p := append(trn(0xC8, 0x89), seq(0xD8)...)
	p = append(p, trn(0xA3, 0x88, 0x85, 0x99, 0x85)...)
	runs := decodeWithAdv(t, p, stubAdv)
	if len(runs) != 2 {
		t.Fatalf("got %d runs, want 2", len(runs))
	}
	approx(t, "run2 X", runs[1].X, 0)
}

func TestReduce_AdvanceUnmappedFontFailsLoud(t *testing.T) {
	// SCFL 5 has no pinned mapping: the reducer's advance closure must fail
	// with Kind/FieldID/Offset attached, preempting later checks.
	p := append(seq(0xF0, 0x05), trn(0xC1)...)
	st := &reduceState{
		cp:       testCP(t),
		textLeft: 1 << 20,
		page:     &line.Page{Width: 612, Height: 792},
		off:      1234,
	}
	err := st.visitPTX(ptxField(p).(*afp.RawField))
	var derr afp.DecodeError
	if !errors.As(err, &derr) {
		t.Fatalf("err = %v, want afp.DecodeError", err)
	}
	if derr.Kind != afp.KindValidation {
		t.Errorf("Kind = %v, want KindValidation", derr.Kind)
	}
	if derr.FieldID != "D3EE9B" {
		t.Errorf("FieldID = %q, want D3EE9B", derr.FieldID)
	}
	if derr.Offset != 1234 {
		t.Errorf("Offset = %d, want 1234", derr.Offset)
	}
}

func TestReduce_AdvanceFailurePreemptsLaterChecks(t *testing.T) {
	// SCFL 5 (unmapped) plus a C1 byte: the decode-time advance failure
	// must win over the C1 rejection that runs after decode. (The
	// in-decoder fail-fast budget in text() still fires first by design —
	// it guards before any byte is accumulated.)
	p := append(seq(0xF0, 0x05), trn(0xC1, 0x80)...) // "A" + C1 0x80
	st := &reduceState{
		cp:       testCP(t),
		textLeft: 1 << 20,
		page:     &line.Page{Width: 612, Height: 792},
		off:      99,
	}
	err := st.visitPTX(ptxField(p).(*afp.RawField))
	var derr afp.DecodeError
	if !errors.As(err, &derr) {
		t.Fatalf("err = %v, want afp.DecodeError", err)
	}
	if derr.Kind != afp.KindValidation {
		t.Errorf("Kind = %v, want KindValidation (advance preempts C1 check)", derr.Kind)
	}
	if derr.FieldID != "D3EE9B" || derr.Offset != 99 {
		t.Errorf("got FieldID %q Offset %d, want D3EE9B/99", derr.FieldID, derr.Offset)
	}
}
