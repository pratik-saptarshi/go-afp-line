package afptoline

import (
	"bytes"
	"errors"
	"testing"

	"github.com/pratik-saptarshi/go-afp-line/afp"
	"github.com/pratik-saptarshi/go-afp-line/line"
)

// fuzzBudget is the decode-text budget enforced inside the fuzzers. It is
// deliberately small so the fail-fast budget path (not just the parser) gets
// exercised by mutated inputs.
const fuzzBudget = 4096

// decodedTextBytes sums the UTF-8 bytes of every run's text — the same unit
// the budget accounts in (see ptocaState.text and reduceState.textLeft).
func decodedTextBytes(runs []Run) int {
	total := 0
	for _, r := range runs {
		total += len(r.Text)
	}
	return total
}

func documentTextBytes(doc line.Document) int {
	total := 0
	for _, p := range doc.Pages {
		for _, r := range p.Runs {
			total += len(r.Text)
		}
	}
	return total
}

// maxBytesPerRune returns the maximum UTF-8 bytes any single source byte of
// cp decodes to. The decoder's fail-fast budget counts source bytes:
// ptocaState.text rejects a segment with len(b) > remaining before appending,
// so decoded UTF-8 text can exceed the budget by at most this factor. The
// exact UTF-8 accounting lives one layer up: visitPTX rejects a PTX whose
// decoded runs exceed the remaining document budget (see reduce.go).
func maxBytesPerRune(cp afp.CodePage) int {
	max := 1
	for i := 0; i < 256; i++ {
		if n := len(cp.Decode([]byte{byte(i)})); n > max {
			max = n
		}
	}
	return max
}

// FuzzDecodePTOCA fuzzes the PTOCA decoder over arbitrary payload bytes,
// wrapped as a PTX field the way the reducer supplies it.
//
// Invariants (Task C8):
//   - no panic on any input;
//   - the fail-fast decode budget is respected: a successful decode yields at
//     most budget*maxBytesPerRune decoded UTF-8 bytes (the decoder budgets
//     source bytes; each source byte decodes to at most maxBytesPerRune
//     UTF-8 bytes, so this is the tight observable bound);
//   - every returned error is an afp.DecodeError (fail loudly, typed).
//
// Seed corpus: testdata/fuzz/FuzzDecodePTOCA/ holds PTX payloads extracted
// from the checked-in fixtures plus seed-multibyte-expansion, a minimized
// fuzzer find pinning the source-byte/UTF-8 accounting above; the inline
// seeds below pin the degenerate cases.
func FuzzDecodePTOCA(f *testing.F) {
	cp, err := afp.LookupCodePage(37)
	if err != nil {
		f.Fatalf("LookupCodePage(37): %v", err)
	}
	maxOut := fuzzBudget * maxBytesPerRune(cp)

	f.Add([]byte{})                                               // empty payload
	f.Add([]byte("A"))                                            // bare text, no control sequences
	f.Add([]byte{0x2B})                                           // truncated control-sequence introducer
	f.Add([]byte{0x2B, 0xD3, 0x04, 0xDA})                         // TRN sequence with no data
	f.Add([]byte{0x2B, 0xD3, 0xFF, 0xDA})                         // TRN length overruns the payload
	f.Add([]byte{0x2B, 0xD3, 0x06, 0xEE, 0x00, 0x00, 0x00, 0x00}) // RPS with zero RLENGTH
	f.Add([]byte{0x2B, 0xD3, 0x05, 0xEE, 0xFF, 0xFF, 'X'})        // RPS repeating one byte to 64 KiB
	f.Add([]byte{0xD3})                                           // chained-class byte without introducer

	f.Fuzz(func(t *testing.T, b []byte) {
		runs, err := decodePTOCA(ptxField(b), cp, fuzzBudget, nil)
		if err != nil {
			var derr afp.DecodeError
			if !errors.As(err, &derr) {
				t.Fatalf("non-DecodeError returned: %T (%v)", err, err)
			}
			return
		}
		if got := decodedTextBytes(runs); got > maxOut {
			t.Fatalf("budget violated: decoded %d UTF-8 bytes with source-byte budget %d (max %d)",
				got, fuzzBudget, maxOut)
		}
	})
}

// FuzzReduce fuzzes the full reduction pipeline over arbitrary byte streams.
//
// Invariants (Task C8):
//   - no panic on any input;
//   - the document text budget is respected: a successful reduction yields
//     at most DefaultMaxTextBytes decoded text bytes.
//
// Seed corpus: testdata/fuzz/FuzzReduce/ holds the three checked-in fixture
// documents whole.
func FuzzReduce(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0x00, 0x08, 0xD3, 0xA8, 0xA8, 0x00, 0x00, 0x00}) // naked BDT introducer, no data

	f.Fuzz(func(t *testing.T, b []byte) {
		doc, _, err := Reduce(bytes.NewReader(b))
		if err != nil {
			return // malformed input fails loudly; only panics are bugs
		}
		if got := documentTextBytes(doc); got > DefaultMaxTextBytes {
			t.Fatalf("budget violated: reduced %d text bytes with budget %d", got, DefaultMaxTextBytes)
		}
	})
}
