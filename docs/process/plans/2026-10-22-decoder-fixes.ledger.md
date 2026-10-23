# Rulings ledger — decoder fix wave: SFI PadFlag + SegFlag (PR #20)

Date: 2026-10-22. Branch: `feat/sec-ptoca-0x80` (worktree
`.worktrees/feat-sec-0x80`). Implementer record of every judgment call.
No commit yet — coordinator commits after independent review.

## Spec ground truth (re-verified by implementer, not taken on paraphrase)

Ran `pdftotext` on `docs/_evidence/modca/MODCA-Reference-09.pdf` and read
"Structured Field Segmentation" and "Structured Field Padding" (pp. 22–23
of the PDF) in full. All three load-bearing points CONFIRMED, no STOP
condition triggered:
- Padding: 1–2 bytes → length in the last padding byte; 256–32,759 bytes →
  length in the last three bytes (last byte X'00', two preceding bytes
  big-endian length); 3–255 bytes → either method. Padding length includes
  the length byte(s); SFLength includes the padding.
- Segmentation: bit 2 = B'1' → fragments are one continuous parameter
  string (data may split at any byte boundary), NO other structured fields
  permitted between; the last field of the sequence must have the flag B'0'.
- Bit numbering cross-checked against the SFI flag-byte table in the same
  PDF (bit 2 = SegFlag, bit 4 = PadFlag): matches the code's
  `flagSeg = 0x20`, `flagPad = 0x08`.
- Also noted: without the flag, interspersed fields are permitted — the fix
  implements only the flagged form, per the task.

Ruling: Proceeded with implementation — paraphrase held on every
load-bearing point. — Cost if wrong: would have shipped a misread of the
padding-length encoding; closed by reading the source text directly.

## Fix 1 — stripFieldPadding in decodeField (afp/field.go)

- Ruling: Stripping happens in `decodeField` immediately after
  `data := b[in.HeaderLength():]`, before registry dispatch — so typed,
  excluded, and contracted kinds all see padding-free data. — Why: the
  three return sites share this one path; stripping anywhere else would
  need three call sites. — Cost if wrong: a new spec kind added later with
  its own data slicing would bypass stripping; the single choke point is
  the cheapest place to keep the invariant.
- Ruling: Cleared PadFlag on the decoded `Introducer` (`in.Flags &^=
  flagPad`) after stripping — beyond the task's letter. — Why: leaving the
  flag set while Data is padding-free makes `Encode()` emit a malformed
  field (flag claims padding, none present), and re-decoding that output
  would mis-strip real payload bytes. The decoded value is the logical
  field; the introducer must describe the data it carries. — Cost if
  wrong: a consumer introspecting `Intro.HasPadding()` can no longer tell
  the wire field was padded — accepted, since the wire fact is
  unrecoverable once padding is stripped anyway.
- Ruling: `RawField` doc updated ("verbatim" → padding-free); the old
  comment claimed raw bytes are always preserved for byte-identical
  re-encoding, which Fix 1 intentionally breaks for padded fields. — Why:
  a stale doc is a false oracle. — Cost if wrong: none.
- Ruling: `RoundTrip` (afp/triplets.go) interaction left as-is: its
  byte-identical gate now fails LOUDLY on padded input (decode strips,
  re-encode is well-formed but shorter). — Why: byte-identical re-encode
  of a padded field is impossible without retaining arbitrary padding
  content; the loud gate failure is the honest fail-closed outcome, not a
  silent corruption. — Cost if wrong: a padded document that used to
  round-trip now fails the gate — accepted as correct strictness; no
  existing test covers padded RoundTrip and the full suite stays green.

## Fix 2 — reassembleSegmented wrapper (afp/sfi.go)

- Ruling: (a) Reassembly lives in a `reassembleSegmented` wrapper applied
  in `Fields` ONLY; `fieldsOp` (the shared core) and `SplitFields` keep
  yielding physical fragments. — Why: minimal blast radius — SplitFields'
  public contract is physical fields, and the task's consumer
  (`afptoline/reduce.go:101`, plus `afp` RoundTrip) wants logical fields.
  The rule is stated explicitly in the `SplitFields` doc comment, which
  previously (falsely) claimed SplitFields wraps "the streaming Fields
  iterator". — Cost if wrong: a future consumer needing physical fragments
  from `Fields` must switch to `SplitFields` — cheap and now documented.
- Ruling: (b) Padding is stripped PER PHYSICAL FRAGMENT, with that
  fragment's own introducer, before concatenation. — Why: padding is a
  physical-field property (each fragment's SFLength includes its own
  padding); stripping after concatenation would misread the length bytes.
  Pinned by `TestFields_SegmentedStripsPerFragmentPadding`. — Cost if
  wrong: fragment-N padding bytes would leak into the logical parameter
  string.
- Ruling: (c) Synthetic introducer = FIRST fragment's introducer verbatim
  (extension bytes preserved), SegFlag and PadFlag cleared, Length
  recomputed as `HeaderLength() + len(concatenated)`, serialized with the
  EXISTING `Introducer.Encode` helper — no hand-rolled serializer. Offset =
  first fragment's wire offset. — Why: Encode is the canonical marshal
  (checked first per the task); clearing both flags keeps the
  "introducer describes data" invariant; recomputed Length keeps
  downstream `DecodeField`'s len==SFLength check passing. — Cost if wrong:
  a hand-rolled serializer could drift from `decodeIntroducer`'s layout;
  using Encode ties them together.
- Ruling: A stream error (not just clean EOF) ending a sequence mid-flight
  yields `KindStructure` "truncated segmented field", NOT the underlying
  error. — Why: the task's stop list says "clean EOF or any end"; no
  consumer can use a half-reassembled field, and the sequence-invariant
  violation (with the sequence's field ID and start offset) is the
  actionable diagnostic. — Cost if wrong: the underlying read cause is
  masked — accepted, the stream is unusable either way.
- Ruling: Reassembled length above `maxSFLength` (32767) is a loud
  `KindStructure` error, not `Introducer.Encode`'s panic. — Why:
  concatenated fragments can exceed the architectural maximum; a panic is
  not a typed fail-closed error. — Cost if wrong: none; unreachable for
  well-formed sequences.
- Ruling: The `decodeIntroducer` re-parse per yielded field in the wrapper
  is intentional duplication, not waste to dedupe. — Why: `fieldsOp`
  already validated, so the re-parse cannot fail on honest input; the
  defensive fail-closed branch covers only a logic contradiction. Keeps
  the wrapper independent of `fieldsOp` internals. — Cost if wrong:
  negligible per-field parse cost.

## Tests (TDD, all in new file afp/sfi_flags_test.go)

Fixture style follows field_test.go (hex byte fixtures, BGR D3A8BB
known-excluded for Fix 1 so RawField.Data observes the shared path
verbatim; PTX D3EE9B contracted for Fix 2).

Fix 1 RED (pre-fix run): 5 failures —
`TestDecodeField_StripsOneByteFormPadding` (`RawField.Data = aa bb cc dd
ee 02, want aa bb cc dd`), `TestDecodeField_StripsThreeByteFormPadding`
(`len = 264, want 4`), `TestDecodeField_PadLenExceedsDataErrors`,
`TestDecodeField_PadFlagEmptyDataErrors`,
`TestDecodeField_ThreeByteFormShortLengthErrors` (all three: `expected
error, got nil`). Negative control
`TestDecodeField_NoPadFlagSmallTailByteUntouched` passed on RED by design.

Fix 2 RED (pre-fix run): 4 failures —
`TestFields_ReassemblesSegmentedSequence` (`got 3 fields, want 1`),
`TestFields_SegmentedInterspersedFieldErrors` (`expected error, got 3
fields`), `TestFields_SegmentedTruncatedAtEOFErrors` (`expected error,
got 1 fields`), `TestFields_SegmentedStripsPerFragmentPadding` (`got 2
fields, want 1`). The two pins (`TestFields_NonSegmentedPassthroughByteIdentical`,
`TestSplitFields_YieldsPhysicalFragments`) passed on RED by design.

GREEN: all 12 pass post-fix; `go test ./...` green (afp, afptoline,
genfixture, line, linetopdf); `gofmt -l` empty; `go vet ./...` clean.

Extra verification (not committed):
- Throwaway E2E test (since removed): `Fields` → `DecodeField` over a
  segmented stream yields one logical field, flags cleared, concatenated
  data, no error — the reduce.go consumer path is transparent. PASS.
- Fuzz smoke: `FuzzDecodeField` 20s (694k execs), `FuzzDecodeIntroducer`
  15s (541k execs) — PASS, no new failures, no corpus files left behind.

## Process notes

- New test file `afp/sfi_flags_test.go` holds both fixes' tests (one place
  for SFI-flag behavior); helpers `paddedBGR`, `segField`, `edtField`,
  `decodeErr`.
- CHANGELOG.md `[Unreleased]` → `### Fixed`: two entries (padding strip,
  segmentation reassembly). No verbatim IBM/MO:DCA prose added anywhere;
  code comments paraphrase with page/section citations.
- Mid-implementation self-correction: an edit replacing a doc-comment line
  accidentally clobbered the `pendingErrReader` doc block and dropped a
  closing brace; restored the comment byte-identical and the brace
  (verified via `git diff`: the only removed line in sfi.go is the old
  `Fields` body). Lesson: when old_text is a single comment line,
  double-check the replacement didn't swallow neighbors.
- NOT committed, NOT pushed — coordinator handles commit after the
  independent reviewer signs off.

## Follow-up: Codex post-wave review (2026-10-22 ~23:45 CDT)
Two new threads on the fresh reassembly code, both legitimate bugs in the
wave's implementation (not pre-existing scope):
- Ruling: the wave's `lin.Length > maxSFLength → KindStructure "exceeds MO:DCA
  maximum"` check was spec-misframed — the 32,767 limit is per physical
  field, and segmentation exists precisely for larger logical fields.
  However, the v1 pipeline genuinely cannot represent a logical field
  > maxSFLength (Introducer.Encode panics, decodeIntroducer rejects,
  DecodeField re-validates, StreamField contract). Full >32767 support would
  need pipeline-level changes — disproportionate for v1, and the spec's
  normal >32767 case is OCA data, which the subset excludes. Resolution:
  keep the bound but reframe honestly as a v1 resource limit with KindBudget
  ("The stream was well-formed; it was simply too large to decode safely"),
  matching the decoded-text budget convention.
- Ruling: the bound is now enforced per-append (boundFrag closure) before
  each fragment is buffered, so a hostile stream cannot accumulate unbounded
  memory before the terminator arrives (DoS guard). The old post-terminator
  check was unreachable afterwards and was removed.
- TDD: TestFields_SegmentedExceedingBoundAbortsEarly failed RED (got
  KindStructure, wanted KindBudget), green post-fix. Full suite green.

## Follow-up 2: Codex review of 837b90b (2026-10-22 ~24:00 CDT)
Three new threads, all verified against the MO:DCA Reference PDF and all
real (spec-confirmed, not nits):
- Ruling (RGLength): spec MCF Syntax p. 229 defines RGLength range as
  7–(n+1). The parser's `rgLen < 2` accepted 2–6; an RGLength==2 empty group
  was silently dropped as "default-LID". Changed to `rgLen < 7`. This
  overrode the MCF wave's deliberate TestParseMCFMapping_EmptyGroup (which
  expected acceptance) — the test contradicted the spec and was updated to
  expect KindStructure, as was the "after empty group" subtest (now uses a
  valid group to exercise the truncated-header check).
- Ruling (FQN exclusivity): spec MCF Semantics forbids X'84' with X'8E',
  X'85', or X'86', and X'8E' with X'85'/X'86' in one repeating group — the
  three are mutually exclusive identification forms. parseMCFGroup now
  validates the collected FQN-type combination (KindStructure on mixing).
  Added mcfFQNTypeCodedFontGRID/CodePageName/CharsetName constants.
- Ruling (RoundTrip): DecodeField strips padding (new behavior), so the
  byte-identical gate failed on padded input. RoundTrip now normalizes the
  input (strip padding, clear PadFlag, recompute length) before comparing —
  the gate verifies decode/encode fidelity, not wire-byte preservation of
  semantically-irrelevant padding.
- TDD: all three new tests failed RED, green post-fix. Full suite green.
