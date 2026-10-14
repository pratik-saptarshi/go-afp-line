# MO:DCA Subset Contract — Research Evidence

Date: 2026-10-13. Task 0 subset-contract research for the Go AFP→line→PDF project.

## Sources

| Key | Source | What it contributed |
|-----|--------|---------------------|
| A | `afplib/` — Java/EMF library (SFName enum, StructuredFieldFactory dispatch tables, AfpInputStream, README, `HelloWorld/hello.afp` byte sample) | SFID table (128 entries), triplet ID table, PTOCA control-sequence codes, introducer/5A layout, 2BD3 escape, chaining bit |
| P | `parse-afp/` — Perl `Parse::AFP` (`Record.pm` DISPATCH_TABLE, `PTX.pm`, `AFP.pm` FORMAT) | SFID table (124 entries), record FORMAT (CC/Length/Type/FlagByte), PTX `2BD3` escape sequence, PTX chaining-bit refresh |
| O | `openafp/` — Haskell library (`Records/AFP.hs` SFID table, `Prelude/InstanceT.hs` triplet table, `Prelude/InstancePTX.hs` PTOCA table, `Prelude/Lookups.hs` canonical names) | SFID table (124 entries), triplet IDs, PTOCA codes (chained + unchained pairs), canonical English names |

**Conflict rule applied:** a TypeID enters the contract only if corroborated by ≥ 2 of the 3 sources.
Programmatic cross-check of all three SFID tables: **zero disagreements** on every shared entry.

**TypeID convention used below:** every source stores the 3-byte SFID `D3 <type> <category>`
at introducer offsets 3–5 (class byte `D3` is invariant across MO:DCA). The "2-byte hex TypeID"
column is the distinguishing `<type><category>` pair, e.g. BDT = `A8A8` on the wire as `D3A8A8`.

## 1. In-scope structured fields (minimal document walk)

| TypeID (2-byte) | SFID (wire) | Name | Corroborated by |
|-----------------|-------------|------|-----------------|
| A8A8 | D3A8A8 | BDT — Begin Document | A, P, O |
| A9A8 | D3A9A8 | EDT — End Document | A, P, O |
| A8AF | D3A8AF | BPG — Begin Page | A, P, O |
| A9AF | D3A9AF | EPG — End Page | A, P, O |
| A8AD | D3A8AD | BNG — Begin Named Page Group | A, P, O |
| A9AD | D3A9AD | ENG — End Named Page Group | A, P, O |
| A8C9 | D3A8C9 | BAG — Begin Active Environment Group | A, P, O |
| A9C9 | D3A9C9 | EAG — End Active Environment Group | A, P, O |
| A89B | D3A89B | BPT — Begin Presentation Text Object | A, P, O |
| A99B | D3A99B | EPT — End Presentation Text Object | A, P, O |
| EE9B | D3EE9B | PTX — Presentation Text Data | A, P, O |

Text-environment companions (not delimiters, needed to interpret PTX):

| TypeID (2-byte) | SFID (wire) | Name | Corroborated by |
|-----------------|-------------|------|-----------------|
| AB8A | D3AB8A | MCF — Map Coded Font | A, P, O |
| B19B | D3B19B | PTD — Presentation Text Descriptor | A, P, O |

Minimal walk nesting: `BDT … [BNG …] BPG → BAG … EAG → [BPT] PTX … [EPT] … EPG … EDT`.
PTX content begins immediately after the 9-byte introducer (afplib parses PTX payload from
`pos + 9`).

## 2. Triplets (1-byte ID + 1-byte length prefix)

Triplet wire format (A + P + O agree): `Length(1) ID(1) Data…`, length includes itself.
Example from afplib README byte sample: `19 65` = length 0x19, ID 0x65 (Comment).

| ID (hex) | Name | Corroborated by |
|----------|------|-----------------|
| 65 | Comment | A, O |
| 01 | Coded Graphic Character Set Global ID (CGCSGID) | A, O |
| 02 | Fully Qualified Name | A, O |
| 04 | Mapping Option | A, O |
| 10 | Object Classification | A, O |
| 18 | MO:DCA Interchange Set | A, O |
| 1D | Text Orientation | A, O |
| 1F | Font Descriptor Specification | A, O |
| 24 | Resource Local ID | A, O |
| 25 | Resource Section Number | A, O |
| 26 | Character Rotation | A, O |
| 2D | Object Byte Offset | A, O |
| 36 | Attribute Value | A, O |
| 43 | Descriptor Position | A, O |
| 45 | Media Eject Control | A, O |
| 47 | Resource Usage Attribute | A, O |
| 4B | Object Area Measurement Units | A, O |
| 4C | Object Area Size | A, O |
| 4E | Color Specification | A, O |
| 50 | Encoding Scheme ID | A, O |
| 56 | Medium Map Page Number | A, O |
| 57 | Object Byte Extent | A, O |
| 58 | Object Structured Field Offset | A, O |
| 59 | Object Structured Field Extent | A, O |
| 5A | Object Offset | A, O |
| 5D | Font Horizontal Scale Factor | A, O |
| 5E | Object Count | A, O |
| 62 | Local Date and Time Stamp | A, O |
| 64 | Object Origin Identifier | A, O |
| 68 | Medium Orientation | A, O |
| 6C | Resource Object Include | A, O |
| 6D | Extension Font | A, O |
| 70 | Presentation Space Reset Mixing | A, O |
| 71 | Presentation Space Mixing Rules | A, O |
| 72 | Universal Date and Time Stamp | A, O |
| 74 | Toner Saver | A, O |
| 75 | Color Fidelity | A, O |
| 78 | Font Fidelity | A, O |
| 79 | Metric Adjustment | A, O |
| 80 | Attribute Qualifier | A, O |
| 81 | Page Position Information | A, O |
| 83 | Presentation Control | A, O |
| 84 | Font Resolution and Metric Technology | A, O |
| 85 | Finishing Operation | A, O |
| 87 | Media Fidelity | A, O |

parse-afp has no triplet table, so triplet IDs are corroborated by A + O only (still ≥ 2).
afplib-only triplet IDs (single source, **excluded** from contract): 86, 88, 8B, 8C, 8E, 95, 96, 97, 9A.
openafp-only triplet IDs (single source, **excluded**): 20, 22, 23, 27, 46, 4D, 73, 82.
See Conflicts for ID 0x21.

## 3. PTOCA control sequences

PTX payload layout (A + P + O agree): `2B D3` escape prefix, then control sequences.
Each control sequence: `Length(1) FunctionType(1) Data…`, length includes itself.
**Chaining:** bit 0 of FunctionType = chained flag (A: `(functionType & 0x01) == 1`, unchain via
`& 0xFE`; P: sets/clears bit 0 on PTX control codes; O: maps both even/odd codes to the same record).
Codes below are the unchained base; chained form = base `|` 0x01.

| Code (hex) | Name | Function (one line) | Corroborated by |
|------------|------|---------------------|-----------------|
| D2 | AMB — Absolute Move Baseline | Move print position to an absolute baseline coordinate | A, O |
| C6 | AMI — Absolute Move Inline | Move print position to an absolute inline coordinate | A, O |
| D4 | RMB — Relative Move Baseline | Move print position relative, in baseline direction | A, O |
| C8 | RMI — Relative Move Inline | Move print position relative, in inline direction | A, O |
| D0 | SBI — Set Baseline Increment | Set line spacing (distance between successive baselines) | A, O |
| C0 | SIM — Set Inline Margin | Set inline margin (text boundary in inline direction) | A, O |
| C2 | SIA — Set Intercharacter Adjustment | Set inter-character spacing adjustment | A, O |
| C4 | SVI — Set Variable-Space Character Increment | Set increment applied to variable-space characters | A, O |
| 74 | STC — Set Text Color | Set text color (carries a color specification) | A, O |
| F2 | BSU — Begin Suppression | Begin conditionally suppressed text block | A, O |
| F4 | ESU — End Suppression | End conditionally suppressed text block | A, O |
| F0 | SCFL — Set Coded Font Local | Switch font: select coded font by local ID | A, O |
| F6 | STO — Set Text Orientation | Set text orientation / rotation | A, O |
| DA | TRN — Transparent Data | Following bytes are literal text (no control interpretation) | A, O |
| EE | RPS — Repeat String | Repeat a character/string N times | A, O |
| E4 | DIR — Draw I-Axis Rule | Draw a rule along the inline axis | A, O |
| E6 | DBR — Draw B-Axis Rule | Draw a rule along the baseline axis | A, O |
| D8 | BLN — Begin Line Next | Begin a new line (advance to next line) | A, O |
| F8 | NOP — No Operation (PTOCA) | No operation (afplib names this class `NOPCS`) | A, O |

afplib-only PTOCA codes (single source, **excluded** from contract): 72 (OVS Overscore),
76 (USC Underscore), 78 (TBM Temporary Baseline Move), 80 (SEC — no description in source).
openafp's lookup table labels ESU "Begin Suppression" — an obvious copy/paste typo for
"End Suppression" (its code pairing F2/BSU vs F4/ESU is correct).

**Task-text note:** the assignment mentioned "AMB, AMC" and "SIC". No source defines
`AMC` or `SIC`: both A and O define `AMI` (Absolute Move Inline, 0xC6) and `SIA`
(Set Intercharacter Adjustment, 0xC2) instead. Treated as task-text misnomers; see Conflicts.

## 4. Exclusion field IDs

All corroborated by A + P + O unless noted.

**GOCA graphics:** BGR D3A8BB / EGR D3A9BB (Begin/End Graphics Object), GAD D3EEBB (Graphics Data),
GDD D3A6BB (Graphics Data Descriptor).

**IOCA image:** BIM D3A8FB / EIM D3A9FB (Begin/End Image Object), IPD D3EEFB (Image Picture Data),
IDD D3A6FB (Image Data Descriptor), IID D3A67B (IM Image Input Descriptor), IOC D3A77B
(IM Image Output Control), ICP D3AC7B (IM Image Cell Position), IRD D3EE7B (IM Image Raster Data).

**BCOCA barcode:** BBC D3A8EB / EBC D3A9EB (Begin/End Bar Code Object), BDA D3EEEB (Bar Code Data),
BDD D3A6EB (Bar Code Data Descriptor), MBC D3ABEB (Map Bar Code Object).

**FOCA font resources:** BCF D3A88A / ECF D3A98A (Begin/End Coded Font), BFN D3A889 / EFN D3A989
(Begin/End Font), BCP D3A887 / ECP D3A987 (Begin/End Code Page), CFC D3A78A (Coded Font Control),
CFI D38C8A (Coded Font Index), CPC D3A787 (Code Page Control), CPD D3A687 (Code Page Descriptor),
CPI D38C87 (Code Page Index), FNC D3A789 (Font Control), FND D3A689 (Font Descriptor),
FNG D3EE89 (Font Patterns), FNI D38C89 (Font Index), FNM D3A289 (Font Patterns Map),
FNN D3AB89 (Font Names), FNO D3AE89 (Font Orientation), FNP D3AC89 (Font Position),
MCF1 D3B18A (Map Coded Font Format-1).

**CMOCA / color management:** BCA D3A877 / ECA D3A977 (Begin/End Color Attribute Table),
MCA D3AB77 (Map Color Attribute Table), CAT D3B077 (Color Attribute Table).

**Overlays / medium constructs:** BMO D3A8DF / EMO D3A9DF (Begin/End Overlay),
BMM D3A8CC / EMM D3A9CC (Begin/End Medium Map), MMO D3B1DF (Map Medium Overlay),
MPO D3ABD8 (Map Page Overlay), IMM D3ABCC (Invoke Medium Map), MCC D3A288 (Medium Copy Count),
MDD D3A688 (Medium Descriptor), MMC D3A788 (Medium Modification Control),
BRG D3A8C6 / ERG D3A9C6 (Begin/End Resource Group), BSG D3A8D9 / ESG D3A9D9
(Begin/End Resource Environment Group), BDG D3A8C4 / EDG D3A9C4
(Begin/End Document Environment Group), BOG D3A8C7 / EOG D3A9C7
(Begin/End Object Environment Group — the overlay's "OEG" construct; no structured field
literally named OEG exists in any source).

**FormDef / PageDef carriers:** BFM D3A8CD / EFM D3A9CD (Begin/End Form Map = FormDef),
BPM D3A8CB / EPM D3A9CB (Begin/End Page Map = PageDef), BDM D3A8CA / EDM D3A9CA
(Begin/End Data Map), BFG D3A8C5 / EFG D3A9C5 (Begin/End Form Environment Group),
FGD D3A6C5 (Form Environment Group Descriptor), LND D3A6E7 (Line Descriptor),
LNC D3AAE7 (Line Descriptor Count).

**Object containers:** BOC D3A892 / EOC D3A992 (Begin/End Object Container), OCD D3EE92
(Object Container Data).

**Misc skip:** NOP D3EEEE (No Operation), TLE D3A090 (Tag Logical Element),
IPS D3AF5F (Include Page Segment), MPS D3B15F (Map Page Segment).

## 5. Structured-field introducer layout & 5A record wrapping

Byte offsets are 0-based from the start of each record. Confirmed by afplib README's
annotated hex breakdown, the `hello.afp` sample (`5A 00 2B D3 A8 A8 …`), afplib
`AfpInputStream`, and parse-afp `Record.pm` FORMAT (`CC=>'H2', Length=>'n', Type=>'H6',
FlagByte=>'a', _=>'n', Data=>'a*'`).

| Offset | Size | Field |
|--------|------|-------|
| 0 | 1 | `5A` — carriage-control / magic byte starting every structured field |
| 1–2 | 2 | Length, big-endian uint16 = total structured-field length **excluding** the 5A byte (includes the 2 length bytes themselves; afplib does `length -= 2` then reads that many bytes) |
| 3–5 | 3 | SFID: class `D3`, type, category (e.g. `D3 A8 A8` = BDT) |
| 6 | 1 | Flags byte |
| 7–8 | 2 | Reserved |
| 9+ | n | Structured-field data |

Notes:
- afplib tolerates up to 4 MVS-style leading length bytes before the first `5A`, and files
  with no `5A` at all (falls back to expecting `D3` at offset 2).
- parse-afp requires the record to start with `5a` (`valid_memberdata`).
- Triplet introducer for self-identifying (chained) triplets and PTX control sequences is
  `2B D3` (afplib `AbstractStructuredFieldFactory.cs()` requires it; parse-afp documents
  PTX `EscapeSequence => 'H4'  # 2BD3`; openafp PTX record carries an `N2` escape-sequence field).

## 6. Conflicts / open questions

1. **Triplet ID 0x21 — genuine name conflict.** afplib maps 0x21 → `ResourceObjectType`;
   openafp maps 0x21 → `T_OFSS` ("Object Function Set Specification") and puts
   `T_ROT` ("Resource Object Type") at 0x22. afplib additionally contains a dead
   `case -33` for `ObjectFunctionSetSpecification` (unreachable since the ID is read
   unsigned) — suggesting afplib's 0x21 mapping is the buggy one. openafp's assignment
   matches the MO:DCA reference. **Recommendation:** adopt openafp's mapping
   (0x21 = Object Function Set Specification, 0x22 = Resource Object Type), or leave
   0x21 out of the contract pending a check against the MO:DCA reference PDF.
2. **AMC / SIC in the task text do not exist in any source.** Both A and O define
   `AMI` (0xC6, Absolute Move Inline) and `SIA` (0xC2, Set Intercharacter Adjustment);
   no `AMC` or `SIC` control sequence was found. Assumed to be misnomers for AMI / SIA.
3. **No structured field named "OEG" exists** in any source. The overlay environment-group
   delimiters are BOG/EOG (D3A8C7/D3A9C7). Contract should reference BOG/EOG, not OEG.
4. **Single-source entries excluded by the 2-of-3 rule:** SFIDs BPF/EPF (D3A8A5/D3A9A5),
   BRS/ERS (D3A8CE/D3A9CE), MMD (D3ABCD), PEC (D3A7A8), PPO (D3ADC3) — afplib only;
   PTOCA codes 0x72/0x76/0x78/0x80 — afplib only; triplet IDs listed in §2 tail — one source each.
5. **openafp typo:** its lookup table describes ESU (0xF4) as "Begin Suppression"; the
   code pairing (F2=BSU/F4=ESU) confirms it means "End Suppression".
6. **Naming-only difference, not a conflict:** PTOCA 0xF8 is class `NOPCS` in afplib vs
   `PTX_NOP` in openafp — same code, same meaning ("No Operation").
7. **BTD/EDT recollection hazard:** BDT = D3A8A8 and EDT = D3A9A8 (not D3A688/D3A988);
   verified against the `hello.afp` byte sample and all three tables.
