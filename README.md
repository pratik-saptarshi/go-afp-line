# go-afp-line

[![CI](https://github.com/pratik-saptarshi/go-afp-line/actions/workflows/ci.yml/badge.svg)](https://github.com/pratik-saptarshi/go-afp-line/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/pratik-saptarshi/go-afp-line.svg)](https://pkg.go.dev/github.com/pratik-saptarshi/go-afp-line)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

A native Go library that reduces **AFP (MO:DCA)** print streams to an intermediate
**line-data** model and renders **untagged PDF** from it — with byte-identical
round-trip fidelity for a contracted AFP subset.

The core idea is *reduce-first*: instead of converting AFP structures directly to
PDF drawing operations, the library first reduces AFP content to a simple,
well-defined line-data model (pages → positioned text runs), then renders PDF from
that model. Reduction loses information by design; anything outside the contracted
subset fails loudly with a diagnostic — never silently.

## Features

- **Streaming MO:DCA structured-field decoder** — 8-byte introducer codec, `5A`-wrapped
  and naked record framing, table-driven field registry, triplet codec.
- **Byte-identical round-trip** — `afp.RoundTrip` proves parse → re-encode fidelity;
  unknown-but-well-formed fields pass through verbatim via `RawField`.
- **Explicit exclusion policy** — known-but-out-of-subset fields (GOCA, BCOCA, IOCA,
  overlays, …) become `line.Exclusion` diagnostics; malformed input aborts with a
  typed `DecodeError`. Nothing is silently dropped.
- **PTOCA control-sequence state machine** — 19 control sequences with per-sequence
  unit tests; wire formats per the AFP subset contract (`docs/subset-contract.md`).
- **Dual code-page support** — EBCDIC 037 and 500 registries with full decode tables.
- **Conservation law** — every PTOCA text byte is accounted for as placed,
  suppressed-with-reason, or excluded-with-diagnostic.
- **Untagged PDF output** — rendered with pdfcpu, verified geometry via an
  independent reader (poppler), producer stamped "untagged; not accessible".
- **Fuzz-tested parsers** — `FuzzDecodeIntroducer` / `FuzzDecodeField` seed corpora;
  fuzzing caught real bugs before release.
- **Permissive dependency tree** — Apache-2.0/MIT/BSD/ISC only; no GPL/AGPL anywhere.

## Architecture

```
AFP bytes ──▶ afp/ ──▶ afptoline/ ──▶ line/ ──▶ linetopdf/ ──▶ PDF bytes
              │            │              │            │
              │  decode +  │  PTOCA state │  Document  │  pdfcpu
              │  round-    │  machine +   │  Page      │  positioned
              │  trip gate │  reduction   │  TextRun   │  text
```

| Package | Responsibility |
|---|---|
| `afp` | Streaming structured-field decoder: introducer codec, framing splitter, field registry, triplets, code pages, `RoundTrip` gate |
| `line` | Intermediate line-data model: `Document`/`Page`/`TextRun`, pinned `FontMetrics`, `Exclusion` diagnostics |
| `afptoline` | AFP → line-data reduction: PTOCA state machine, medium resolution, reduction walk with conservation law, fixture generator |
| `linetopdf` | Line-data → untagged PDF rendering via pdfcpu |

## Installation

Requires Go 1.27+.

```bash
go get github.com/pratik-saptarshi/go-afp-line@latest
```

## Quick start

```go
package main

import (
	"fmt"
	"os"

	"github.com/pratik-saptarshi/go-afp-line/afp"
	"github.com/pratik-saptarshi/go-afp-line/afptoline"
	"github.com/pratik-saptarshi/go-afp-line/linetopdf"
)

func main() {
	f, err := os.Open("statement.afp")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	// Tier 1 gate: the stream must round-trip byte-identically.
	if err := afp.RoundTrip(f); err != nil {
		panic(fmt.Sprintf("not a clean AFP stream: %v", err))
	}
	if _, err := f.Seek(0, 0); err != nil {
		panic(err)
	}

	// Reduce AFP to line data. Out-of-subset content arrives
	// as exclusions — inspect them, don't ignore them.
	doc, exclusions, err := afptoline.Reduce(f)
	if err != nil {
		panic(err)
	}
	for _, x := range exclusions {
		fmt.Println("excluded:", x.Diagnostic())
	}

	// Render untagged PDF.
	out, err := os.Create("statement.pdf")
	if err != nil {
		panic(err)
	}
	defer out.Close()
	if err := linetopdf.Render(doc, out); err != nil {
		panic(err)
	}
}
```

## The contracted subset (v1)

The library deliberately supports a tight subset of MO:DCA. The full contract lives
in [`docs/subset-contract.md`](docs/subset-contract.md); the distilled file-format
reference is [`docs/afp-file-format-spec.md`](docs/afp-file-format-spec.md).

**In scope:** BDT/EDT, BPG/EPG, BNG/ENG, BAG/EAG, BPT/EPT, PTX (PTOCA text),
MCF, PTD; 45 triplets; code pages 037 and 500; naked and `5A`-wrapped framing.

**Excluded (loud diagnostics):** GOCA vector graphics, BCOCA barcodes, IOCA images,
CMOCA color management, FOCA fonts, overlays, FormDef/PageDef carriers, DBCS.

**Output is untagged PDF only.** This library makes no accessibility claims; the
producer metadata states "untagged; not accessible".

## Testing

```bash
go test ./...            # full suite
go test -race ./...      # with race detector
go vet ./...             # static analysis
gofmt -l .               # formatting (must be empty)

# Fuzz the binary parsers (AFP decoders are classic fuzz targets):
go test -fuzz=FuzzDecodeIntroducer -fuzztime=60s ./afp/
go test -fuzz=FuzzDecodeField -fuzztime=60s ./afp/
```

### Test dependency: poppler (`pdftotext`)

The PDF geometry tests in `linetopdf` (`TestRender_Positions`,
`TestRender_WidthMatchesMetrics`, `TestRender_Untagged`) render a fixture to PDF
and read back the word positions with `pdftotext -bbox` — poppler acts as the
independent reader (it shares no code with the pdfcpu writer). You need
`pdftotext` on your PATH before running the full suite.

Install it:

```bash
sudo apt-get install -y poppler-utils   # Debian/Ubuntu
brew install poppler                    # macOS
```

This mirrors CI: `.github/workflows/ci.yml` installs `poppler-utils` via
`sudo apt-get update && sudo apt-get install -y poppler-utils` on
`ubuntu-latest` before running `go test -race -shuffle=on ./...`.

If `pdftotext` is missing, the tests that need it **skip** locally. With
`CI=true` in the environment, they **fail fast** instead (`t.Fatal`), so CI
never silently passes with the independent-reader checks disabled.

CI runs `gofmt`, `go mod tidy -diff`, `go vet`, and `go test -race` on every push.

## Repository layout

```
.
├── afp/                  # MO:DCA decoder (sfi, field registry, triplets, code pages)
├── line/                 # line-data model + font metrics + exclusions
├── afptoline/            # PTOCA state machine, medium resolution, reduction walk
│   ├── fixtures/         # hand-authored synthetic AFP fixtures (spec-derived, not generated)
│   └── genfixture/       # fixture generator (assembles hand-authored hex)
├── linetopdf/            # untagged PDF renderer (pdfcpu)
├── docs/
│   ├── afp-file-format-spec.md   # distilled MO:DCA + MOCA file-format reference
│   ├── subset-contract.md        # Phase 0 decision record: the contracted subset
│   └── writer-spike.md           # pdfcpu-vs-gofpdf evaluation (pdfcpu won)
└── .github/workflows/ci.yml
```

## Design principles

1. **Test-first.** Every feature lands via red/green TDD; no implementation code before a failing test.
2. **Loud failures.** Malformed input aborts with typed errors; out-of-subset content becomes named diagnostics. Silent drops are bugs.
3. **Independent verification.** Geometry is read back with poppler, not the writer; fixtures are hand-authored hex, not library output; the round-trip gate proves the codec, the conservation law proves the reduction.
4. **License hygiene.** No GPL/AGPL in the dependency tree, ever.

## References

- [MO:DCA Reference (AFP Consortium)](https://afpcinc.org/wp-content/uploads/2017/12/MODCA-Reference-09.pdf)
- [MOCA Reference (AFPC-0013-01)](https://afpcinc.org/wp-content/uploads/2016/08/MOCA-Reference-Metadata-Object-Content-Architecture-Reference.pdf)
- [afplib](https://github.com/yan74/afplib) (Java, Apache-2.0) — parsing reference
- [openafp](https://github.com/audreyt/openafp) (Haskell, CC0) — field tables
- [parse-afp](https://github.com/audreyt/parse-afp) (Perl, CC0) — round-trip model

## Status

v1 implements the contracted subset above. See
[`docs/subset-contract.md`](docs/subset-contract.md) for the Phase 4 fidelity gate
criteria and the honest list of what's deferred (real font embedding, IOCA image
subset, Alpheus oracle comparison pending license review).

## License

Apache License 2.0 — see [LICENSE](LICENSE).
