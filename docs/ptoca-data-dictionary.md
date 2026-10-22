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
| F0 | SCFL — Set Coded Font Local | font switch by local ID | yes | applied (LID 0 default; other LIDs are a loud error — v1 never guesses metrics) |
| F6 | STO — Set Text Orientation | text orientation / rotation | yes | applied to state |
| DA | TRN — Transparent Data | literal text bytes follow | no (immediate) | decoded as text |
| EE | RPS — Repeat String | repeat character/string | no (immediate) | expanded to text |
| E4 | DIR — Draw I-Axis Rule | inline-axis rule | no (immediate) | rule run |
| E6 | DBR — Draw B-Axis Rule | baseline-axis rule | no (immediate) | rule run |
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

## Known but excluded

| Code | Status |
|------|--------|
| 72, 76, 78 | afplib-only; **unverified** against IBM docs — excluded by the 2-of-3 rule. Do not generalize the SEC finding to these codes. |

## Conventions

- Unknown function types abort with `unknown PTOCA control sequence 0xXX`
  (`KindValidation`) — fail-closed, never silently skipped.
- Truncated or overrunning sequences abort with `KindStructure`.
