// Package afp implements a streaming MO:DCA structured-field decoder with
// byte-identical re-encoding for a contracted AFP subset.
//
// The decoder is strict by design: anything outside the contracted subset
// fails loudly with a typed DecodeError. See docs/subset-contract.md and
// docs/afp-file-format-spec.md for the subset definition and format reference.
package afp

import "fmt"

// DecodeError describes a failure to decode AFP structured-field data.
// FieldID is the hex SFID (e.g. "D3A8A8"); it is empty when the type ID
// could not be read. Offset is the byte offset in the decoded input where
// decoding failed. Reason is a human-readable cause.
type DecodeError struct {
	FieldID string
	Offset  int
	Reason  string
}

// Error implements the error interface.
func (e DecodeError) Error() string {
	if e.FieldID == "" {
		return fmt.Sprintf("afp: decode error at offset %d: %s", e.Offset, e.Reason)
	}
	return fmt.Sprintf("afp: decode error at offset %d (field %s): %s", e.Offset, e.FieldID, e.Reason)
}
