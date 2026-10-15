# F02 Inline Position Advancement — Design (rev 2, panel-amended)

Date: 2026-10-15. Status: APPROVED WITH CHANGES by adversarial panel
(Architecture Critic, Correctness Hawk, Feasibility Analyst, Devil's
Advocate — unanimous after debate). This revision incorporates all 9
panel findings. Supersedes the draft proposal of the same date.

## Problem

`DecodePTOCA` (afptoline/ptoca.go) tracks the inline position `st.ic`
through control sequences (AMI, RMI, SIM, BLN) but never advances it past
emitted text, so consecutive text runs in one PTX start at the same X
unless an explicit move intervenes. Documented v1 limitation
(ptoca.go:119-121, "applied by Task 8"). The blocker is gone:
`line.FontMetrics.Width` and `line.LookupFont` exist, and the reducer maps
SCFL local IDs via `fontIDForLID`.

## Design

**Placement: the decoder, unconditionally on the Reduce path.** PTOCA
presentation semantics say text advances the inline position; the state
machine owns `st.ic` and explicit moves interleave with text, so it does
the advancing. A reducer post-pass is provably ambiguous (overstrike-AMI:
many-to-one move→position mapping — conceded by the Devil's Advocate).
`Reduce`, `ReduceWithBudget`, and `ReduceWithCodePage` (all via `visitPTX`)
advance unconditionally — fulfilling the documented "applied by Task 8"
intent is not a silent contract change. `DecodePTOCA` (exported) is
unchanged and stays byte-for-byte compatible (no advancement).

**Hook site: `flush()`, gated on actual emission.** `doTRN`/`doRPS` never
finalize runs and the raw-bytes path bypasses them, so the hook lives in
`flush()` (ptoca.go:91), whose existing `len(st.cur) == 0` early return is
exactly the emission gate — covering raw bytes, TRN, and RPS, and excluding
suppressed or empty text. `flush()` gains an error return; all five call
sites propagate it.

**Unexported injection seam (no new exported API).** An exported
`AdvanceFunc`/`DecodePTOCAWithAdvance` for a single caller contradicts the
F01 API narrowing (YAGNI). Instead:

```go
// advanceFunc returns the inline advance width in points for text set in
// the given SCFL local font ID. Unexported: only the reducer supplies one.
type advanceFunc func(fontLID byte, text string) (float64, error)

func decodePTOCA(f afp.Field, cp afp.CodePage, maxBytes int, adv advanceFunc) ([]Run, error)
```

The A3 fail-fast budget is preserved by threading `maxBytes` alongside
`adv` (the reducer keeps calling with `st.textLeft`). `DecodePTOCA` calls
`decodePTOCA(f, cp, -1, nil)`. Tests use the internal variant (A3
precedent). The decoder stays font-agnostic; `adv` is stored on
`ptocaState`.

**Advance formula (sia folded in).** In `flush()`, after appending the run:

```
text   = st.cp.Decode(st.cur)          // already decoded for the run
w, err = st.adv(st.curFont, text)      // width in points; may fail (see errors)
reject w if NaN, ±Inf, or < 0         // width-domain contract, enforced at consumption
gaps   = max(runeCount(text) - 1, 0)   // intercharacter gaps; 0 for empty/single-char
st.ic += w*lUnitsPerPoint + float64(st.sia)*float64(gaps)
```

Framing (per the judge): the hook advance equals the decoder's tracked
x-delta over the run, so `sia`/`svi` handling falls out of decoder state
instead of being re-derived. `sia` is an int16 in L-units, signed —
negative values tighten spacing naturally. Boundary semantics: sia applies
per intercharacter gap (N−1 for N runes), not per character. `svi` stays
excluded: see "Documented limitations".

**Error contract.** Advance failures are Kind-tagged without field context;
`visitPTX` re-attaches identity, generalizing the `KindBudget` precedent
(reduce.go:269-270):

- Closure errors (unmapped SCFL ID from `fontIDForLID`; `LookupFont` false):
  `KindValidation`, empty FieldID — fail loudly, "v1 never guesses".
- Width-domain rejection (NaN/±Inf/negative): `KindValidation`, empty
  FieldID, raised in `flush()`.
- `visitPTX`: when the decoded error has empty FieldID, re-attach
  `FieldID: t.TypeID(), Offset: st.off` (decoder's own errors always set
  FieldID, so empty is a reliable discriminator).
- **Precedence (documented):** inside decode, the fail-fast byte budget in
  `text()` fires before any byte is accumulated, so it still preempts
  advance; a decode-time advance failure preempts the C1-control check and
  the `visitPTX`-level `textLeft` budget check, since decode runs first.
  Fail-loud either way; only the reported Kind changes for inputs that are
  both unadvancable and otherwise invalid.

**Wiring (reduce.go).** `visitPTX` passes a closure:

```go
adv := func(lid byte, text string) (float64, error) {
    id, err := fontIDForLID(lid, st.off)
    if err != nil { return 0, err }
    m, ok := line.LookupFont(id)
    if !ok { return 0, afp.DecodeError{Kind: afp.KindValidation, Reason: ...} }
    return m.Width(text, defaultFontSize), nil
}
runs, err := decodePTOCA(t, st.cp, st.textLeft, adv)
```

`LookupFont` returns a defensive copy (F01), so no aliasing. `Width` is in
points at `defaultFontSize` (10.0); `flush()` converts ×20 (`lUnitsPerPoint`).

## Documented limitations

- `svi` (variable-space character increment) remains unapplied. Bound: it
  affects only space-character advances, so maximum drift per run is
  `svi × (space count)` L-units. Sentinel design for later: track whether
  SVI was ever non-zero in the PTX and, if so, either apply per-space
  increments (needs char-class rulings on what counts as "space" post-decode)
  or fail loudly — do not silently widen the gap.
- SIA is applied per intercharacter gap (N−1), but v1 counts gaps adjacent
  to spaces too; per the PTOCA reference SIA inserts only between characters
  forming a word. Over-advance is 2×sia per space; nil at sia=0 (the
  initialized value). A mid-run SIA change applies the final value to all
  gaps of the merged run (SIA does not split runs).
- STO non-horizontal orientation: v1 assumes horizontal text; advancement
  along a rotated axis is out of scope (note, don't handle).
- The single-field wrapped-stream `finish()` offset residual is unaffected.

## Tests (TDD)

- SCFL + "Hi" + "there": second run's X == first X + Width("Hi")×20.
- Nil-advance (`DecodePTOCA`): byte-for-byte today's behavior.
- `sia` ≠ 0: advance includes sia×(runes−1); boundary: single-char run
  gets no sia term; negative sia tightens.
- Width-domain: closure returning NaN/−1 → KindValidation, FieldID/Offset
  re-attached by visitPTX.
- Unmapped SCFL ID: decode-time KindValidation preempts C1/budget checks
  (assert the Kind).
- Adversarial (kill the reducer-post-pass alternative): overstrike-AMI
  (text, AMI back, text) — second run does NOT shift by first run's width;
  BLN-to-margin — next line starts at margin, not advanced.
- Golden updates: `TestReduce_Statement` X expectations (run 2 ≈ 108pt),
  plus CHANGELOG.md entry and ptoca.go comment updates (119–121, sia
  comment, doRPS comment).

## Panel notes not adopted as requirements

- Exact `Reason` strings: implementer's TDD choice.
- Future exported advancing variant if an external caller materializes:
  YAGNI — decide then.
- Downstream migration: none in-repo beyond tests.
