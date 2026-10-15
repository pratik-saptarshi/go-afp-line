package afp

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestErrorKind_DocumentedValues pins the ErrorKind taxonomy: the values are
// part of the error contract, so programmatic consumers can rely on them.
func TestErrorKind_DocumentedValues(t *testing.T) {
	cases := []struct {
		kind ErrorKind
		want string
	}{
		{KindStructure, "structure"},
		{KindEncoding, "encoding"},
		{KindBudget, "budget"},
		{KindValidation, "validation"},
	}
	for _, c := range cases {
		if string(c.kind) != c.want {
			t.Errorf("ErrorKind = %q, want documented value %q", string(c.kind), c.want)
		}
	}
}

// TestDecodeError_KindsOnRepresentativeErrors: every DecodeError carries the
// Kind matching its failure class.
func TestDecodeError_KindsOnRepresentativeErrors(t *testing.T) {
	// structure: a truncated introducer is malformed wire data.
	if _, err := DecodeIntroducer([]byte{0x00}); err == nil {
		t.Fatal("DecodeIntroducer(truncated): got nil error")
	} else {
		derr, ok := err.(DecodeError)
		if !ok {
			t.Fatalf("DecodeIntroducer(truncated): error = %T, want DecodeError", err)
		}
		if derr.Kind != KindStructure {
			t.Errorf("truncated introducer: Kind = %q, want %q", derr.Kind, KindStructure)
		}
	}

	// encoding: an unregistered code page is a character-encoding failure.
	if _, err := LookupCodePage(1141); err == nil {
		t.Fatal("LookupCodePage(1141): got nil error")
	} else {
		derr, ok := err.(DecodeError)
		if !ok {
			t.Fatalf("LookupCodePage(1141): error = %T, want DecodeError", err)
		}
		if derr.Kind != KindEncoding {
			t.Errorf("unknown code page: Kind = %q, want %q", derr.Kind, KindEncoding)
		}
	}

	// validation: a well-formed field of unknown type violates the
	// contracted subset.
	if _, err := DecodeField(unknownField); err == nil {
		t.Fatal("DecodeField(unknown type): got nil error")
	} else {
		derr, ok := err.(DecodeError)
		if !ok {
			t.Fatalf("DecodeField(unknown type): error = %T, want DecodeError", err)
		}
		if derr.Kind != KindValidation {
			t.Errorf("unknown field type: Kind = %q, want %q", derr.Kind, KindValidation)
		}
	}
}

// TestDecodeError_ErrorIncludesKind: the human-readable form names the kind
// so a bare log line is still classifiable.
func TestDecodeError_ErrorIncludesKind(t *testing.T) {
	e := DecodeError{FieldID: "D3A8A8", Offset: 12, Kind: KindBudget, Reason: "decoded text budget exceeded"}
	s := e.Error()
	for _, want := range []string{"12", "D3A8A8", "budget", "decoded text budget exceeded"} {
		if !strings.Contains(s, want) {
			t.Errorf("Error() = %q, want it to contain %q", s, want)
		}
	}
}

// TestDecodeError_ErrorWithoutKind: a zero-value Kind (only possible in
// synthetic constructions) degrades gracefully instead of printing an empty
// bracket.
func TestDecodeError_ErrorWithoutKind(t *testing.T) {
	s := DecodeError{Offset: 3, Reason: "x"}.Error()
	if strings.Contains(s, "[]") || strings.Contains(s, "()") {
		t.Errorf("Error() = %q, want no empty kind/field markers", s)
	}
}

// TestTruncateEcho: user-influenced text echoed into error reasons is capped
// at MaxEchoRunes runes so a hostile stream cannot bloat error strings.
func TestTruncateEcho(t *testing.T) {
	if MaxEchoRunes != 80 {
		t.Errorf("MaxEchoRunes = %d, want 80 (the documented echo cap)", MaxEchoRunes)
	}
	short := "medium map name"
	if got := TruncateEcho(short); got != short {
		t.Errorf("TruncateEcho(%q) = %q, want unchanged", short, got)
	}
	exact := strings.Repeat("a", 80)
	if got := TruncateEcho(exact); got != exact {
		t.Errorf("TruncateEcho(80 runes) changed the input: %q", got)
	}
	long := strings.Repeat("b", 200)
	got := TruncateEcho(long)
	if n := utf8.RuneCountInString(got); n != 81 { // 80 kept + ellipsis
		t.Errorf("TruncateEcho(200 runes) = %d runes, want 81 (80 + ellipsis)", n)
	}
	if !strings.HasPrefix(got, strings.Repeat("b", 80)) {
		t.Errorf("TruncateEcho(200 runes) does not keep the first 80 runes: %q", got)
	}
	// Rune-boundary safety: 100 two-byte runes must not be split mid-rune.
	wide := strings.Repeat("é", 100)
	got = TruncateEcho(wide)
	if !utf8.ValidString(got) {
		t.Errorf("TruncateEcho split a multi-byte rune: %q", got)
	}
	if n := utf8.RuneCountInString(got); n != 81 {
		t.Errorf("TruncateEcho(100 wide runes) = %d runes, want 81", n)
	}
}
