# SDD Rulings Ledger — MCF font-mapping plan (2026-10-22)

Plan: `docs/process/plans/2026-10-22-mcf-font-mapping.md` (v2, wringer-validated)
Branch: `feat/mcf-font-mapping` (stacked on `feat/sec-ptoca-0x80`)
Range: `35ae8cf` → `aa5431d` (7 commits)
Mode: strict TDD; every task was implemented and then independently reviewed, plus one final whole-branch review and one fix wave.

Rulings are recorded as `Ruling: <what> — <why> — <cost if wrong>`.

## Pre-flight
- Ruling: No unresolved conflicts in the pre-flight scan; proceed to dispatch Task 1 — the task-pair interface table (Task 1→2 API shape, Task 2→3 diagnostic strings, Task 3→constraints) and per-task self-consistency checks were all clean — cost if wrong: a missed conflict would surface in the per-task review loop anyway.

## Task 1 (parser)
- Ruling: Accept the fail-closed extension found during implementation — duplicate LIDs *within* `Unsupported` also error, beyond the brief's two specified duplicate cases — the plan's duplicate rule covers ByLID-internal and cross-list; the same LID twice in two unsupported-form groups is the same MO:DCA uniqueness violation, and erroring is consistent with the plan's fail-closed mandate — cost if wrong: a malformed MCF that previously parsed now aborts, the same accepted trade as every other loud-duplicate path.
- Ruling: No CHANGELOG entry in Task 1 — the plan places the CHANGELOG under Task 3 (Unreleased → Added); the task correctly followed the brief's commit file list — cost if wrong: none, Task 3 covered it.
- Ruling: Drop the `"MCF: "` reason prefix — the brief mandates `Reason: "duplicate MCF local font ID %d"` exactly; downstream dictionary quotes diagnostic strings, so exactness wins over the contextual prefix — cost if wrong: cosmetic.
- Ruling: Narrow `Unsupported` membership to X'24'-carrying groups in *code* (struct doc stays as written) — an X'24'-less nameless group defaults to LID 0 which bypasses the map anyway, so tracking it is noise — cost if wrong: a nameless X'24'-less group becomes invisible to the unsupported diagnostic, but LID 0 never reaches that path.

## Task 2 (reducer wiring)
- Ruling: The concern raised during implementation ("later MCF replaces the earlier map" is documented-only, not test-pinned) was left to the review step rather than dispatched as an immediate fix — the plan specifies "(documented)"; the review loop is the net — cost if wrong: none; the final whole-branch review escalated it to Important and the fix wave pinned it with a mutant-killer test.

## Task 3 (docs)
- No rulings; review was clean (two unverifiable-but-not-defect notes taken on implementation evidence; the profile-file claim was hard-confirmed by Task 4's real-file run).

## Task 4 (acceptance)
- No rulings; PASS on first run.

## Final review fix wave
- Ruling: Handle all findings in a single fix pass rather than one pass per finding — per-finding passes each rebuild context and re-run suites at multiplied cost — cost if wrong: a finding could be misaddressed, but the single scoped re-review verifies each one individually.
- Ruling: Apply the final reviewer's recommendation 5 (BDT reset test pin) in the same wave as the Important findings — it's the same test file and the same fixture pattern; a separate round would cost a full re-review cycle for one test — cost if wrong: none, the scoped re-review covered it.

## Deferred / parked
- None. All findings across task reviews, the fix loop, and the final review were addressed; the re-review verdict is PASS with no new breakage.
