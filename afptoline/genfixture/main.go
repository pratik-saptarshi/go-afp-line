package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// main emits the hand-authored fixture corpus to ../fixtures/<name>.afp
// (relative to this package's directory). The fixtures themselves are
// assembled in fixtures.go from hand-written byte slices; this command only
// persists them to disk so downstream tasks have stable corpus artifacts.
func main() {
	fixtures := []struct {
		name  string
		build func() []byte
	}{
		{"statement-001", buildStatement001},
		{"exclusion-goca", buildExclusionGOCA},
		{"medium-ref", buildMediumRef},
	}

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		fatal("genfixture: cannot locate package directory")
	}
	dir := filepath.Join(filepath.Dir(thisFile), "..", "fixtures")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fatal("genfixture: mkdir %s: %v", dir, err)
	}

	for _, f := range fixtures {
		b := f.build()
		path := filepath.Join(dir, f.name+".afp")
		if err := os.WriteFile(path, b, 0o644); err != nil {
			fatal("genfixture: write %s: %v", path, err)
		}
		fmt.Printf("wrote %s (%d bytes)\n", path, len(b))
	}
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
