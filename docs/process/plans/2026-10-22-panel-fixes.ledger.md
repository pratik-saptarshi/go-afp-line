# Panel fixes ledger — 2026-10-22

Fix wave for the paperthin lens panel (hate/feynman/macrothink/prism) verdict
PROCEED-WITH-FIXES on PR #20 @ 76296ca. All fixes TDD; behavior changes get a
failing test first.

- Ruling: fontIDForLID LID-0 miss branch (default font unpinned) is untestable without test hooks on line's package-level pinned table — adding a hook is over-machinery for a latent path; the success path is pinned by TestFontIDForLID_DefaultVerified and the miss-branch shape is exercised by TestReduce_MCFUnpinnedFont through the same function. Cost if wrong: a future registry edit removing FIX-MONO-10 would hit an untested 3-line branch — but that branch is symmetric with the tested mapped-LID miss path and fails loudly by construction.
- Ruling: Fix 1 — uniqueness keyed on FQNType (Data[0]) read *before* the length guard; for empty Data firstByte returns 0 but the length guard fires first, so no phantom 0x00-type duplicate is reachable. Cost if wrong: a crafted empty X'02' pair could false-duplicate — impossible since the first one aborts.
- Ruling: Fix 1 — kept the "MCF: " prefix on the pre-existing X'24'/X'02'-shape diagnostics but the new duplicate-FQNType reason drops it, matching the duplicate-LID reason format (per the earlier SDD ledger ruling 4). Cost: cosmetic inconsistency across diagnostic strings.
- Ruling: Fix 2 — documented rejection of the p. 232 double-byte exception rather than implementing it; double-byte font support is out of the contracted subset. Cost: MCFs using the exception now abort loudly where they previously reduced (accepted-and-noted) — a conscious fail-closed behavior change, stated in the dictionary.
- Ruling: Fix 3 — mirrored visitPTX's errors.As + unconditional rebase (no KindBudget carve-out): the MCF parser has no budget concept, all its errors are structure/validation with payload-relative offsets. Cost: none — FieldID was always empty from this parser.
- Ruling: Fix 4 — recorded above (untestable miss branch).
- Ruling: Fix 5 — paraphrased all quoted spec prose; left the author's own scare-quoted coinages ("afplib-only") and code snippets. Cost: none.
- Ruling: Fix 7 — pinned the existing uniform-duplicate behavior for LID 0 with a test rather than changing it; dictionary now states the LID-0 semantics explicitly. Cost: none.
- Ruling: real-file acceptance re-verified after the parser changes (scratch driver, /tmp, deleted): Reduce still fails with "SCFL local font ID 1 has no MCF mapping (no MCF in stream)" at offset 6559. Cost if wrong: none — it matched.
