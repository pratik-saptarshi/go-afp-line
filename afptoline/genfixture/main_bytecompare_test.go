package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestFixtures_ByteCompareGeneratorOutput (Task C6): the committed fixture
// files in ../fixtures must be byte-identical to the generator's current
// output — proof that the fixtures are exactly what the generator emits,
// with no hand edits drifting them apart.
//
// Nondeterminism audit (2026-10-15): the builders are deterministic by
// construction — they assemble only hand-written byte slices and fixed
// package-var initialization order. There is no map iteration (ebcdic037 is
// read by lookup only), no time.Now, no random IDs, no goroutines.
// TestGenerator_Deterministic already pins repeat-call determinism; this
// test pins the on-disk artifacts against the builders.
func TestFixtures_ByteCompareGeneratorOutput(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate package directory")
	}
	dir := filepath.Join(filepath.Dir(thisFile), "..", "fixtures")

	builders := []struct {
		name  string
		build func() []byte
	}{
		{"statement-001", buildStatement001},
		{"exclusion-goca", buildExclusionGOCA},
		{"medium-ref", buildMediumRef},
	}
	for _, b := range builders {
		t.Run(b.name, func(t *testing.T) {
			got := b.build()
			want, err := os.ReadFile(filepath.Join(dir, b.name+".afp"))
			if err != nil {
				t.Fatalf("read committed fixture: %v", err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("generator output differs from committed %s.afp (%d vs %d bytes)",
					b.name, len(got), len(want))
			}
		})
	}
}
