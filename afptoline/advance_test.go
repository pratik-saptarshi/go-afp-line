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
	// SIA = 5 L-units; "Hi" has 2 runes → 1 gap.
	// Advance = 12pt*20 + 5*1 = 245 L-units = 12.25pt.
	p := append(seq(0xC2, 0x00, 0x05), trn(0xC8, 0x89)...)
	p = append(p, seq(0xF0, 0x01)...)
	p = append(p, trn(0xE7)...)
	runs := decodeWithAdv(t, p, stubAdv)
	if len(runs) != 2 {
		t.Fatalf("got %d runs, want 2", len(runs))
	}
	approx(t, "run2 X", runs[1].X, 12.25)
}

func TestAdvance_SIASingleCharHasNoGap(t *testing.T) {
	// SIA = 5 but "X" is one rune → 0 gaps → 6pt advance, no sia term.
	p := append(seq(0xC2, 0x00, 0x05), trn(0xE7)...)
	p = append(p, seq(0xF0, 0x01)...)
	p = append(p, trn(0xE7)...)
	runs := decodeWithAdv(t, p, stubAdv)
	if len(runs) != 2 {
		t.Fatalf("got %d runs, want 2", len(runs))
	}
	approx(t, "run2 X", runs[1].X, 6)
}

func TestAdvance_NegativeSIATightens(t *testing.T) {
	// SIA = -10 L-units (0xFFF6); "Hi": 240 - 10 = 230 L-units = 11.5pt.
	p := append(seq(0xC2, 0xFF, 0xF6), trn(0xC8, 0x89)...)
	p = append(p, seq(0xF0, 0x01)...)
	p = append(p, trn(0xE7)...)
	runs := decodeWithAdv(t, p, stubAdv)
	if len(runs) != 2 {
		t.Fatalf("got %d runs, want 2", len(runs))
	}
	approx(t, "run2 X", runs[1].X, 11.5)
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
