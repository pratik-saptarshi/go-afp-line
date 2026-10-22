# Gap remediation (DAP Decision 0021) Implementation Plan — v2 (wringer-rewritten)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remediate, in the go-afp-line library, the mechanical gaps DAP Decision 0021 recorded against v0.1.0-beta.1 — 10 unregistered SFIDs aborting `DecodeField`, 6 of them additionally aborting `Reduce`, and the bare-EDT length strictness — so the owner-approved 0020 profile file decodes and reduces without abort. This is library improvement work; it is **not** the DAP adoption decision.

**Architecture:** Table-driven registry extension in `afp/field.go` (10 new rows reusing the existing `kindContracted`/`kindExcluded` → `*RawField` passthrough), accept-and-note cases for the 6 contracted SFIDs in `afptoline/reduce.go`'s `visitRaw` (the abort lives there, not just in the registry), and a BPG-style presence flag for the introducer-only EDT form so byte-identical re-encoding is preserved. No new dependencies, no API shape changes; the default contract (unknown SFID aborts) is preserved.

**Tech Stack:** Go 1.27, stdlib only.

**Spec:** DAP Decision 0021 — **Proposed, unmerged** record on DAP PR #184 (`review/dap-jo9-4-4-dependency-review` worktree: `docs/decisions/0021-go-afp-line-dependency-review.md`; PR: https://github.com/pratik-saptarshi/document-accessibility-pipeline/pull/184), §(e)+A2 — the 10 SFIDs, the bare-EDT finding, the DEFER recommendation; plus this repo's `docs/subset-contract.md` §2 (conflict rule: **IBM text wins; two corroborating open sources only where IBM is inaccessible**), §3 (in-scope fields), §9 (closed exclusion list). The plan argues from these; executors read both. Where this plan previously cited 0021 as settled, that was wrong — it is the best available record, not an accepted decision.

## Owner decision required (gate — resolve before Task 1)

Decision 0021 reserves the remediation path as an owner/product call: "(a) a registry/exclusion extension for the 10 SFIDs plus relaxing the EDT length rule, or (b) a profile-scoped pre-processing decision." This plan implements path (a). **Do not start Task 1 until Pratik explicitly authorizes path (a) by name.** Choosing (a) here does not decide DAP adoption — that stays with `dap-jo9.4.2`.

## Scope ruling

- This plan delivers library changes only. It does **not** close `dap-jo9.4.2`, does not authorize DAP adoption, and does not satisfy 0021 §(k)'s adoption preconditions (DAP-side integration test, fidelity comparison).
- Reduce-without-abort is a **mechanical** result. Per 0021 §(i), it is not fidelity evidence: a zero-exclusion count alone is not a fidelity result, and the 3 excluded object-container fields drop content with diagnostics. The completion report must carry this non-claim explicitly.
- Out of scope: any release tag (standing rule: no v0.x tags without explicit authorization — this plan ships no tag), fidelity validation, DAP-side work.

## Global Constraints

- Go 1.27. `gofmt` clean, `go vet ./...` clean, `go test ./...` green before every PR.
- CI also enforces `govulncheck` and a fuzz-smoke run; both must pass at the exact PR head.
- Conventional Commit subjects (`feat(afp): …`, `fix(afp): …`, `feat(afptoline): …`). Squash-merge PRs.
- Independent review + green exact-head CI before merge; check review comments, not just CI status.
- Test-first: failing test → minimal change → green → refactor. A test that passes with the guarded logic removed is rejected (mutation check per task).
- Contract conflict rule (`docs/subset-contract.md` §2): **IBM text wins.** The vendored primary-source copy `docs/_evidence/modca/MODCA-Reference-09.pdf` is checked first; two corroborating open sources (openafp, parse-afp, afplib tables under `docs/_evidence/afp-research/`) only where the primary is silent. Single-source entries stay out without a written owner waiver.
- `CHANGELOG.md` follows Keep a Changelog: every user-facing change gets an `[Unreleased]` entry.
- No new module dependencies. Process docs stay under `docs/process/`.
- Task 5 is DECISION-GATED: do not implement without Pratik's explicit go-ahead naming it.
- The real profile file `/home/hatch/workspace/afp-samples/health/01_Health_Coverage.afp` (12,784 bytes, SHA-256 `553a336db9283ebbf85092320b68c060aacdbea4138fa720810f09431f86b73a` — verified 2026-10-22) is **read-only test input**. It is outside both repos: use it for local verification, never commit it.

## Review Focus

1. A field carrying one of the 10 new SFIDs but with malformed framing (length mismatch) → must still be a typed `DecodeError`, never silently passed through. (Framing validation in `DecodeField` precedes registry dispatch — passthrough must not weaken it.)
2. Bare 8-byte EDT → accepted **and** re-encodes to the identical 8 bytes; EDT with 1–9 data bytes → still `DecodeError` KindStructure; BDT with 0 data bytes → still rejected. (Only EDT is relaxed; byte-identity is non-negotiable.)
3. The 4 excluded types (BOC/EOC/OCD/IOB) surface as `line.Exclusion` diagnostics through `Reduce`; the 6 contracted types reduce with zero exclusions and zero aborts.
4. An SFID in neither the old nor the new registry (e.g. `D3FFFF`) → still aborts with the exact existing `unknown structured field type` error. (Default contract preserved while Task 5 is ungated.)
5. `RoundTrip` byte-identity holds on a stream containing all 10 new SFIDs plus the bare EDT.

## File Structure

- Modify `afp/field.go` — `fieldRegistry` (+10 rows), `RawField` doc comment (add object containers), EDT struct (+`HasDocName` flag), `EDT.Encode()` (conditional emit), new `decodeEDTField` (BPG-precedent switch).
- Modify `afptoline/reduce.go` — `visitRaw`: accept-and-note cases for the 7 contracted SFIDs with position guards derived from the real census.
- Modify `afp/field_test.go` — table tests (internal `package afp`: no `afp.` qualifiers; follow the existing literal byte-slice fixture convention, e.g. `bgrExcluded`, `unknownField`).
- Modify `afptoline/reduce_test.go` — census-order stream test (reduce-level; needs `reduceState`).
- Modify `docs/subset-contract.md` — §3 gains the 7 contracted types (IBM-primary citations); §9 already lists BOC/EOC/OCD (no change).
- Modify `CHANGELOG.md` — `[Unreleased]` entries per task.

## Task 1: Register BOC/EOC/OCD as known-excluded

**Files:**
- Modify: `afp/field.go` (3 rows in the `kindExcluded` table; `RawField` doc comment)
- Modify: `afp/field_test.go` (table test + registry parity test)
- Modify: `CHANGELOG.md`

**Interfaces:**
- Consumes: `fieldRegistry`, `kindExcluded`, `DecodeField(b []byte) (Field, error)` (`afp/field.go`)
- Produces: `DecodeField` returns `*RawField{Excluded: true}` for `D3A892` (BOC), `D3A992` (EOC), `D3EE92` (OCD)

**Rationale (from spec):** `docs/subset-contract.md` §9 already lists "Object containers: BOC/EOC, OCD" in the closed exclusion list — the code registry is behind the contract. Conformance, not a judgment call.

- [ ] **Step 1: Write the failing test** `TestDecodeField_ObjectContainerExcluded` in `afp/field_test.go` (internal package — no `afp.` qualifiers; build field byte sequences with the file's existing literal-fixture convention):

```go
func TestDecodeField_ObjectContainerExcluded(t *testing.T) {
    // for SFIDs D3A892 (BOC), D3A992 (EOC), D3EE92 (OCD):
    // DecodeField(full field bytes) → err == nil, *RawField, Excluded == true
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./afp/ -run TestDecodeField_ObjectContainerExcluded -v`
Expected: FAIL — `DecodeError` "unknown structured field type" × 3.

- [ ] **Step 3: Register the three rows** in `buildRegistry()`'s `kindExcluded` table, and update the `RawField` doc comment's excluded-family list to include object containers:

```go
// Object containers (contract §9 — excluded)
{[3]byte{0xD3, 0xA8, 0x92}, "BOC"},
{[3]byte{0xD3, 0xA9, 0x92}, "EOC"},
{[3]byte{0xD3, 0xEE, 0x92}, "OCD"},
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./afp/ -run TestDecodeField_ObjectContainerExcluded -v`
Expected: PASS.

- [ ] **Step 5: Negative pins** — `TestDecodeField_UnknownStillAborts`: SFID `D3FFFF` still returns the exact `unknown structured field type: aborting (not in contracted subset)` error; and `TestDecodeField_NewTypeMalformedFraming`: a BOC field with a length mismatch still returns the `KindStructure` framing `DecodeError` (Review Focus #1).

Run: `go test ./afp/ -v`
Expected: PASS.

- [ ] **Step 6: Registry↔contract parity test** `TestRegistryMatchesContract` in `afp/field_test.go`: assert the registry's full SFID→(name, kind) set equals a hardcoded expected map in the test (reviewer checks the map against `docs/subset-contract.md` §3/§9). Cheap tripwire against future code/contract drift.

Run: `go test ./afp/ -run TestRegistryMatchesContract -v`
Expected: PASS.

- [ ] **Step 7: CHANGELOG + commit**

`### Fixed` under `[Unreleased]`: "Object-container fields BOC/EOC/OCD (contract §9) no longer abort the decode; they decode as excluded `RawField` passthrough."

```bash
git add afp/field.go afp/field_test.go CHANGELOG.md
git commit -m "feat(afp): register BOC/EOC/OCD as known-excluded passthrough"
```

## Task 2: Verify semantics and register the 7 structural SFIDs as contracted

**Files:**
- Modify: `afp/field.go` (7 rows in the `kindContracted` table, with source-citation comments)
- Modify: `afp/field_test.go` (table test + framing pin)
- Modify: `docs/subset-contract.md` (§3 gains 7 rows)
- Modify: `CHANGELOG.md`

**Interfaces:**
- Consumes: `fieldRegistry`, `kindContracted`; Task 1's passthrough behavior
- Produces: `DecodeField` returns `*RawField{Excluded: false}` for `D3A8A5` (BPF), `D3A9A5` (EPF), `D3A8CE` (BRS), `D3A9CE` (ERS), `D3A6AF` (PGD), `D3ABC3` (MDR), `D3AFC3` (IOB)

- [ ] **Step 1: Verify each SFID against the primary source first.** Open the vendored primary-source copy `docs/_evidence/modca/MODCA-Reference-09.pdf` (the contract's primary source per §2) and confirm each SFID's identity; fall back to two corroborating open sources (openafp, parse-afp, afplib under `docs/_evidence/afp-research/`) only where the primary is silent. Known resolutions to confirm, not assume: BPF/EPF = "Begin/End Print File" (primary); BRS/ERS per the primary win over openafp/parse-afp's "BR/ER" — record "BRS/ERS (IBM primary; opens say BR/ER)"; PGD/MDR/IOB are triple-sourced (openafp + parse-afp + afplib), contrary to 0021's stale single-source note. Record the outcome as a comment block above the new table rows, one line per SFID with its source. **IOB caveat:** confirm whether IOB references content-bearing objects in the profile file's usage — if it does, register IOB as `kindExcluded` instead and note the deviation explicitly. If any SFID cannot be sourced per the §2 rule, STOP and report — do not register it.
- [ ] **Step 2: Write the failing test** `TestDecodeField_StructuralContracted` — same shape as Task 1's test, asserting `err == nil`, `*RawField`, `Excluded == false` for all 7 SFIDs (internal package, existing fixture conventions).
- [ ] **Step 3: Run to verify it fails**

Run: `go test ./afp/ -run TestDecodeField_StructuralContracted -v`
Expected: FAIL with "unknown structured field type" × 7.

- [ ] **Step 4: Register the seven rows** in the `kindContracted` table with the Step 1 citation comments.
- [ ] **Step 5: Run to verify it passes**, plus the framing pin from Task 1's Step 5 pattern extended to one new contracted type (e.g. truncated BPF → `KindStructure` framing error, never silent passthrough).

Run: `go test ./afp/ -v`
Expected: PASS.

- [ ] **Step 5b: Extend the parity test.** Update `TestRegistryMatchesContract`'s expected map (Task 1 Step 6) with the 7 new SFID→(name, kindContracted) entries — otherwise the Task 1 test goes red.

Run: `go test ./afp/ -run TestRegistryMatchesContract -v`
Expected: PASS.

- [ ] **Step 6: Update `docs/subset-contract.md` §3** — add the 7 types (wire SFID, name, source citation), matching the existing row format.
- [ ] **Step 7: CHANGELOG + commit**

`### Added` under `[Unreleased]`: "Registered 7 structural SFIDs (BPF/EPF, BRS/ERS, PGD, MDR, IOB) as contracted passthrough with IBM-primary source citations."

```bash
git add afp/field.go afp/field_test.go docs/subset-contract.md CHANGELOG.md
git commit -m "feat(afp): register 7 structural SFIDs as contracted passthrough"
```

## Task 3: Accept the 6 contracted SFIDs in `Reduce`

**Files:**
- Modify: `afptoline/reduce.go` (`visitRaw` — new accept cases)
- Modify: `afptoline/reduce_test.go` (reduce-level test + census-order stream test)
- Modify: `CHANGELOG.md`

**Interfaces:**
- Consumes: `*afp.RawField` with the 6 contracted SFIDs from Task 2 (IOB went excluded per the Task 2 caveat); `visitRaw`'s existing MCF/PTD note-and-continue template (`reduce.go` ~line 347)
- Produces: `Reduce` accepts the 6 SFIDs without abort and without exclusions

**Rationale:** `DecodeField` returning `*RawField{Excluded:false}` is not the end of the pipeline — `visitRaw`'s `default` branch fails with "contracted field without v1 reduction semantics". Registry-only changes cannot make `Reduce` succeed. This task was missing from the v1 draft entirely.

- [ ] **Step 1: Dump the real census.** With a scratch driver (in `/tmp`, read-only input — never copy the profile file into the repo), run `SplitFields` over `/home/hatch/workspace/afp-samples/health/01_Health_Coverage.afp` and record the ordered SFID sequence with nesting-relevant positions (especially: BPF before BDT, EPF after EDT, and where BRS/ERS/PGD/MDR/IOB and BOC/EOC/OCD sit). Checkable result: the ordered 25-SFID list in the task notes. The stream test in Step 4 models this order — it is derived, not invented.
- [ ] **Step 2: Write the failing test** — reduce a synthetic stream in the Step 1 order containing all 10 new SFIDs plus the bare EDT:

```go
func TestReduce_ProfileCensusOrder(t *testing.T) {
    // stream in the Step-1 census order →
    // Reduce → err == nil; exclusions recorded exactly for BOC/EOC/OCD/IOB;
    // zero exclusions and zero aborts for the 6 contracted SFIDs.
}
```

- [ ] **Step 3: Run to verify it fails**

Run: `go test ./afptoline/ -run TestReduce_ProfileCensusOrder -v`
Expected: FAIL — `afp: decode error at offset 0 [validation] (field D3A8A5): contracted field without v1 reduction semantics`.

- [ ] **Step 4: Implement the accept cases** in `visitRaw`'s switch, following the MCF/PTD note-and-continue template. Position guards derived from the Step 1 census, recorded in code comments:
  - `D3A8A5` (BPF): accept when `!st.inDoc` — the print file opens before the document (field 0 precedes BDT); note-and-continue.
  - `D3A9A5` (EPF): accept when `!st.inDoc` — after EDT closed the document; note-and-continue.
  - `D3A8CE`/`D3A9CE` (BRS/ERS): accept when `!st.inDoc` — the census places them at fields 2 and 6, print-file level before BDT (field 8); note-and-continue.
  - `D3A6AF` (PGD), `D3ABC3` (MDR): accept when `st.inDoc` — the census places them at fields 11 and 12, inside the document — mirroring the MCF/PTD "environment field outside document" guard; note-and-continue. (If the Step 1 census dump disagrees with these positions, follow the dump and document why in the code comment.)
  - IOB (`D3AFC3`) is excluded, not contracted — it flows through `visitRaw`'s existing `t.Excluded` branch automatically; no accept case needed.
  - No new group tracking in v1: pairing validation for print-file/resource wrappers is a design decision beyond this fix; note-and-continue only.

- [ ] **Step 5: Run to verify it passes**

Run: `go test ./afptoline/ -v` (full package — the new cases must not disturb existing pairing validation)
Expected: PASS.

- [ ] **Step 6: CHANGELOG + commit**

`### Fixed` under `[Unreleased]`: "`Reduce` accepts the 6 newly contracted structural SFIDs (note-and-continue; no v1 content semantics) instead of aborting."

```bash
git add afptoline/reduce.go afptoline/reduce_test.go CHANGELOG.md
git commit -m "feat(afptoline): accept 7 contracted structural SFIDs in Reduce"
```

## Task 4: Accept introducer-only EDT with byte-identical re-encoding

**Files:**
- Modify: `afp/field.go` (EDT struct + `HasDocName`, `EDT.Encode()`, new `decodeEDTField`)
- Modify: `afp/field_test.go` (EDT length matrix + byte-identity test)
- Modify: `CHANGELOG.md`

**Interfaces:**
- Consumes: `decodeDocField` (kept strict, still used by BDT unchanged), `EDT`, `EDT.Encode()`
- Produces: bare 8-byte EDT → `*EDT{HasDocName: false}`; `EDT.Encode()` reproduces the exact 8-byte wire form when `HasDocName` is false

**Rationale:** the 0020 profile file contains a bare 8-byte EDT (field 23). The v1 draft's approach (empty DocName through the shared decoder) breaks `RoundTrip` byte-identity because `EDT.Encode()` unconditionally emits 10 data bytes. The BPG `HasPageName` precedent (`field.go` ~lines 96–116, `decodePageField` ~lines 413–432) is the codebase's own solution to exactly this problem — mirror it.

- [ ] **Step 1: Write the failing tests** in `afp/field_test.go` (internal package, existing fixture conventions):

```go
func TestDecodeField_BareEDT(t *testing.T) {
    // 1. bare EDT (8-byte introducer, zero data) → *EDT, err == nil, HasDocName == false.
    // 2. EDT.Encode() of that value → byte-identical to the 8-byte input.
    // 3. EDT with 4 data bytes → DecodeError KindStructure.
    // 4. bare BDT (8-byte introducer, zero data) → DecodeError KindStructure (BDT strictness unchanged).
    // 5. EDT with 10 data bytes → *EDT with HasDocName == true, as before.
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./afp/ -run TestDecodeField_BareEDT -v`
Expected: FAIL — bare EDT returns "need 10 data bytes".

- [ ] **Step 3: Implement.**
  - Add `HasDocName bool` to the `EDT` struct (comment mirrors BPG's: "true when the 8-byte name was present on the wire; required for byte-identical re-encoding").
  - `EDT.Encode()`: emit `DocName(8)+reserved(2)` only when `HasDocName`, mirroring `BPG.Encode()`.
  - Add `decodeEDTField`, mirroring `decodePageField`'s switch: `len(data) == 0` → `&EDT{…, HasDocName: false}`; `len(data) >= 10` → current `decodeDocField("EDT", …)` behavior with `HasDocName: true`; else → the existing `KindStructure` error. Wire EDT's registry row to it; BDT keeps `decodeDocField` untouched.
  - Self-consistent reading pinned: the `< 10` floor applies to all non-empty data — no `data[0:8]` slice is ever reached with fewer than 10 bytes.

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./afp/ -v`
Expected: PASS.

- [ ] **Step 5: CHANGELOG + commit**

`### Fixed` under `[Unreleased]`: "EDT accepts the introducer-only (8-byte) form and re-encodes it byte-identically via a `HasDocName` presence flag (BPG precedent); 1–9 data bytes still rejected; BDT strictness unchanged."

```bash
git add afp/field.go afp/field_test.go CHANGELOG.md
git commit -m "fix(afp): accept introducer-only EDT with byte-identical re-encoding"
```

**Known residual (documented, not blocking):** a stream truncated exactly at an EDT boundary (declared length 8, intended DocName lost) now decodes with `HasDocName=false` where it previously failed loudly. Framing checks still catch genuine truncation; the residual surface is narrow but real.

## Task 5 (DECISION-GATED): Opt-in lenient decode mode — DO NOT IMPLEMENT without Pratik's explicit go-ahead

**Gate:** Pratik must explicitly approve this task by name. It generalizes the fail-fast posture beyond the contracted subset — a product decision. If not granted, this task is dropped; Tasks 1–4 stand alone.

**Files (if gated):** `afp/field.go`, `afp/field_test.go`, `docs/subset-contract.md` (§9 generalization note), `CHANGELOG.md`.

**Interfaces (if gated):**
- Consumes: `DecodeField(b []byte) (Field, error)`, `fieldRegistry`
- Produces: `func DecodeFieldLenient(b []byte) (Field, error)` — identical to `DecodeField` except an unregistered SFID returns `&RawField{Intro, Data, Excluded: true}, nil` instead of the abort error. `DecodeField` behavior unchanged. Malformed framing still errors in both modes.

**Steps (if gated):** test-first per the established pattern — unknown SFID → `*RawField` Excluded=true via `DecodeFieldLenient` while `DecodeField` on the same input still aborts; contract §9 documents the mode; CHANGELOG entry.

## Test Plan (whole-plan)

- Per-task: `go test` RED then GREEN as specified in each task's steps; mutation check per the repo bar (revert the guarded logic, observe FAIL, restore).
- After Task 4: full `go test ./...`, `go vet ./...`, `gofmt -l .` empty — the repo's pre-PR gate.
- Real-file acceptance (runnable locally — no longer an external dependency): the hash-verified `/home/hatch/workspace/afp-samples/health/01_Health_Coverage.afp` (read-only; never commit it):
  - `SplitFields` → 25 fields, no error; `DecodeField` each → no aborts;
  - `Reduce` → completes; exclusions recorded exactly for the excluded set {BRG, ERG, BOC, EOC, OCD, IOB} (BRG/ERG are pre-existing excluded entries at fields 1 and 7);
  - `RoundTrip` → byte-identical (this is what caught the v1 draft's Task 3 defect — it stays in the gate).
- Fidelity non-claim: none of the above is fidelity evidence per 0021 §(i). The completion report states the mechanical result only.

## Acceptance Criteria

- [ ] AC1: `gofmt` clean, `go vet ./...` clean, `go test ./...` green; CI (build, fuzz-smoke, govulncheck) green at the exact PR head.
- [ ] AC2: All 10 SFIDs decode without abort — BOC/EOC/OCD/IOB with `Excluded=true`, the other 6 with `Excluded=false`; registry↔contract parity test green.
- [ ] AC3: `Reduce` completes on the census-order stream and on the real profile file. Synthetic stream → exclusions recorded exactly for BOC/EOC/OCD/IOB. Real file → exclusions recorded exactly for the full excluded set {BRG, ERG, BOC, EOC, OCD, IOB} (BRG/ERG are pre-existing `kindExcluded` entries at fields 1 and 7). Zero aborts in both.
- [ ] AC4: `RoundTrip` byte-identical on the real profile file, including the bare EDT.
- [ ] AC5: Bare 8-byte EDT decodes and re-encodes byte-identically; EDT with 1–9 data bytes still errors; BDT strictness unchanged.
- [ ] AC6: An SFID in neither registry (e.g. `D3FFFF`) still aborts with the exact existing error — default contract preserved.
- [ ] AC7: `CHANGELOG.md` `[Unreleased]` entries present; `docs/subset-contract.md` §3 updated for the 7 new types with source citations.
- [ ] AC8: Task 5 not implemented unless the gate was explicitly granted; if granted, `DecodeField` default behavior unchanged.
- [ ] AC9: Independent review APPROVED with no unresolved findings before merge (repo bar).
- [ ] AC10: Completion report states the mechanical result only — no fidelity claim, no adoption claim, no `dap-jo9.4.2` closure.
