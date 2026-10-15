# Writer spike — PDF writer selection

**Decision: pdfcpu v0.16.2.** Loser: gofpdf v1.16.2 (unmaintained — author-closed, last release 2019).

## Rubric and result

Decision rubric (plan Task 0 Step 3): first candidate passing all five criteria wins; ties break
toward the smaller dependency footprint; record the loser and why.

| # | Criterion | pdfcpu v0.16.2 | gofpdf v1.16.2 |
|---|-----------|----------------|----------------|
| 1 | Arbitrary (x,y) text placement | PASS | PASS |
| 2 | Font embedding with declared metrics | PASS (standard-14 references; sizes verified in content stream) | PASS (same mechanism) |
| 3 | Read-back geometry within ±1pt | PASS — 0.000pt deltas | PASS — 0.000pt deltas |
| 4 | License + transitive deps clean | PASS — Apache-2.0, no GPL/AGPL anywhere in graph | PASS — MIT, zero deps |
| 5 | Maintenance posture | PASS — v0.16.2 tagged 2026-10-11, very active | **FAIL** — last release 2019-11-19, README "We Are Closed", Snyk: Inactive |

The tie-break rule did not apply: criterion 5 is a decisive fail for gofpdf, not a tie.
For a long-lived library, an abandoned writer dependency is the dominant risk.

## Method

Spike code and measurement evidence: `docs/_evidence/writerspike/` (`main.go`,
`gofpdf.bbox.xml`, `pdfcpu.bbox.xml`).
<!-- Evidence note: the spike's former nested go.mod/go.sum were removed (de-modulized);
     the source and measurement evidence is retained. -->

Each candidate generated a one-page Letter PDF with two text runs:
- "HELLO", Helvetica 12pt, baseline origin (100, 92) in PDF user space (bottom-left origin)
- "WORLD", Courier 10pt, baseline origin (300, 292)

Raw content streams verified byte-level placement: gofpdf emitted
`BT 100.00 92.00 Td (HELLO) Tj ET` / `BT 300.00 292.00 Td (WORLD) Tj ET`;
pdfcpu (raw `Tm`/`Tj` content stream via `model.NewPage` + `create.UpdatePageTree` +
`api.CreatePDFFile` — the same machinery pdfcpu's own watermark code uses) emitted
`1 0 0 1 100.0 92.0 Tm` / `1 0 0 1 300.0 292.0 Tm`.

**Independent reader:** poppler `pdftotext -bbox` (+ `pdffonts`, `pdfinfo`).
Independence justification: poppler is a separate C++ implementation sharing no code
with either Go candidate, so measured geometry is a true third-party observation.

Measured word boxes (both files, identical to 3 decimals):
- HELLO: xMin=100.000 yMin=691.384 xMax=139.348 yMax=702.484
- WORLD: xMin=300.000 yMin=493.710 xMax=330.000 yMax=501.570

Expected from published AFM metrics (not fed to either writer):
HELLO yMin = 792−(92+0.718×12) = 691.384 ✓; WORLD yMin = 792−(292+0.629×10) = 493.710 ✓.
xMin deltas 0.000pt for both runs, both writers. Pixel-exact.

**Fonts** (`pdffonts`, both files): Helvetica / Courier, Type 1, WinAnsi, `emb=no`.
Neither embedded the standard-14 fonts — each candidate's documented standard-font
mechanism, not a failure. Declared sizes (12.0/10.0 Tf) verified in content streams.

**Dependency audit** (`go list -deps`, writer path):
- gofpdf: itself only (MIT) — zero transitive deps.
- pdfcpu: itself (Apache-2.0) + clipperhouse/uax29/v2 (MIT), hhrutter/tiff (BSD),
  mattn/go-runewidth (MIT), xdg-go/stringprep (Apache-2.0), go.yaml.in/yaml/v3
  (MIT+Apache), golang.org/x/{crypto,image,text} (BSD-3). Full 29-module graph
  checked: all MIT/BSD/ISC/Apache, no GPL/AGPL anywhere.

**Maintenance:** pdfcpu v0.16.2 (2026-10-11); v0.16.1 ~Oct 4 carried security fixes
(GHSA-6524-w46v-6399 et al.); v0.16.0 ~Sep 28. gofpdf v1.16.2 (2019-11-19), unmaintained.

## Caveats and follow-ups

- **Real font embedding** (non-standard-14 fonts, which AFP fidelity will need) was NOT
  tested — both candidates used non-embedded standard-14 references. Recommended
  follow-up: spike pdfcpu's user-font embedding path before the line→PDF task
  depends on it.
- **pdfcpu writer ergonomics are lower-level** than gofpdf's `Text(x,y)` one-liner;
  the library will wrap the raw `Tm`/`Tj` content-stream construction. Real cost, accepted.
- Consolidation bonus: pdfcpu also serves as the project's PDF reader/validator, so
  the module converges on one actively-maintained dependency instead of two.
