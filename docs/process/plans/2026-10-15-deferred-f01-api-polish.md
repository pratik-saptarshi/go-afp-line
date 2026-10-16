# API Polish (F01) Implementation Plan

**Goal:** Resolve the deferred API-polish findings (R2P2-F01): exported mutable maps, split font tables, the dead `MaxFieldSize` check, and the wide API surface.

**Architecture:** Design-first: Task 1 audits the surface and records decisions in a design doc; an independent reviewer signs off before Task 2 implements (same governance gate as C13). Implementation is mechanical refactoring — unexporting, dead-code removal, doc updates — with TDD where observable behavior changes.

**Tech Stack:** Go; `go vet`, `staticcheck`-style reasoning done by hand.

**Spec:** `docs/process/plans/2026-10-15-panel-round2-findings.md`, Backlog entry R2P2-F01 ("Exported mutable maps, split font tables, dead `MaxFieldSize` check, wide API — defer — pre-existing API polish; revisit once the API surface stabilizes"). The API has since stabilized (Phases A/B/C shipped), so the deferral rationale has expired. Survey 2026-10-15: `afp.CodePages` (`afp/codepage.go:29`, lookup `LookupCodePage` exists), `line.PinnedFonts` (`line/line.go:136`, lookup `LookupFont` exists, no external callers), `DecodeIntroducer`'s `length > MaxFieldSize` check (`afp/sfi.go:113`), 194 exported declarations total, zero `Example` functions.

## Global Constraints

- Go 1.27 toolchain; no new module dependencies.
- Conventional Commits subjects; identity `automation <waynemanorsaptarshi@gmail.com>`.
- Breaking changes to exported API are allowed here (v1 reference library, pre-1.0 semantics) but each must be recorded in the design doc with rationale.
- Never force-push; work on branch `chore/deferred-f01-api-polish` from `origin/main`.
- Full `gofmt`, `go vet`, `go test -count=1 -race ./...` green after every task.

## Review Focus

- Unexporting something an external user (or a test) actually needs: the audit must grep all callers, including `_test.go` files, before each unexport.
- Removing the "dead" check and breaking an external caller who relied on it: the design doc must rule explicitly (remove vs. keep as defense-in-depth) with the call-graph evidence.
- Behavior change disguised as refactoring: any task that changes observable behavior gets a failing test first.
- Doc comments referencing removed identifiers: grep for stale references after renames.

---

## File Structure

- Create `docs/process/designs/2026-10-15-f01-api-audit.md` — Task 1's audit + decisions (the contract Task 2 implements).
- Modify per Task 1's decisions — expected: `afp/codepage.go` (unexport map), `line/line.go` (unexport map), possibly `afp/sfi.go` (dead check), plus whatever the wide-API audit surfaces.
- Test: existing suite must stay green; new tests only where behavior changes.

---

### Task 1: API audit and design decisions

**Files:**
- Create: `docs/process/designs/2026-10-15-f01-api-audit.md`
- Modify: none (audit only)
- Test: n/a — deliverable is the design doc + reviewer sign-off

**Interfaces:**
- Consumes: the exported API surface (`go doc ./...`), call graphs from grep.
- Produces: the design doc; Task 2 implements exactly its decisions.

- [ ] **Step 1: Enumerate the full exported surface**

Run: `go doc ./... 2>/dev/null | grep -E "^(func|type|var|const) [A-Z]" | sort | uniq -c | sort -rn | head -60`
Record the complete list in the doc's appendix. For each of the four findings, investigate:

**a. Exported mutable maps.** Verify: `afp.CodePages` (callers of the map itself vs `LookupCodePage`), `line.PinnedFonts` (callers of the map itself vs `LookupFont`, including tests). Decision to record: unexport to `codePages`/`pinnedFonts`, keeping the lookup functions as the access path. Note any caller that needs migration.

**b. Dead `MaxFieldSize` check.** Trace every path into `DecodeIntroducer`'s `length > MaxFieldSize` branch: `fieldsOp` (pre-checked — dead), `DecodeField` ← `RoundTrip`/`ReduceWithBudget` (fields pre-checked — dead), and direct external callers of the exported `DecodeIntroducer`/`DecodeField` (possible — live). Decision to record: REMOVE the check (with rationale: all in-repo paths pre-check; the exported functions document their preconditions) or KEEP as defense-in-depth (with rationale). If removed, state what test pins the new behavior.

**c. Split font tables.** The survey found exactly one font table (`line.PinnedFonts`); verify there is no second table (check `linetopdf`, test fixtures, docs). Decision to record: either "no second table exists — no action, panel claim refuted" or the consolidation plan if one is found.

**d. Wide API.** From the 194 exports, flag candidates that are (i) unreferenced outside their package and tests, (ii) intermediate types that could be unexported, or (iii) helpers that tests use but users don't (those stay exported only with justification). Decision to record: explicit keep/unexport per candidate, or "no change" with rationale.

- [ ] **Step 2: Write the design doc**

`docs/process/designs/2026-10-15-f01-api-audit.md` with: per-finding evidence (call graphs), a decision per item, and the exact renames/removals Task 2 will make. End with an "Open questions" section if any.

- [ ] **Step 3: Independent reviewer sign-off**

A fresh reviewer (subagent, most capable model) verifies the call-graph claims against the code and signs off or requests changes — the C13 governance gate, applied here. No implementation until APPROVED.

- [ ] **Step 4: Commit the doc**

```bash
git add docs/process/designs/2026-10-15-f01-api-audit.md
git commit -m "docs: API audit and decisions for F01"
```

---

### Task 2: Implement the approved API changes

**Files:**
- Modify: per the design doc (expected: `afp/codepage.go`, `line/line.go`, possibly `afp/sfi.go`, plus audit findings)
- Test: existing suite; new failing-first tests for any behavior change the doc specifies

**Interfaces:**
- Consumes: the design doc's decision list (exact renames/removals).
- Produces: the narrowed API surface.

- [ ] **Step 1: Apply the mechanical renames/removals**

For each decision in the doc: unexport / remove / update callers (including `_test.go` files) / update doc comments that reference renamed identifiers. One commit per finding group (e.g. `refactor(afp): unexport CodePages map`, `refactor(line): unexport PinnedFonts`).

- [ ] **Step 2: For any behavior change the doc specifies, write the failing test first**

Example: if the dead-check removal changes `DecodeField`'s error for oversized input, a test pins the new contract before the removal lands. RED → GREEN per the TDD skill.

- [ ] **Step 3: Full verification**

Run: `test -z "$(gofmt -l .)" && go vet ./... && go test -count=1 -race ./...`
Expected: all clean/pass.

- [ ] **Step 4: Grep for stale references**

Run: `grep -rn "CodePages\b" --include="*.go" . | grep -v _test | grep -v "codePages"` (and the equivalents for each rename)
Expected: no stale references to removed identifiers outside the design doc.

---

## Self-Review

1. **Spec coverage:** R2P2-F01's four items map to audit sections a–d; F03's items are in the separate F03 plan (not this plan's scope).
2. **Step scan:** Task 1's steps are investigation + documentation (no code to TDD); Task 2's steps are mechanical with TDD only where behavior changes — proportionate.
3. **Type consistency:** Task 2 consumes the doc's exact rename list; no invented names.
4. **Review Focus:** caller-migration risk, dead-check removal risk, behavior-change discipline, and stale doc references are all covered by explicit steps.
5. **Proportion:** the plan is short because Task 1 does the thinking that Task 2 transcribes — the design doc, not this plan, carries the detail.
