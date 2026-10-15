# C13 iterator contract — design (pending reviewer sign-off)

**Task:** C13 — stream input instead of `io.ReadAll` (ARCH-1).
**Status:** DESIGN — no implementation. Reviewer sign-off required before any code.
**Scope:** `afp/sfi.go` (`SplitFields`), `afptoline/reduce.go` (`ReduceWithBudget`),
`afp/triplets.go` (`RoundTrip`).

## Problem

`SplitFields` does `io.ReadAll(r)` and returns `[][]byte`: the entire input
materializes in memory. `ReduceWithBudget` buffers once more before that
(`io.ReadAll` + `IsWrapped` + `SplitFields(bytes.NewReader(data))`), so a
large AFP stream is held twice. The A3 fail-fast text budget bounds
per-field *expansion*; this task bounds input *retention*.

## Chosen shape: `iter.Seq2[StreamField, error]`

```go
// StreamField is one structured field yielded by Fields.
type StreamField struct {
    // Data holds the complete field bytes starting at the introducer
    // (the 5A record marker is stripped on wrapped streams).
    // The buffer is owned by the caller; the iterator never reuses it.
    Data []byte
    // Offset is the wire offset of the field's first wire byte: the 5A
    // marker on wrapped streams, the introducer otherwise.
    Offset int
}

// Fields returns an iterator over the structured fields of a MO:DCA
// stream, read incrementally: at most one field (plus a small read
// buffer) is retained at a time. It accepts naked and 5A-wrapped
// streams, exactly like SplitFields.
func Fields(r io.Reader) iter.Seq2[StreamField, error]
```

**Rationale.** Go 1.23+ range-over-func is the idiomatic streaming shape.
No goroutines or channels, so an early `break`/`return` (the normal path —
`Reduce` returns on the first error) leaks nothing and holds no locks.
Errors surface inline as the second yield value, so a mid-stream corrupt
field terminates iteration with the existing typed `afp.DecodeError`
kinds. `StreamField` carries the wire offset because `Reduce` needs true
stream offsets for diagnostics (see below).

**Rejected alternatives.**
- *Callback-based* (`ForEachField(r, func(StreamField) error)`): composes
  worse — early exit from nested consumer logic is awkward, and tests
  can't use plain `for-range`.
- *Channel-based*: requires a producer goroutine; a consumer that stops
  early leaks it (or needs explicit close coordination). Unnecessary
  machinery for a single-threaded parse.

**Name note:** `afp.Field` is the decoded-field interface and
`afp.RawField` is the decoded catch-all field type, so the yielded struct
is `StreamField`, not `Field` or `RawField`.

## Preservation contract

The iterator must reproduce `SplitFields`' observable behavior exactly,
except for documented items under "Accepted changes".

### Framing

- Wrap detection is identical to today: peek the first up-to-6 bytes once,
  apply the existing `IsWrapped` predicate (short input ⇒ not wrapped).
  Streams are homogeneous; the decision is stream-global.
- On wrapped streams, each record's leading `0x5A` is validated (C1) and
  stripped; the 2-byte record length doubles as SFLength, exactly as today.

### Error sequence (in order, per field)

`fieldStart` is the wire offset *including* the 5A marker on wrapped
streams (captured before stripping), matching today's `fieldStart := off`.

1. Read error at any point ⇒ `fmt.Errorf("afp: Fields: %w", err)`.
   (SplitFields keeps its own `"afp: SplitFields: %w"` text via the
   wrapper — see below.)
2. Wrapped and next byte ≠ `0x5A` ⇒
   `DecodeError{Kind: KindStructure, Offset: fieldStart, Reason: "expected 5A record marker"}`.
3. Fewer than 2 bytes remain ⇒
   `DecodeError{Offset: fieldStart, Kind: KindStructure, Reason: "truncated field length"}`.
4. `flen < 8` ⇒
   `DecodeError{Offset: fieldStart, Kind: KindStructure, Reason: "invalid field length %d: minimum 8"}`.
5. `flen > MaxFieldSize` ⇒
   `DecodeError{Offset: fieldStart, Kind: KindStructure, Reason: "field length %d exceeds MaxFieldSize %d"}`.
6. Fewer than `flen` bytes remain ⇒
   `DecodeError{Offset: fieldStart, Kind: KindStructure, Reason: "truncated field: need %d bytes, have %d"}`
   where `have` is the bytes actually available for this field.
7. Introducer invalid ⇒ today's `checkIntroducer` result: `DecodeError`
   from `DecodeIntroducer` with `Offset += fieldStart`.

Error *precedence* matters: truncation (6) is reported before introducer
validation (7), and the length bound checks (4–5) before truncation.
The recommended implementation reads the header first (8 bytes, plus
extension bytes when the ext bit is set and the claimed length permits,
capped at `flen`), runs the bound checks, then reads the body — which
preserves this order. No allocation is sized by an unvalidated claim:
the body buffer is allocated only after the `MaxFieldSize` bound check,
and the introducer is semantically validated (via the existing
`DecodeIntroducer` on the assembled header) before the body is trusted.

### Allocation / retention bound

Peak retained memory is O(one field + read buffer): each yielded `Data`
buffer is freshly allocated and ownership transfers to the consumer; the
iterator retains no prior fields. (Transient per-field allocation up to
`MaxFieldSize` for a field later rejected by introducer validation is
accepted — bounded, not retained. Today's code avoids even that by
validating before copying; the recommended header-first read above keeps
that property for the 32767..1MiB window too.)

### Byte-identical results

For valid inputs, the sequence of yielded `Data` buffers is byte-identical
to today's `SplitFields` output (same bytes, same order), and every error
above carries the same Kind/Reason/Offset as today.

## `SplitFields` disposition: keep as a collecting wrapper

```go
func SplitFields(r io.Reader) ([][]byte, error) {
    var out [][]byte
    for f, err := range Fields(r) { // internal split with the "SplitFields" op label for read-error text
        if err != nil { return nil, err }
        out = append(out, f.Data)
    }
    return out, nil
}
```

**Rationale:** `SplitFields` is exported API; removing it is a breaking
change for a convenience that tests and external users of this reference
library may rely on. Keeping it as a thin wrapper preserves its exact
contract — including the `"afp: SplitFields: %w"` read-error text
(implemented via an internal iterator parameterized by op label, or by
re-wrapping; implementer's choice, verified by the existing tests which
must pass unchanged).

## Caller migrations

- **`ReduceWithBudget`** (`afptoline/reduce.go`): replace the
  `io.ReadAll` + `IsWrapped` + `SplitFields(bytes.NewReader(data))`
  triple with `for f, err := range afp.Fields(r)`. Offset accounting
  becomes `st.off = f.Offset` at the top of each iteration (before
  `visit`), replacing the accumulate-and-add-one logic; the values are
  identical because `f.Offset` is the true wire offset. The `wrapped`
  local and the `bytes` import go away if unused.
- **`RoundTrip`** (`afp/triplets.go`): migrate to `Fields` as well so the
  fidelity gate itself streams. Terminal iterator error ⇒
  `fmt.Errorf("afp: RoundTrip: split: %w", err)` (unchanged); the
  per-field index for the re-encode-mismatch error becomes a loop counter
  (today it is the slice index — same numbering).

## Accepted changes (observable but approved in advance)

1. **Read-error text in `Reduce`.** Today a read failure surfaces as
   `afptoline: read input: <err>`; after migration it surfaces as
   `afp: Fields: <err>`. No test asserts the old text (verified). Error
   *kinds* are unaffected (read errors were never `DecodeError`).
2. **`SplitFields` on huge inputs** still materializes — it is documented
   as the convenience wrapper; `Fields` is the streaming path.

## Memory-ceiling test strategy

Proving "bounded retention" without materializing a huge input:

- `TestFields_BoundedMemory` (`afp`): a custom `io.Reader` that generates
  N valid naked fields *on demand* from a counter (e.g. N = 2_000_000
  minimal 8-byte fields — never materialized by the reader). Iterate to
  exhaustion, assert the count and per-field validity, and assert peak
  heap (via `runtime.ReadMemStats` with `GC` before/after) stays under a
  generous ceiling (e.g. 16 MiB against ~48 MiB of logical field data —
  materializing everything would exceed it several times over). Generous
  headroom keeps the test non-flaky; it is a smoke bound, not a precise
  measurement.
- `TestRoundTrip_BoundedMemory` (`afp`): same lazy reader through
  `RoundTrip` — proves the whole input path streams, including the
  fidelity gate.
- No `afptoline`-level memory test is required: reduction output
  (`line.Document`) inherently grows with document content, so input
  retention can't be isolated there. Behavioral equivalence of `Reduce`
  is covered by the existing suite, which must pass unchanged.

The test asserts the *ceiling*, not an actual huge allocation, per the
task's Done-when.

## Open questions for the reviewer

1. **Keep `SplitFields` as a wrapper (recommended) vs. remove it?**
   Removal is cleaner but breaking; the wrapper is 8 lines.
2. **Migrate `RoundTrip` to the iterator (recommended) vs. leave it on
   `SplitFields`?** Migration keeps the fidelity gate streaming;
   leaving it is less churn.
3. **Read-error text in `Reduce`** — accept `afp: Fields: <err>`
   (recommended) or preserve `afptoline: read input:` via re-wrap of
   non-`DecodeError` terminal errors?
4. **`IsWrapped` stays exported** even though `Reduce` no longer calls
   it (compat; it documents the framing predicate)? Recommended: yes.
5. **`StreamField` shape** — is `{Data []byte, Offset int}` the right
   minimal surface, or should the iterator also expose `Wrapped()`
   for consumers that need framing-aware accounting?
6. **Header-first read sketch** (header → bound checks → introducer
   validation → body alloc → body read): does it preserve the error
   precedence in §"Error sequence" in all edge cases the reviewer can
   construct (especially truncated-extension and 32767 < flen ≤ 1MiB)?
