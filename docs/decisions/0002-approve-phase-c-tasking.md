# ADR 0002: Approve Phase C tasking as planned

**Status:** Accepted (2026-10-15)
**Deciders:** Pratik Saptarshi

## Context

The plan-review-integrator (skill v2.1) classified the 23 remaining Round 2
panel findings: 15 bundled into Phase C tasks C1–C13, 7 deferred with
rationale, 1 informational. Traceability is in `integration_log.jsonl`.
The two open items were the `context.Context` scoping decision (resolved by
ADR 0001: drop) and approval of the Phase C tasking itself.

## Decision

Approve the Phase C tasking as written in
`docs/superpowers/plans/2026-10-15-panel-round2-findings.md`:

- **Parsing/encoding workstream** — C1 (5A marker validation), C2 (triplet
  overflow + stale introducer length; the encode-error contract is decided
  by implementer + reviewer, options documented in the task), C3 (CHAR
  encode fidelity), C4 (code page 500 via `ReduceWithCodePage`, default
  behavior byte-identical to `Reduce`).
- **Test hardening workstream** — C5 (RPS/BAG edge cases), C6
  (generator-vs-fixture byte-compare), C7 (reducer/PTOCA error-path
  coverage), C8 (fuzz targets beyond `afp`), C9 (`-shuffle` + coverage
  floor in CI).
- **CI/docs workstream** — C10 (SHA-pinned actions, `permissions` block +
  timeouts, push+pull_request double-run fix), C11 (poppler dependency
  documented), C12 (de-modulize `writerspike`, keeping evidence files —
  wrong-fix override of the verdict's "remove the module").
- **Architecture workstream** — C13 (streaming field iterator), sequenced
  last; the reviewer must sign off the iterator contract before
  implementation (governance gate).

Execution: one PR per workstream, stacked while predecessors are unmerged;
TDD with independent review per task, following the Phase A/B discipline.

## Consequences

- Phase C execution is queued on branch `fix/panel-round2-phase-c`.
- P2 items remain deferred backlog with the rationale recorded in the plan;
  they are not worsened by this plan.
- Any adjustment to tasking goes through the plan document first.
