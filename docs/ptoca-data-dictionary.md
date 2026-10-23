# PTOCA Data Dictionary

Known PTOCA control-sequence function-type mappings for go-afp-line.
Codes are the unchained base; the chained form is base `|` 0x01 (bit 0 of the
function-type byte is the chain flag). Wire form: `2B D3 Length Type Params…`
(`Length` counts itself and the type byte).

Sources: the subset contract (§5, panel-verified against IBM/AFPC/afplib/openafp)
for the 19 original sequences; `docs/_evidence/ptoca-0x80-research.md` for SEC.

## In-scope sequences (handled)

| Code | Name | Effect | Modal | go-afp-line handling |
|------|------|--------|-------|----------------------|
| D2 | AMB — Absolute Move Baseline | absolute baseline coordinate | no (immediate) | applied to position |
| C6 | AMI — Absolute Move Inline | absolute inline coordinate | no (immediate) | applied to position |
| D4 | RMB — Relative Move Baseline | relative baseline move | no (immediate) | applied to position |
| C8 | RMI — Relative Move Inline | relative inline move | no (immediate) | applied to position |
| D0 | SBI — Set Baseline Increment | line spacing | yes | applied to state |
| C0 | SIM — Set Inline Margin | inline margin | yes | applied to state |
| C2 | SIA — Set Intercharacter Adjustment | inter-character spacing | yes | applied to state |
| C4 | SVI — Set Variable-Space Character Increment | variable-space increment | yes | tracked |
| 74 | STC — Set Text Color | color specification | yes | recorded, not applied |
| 80 | SEC — Set Extended Text Color | extended color (RGB/CMYK/CIELAB/highlight/OCA) | yes | recorded, not applied |
| F2 | BSU — Begin Suppression | begin suppressed text block | yes (scoped) | gated text accumulation |
| F4 | ESU — End Suppression | end suppressed text block | yes (scoped) | gated text accumulation |
| F0 | SCFL — Set Coded Font Local | font switch by local ID | yes | decoder records LID (surfaced as Run.FontID); Reduce resolves non-zero LIDs through the MCF mapping (LID → coded font name → pinned metrics); unmapped LIDs fail loudly — v1 never guesses metrics |
| F6 | STO — Set Text Orientation | text orientation / rotation | yes | tracked, not applied (orientation recorded in state; not carried into runs) |
| DA | TRN — Transparent Data | literal text bytes follow | no (immediate) | decoded as text |
| EE | RPS — Repeat String | repeat character/string | no (immediate) | expanded to text |
| E4 | DIR — Draw I-Axis Rule | inline-axis rule | no (immediate) | length-validated only; no run emitted |
| E6 | DBR — Draw B-Axis Rule | baseline-axis rule | no (immediate) | length-validated only; no run emitted |
| D8 | BLN — Begin Line Next | advance to next line | no (immediate) | applied to position |
| F8 | NOP — No Operation | no operation | n/a | ignored |

### SEC (0x80) detail

- IBM PTOCA Reference SC31-6803-02, pp. 79–80: `TYPE X'80'–X'81'`, `LENGTH 14–16`,
  `COLSPCE` (`01` RGB, `04` CMYK, `06` highlight, `08` CIELAB, `40` standard OCA),
  `COLSIZE1–4`, `COLVALUE`. Corroborated by AFPC Programming Guide AFPC-0010-05
  Table 11 (PT3), afplib (`case 128 → createSEC()`), Apache FOP (`SEC = 0x80`).
- Semantics: sets the foreground color of subsequent text; "the last issued
  [STC/SEC] determines the current text color". Affects appearance only — never
  positioning or layout. IBM's sanctioned fallback for unsupported color is the
  device default color, which is why go-afp-line records SEC without applying it.
- Lengths outside 14–16 are a loud `KindStructure` error.

## Font mapping (MCF / SCFL)

Sources: IBM MO:DCA Reference — MCF Syntax (p. 229), Fully Qualified Name
triplet X'02' (pp. 341–342), Resource Local Identifier triplet X'24' (p. 368).
The wire rules below are paraphrased; the reference is normative.

An MCF (Map Coded Font, D3AB8A) field carries zero or more repeating groups. Each
group opens with a 2-byte big-endian `RGLength` (self-inclusive) followed by a
run of MO:DCA triplets (`afp.ParseMCFMapping`). An empty MCF yields an empty
mapping, not an error.

- **LID** comes from the X'24' triplet: triplet-data byte 1 is the resource
  type, byte 2 the one-byte LID (p. 368). Inside an MCF group only resource
  type X'05' (coded font) is permitted; anything else is a loud
  `KindStructure` error. A group without an X'24' borrows the architected
  default LID X'00' — applied per group, which is v1's reading of the
  reference default (p. 231). Rationale for the per-group reading: the
  repeating group is the unit that specifies a LID, so every group is keyed;
  the uniform duplicate rule below makes collisions loud rather than
  silently merged, which is the fail-closed outcome the reference's
  uniqueness rule demands.
- **Name** comes from the X'02' triplet with FQNType X'8E' (coded-font name
  reference): triplet-data byte 1 is the FQNType, byte 2 the format, and the
  name bytes follow, decoded as EBCDIC-037 (pp. 341–342). X'02' uniqueness is
  per FQNType, not per group: X'85' (code page name) + X'86' (charset name)
  in one group is the architecturally standard pair (p. 230); a repeated
  FQNType in one group is a loud `KindStructure` error.
- **Duplicate LIDs are a loud error:** the X'24' LID is defined as a unique
  identifier (p. 368), so a repeated LID across the field's groups aborts
  with `KindStructure` — never merged, never shadowed. This knowingly
  ignores p. 232's exception for double-byte font sections (same X'24' value
  across contiguous groups): v1 does not implement double-byte fonts, so
  such MCFs abort loudly at parse time rather than mis-resolving later.
- **LID 0 is not special to the parser:** the duplicate rule applies to LID 0
  uniformly — two X'24'-less groups both borrowing the default abort, and an
  explicit X'24' ResLID X'00' group participates like any other. LID 0 entries
  may exist in `ByLID`, but they are never consulted: SCFL LID 0 bypasses the
  map to the default font (`afptoline/fontIDForLID`).
- **Other identification forms are tracked, never silently skipped:** a group
  with an X'24' but no X'8E' name (e.g. GRID reference X'84', or a
  code-page/charset pair X'85'/X'86') is recorded in
  `MCFMapping.Unsupported`, in group order, and surfaced loudly at
  resolution time.

### Resolution chain

`Reduce` keeps one `*afp.MCFMapping` per document (`reduceState.mcf`):

1. SCFL local ID from the PTOCA stream →
2. MCF `ByLID` map → coded font name →
3. `line.LookupFont` → pinned metrics.

LID 0 bypasses the map entirely (the default font, `FIX-MONO-10`). A later
MCF in the same document replaces the earlier map; the map resets to nil at
BDT, so mappings never leak across documents.

The pinned table is the only autonomous font source: `FIX-MONO-10`
(synthetic monospace, every glyph 600/1000 em) and `FIX-PROP-10` (synthetic
proportional, a few declared advances — space 250, A 700, e 550, H 720, l 280,
o 560 — default 500/1000 em). These are synthetic fixtures only; real FOCA
resources stay excluded per the subset contract.

### Fail-closed

v1 never guesses metrics. Every broken link in the chain is a loud
`afp.DecodeError` (`fontIDForLID`, `afptoline/reduce.go`):

- duplicate LID, repeated FQNType in one group, or non-X'05' ResType in an MCF
  field → `KindStructure` at parse time;
- `"SCFL local font ID %d has no MCF mapping (no MCF in stream)"` — nonzero
  SCFL LID with no MCF yet in the document;
- `"SCFL local font ID %d has no MCF mapping (MCF present, LID unmapped)"`
  — LID absent from the map;
- `"MCF local font ID %d uses unsupported font identification (only X'8E' coded-font-name references are mapped)"`
  — the group carried an X'24' but no X'8E' name;
- `"coded font %q (MCF local font ID %d) has no pinned metrics"` — the
  mapped name has no entry in the pinned table.

The 0020 profile file (`01_Health_Coverage.afp`) carries no MCF while its PTX
uses SCFL LIDs 1–3: reduction fails on LID 1 with the no-MCF-mapping
diagnostic. (Verified locally against the profile bundle; the file is not in
this repo.)

**Rejected alternative:** silent fallback to default metrics for unmapped
LIDs — explicitly not implemented; needs a product call.

## Known but excluded

| Code | Status |
|------|--------|
| 72, 76, 78 | afplib-only; **unverified** against IBM docs — excluded by the 2-of-3 rule. Do not generalize the SEC finding to these codes. |

## Conventions

- Unknown function types abort with `unknown PTOCA control sequence 0xXX`
  (`KindValidation`) — fail-closed, never silently skipped.
- Truncated or overrunning sequences abort with `KindStructure`.
