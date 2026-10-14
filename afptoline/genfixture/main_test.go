package main

import (
	"bytes"
	"testing"

	"github.com/pratik-saptarshi/go-afp-line/afp"
)

// TestGenerator_Deterministic is the Task 7 acceptance gate, written before
// the builders (TDD step 1). It pins two properties of the hand-authored
// fixture corpus:
//
//  1. Determinism: every builder must produce byte-identical output on
//     repeated calls, so fixtures are stable corpus artifacts.
//  2. Tier 1 fidelity: afp.RoundTrip (split → decode → re-encode →
//     byte-compare) must accept every fixture. RoundTrip is used here ONLY
//     as a check; the fixture bytes are hand-authored per the spec and
//     never produced by the afp package.
func TestGenerator_Deterministic(t *testing.T) {
	builders := []struct {
		name  string
		build func() []byte
	}{
		{"statement-001", buildStatement001},
		{"exclusion-goca", buildExclusionGOCA},
		{"medium-ref", buildMediumRef},
	}
	for _, b := range builders {
		first, second := b.build(), b.build()
		if !bytes.Equal(first, second) {
			t.Errorf("%s: builder not deterministic: two calls differ (%d vs %d bytes)",
				b.name, len(first), len(second))
		}
		if err := afp.RoundTrip(bytes.NewReader(first)); err != nil {
			t.Errorf("%s: afp.RoundTrip: %v", b.name, err)
		}
	}
}
