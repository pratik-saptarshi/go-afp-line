# Subset contract — Go AFP → line-data → PDF (v1)

**Status:** Proposed decision record (Task 0, plan rev 3). Research evidence:
`docs/_evidence/contract-research.md`. This document is the Phase 0 deliverable;
Tasks 1–9 are blocked on its acceptance by review.

## 1. Declared use case (first, Proposed)

**Use case:** visual comparison of regenerated statements against reference renderings
by a human reviewer at 100% zoom.

**Derived tolerance (Proposed — calibration task against the Phase 4 oracle before
any fidelity claim is made):**
- geometry: ±1pt on text-run positions;
- text content: byte-exact after code-page decoding;
- placed width: within declared font metrics.

## 2. Reference stack

- **Primary:** IBM MO:DCA Reference (S550-1058). Where directly accessible, its text wins.
- **Corroborating opens:** afplib (Java, Apache-2.0) structured-field/triplet/PTOCA
  tables; openafp (Haskell, CC0) record/triplet/PTOCA tables; parse-afp (Perl, CC0)
  dispatch tables and round-trip model. Reference only — no code is copied.
- **Conflict rule:** IBM text wins. Where IBM is inaccessible, a TypeID enters the
  contract only with **two corroborating open sources**. Single-source entries are
  excluded (listed in `docs/_evidence/contract-research.md` §6.4).
- **TypeID convention:** 2-byte hex = `<type><category>` (the class byte `D3` is
  invariant across MO:DCA). The full 3-byte wire SFID is shown alongside.
  **IBM section references:** to be verified against S550-1058 (marked TBD below);
  entries are dual-corroborated by open sources in the meantime.

## 3. In-scope structured fields (minimal document walk)

| TypeID | Wire SFID | Name | IBM § |
|--------|-----------|------|-------|
| A8A8 | D3A8A8 | BDT — Begin Document | TBD |
| A9A8 | D3A9A8 | EDT — End Document | TBD |
| A8AF | D3A8AF | BPG — Begin Page | TBD |
| A9AF | D3A9AF | EPG — End Page | TBD |
| A8AD | D3A8AD | BNG — Begin Named Page Group | TBD |
| A9AD | D3A9AD | ENG — End Named Page Group | TBD |
| A8C9 | D3A8C9 | BAG — Begin Active Environment Group | TBD |
| A9C9 | D3A9C9 | EAG — End Active Environment Group | TBD |
| A89B | D3A89B | BPT — Begin Presentation Text Object | TBD |
| A99B | D3A99B | EPT — End Presentation Text Object | TBD |
| EE9B | D3EE9B | PTX — Presentation Text Data | TBD |
| A8A5 | D3A8A5 | BPF — Begin Print File | IBM MODCA-Ref-09 |
| A9A5 | D3A9A5 | EPF — End Print File | IBM MODCA-Ref-09 |
| A8CE | D3A8CE | BRS — Begin Resource (IBM; openafp/parse-afp say BR/ER) | IBM MODCA-Ref-09 |
| A9CE | D3A9CE | ERS — End Resource (IBM; openafp/parse-afp say BR/ER) | IBM MODCA-Ref-09 |
| A6AF | D3A6AF | PGD — Page Descriptor | IBM MODCA-Ref-09 |
| ABC3 | D3ABC3 | MDR — Map Data Resource | IBM MODCA-Ref-09 |

Text-environment companions (needed to interpret PTX):

| TypeID | Wire SFID | Name | IBM § |
|--------|-----------|------|-------|
| AB8A | D3AB8A | MCF — Map Coded Font | TBD |
| B19B | D3B19B | PTD — Presentation Text Descriptor | TBD |

Minimal walk nesting: `BDT … [BNG …] BPG → BAG … EAG → [BPT] PTX … [EPT] … EPG … EDT`.
PTX payload begins after the 8-byte introducer; control sequences are prefixed `2B D3`.

## 4. In-scope triplets

Wire format `Length(1) ID(1) Data…` (length includes itself). All 45 dual-corroborated
IDs are registered; v1 decode paths require only the Comment triplet.

| ID | Name | ID | Name |
|----|------|----|------|
| 01 | Coded Graphic Character Set Global ID | 50 | Encoding Scheme ID |
| 02 | Fully Qualified Name | 56 | Medium Map Page Number |
| 04 | Mapping Option | 57 | Object Byte Extent |
| 10 | Object Classification | 58 | Object Structured Field Offset |
| 18 | MO:DCA Interchange Set | 59 | Object Structured Field Extent |
| 1D | Text Orientation | 5A | Object Offset |
| 1F | Font Descriptor Specification | 5D | Font Horizontal Scale Factor |
| 24 | Resource Local ID | 5E | Object Count |
| 25 | Resource Section Number | 62 | Local Date and Time Stamp |
| 26 | Character Rotation | 64 | Object Origin Identifier |
| 2D | Object Byte Offset | 65 | **Comment** |
| 36 | Attribute Value | 68 | Medium Orientation |
| 43 | Descriptor Position | 6C | Resource Object Include |
| 45 | Media Eject Control | 6D | Extension Font |
| 47 | Resource Usage Attribute | 70 | Presentation Space Reset Mixing |
| 4B | Object Area Measurement Units | 71 | Presentation Space Mixing Rules |
| 4C | Object Area Size | 72 | Universal Date and Time Stamp |
| 4E | Color Specification | 74 | Toner Saver |
| — | — | 75 | Color Fidelity |
| — | — | 78 | Font Fidelity |
| — | — | 79 | Metric Adjustment |
| — | — | 80 | Attribute Qualifier |
| — | — | 81 | Page Position Information |
| — | — | 83 | Presentation Control |
| — | — | 84 | Font Resolution and Metric Technology |
| — | — | 85 | Finishing Operation |
| — | — | 87 | Media Fidelity |

**Triplet 0x21 conflict (decision):** afplib maps 0x21 → ResourceObjectType; openafp
maps 0x21 → Object Function Set Specification (0x22 = Resource Object Type), and
afplib carries a dead `case -33` suggesting its mapping is buggy. **0x21 is excluded
from the decoder pending a check against S550-1058**; encountering it returns a typed
error naming the triplet. Revisit after the primary check.

## 5. In-scope PTOCA control sequences

Each sequence: `Length(1) FunctionType(1) Data…`. Chaining: bit 0 of FunctionType is the
chained flag; codes below are the unchained base (chained = base `|` 0x01).

| Code | Name | Function |
|------|------|----------|
| D2 | AMB — Absolute Move Baseline | absolute baseline coordinate |
| C6 | AMI — Absolute Move Inline | absolute inline coordinate |
| D4 | RMB — Relative Move Baseline | relative baseline move |
| C8 | RMI — Relative Move Inline | relative inline move |
| D0 | SBI — Set Baseline Increment | line spacing |
| C0 | SIM — Set Inline Margin | inline margin |
| C2 | SIA — Set Intercharacter Adjustment | inter-character spacing |
| C4 | SVI — Set Variable-Space Character Increment | variable-space increment |
| 74 | STC — Set Text Color | color specification |
| F2 | BSU — Begin Suppression | begin suppressed text block |
| F4 | ESU — End Suppression | end suppressed text block |
| F0 | SCFL — Set Coded Font Local | font switch by local ID |
| F6 | STO — Set Text Orientation | text orientation / rotation |
| DA | TRN — Transparent Data | literal text bytes follow |
| EE | RPS — Repeat String | repeat character/string |
| E4 | DIR — Draw I-Axis Rule | inline-axis rule |
| E6 | DBR — Draw B-Axis Rule | baseline-axis rule |
| D8 | BLN — Begin Line Next | advance to next line |
| F8 | NOP — No Operation | no operation |

**Corrections applied:** the plan draft named `AMC` and `SIC`; no source defines either —
the correct sequences are **AMI (C6)** and **SIA (C2)**. Afplib-only codes
(0x72, 0x76, 0x78, 0x80) are excluded by the 2-of-3 rule.

## 6. Code pages

**037 and 500** (SBCS). EBCDIC → UTF-8 decode tables for both; any other code-page ID
is an explicit exclusion with a diagnostic. Dual-code-page fixtures are required by the
Review Focus.

## 7. Coordinate system and medium resolution

- PTOCA inline/baseline coordinates in L-units; page geometry from the Active
  Environment Group (BAG … EAG) text descriptors (PTD) in v1.
- **FormDef / PageDef (BFM/EFM, BPM/EPM, BDM/EDM, BFG/EFG) are excluded in v1**:
  any reference to them produces a loud diagnostic. (See Task 8/9: contracted fixed
  geometry is the v1 fallback.)

## 8. Font / code-page resolution story

- MCF (Map Coded Font) maps coded-font local IDs → (GCSGID, CPGID) pairs; SCFL
  switches the active coded font inside PTOCA.
- v1 uses a **pinned synthetic metrics table** for fixture fonts (declared in fixtures);
  Task 6 tests assert placed width matches the table.
- **FOCA font resources are excluded** (BCF/ECF, BFN/EFN, BCP/ECP, CFC, CFI, CPC, CPD,
  CPI, FNC, FND, FNG, FNI, FNM, FNN, FNO, FNP, MCF1) — any occurrence is a diagnostic,
  never a silent fallback.

## 9. Closed exclusion list

Out-of-subset-but-well-formed fields become `line.Exclusion` diagnostics and reduction
continues; malformed fields abort with a typed `DecodeError`. Nothing is silent.

- **IOCA (classified — excluded):** BIM/EIM, IPD, IDD, IID, IOC, ICP, IRD.
- **GOCA:** BGR/EGR, GAD, GDD.
- **BCOCA:** BBC/EBC, BDA, BDD, MBC.
- **FOCA:** per §8.
- **CMOCA / color:** BCA/ECA, MCA, CAT.
- **Overlays / medium:** BMO/EMO, BMM/EMM, MMO, MPO, IMM, MCC, MDD, MMC, BRG/ERG,
  BSG/ESG, BDG/EDG, BOG/EOG (note: no field literally named "OEG" exists).
- **FormDef / PageDef carriers:** per §7.
- **Object containers:** BOC/EOC, OCD.
- **Include Object (excluded):** IOB — the profile file's IOB includes object
  "I0000001" (ObjType X'92', Other object data) whose matching OCD payload is
  JPEG image data; content-bearing excluded-class content, so IOB decodes as
  excluded rather than contracted (DAP-0021 Task 2 caveat).
- **Misc skip:** NOP, TLE, IPS, MPS.

## 10. Framing contract

The decoder accepts both **naked MO:DCA streams** and **`5A`-wrapped variable-blocked
records**. Record layout (0-based): `[0]=5A`, `[1–2]=length (big-endian, excludes the
5A byte, includes itself)`, `[3–5]=SFID (D3 type category)`, `[6]=flags`,
`[7–8]=reserved`, `[9+]=data`. The splitter normalizes both framings to field byte
sequences before the SFI codec runs.

## 11. Demand validation (honest)

**Beneficiary hypothesis (unvalidated):** enterprises migrating AFP print archives
(statements, invoices, notices) to PDF for document pipelines, served by an
open-source native-Go tool. **No customer evidence has been gathered; demand is
unvalidated.** The Phase 4 gate must revisit demand before any productization or
maintenance commitment.

## 12. Phase 4 gate criteria

- **Decider:** Pratik Saptarshi. **Review date:** no later than 2026-11-13, or at
  completion of Task 9, whichever comes first.
- **Quantitative bar:** 100% byte-identical round-trip on the fixture corpus (Tier 1);
  100% conservation-law pass (every PTOCA text byte placed, suppressed-with-reason,
  or excluded-with-diagnostic); 100% of text runs within the §1 tolerance against
  the independent reader. The decider may grant written waivers.
- **Go/no-go:** the gate decides whether the bundle proceeds to productization,
  stays a feasibility artifact (sunset clause), or is retired.

## 13. Resourcing assumptions (rough, Proposed)

- Owner: Pratik Saptarshi (single owner). Effort (order of magnitude, part-time):
  Phase 1 (decoder + round-trip) ~2–3 weeks; Phase 2 (AFP → line-data) ~2 weeks;
  Phase 3 (line-data → PDF) ~1 week; Phase 4 (fidelity gate) ~1 week.

## 14. Per-phase kill criteria

- **Task 0 → 1:** this contract accepted by review.
- **Phase 1 (Tasks 1–3):** Tier 1 byte-identical gate must pass on the full fixture
  corpus, or the plan stops.
- **Phase 2 (Tasks 4–8):** conservation law holds on all fixtures; exclusion
  precision 100% (no silent drops on the exclusion corpus), or the plan stops.
- **Phase 3 (Task 9):** independent read-back within the §1 tolerance, or the plan stops.
- **Phase 4:** the gate in §12 decides go / sunset / retire. No-go remains a valid outcome.
