# MCF Font Mapping Implementation Plan — v2

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build precise, fail-closed MCF font-mapping diagnostics plus the LID→coded-font-name parser infrastructure for future pinned-font decisions, and record the complete mapping landscape in the data dictionary. This plan does not make `Reduce` succeed on the 0020 profile file — the stream contains no MCF and the pinned table is synthetic-only by contract, so real-file success awaits his future product calls (real pinned fonts, silent-fallback policy).

**Architecture:** New `afp.ParseMCFMapping` parses MCF repeating groups into an `MCFMapping` (LID→name for X'02'/X'8E' groups; LIDs with other FQN forms tracked as unsupported, never silently dropped). `reduceState` carries `*MCFMapping` (nil until the first MCF; reset on BDT so maps never leak across documents). `fontIDForLID` resolves LID → name → `line.LookupFont`, failing loudly with one of three precise diagnostics (no MCF in stream / LID unmapped / unsupported identification form / unpinned font). The v1 "never guesses metrics" rule is preserved.

Why a parser instead of presence-only tracking: the LID→name map is the load-bearing artifact Pratik directed ("autonomous mapping in data dictionary") and the prerequisite for any future pinned-font decision; presence-only tracking would re-derive it later. The parser is explicitly infrastructure, not gap resolution.

**Tech Stack:** Go 1.27, stdlib only.

**Spec:** `docs/subset-contract.md` §8 (MCF maps local IDs; pinned synthetic metrics table; FOCA excluded; never silent fallback); MO:DCA Reference §"MCF (X'D3AB8A') Syntax" p. 229 (repeating groups: `RGLength UBIN[2]` including itself + triplets; LID from triplet X'24'); §"Resource Local Identifier Triplet X'24'" p. 368 (`Data[0]`=ResType — only X'05' Coded Font permitted in MCF — `Data[1]`=ResLID; Tlength fixed 4; architected default LID X'00' when absent); §"Fully Qualified Name Triplet X'02'" p. 341 (`Data[0]`=FQNType, `Data[1]`=FQNFmt, `Data[2:]`=FQName; X'8E'=Coded Font Name Reference; names are EBCDIC-encoded — v1 decodes as EBCDIC-037 per package convention).

## Global Constraints

- Go 1.27, stdlib only; `gofmt -l` empty, `go vet ./...` clean, `go test ./...` all green per task.
- TDD: failing test first for every behavior change; RED→GREEN→commit per task.
- Fail-closed is preserved: duplicate LIDs, bad ResType, unsupported identification forms, unmapped LIDs, and unpinned fonts are all loud diagnostics — never silent picks or fallbacks.
- No verbatim IBM/MO:DCA prose in the repo — paraphrase with source + page citation (repo policy; cf. the 2026-10-14 filter-branch).
- The 0020 profile file (`01_Health_Coverage.afp`) is read-only test input: use for local verification, never commit it.
- Conventional commits; no release tag.
- Branch `feat/mcf-font-mapping`, stacked on `feat/sec-ptoca-0x80`. If PR #20 is rejected or heavily reworked, rebase onto `origin/main`; Task 3 then creates `docs/ptoca-data-dictionary.md` containing only the font-mapping section.

## Review Focus

- MCF with zero repeating groups → zero `MCFMapping` (empty maps), nil error; a later SCFL LID≠0 fails via the no-mapping path.
- Duplicate LID across repeating groups → loud `KindStructure` "duplicate MCF local font ID" (MO:DCA: each group maps a *unique* LID; duplicates are malformed input, and picking one would be guessing).
- Repeating group with no X'24' → architected default LID X'00', applied per group (v1 deviation from the field-level phrasing, documented in the dictionary); two such groups → duplicate-LID error per the rule above. LID 0 always bypasses the map (existing default-font behavior, unchanged).
- FQN X'02'/X'8E' name bytes → EBCDIC-037 per the reference and package convention; the test pins it.
- MCF outside the document → the existing `inDoc` guard stays exactly as-is; parsing happens only after it passes. PTD (`D3B19B`) shares the case label — the parse must be conditioned on the MCF id so PTD keeps passing through untouched.

---

### Task 1: MCF repeating-group parser

**Files:**
- Create: `afp/mcf.go`
- Test: `afp/mcf_test.go`

**Interfaces:**
- Produces:
  ```go
  // MCFMapping is the parsed result of an MCF field's repeating groups.
  type MCFMapping struct {
      ByLID map[byte]string // local font ID → coded font name (X'02'/X'8E' groups)
      Unsupported []byte     // LIDs whose groups lack X'8E', in group order
  }
  func ParseMCFMapping(data []byte) (MCFMapping, error)
  ```
  - `data`: MCF field data after the introducer (verbatim bytes).
  - Per group: read `RGLength` as big-endian UBIN[2] at `pos`; require `RGLength >= 2` and `pos+RGLength <= len(data)` else `afp.DecodeError{Kind: afp.KindStructure}`; parse triplets from `data[pos+2:pos+RGLength]` with `decodeTriplets`.
  - X'24': require `len(Data)==2` and `Data[0]==0x05` (ResType), else `KindStructure`; LID = `Data[1]`. Absent X'24' → LID `0x00` (architected default, applied per group — v1's documented reading). A second X'24' in one group → `KindStructure`.
  - X'02': require `len(Data) >= 2` (FQNType/FQNFmt mandatory) and `Data[0]==0x8E`, else `KindStructure`; name = EBCDIC-037 decode of `Data[2:]`. A second X'02' in one group → `KindStructure`. Group with X'24' but no X'8E' → LID appended to `Unsupported` (group order preserved).
  - Duplicate LID in `ByLID`, or a LID appearing in both `ByLID` and `Unsupported` → `afp.DecodeError{Kind: afp.KindStructure, Reason: "duplicate MCF local font ID %d"}` (MO:DCA: each group maps a *unique* LID; picking one would be guessing).
  - Empty `data` → zero `MCFMapping` (non-nil empty maps), nil error.
  - Limitation (documented, not fixed): `decodeTriplets` aborts on unregistered triplet IDs (package fail-closed policy), so a group containing e.g. X'20' fails the whole parse loudly.

- [ ] **Step 1: Write the failing tests** in `afp/mcf_test.go` (triplet bytes fully specified per the reference; "FIX-MONO-10"/"FIX-PROP-10" are 11 chars → X'02' Tlength `0x0F`):
  - `TestParseMCFMapping`: group 1 = `00 15` + X'24' `[04, 24, 05, 01]` + X'02' `[0F, 02, 8E, 00, C6 C9 E7 60 D4 D6 D5 D6 60 F1 F0]` ("FIX-MONO-10" in EBCDIC-037); group 2 = `00 11` + X'02' `[0F, 02, 8E, 00, C6 C9 E7 60 D7 D9 D6 D7 60 F1 F0]` ("FIX-PROP-10"), no X'24'. Assert `ByLID == map[byte]string{1: "FIX-MONO-10", 0: "FIX-PROP-10"}` and `Unsupported` empty.
  - `TestParseMCFMapping_UnsupportedForm`: group with X'24' `[04, 24, 05, 03]` + X'02' FQN type X'84' (no X'8E') → `ByLID` empty, `Unsupported == []byte{3}`.
  - `TestParseMCFMapping_DuplicateLID`: two groups with X'24' LID 1 → expect `KindStructure` "duplicate MCF local font ID". Also: LID 3 in an X'8E' group and again in an X'84' group → same duplicate error (the rule covers both lists).
  - `TestParseMCFMapping_BadResType`: X'24' as `[04, 24, 06, 01]` → expect `KindStructure`. Also X'24' with 1-byte data → `KindStructure`.
  - `TestParseMCFMapping_ShortFQN`: X'02' with 1-byte data → expect `KindStructure` (FQNType/FQNFmt mandatory).
  - `TestParseMCFMapping_Truncated`: `RGLength` overrunning data → expect `KindStructure`.
  - `TestParseMCFMapping_Empty`: empty data → zero mapping, nil error.
- [ ] **Step 2: Run tests to verify they fail**

  Run: `go test ./afp/ -run TestParseMCFMapping -v` (from the worktree root, `PATH` including `$HOME/sdk/go/bin:$HOME/go/bin`)
  Expected: FAIL with "undefined: ParseMCFMapping"

- [ ] **Step 3: Implement** `MCFMapping` and `ParseMCFMapping` in `afp/mcf.go` per the interface above.

- [ ] **Step 4: Run tests to verify they pass**

  Run: `go test ./afp/ -run TestParseMCFMapping -v`
  Expected: PASS

- [ ] **Step 5: Commit**

  ```bash
  git add afp/mcf.go afp/mcf_test.go
  git commit -m "feat(afp): parse MCF repeating groups to LID->coded-font-name mapping"
  ```

### Task 2: Reducer wiring — MCF-aware fontIDForLID

**Files:**
- Modify: `afptoline/reduce.go` (reduceState struct ~line 146-160, BDT case ~line 221, D3AB8A case ~line 347, `fontIDForLID` ~line 496, `advanceFunc` closure ~line 476)
- Modify: `afptoline/reduce_test.go` (existing `TestReduce_BNGENGAndMCFValid` fixture — see Step 3)
- Test: `afptoline/mcf_test.go` (new file; follow the fixture construction pattern in `afptoline/reduce_test.go`, e.g. the SCFL test near line 1055)

**Interfaces:**
- Consumes: `afp.ParseMCFMapping` / `afp.MCFMapping` from Task 1.
- Produces: `func fontIDForLID(lid byte, mcf *MCFMapping, offset int) (string, error)`
  - `lid == 0` → `(defaultFontID, nil)` — unchanged, bypasses the map.
  - `mcf == nil` → `afp.DecodeError{Kind: afp.KindValidation, Offset: offset, Reason: fmt.Sprintf("SCFL local font ID %d has no MCF mapping (no MCF in stream)", lid)}`
  - `name, ok := mcf.ByLID[lid]; ok` → `line.LookupFont(name)`; miss → `afp.DecodeError{Kind: afp.KindValidation, Reason: fmt.Sprintf("coded font %q (MCF local font ID %d) has no pinned metrics", name, lid)}`
  - `slices.Contains(mcf.Unsupported, lid)` → `afp.DecodeError{Kind: afp.KindValidation, Reason: fmt.Sprintf("MCF local font ID %d uses unsupported font identification (only X'8E' coded-font-name references are mapped)", lid)}`
  - else → `afp.DecodeError{Kind: afp.KindValidation, Offset: offset, Reason: fmt.Sprintf("SCFL local font ID %d has no MCF mapping (MCF present, LID unmapped)", lid)}`
- `reduceState` gains field `mcf *MCFMapping` (nil until the first MCF is parsed).
- BDT case: reset `st.mcf = nil` alongside the existing document-open state (maps never leak across documents).
- D3AB8A case: split from the shared `D3B19B` (PTD) label or guard the parse on `id == "D3AB8A"`; keep the existing `inDoc` guard untouched; after it passes, `m, err := afp.ParseMCFMapping(t.Data)` (`t` is the `*afp.RawField`; on err return err), then `st.mcf = &m`. A later MCF in the same document replaces the earlier map (documented).
- `advanceFunc` closure: pass `st.mcf` into `fontIDForLID`; remove the now-dead `line.LookupFont` block it replaces (keep the `fontIDForLID` call as the single lookup path).

- [ ] **Step 1: Write the failing tests** in `afptoline/mcf_test.go`:
  - `TestReduce_MCFMappedFont`: synthetic stream BDT → MCF (one group: X'24' `[04, 24, 05, 01]` + X'02'/X'8E' "FIX-MONO-10") → BPG → BPT → PTX (SCFL LID 1, then TRN "A") → EPT → EPG → EDT, with legal pairing per the existing reduce tests. Assert `Reduce` returns no error.
  - `TestReduce_MCFUnmappedLID`: same stream without the MCF field → expect error containing `SCFL local font ID 1 has no MCF mapping (no MCF in stream)`.
  - `TestReduce_MCFUnpinnedFont`: MCF maps LID 1 → "NOPE-NOT-PINNED", SCFL LID 1 → expect error containing `coded font "NOPE-NOT-PINNED" (MCF local font ID 1) has no pinned metrics`.
  - `TestReduce_MCFUnsupportedForm`: MCF group with X'24' LID 2 + X'02'/X'84' (no X'8E'), SCFL LID 2 → expect error containing `uses unsupported font identification`.
- [ ] **Step 2: Run tests to verify they fail**

  Run: `go test ./afptoline/ -run 'TestReduce_MCF' -v`
  Expected: FAIL — `TestReduce_MCFMappedFont` fails with the current "has no pinned metrics mapping (MCF unmapped)" error; the others fail on message mismatch.

- [ ] **Step 3: Implement** the `reduceState.mcf` field, the BDT reset, the split D3AB8A case with parse-and-store, and the new `fontIDForLID` signature/logic in `afptoline/reduce.go`. Restructure `advanceFunc` so `fontIDForLID` is the existence check and the closure still calls `line.LookupFont(id)` for the metrics (now infallible by construction — remove only the redundant miss-branch, not the metrics lookup). Also update the existing `TestReduce_BNGENGAndMCFValid` fixture (`afptoline/reduce_test.go:1118`): it feeds an in-document MCF of 8 EBCDIC spaces and asserts nil error, but post-wiring those bytes parse as `RGLength=0x4040` → overrun abort; change the fixture to empty MCF data, which preserves the test's "accepted, not an abort" intent via the empty→zero-mapping rule.

- [ ] **Step 4: Run tests to verify they pass**

  Run: `go test ./afptoline/ -run 'TestReduce_MCF' -v`, then `go test ./...`
  Expected: PASS, full suite green (the existing LID-5 loud-error test still passes — it asserts the substring `SCFL local font ID 5`, which the new no-MCF diagnostic contains).

- [ ] **Step 5: Commit**

  ```bash
  git add afptoline/reduce.go afptoline/mcf_test.go
  git commit -m "feat(afptoline): resolve SCFL local font IDs through MCF to pinned metrics"
  ```

### Task 3: Data dictionary font-mapping section + contract touch-up

**Files:**
- Modify: `docs/ptoca-data-dictionary.md` (add `## Font mapping (MCF / SCFL)` section; if PR #20 was rebased away per the branch contingency, create the file with only this section)
- Modify: `docs/subset-contract.md` (§8, one-line touch-up)
- Modify: `CHANGELOG.md` (Unreleased → Added)

**Interfaces:**
- Consumes: `afp.ParseMCFMapping` / `MCFMapping` behavior (Task 1), `fontIDForLID` resolution chain + the four diagnostic strings (Task 2), pinned fonts table (`line/line.go`: FIX-MONO-10, FIX-PROP-10 — synthetic fixtures only).
- Produces: dictionary section documenting —
  - MCF repeating-group structure: `RGLength UBIN[2]` + triplets; LID from X'24' (`Data[1]`, ResType X'05' validated; default X'00' per group, documented as v1's reading); name from X'02'/X'8E' (`Data[2:]`, EBCDIC-037); duplicate LIDs are a loud error (MO:DCA: unique LIDs); groups with other FQN forms are tracked as unsupported, never silently skipped (MO:DCA Reference MCF Syntax p. 229; FQN X'02' p. 341; X'24' p. 368 — paraphrase, cite, no verbatim prose);
  - the resolution chain: SCFL LID → MCF map → coded font name → `line.LookupFont` pinned metrics; LID 0 bypasses the map (default font); maps reset at BDT (no cross-document leakage); later MCF replaces earlier;
  - the pinned-fonts table as the autonomous mapping (name → synthetic metrics note);
  - the fail-closed rule: duplicate LID, bad ResType, unmapped LID, unsupported form, and unpinned font are all loud diagnostics — v1 never guesses metrics;
  - the 0020 profile file's actual state: the stream contains **no MCF**; its SCFL LID 1 therefore fails with the no-MCF-mapping diagnostic;
  - rejected alternative (explicitly not implemented): silent fallback to default metrics for unmapped LIDs — needs his product call.

- [ ] **Step 1: Write the dictionary section and contract touch-up**

  Contract §8 touch-up (one line, factual): MCF is now parsed to an LID→coded-font-name mapping (was "noted"); resolution is against the pinned table. Keep the FOCA-exclusion and never-silent-fallback lines untouched.

- [ ] **Step 2: Verify every claim** against the merged code: re-read `afp/mcf.go`, `fontIDForLID`, and `line/line.go` pinned table; each dictionary sentence must trace to code or the cited reference section.

- [ ] **Step 3: Commit**

  ```bash
  git add docs/ptoca-data-dictionary.md docs/subset-contract.md CHANGELOG.md
  git commit -m "docs: font-mapping section in data dictionary; contract MCF touch-up"
  ```

### Task 4: Acceptance verification

**Files:** none committed (scratch only); then confirm the plan file's own hygiene.

- [ ] **Step 1: Real-file scratch verification** (in `/tmp`, read-only input — never copy the profile file into the repo):

  Run a scratch driver calling `afptoline.Reduce` on the hash-verified `01_Health_Coverage.afp`.
  Expected: Reduce fails with a `KindValidation` diagnostic containing `SCFL local font ID 1 has no MCF mapping (no MCF in stream)` — proving it proceeds past the old 0x80 abort AND past the old vague message, and stops exactly at the documented unmapped-LID gap.

- [ ] **Step 2: Full gate**

  Run: `gofmt -l .` (expect empty), `go vet ./...` (expect clean), `go test ./...` (expect all packages ok).

- [ ] **Step 3: Confirm the plan's scope boundary and hygiene**

  Re-read the final diff (`git diff feat/sec-ptoca-0x80...HEAD --stat` from the worktree, or against `origin/main` after the contingency rebase): it must contain only `afp/mcf.go`, `afp/mcf_test.go`, `afptoline/reduce.go`, `afptoline/mcf_test.go`, `docs/ptoca-data-dictionary.md`, `docs/subset-contract.md`, `CHANGELOG.md`, and this plan file. No MCF1/FOCA handling, no silent fallbacks, no profile-file bytes. Scrub this plan file's `REQUIRED SUB-SKILL` agent header before the PR merges (repo policy — Codex P1'd it on PR #19).

## Test Plan

- `go test ./afp/ -run TestParseMCFMapping -v` — parser: two-group map, unsupported-form tracking, duplicate LID (incl. cross-list) → KindStructure, bad ResType / short X'24' / short X'02' → KindStructure, truncated group → KindStructure, empty → zero mapping.
- `go test ./afptoline/ -run 'TestReduce_MCF' -v` — mapped LID resolves (Reduce completes on fixture); the three miss diagnostics fire with their exact strings; PTD still passes through (existing PTD fixture test, if any, stays green).
- `go test ./...` — no regressions (existing SCFL/font tests, incl. the LID-5 loud-error test and the updated `TestReduce_BNGENGAndMCFValid`, still pass).
- Scratch: real-file Reduce → `SCFL local font ID 1 has no MCF mapping (no MCF in stream)` diagnostic.

## Acceptance Criteria

1. `afp.ParseMCFMapping` exists with the exact signature above; unit tests pin the X'24' (`Data[1]`, ResType X'05') and X'02'/X'8E' (`Data[2:]`, EBCDIC-037) extraction, the X'00' default, duplicate (incl. cross-list) / bad-ResType / short-triplet / truncation errors, unsupported-form tracking, and empty input.
2. `fontIDForLID` resolves LID 0 → default, mapped LID → pinned font, and fails loudly with the exact diagnostic strings above for the three miss cases; `mcf` resets at BDT; PTD parsing is untouched.
3. The data dictionary's font-mapping section traces every claim to code or cited reference; the rejected silent-fallback alternative is recorded as needing his product call.
4. Real-file Reduce reaches the SCFL LID 1 gap with the new precise diagnostic (no 0x80 abort, no vague message).
5. `gofmt`/`vet`/full suite green; diff contains only `afp/mcf.go`, `afp/mcf_test.go`, `afptoline/reduce.go`, `afptoline/mcf_test.go`, `afptoline/reduce_test.go` (fixture update), `docs/ptoca-data-dictionary.md`, `docs/subset-contract.md`, `CHANGELOG.md`, and this plan file; plan header scrubbed before PR merge.
