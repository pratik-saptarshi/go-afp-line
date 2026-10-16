# Example Tests + Package Comment (F03) Implementation Plan

**Goal:** Add runnable example tests for the library's main entry points and fix the duplicate package comment in `afptoline` (deferred P2 R2P2-F03).

**Architecture:** Docs-only change: no production code is touched. `Example` functions with `// Output:` comments go in `example_test.go` files beside the packages they demonstrate (Go convention — examples appear on pkg.go.dev); the duplicate `// Package afptoline` comment is removed from `ptoca.go`, keeping `reduce.go`'s.

**Tech Stack:** Go testing, `testing` package examples.

**Spec:** `docs/process/plans/2026-10-15-panel-round2-findings.md`, Backlog entry R2P2-F03 ("Example tests, duplicate package comment in `afptoline` — defer — trivial; bundle into any future test task"). Zero `Example` functions exist today (verified 2026-10-15); the duplicate comments are at `afptoline/ptoca.go:1` and `afptoline/reduce.go:1`.

## Global Constraints

- Go 1.27 toolchain; no new module dependencies.
- Conventional Commits subjects; identity `automation <waynemanorsaptarshi@gmail.com>`.
- Examples must be runnable by `go test` and deterministic (no map iteration order, no timing).
- Never force-push; work on branch `chore/deferred-f03-examples` from `origin/main`.

## Review Focus

- Example output mismatching actual output (whitespace, ordering): every example has an exact `// Output:` comment verified by `go test`.
- Examples that duplicate existing unit tests without teaching anything: each example must demonstrate a usage pattern (the happy path a new user copies), not re-assert an edge case.
- The package comment removal breaking `golint`-style checks: exactly one `// Package afptoline` comment must remain (in `reduce.go`).

---

## File Structure

- Modify `afptoline/ptoca.go` — delete line 1 (`// Package afptoline reduces AFP structures to line data.`), keeping the rest of the file's doc comments intact.
- Create `afp/example_test.go` — `ExampleFields`: iterate a two-field naked stream, print field count and first field's offset.
- Create `afptoline/example_test.go` — `ExampleReduce`: reduce a minimal one-page AFP document (BDT/BPG/PTX/BAG.../EPG/EDT from existing test fixtures), print page count and run count.

---

### Task 1: Fix the duplicate package comment

**Files:**
- Modify: `afptoline/ptoca.go` (delete line 1 only)
- Test: `go vet ./afptoline/` + grep assertion

**Interfaces:**
- Consumes: nothing.
- Produces: single package comment in `afptoline` (in `reduce.go`).

- [ ] **Step 1: Delete the duplicate comment**

Remove line 1 of `afptoline/ptoca.go` (`// Package afptoline reduces AFP structures to line data.`). Do not touch any other comment in the file.

- [ ] **Step 2: Verify exactly one package comment remains**

Run: `grep -rn "^// Package afptoline" afptoline/*.go`
Expected: exactly one line — `afptoline/reduce.go:1`.

- [ ] **Step 3: Run vet and the package tests**

Run: `go vet ./afptoline/ && go test -count=1 ./afptoline/`
Expected: both clean/pass.

- [ ] **Step 4: Commit**

```bash
git add afptoline/ptoca.go
git commit -m "docs: remove duplicate package comment in afptoline"
```

---

### Task 2: Example tests for the main entry points

**Files:**
- Create: `afp/example_test.go`
- Create: `afptoline/example_test.go`
- Test: `go test -run Example ./... -v`

**Interfaces:**
- Consumes: `afp.Fields` (`afp/sfi.go`), `afptoline.Reduce` (`afptoline/reduce.go`), existing fixture byte slices for a minimal document (reuse the `bdtHello`-style fixtures from `afp/*_test.go` / `afptoline/*_test.go` — copy the minimal bytes into the example, do not import test files).
- Produces: runnable examples visible on pkg.go.dev.

- [ ] **Step 1: Write `ExampleFields` in `afp/example_test.go` (failing first)**

```go
func ExampleFields() {
	// Two minimal naked BNG fields.
	stream := []byte{
		0x00, 0x08, 0xD3, 0xA8, 0xAD, 0x00, 0x00, 0x00,
		0x00, 0x08, 0xD3, 0xA8, 0xAD, 0x00, 0x00, 0x00,
	}
	count := 0
	for f, err := range Fields(bytes.NewReader(stream)) {
		if err != nil {
			fmt.Println("error:", err)
			return
		}
		fmt.Printf("field %d at offset %d, %d bytes\n", count, f.Offset, len(f.Data))
		count++
	}
	fmt.Println("total:", count)
	// Output:
	// field 0 at offset 0, 8 bytes
	// field 1 at offset 8, 8 bytes
	// total: 2
}
```

Run: `go test ./afp/ -run ExampleFields -v`
Expected: FAIL — "got" output won't match until the exact bytes/offsets are confirmed (this is the RED step: the Output comment is a prediction; adjust it to the true values after running, then it becomes the pin).

- [ ] **Step 2: Correct the Output comment to the true values and re-run**

Run: `go test ./afp/ -run ExampleFields -v`
Expected: PASS.

- [ ] **Step 3: Write `ExampleReduce` in `afptoline/example_test.go`**

Build the smallest valid one-page document the reducer accepts (BDT, BPG, BAG with one PTX carrying "Hi", EAG, EPG, EDT — mirror the minimal fixture used by the existing reduce tests). Print the page count and the first run's text:

```go
func ExampleReduce() {
	doc, excl, err := Reduce(bytes.NewReader(minimalDoc))
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println("pages:", len(doc.Pages))
	fmt.Println("runs:", len(doc.Pages[0].Runs))
	fmt.Println("text:", doc.Pages[0].Runs[0].Text)
	fmt.Println("exclusions:", len(excl))
	// Output:
	// pages: 1
	// runs: 1
	// text: Hi
	// exclusions: 0
}
```

(Use the true field names from `line.Document`/`line.Run` — check `line/line.go`; adjust the Output comment to true values per the RED→GREEN cycle in Steps 1–2.)

Run: `go test ./afptoline/ -run ExampleReduce -v`
Expected: PASS after correcting the Output comment to true values.

- [ ] **Step 4: Run the full suite**

Run: `go test -count=1 ./...`
Expected: all packages pass (examples run as part of `go test`).

- [ ] **Step 5: Commit**

```bash
git add afp/example_test.go afptoline/example_test.go
git commit -m "docs: add runnable examples for Fields and Reduce"
```

---

## Self-Review

1. **Spec coverage:** R2P2-F03 = example tests (Task 2) + duplicate package comment (Task 1). Both covered.
2. **Step scan:** each step is one checkable action; the RED step for examples is honest (Output comments are predictions corrected to true values).
3. **Type consistency:** field names (`doc.Pages`, `Runs`, `Text`) must be verified against `line/line.go` at implementation time — flagged in the step.
4. **Review Focus:** output-exactness pinned by `// Output:`; teaching-value over duplication stated; single package comment verified by grep.
5. **Proportion:** compact — two small tasks for a docs-only change.
