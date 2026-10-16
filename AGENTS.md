# AGENTS.md

Agent working notes for this repository.

## Project boundaries

- This is a public Go library: AFP → line → PDF (`afp/`, `line/`, `afptoline/`, `linetopdf/`). Keep the public API surface minimal; unexport identifiers that don't need to be public.
- Apache-2.0 licensed. Never add copyrighted specification extracts or third-party code without a compatible license grant. Local reference PDFs under `docs/_evidence/` are gitignored — never commit, publish, or delete them.
- Go is the implementation language. Prefer the standard library. Record license and maintenance posture before adopting a new dependency.
- Benchmarks are deliberately deferred (no performance target exists). Don't add benchmarking infrastructure unprompted.

## Work and tests

- Test-first: add a failing test, make the smallest change that passes, then refactor with tests green. Tighten assertions that mask real behavior — a ±1pt tolerance once hid an inter-run ordering bug that ±0.01 caught.
- Run before every PR:
  ```sh
  test -z "$(gofmt -l .)"
  go vet ./...
  go test ./...
  ```
  CI also enforces `govulncheck` and a fuzz-smoke run.
- Fuzz parsers and hostile-input boundaries where a useful invariant can be asserted. Write mutant-killer tests: when adding a test, consider what plausible mutant would survive it.

## Git and review

- Use Conventional Commit subjects (`type(scope): imperative summary`). Squash-merge PRs; delete stale remote branches after merge.
- Require independent review and green exact-head CI before merge. Check review comments, not just CI status — a comment can land seconds before the merge.
- Before merging, verify the PR head ref equals the pushed branch ref; a stale head silently drops newer pushes.

## Documentation

- Keep docs concise and Markdown-only. Process docs (plans, designs, reviews, integration logs) live under `docs/process/` — never scattered in the public tree.
- `CHANGELOG.md` follows Keep a Changelog. Add entries under `[Unreleased]` with every user-facing change.
- Repair intra-repo references when moving files; grep-verify nothing dangles.

## Public-tree hygiene

- After agentic implementation, before opening a PR: run the `scrubbing-public-tree` skill (`docs/skills/scrubbing-public-tree/SKILL.md`) — relocate process docs to `docs/process/`, scrub agent headers/machine paths/identities, delete scratch modules, dedupe CHANGELOG, rebase intra-repo references and grep-verify none dangle.
