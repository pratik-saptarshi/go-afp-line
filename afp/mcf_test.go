package afp

import (
	"errors"
	"reflect"
	"strings"
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
		if !strings.Contains(derr.Reason, "duplicate MCF local font ID 1") {
			t.Errorf("reason %q does not name the duplicate LID", derr.Reason)
		}
	})
	t.Run("mapped then unsupported", func(t *testing.T) {
		data := append(
			mcfGroup(tripletX24(0x05, 0x03), tripletX02(0x8E, fixMono10)),
			mcfGroup(tripletX24(0x05, 0x03), tripletX02(0x84, fixProp10))...,
		)
		_, err := ParseMCFMapping(data)
		derr := assertDecodeKindStructure(t, err, "duplicate LID across ByLID/Unsupported")
		if !strings.Contains(derr.Reason, "duplicate MCF local font ID 3") {
			t.Errorf("reason %q does not name the duplicate LID", derr.Reason)
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
