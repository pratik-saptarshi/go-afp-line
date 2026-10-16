# Round 2 adversarial panel verdict — go-afp-line `fix/afp-panel-p0` @ d876cdd

Source: Claude agent session (branch `claude/adversarial-architecture-panel-b0zgc9`), pasted 2026-10-15 ~06:48 CDT.
Note: the panel ran against d876cdd (pre-merge head). `main` had been history-rewritten to drop the IBM spec extracts; the fix branch was a clean fast-forward of it. A judge checked claims with probes. The agent changed nothing and did not touch the designated branch. Tests, vet and `-race` pass on the fix branch.

**Verdict: hold the merge.** The code fixes are sound and the judge found no regressions. The branch still ships files that shouldn't be public, and it leaves open a pairing bug of the same kind it was meant to fix.

## Round 1 P0 fixes

| Fix | Status |
|---|---|
| CQ-1: BAG/EAG inside a page | Fixed |
| CQ-2: RPS repeat semantics | Fixed, but see the new RPS memory bug below |
| CQ-4: WinAnsi encoding | Fixed. The font dict carries `/Encoding /WinAnsiEncoding`, and "Œ€é•ÿ" round-trips through `pdftotext` |
| CQ-5: +Inf in `Validate` | Fixed |
| TEST-1: poppler in CI | Fixed. The helper now fails instead of skipping when `CI=true` |

## Merge-blockers for this branch

1. **Agent plan files leak (BP2-1, BP2-2).** The agent plan files (since moved to `docs/process/plans/`) contain "REQUIRED SUB-SKILL" headers and a local home-directory path with a username. One also quotes an IBM sentence and cites a removed file. Remove them (S).
2. **Dangling and overclaiming docs (BP2-3, BP2-4, BP-3).**
   - `docs/afp-file-format-spec.md:10` cites PDFs that no longer exist.
   - `docs/_evidence/README.md` is leftover process notes.
   - `README.md:27` says wire formats are "verified against the IBM PTOCA Reference", but the repo has no spec text and the contract marks the IBM section references TBD.

   Delete the stale references and reword the README (S).
3. **BPT/EPG pairing hole (ARCH2-1, CQ2-5, CQ-6).** `BDT BPG BPT EPG BPG EPT EPG EDT` is accepted. EPG checks the BAG counter but not the BPT counter. Make EPG require `bpt == 0`, check ENG/EAG/EPT nesting, and add tests (S).
4. **RPS amplification DoS (CQ2-1).**
   - A 7-byte sequence expands to 65,535 bytes.
   - The judge measured about 200 MB of text, 6 seconds and 1.7 GB allocated from a single 32 KB field.
   - The cost multiplies across fields, so this is reachable from `Reduce` on untrusted input.
   - The panelist's "63 KB PTX" figure was impossible, because field length is capped at 32,767.
   - Fix: a decoded-text budget per PTX and per document. The judge suggests 1 MiB per document by default, configurable, with the error carrying the offset (S–M).
5. **Lock in escaping and WinAnsi behavior with tests (TEST2-1 to 3, 5).**
   - Six WinAnsi mutations survive the current suite, including the paren/backslash escape, control-byte octal escapes, the Œ mapping and the 0xFF boundary.
   - The judge overruled the panel's "content-stream injection" claim: escaping is correct and `a) Tj ET …` round-trips as plain text.
   - So this is a test gap, not an exploit. Add tests for `( ) \`, control bytes, 0x7F, the C1 boundary, and NaN/-Inf on dimensions (S).

## P0 for the next PR

- **C1 characters (ARCH2-2, CQ2-2, CQ2-3).** `Reduce` accepts EBCDIC bytes that decode to U+0080–U+009F, then `Render` fails late with no AFP offset. The judge's policy is to reject, not map. Add one shared "encodable" check in `line.TextRun.Validate` so the failure happens in `Reduce` with the offset (S).
- **Coordinate bounds (CQ2-4).** 1e300 passes `Validate` and emits 300-digit numbers. Bound page size (14,400 pt) and run coordinates (S).
- **Error offsets and kinds (CQ-10, ARCH-2, BP-6, ARCH2-4).** Add stream offsets and a typed error kind, and truncate the text echoed in errors (M).

## P1

- **Parsing and encoding:**
  - CQ-3: non-ASCII CHAR bytes re-encode wrongly, and names over 8 characters are truncated.
  - CQ-7: the 5A marker is never checked.
  - CQ-11: `Triplet.Encode` overflows past 253 bytes, and `Intro.Length` goes stale.
  - ARCH-3: code page 500 is unreachable from `Reduce`.
  - ARCH-1: input is buffered with `io.ReadAll` instead of streamed.
  - ARCH-6: no context support.
- **Tests:**
  - RPS and BAG edge cases.
  - A byte-compare of the generator output against the committed fixtures.
  - Reducer and PTOCA error-path coverage.
  - Fuzz targets beyond `afp`.
  - `-shuffle` and a coverage floor in CI.
- **CI and docs:**
  - Document the poppler dependency.
  - SHA-pin actions.
  - Add a `permissions` block and timeouts.
  - Fix the push plus pull_request double-run.
  - Remove the nested `writerspike` module.

## P2

- Exported mutable maps, split font tables, the dead `MaxFieldSize` check and the wide API.
- Inline position not advanced after text.
- Example tests, and the duplicate package comment in `afptoline`.
- `govulncheck`, Dependabot, `SECURITY.md`, a changelog and a fuller `.gitignore`.
- Benchmarks.

## Overruled or downgraded

- **ARCH2-3 (missing `/Encoding`):** refuted, as above.
- **TEST2-1:** downgraded from High to Medium, since the code is correct.
- **CQ2-3 (0x7F renders as a bullet):** not probed by the judge. The code emits `\177`, so treat it as low.
- **Stale local `main` and commit hygiene:** left alone.
- **Go 1.27 directive and the hardwired Courier font:** left alone.

## Suggested order

1. Delete the plan docs, fix the doc references and the README claim, then fix the BPT/EPG pairing with tests.
2. Add the RPS budget with a regression test, then the escape and WinAnsi tests.
3. In the next PR, add the C1 check, coordinate bounds, and error offsets and kinds. Then the P1 test and CI hygiene.
