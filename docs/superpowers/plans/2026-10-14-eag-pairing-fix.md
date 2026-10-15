# BAG/EPG Pairing Fix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the Codex P2 review comment on go-afp-line PR #1 ("Reject BAG groups that cross EPG") by making `EPG` fail when a BAG group is still open, restoring the reducer's begin/end pairing guarantee.

**Architecture:** One guard change in `afptoline/reduce.go` plus a regression test. The malformed stream `BDT, BPG, BAG, EPG, EAG, EDT` is currently accepted because `EPG` does not check `st.bag`. The fix (the review comment's first suggestion, matching its title "Reject BAG groups that cross EPG"): `EPG` fails when a BAG group is still open. This alone closes every reachable hole — any `EAG` with `st.bag == 1` and no open page would require an `EPG` to have crossed the open BAG, which the new guard forbids — so no `EAG` change is needed (YAGNI). An unclosed `BAG` at stream end remains caught by `finish()`'s "unclosed group" check as a backstop.

**Tech Stack:** Go (existing module `github.com/pratik-saptarshi/go-afp-line`), existing test helpers in `afptoline/reduce_test.go`.

**Spec:** Codex review comment on PR #1 (inline, `afptoline/reduce.go:157`): "Reject BAG groups that cross EPG — Now that this guard permits `BAG` inside a page, a malformed `BDT, BPG, BAG, EPG, EAG, EDT` stream is accepted: `EPG` closes and appends the page without checking `st.bag`, and the later `EAG` clears the counter even though no page is open, allowing `EDT` to succeed. This breaks the reducer's stated begin/end pairing guarantee; require the active environment group to close before `EPG` (and/or require an open page for `EAG`)." Plus the subset contract walk `BPG → BAG … EAG … EPG` (BAG/EAG inside an open page).

## Global Constraints

- Work in the existing worktree `/home/hatch/workspace/repos/go-afp-line/.worktrees/fix-afp-panel-p0` on branch `fix/afp-panel-p0` (the PR #1 branch). Do not create a new branch.
- Go toolchain: `export PATH="$HOME/sdk/go/bin:$HOME/go/bin:$PATH"`.
- Conventional Commits subjects (`fix(afptoline): ...`).
- TDD: failing test first, then the guard change.

## Review Focus

- `BDT, BPG, BAG, EPG, EAG, EDT` must now fail at `EPG` (pinned by this task's test, Case 1).
- `BDT, BPG, BAG, EPG, EDT` (unclosed BAG, no EAG) must still fail — now at `EPG` instead of `finish()` (pin as Case 2 in the same test).
- The valid contract walk `BDT, BPG, BAG, EAG, EPG, EDT` must keep passing (covered by existing `TestReduce_BAGInsidePage`; do not break it).
- No `EAG` guard change: with the `EPG` guard, an `EAG` with `st.bag == 1` and no open page is unreachable (it would require an `EPG` to have crossed the open BAG). The reviewer should confirm this reachability argument.
- Adjacent gap, explicitly out of scope: `BPT` has the same shape (`BDT, BPG, BPT, EPG, EDT` is caught only by `finish()`'s unclosed-group check). The review comment is about BAG; do not expand scope.

---

### Task 1: Reject BAG groups that cross EPG

**Files:**
- Modify: `afptoline/reduce.go` (EPG guard, currently checks only `st.page == nil`)
- Test: `afptoline/reduce_test.go` (new `TestReduce_BAGMustNotCrossEPG`, next to `TestReduce_BAGInsidePage` at line 249)

**Interfaces:**
- Consumes: existing test helpers `rawField`, `tBDT`, `tBPG`, `tBAG`, `tEAG`, `tEPG`, `tEDT`, `ebcdicSpaces`, and `Reduce(&buf)` returning `(doc, _, err)` — all defined in `afptoline/reduce_test.go`.
- Produces: the tightened `EPG` guard; no new exported API.

- [ ] **Step 1: Write the failing test** `TestReduce_BAGMustNotCrossEPG` in `afptoline/reduce_test.go`, following the structure of `TestReduce_BAGInsidePage` (lines 249-268):

```go
func TestReduce_BAGMustNotCrossEPG(t *testing.T) {
	bdt := rawField(tBDT, append(ebcdicSpaces(8), 0x00, 0x00))
	bpg := rawField(tBPG, ebcdicSpaces(8))
	bag := rawField(tBAG, nil)
	eag := rawField(tEAG, nil)
	epg := rawField(tEPG, ebcdicSpaces(8))
	edt := rawField(tEDT, append(ebcdicSpaces(8), 0x00, 0x00))

	// Case 1: BAG group crossing EPG — must fail at EPG.
	var buf bytes.Buffer
	for _, f := range [][]byte{bdt, bpg, bag, epg, eag, edt} {
		buf.Write(f)
	}
	if _, _, err := Reduce(&buf); err == nil {
		t.Fatalf("Reduce accepted BAG group crossing EPG")
	} else if !strings.Contains(err.Error(), "EPG") {
		t.Fatalf("error %q does not name EPG", err)
	}

	// Case 2: BAG never closed — must fail at EPG.
	buf.Reset()
	for _, f := range [][]byte{bdt, bpg, bag, epg, edt} {
		buf.Write(f)
	}
	if _, _, err := Reduce(&buf); err == nil {
		t.Fatalf("Reduce accepted stream with unclosed BAG")
	}
}
```

(`strings` and `bytes` are already imported in the test file — verify before adding imports.)

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./afptoline/ -run TestReduce_BAGMustNotCrossEPG -v`
Expected: FAIL at Case 1 (`Reduce accepted BAG group crossing EPG`).

- [ ] **Step 3: Tighten the EPG guard** in `afptoline/reduce.go`:

```go
case *afp.EPG:
    if st.page == nil {
        return st.fail(t.TypeID(), "EPG without BPG")
    }
    if st.bag != 0 {
        return st.fail(t.TypeID(), "EPG with open BAG group")
    }
    st.pages = append(st.pages, *st.page)
    st.page = nil
    return nil
```

Only the `st.bag != 0` check is new; the rest of the case is unchanged.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./afptoline/ -v -run 'TestReduce'`
Expected: PASS, including the pre-existing `TestReduce_BAGInsidePage` (valid walk unaffected).
Then: `go vet ./afptoline/`, `gofmt -l afptoline/` (empty output), and `go test -count=1 ./...` for the whole module.

- [ ] **Step 5: Commit and push** (branch `fix/afp-panel-p0`, worktree `/home/hatch/workspace/repos/go-afp-line/.worktrees/fix-afp-panel-p0`)

```bash
git add afptoline/reduce.go afptoline/reduce_test.go
git commit -m "fix(afptoline): reject BAG groups that cross EPG

Closes the Codex P2 on PR #1: BDT, BPG, BAG, EPG, EAG, EDT was accepted
because EPG closed the page without checking st.bag. EPG now fails
when a BAG group is still open, restoring the begin/end pairing
guarantee."
git push origin fix/afp-panel-p0
```
