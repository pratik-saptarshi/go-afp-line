package afptoline

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/pratik-saptarshi/go-afp-line/afp"
)

// decodeErr reduces fields and returns the failure as afp.DecodeError.
func decodeErr(t *testing.T, fields ...[]byte) afp.DecodeError {
	t.Helper()
	var buf bytes.Buffer
	for _, f := range fields {
		buf.Write(f)
	}
	_, _, err := Reduce(&buf)
	if err == nil {
		t.Fatal("Reduce accepted the stream, want a DecodeError")
	}
	var derr afp.DecodeError
	if !errors.As(err, &derr) {
		t.Fatalf("error %T (%v), want afp.DecodeError", err, err)
	}
	return derr
}

func docFields() (bdt, edt []byte) {
	bdt = rawField(tBDT, append(ebcdicSpaces(8), 0x00, 0x00))
	edt = rawField(tEDT, append(ebcdicSpaces(8), 0x00, 0x00))
	return bdt, edt
}

// TestReduce_StructureErrorCarriesOffsetAndKind: a structural pairing
// violation (nested BDT) reports the true cumulative stream offset through
// the offending field and the structure kind.
func TestReduce_StructureErrorCarriesOffsetAndKind(t *testing.T) {
	bdt1, _ := docFields()
	bdt2, _ := docFields()
	derr := decodeErr(t, bdt1, bdt2)
	if derr.Kind != afp.KindStructure {
		t.Errorf("Kind = %q, want %q", derr.Kind, afp.KindStructure)
	}
	if want := len(bdt1); derr.Offset != want {
		t.Errorf("Offset = %d, want %d (field-start offset of the offending BDT)", derr.Offset, want)
	}
	if derr.FieldID != "D3A8A8" {
		t.Errorf("FieldID = %q, want D3A8A8", derr.FieldID)
	}
}

// TestReduce_BudgetErrorCarriesKind: an amplification failure is a budget
// error with the true offset of the offending PTX.
func TestReduce_BudgetErrorCarriesKind(t *testing.T) {
	bdt, edt := docFields()
	bpg := rawField(tBPG, ebcdicSpaces(8))
	var payload []byte
	for i := 0; i < 4; i++ {
		payload = append(payload, seq(0xEE, 0x07, 0xD0, 0xC1)...)
	}
	ptx := rawField(tPTX, payload)
	epg := rawField(tEPG, ebcdicSpaces(8))

	var buf bytes.Buffer
	for _, f := range [][]byte{bdt, bpg, ptx, epg, edt} {
		buf.Write(f)
	}
	_, _, err := ReduceWithBudget(&buf, 4096)
	if err == nil {
		t.Fatal("ReduceWithBudget accepted an RPS stream expanding past the budget")
	}
	var derr afp.DecodeError
	if !errors.As(err, &derr) {
		t.Fatalf("error %T (%v), want afp.DecodeError", err, err)
	}
	if derr.Kind != afp.KindBudget {
		t.Errorf("Kind = %q, want %q", derr.Kind, afp.KindBudget)
	}
	if want := len(bdt) + len(bpg); derr.Offset != want {
		t.Errorf("Offset = %d, want %d (field-start offset of the offending PTX)", derr.Offset, want)
	}
}

// TestReduce_EncodingErrorCarriesKind: a C1 control (no WinAnsi encoding)
// fails as an encoding error with the true offset of the offending PTX.
func TestReduce_EncodingErrorCarriesKind(t *testing.T) {
	bdt, edt := docFields()
	bpg := rawField(tBPG, ebcdicSpaces(8))
	// 0x20 in code page 037 decodes to U+0080, the low end of the C1 range.
	ptx := rawField(tPTX, trn(0xC1, 0x20, 0xC2))
	epg := rawField(tEPG, ebcdicSpaces(8))

	derr := decodeErr(t, bdt, bpg, ptx, epg, edt)
	if derr.Kind != afp.KindEncoding {
		t.Errorf("Kind = %q, want %q", derr.Kind, afp.KindEncoding)
	}
	if want := len(bdt) + len(bpg); derr.Offset != want {
		t.Errorf("Offset = %d, want %d (field-start offset of the offending PTX)", derr.Offset, want)
	}
}

// TestReduce_ValidationErrorCarriesKind: an SCFL font ID with no pinned
// metrics mapping violates reducer policy — a validation error carrying the
// true offset of the offending PTX.
func TestReduce_ValidationErrorCarriesKind(t *testing.T) {
	bdt, edt := docFields()
	bpg := rawField(tBPG, ebcdicSpaces(8))
	ptx := rawField(tPTX, append(seq(0xF0, 0x05), trn(0xC1)...)) // SCFL LID 5, then "A" in font 5
	epg := rawField(tEPG, ebcdicSpaces(8))

	derr := decodeErr(t, bdt, bpg, ptx, epg, edt)
	if derr.Kind != afp.KindValidation {
		t.Errorf("Kind = %q, want %q", derr.Kind, afp.KindValidation)
	}
	if !strings.Contains(derr.Reason, "SCFL") {
		t.Errorf("Reason = %q, want it to name SCFL", derr.Reason)
	}
	if want := len(bdt) + len(bpg); derr.Offset != want {
		t.Errorf("Offset = %d, want %d (field-start offset of the offending PTX)", derr.Offset, want)
	}
}

// TestReduce_FailSitesCarryTrueOffsets: every reducer structural failure must
// carry a non-fake stream offset — st.off is incorporated by reduceState.fail
// itself, so no call site can forget it.
func TestReduce_FailSitesCarryTrueOffsets(t *testing.T) {
	bdt, edt := docFields()
	bpg := rawField(tBPG, ebcdicSpaces(8))
	epg := rawField(tEPG, ebcdicSpaces(8))
	bng := rawField([3]byte{0xD3, 0xA8, 0xAD}, nil)
	eng := rawField([3]byte{0xD3, 0xA9, 0xAD}, nil)

	cases := []struct {
		name   string
		fields [][]byte
		want   int // field-start offset of the offending field (0 is legitimate)
	}{
		{"EDT without BDT", [][]byte{edt}, 0},
		{"BPG outside document", [][]byte{bpg}, 0},
		{"EPG without BPG", [][]byte{bdt, epg}, len(bdt)},
		{"BNG in illegal position", [][]byte{bng}, 0},
		{"ENG without BNG", [][]byte{bdt, eng}, len(bdt)},
		{"end of stream with open document", [][]byte{bdt}, len(bdt)},
		{"end of stream with open page", [][]byte{bdt, bpg}, len(bdt) + len(bpg)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			derr := decodeErr(t, c.fields...)
			if derr.Kind != afp.KindStructure {
				t.Errorf("Kind = %q, want %q", derr.Kind, afp.KindStructure)
			}
			if derr.Offset != c.want {
				t.Errorf("Offset = %d, want %d (field-start offset of the offending field)", derr.Offset, c.want)
			}
		})
	}
}

// TestReduce_PTOCADecoderErrorOffsetRebased: a PTOCA decoder error carries
// a decoder-local offset (relative to the PTX payload); visitPTX must
// rebase it by the PTX field's stream start offset so DecodeError.Offset
// is stream-relative. Guards the st.off + derr.Offset rebase in visitPTX.
func TestReduce_PTOCADecoderErrorOffsetRebased(t *testing.T) {
	bdt, edt := docFields()
	bpg := rawField(tBPG, ebcdicSpaces(8))
	// Truncated chained introducer: decoder-local offset 5 (see
	// TestDecodePTOCA_ErrorPaths).
	ptx := rawField(tPTX, append(seq(0xF3, 0x05), 0x05))
	epg := rawField(tEPG, ebcdicSpaces(8))

	derr := decodeErr(t, bdt, bpg, ptx, epg, edt)
	if derr.Kind != afp.KindStructure {
		t.Errorf("Kind = %q, want %q", derr.Kind, afp.KindStructure)
	}
	if derr.FieldID != "D3EE9B" {
		t.Errorf("FieldID = %q, want D3EE9B", derr.FieldID)
	}
	// Stream-relative: PTX field start + decoder-local offset 5.
	if want := len(bdt) + len(bpg) + 5; derr.Offset != want {
		t.Errorf("Offset = %d, want %d (PTX field start + decoder-local offset 5)", derr.Offset, want)
	}
}

// TestReduce_CoordinateBoundsErrorIsTyped: a run whose X exceeds
// MaxCoordinate must fail Reduce as a typed afp.DecodeError
// (KindValidation), not an untyped line.ValidationError.
func TestReduce_CoordinateBoundsErrorIsTyped(t *testing.T) {
	bdt, edt := docFields()
	bpg := rawField(tBPG, ebcdicSpaces(8))
	// 9 × RMI(+32767 L-units) = 294903 L-units = 14745.15pt > MaxCoordinate
	// (14400pt). The following run's X is out of bounds.
	var payload []byte
	for i := 0; i < 9; i++ {
		payload = append(payload, seq16(0xC8, 32767)...)
	}
	payload = append(payload, trn(0xC1)...)
	ptx := rawField(tPTX, payload)
	epg := rawField(tEPG, ebcdicSpaces(8))

	var buf bytes.Buffer
	for _, f := range [][]byte{bdt, bpg, ptx, epg, edt} {
		buf.Write(f)
	}
	_, _, err := Reduce(&buf)
	if err == nil {
		t.Fatal("Reduce accepted a run with X beyond MaxCoordinate")
	}
	var derr afp.DecodeError
	if !errors.As(err, &derr) {
		t.Fatalf("error %T (%v), want afp.DecodeError", err, err)
	}
	if derr.Kind != afp.KindValidation {
		t.Errorf("Kind = %q, want %q", derr.Kind, afp.KindValidation)
	}
}
