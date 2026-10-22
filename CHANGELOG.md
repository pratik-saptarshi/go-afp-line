# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added
- Registered 7 structural SFIDs (BPF/EPF, BRS/ERS, PGD, MDR, IOB) as contracted passthrough with IBM-primary source citations. IOB decodes as excluded rather than contracted per the profile-usage check: the profile file's IOB includes an object container whose OCD payload is JPEG image data.

### Fixed
- Object-container fields BOC/EOC/OCD (contract §9) no longer abort the decode; they decode as excluded `RawField` passthrough.

## [0.1.0-beta.1] - 2026-10-21

First beta release.

### Added
- Dependabot coverage for Go modules and GitHub Actions (weekly).
- `govulncheck` vulnerability scanning in CI.
- Runnable example tests `ExampleFields` (`afp`) and `ExampleReduce` (`afptoline`); the packages previously had zero examples.
- Duplicate `// Package afptoline` doc comment removed.

### Changed
- API polish: 12 internal-only identifiers unexported across `afp`, `line`, and `afptoline`; `LookupFont` now returns a defensive copy (its struct shared a mutable map with the global font table).
- Provably-dead `MaxFieldSize` checks removed; the streaming iterator now fail-fasts on `flen > maxSFLength` (32767) before allocating.
- PTOCA inline position now advances past emitted text on the `Reduce` path, using pinned font metrics plus the SIA intercharacter adjustment. Runs after text in one PTX start at the advanced position; absolute moves (AMI) and new lines (BLN) still override it. `DecodePTOCA` is unchanged (no advancement).

### Fixed
- Bound-aware framing fallback: 5A-prefixed records with an over-max length are now rejected by the length bound instead of being misclassified as naked framing (reported as truncation).
- Map EBCDIC NEL (U+0085) to space instead of failing with a C1 encoding rejection.
- Persist PTOCA decoder state (position, SIA, font, suppression) across PTX fields within a presentation text object; reset on BPT, discarded on EPT, cleared on page boundary.

## [Round 2 Phase C] - 2026-10-15

Fourth batch of adversarial panel findings (PRs #4–#7).

### Added
- `afp.Fields`: incremental streaming field iterator (`iter.Seq2`) bounding input retention; `SplitFields` kept as a collecting wrapper.
- Code page 500 support via `afptoline.ReduceWithCodePage`; `Reduce` remains byte-identical to code page 37.
- Fuzz targets for PTOCA decode and `Reduce` with committed seed corpus; `-shuffle=on` and an 85% coverage floor in CI.

### Changed
- `context.Context` support explicitly dropped (ADR 0001): no caller scenario justified the breaking change.

### Fixed
- 5A record-marker validation on wrapped streams.
- `Triplet.Encode` rejects lengths outside the architectural 3..254 range (was byte-capacity-only).
- Read errors arriving alongside bytes are surfaced after the bytes are processed, per the `io.Reader` contract.
- EOF arriving with a field's final body bytes is accepted, not misreported as truncation.
- CI concurrency group scoped to the source repository; PR branch pushes test the merge commit.

## [Round 2 Phases A/B] - 2026-10-15

Second adversarial panel review; all blockers fixed forward (PRs #2, #3).

### Fixed
- Fail-fast decoded-text budget inside PTOCA decoding (RPS expansion bounded).
- EPG rejects an open BPT group.
- U+0080–U+009F C1 controls rejected during `Reduce`.
- Page dimensions limited to (0, 14400]; run coordinates bounded to |X|,|Y| ≤ 14400.
- Typed `DecodeError` kinds (structure, encoding, budget, validation) with stream offsets.

## [Round 1 P0 batch] - 2026-10-15

First adversarial panel review; five blockers fixed (PR #1).

### Fixed
- BAG/EAG groups allowed inside an open page.
- RPS repeats RPTDATA until RLENGTH (was treated as literal text length).
- WinAnsi single-byte encoding with errors on unmappable characters; control-character escaping.
- `+Inf` rejected for dimensions and font size.
- CI installs poppler and fails fast when missing.

## [Initial release] - 2026-10-14

Public repository publication.

### Added
- AFP→line→PDF reference library: `afp` (MO:DCA structured-field codec), `line` (document model), `afptoline` (PTOCA decode + reduction), `linetopdf` (pdfcpu rendering).
- Apache-2.0 license.
