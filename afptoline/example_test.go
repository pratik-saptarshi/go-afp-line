package afptoline_test

import (
	"bytes"
	"fmt"

	"github.com/pratik-saptarshi/go-afp-line/afptoline"
)

// field assembles one naked-framed structured field: an 8-byte introducer
// (SFLength big-endian including the introducer, D3 type triple, flags,
// X'0000') followed by data.
func field(typ [3]byte, data []byte) []byte {
	n := 8 + len(data)
	out := []byte{byte(n >> 8), byte(n), typ[0], typ[1], typ[2], 0x00, 0x00, 0x00}
	return append(out, data...)
}

// ebcdic037 encodes s in EBCDIC code page 037 (letters and space only;
// sufficient for this example).
func ebcdic037(s string) []byte {
	table := map[rune]byte{'H': 0xC8, 'i': 0x89, ' ': 0x40}
	out := make([]byte, 0, len(s))
	for _, r := range s {
		out = append(out, table[r])
	}
	return out
}

// trn wraps EBCDIC bytes in a PTOCA Transparent Data (TRN) control sequence.
func trn(data ...byte) []byte {
	out := []byte{0x2B, 0xD3, byte(2 + len(data)), 0xDA}
	return append(out, data...)
}

// ExampleReduce reduces a minimal one-page AFP document — BDT/BPG framing,
// one PTX carrying "Hi" inside a BAG group, then EPG/EDT — to line data.
func ExampleReduce() {
	var stream bytes.Buffer
	bdt := [3]byte{0xD3, 0xA8, 0xA8}
	bpg := [3]byte{0xD3, 0xA8, 0xAF}
	bag := [3]byte{0xD3, 0xA8, 0xC9}
	ptx := [3]byte{0xD3, 0xEE, 0x9B}
	eag := [3]byte{0xD3, 0xA9, 0xC9}
	epg := [3]byte{0xD3, 0xA9, 0xAF}
	edt := [3]byte{0xD3, 0xA9, 0xA8}
	spaces := bytes.Repeat([]byte{0x40}, 8) // EBCDIC-037 spaces
	for _, f := range [][]byte{
		field(bdt, append(spaces, 0x00, 0x00)),
		field(bpg, spaces),
		field(bag, nil),
		field(ptx, trn(ebcdic037("Hi")...)),
		field(eag, nil),
		field(epg, spaces),
		field(edt, append(spaces, 0x00, 0x00)),
	} {
		stream.Write(f)
	}

	doc, excls, err := afptoline.Reduce(&stream)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println("pages:", len(doc.Pages))
	fmt.Println("runs:", len(doc.Pages[0].Runs))
	fmt.Println("text:", doc.Pages[0].Runs[0].Text)
	fmt.Println("exclusions:", len(excls))
	// Output:
	// pages: 1
	// runs: 1
	// text: Hi
	// exclusions: 0
}
