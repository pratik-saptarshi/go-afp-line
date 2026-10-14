package main

// Hand-authored AFP fixture corpus (Task 7).
//
// ANTI-CIRCULARITY RULE (load-bearing): every byte below is hand-authored
// per the MO:DCA references and the module's file-format spec. The generator
// only ASSEMBLES hand-written byte slices (SFI framing + field-data
// concatenation). It never calls afp decode/encode to produce bytes.
// afp.RoundTrip is used solely as a CHECK in main_test.go.
// Every hex constant carries a spec-citation comment.

// ebcdic037 is a LOCAL hand-verified EBCDIC-037 encode map for the graphic
// characters used in fixture text runs. It does NOT depend on afp's
// unexported tables (anti-circularity: fixtures must never be output of the
// library under test). Each entry was verified against afp/codepage.go
// cp037Table rows 0x40 (space), 0xC1-0xC9 (A-I), 0xD1-0xD9 (J-R),
// 0xE2-0xE9 (S-Z), 0xF0-0xF9 (0-9) and cross-checked with Python's cp037
// codec before use.
var ebcdic037 = map[rune]byte{
	' ': 0x40,
	'A': 0xC1, 'B': 0xC2, 'C': 0xC3, 'D': 0xC4, 'E': 0xC5,
	'F': 0xC6, 'G': 0xC7, 'H': 0xC8, 'I': 0xC9,
	'J': 0xD1, 'K': 0xD2, 'L': 0xD3, 'M': 0xD4, 'N': 0xD5,
	'O': 0xD6, 'P': 0xD7, 'Q': 0xD8, 'R': 0xD9,
	'S': 0xE2, 'T': 0xE3, 'U': 0xE4, 'V': 0xE5, 'W': 0xE6,
	'X': 0xE7, 'Y': 0xE8, 'Z': 0xE9,
	'0': 0xF0, '1': 0xF1, '2': 0xF2, '3': 0xF3, '4': 0xF4,
	'5': 0xF5, '6': 0xF6, '7': 0xF7, '8': 0xF8, '9': 0xF9,
}

// graphic encodes s to EBCDIC-037 bytes using the local map only.
// Unknown runes panic: fixtures must fail loudly, never guess a code point.
func graphic(s string) []byte {
	b := make([]byte, 0, len(s))
	for _, r := range s {
		c, ok := ebcdic037[r]
		if !ok {
			panic("genfixture: no EBCDIC-037 mapping for rune " + string(r))
		}
		b = append(b, c)
	}
	return b
}

// sfi assembles one structured field: the 8-byte introducer followed by the
// hand-authored field data.
// SFI: SFLength(2B BE, includes introducer) | D3 TT CC | flags(1B) | X'0000'
// — file-format-spec §3. SFLength is 8 + len(data); naked framing (no 5A wrap).
func sfi(typ [3]byte, flags byte, data []byte) []byte {
	n := 8 + len(data)
	out := make([]byte, 0, n)
	out = append(out, byte(n>>8), byte(n)) // SFLength, big-endian
	out = append(out, typ[0], typ[1], typ[2])
	out = append(out, flags, 0x00, 0x00) // flags, reserved X'0000'
	out = append(out, data...)
	return out
}

// Structured field type IDs (D3 TT CC) — file-format-spec §3.
var (
	sfBDT = [3]byte{0xD3, 0xA8, 0xA8} // Begin Document
	sfEDT = [3]byte{0xD3, 0xA9, 0xA8} // End Document
	sfBPG = [3]byte{0xD3, 0xA8, 0xAF} // Begin Page
	sfEPG = [3]byte{0xD3, 0xA9, 0xAF} // End Page
	sfPTX = [3]byte{0xD3, 0xEE, 0x9B} // Presentation Text Data
	sfBGR = [3]byte{0xD3, 0xA8, 0xBB} // Begin Graphics (GOCA, known-excluded)
	sfIMM = [3]byte{0xD3, 0xAB, 0xCC} // Invoke Medium Map (known-excluded)
)

// BDT field data, DocName "STMT0001" with a Comment triplet "FIXTURE".
// BDT layout — file-format-spec §3: DocName CHAR[8] (EBCDIC-037) + reserved
// X'0000' + triplets. SFLength = 8 + 8 + 2 + 9 = 27 = 0x001B.
var bdtCommentFixture = concat(
	[]byte{0xE2, 0xE3, 0xD4, 0xE3, 0xF0, 0xF0, 0xF0, 0xF1}, // "STMT0001" — EBCDIC-037 CHAR[8]
	[]byte{0x00, 0x00}, // reserved X'0000'
	// Comment triplet X'65': Tlength(1B, includes itself) | 65 | contents
	// — file-format-spec §5. Contents = "FIXTURE" (7 bytes EBCDIC-037).
	[]byte{0x09, 0x65}, // Tlength=9, Tid=X'65' Comment
	[]byte{0xC6, 0xC9, 0xE7, 0xE3, 0xE4, 0xD9, 0xC5}, // "FIXTURE"
)

// BDT/EDT field data without triplets (DocName + reserved only).
// SFLength = 8 + 10 = 18 = 0x0012.
var docPlainData = concat(
	[]byte{0xE2, 0xE3, 0xD4, 0xE3, 0xF0, 0xF0, 0xF0, 0xF1}, // "STMT0001" — EBCDIC-037 CHAR[8]
	[]byte{0x00, 0x00}, // reserved X'0000'
)

// EDT data: DocName CHAR[8] mirrored + X'0000' — file-format-spec §3.
var edtData = docPlainData

// BPG/EPG field data: PageName CHAR[8] (EBCDIC-037), no triplets.
// SFLength = 8 + 8 = 16 = 0x0010 — file-format-spec §3.
func pageNameData(name string) []byte {
	b := graphic(name)
	if len(b) != 8 {
		panic("genfixture: PageName must be exactly 8 bytes")
	}
	return b
}

// Unchained PTOCA control sequences — docs/_evidence/ptoca-reference-extract.txt
// ("Absolute Move Baseline (AMB)", unchained X'D2', chained X'D3'):
// 2B (control-sequence prefix) | D3 (class) | 04 (length: itself + type +
// 2 displacement bytes) | D2 (unchained AMB) | displacement (2B BE, L-units).
var (
	amb720  = []byte{0x2B, 0xD3, 0x04, 0xD2, 0x02, 0xD0} // Bcnew = Bo + 720 L-units
	amb1440 = []byte{0x2B, 0xD3, 0x04, 0xD2, 0x05, 0xA0} // Bcnew = Bo + 1440 L-units
)

// PTOCA data for statement-001 page 1: AMB + text run + AMB + text run.
// Graphic characters after a control sequence are EBCDIC-037.
// Data = 6 + 18 + 6 + 18 = 48 bytes; SFLength = 56 = 0x0038.
var ptxPage1Data = concat(
	amb720,
	graphic("STATEMENT LINE ONE"), // 18 bytes EBCDIC-037
	amb1440,
	graphic("STATEMENT LINE TWO"), // 18 bytes EBCDIC-037
)

// PTOCA data for statement-001 page 2: same shape, different lines.
// Data = 6 + 20 + 6 + 19 = 51 bytes; SFLength = 59 = 0x003B.
var ptxPage2Data = concat(
	amb720,
	graphic("STATEMENT LINE THREE"), // 20 bytes EBCDIC-037
	amb1440,
	graphic("STATEMENT LINE FOUR"), // 19 bytes EBCDIC-037
)

// PTOCA data for the single-run pages: one AMB + one text run.
// Data = 6 + 9 = 15 bytes; SFLength = 23 = 0x0017.
var ptxSingleRunData = concat(
	amb720,
	graphic("PAGE TEXT"), // 9 bytes EBCDIC-037
)

// BGR (Begin Graphics, GOCA) — known-excluded per subset-contract §9.
// Minimal well-formed field: 8-byte SFI, SFLength=8, zero data bytes.
// Decodes to RawField with Excluded=true; round-trips byte-identically.
var bgrData = []byte{}

// IMM (Invoke Medium Map) — known-excluded per subset-contract §9. MODCA: an
// IMM not following an internal medium map references a medium map in the
// processing system's form map, i.e. THE FormDef reference.
// Data = MMPName CHAR[8] EBCDIC-037; SFLength = 8 + 8 = 16 = 0x0010.
var immFormDefData = []byte{
	0xC6, 0xD6, 0xD9, 0xD4, 0xC4, 0xC5, 0xC6, 0xF1, // "FORMDEF1" — EBCDIC-037 CHAR[8]
}

// concat is pure byte-slice assembly (no afp package involvement).
func concat(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// buildStatement001 assembles statement-001.afp: BDT("STMT0001" + Comment
// triplet "FIXTURE"), then two pages of BPG / PTX (two text runs each, each
// run preceded by an unchained AMB) / EPG, then EDT. Exercises multiple text
// runs per page, an absolute move (AMB), and the Comment triplet on BDT.
func buildStatement001() []byte {
	var out []byte
	out = append(out, sfi(sfBDT, 0x00, bdtCommentFixture)...)
	out = append(out, sfi(sfBPG, 0x00, pageNameData("PAGE0001"))...)
	out = append(out, sfi(sfPTX, 0x00, ptxPage1Data)...)
	out = append(out, sfi(sfEPG, 0x00, pageNameData("PAGE0001"))...)
	out = append(out, sfi(sfBPG, 0x00, pageNameData("PAGE0002"))...)
	out = append(out, sfi(sfPTX, 0x00, ptxPage2Data)...)
	out = append(out, sfi(sfEPG, 0x00, pageNameData("PAGE0002"))...)
	out = append(out, sfi(sfEDT, 0x00, edtData)...)
	return out
}

// buildExclusionGOCA assembles exclusion-goca.afp: BDT, BPG, one PTX with a
// single text run, BGR (D3A8BB, empty data — the known-excluded GOCA field),
// EPG, EDT. The BGR must decode to RawField{Excluded:true} and round-trip
// byte-identically.
func buildExclusionGOCA() []byte {
	var out []byte
	out = append(out, sfi(sfBDT, 0x00, docPlainData)...)
	out = append(out, sfi(sfBPG, 0x00, pageNameData("PAGE0001"))...)
	out = append(out, sfi(sfPTX, 0x00, ptxSingleRunData)...)
	out = append(out, sfi(sfBGR, 0x00, bgrData)...)
	out = append(out, sfi(sfEPG, 0x00, pageNameData("PAGE0001"))...)
	out = append(out, sfi(sfEDT, 0x00, edtData)...)
	return out
}

// buildMediumRef assembles medium-ref.afp: BDT, IMM(D3ABCC, "FORMDEF1"),
// BPG, one PTX with a single text run, EPG, EDT. The IMM is the FormDef
// reference (known-excluded per subset-contract §9) and must round-trip
// byte-identically.
func buildMediumRef() []byte {
	var out []byte
	out = append(out, sfi(sfBDT, 0x00, docPlainData)...)
	out = append(out, sfi(sfIMM, 0x00, immFormDefData)...)
	out = append(out, sfi(sfBPG, 0x00, pageNameData("PAGE0001"))...)
	out = append(out, sfi(sfPTX, 0x00, ptxSingleRunData)...)
	out = append(out, sfi(sfEPG, 0x00, pageNameData("PAGE0001"))...)
	out = append(out, sfi(sfEDT, 0x00, edtData)...)
	return out
}
