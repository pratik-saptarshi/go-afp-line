# Round 2 Panel Findings — Implementation Plan

**Goal:** Fix-forward the Round 2 adversarial panel findings on go-afp-line `main` (post-merge of PR #1, merge commit 43f4472). Phase A covers the five ex-merge-blockers; Phase B covers the three next-PR P0s; Phase C plans the P1 items as concrete tasks (P2 items are dispositioned as deferred backlog with rationale).

**Architecture:** No new packages. Changes are local to `afptoline` (reducer guards, text budget), `line` (validation bounds), `linetopdf` (test pins only), and docs. The RPS budget is enforced at PTX-append time in the reducer (per-document accounting, error attributed to the offending PTX), which subsumes the panel's "per PTX and per document" ask: no single PTX can exceed the document budget.

**Tech Stack:** Go 1.27, stdlib only for the fixes. Repo checks per task: `gofmt -l`, `go vet ./...`, `go test -count=1 -race ./...`.

**Spec:** `docs/process/reviews/panel-round2-2026-10-15.md` (the panel verdict, verbatim). Key spec facts used: EPG must require `bpt == 0`; text budget 1 MiB per document default, configurable, error carries offset; C1 policy is reject-not-map; page bound 14,400 pt.

## Validation performed (2026-10-15)

Every blocker claim was corroborated against the merged tree before planning:

- **Plan-file leak:** confirmed — both `2026-10-14-*.md` plan files carry "REQUIRED SUB-SKILL" headers; the EAG plan has local home-directory paths; the P0 plan cites removed `docs/_evidence/ptoca-reference-extract.txt` and quotes the RPS spec sentence.
- **Dangling docs:** confirmed — `docs/afp-file-format-spec.md:10` cites removed PDFs; `docs/_evidence/README.md` is stale Task-0 process notes; `README.md:27` claims "verified against the IBM PTOCA Reference" with no spec text in repo.
- **BPT/EPG hole:** confirmed — `afptoline/reduce.go` EPG case checks `st.page == nil` and `st.bag != 0` but not `st.bpt != 0`; the reported stream is accepted.
- **RPS amplification:** confirmed — RLENGTH is uint16 (64 KiB per sequence max), no budget exists anywhere on the decode path.
- **WinAnsi test gaps:** confirmed — `TestEncodeText_WinAnsiSpecials` pins only € and smart quotes; no coverage for paren/backslash escapes, control-byte octal escapes, Œ, 0xFF boundary, 0x7F.
- P0-next items (C1, coordinate bounds, error offsets/kinds) accepted on the judge's probing per the verdict.

## Global Constraints

- Branch: new `fix/panel-round2` from `origin/main` (43f4472) for Phase A. Phase B is a separate PR after Phase A merges. Phase C branches `fix/panel-round2-phase-c` from `origin/main` after Phase B merges.
- TDD: failing test first for A2, A3, A4, B1, B2, and every Phase C code task. Docs task A1 needs no tests.
- Conventional Commits subjects; no force-push on the branch once published.
- No absolute local paths or usernames in plans, code, or comments — repo-relative only.
- Work in an isolated worktree; do not disturb other checkouts.
- Do not implement P1/P2 items in Phases A/B.

## Review Focus

The reviewer should attack these five inputs specifically:

1. **Budget accounting:** bytes counted as UTF-8 length of decoded run text; excluded fields never decode, so they consume no budget — confirm a PTX-heavy excluded stream cannot bypass.
2. **BPT nesting edges:** `BDT BPG BPT EPT EPG EDT` stays valid; `BDT BPG EPG EDT` with `bpt == 0` unaffected; nested `BPT BPT` still fails at the second BPT.
3. **C1 check placement:** it must run on the decoded rune, independent of code page (code page 500 remains unreachable from `Reduce` per ARCH-3 — out of scope).
4. **Coordinate bound edges:** exactly 14,400 passes, 14,400.01 fails; negative run coordinates remain legal (off-page); NaN/±Inf still rejected.
5. **Budget configurability edge:** `ReduceWithBudget(r, n)` with `n <= 0` returns an immediate argument error; a document at exactly the budget passes, budget+1 fails.

---

## Phase A — fix-forward (one PR)

### Task A1: Doc hygiene — remove leaks and stale references

**Files:** delete the `2026-10-14-afp-panel-p0-batch.md` and `2026-10-14-eag-pairing-fix.md` plan files, `docs/_evidence/README.md`; modify `docs/afp-file-format-spec.md`, `README.md`. (This plan file stays as the active working document; it contains no local paths.)

**Steps:**
1. Confirm nothing links to `docs/_evidence/README.md` (`grep -rn "_evidence/README" docs README.md`); delete it.
2. Delete the two old plan files.
3. `docs/afp-file-format-spec.md:10`: delete the "Local copies: ..." line (PDFs were removed in the spec-extract scrub).
4. `README.md:27`: reword "wire formats verified against the IBM PTOCA Reference" → "wire formats per the AFP subset contract (`docs/subset-contract.md`)".
5. `gofmt` clean (docs only — sanity check nothing else touched).

**Done when:** the four stale references are gone, no grep hits remain for `ptoca-reference-extract`, `_evidence/modca`, or "IBM PTOCA Reference" outside the new verdict/plan files.
**Commit:** `docs: remove stale plan files and scrubbed-spec references`

### Task A2: Reject BPT groups that cross EPG

**Files:** `afptoline/reduce.go`, `afptoline/reduce_test.go`

**Steps:**
1. Write `TestReduce_BPTMustNotCrossEPG` mirroring `TestReduce_BAGMustNotCrossEPG`: stream `(bdt,bpg,bpt,epg,ept,epg,edt)` must fail naming EPG; stream `(bdt,bpg,bpt,epg,edt)` must fail. Confirm both fail before the fix.
2. In the EPG case of `reduceState.visit`, add after the BAG check: `if st.bpt != 0 { return st.fail(t.TypeID(), "EPG with open BPT group") }`.
3. Confirm the new tests pass; confirm the valid stream `(bdt,bpg,bpt,ptx,ept,epg,edt)` still reduces.
4. Full `go test -count=1 -race ./...`, `go vet ./...`, `gofmt -l` clean.

**Done when:** the pairing hole is closed with the same message shape as the BAG fix, and the reviewer confirms ENG/EAG/EPT guards are sufficient (each requires its own counter == 1; BNG cannot open inside a page).
**Commit:** `fix(afptoline): reject BPT groups that cross EPG`

### Task A3: Decoded-text budget (RPS amplification DoS)

**Files:** `afptoline/ptoca.go`, `afptoline/reduce.go`, `afptoline/reduce_test.go` (and/or `ptoca_test.go`)

**Background (spec amendment 2026-10-15):** the first implementation enforced the
budget *after* `DecodePTOCA` fully expanded the field. Independent review
measured ~4.6 GiB cumulative allocation (~820 MiB peak) from a 32.1 KiB hostile
stream before the error fired — a transient DoS, still an OOM kill on
constrained hosts. The budget MUST be enforced fail-fast *inside* the decode
loop. This amends the interfaces below (supersedes the 2026-10-15 plan rev 1).

**Interfaces (final, rev 2):**
- `const DefaultMaxTextBytes = 1 << 20` in `afptoline` (unchanged).
- `func ReduceWithBudget(r io.Reader, maxTextBytes int) (line.Document, []line.Exclusion, error)`; `maxTextBytes <= 0` returns an immediate error (`"non-positive text budget"`); `Reduce(r)` delegates (unchanged).
- `ptocaState` gains `remaining int`: max additional `text()` bytes allowed; negative = unlimited.
- `func (st *ptocaState) text(b []byte) error`: bytes inside a suppression region are dropped without consuming budget (nothing is allocated); otherwise, if `st.remaining >= 0 && len(b) > st.remaining`, return `afp.DecodeError{Reason: "decoded text budget exceeded"}` **before** appending; else append and `st.remaining -= len(b)`. All existing `text()` call sites updated for the error return.
- `doRPS`: before allocating the repeat buffer, if `st.remaining >= 0 && int(rlength) > st.remaining`, return the budget error immediately (no 64 KiB transient).
- `DecodePTOCA(f, cp)` keeps its exported signature and delegates to the new internal `decodePTOCA(f afp.Field, cp afp.CodePage, maxBytes int) ([]Run, error)` with `maxBytes = -1`.
- `visitPTX` calls `decodePTOCA(f, st.cp, st.textLeft)`; the decoder's budget error propagates wrapped with the PTX field ID and `st.off`.
- `reduceState` keeps `textLeft int` (init from budget) and `off int`; after a successful decode, `st.textLeft -= <sum of UTF-8 bytes of decoded run texts>` as a retained-text cap (defense-in-depth; the decoder's fail-fast is the security boundary).
- Fix `st.off` to equal the true wire offset on 5A-wrapped streams (the `len(raw.Encode())` accounting under-counts framing bytes); add a regression test with a 5A-wrapped PTX asserting the budget error's offset.

**Steps:**
1. Write failing tests first:
   - White-box fail-fast: `text()` with `remaining = 10` and an 11-byte input returns the budget error and leaves `st.cur` unchanged; `doRPS` with `rlength = 65535`, `remaining = 4096` returns before allocating (assert error, and assert no large `st.cur` growth).
   - End-to-end: RPS-heavy PTX via `ReduceWithBudget(r, 4096)` fails with the budget error naming the PTX offset.
   - Boundaries (ASCII, so source bytes == decoded bytes): document at exactly the budget passes; budget+1 fails.
2. Implement per the interfaces above.
3. `go test -count=1 -race ./...`, `go vet ./...`, `gofmt -l` clean.

**Done when:** a hostile RPS stream fails fast with no per-field expansion beyond `remaining` + one segment, the error carries the offending PTX's true wire offset, all existing tests pass unmodified, and the independent reviewer approves.
**Commit:** `fix(afptoline): bound decoded text per document (RPS amplification)`

### Task A4: Pin escaping, WinAnsi boundaries, non-finite dimensions

**Files:** `linetopdf/render_test.go`, plus `line` package tests where dimension validation lives

**Steps:**
1. Table-driven `TestEncodeText_Escapes`: `"("` → `\(`, `")"` → `\)`, `"\\"` → `\\`; control bytes (e.g. `"\x01\x02"`) → octal escapes (read `encodeText` for exact expected output — pin, don't invent); `"\x7f"` → `\177`.
2. `TestEncodeText_WinAnsiBoundaries`: `Œ` (U+0152) → `\x8c`; `Ÿ` (U+009F is C1 — must NOT appear; use U+0178) → `\x9f`; U+00FF → `\xff`; U+0100 → error.
3. Dimension pins: NaN and -Inf Width/Height/FontSize rejected by validation (locate the dimension validator in `line`; extend its table test).
4. Full checks clean.

**Done when:** the six surviving mutations from the panel (paren/backslash, control-byte octals, Œ, 0xFF, 0x7F, NaN/-Inf) are all pinned by tests.
**Commit:** `test(linetopdf,line): pin escaping, WinAnsi boundaries, non-finite dimensions`

---

## Phase B — next PR (after Phase A merges)

### Task B1: Reject C1 characters in Reduce with offset

**Files:** `line/line.go` (or wherever `TextRun.Validate` lives), `afptoline/reduce.go`, tests in both

**Interfaces (final):** `TextRun.Validate` rejects any rune in U+0080–U+009F ("C1 controls have no WinAnsi encoding; rejected per policy, not mapped"). `visitPTX` validates each decoded run immediately and returns the `afp.DecodeError` with the PTX field ID and `st.off` (offset tracking from Task A3).

**Steps:** TDD — EBCDIC byte decoding to U+0080 in a PTX must fail `Reduce` naming the offset; valid WinAnsi-mapped bytes (0x80–0x9F are Windows-1252 printable, not C1 — the check is on the decoded Unicode rune) still pass.

**Commit:** `fix(afptoline,line): reject C1 characters in Reduce with offset`

### Task B2: Coordinate bounds

**Interfaces (final):** page Width/Height must satisfy `0 < v <= 14400`; run X/Y must satisfy `|v| <= 14400` (negatives remain legal off-page); NaN/±Inf still rejected. Enforced in `line` validation (`Document`/`Page`/`TextRun` as appropriate).

**Steps:** TDD boundary tests (14400 passes, 14400.01 fails).

**Commit:** `fix(line): bound page size and run coordinates`

### Task B3: Error offsets, typed kinds, truncation

**Interfaces (final):** extend `afp.DecodeError` with a typed `Kind` (e.g. `type ErrorKind string` with documented values); generalize Task A3's offset tracking so every reducer `fail` carries the stream offset; truncate text echoed in `Reason` to 80 characters.

**Steps:** implementer designs the Kind taxonomy (structure, encoding, budget, validation); reviewer checks every `fail` call site carries a real offset.

**Commit:** `fix(afp,afptoline): typed error kinds, stream offsets, truncated echoes`

---

## Phase C — P1 backlog (after Phase B merges)

Phase C implements the verdict's P1 items in four workstreams: parsing/encoding
defects (C1–C4), test hardening (C5–C9), CI/docs hygiene (C10–C12), and the
streaming-input architecture change (C13, sequenced last). All code tasks are
TDD with independent review, following the Phase A/B discipline. Ship one PR
per workstream, stacked while predecessors are unmerged.

### Review Focus (Phase C)

The reviewer should attack these inputs specifically:

1. **Encode contract (C2/C3):** one consistent encode-error policy across every `Encode` method — no silent truncation or substitution may remain anywhere.
2. **C4 default preservation:** `Reduce` must remain byte-identical to `ReduceWithCodePage(37)`; the code page knob must not change default behavior.
3. **C8 fuzzing:** invariants must hold under `-race`; the seed corpus is committed.
4. **C13 contract:** the iterator must preserve 5A framing semantics, `MaxFieldSize` checks, and error kinds exactly; reviewer signs off the contract before implementation.

### Task C1: Check the 5A record marker in SplitFields (CQ-7)

**Files:** `afp/sfi.go`, `afp/sfi_test.go`

**Steps:**
1. Failing test first: a 5A-wrapped stream with a corrupt (non-`0x5A`) record marker byte must fail loudly with `KindStructure` naming the field offset (today the byte is silently skipped by `off++`).
2. In `SplitFields`, when `wrapped`, verify `data[off] == 0x5A` before stripping; on mismatch return `DecodeError{Kind: KindStructure, Offset: fieldStart, Reason: "expected 5A record marker"}`.
3. Confirm valid wrapped and naked streams still parse; full checks clean.

**Done when:** a corrupt marker fails loudly; all existing `SplitFields` tests pass unmodified.
**Commit:** `fix(afp): validate the 5A record marker in SplitFields`

### Task C2: Triplet.Encode overflow + stale Intro.Length (CQ-11)

**Files:** `afp/triplets.go`, `afp/field.go`, tests

**Background / interface decision:** `Triplet.Encode() []byte` cannot return an error, yet silently truncates via `byte(len(t.Data)+2)` when the triplet exceeds the 1-byte length field; `Introducer.Encode()` emits the stored `Length`, which goes stale if field data is mutated post-decode. The implementer chooses the encode-error contract — options: (a) extend `Encode` to return an error across the `Field` interface (breaking; audit every call site), (b) validate at triplet construction and recompute `Intro.Length` from the built bytes inside each `Field.Encode()`. The reviewer must confirm the chosen contract is applied consistently to every `Encode` method; document the decision in the commit message.

**Steps:**
1. Failing tests: a triplet with `len(Data)+2 > 255` must not silently truncate; a BDT whose triplets changed post-decode must re-encode with a correct length prefix (byte-identical round trip).
2. Implement per the chosen contract.
3. Full checks clean.

**Done when:** no silent truncation or stale length survives; round-trip tests pass.
**Commit:** `fix(afp): fail loudly on triplet overflow; recompute introducer length on encode`

### Task C3: CHAR encode fidelity (CQ-3)

**Files:** `afp/field.go` (`encodeCHAR8`, `decodeCHAR`), tests

**Background / interface decision:** `decodeCHAR` decodes the full EBCDIC-037 table (non-ASCII runes possible), but `encodeCHAR8` silently maps non-ASCII to `0x6F` (`?`) and truncates names over 8 characters — breaking the byte-identical `RoundTrip` gate. Same `Encode() []byte` signature constraint as C2; coordinate the contract decision with C2 so the `afp` package ends up with one consistent encode-error policy. The fail-loud direction is fixed (this repo fails loudly; silent `?` substitution is data corruption) — only the mechanism is open.

**Steps:**
1. Failing tests: round-trip of a name containing non-ASCII EBCDIC bytes fails loudly instead of substituting `?`; a name longer than 8 chars fails loudly instead of truncating.
2. Implement per the C2-coordinated contract.
3. Full checks clean.

**Done when:** CHAR fields round-trip byte-identically or fail loudly; no silent substitution or truncation.
**Commit:** `fix(afp): CHAR fields fail loudly on non-encodable runes`

### Task C4: Code page 500 reachable from Reduce (ARCH-3)

**Files:** `afptoline/reduce.go`, tests

**Interfaces (final):** add `func ReduceWithCodePage(r io.Reader, cp afp.CodePage) (line.Document, []line.Exclusion, error)`; `Reduce` delegates with `afp.LookupCodePage(37)` (default behavior unchanged). Selecting the *correct* code page from stream metadata (MCF/FQN) is out of scope — this only exposes the knob; say so in the function comment.

**Steps:**
1. Failing test: decode a PTX through `ReduceWithCodePage` with code page 500 using bytes that differ between 037 and 500 → correct text; `Reduce` output unchanged on the same input.
2. Implement; full checks clean.

**Done when:** both code pages reachable; the default path is byte-identical to before.
**Commit:** `feat(afptoline): expose code page selection in Reduce`

### Task C5: RPS and BAG edge cases

**Files:** `afptoline/*_test.go`

**Steps:**
1. Table tests for: RPS with `RLENGTH == len(RPTDATA)` (transparent-equivalent), `RLENGTH == 0`, RPS at the budget boundary (complements A3 without duplicating it), BAG nested to depth 2, BAG/EAG with an empty group.
2. Do not duplicate A2/A3 tests — the reviewer checks the new cases against the existing suite first.
3. Full checks clean.

**Done when:** the edge cases are pinned; no duplication of A2/A3 coverage.
**Commit:** `test(afptoline): RPS and BAG edge cases`

### Task C6: Generator-vs-fixture byte-compare

**Files:** `afptoline/genfixture` tests (or a new test alongside the fixtures)

**Steps:**
1. The implementer first checks the fixture generator for nondeterminism (map iteration, timestamps, random IDs).
2. Add a test that regenerates the fixtures and byte-compares against the committed files; pin or document any nondeterminism found in step 1.
3. Full checks clean.

**Done when:** the committed fixtures are proven to be generator output, byte-identical.
**Commit:** `test(afptoline): byte-compare generator output against fixtures`

### Task C7: Reducer and PTOCA error-path coverage

**Files:** `afptoline/reduce_test.go`, `afptoline/ptoca_test.go`

**Steps:**
1. Enumerate every `st.fail` site in the reducer and every error return in the PTOCA decoder (truncated introducer, invalid length, unknown sequence, unmatched ESU, budget errors, etc.).
2. Add a failing test per uncovered site, then fill them: each malformed stream must produce the expected `Kind` and naming.
3. Measure with `go test -cover`; the reviewer verifies the enumeration against the source — no error return left untested.

**Done when:** all documented error paths have tests; the reviewer confirms the enumeration is complete.
**Commit:** `test(afptoline): cover reducer and PTOCA error paths`

### Task C8: Fuzz targets beyond afp

**Files:** new fuzz tests in `afptoline` (and `line`/`linetopdf` if the implementer finds useful invariants)

**Steps:**
1. Add `FuzzDecodePTOCA` (invariants: no panic, budget respected, all errors are `afp.DecodeError`) and `FuzzReduce` (invariants: no panic on arbitrary bytes, budget respected), following the existing `FuzzDecodeIntroducer`/`FuzzDecodeField` pattern; seed corpora from existing fixtures.
2. Run each fuzzer briefly before committing; keep the seed corpus.
3. Full checks clean.

**Done when:** new fuzz targets exist with committed seed corpora; short fuzz runs clean.
**Commit:** `test(afptoline): fuzz PTOCA decode and Reduce`

### Task C9: -shuffle and coverage floor in CI

**Files:** `.github/workflows/ci.yml`

**Steps:**
1. Add `-shuffle=on` to the `go test` invocations.
2. Add a coverage floor: the implementer measures current total coverage, sets the floor at or below it, and documents the threshold; falling below fails the build.
3. Full checks clean.

**Done when:** CI runs shuffled; coverage below the floor fails the build.
**Commit:** `ci: shuffle tests and enforce coverage floor`

### Task C10: CI hygiene — pin, permissions, timeouts, single run

**Files:** `.github/workflows/ci.yml`

**Steps:**
1. SHA-pin all third-party actions (keep a trailing comment with the human-readable version tag).
2. Add a top-level least-privilege `permissions:` block and `timeout-minutes` on every job.
3. Fix the push+pull_request double-run (e.g., `concurrency` cancellation or event conditions — implementer picks the mechanism consistent with the file).
4. Verify: no floating action refs remain; a push to an open PR triggers exactly one run.

**Done when:** pins, permissions, and timeouts in place; single run per push.
**Commit:** `ci: pin actions, least-privilege permissions, timeouts, single run`

### Task C11: Document the poppler dependency

**Files:** `README.md` (or the repo-consistent docs location — implementer picks)

**Steps:** document that PDF position tests require `poppler-utils` (`pdftotext`), how CI installs it, and the `CI=true` fail-fast behavior from TEST-1.

**Done when:** a new contributor can satisfy the dependency from the docs alone.
**Commit:** `docs: document the poppler test dependency`

### Task C12: Remove the nested writerspike module

**Files:** `docs/_evidence/writerspike/`, `docs/writer-spike.md`

**Background / prescription refinement:** the verdict says "remove the nested `writerspike` module", but `docs/writer-spike.md` cites the directory's evidence files (`main.go`, bbox XMLs). Deleting the evidence would contradict the repo's evidence-keeping; the actual hazard is the nested `go.mod` confusing tooling. So: de-modulize rather than delete.

**Steps:**
1. Confirm nothing builds or references the module (`go build`/`go vet` from the repo root ignore it; grep for imports).
2. Delete `docs/_evidence/writerspike/go.mod` and `go.sum`; keep the evidence files.
3. Update `docs/writer-spike.md`'s Method paragraph to no longer list `go.mod`/`go.sum`.
4. Full checks clean.

**Done when:** no nested module remains; evidence files and doc reference intact.
**Commit:** `chore: de-modulize docs/_evidence/writerspike`

### Task C13: Stream input instead of io.ReadAll (ARCH-1)

**Files:** `afp/sfi.go` (`SplitFields`), `afptoline/reduce.go`

**Scope note:** architectural change — `SplitFields` returns `[][]byte` after `io.ReadAll`, and `ReduceWithBudget` buffers once more before that. This task converts field splitting to an incremental iterator so unbounded inputs never fully materialize (the A3 budget already bounds per-field expansion; this bounds input retention). **Governance:** scope-expanding — the reviewer must sign off the iterator contract before implementation.

**Steps:**
1. Design the iterator contract first (options: `iter.Seq2[[]byte, error]`, callback-based, channel-based); reviewer approves before any implementation.
2. Failing test: a logically large input (a reader yielding many fields without materializing them) reduces under a bounded memory ceiling — assert the ceiling, not an actual huge allocation.
3. Migrate `ReduceWithBudget` and any other callers to the iterator; keep or remove the old `SplitFields` per reviewer decision.
4. Full checks clean.

**Done when:** no `io.ReadAll` remains on the input path; memory stays bounded on large inputs per the test.
**Commit:** `feat(afp): stream structured fields instead of buffering`

---

## Backlog (dispositioned — not planned)

P1 items from the verdict are now Phase C tasks above; what remains below is deferred with rationale. None is worsened by this plan.

- **Exported mutable maps, split font tables, dead `MaxFieldSize` check, wide API (R2P2-F01):** defer — pre-existing API polish; revisit once the API surface stabilizes.
- **Inline position not advanced after text (R2P2-F02):** defer — deliberate v1 limitation; `DecodePTOCA` documents that per-character advance needs font metrics (original Task 8 design).
- **Example tests, duplicate package comment in `afptoline` (R2P2-F03):** defer — trivial; bundle into any future test task.
- **`govulncheck`, Dependabot, `SECURITY.md`, changelog, fuller `.gitignore` (R2P2-F04):** defer — operational workstream, separate from code correctness.
- **Benchmarks (R2P2-F05):** defer — no performance requirement exists yet; add when optimizing.
- **Codex PR #3 P2 — rebase non-budget PTOCA decoder offsets to stream offsets (CODEX-F01):** defer — diagnostic-only (fail-loud, correct FieldID); the proper fix threads stream offsets into the decoder error path (introducer length + 5A framing), a contract change across all malformed-input errors. Schedule alongside future decoder error-path work.
- **ARCH-6 — no context support (R2P1-F06):** dropped per ADR 0001 (docs/decisions/0001-drop-context-support.md): no caller scenario justifies the breaking API change; resource exhaustion is covered by the RPS text budget and the C13 streaming iterator, and a context cannot unstick a hung `Read`.

The verdict's `## Overruled or downgraded` items (ARCH2-3 refuted, TEST2-1 downgraded, CQ2-3 low, Go 1.27 directive and hardwired Courier left alone) required no plan action and are recorded here for completeness.
