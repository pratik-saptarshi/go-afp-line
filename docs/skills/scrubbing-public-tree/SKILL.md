---
name: scrubbing-public-tree
description: Use when preparing an agent-built repo or branch for public viewing — after agentic implementation work, before opening a PR or publishing. Triggers: docs or comments containing agent headers, absolute machine paths, or bot identity strings; process artifacts (plans, designs, reviews, integration logs) sitting in the public tree; nested scratch or spike modules; duplicate CHANGELOG sections.
---

# Scrubbing the Public Tree

## Overview

Agentic implementation leaves traces that don't belong in a public repo: machine-specific paths, agent identity strings, prompt headers, and process paperwork scattered where users will read it. **Core principle:** the public tree should read as if a careful human wrote it — no agent artifacts, no machine paths, no misplaced process docs, no dangling references.

Language-agnostic: the playbook detects the build system instead of assuming one — Go, Rust, TypeScript, Python, or anything else.

**Announce at start:** "I'm using the scrubbing-public-tree skill to clean the public tree."

## When to Use

- After a subagent-driven (or any agentic) implementation run, before opening the PR
- Before publishing a repo or branch that agents have worked in
- When a review flags "agent artifacts" or machine-specific content in the tree

**When NOT to use:** private scratch repos nobody will read; repos where process docs are intentionally public at their current paths (check the repo's docs conventions first).

## The Playbook

Work through these in order. Each step ends with a checkable result.

| # | Step | What to do | Done when |
|---|------|-----------|-----------|
| 1 | Inventory | List process artifacts (plans, designs, reviews, integration logs) and agent/machine traces (headers, absolute paths, identity strings) in the public tree | You have the full list |
| 2 | Relocate | Move process docs under `docs/process/` (`plans/`, `designs/`, `reviews/` subdirs as appropriate) | Public tree has no build paperwork |
| 3 | Scrub | Remove agent headers, absolute machine paths (`/home/...`, `/tmp/...`, `/Users/...`), and bot identity strings from every public-tree file | `grep -r` for each pattern returns empty |
| 4 | Delete | Remove nested scratch/spike modules and evidence dirs that shouldn't publish | No dead modules remain |
| 5 | Dedupe | Merge duplicate CHANGELOG sections; keep one entry per version | Changelog parses cleanly |
| 6 | Repair | Rebase intra-repo references to the new paths; then grep-verify no dangling references remain | Every moved file's inbound links resolve |
| 7 | Verify | Detect the build system and run the full suite (see below) | Green, same as before the scrub |

## Implementation

**Scrub patterns** (extend per repo; these are the common ones). Search all text files, excluding VCS and build output:
```bash
EXCL="--exclude-dir=.git --exclude-dir=target --exclude-dir=node_modules --exclude-dir=vendor --exclude-dir=dist"
grep -rn -E '/home/|/root/|/tmp/|/Users/' . $EXCL
grep -rniE 'cloudbot|neo-green' . $EXCL
```

**Scrub, don't just delete:** when a machine path carries meaning ("build output lives at /home/hatch/build/out"), generalize it to a neutral description ("the configured build output directory") rather than deleting the sentence — the information matters, the absolute path doesn't.

**Build detection** (step 7) — check for manifest files at the repo root, in this order. Guard each with `command -v` so a manifest without its toolchain reports cleanly instead of failing opaquely:

| Manifest | Run |
|---|---|
| `Cargo.toml` | `command -v cargo >/dev/null && cargo test` |
| `go.mod` | `command -v go >/dev/null && go test ./...` |
| `package.json` | `command -v npm >/dev/null && npm test` |
| `pyproject.toml` / `requirements.txt` | `command -v pytest >/dev/null && pytest` |

Detection is root-manifest-only: workspace members and nested modules are covered by the root suite — don't hunt for nested manifests. If several manifests exist, run each suite. If a manifest exists but its toolchain doesn't, note it and move on. If no manifests exist, note it and move on — don't invent a build.

**Relocation:** prefer moving over copying — a copy leaves two sources of truth that drift. Step 6 is mandatory after every move: references break silently, including in ADRs and decision records (updating a moved path doesn't alter the decision; a broken link in a day-old ADR serves nobody).

## Common Mistakes

| Mistake | Why it hurts | Fix |
|---------|--------------|-----|
| Scrubbing only `*.md` | Paths hide in code comments, scripts, fixtures — in any language | Grep all text files (see patterns above) |
| Assuming one build system | `go test` on a Rust repo wastes the verification step | Detect via manifest files (step 7) |
| Leaving ADRs pointing at moved files | Traceability rots immediately | Repair is part of the move |
| Copying instead of moving | Two copies drift; no canonical source | Move, then repair references |
| Skipping step 7 | Scrub edits can break samples or fixture paths | Build + tests after every scrub |

## Real-World Impact

Applied to `go-afp-line` PR #15: process docs relocated to `docs/process/`, agent headers/machine paths/identity scrubbed, a nested spike module deleted, CHANGELOG deduped, and references rebased — with one fix round for dangling refs caught by verification. The PR merged with no hygiene findings.
