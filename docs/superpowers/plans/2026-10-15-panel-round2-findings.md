# Round 2 Panel Findings — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use the superpowers skill `test-driven-development` for every code change and `finishing-a-development-branch` when each phase is complete.

**Goal:** Fix-forward the Round 2 adversarial panel findings on go-afp-line `main` (post-merge of PR #1, merge commit 43f4472). Phase A covers the five ex-merge-blockers; Phase B covers the three next-PR P0s. P1/P2 items are backlog, not planned here.

**Architecture:** No new packages. Changes are local to `afptoline` (reducer guards, text budget), `line` (validation bounds), `linetopdf` (test pins only), and docs. The RPS budget is enforced at PTX-append time in the reducer (per-document accounting, error attributed to the offending PTX), which subsumes the panel's "per PTX and per document" ask: no single PTX can exceed the document budget.

**Tech Stack:** Go 1.27, stdlib only for the fixes. Repo checks per task: `gofmt -l`, `go vet ./...`, `go test -count=1 -race ./...`.

**Spec:** `docs/reviews/panel-round2-2026-10-15.md` (the panel verdict, verbatim). Key spec facts used: EPG must require `bpt == 0`; text budget 1 MiB per document default, configurable, error carries offset; C1 policy is reject-not-map; page bound 14,400 pt.

## Validation performed (2026-10-15)

Every blocker claim was corroborated against the merged tree before planning:

- **Plan-file leak:** confirmed — both `docs/superpowers/plans/2026-10-14-*.md` carry "REQUIRED SUB-SKILL" headers; the EAG plan has `/home/hatch/...` paths; the P0 plan cites removed `docs/_evidence/ptoca-reference-extract.txt` and quotes the RPS spec sentence.
- **Dangling docs:** confirmed — `docs/afp-file-format-spec.md:10` cites removed PDFs; `docs/_evidence/README.md` is stale Task-0 process notes; `README.md:27` claims "verified against the IBM PTOCA Reference" with no spec text in repo.
- **BPT/EPG hole:** confirmed — `afptoline/reduce.go` EPG case checks `st.page == nil` and `st.bag != 0` but not `st.bpt != 0`; the reported stream is accepted.
- **RPS amplification:** confirmed — RLENGTH is uint16 (64 KiB per sequence max), no budget exists anywhere on the decode path.
- **WinAnsi test gaps:** confirmed — `TestEncodeText_WinAnsiSpecials` pins only € and smart quotes; no coverage for paren/backslash escapes, control-byte octal escapes, Œ, 0xFF boundary, 0x7F.
- P0-next items (C1, coordinate bounds, error offsets/kinds) accepted on the judge's probing per the verdict.

## Global Constraints

- Branch: new `fix/panel-round2` from `origin/main` (43f4472) for Phase A. Phase B is a separate PR after Phase A merges.
- TDD: failing test first for A2, A3, A4, B1, B2. Docs task A1 needs no tests.
- Conventional Commits subjects; no force-push on the branch once published.
- No absolute local paths or usernames in plans, code, or comments — repo-relative only.
- Work in an isolated worktree; do not disturb other checkouts.
- Do not implement P1/P2 items in these phases.

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

**Files:** delete `docs/superpowers/plans/2026-10-14-afp-panel-p0-batch.md`, `docs/superpowers/plans/2026-10-14-eag-pairing-fix.md`, `docs/_evidence/README.md`; modify `docs/afp-file-format-spec.md`, `README.md`. (This plan file stays as the active working document; it contains no local paths.)

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

## Backlog (not planned — listed for visibility)

- **P1 parsing/encoding:** CQ-3 (non-ASCII CHAR re-encode, >8-char names), CQ-7 (5A marker unchecked), CQ-11 (`Triplet.Encode` >253 bytes, stale `Intro.Length`), ARCH-3 (code page 500 unreachable), ARCH-1 (stream input), ARCH-6 (context support).
- **P1 tests:** RPS/BAG edge cases, generator-vs-fixture byte-compare, reducer/PTOCA error-path coverage, fuzz beyond `afp`, `-shuffle` + coverage floor in CI.
- **P1 CI/docs:** poppler dependency documented, SHA-pinned actions, `permissions` block + timeouts, push+pull_request double-run fix, remove nested `writerspike` module.
- **P2:** exported mutable maps, split font tables, dead `MaxFieldSize` check, wide API, inline position, example tests, duplicate package comment, `govulncheck`/Dependabot/`SECURITY.md`/changelog/`.gitignore`, benchmarks.
