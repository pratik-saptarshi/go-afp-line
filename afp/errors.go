// Package afp implements a streaming MO:DCA structured-field decoder with
// byte-identical re-encoding for a contracted AFP subset.
//
// The decoder is strict by design: anything outside the contracted subset
// fails loudly with a typed DecodeError. See docs/subset-contract.md and
// docs/afp-file-format-spec.md for the subset definition and format reference.
package afp

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// ErrorKind classifies a DecodeError so callers can react programmatically
// instead of matching on Reason text.
type ErrorKind string

// DecodeError kinds. Each construction site sets exactly one.
const (
	// KindStructure: the input is malformed at the wire level — truncated
	// introducers, fields, triplets or control sequences; invalid or
	// mismatched lengths; broken document structure (unpaired begin/end
	// fields, groups crossing boundaries, unclosed groups at end of
	// stream); re-encode mismatch in the round-trip gate.
	KindStructure ErrorKind = "structure"

	// KindEncoding: a character-encoding failure — an unsupported code page
	// (LookupCodePage), or text that cannot be represented in the v1 target
	// encoding (C1 controls rejected by WinAnsi policy).
	KindEncoding ErrorKind = "encoding"

	// KindBudget: a resource budget was exceeded — the decoded-text budget
	// (RPS amplification guard). The stream was well-formed; it was simply
	// too large to decode safely.
	KindBudget ErrorKind = "budget"

	// KindValidation: the input is well-formed but violates the contracted
	// subset or reducer policy — unknown field type, unknown triplet ID,
	// unknown PTOCA control sequence, SCFL font ID with no pinned metrics
	// mapping, or a non-RawField / non-PTX argument to DecodePTOCA.
	// (Placement violations such as an environment field outside a document
	// are KindStructure: broken document structure.)
	KindValidation ErrorKind = "validation"
)

// DecodeError describes a failure to decode AFP structured-field data.
// FieldID is the hex SFID (e.g. "D3A8A8"); it is empty when the type ID
// could not be read. Offset is the byte offset in the decoded input where
// decoding failed. Kind classifies the failure (see ErrorKind). Reason is a
// human-readable cause; user-influenced text echoed into it must be wrapped
// in truncateEcho.
// truncateEcho at maxEchoRunes runes.
type DecodeError struct {
	FieldID string
	Offset  int
	Kind    ErrorKind
	Reason  string
}

// Error implements the error interface.
func (e DecodeError) Error() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "afp: decode error at offset %d", e.Offset)
	if e.Kind != "" {
		fmt.Fprintf(&sb, " [%s]", e.Kind)
	}
	if e.FieldID != "" {
		fmt.Fprintf(&sb, " (field %s)", e.FieldID)
	}
	sb.WriteString(": " + e.Reason)
	return sb.String()
}

// maxEchoRunes caps user-influenced text echoed into a DecodeError Reason.
const maxEchoRunes = 80

// truncateEcho caps user-influenced text echoed into error reasons at
// maxEchoRunes runes, appending "…" when truncated. It prevents a hostile
// stream from bloating error strings with megabytes of echoed payload.
// Cutting on rune boundaries keeps the result valid UTF-8.
func truncateEcho(s string) string {
	if utf8.RuneCountInString(s) <= maxEchoRunes {
		return s
	}
	return string([]rune(s)[:maxEchoRunes]) + "…"
}
