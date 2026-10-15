# go-afp-line P0 Batch Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the five P0 defects from the 2026-10-14 adversarial panel verdict, each with a positive test.

**Architecture:** Five independent single-area fixes, each a small vertical slice: reducer nesting (Task 1), PTOCA repeat semantics (Task 2), PDF text encoding (Task 3), float validation (Task 4), CI test infrastructure (Task 5). No task depends on another; order follows the panel's numbering.

**Tech Stack:** Go 1.27, pdfcpu (PDF generation), GitHub Actions (ubuntu-latest).

**Spec:** The adversarial panel verdict (2026-10-14), corroborated against `docs/subset-contract.md`, `docs/_evidence/ptoca-reference-extract.txt`, and the source. Key spec facts used verbatim: contract walk `BDT … [BNG …] BPG → BAG … EAG → [BPT] PTX … [EPT] … EPG … EDT`; RPS "repeated until the number of bytes in the graphic characters presented is equal to a specified number of bytes".

## Global Constraints

- Conventional Commits required (`fix(afptoline): …`, `fix(linetopdf): …`, `fix(line): …`, `ci: …`) per CONTRIBUTING.md. Push via the branch; no publisher script in this repo.
- Go toolchain is at `~/sdk/go`; binaries in `~/go/bin`. Every shell command that invokes Go must start with `export PATH="$HOME/sdk/go/bin:$HOME/go/bin:$PATH"`.
- Synthetic fixtures only — never commit production AFP data, even "anonymized" (CONTRIBUTING.md).
- TDD: failing test first, minimal fix, tests green. Do not weaken or skip a failing check to get a change through.
- Baseline is green: `go test`, `go vet`, `gofmt`, `-race -shuffle -count=2` all pass at 84.6% coverage. Keep it green.

## Review Focus

- A BAG/EAG fix that also loosens BNG/ENG or page-group pairing: the contract places BNG before BPG and BAG after BPG — only the BAG/EAG guard changes.
- An RPS fix that mishandles `RLENGTH=0` or empty RPTDATA with `RLENGTH>0`: decide per the spec text (repeating nothing to a positive length is impossible — error, don't silently emit nothing).
- A WinAnsi encoder that maps unmappable characters silently (e.g., to `?`): the panel requires an error, not silent substitution.
- A `+Inf` fix that also rejects `-Inf` for Width/Height/FontSize where the old code accepted it via a different path: `!(x > 0)` already rejects negatives and `-Inf`; adding `IsInf` only closes the `+Inf` hole — keep the error messages unchanged.
- A CI poppler install that makes the whole job fail on apt errors unrelated to poppler: scope the install step so its failure mode is obvious.

---

### Task 1: Allow BAG/EAG while a page is open (CQ-1)

**Files:**
- Modify: `afptoline/reduce.go:157-163` (the `D3A8C9` BAG case guard)
- Test: the existing reducer test file covering "BAG in illegal position" (add positive case; keep the existing unclosed-BAG negative test)

**Interfaces:**
- Consumes: nothing from other tasks.
- Produces: BAG/EAG accepted inside an open page; `st.bag` pairing still enforced.

- [ ] **Step 1: Write the failing positive test**

Add a test that walks `BDT … BPG → BAG … EAG → EPG … EDT` (minimal per the contract walk) and asserts `Reduce` succeeds. Run it; expected FAIL with "BAG in illegal position".

- [ ] **Step 2: Fix the BAG guard in `afptoline/reduce.go`**

Change the `D3A8C9` case guard from `if !st.inDoc || st.page != nil || st.bag != 0` to `if !st.inDoc || st.page == nil || st.bag != 0` — BAG requires an open page (it follows BPG), matching the contract. Do not touch the BNG/ENG guards or the EAG `st.bag != 1` pairing check.

- [ ] **Step 3: Run the tests**

Run: `export PATH="$HOME/sdk/go/bin:$HOME/go/bin:$PATH" && go test -race ./afptoline/ -v -run 'TestReduce|TestBAG'`
Expected: PASS, including the pre-existing unclosed-BAG negative test.

- [ ] **Step 4: Run the full suite and commit**

Run: `export PATH="$HOME/sdk/go/bin:$HOME/go/bin:$PATH" && go test ./... && go vet ./... && test -z "$(gofmt -l .)"`
Expected: all PASS, no output from gofmt.

```bash
git add afptoline/
git commit -m "fix(afptoline): allow BAG/EAG inside an open page per subset contract"
```

### Task 2: RPS repeats RPTDATA to RLENGTH (CQ-2)

**Files:**
- Modify: `afptoline/ptoca.go:350-365` (`doRPS`)
- Test: the PTOCA test file covering `doRPS` (add repeat + no-op cases)

**Interfaces:**
- Consumes: nothing from other tasks.
- Produces: `doRPS` emitting `RLENGTH` bytes by cycling RPTDATA.

- [ ] **Step 1: Write the failing tests**

Add tests: (a) `RLENGTH=5` with 1-byte RPTDATA yields 5 repeated bytes; (b) `RLENGTH=4` with 2-byte RPTDATA yields the 2 bytes twice; (c) `RLENGTH` equal to RPTDATA length behaves as TRN (existing behavior, pin it); (d) `RLENGTH=0` is a no-op emitting nothing. Run; expected FAIL on (a), (b), (d).

- [ ] **Step 2: Implement repeat semantics in `doRPS`**

Replace the `st.text(params[2:2+n])` slice with a loop that cycles `params[2:]` until `n` bytes are emitted. Keep the existing `n > len(params)-2` guard replaced by: if `len(params) == 2` (empty RPTDATA) and `n > 0`, return the existing `afp.DecodeError` shape (cannot repeat nothing); if `n == 0`, emit nothing and return nil.

- [ ] **Step 3: Run the tests**

Run: `export PATH="$HOME/sdk/go/bin:$HOME/go/bin:$PATH" && go test -race ./afptoline/ -v -run 'TestRPS|TestDoRPS|TestPTOCA'`
Expected: PASS.

- [ ] **Step 4: Run the full suite and commit**

Run: `export PATH="$HOME/sdk/go/bin:$HOME/go/bin:$PATH" && go test ./... && go vet ./... && test -z "$(gofmt -l .)"`
Expected: all PASS.

```bash
git add afptoline/
git commit -m "fix(afptoline): RPS repeats RPTDATA until RLENGTH bytes"
```

### Task 3: Encode PDF text as WinAnsi (CQ-4)

**Files:**
- Modify: `linetopdf/render.go:52-57` (`escapeText`) and its call site at line 89-90
- Test: `linetopdf/render_test.go` (add "café" test)

**Interfaces:**
- Consumes: nothing from other tasks.
- Produces: `escapeText` (or its replacement) returning an error on unmappable characters.

- [ ] **Step 1: Write the failing test**

Add a test rendering a run with text `"café"` and assert the content stream contains the WinAnsi byte `0xE9` for `é` (single byte, not the UTF-8 pair `0xC3 0xA9`). Run; expected FAIL.

- [ ] **Step 2: Implement WinAnsi encoding**

Replace the raw passthrough in `escapeText` with encoding to single-byte WinAnsi (code page 1252): map each rune to its WinAnsi byte; return an error naming the offending character for anything unmappable (no silent `?` substitution); escape PDF control characters (`\`, `(`, `)`) and raw control bytes as before. Change the signature to return `(string, error)` and handle the error at the call site by wrapping with `fmt.Errorf("linetopdf: ...")`.

- [ ] **Step 3: Run the tests**

Run: `export PATH="$HOME/sdk/go/bin:$HOME/go/bin:$PATH" && go test -race ./linetopdf/ -v`
Expected: PASS (the pdftotext-dependent tests skip locally if poppler is absent — that is Task 5's concern, not this task's).

- [ ] **Step 4: Run the full suite and commit**

Run: `export PATH="$HOME/sdk/go/bin:$HOME/go/bin:$PATH" && go test ./... && go vet ./... && test -z "$(gofmt -l .)"`
Expected: all PASS.

```bash
git add linetopdf/
git commit -m "fix(linetopdf): encode text as WinAnsi, error on unmappable chars"
```

### Task 4: Reject +Inf in float validation (CQ-5)

**Files:**
- Modify: `line/line.go:42` (Width), `line/line.go:45` (Height), `line/line.go:66` (FontSize)
- Test: the `line` package test file covering `Validate`

**Interfaces:**
- Consumes: nothing from other tasks.
- Produces: `+Inf` rejected for Width, Height, FontSize.

- [ ] **Step 1: Write the failing tests**

Add table cases asserting `Page{Width: math.Inf(1)}.Validate()`, `Page{Height: math.Inf(1)}.Validate()`, and `TextRun{FontSize: math.Inf(1)}.Validate()` all return errors. Run; expected FAIL (currently accepted).

- [ ] **Step 2: Add `math.IsInf` checks**

At each of the three sites, extend the condition with `|| math.IsInf(x, 0)` following the existing X/Y pattern at lines 69/72. Keep the existing error messages and `ValidationError` shapes unchanged.

- [ ] **Step 3: Run the tests and commit**

Run: `export PATH="$HOME/sdk/go/bin:$HOME/go/bin:$PATH" && go test -race ./line/ -v && go test ./... && go vet ./... && test -z "$(gofmt -l .)"`
Expected: all PASS.

```bash
git add line/
git commit -m "fix(line): reject +Inf for Width, Height and FontSize"
```

### Task 5: Make PDF position tests run in CI (TEST-1)

**Files:**
- Modify: `.github/workflows/ci.yml` (install poppler)
- Modify: `linetopdf/render_test.go:55-56` (fail instead of skip when `CI=true`)

**Interfaces:**
- Consumes: nothing from other tasks.
- Produces: CI installs poppler; missing `pdftotext` fails the test under `CI=true`, skips locally.

- [ ] **Step 1: Write the failing behavior (locally verifiable)**

Temporarily set `CI=true` with `pdftotext` absent from PATH and run the position tests; expected FAIL after the change (currently SKIP). The test change: replace `t.Skip(...)` with `if os.Getenv("CI") == "true" { t.Fatal("pdftotext (poppler) not available: independent reader required") }` before the skip fallback.

- [ ] **Step 2: Install poppler in CI**

Add a step to `.github/workflows/ci.yml` before the `test` step: `sudo apt-get update && sudo apt-get install -y poppler-utils`. Keep the existing step order and style.

- [ ] **Step 3: Verify locally and commit**

Run: `export PATH="$HOME/sdk/go/bin:$HOME/go/bin:$PATH" && go test -race ./linetopdf/ -v` (skips locally if poppler absent — acceptable) and `CI=true go test -run 'TestPDFPosition|TestPositions' ./linetopdf/` to confirm the fail-fast path triggers when pdftotext is missing. Then `gofmt`/`go vet` clean.

```bash
git add .github/workflows/ci.yml linetopdf/render_test.go
git commit -m "ci: install poppler so PDF position tests run; fail fast when CI=true"
```

---

## Self-review

**1. Spec coverage:** Panel P0s → Tasks 1–5, one per finding. The P1s (fuzzing, nesting strictness, error contract, input caps, code page 500, Encode fidelity, CI hardening, docs/_evidence licensing, context support) are explicitly out of scope for this batch — the panel's recommended order puts them in steps 2–3.

**2. Step scan:** Each step is one action with a checkable result. Task 5 Step 1's "temporarily set CI=true" is verifiable via the named test run. No step carries more than its action.

**3. Type consistency:** No cross-task interfaces; each task is self-contained in its package. Exact file:line references given for every change site.

**4. Review Focus:** Covered — BAG guard scope (Task 1), RPS no-op cases (Task 2), no silent substitution (Task 3), unchanged error messages (Task 4), scoped apt failure (Task 5). Each has its owning task.

**5. Proportion:** Five small tasks for five small fixes; steps are actions with commands, not code transcripts. The longest content is the exact guard change in Task 1 and the RPS edge-case table in Task 2, both values the implementer cannot invent.
