# PTOCA function type 0x80 — Research Evidence

Date: 2026-10-22. Research for the go-afp-line 0x80 product decision (chained 0x80 in the
0020 profile file aborts `Reduce`). Primary sources only: the normative IBM PTOCA Reference,
the AFP Consortium programming guide, and open-source implementations' source code.

## Headline

**PTOCA function type `0x80` is SEC — Set Extended Text Color.** It is defined by IBM, not an
afplib invention. Unchained form is byte `0x80`; chained form is `0x81` (low bit = chain bit).
It is a modal control sequence that sets the foreground text color (RGB, CMYK, CIELAB,
highlight, or standard OCA color space). It affects text **appearance** (color), not
positioning or layout.

## 1. What afplib defines 0x80 as

afplib (yan74/afplib, Java/EMF AFP library) maps function byte 128 (`0x80`) to **SEC**:

- Parse dispatch, `org.afplib/src/main/java/org/afplib/io/StructuredFieldFactory.java:10319-10320`:
  `case 128 : { SEC obj = AfplibFactory.eINSTANCE.createSEC();` — inside `Triplet cs(...)`,
  the PTOCA control-sequence parser (same switch handles `case 210` → AMB `0xD2`,
  `case 198` → AMI `0xC6`, `case 238` → RPS `0xEE`).
- Save dispatch, same file (~22560): `case AfplibPackage.SEC : { m.setTripletId(128);`
- Chain-bit handling on save, same file (22654–22658):
  `buffer[start] = Data.toUnsignedByte(128 & 0xfe)[0]; // unchain it` and
  `buffer[start] = Data.toUnsignedByte(128 | 0x01)[0]; // chain it`
  — i.e. afplib treats the low bit of the function byte as the chain bit, `0x80` unchained.
- Data model, `org.afplib/src/main/java/org/afplib/afplib/SEC.java`: fields
  `RESERVED`, `COLSPCE`, `COLSIZE1`–`COLSIZE4`, `COLVALUE` — matching IBM's SEC layout.

## 2. What IBM defines 0x80 as

IBM **does** define function type `0x80`. Two IBM sources:

**a) Presentation Text Object Content Architecture Reference, SC31-6803-02**
(IBM official publication library, publibz.boulder.ibm.com/epubs/pdf/ha3p2m00.pdf) —
the normative PTOCA spec. Chapter 4, the SEC control-sequence chapter (p. 79–80):

- `TYPE X'80' - X'81'` — the syntax table's offset-3 field, identifying the
  control sequence's function type.
- Full syntax: `PREFIX X'2B'` (offset 0), `CLASS X'D3'` (offset 1), `LENGTH 14-16`
  (offset 2), `TYPE X'80'-X'81'` (offset 3), reserved zero (offset 4), `COLSPCE`
  (offset 5: `X'01'` RGB, `X'04'` CMYK, `X'06'` highlight color space, `X'08'` CIELAB,
  `X'40'` standard OCA color space), reserved (offsets 6–9), `COLSIZE1`–`COLSIZE4`
  (offsets 10–13), `COLVALUE` (offsets 14–17).
- Semantics (paraphrased from p. 79–80): SEC carries a color value together with its
  color space and encoding; the color applies to the foreground areas of the text
  presentation space (glyph strokes/fills, rules, underscores). It is modal — it
  remains in effect until superseded. STC and SEC may both appear in a stream;
  the most recently issued one wins. An invalid or unsupported color space raises
  exception EC-0E02, whose standard action is to fall back to the device default
  color.

**b) AFP Programming Guide and Line Data Reference, AFPC-0010-05** (6th ed., Feb 2018;
AFP Consortium, successor to IBM S544-3884-04; afpconsortium.org). Table 11, the
PTX control-sequence table (p. 60), PT3 section:

- `Set Extended Text Color — xx80 (Unchained / Even Function), xx81 (Chained / Odd Function)`
- Text (p. 58, paraphrased): PT3 extends PT2 with the SEC sequence to support spot and process colors in text.
- Alphabetical index (p. ~190): `SEC 80(81) — Set Extended Text Color`.

## 3. Other primary sources

| Source | Defines 0x80? | Entry |
|--------|--------------|-------|
| IBM PTOCA Reference SC31-6803-02 | **Yes** | SEC, `X'80'`–`X'81'`, modal, full syntax + semantics (p. 79–80) |
| AFPC Programming Guide AFPC-0010-05 | **Yes** | SEC, `xx80`/`xx81`, PT3 subset (Table 11, p. 60) |
| afplib (yan74/afplib) | **Yes** | `case 128 → createSEC()`; `SEC → tripletId 128`; chain-bit mask `0xfe`/`0x01` |
| Apache FOP (xmlgraphics-fop) | **Yes** | `PtocaConstants.java:56`: `byte SEC = (byte)0x80;` — `PtocaBuilder.setExtendedTextColor` emits IBM-conformant COLSPCE/COLSIZE/COLVALUE bytes |
| openafp (audreyt/openafp) | No | 19 PTX control-sequence types; no SEC (verified against current HEAD `src/OpenAFP/Prelude/InstancePTX.hs`) |
| Parse::AFP 0.25 (Perl, AUDREYT) | Neither | `PTX.pm` has no per-function table — control sequences handled generically; chain bit handled as `\| "\x01"` / `^ "\x01"` in `_refresh` |

No conflicts found: every source that names `0x80` agrees it is SEC / Set Extended Text Color
with the low-bit chain convention. (The known afplib error was in its SFID table — `0x21`
ObjectFunctionSetSpecification — not its PTOCA table.)

## 4. Presentation-relevance and data semantics

**Presentation-relevant: yes — color only.** SEC sets the foreground color of subsequent text
(modal: stays in effect until another STC/SEC). It does not move the print position, change
fonts, or alter layout. Per IBM, the sanctioned degradation for an unsupported color space is
the **device default color** (exception EC-0E02) — i.e. IBM's own sanctioned fallback
is to leave the color unapplied, which bounds the risk of a pass-through: worst case is
wrong-colored text, never mispositioned or corrupted text.

Data bytes (offsets relative to the `2BD3` escape): reserved `00`, `COLSPCE`, 4 reserved
bytes, `COLSIZE1`–`4`, then `COLVALUE` (1–4 components per the color space).

## 5. The chained bit

Per the AFPC Programming Guide (p. 58): a control sequence may be unchained (even function number) or chained (odd function number). The chain bit is the
**low-order bit of the function-type byte**. A chained sequence directly abuts the
previous one — no `X'2BD3'` escape intervenes — and a chain is terminated by a final
sequence in the even-numbered (unchained) form, which signals the end of the chain.
Format: first byte = total length (length byte + function byte + parameters), second byte =
function code, then 0–253 parameter bytes.

So a "chained `0x80`" on the wire is byte **`0x81`** — function type SEC with the chain bit
set, meaning at least one more control sequence follows in the same chain. go-afp-line's
`unknown PTOCA control sequence 0x80` diagnostic already reports the chain-masked base
type, which is correct behavior for the lookup.

## 6. The actual specimen in the 0020 profile file

Decoded from the profile file's PTX at field offset 6039 (read-only local copy, SHA-256
`553a336d…b73a`; bytes verified 2026-10-22):

```
seq3: len=15 func=0x81 base=0x80 chained=true
      params = 00 01 00 00 00 00 08 08 08 00 ff ff ff
```

Per the IBM syntax: reserved `00` ✓, `COLSPCE = 0x01` (RGB), reserved ×4 ✓,
`COLSIZE = 08 08 08 00` (8 bits per R/G/B component), `COLVALUE = ff ff ff` → **white**.
Length 15 is within IBM's 14–16 range. This is a well-formed SEC requesting RGB white,
chained (further sequences follow — AMI/AMB/DIR moves in the same chain).

## 7. Implication for the go-afp-line decision (inference, not source fact)

- The premise "0x80 is afplib-only" is **wrong as a general claim**. It was true only of the
  panel's three code sources (afplib / parse-afp / openafp): parse-afp is function-agnostic
  and openafp simply lacks SEC. IBM's normative PTOCA Reference defines it, the AFPC
  programming guide lists it, and Apache FOP emits it. Under the contract's own "IBM docs
  primary" spirit, `0x80` = SEC is corroborated by IBM + afplib + FOP.
- The sibling codes `0x72/0x76/0x78` were **not** checked against IBM docs in this pass —
  their "afplib-only" status is unverified either way. Do not generalize this finding to them.
- go-afp-line already tracks STC (`0x74`) via `doSTC` (records the raw color spec, not
  applied to advancement) in `afptoline/ptoca.go`. SEC is STC's extended-color sibling;
  the same record-without-applying treatment is the natural, IBM-sanctioned shape
  (IBM's own fallback for unhandled color is the device default).
- Risk of admitting SEC as accept-and-note: bounded — worst case is text rendered in the
  default color instead of the specified one; no positioning or layout impact is possible
  from this control sequence per its IBM semantics.
