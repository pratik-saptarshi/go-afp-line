# AFP File Format Specification (working reference)

Distilled from the AFP Consortium references below. This is a working summary for
implementers — paraphrased, not a reproduction. For authoritative detail, consult the sources.

**Sources:**
- MO:DCA Reference (MODCA-Reference-09): https://afpcinc.org/wp-content/uploads/2017/12/MODCA-Reference-09.pdf
- MOCA Reference (AFPC-0013-01): https://afpcinc.org/wp-content/uploads/2016/08/MOCA-Reference-Metadata-Object-Content-Architecture-Reference.pdf

Local copies: `docs/_evidence/modca/MODCA-Reference-09.pdf`, `docs/_evidence/modca/MOCA-Reference.pdf`.

---

## 1. What an AFP file is

An AFP document is a **MO:DCA data stream**: an ordered sequence of self-describing
binary records called **structured fields**. Each structured field carries a typed
parameter set. Documents are delimited by begin/end pairs (e.g. BDT…EDT for the
document, BPG…EPG for each page). Presentation content (text, images, graphics,
barcodes) lives in **data objects** governed by their own object architectures
(PTOCA for text, IOCA for images, GOCA for graphics, BCOCA for barcodes).

## 2. Record framing

MO:DCA defines the structured fields, not the transport record. Platforms may wrap
each structured field in a platform record (for example, prefixing extra bytes such
as an `X'5A'` marker), which can make the on-disk record longer than the structured
field itself. A portable decoder therefore:

1. Strips/recognizes platform record wrapping first (accept both naked streams and
   `X'5A'`-wrapped variable-blocked records).
2. Parses the structured field starting at its introducer length field.

The reference recommends capping structured-field length at `X'7FF0` (32,752) for
portability, since some platforms misread longer record lengths as negative.

## 3. Structured Field Introducer (SFI) — 8 bytes

Every structured field begins with an 8-byte introducer (longer if extended):

| Offset | Size | Name | Meaning |
|---|---|---|---|
| 0–1 | 2B | SFLength | Unsigned big-endian total length of the structured field **including the introducer itself**. Range 8–32,767. |
| 2–4 | 3B | SFTypeID | `D3TTCC`: class `D3` (MO:DCA), type code `TT`, category code `CC`. Uniquely identifies the field. |
| 5 | 1B | Flags | Bit 0: extension present; Bit 2: data segmented; Bit 4: padding present. Other bits reserved (zero). |
| 6–7 | 2B | Reserved | `X'0000`. |

If bit 0 of Flags is set, an extension follows: 1-byte ExtLength (1–255, includes
itself) + extension bytes (reserved). Introducer is then 8–263 bytes.

**Example:** a 17-byte BDT field: `00 11 D3 A8 A8 00 00 00` + 9 data bytes.

## 4. Type codes (`TT` byte)

| Code | Function | Role |
|---|---|---|
| `A0` | Attribute | Name/value attributes |
| `A2` | Copy Count | Sheet-copy subgroups |
| `A6` | Descriptor | Page/object characteristics, units |
| `A7` | Control | Copy-subgroup modifications |
| `A8` | **Begin** | Opens a document component (carries its name) |
| `A9` | **End** | Closes a document component |
| `AB` | Map | Resource mapping (e.g. map coded font) |
| `AC` | Position | Coordinate offset / orientation |
| `AD` | Process | Processing directives |
| `AF` | Include | Embed a named resource inline |
| `B1` | Migration | Format-1 vs Format-2 disambiguation |
| `B2` | Variable | Variable data |
| `B4` | Link | Logical linkage between components |
| `EE` | **Data** | Object content governed by an object architecture (e.g. PTX) |

## 5. Category codes (`CC` byte)

| Code | Component | Code | Component |
|---|---|---|---|
| `5F` | Page Segment | `A8` | **Document** |
| `6B` | Object Area | `AD` | Page Group |
| `7B` | IM Image | `AF` | **Page** |
| `88` | Medium | `BB` | Graphics |
| `8A` | Coded Font | `C3` | Data Resource |
| `90` | Process Element | `C4` | Document Env. Group |
| `92` | Object Container | `C6` | Resource Group |
| `9B` | **Presentation Text** | `C7` | Object Env. Group |
| `A5` | Print File | `C9` | Active Env. Group |
| `A7` | Index | `CC`/`CD` | Medium Map / Form Map |
| `D8`/`DF` | Page Overlay / Overlay | `EA` | Data Suppression |
| `D9` | Resource Env. Group | `EB` | Bar Code |
| `DC` | Preprinted Form Overlay | `EE` | No Operation |
| `CE` | Name Resource | `FB` | Image |

## 6. Core structured fields (document skeleton subset)

| Field | ID | Params (offsets relative to data) |
|---|---|---|
| BDT Begin Document | `D3A8A8` | DocName CHAR[8] (0–7), reserved (8–9), triplets (10–n) |
| EDT End Document | `D3A9A8` | DocName CHAR[8], reserved, triplets |
| BPG Begin Page | `D3A8AF` | PageName CHAR[8] optional (0–7), triplets (8–n) |
| EPG End Page | `D3A9AF` | PageName CHAR[8] optional, triplets |
| PTX Presentation Text Data | `D3EE9B` | PTOCA data bytes (0–n), up to 32,759 |
| BOC Begin Object Container | `D3A892` | Carries object containers (e.g. MOCA metadata objects) |
| EOC End Object Container | `D3A992` | Closes the container |
| TLE Tag Logical Element | — | Attribute name/value pairs on pages/page groups (indexing, no presentation effect) |
| NOP No Operation | `D3EEEE` | Ignored |

Begin/end pairs must match; a BPG without a following EPG is an exception (`X'08`).

## 7. Triplets

Secondary data structure appearing inside structured-field parameter sets:

| Offset | Size | Name | Meaning |
|---|---|---|---|
| 0 | 1B | Tlength | Total triplet length including itself. Range 3–254. |
| 1 | 1B | Tid | Triplet identifier. Unknown Tid → `X'10` exception. |
| 2–n | nB | Contents | Triplet data, format depends on Tid. |

Notable triplets: `X'01` Coded Graphic Character Set Global Identifier (establishes the
code page / character set for interpreting CHAR parameters in scope — GCSGID/CPGID
or CCSID form); `X'02` Fully Qualified Name (e.g. type `X'01` Replace First GID Name,
overrides the begin field's name); `X'5A` Object Offset (counts object types in a
container/index).

## 8. Segmentation and padding

- If introducer + data exceeds 32,767 bytes, data is split across consecutive
  structured fields (Flag bit 2). Normally split on OCA data-element boundaries;
  if bit 2 is set on non-final fragments, the split may occur at any byte and no
  other field may intervene. The final fragment has bit 2 clear.
- Padding (Flag bit 4): 1–2 pad bytes → length in the last pad byte; 256–32,759
  pad bytes → length in the last 3 bytes (last byte `X'00`, two preceding bytes
  = length).

## 9. Character encoding

Text parameters use EBCDIC code pages. The `X'01` triplet (CGCSGID) sets the code
page (CPGID) and character set (GCSGID) for CHAR parameters in its scope; without
it, a default/code-page-037-style single-byte EBCDIC applies. PTOCA text bytes are
decoded per the active code page — multi-code-page streams switch via font/code-page
controls, which is why text extraction must track code-page state, not assume one.

## 10. MOCA metadata objects

MOCA defines a **Metadata Object (MO)** — descriptive metadata (no presentation
semantics) carried inside a MO:DCA **object container** (BOC/EOC, category `X'92`).
One MO type/format is defined: type `DES` (descriptive), format **XMP**.

MO layout (offsets from MO start):

| Offset | Size | Name | Meaning |
|---|---|---|---|
| 0–3 | 4B | MOLength | Total MO length incl. itself, `X'00000032`–`X'FFFFFFFF` |
| 4–5 | 2B | HeaderLength | Header length incl. itself, ≥ 46 (`X'002E`) |
| 6–11 | 6B | MOType | UTF-16BE `"DES"` |
| 12–19 | 8B | MOFormat | UTF-16BE `"XMP "` |
| 20–39 | 20B | MOCompression | UTF-16BE `"NONE"`, `"GZIP"`, or `"EXI"` (padded with `X'0040`) |
| 40–47 | 8B | Reserved | Zero |
| 48–49 | 2B | MONameLength | Even, 0–250 |
| 50–… | 0–250B | MOName | Optional UTF-16BE human-readable name |
| … | — | MOData | The XMP payload (possibly compressed per MOCompression) |

Invalid values raise optional exceptions (`EC-0100`…`EC-0250`). For the AFP→PDF
pipeline, MOs are **out of scope for rendering** but in scope for *detection*:
a BOC/EOC container carrying an MO must be recognized (not misparsed as image
data) and reported via the exclusion/diagnostic path.

## 11. Minimal document skeleton

```
BDT (D3A8A8) "DOCNAME "
  BPG (D3A8AF) "PAGE0001"
    PTX (D3EE9B) <PTOCA bytes>
    ...
  EPG (D3A9AF)
EDT (D3A9A8)
```

Real streams interleave environment groups (BAG/EAG with descriptors and map
coded fonts), resource groups, and object containers between these delimiters.

## 12. Implementer notes (decoder pitfalls)

1. **SFI is 8 bytes, not 6.** The length field comes first; the type ID is at
   offset 2. A frequent bug is treating `D3…` as the field start.
2. **SFLength includes the introducer.** Data length = SFLength − introducer length.
3. **Never `malloc(SFLength)` before validating.** Cap claimed lengths (e.g. 1 MiB)
   and return a typed error on violation — hostile streams lie about length.
4. **Framing first.** Strip platform record wrapping before parsing the SFI.
5. **Begin/end pairing is structural.** Track a stack; mismatches are errors, not
   warnings.
6. **CHAR parameters are EBCDIC** and scoped by the nearest `X'01` triplet —
   decoding them as ASCII/Latin-1 is the classic mojibake bug.
7. **Segmented fields must be reassembled** before interpreting OCA data.
8. **Unknown TypeIDs are not skippable by blind length alone** unless you also
   preserve them: byte-identical round-trip requires re-emitting unknown fields
   verbatim (the `RawField` passthrough pattern).
