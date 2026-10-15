# ADR 0001: Drop `context.Context` support

**Status:** Accepted (2026-10-15)
**Deciders:** Pratik Saptarshi, via brainstorming session

## Context

Round 2 panel finding ARCH-6 flagged that the library has no `context.Context`
support. The plan-review-integrator deferred it: adding `context.Context` is a
breaking API change across the decode paths with no agreed shape. A
brainstorming session was held to determine whether any caller scenario
justifies it.

The only scenarios identified were infrastructure-level: machine/container
resource exhaustion, or a hung process. There is no application-level
"user cancelled" story — this is a batch decoder.

## Options considered

### A. Drop context support (chosen)

- Pro: no breaking API change for a feature with no caller scenario. The
  failure modes it would gesture at are already covered: the fail-fast RPS
  text budget (PR #2) hard-bounds decode memory, and the C13 streaming
  iterator bounds input retention. A context cannot unstick a hung `Read` —
  only the reader itself can — so library-side cancellation checks would
  offer a false sense of safety unless every read were wrapped in a
  goroutine+select, which buys goroutine-leak risk for almost no gain.
- Con: the panel finding stays formally unaddressed. If a server-style use
  case (e.g., per-request deadlines in an AFP-to-PDF service) emerges, adding
  `ReduceContext` variants then is additive and cheap — and callers can
  already enforce timeouts around `Reduce` themselves today.

### B. Keep as deferred backlog

- Pro: keeps the option visible.
- Con: deferred-without-a-scenario is a slower drop — it rots and invites
  re-litigating a settled question.

### C. Add `ReduceContext` variants now

- Rejected: breaking (or at minimum surface-expanding) API change with no
  pull from any caller scenario; the scenarios named are not fixed by it.

## Decision

Drop `context.Context` support. Do not add context parameters to the decode
API. Resource-exhaustion scenarios are addressed by the RPS text budget and
the planned streaming iterator (C13), not by cooperative cancellation.

## Consequences

- `Reduce`, `ReduceWithBudget`, and `DecodePTOCA` keep their current
  signatures.
- If a genuine caller scenario emerges, revisit via additive
  `ReduceContext`-style variants — no existing callers break.
- Plan backlog entry R2P1-F06 updated from "defer pending scoping" to
  "dropped per this ADR".
