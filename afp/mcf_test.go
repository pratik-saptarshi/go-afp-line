package afp

import (
	"errors"
	"reflect"
	"testing"
)

// EBCDIC-037 encodings of the coded font names used in these tests:
// "FIX-MONO-10" and "FIX-PROP-10". Both are 11 characters, so the X'02'
// triplet Tlength is 15 (0x0F): 1 Tlength + 1 Tid + 2 (FQNType/FQNFmt) +
// 11 name bytes.
var (
	fixMono10 = []byte{0xC6, 0xC9, 0xE7, 0x60, 0xD4, 0xD6, 0xD5, 0xD6, 0x60, 0xF1, 0xF0}
	fixProp10 = []byte{0xC6, 0xC9, 0xE7, 0x60, 0xD7, 0xD9, 0xD6, 0xD7, 0x60, 0xF1, 0xF0}
)

// mcfGroup builds one MCF repeating group: RGLength (UBIN[2], big-endian)
// followed by the supplied triplet bytes.
func mcfGroup(triplets ...[]byte) []byte {
	var g []byte
	for _, t := range triplets {
		g = append(g, t...)
	}
	out := make([]byte, 2, 2+len(g))
	out[0] = byte((len(g) + 2) >> 8)
	out[1] = byte(len(g) + 2)
	return append(out, g...)
}

// tripletX24 builds an X'24' (Resource Local ID) triplet: ResType byte and
// LID byte.
func tripletX24(resType, lid byte) []byte {
	return []byte{0x04, 0x24, resType, lid}
}

// tripletX02 builds an X'02' (Fully Qualified Name) triplet with the given
// FQNType and raw EBCDIC name bytes.
func tripletX02(fqnType byte, name []byte) []byte {
	out := []byte{byte(len(name) + 4), 0x02, fqnType, 0x00}
	return append(out, name...)
}

func assertDecodeKindStructure(t *testing.T, err error, what string) DecodeError {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: expected error, got nil", what)
	}
	var derr DecodeError
	if !errors.As(err, &derr) {
		t.Fatalf("%s: expected DecodeError, got %T: %v", what, err, err)
	}
	if derr.Kind != KindStructure {
		t.Fatalf("%s: expected KindStructure, got %q (reason: %s)", what, derr.Kind, derr.Reason)
	}
	return derr
}

func TestParseMCFMapping(t *testing.T) {
	data := append(
		mcfGroup(tripletX24(0x05, 0x01), tripletX02(0x8E, fixMono10)),
		mcfGroup(tripletX02(0x8E, fixProp10))...,
	)
	m, err := ParseMCFMapping(data)
	if err != nil {
		t.Fatalf("ParseMCFMapping: unexpected error: %v", err)
	}
	want := map[byte]string{1: "FIX-MONO-10", 0: "FIX-PROP-10"}
	if !reflect.DeepEqual(m.ByLID, want) {
		t.Errorf("ByLID: got %v, want %v", m.ByLID, want)
	}
	if len(m.Unsupported) != 0 {
		t.Errorf("Unsupported: got %v, want empty", m.Unsupported)
	}
}

func TestParseMCFMapping_FQNFmtOIDNotAName(t *testing.T) {
	// X'02' with FQNType X'8E' but FQNFmt X'10' (OID, not the
	// character-string format X'00'): the value bytes are not a character
	// string, so the group carries no coded-font name — LID 4 lands in
	// Unsupported and ByLID stays empty, with no error.
	data := mcfGroup(
		tripletX24(0x05, 0x04),
		[]byte{0x06, 0x02, 0x8E, 0x10, 0xC1, 0xC2},
	)
	m, err := ParseMCFMapping(data)
	if err != nil {
		t.Fatalf("ParseMCFMapping: unexpected error: %v", err)
	}
	if len(m.ByLID) != 0 {
		t.Errorf("ByLID: got %v, want empty (OID-format X'8E' is not a coded-font name)", m.ByLID)
	}
	if !reflect.DeepEqual(m.Unsupported, []byte{0x04}) {
		t.Errorf("Unsupported: got %v, want [4]", m.Unsupported)
	}
}

func TestParseMCFMapping_UnsupportedForm(t *testing.T) {
	// X'24' LID 3 with an X'02' FQN of a type other than X'8E'
	// (coded-font name): the LID cannot be mapped.
	data := mcfGroup(tripletX24(0x05, 0x03), tripletX02(0x84, fixMono10))
	m, err := ParseMCFMapping(data)
	if err != nil {
		t.Fatalf("ParseMCFMapping: unexpected error: %v", err)
	}
	if len(m.ByLID) != 0 {
		t.Errorf("ByLID: got %v, want empty", m.ByLID)
	}
	if !reflect.DeepEqual(m.Unsupported, []byte{0x03}) {
		t.Errorf("Unsupported: got %v, want [3]", m.Unsupported)
	}
}

func TestParseMCFMapping_DuplicateLID(t *testing.T) {
	t.Run("both mapped", func(t *testing.T) {
		data := append(
			mcfGroup(tripletX24(0x05, 0x01), tripletX02(0x8E, fixMono10)),
			mcfGroup(tripletX24(0x05, 0x01), tripletX02(0x8E, fixProp10))...,
		)
		_, err := ParseMCFMapping(data)
		derr := assertDecodeKindStructure(t, err, "duplicate LID in ByLID")
		if derr.Reason != "duplicate MCF local font ID 1; v1 does not support the MO:DCA p. 232 double-byte font exception" {
			t.Errorf("reason: got %q, want %q", derr.Reason, "duplicate MCF local font ID 1; v1 does not support the MO:DCA p. 232 double-byte font exception")
		}
	})
	t.Run("mapped then unsupported", func(t *testing.T) {
		data := append(
			mcfGroup(tripletX24(0x05, 0x03), tripletX02(0x8E, fixMono10)),
			mcfGroup(tripletX24(0x05, 0x03), tripletX02(0x84, fixProp10))...,
		)
		_, err := ParseMCFMapping(data)
		derr := assertDecodeKindStructure(t, err, "duplicate LID across ByLID/Unsupported")
		if derr.Reason != "duplicate MCF local font ID 3; v1 does not support the MO:DCA p. 232 double-byte font exception" {
			t.Errorf("reason: got %q, want %q", derr.Reason, "duplicate MCF local font ID 3; v1 does not support the MO:DCA p. 232 double-byte font exception")
		}
	})
}

func TestParseMCFMapping_DuplicateTripletInGroup(t *testing.T) {
	t.Run("second X'24'", func(t *testing.T) {
		data := mcfGroup(
			tripletX24(0x05, 0x01),
			tripletX24(0x05, 0x01),
			tripletX02(0x8E, fixMono10),
		)
		_, err := ParseMCFMapping(data)
		derr := assertDecodeKindStructure(t, err, "second X'24' in one repeating group")
		if derr.Reason != "MCF: duplicate X'24' triplet in one repeating group" {
			t.Errorf("reason: got %q, want %q", derr.Reason, "MCF: duplicate X'24' triplet in one repeating group")
		}
	})
	t.Run("second X'02'", func(t *testing.T) {
		data := mcfGroup(
			tripletX24(0x05, 0x01),
			tripletX02(0x8E, fixMono10),
			tripletX02(0x8E, fixProp10),
		)
		_, err := ParseMCFMapping(data)
		derr := assertDecodeKindStructure(t, err, "second X'02' with same FQNType in one repeating group")
		if derr.Reason != "duplicate FQN type 8E in one repeating group" {
			t.Errorf("reason: got %q, want %q", derr.Reason, "duplicate FQN type 8E in one repeating group")
		}
	})
}

func TestParseMCFMapping_BadResType(t *testing.T) {
	t.Run("wrong ResType", func(t *testing.T) {
		data := mcfGroup(tripletX24(0x06, 0x01), tripletX02(0x8E, fixMono10))
		_, err := ParseMCFMapping(data)
		assertDecodeKindStructure(t, err, "X'24' with ResType != 0x05")
	})
	t.Run("short X24 data", func(t *testing.T) {
		// Tlength 3: X'24' with a single data byte (needs ResType+LID).
		data := mcfGroup([]byte{0x03, 0x24, 0x05}, tripletX02(0x8E, fixMono10))
		_, err := ParseMCFMapping(data)
		assertDecodeKindStructure(t, err, "X'24' with 1-byte data")
	})
}

func TestParseMCFMapping_ShortFQN(t *testing.T) {
	// Tlength 3: X'02' with a single data byte (needs FQNType/FQNFmt).
	data := mcfGroup(tripletX24(0x05, 0x01), []byte{0x03, 0x02, 0x8E})
	_, err := ParseMCFMapping(data)
	assertDecodeKindStructure(t, err, "X'02' with 1-byte data")
}

func TestParseMCFMapping_Truncated(t *testing.T) {
	// RGLength claims 21 bytes but only 10 follow.
	data := []byte{0x00, 0x15, 0x04, 0x24, 0x05, 0x01, 0x0F, 0x02, 0x8E, 0x00, 0xC6, 0xC9}
	_, err := ParseMCFMapping(data)
	assertDecodeKindStructure(t, err, "RGLength overrunning data")
}

func TestParseMCFMapping_DuplicateUnsupported(t *testing.T) {
	// Two X'84' groups with the same LID: a repeat within Unsupported is
	// rejected fail-closed, like any other duplicate LID.
	data := append(
		mcfGroup(tripletX24(0x05, 0x03), tripletX02(0x84, fixMono10)),
		mcfGroup(tripletX24(0x05, 0x03), tripletX02(0x84, fixProp10))...,
	)
	_, err := ParseMCFMapping(data)
	derr := assertDecodeKindStructure(t, err, "duplicate LID within Unsupported")
	if derr.Reason != "duplicate MCF local font ID 3; v1 does not support the MO:DCA p. 232 double-byte font exception" {
		t.Errorf("reason: got %q, want %q", derr.Reason, "duplicate MCF local font ID 3; v1 does not support the MO:DCA p. 232 double-byte font exception")
	}
}

func TestParseMCFMapping_TruncatedHeader(t *testing.T) {
	t.Run("single byte", func(t *testing.T) {
		// A single trailing byte cannot hold a 2-byte repeating-group
		// header.
		data := []byte{0x00}
		_, err := ParseMCFMapping(data)
		assertDecodeKindStructure(t, err, "truncated repeating-group header")
	})
	t.Run("after valid group", func(t *testing.T) {
		// One valid group, then a dangling byte: the header check must
		// fire on later iterations too.
		data := append(mcfGroup(tripletX24(0x05, 0x01), tripletX02(0x8E, fixMono10)), 0x00)
		_, err := ParseMCFMapping(data)
		assertDecodeKindStructure(t, err, "truncated repeating-group header")
	})
}

func TestParseMCFMapping_EmptyGroup(t *testing.T) {
	// RGLength=2 is below the MO:DCA minimum of 7 (MCF syntax, p. 229):
	// an empty group is malformed, not a default-LID group.
	data := mcfGroup()
	_, err := ParseMCFMapping(data)
	assertDecodeKindStructure(t, err, "RGLength 2 empty group")
}

func TestParseMCFMapping_Empty(t *testing.T) {
	m, err := ParseMCFMapping(nil)
	if err != nil {
		t.Fatalf("ParseMCFMapping(nil): unexpected error: %v", err)
	}
	if m.ByLID == nil {
		t.Error("ByLID: want non-nil empty map, got nil")
	}
	if m.Unsupported == nil {
		t.Error("Unsupported: want non-nil empty slice, got nil")
	}
	if len(m.ByLID) != 0 || len(m.Unsupported) != 0 {
		t.Errorf("zero mapping: got %+v, want empty", m)
	}
}

func TestParseMCFMapping_FQNTypePair(t *testing.T) {
	// MO:DCA p. 230: X'85' (code page name) + X'86' (charset name) is a
	// mandatory minimum identification pair — two X'02' triplets with
	// *different* FQNTyp es in one repeating group. The group carries no
	// X'8E' coded-font name, so its LID lands in Unsupported, not ByLID.
	data := mcfGroup(tripletX24(0x05, 0x01), tripletX02(0x85, fixMono10), tripletX02(0x86, fixProp10))
	m, err := ParseMCFMapping(data)
	if err != nil {
		t.Fatalf("ParseMCFMapping: unexpected error: %v", err)
	}
	if len(m.ByLID) != 0 {
		t.Errorf("ByLID: got %v, want empty", m.ByLID)
	}
	if !reflect.DeepEqual(m.Unsupported, []byte{0x01}) {
		t.Errorf("Unsupported: got %v, want [1]", m.Unsupported)
	}
}

func TestParseMCFMapping_DuplicateFQNType(t *testing.T) {
	// Two X'02' triplets with the *same* FQNType in one group is malformed:
	// fail loudly rather than shadowing one name with the other.
	data := mcfGroup(tripletX24(0x05, 0x01), tripletX02(0x8E, fixMono10), tripletX02(0x8E, fixProp10))
	_, err := ParseMCFMapping(data)
	derr := assertDecodeKindStructure(t, err, "duplicate FQNType")
	if want := "duplicate FQN type 8E in one repeating group"; derr.Reason != want {
		t.Errorf("Reason: got %q, want %q", derr.Reason, want)
	}
}

func TestParseMCFMapping_DuplicateLIDDocumentsP232(t *testing.T) {
	// The blanket duplicate-LID rejection knowingly ignores MO:DCA p. 232's
	// exception for double-byte font sections (same X'24' across contiguous
	// groups): v1 does not implement double-byte fonts, so the diagnostic
	// must say so loudly instead of citing uniqueness alone.
	data := append(
		mcfGroup(tripletX24(0x05, 0x01), tripletX02(0x8E, fixMono10)),
		mcfGroup(tripletX24(0x05, 0x01), tripletX02(0x8E, fixProp10))...,
	)
	_, err := ParseMCFMapping(data)
	derr := assertDecodeKindStructure(t, err, "duplicate LID documents p. 232")
	if want := "duplicate MCF local font ID 1; v1 does not support the MO:DCA p. 232 double-byte font exception"; derr.Reason != want {
		t.Errorf("Reason: got %q, want %q", derr.Reason, want)
	}
}

func TestParseMCFMapping_TwoDefaultGroupsDuplicate(t *testing.T) {
	// Two X'24'-less groups both borrow the architected default LID 0: per
	// the reference each group maps a *unique* LID, so this is a duplicate,
	// not a merge. The duplicate rule applies to LID 0 uniformly.
	data := append(
		mcfGroup(tripletX02(0x8E, fixMono10)),
		mcfGroup(tripletX02(0x8E, fixProp10))...,
	)
	_, err := ParseMCFMapping(data)
	derr := assertDecodeKindStructure(t, err, "two default-LID groups")
	want := "duplicate MCF local font ID 0; v1 does not support the MO:DCA p. 232 double-byte font exception"
	if derr.Reason != want {
		t.Errorf("Reason: got %q, want %q", derr.Reason, want)
	}
}

// MO:DCA (MCF syntax, p. 229) defines RGLength range as 7–(n+1). A group
// with RGLength 6 is malformed — it is not a default-LID group.
func TestParseMCFMapping_RejectsShortRGLength(t *testing.T) {
	data := []byte{0x00, 0x06, 0x04, 0x24, 0x05, 0x01}
	_, err := ParseMCFMapping(data)
	assertDecodeKindStructure(t, err, "RGLength 6")
}

// MO:DCA (MCF semantics) forbids X'84', the X'85'/X'86' pair, and X'8E' in
// the same repeating group — they are mutually exclusive identification
// forms. A group mixing X'8E' with X'85' is malformed.
func TestParseMCFMapping_RejectsMixedFQNForms(t *testing.T) {
	data := mcfGroup(
		tripletX24(0x05, 0x01),
		tripletX02(0x8E, fixMono10),
		tripletX02(0x85, fixMono10),
	)
	_, err := ParseMCFMapping(data)
	assertDecodeKindStructure(t, err, "X'8E'+X'85' mixed forms")
}
