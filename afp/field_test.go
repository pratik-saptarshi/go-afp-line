package afp

import (
	"bytes"
	"fmt"
	"testing"
)

// Test vectors use real EBCDIC-037 bytes. "HELLOWLD" =
// C8 C5 D3 D3 D6 E6 D3 C4, "PAGE0001" = D7 C1 C7 C5 F0 F0 F0 F1.
// The structured field introducer is 8 bytes: 2B length (big-endian,
// including the introducer) + 3B type ID + 1B flags + 2B reserved.

var (
	// BDT, SFLength=18: introducer + DocName(8) + reserved(2).
	bdtHello = []byte{
		0x00, 0x12, 0xD3, 0xA8, 0xA8, 0x00, 0x00, 0x00,
		0xC8, 0xC5, 0xD3, 0xD3, 0xD6, 0xE6, 0xD3, 0xC4, // "HELLOWLD"
		0x00, 0x00,
	}
	// EDT, SFLength=18.
	edtHello = []byte{
		0x00, 0x12, 0xD3, 0xA9, 0xA8, 0x00, 0x00, 0x00,
		0xC8, 0xC5, 0xD3, 0xD3, 0xD6, 0xE6, 0xD3, 0xC4, // "HELLOWLD"
		0x00, 0x00,
	}
	// BPG, SFLength=16: introducer + PageName(8).
	bpgPage = []byte{
		0x00, 0x10, 0xD3, 0xA8, 0xAF, 0x00, 0x00, 0x00,
		0xD7, 0xC1, 0xC7, 0xC5, 0xF0, 0xF0, 0xF0, 0xF1, // "PAGE0001"
	}
	// EPG, SFLength=16.
	epgPage = []byte{
		0x00, 0x10, 0xD3, 0xA9, 0xAF, 0x00, 0x00, 0x00,
		0xD7, 0xC1, 0xC7, 0xC5, 0xF0, 0xF0, 0xF0, 0xF1, // "PAGE0001"
	}
	// BPG with no page name: introducer only, SFLength=8.
	bpgNoName = []byte{0x00, 0x08, 0xD3, 0xA8, 0xAF, 0x00, 0x00, 0x00}
	// Unknown type D3FFFF.
	unknownField = []byte{0x00, 0x08, 0xD3, 0xFF, 0xFF, 0x00, 0x00, 0x00}
	// BGR (Begin Graphics, D3A8BB): known-excluded per the subset contract.
	bgrExcluded = []byte{0x00, 0x08, 0xD3, 0xA8, 0xBB, 0x00, 0x00, 0x00}
	// BOC (Begin Object Container, D3A892), EOC (D3A992), OCD (Object
	// Container Data, D3EE92): known-excluded per the contract §9 closed
	// exclusion list.
	bocExcluded = []byte{0x00, 0x08, 0xD3, 0xA8, 0x92, 0x00, 0x00, 0x00}
	eocExcluded = []byte{0x00, 0x08, 0xD3, 0xA9, 0x92, 0x00, 0x00, 0x00}
	ocdExcluded = []byte{0x00, 0x08, 0xD3, 0xEE, 0x92, 0x00, 0x00, 0x00}
	// PTX (D3EE9B): contracted in-subset, typed decoder arrives in Task 6.
	ptxContracted = []byte{0x00, 0x08, 0xD3, 0xEE, 0x9B, 0x00, 0x00, 0x00}
	// Structural envelope/resource/page-map fields (DAP-0021 Task 2):
	// BPF/EPF (Begin/End Print File), BRS/ERS (Begin/End Resource),
	// PGD (Page Descriptor), MDR (Map Data Resource) — contracted
	// in-subset per the contract §3; typed decoders arrive in later tasks.
	bpfContracted = []byte{0x00, 0x08, 0xD3, 0xA8, 0xA5, 0x00, 0x00, 0x00}
	epfContracted = []byte{0x00, 0x08, 0xD3, 0xA9, 0xA5, 0x00, 0x00, 0x00}
	brsContracted = []byte{0x00, 0x08, 0xD3, 0xA8, 0xCE, 0x00, 0x00, 0x00}
	ersContracted = []byte{0x00, 0x08, 0xD3, 0xA9, 0xCE, 0x00, 0x00, 0x00}
	pgdContracted = []byte{0x00, 0x08, 0xD3, 0xA6, 0xAF, 0x00, 0x00, 0x00}
	mdrContracted = []byte{0x00, 0x08, 0xD3, 0xAB, 0xC3, 0x00, 0x00, 0x00}
	// IOB (Include Object, D3AFC3): known-excluded per the DAP-0021 Task 2
	// caveat — the profile file's IOB includes an object container whose
	// OCD payload is JPEG image data (content-bearing, excluded class).
	iobExcluded = []byte{0x00, 0x08, 0xD3, 0xAF, 0xC3, 0x00, 0x00, 0x00}
	// BDT claiming 12 bytes but carrying only 4 data bytes: truncated data.
	bdtTruncatedData = []byte{
		0x00, 0x0C, 0xD3, 0xA8, 0xA8, 0x00, 0x00, 0x00,
		0xC8, 0xC5, 0xD3, 0xD3,
	}
	// BOC (newly-registered excluded type) claiming SFLength=16 but carrying
	// only 12 bytes total: framing must still fail with KindStructure.
	bocLengthMismatch = []byte{
		0x00, 0x10, 0xD3, 0xA8, 0x92, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
	}
	// BPF (newly-registered contracted type) claiming SFLength=16 but
	// carrying only 12 bytes total: framing must still fail with
	// KindStructure, never silent passthrough.
	bpfLengthMismatch = []byte{
		0x00, 0x10, 0xD3, 0xA8, 0xA5, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
	}
)

func TestDecodeField_BDT(t *testing.T) {
	f, err := DecodeField(bdtHello)
	if err != nil {
		t.Fatalf("DecodeField(BDT) error: %v", err)
	}
	bdt, ok := f.(*BDT)
	if !ok {
		t.Fatalf("DecodeField(BDT) type = %T, want *BDT", f)
	}
	if bdt.DocName != "HELLOWLD" {
		t.Errorf("DocName = %q, want %q", bdt.DocName, "HELLOWLD")
	}
	if f.TypeID() != "D3A8A8" {
		t.Errorf("TypeID() = %q, want %q", f.TypeID(), "D3A8A8")
	}
}

func TestDecodeField_EDT(t *testing.T) {
	f, err := DecodeField(edtHello)
	if err != nil {
		t.Fatalf("DecodeField(EDT) error: %v", err)
	}
	edt, ok := f.(*EDT)
	if !ok {
		t.Fatalf("DecodeField(EDT) type = %T, want *EDT", f)
	}
	if edt.DocName != "HELLOWLD" {
		t.Errorf("DocName = %q, want %q", edt.DocName, "HELLOWLD")
	}
}

func TestDecodeField_BPG(t *testing.T) {
	f, err := DecodeField(bpgPage)
	if err != nil {
		t.Fatalf("DecodeField(BPG) error: %v", err)
	}
	bpg, ok := f.(*BPG)
	if !ok {
		t.Fatalf("DecodeField(BPG) type = %T, want *BPG", f)
	}
	if bpg.PageName != "PAGE0001" {
		t.Errorf("PageName = %q, want %q", bpg.PageName, "PAGE0001")
	}
}

func TestDecodeField_EPG(t *testing.T) {
	f, err := DecodeField(epgPage)
	if err != nil {
		t.Fatalf("DecodeField(EPG) error: %v", err)
	}
	if _, ok := f.(*EPG); !ok {
		t.Fatalf("DecodeField(EPG) type = %T, want *EPG", f)
	}
}

func TestDecodeField_BPG_NoName(t *testing.T) {
	f, err := DecodeField(bpgNoName)
	if err != nil {
		t.Fatalf("DecodeField(BPG, no name) error: %v", err)
	}
	bpg, ok := f.(*BPG)
	if !ok {
		t.Fatalf("type = %T, want *BPG", f)
	}
	if bpg.PageName != "" {
		t.Errorf("PageName = %q, want empty", bpg.PageName)
	}
}

func TestDecodeField_BPG_BlankName(t *testing.T) {
	// All-spaces PageName: decodes to "" but must re-encode byte-identically.
	blank := []byte{
		0x00, 0x10, 0xD3, 0xA8, 0xAF, 0x00, 0x00, 0x00,
		0x40, 0x40, 0x40, 0x40, 0x40, 0x40, 0x40, 0x40,
	}
	f, err := DecodeField(blank)
	if err != nil {
		t.Fatalf("DecodeField(BPG, blank name) error: %v", err)
	}
	bpg, ok := f.(*BPG)
	if !ok {
		t.Fatalf("type = %T, want *BPG", f)
	}
	if bpg.PageName != "" {
		t.Errorf("PageName = %q, want empty", bpg.PageName)
	}
	if !bpg.HasPageName {
		t.Errorf("HasPageName = false, want true: the 8-byte name was on the wire")
	}
	if got := f.Encode(); !bytes.Equal(got, blank) {
		t.Errorf("Encode() = %X, want %X", got, blank)
	}
}

func TestDecodeField_UnknownType(t *testing.T) {
	_, err := DecodeField(unknownField)
	derr, ok := err.(DecodeError)
	if !ok {
		t.Fatalf("DecodeField(unknown) error = %T (%v), want DecodeError", err, err)
	}
	if derr.FieldID != "D3FFFF" {
		t.Errorf("FieldID = %q, want %q", derr.FieldID, "D3FFFF")
	}
}

func TestDecodeField_ExcludedGOCA(t *testing.T) {
	f, err := DecodeField(bgrExcluded)
	if err != nil {
		t.Fatalf("DecodeField(BGR) error: %v (excluded fields must not error)", err)
	}
	raw, ok := f.(*RawField)
	if !ok {
		t.Fatalf("DecodeField(BGR) type = %T, want *RawField", f)
	}
	if !raw.Excluded {
		t.Errorf("RawField.Excluded = false, want true for a known-excluded field")
	}
	if !bytes.Equal(raw.Encode(), bgrExcluded) {
		t.Errorf("RawField.Encode() = %X, want input bytes %X", raw.Encode(), bgrExcluded)
	}
}

func TestDecodeField_ObjectContainerExcluded(t *testing.T) {
	vectors := []struct {
		name   string
		raw    []byte
		typeID string
	}{
		{"BOC", bocExcluded, "D3A892"},
		{"EOC", eocExcluded, "D3A992"},
		{"OCD", ocdExcluded, "D3EE92"},
		// IOB is excluded per the DAP-0021 Task 2 caveat: the profile
		// file's IOB includes an object container whose OCD payload is
		// JPEG image data (content-bearing, excluded class).
		{"IOB", iobExcluded, "D3AFC3"},
	}
	for _, v := range vectors {
		f, err := DecodeField(v.raw)
		if err != nil {
			t.Errorf("DecodeField(%s) error: %v (excluded fields must not error)", v.name, err)
			continue
		}
		raw, ok := f.(*RawField)
		if !ok {
			t.Errorf("DecodeField(%s) type = %T, want *RawField", v.name, f)
			continue
		}
		if !raw.Excluded {
			t.Errorf("DecodeField(%s): RawField.Excluded = false, want true", v.name)
		}
		if f.TypeID() != v.typeID {
			t.Errorf("DecodeField(%s): TypeID() = %q, want %q", v.name, f.TypeID(), v.typeID)
		}
		if !bytes.Equal(raw.Encode(), v.raw) {
			t.Errorf("DecodeField(%s): Encode() = %X, want input bytes %X", v.name, raw.Encode(), v.raw)
		}
	}
}

// TestDecodeField_StructuralContracted pins the DAP-0021 Task 2
// registrations: the structural envelope/resource/page-map fields decode as
// *RawField with Excluded=false (contracted in-subset; typed decoders arrive
// in later tasks), never an error.
func TestDecodeField_StructuralContracted(t *testing.T) {
	vectors := []struct {
		name   string
		raw    []byte
		typeID string
	}{
		{"BPF", bpfContracted, "D3A8A5"},
		{"EPF", epfContracted, "D3A9A5"},
		{"BRS", brsContracted, "D3A8CE"},
		{"ERS", ersContracted, "D3A9CE"},
		{"PGD", pgdContracted, "D3A6AF"},
		{"MDR", mdrContracted, "D3ABC3"},
	}
	for _, v := range vectors {
		f, err := DecodeField(v.raw)
		if err != nil {
			t.Errorf("DecodeField(%s) error: %v (contracted fields must not error)", v.name, err)
			continue
		}
		raw, ok := f.(*RawField)
		if !ok {
			t.Errorf("DecodeField(%s) type = %T, want *RawField", v.name, f)
			continue
		}
		if raw.Excluded {
			t.Errorf("DecodeField(%s): RawField.Excluded = true, want false: contracted, not excluded", v.name)
		}
		if f.TypeID() != v.typeID {
			t.Errorf("DecodeField(%s): TypeID() = %q, want %q", v.name, f.TypeID(), v.typeID)
		}
		if !bytes.Equal(raw.Encode(), v.raw) {
			t.Errorf("DecodeField(%s): Encode() = %X, want input bytes %X", v.name, raw.Encode(), v.raw)
		}
	}
}

// TestDecodeField_ContractedMalformedFraming pins that a contracted type
// with a framing violation fails with KindStructure before the registry is
// consulted: 12 bytes on the wire against a claimed SFLength of 16.
func TestDecodeField_ContractedMalformedFraming(t *testing.T) {
	_, err := DecodeField(bpfLengthMismatch)
	derr, ok := err.(DecodeError)
	if !ok {
		t.Fatalf("DecodeField(length-mismatched BPF) error = %T (%v), want DecodeError", err, err)
	}
	if derr.Kind != KindStructure {
		t.Errorf("DecodeError.Kind = %v, want KindStructure", derr.Kind)
	}
	if want := "field byte count 12 does not match SFLength 16"; derr.Reason != want {
		t.Errorf("DecodeError.Reason = %q, want %q", derr.Reason, want)
	}
	if derr.FieldID != "D3A8A5" {
		t.Errorf("DecodeError.FieldID = %q, want %q", derr.FieldID, "D3A8A5")
	}
}

func TestDecodeField_UnknownStillAborts(t *testing.T) {
	_, err := DecodeField(unknownField)
	derr, ok := err.(DecodeError)
	if !ok {
		t.Fatalf("DecodeField(D3FFFF) error = %T (%v), want DecodeError", err, err)
	}
	if derr.Kind != KindValidation {
		t.Errorf("DecodeError.Kind = %v, want KindValidation", derr.Kind)
	}
	if want := "unknown structured field type: aborting (not in contracted subset)"; derr.Reason != want {
		t.Errorf("DecodeError.Reason = %q, want %q", derr.Reason, want)
	}
	if derr.FieldID != "D3FFFF" {
		t.Errorf("DecodeError.FieldID = %q, want %q", derr.FieldID, "D3FFFF")
	}
}

func TestDecodeField_NewTypeMalformedFraming(t *testing.T) {
	// BOC is now a registered excluded type, but framing violations must
	// still fail before the registry is consulted: 12 bytes on the wire
	// against a claimed SFLength of 16.
	_, err := DecodeField(bocLengthMismatch)
	derr, ok := err.(DecodeError)
	if !ok {
		t.Fatalf("DecodeField(length-mismatched BOC) error = %T (%v), want DecodeError", err, err)
	}
	if derr.Kind != KindStructure {
		t.Errorf("DecodeError.Kind = %v, want KindStructure", derr.Kind)
	}
	if want := "field byte count 12 does not match SFLength 16"; derr.Reason != want {
		t.Errorf("DecodeError.Reason = %q, want %q", derr.Reason, want)
	}
	if derr.FieldID != "D3A892" {
		t.Errorf("DecodeError.FieldID = %q, want %q", derr.FieldID, "D3A892")
	}
}

// TestRegistryMatchesContract is a tripwire against code/contract drift:
// the registry's full SFID->(name, kind) set must equal this hardcoded map,
// which mirrors docs/subset-contract.md §3 (in-scope) and §9 (closed
// exclusion list). The reviewer checks the map against the contract.
func TestRegistryMatchesContract(t *testing.T) {
	want := map[string]struct {
		name string
		kind fieldKind
	}{
		// Typed in this task (contract §3).
		"D3A8A8": {"BDT", kindTyped},
		"D3A9A8": {"EDT", kindTyped},
		"D3A8AF": {"BPG", kindTyped},
		"D3A9AF": {"EPG", kindTyped},
		// Contracted in-subset; typed decoders arrive in later tasks (§3).
		"D3A8AD": {"BNG", kindContracted},
		"D3A9AD": {"ENG", kindContracted},
		"D3A8C9": {"BAG", kindContracted},
		"D3A9C9": {"EAG", kindContracted},
		"D3A89B": {"BPT", kindContracted},
		"D3A99B": {"EPT", kindContracted},
		"D3EE9B": {"PTX", kindContracted},
		"D3AB8A": {"MCF", kindContracted},
		"D3B19B": {"PTD", kindContracted},
		// Structural envelope/resource/page-map fields (contract §3,
		// DAP-0021 Task 2; IBM MODCA-Reference-09 primary).
		"D3A8A5": {"BPF", kindContracted},
		"D3A9A5": {"EPF", kindContracted},
		"D3A8CE": {"BRS", kindContracted},
		"D3A9CE": {"ERS", kindContracted},
		"D3A6AF": {"PGD", kindContracted},
		"D3ABC3": {"MDR", kindContracted},
		// Known-excluded per the contract §9 closed exclusion list.
		// IOCA (excluded)
		"D3A8FB": {"BIM", kindExcluded},
		"D3A9FB": {"EIM", kindExcluded},
		"D3EEFB": {"IPD", kindExcluded},
		"D3A6FB": {"IDD", kindExcluded},
		"D3A67B": {"IID", kindExcluded},
		"D3A77B": {"IOC", kindExcluded},
		"D3AC7B": {"ICP", kindExcluded},
		"D3EE7B": {"IRD", kindExcluded},
		// GOCA (excluded)
		"D3A8BB": {"BGR", kindExcluded},
		"D3A9BB": {"EGR", kindExcluded},
		"D3EEBB": {"GAD", kindExcluded},
		"D3A6BB": {"GDD", kindExcluded},
		// BCOCA (excluded)
		"D3A8EB": {"BBC", kindExcluded},
		"D3A9EB": {"EBC", kindExcluded},
		"D3EEEB": {"BDA", kindExcluded},
		"D3A6EB": {"BDD", kindExcluded},
		"D3ABEB": {"MBC", kindExcluded},
		// FOCA (excluded)
		"D3A88A": {"BCF", kindExcluded},
		"D3A98A": {"ECF", kindExcluded},
		"D3A889": {"BFN", kindExcluded},
		"D3A989": {"EFN", kindExcluded},
		"D3A887": {"BCP", kindExcluded},
		"D3A987": {"ECP", kindExcluded},
		"D3A78A": {"CFC", kindExcluded},
		"D38C8A": {"CFI", kindExcluded},
		"D3A787": {"CPC", kindExcluded},
		"D3A687": {"CPD", kindExcluded},
		"D38C87": {"CPI", kindExcluded},
		"D3A789": {"FNC", kindExcluded},
		"D3A689": {"FND", kindExcluded},
		"D3EE89": {"FNG", kindExcluded},
		"D38C89": {"FNI", kindExcluded},
		"D3A289": {"FNM", kindExcluded},
		"D3AB89": {"FNN", kindExcluded},
		"D3AE89": {"FNO", kindExcluded},
		"D3AC89": {"FNP", kindExcluded},
		"D3B18A": {"MCF1", kindExcluded},
		// CMOCA / color (excluded)
		"D3A877": {"BCA", kindExcluded},
		"D3A977": {"ECA", kindExcluded},
		"D3AB77": {"MCA", kindExcluded},
		"D3B077": {"CAT", kindExcluded},
		// Overlays / medium (excluded)
		"D3A8DF": {"BMO", kindExcluded},
		"D3A9DF": {"EMO", kindExcluded},
		"D3A8CC": {"BMM", kindExcluded},
		"D3A9CC": {"EMM", kindExcluded},
		"D3B1DF": {"MMO", kindExcluded},
		"D3ABD8": {"MPO", kindExcluded},
		"D3ABCC": {"IMM", kindExcluded},
		"D3A288": {"MCC", kindExcluded},
		"D3A688": {"MDD", kindExcluded},
		"D3A788": {"MMC", kindExcluded},
		"D3A8C6": {"BRG", kindExcluded},
		"D3A9C6": {"ERG", kindExcluded},
		"D3A8D9": {"BSG", kindExcluded},
		"D3A9D9": {"ESG", kindExcluded},
		"D3A8C4": {"BDG", kindExcluded},
		"D3A9C4": {"EDG", kindExcluded},
		"D3A8C7": {"BOG", kindExcluded},
		"D3A9C7": {"EOG", kindExcluded},
		// Misc skip (excluded, surfaced as diagnostics, never silent)
		"D3EEEE": {"NOP", kindExcluded},
		"D3A090": {"TLE", kindExcluded},
		"D3AF5F": {"IPS", kindExcluded},
		"D3B15F": {"MPS", kindExcluded},
		// Object containers (contract §9 — excluded)
		"D3A892": {"BOC", kindExcluded},
		"D3A992": {"EOC", kindExcluded},
		"D3EE92": {"OCD", kindExcluded},
		// IOB: excluded per the DAP-0021 Task 2 caveat (profile file's
		// IOB includes an object container whose OCD payload is JPEG
		// image data), not contracted.
		"D3AFC3": {"IOB", kindExcluded},
	}

	got := make(map[string]fieldSpec, len(fieldRegistry))
	for id, spec := range fieldRegistry {
		key := fmt.Sprintf("%02X%02X%02X", id[0], id[1], id[2])
		got[key] = spec
	}
	if len(got) != len(want) {
		t.Errorf("registry has %d entries, contract map has %d", len(got), len(want))
	}
	for key, w := range want {
		spec, ok := got[key]
		if !ok {
			t.Errorf("SFID %s (%s): in contract, missing from registry", key, w.name)
			continue
		}
		if spec.name != w.name {
			t.Errorf("SFID %s: registry name %q, contract %q", key, spec.name, w.name)
		}
		if spec.kind != w.kind {
			t.Errorf("SFID %s (%s): registry kind %d, contract kind %d", key, w.name, spec.kind, w.kind)
		}
	}
	for key, spec := range got {
		if _, ok := want[key]; !ok {
			t.Errorf("SFID %s (%s): in registry but not in the contract map", key, spec.name)
		}
	}
}

func TestDecodeField_ContractedUntyped(t *testing.T) {
	f, err := DecodeField(ptxContracted)
	if err != nil {
		t.Fatalf("DecodeField(PTX) error: %v", err)
	}
	raw, ok := f.(*RawField)
	if !ok {
		t.Fatalf("DecodeField(PTX) type = %T, want *RawField", f)
	}
	if raw.Excluded {
		t.Errorf("RawField.Excluded = true, want false: PTX is contracted, not excluded")
	}
}

func TestDecodeField_TruncatedData(t *testing.T) {
	if _, err := DecodeField(bdtTruncatedData); err == nil {
		t.Errorf("DecodeField(truncated BDT data) = nil error, want DecodeError")
	}
}

func TestField_RoundTrip(t *testing.T) {
	vectors := [][]byte{
		bdtHello, edtHello, bpgPage, epgPage, bpgNoName,
		bgrExcluded, ptxContracted,
		bpfContracted, epfContracted, brsContracted, ersContracted,
		pgdContracted, mdrContracted, iobExcluded,
	}
	for i, v := range vectors {
		f, err := DecodeField(v)
		if err != nil {
			t.Fatalf("vector %d: DecodeField error: %v", i, err)
		}
		if got := f.Encode(); !bytes.Equal(got, v) {
			t.Errorf("vector %d (%s): Encode() = %X, want %X", i, f.TypeID(), got, v)
		}
	}
}

func FuzzDecodeField(f *testing.F) {
	f.Add(bdtHello)
	f.Add(bpgPage)
	f.Add(bgrExcluded)
	f.Add(unknownField)
	f.Add([]byte{})
	f.Add([]byte{0x00, 0x08, 0xD3})
	f.Fuzz(func(t *testing.T, b []byte) {
		fld, err := DecodeField(b)
		if err != nil {
			return // loud, typed failure: the contract for malformed input
		}
		enc := fld.Encode()
		fld2, err := DecodeField(enc)
		if err != nil {
			t.Fatalf("re-decode of Encode() output failed: %v", err)
		}
		if fld2.TypeID() != fld.TypeID() {
			t.Fatalf("round-trip TypeID changed: %q -> %q", fld.TypeID(), fld2.TypeID())
		}
	})
}

func TestAcronym(t *testing.T) {
	cases := []struct {
		typeID string
		want   string
		ok     bool
	}{
		{"D3A8A8", "BDT", true},
		{"D3A9AF", "EPG", true},
		{"D3EE9B", "PTX", true}, // contracted in-subset: named, no typed decoder yet
		{"D3A8BB", "BGR", true},
		{"D3ABCC", "IMM", true},
		{"D3FFFF", "", false}, // truly unknown
		{"ZZ", "", false},     // malformed hex
		{"D3A8", "", false},   // wrong length
	}
	for _, c := range cases {
		got, ok := Acronym(c.typeID)
		if got != c.want || ok != c.ok {
			t.Errorf("Acronym(%q) = (%q, %v), want (%q, %v)", c.typeID, got, ok, c.want, c.ok)
		}
	}
}

func TestEncodeCHAR8_FullReverseTable(t *testing.T) {
	// Every EBCDIC-037 byte must survive decodeCHAR -> encodeCHAR8: the
	// decode table is injective, so the reverse map is total over decoded
	// output. The old ASCII-only reverse map turned e.g. 0x42 ('â') into
	// '?' (0x6F), breaking the byte-identical RoundTrip gate.
	for v := 0; v < 256; v++ {
		s := decodeCHAR([]byte{byte(v)})
		enc := encodeCHAR8(s)
		if len(enc) != 8 {
			t.Fatalf("byte 0x%02X: encodeCHAR8 len = %d, want 8", v, len(enc))
		}
		if enc[0] != byte(v) {
			t.Errorf("byte 0x%02X: round trip gave 0x%02X (via %q)", v, enc[0], s)
		}
	}
}

func TestBDT_NonASCIIDecodableNameRoundTrips(t *testing.T) {
	// DocName "DOCâ    " (0x42 = U+00E2 â): decode yields non-ASCII runes
	// that still have EBCDIC-037 mappings, so re-encode must be
	// byte-identical — no silent '?' substitution.
	raw := []byte{
		0x00, 0x12, 0xD3, 0xA8, 0xA8, 0x00, 0x00, 0x00,
		0xC4, 0xD6, 0xC3, 0x42, 0x40, 0x40, 0x40, 0x40, // "DOCâ    "
		0x00, 0x00,
	}
	f, err := DecodeField(raw)
	if err != nil {
		t.Fatalf("DecodeField(BDT) error: %v", err)
	}
	bdt, ok := f.(*BDT)
	if !ok {
		t.Fatalf("type = %T, want *BDT", f)
	}
	if bdt.DocName != "DOCâ" {
		t.Fatalf("DocName = %q, want %q", bdt.DocName, "DOCâ")
	}
	if enc := f.Encode(); !bytes.Equal(enc, raw) {
		t.Errorf("Encode() = %X, want %X", enc, raw)
	}
}

func TestEncodeCHAR8_UnmappableRunePanics(t *testing.T) {
	// U+20AC EURO SIGN has no EBCDIC-037 mapping: loud panic, never '?'.
	assertPanics(t, "encodeCHAR8 unmappable rune", func() {
		encodeCHAR8("DOC€0001")
	})
}

func TestEncodeCHAR8_TooLongPanics(t *testing.T) {
	// 9 characters cannot fit CHAR[8]: loud panic, never truncation.
	assertPanics(t, "encodeCHAR8 >8 characters", func() {
		encodeCHAR8("TOOLONG99")
	})
}

func TestBDT_EncodeUnmappableDocNamePanics(t *testing.T) {
	// Post-decode mutation to a name with no EBCDIC-037 mapping must fail
	// loudly at Encode, not substitute '?'.
	f, err := DecodeField(bdtHello)
	if err != nil {
		t.Fatalf("DecodeField(BDT) error: %v", err)
	}
	bdt := f.(*BDT)
	bdt.DocName = "DOC€0001"
	assertPanics(t, "BDT.Encode with unmappable DocName", func() {
		bdt.Encode()
	})
}

func TestBDT_EncodeTooLongDocNamePanics(t *testing.T) {
	// Post-decode mutation to a 9-character name must fail loudly at
	// Encode, not truncate to 8.
	f, err := DecodeField(bdtHello)
	if err != nil {
		t.Fatalf("DecodeField(BDT) error: %v", err)
	}
	bdt := f.(*BDT)
	bdt.DocName = "TOOLONG99"
	assertPanics(t, "BDT.Encode with >8-char DocName", func() {
		bdt.Encode()
	})
}
