# Operational Hygiene (F04) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the deferred operational-hygiene items (F04) to go-afp-line: vulnerability scanning in CI, Dependabot updates, a security policy, a changelog, and a complete `.gitignore`.

**Architecture:** Config-and-docs-only change: no production code is touched. CI gains a `vuln` job (govulncheck) and Dependabot covers Go modules plus GitHub Actions; repo-root docs (`SECURITY.md`, `CHANGELOG.md`) follow standard layouts; `.gitignore` gains the standard Go set while preserving the existing evidence exclusions.

**Tech Stack:** GitHub Actions, Dependabot, govulncheck, Keep-a-Changelog format.

**Spec:** `docs/superpowers/plans/2026-10-15-panel-round2-findings.md`, Backlog entry R2P2-F04 ("`govulncheck`, Dependabot, `SECURITY.md`, changelog, fuller `.gitignore` — defer — operational workstream, separate from code correctness").

## Global Constraints

- Go 1.27 toolchain (per `go.mod`); no new module dependencies.
- Conventional Commits subjects; identity `CloudBot Neo-green <waynemanorsaptarshi@gmail.com>`.
- CI style follows `.github/workflows/ci.yml`: SHA-pinned actions with trailing version comments, top-level `permissions: contents: read`, `timeout-minutes` on every job.
- Never force-push; work on branch `chore/operational-hygiene-f04` from `origin/main`.
- Docs live at repo root (`SECURITY.md`, `CHANGELOG.md`), matching `README.md`/`CONTRIBUTING.md` placement.

## Review Focus

- govulncheck reporting a real vulnerability in pdfcpu or the toolchain: the task's verify step surfaces it; the implementer reports it instead of shipping red CI — a failing scan is a finding, not a reason to weaken the check.
- Dependabot opening a major-version bump that breaks the build: CI guards every Dependabot PR; `open-pull-requests-limit` bounds the noise.
- CHANGELOG entries drifting from history: every entry must be traceable to a merged PR or commit in `git log`.
- `.gitignore` accidentally excluding committed files (e.g. `docs/_evidence/*.md` notes): the verify step asserts the evidence `.md` files are NOT ignored.
- SECURITY.md naming a reporting channel that doesn't exist: use GitHub's private vulnerability reporting (Security tab), which needs no email address.

---

## File Structure

- Create `.github/dependabot.yml` — Dependabot config: `gomod` + `github-actions` ecosystems, weekly schedule, PR limit.
- Modify `.github/workflows/ci.yml` — add a `vuln` job running govulncheck.
- Create `SECURITY.md` — supported versions + how to report a vulnerability.
- Create `CHANGELOG.md` — Keep-a-Changelog, seeded from shipped history (Unreleased + past phases).
- Modify `.gitignore` — append the standard Go/OS/IDE set; keep the two evidence exclusions at top.

Each task below is independently committable and reviewable; tasks touch disjoint files except ci.yml (Task 1 only).

---

### Task 1: Automated dependency hygiene — Dependabot + govulncheck

**Files:**
- Create: `.github/dependabot.yml`
- Modify: `.github/workflows/ci.yml` (append `vuln` job)
- Test: shell verification commands (no Go test files — config-only)

**Interfaces:**
- Consumes: existing `ci.yml` job conventions (SHA pins, `permissions:`, `timeout-minutes: 30`).
- Produces: `.github/dependabot.yml`; `vuln` job in CI.

- [ ] **Step 1: Write `.github/dependabot.yml`**

```yaml
version: 2
updates:
  - package-ecosystem: "gomod"
    directory: "/"
    schedule:
      interval: "weekly"
    open-pull-requests-limit: 5
  - package-ecosystem: "github-actions"
    directory: "/"
    schedule:
      interval: "weekly"
```

- [ ] **Step 2: Verify the YAML parses**

Run: `python3 -c "import yaml; yaml.safe_load(open('.github/dependabot.yml')); print('OK')"`
Expected: `OK`

- [ ] **Step 3: Append the `vuln` job to `.github/workflows/ci.yml`**

After the `build` job, add:

```yaml
  vuln:
    runs-on: ubuntu-latest
    timeout-minutes: 15
    permissions:
      contents: read
    steps:
      - uses: actions/checkout@11d5960a326750d5838078e36cf38b85af677262 # v4
      - uses: actions/setup-go@40f1582b2485089dde7abd97c1529aa768e1baff # v5
        with:
          go-version: "1.27"
      - name: govulncheck
        run: go run golang.org/x/vuln/cmd/govulncheck@latest ./...
```

(SHA pins match the existing `build` job; `@latest` for the tool itself is the upstream-recommended invocation.)

- [ ] **Step 4: Run govulncheck locally to establish the baseline**

Run: `go run golang.org/x/vuln/cmd/govulncheck@latest ./...` from the repo root.
Expected: exit 0 with "No vulnerabilities found." If it reports a vulnerability, STOP and report it — do not weaken the check to make CI green.

- [ ] **Step 5: Verify the workflow YAML still parses**

Run: `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml')); print('OK')"`
Expected: `OK`

- [ ] **Step 6: Commit**

```bash
git add .github/dependabot.yml .github/workflows/ci.yml
git commit -m "ci: add Dependabot and govulncheck vulnerability scanning"
```

---

### Task 2: Repo policy docs — SECURITY.md + CHANGELOG.md

**Files:**
- Create: `SECURITY.md`
- Create: `CHANGELOG.md`
- Test: content review against the constraints below (docs-only)

**Interfaces:**
- Consumes: `git log --oneline` history for changelog entries (see Step 3).
- Produces: `SECURITY.md`, `CHANGELOG.md` at repo root.

- [ ] **Step 1: Write `SECURITY.md`**

Content (exact sections):
- `# Security Policy`
- `## Supported Versions` — a table stating only the latest `main` is supported (`| main | :white_check_mark: |`, older revisions `:x:`).
- `## Reporting a Vulnerability` — "Please report vulnerabilities via GitHub's private vulnerability reporting (the repository's Security tab → 'Report a vulnerability'). Do not open a public issue." Plus: what to include (affected version/commit, reproduction steps) and the promise of a response ("we aim to acknowledge within 5 business days").
- No email address (nothing to keep current).

- [ ] **Step 2: Verify SECURITY.md has no dead reporting channel**

Check: the file names only GitHub private vulnerability reporting, no email/URL that could rot.
Expected: no `mailto:` and no external URLs except `https://github.com/pratik-saptarshi/go-afp-line/security/advisories/new` (the private-reporting link).

- [ ] **Step 3: Write `CHANGELOG.md` in Keep-a-Changelog format**

Sections, newest first, every entry traceable to `git log`:
- `# Changelog`, link headers `https://keepachangelog.com/en/1.1.0/`.
- `## [Unreleased]` — empty section (placeholder for future changes).
- `## [Round 2 Phase C] - 2026-10-15` — one-line bullets for the four workstreams: streaming field iterator (`afp.Fields`), parsing/encoding hardening (5A validation, triplet bounds, CHAR fidelity, code page 500), test hardening (fuzz, shuffle, coverage floor), CI hygiene (pinned actions, concurrency, Dependabot-ready).
- `## [Round 2 Phases A/B] - 2026-10-15` — RPS text budget, BPT/EPG guard, C1 rejection, coordinate bounds, typed errors.
- `## [Round 1 P0 batch] - 2026-10-15` — the five P0 fixes in one line each.
- `## [Initial release] - 2026-10-14` — public repo publication: AFP→line→PDF reference library (afp, line, afptoline, linetopdf packages), Apache-2.0.
- Keep each bullet to one line; no invented details — if a bullet can't be traced to the log, cut it.

- [ ] **Step 4: Commit**

```bash
git add SECURITY.md CHANGELOG.md
git commit -m "docs: add security policy and changelog"
```

---

### Task 3: Fuller .gitignore

**Files:**
- Modify: `.gitignore` (append; keep the existing two evidence exclusions at top untouched)
- Test: `git check-ignore` assertions

**Interfaces:**
- Consumes: nothing from other tasks.
- Produces: extended `.gitignore`.

- [ ] **Step 1: Append the standard Go/OS/IDE set to `.gitignore`**

Append exactly (existing evidence lines stay at the top):

```
# Binaries and build output
*.exe
*.exe~
*.dll
*.so
*.dylib
*.out
/bin/
/dist/

# Test and coverage artifacts
*.test
*.prof
coverage.out
coverage.html

# IDE and editor files
.idea/
.vscode/
*.swp
*.swo
*~

# OS files
.DS_Store
Thumbs.db

# Go workspace files
go.work
go.work.sum
```

- [ ] **Step 2: Verify ignores behave as intended**

Run:
```bash
git check-ignore -q fakebinary.exe && echo "exe ignored"
git check-ignore -q .DS_Store && echo "ds_store ignored"
git check-ignore -q docs/_evidence/SOURCES.md && echo "PROBLEM: evidence md ignored" || echo "evidence md kept"
```
Expected: `exe ignored`, `ds_store ignored`, `evidence md kept`. (The evidence exclusions are directory-scoped to `docs/_evidence/afp-research/` and `docs/_evidence/modca/`; committed `.md` notes must stay tracked.)

- [ ] **Step 3: Verify no tracked file becomes ignored**

Run: `git ls-files | git check-ignore --stdin | head`
Expected: no output (empty — nothing tracked is ignored).

- [ ] **Step 4: Commit**

```bash
git add .gitignore
git commit -m "chore: expand .gitignore with standard Go/OS/IDE entries"
```

---

## After All Tasks

- [ ] Push the branch and open a PR against `main` (do NOT merge).
- [ ] Confirm the new `vuln` job appears and passes in the PR's CI.

---

## Self-Review

1. **Spec coverage:** R2P2-F04 lists govulncheck (Task 1), Dependabot (Task 1), SECURITY.md (Task 2), changelog (Task 2), fuller .gitignore (Task 3). All covered. Nothing else in the F04 entry.
2. **Step scan:** each step produces one checkable thing; config "tests" are verification commands with expected outputs, appropriate for a code-free change.
3. **Type consistency:** no cross-task interfaces beyond file paths; ci.yml touched only by Task 1.
4. **Review Focus:** the five risks are listed above; govulncheck-finds-vuln is handled in Task 1 Step 4 (stop and report); Dependabot noise bounded by PR limit; changelog traceability enforced in Task 2 Step 3; .gitignore safety in Task 3 Steps 2–3; SECURITY.md channel rot in Task 2 Step 2.
5. **Proportion:** plan is deliberately compact — the change is five files, mostly config and docs; no code blocks beyond the exact file contents the implementer must write verbatim.
