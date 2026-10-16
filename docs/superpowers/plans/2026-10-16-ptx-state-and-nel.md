# PTX State Persistence + NEL Mapping Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix two correctness gaps in afptoline: (1) PTOCA decoder state (position, SIA, font, suppression) persists across PTX fields within a presentation text object instead of resetting per field; (2) EBCDIC NEL (0x15 → U+0085) maps to U+0020 instead of failing conversion with a C1 rejection.

**Architecture:** Task 1 is a one-line mapping in the decoder's `flush()`. Tasks 2–3 refactor the PTOCA decode loop into a `(*ptocaState).decode` method (no behavior change), then give the reducer a persistent `ptocaState` that is reset on BPT, discarded on EPT, and cleared on BPG (page boundary). Per-PTX run emission is preserved; only the carried state changes.

**Tech Stack:** Go (repo toolchain, see `~/sdk/go`), existing `afptoline` package. No new dependencies.

**Spec:** Pratik's approved decisions (2026-10-16), no separate spec doc — this plan is the authority:
- CQ3-5: persist PTOCA state across PTX fields (BPT resets, EPT discards, BPG clears); minimal version keeps per-PTX run emission.
- NEL: map U+0085 → U+0020 in `flush()` (applies to both `DecodePTOCA` and `Reduce`); true line-break semantics deferred.

## Global Constraints

- `gofmt -l` clean and `go vet ./...` clean after every task.
- TDD: write the failing test first, watch it fail for the expected reason, then minimal implementation.
- Full suite `go test ./...` (and `go test -race ./...` before commit) green after every task; name any failure in the report.
- Conventional Commits (`fix(afptoline): ...`).
- No new exported API; no changes to `DecodePTOCA`'s signature or behavior contract.
- Work on the plan's feature branch in the isolated worktree; never commit to `main`.

## Review Focus

1. Page boundary: PTX on page 2 after PTX on page 1 (no BPT anywhere) must NOT continue page 1's position — BPG clears the stream. Pinned by `TestReduce_BPGResetsPTXState` (Task 3).
2. Run-slice aliasing: `runs := ps.runs[n:]` followed by `ps.runs = nil` — runs handed to `visitPTX` must stay valid while later PTXs decode into a fresh array. Pinned by Task 3's cross-PTX test asserting both runs' text and X.
3. Budget: `ps.remaining` is set fresh from `st.textLeft` on every PTX visit — never carried across fields, never stale. Covered by the existing RPS/budget tests; the implementer must confirm they still pass.
4. NEL mapping must not alter other C1 handling: U+0085 → space, but U+0080 (and the rest of U+0080–U+009F) still rejected. Pinned by existing C1 tests + Task 1's test.
5. BPT resets the *whole* stream state (position, SIA/prevWord, font, suppress), not just position. Pinned by `TestReduce_BPTResetsPTXState` asserting the second BPT's first run starts at X=0.

## File Structure

- `afptoline/ptoca.go` — Task 1: `flush()` maps NEL→space (add `strings` import). Task 2: extract `func (st *ptocaState) decode(data []byte) error` from `decodePTOCA`; `decodePTOCA` becomes validation + fresh state + `decode`.
- `afptoline/reduce.go` — Task 3: `reduceState` gains `ptxSt *ptocaState`; `visitPTX` reuses it; BPT case resets it; EPT case discards it; BPG case clears it.
- `afptoline/ptoca_test.go` — Task 1 decoder-level test.
- `afptoline/reduce_test.go` — Task 1 Reduce-level test, Task 3 persistence/reset tests.

---

### Task 1: Map NEL (U+0085) to space in flush()

**Files:**
- Modify: `afptoline/ptoca.go` (`flush`, ~line 113; add `"strings"` import)
- Test: `afptoline/ptoca_test.go`, `afptoline/reduce_test.go`

**Interfaces:**
- Consumes: `st.cp.Decode(st.cur)` output in `flush()`.
- Produces: `flush()` maps every U+0085 to U+0020 post-decode, before run emission and advance-width computation. No signature changes; `visitPTX`'s C1 scan then never sees U+0085.

- [ ] **Step 1: Write the failing tests**

In `afptoline/ptoca_test.go` (helpers available: `testCP(t)`, `ptxField(payload)`, `trn(...)` from `ptoca_test.go:45` which builds `seq(0xDA, data...)`):

```go
func TestDecodePTOCA_NELMapsToSpace(t *testing.T) {
    // 0xC8='H' (CP037), 0x15=NEL→U+0085.
    runs, err := decodePTOCA(ptxField(trn(0xC8, 0x15, 0xC8)), testCP(t), -1, nil)
    if err != nil {
        t.Fatalf("decodePTOCA: %v", err)
    }
    if len(runs) != 1 || runs[0].Text != "H H" {
        t.Fatalf("got %q, want %q", runs[0].Text, "H H")
    }
}
```

In `afptoline/reduce_test.go` (helpers available: `docFields()` → bdt/edt, `rawField`, `tBPG/tEPG/tPTX`, `ebcdicSpaces`, `decodeErr`):

```go
func TestReduce_NELAccepted(t *testing.T) {
    // A PTX containing NEL must convert, not fail with KindEncoding.
    bdt, edt := docFields()
    bpg := rawField(tBPG, ebcdicSpaces(8))
    ptx := rawField(tPTX, trn(0xC8, 0x15, 0xC8))
    epg := rawField(tEPG, ebcdicSpaces(8))
    var buf bytes.Buffer
    for _, f := range [][]byte{bdt, bpg, ptx, epg, edt} {
        buf.Write(f)
    }
    doc, _, err := Reduce(&buf)
    if err != nil {
        t.Fatalf("Reduce: %v", err)
    }
    if got := doc.Pages[0].Runs[0].Text; got != "H H" {
        t.Errorf("text = %q, want %q", got, "H H")
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./afptoline/ -run 'TestDecodePTOCA_NELMapsToSpace|TestReduce_NELAccepted' -v`
Expected: FAIL — decoder test gets `"H\u0085H"`; Reduce test fails with `KindEncoding` C1 rejection.

- [ ] **Step 3: Implement the mapping in `flush()`** (`afptoline/ptoca.go`)

After `text := st.cp.Decode(st.cur)`, insert:
```go
// NEL (U+0085, EBCDIC 0x15) is the mainframe newline. It has no WinAnsi
// encoding, but rejecting it fails legitimate documents; map to space so
// text stays readable, searchable, and word-broken. (v0.x: true line-break
// semantics are a future design.)
text = strings.ReplaceAll(text, "", " ")
```
Add `"strings"` to the import block. The mapping sits before run emission and the `st.adv` width computation, so widths are computed on the mapped text.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./afptoline/ -run 'TestDecodePTOCA_NELMapsToSpace|TestReduce_NELAccepted' -v`
Expected: PASS. Then full suite: `go test ./...` and `go test -race ./afptoline/` — all green (existing C1 tests with U+0080 must still reject).

- [ ] **Step 5: Commit**

```bash
git add afptoline/ptoca.go afptoline/ptoca_test.go afptoline/reduce_test.go
git commit -m "fix(afptoline): map NEL (U+0085) to space instead of C1-rejecting"
```

---

### Task 2: Extract (*ptocaState).decode (refactor, no behavior change)

**Files:**
- Modify: `afptoline/ptoca.go`

**Interfaces:**
- Consumes: the decode loop body currently inside `decodePTOCA` (`afptoline/ptoca.go:199`–end of function: `data := rf.Data`, the `pos` loop, final `st.flush()`).
- Produces: `func (st *ptocaState) decode(data []byte) error` — runs the PTOCA decode loop over `data`, appending to `st.runs` and flushing at end; returns decode errors unchanged. `decodePTOCA(f, cp, maxBytes, adv)` keeps its signature and contract: validate `*afp.RawField` + D3EE9B, fresh `&ptocaState{cp: cp, remaining: maxBytes, adv: adv}`, `st.decode(rf.Data)`, return `st.runs`.

- [ ] **Step 1: Extract the method**

Move the loop (and only the loop — not field validation, not state construction) into `func (st *ptocaState) decode(data []byte) error`. `decodePTOCA` becomes:
```go
st := &ptocaState{cp: cp, remaining: maxBytes, adv: adv}
if err := st.decode(rf.Data); err != nil {
    return nil, err
}
return st.runs, nil
```
No test-first cycle here: this is a pure refactor whose net is the existing suite (dozens of `decodePTOCA`/`DecodePTOCA` callers in tests).

- [ ] **Step 2: Run the full suite to prove no behavior change**

Run: `go test ./...`
Expected: all green, identical to before the refactor.

- [ ] **Step 3: Commit**

```bash
git add afptoline/ptoca.go
git commit -m "refactor(afptoline): extract ptocaState.decode from decodePTOCA"
```

---

### Task 3: Persist PTOCA state across PTX fields

**Files:**
- Modify: `afptoline/reduce.go` (`reduceState` struct; `visitPTX`; `case "D3A89B"` BPT ~line 320; `case "D3A99B"` EPT ~line 325; `case *afp.BPG` ~line 229)
- Test: `afptoline/reduce_test.go`

**Interfaces:**
- Consumes: `func (st *ptocaState) decode(data []byte) error` from Task 2; `t.Data` on `*afp.RawField` (visitPTX is dispatched on `ptxTypeID == "D3EE9B"` at `reduce.go:273`, so no re-validation needed).
- Produces: `reduceState.ptxSt *ptocaState` — lazily created on first PTX, reset on BPT, discarded on EPT, cleared on BPG. `visitPTX` decodes each PTX's payload through the persistent state and takes only the newly appended runs.

- [ ] **Step 1: Write the failing tests**

`TestReduce_PTXStatePersistsAcrossFields` — BPT, PTX(TRN "HE"), PTX(TRN "WO"), EPT inside one page. Helpers available: `tBPT`/`tEPT` (`reduce_test.go:196-197`), `rawField`, `docFields`, plus `checkRuns(t, pageNo, got, want)` / `wantRun{text, x}` (`reduce_test.go:33`, X asserted ±1pt):
```go
// checkRuns with wantRuns: {"HE", 0}, {"WO", wantX > 0} — position carried
// across the PTX boundary. checkRuns asserts text byte-exact, which guards
// the runs-slice aliasing in the implementation (both runs' text intact).
// For wantX, compute the advance of "HE" via the same font metrics the
// reducer uses, or assert runs[1].X > runs[0].X with runs[0].X == 0.
```

`TestReduce_BPTResetsPTXState` — BPT, PTX(TRN "HE"), EPT, BPT, PTX(TRN "WO"), EPT: the second BPT's run starts at X == 0 (fresh stream).

`TestReduce_BPGResetsPTXState` — page 1: BPG PTX(TRN "HE") EPG; page 2: BPG PTX(TRN "WO") EPG: page 2's run starts at X == 0 (page boundary clears the stream).

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./afptoline/ -run 'TestReduce_PTXStatePersistsAcrossFields|TestReduce_BPTResetsPTXState|TestReduce_BPGResetsPTXState' -v`
Expected: FAIL — the persistence test gets `runs[1].X == 0` (state reset per PTX); the reset tests pass already (document that they are pinning current-correct behavior).

- [ ] **Step 3: Implement persistent state in `afptoline/reduce.go`**

1. Add to `reduceState`: `ptxSt *ptocaState // PTOCA decoder state carried across PTX fields; reset on BPT, discarded on EPT, cleared on BPG`.
2. BPT case (`"D3A89B"`), after `st.openGroup(grpBPT)`: `st.ptxSt = &ptocaState{}`.
3. EPT case (`"D3A99B"`), after `st.closeGroup() // grpBPT`: `st.ptxSt = nil`.
4. BPG case (`*afp.BPG`), after `st.openGroup(grpBPG)`: `st.ptxSt = nil` (new page = new coordinate space).
5. `visitPTX`: replace `runs, err := decodePTOCA(t, st.cp, st.textLeft, st.advanceFunc())` with:
```go
// PTOCA state persists across PTX fields: a text object split across PTX
// records is one continuous stream. BPT resets, EPT discards, BPG clears;
// a PTX outside any BPT lazily starts (and keeps) a stream.
if st.ptxSt == nil {
    st.ptxSt = &ptocaState{}
}
ps := st.ptxSt
ps.cp = st.cp
ps.adv = st.advanceFunc()
ps.remaining = st.textLeft // per-PTX budget, reset every field — never carried
n := len(ps.runs)
if err := ps.decode(t.Data); err != nil {
    // ... existing errors.As / DecodeError handling, unchanged ...
}
runs := ps.runs[n:]
ps.runs = nil
```
The existing `errors.As` rebase/budget/C1 handling below operates on `runs` and `err` exactly as before. Everything after (C1 scan, budget accounting, run append loop) is unchanged.

- [ ] **Step 4: Run tests to verify they pass**

Run the three new tests: expected PASS. Then `go test ./...` and `go test -race ./...`: all green (SIA ordering tests, RPS budget test, and the offset-rebase tests exercise this path heavily).

- [ ] **Step 5: Commit**

```bash
git add afptoline/reduce.go afptoline/reduce_test.go
git commit -m "fix(afptoline): persist PTOCA decoder state across PTX fields"
```

---

## Self-Review

1. **Spec coverage:** NEL→space — Task 1. State persistence incl. BPT reset / EPT discard / BPG clear — Task 3. Refactor prerequisite — Task 2. Covered.
2. **Step scan:** each test step names the test, file, helpers, and assertions; each code step names the exact signature/file/lines. The Task 2 refactor honestly notes the existing suite is the net rather than inventing a RED cycle for moved code.
3. **Type consistency:** `decode(data []byte) error` (Task 2) matches its use as `ps.decode(t.Data)` (Task 3); `reduceState.ptxSt *ptocaState` is the only new field; `runs := ps.runs[n:]` is `[]Run` as `visitPTX` expects.
4. **Review Focus:** all five items have owning tests (three new in Task 3, existing suite for budget/C1).
5. **Proportion:** plan is shorter than the code it changes; no function bodies transcribed except the 5-line visitPTX core the implementer cannot derive from the signature alone.
