package afp_test

import (
	"bytes"
	"fmt"

	"github.com/pratik-saptarshi/go-afp-line/afp"
)

// ExampleFields iterates the structured fields of a naked (unwrapped) AFP
// stream. Each field's Data starts at its introducer; Offset is the wire
// offset of the field's first byte.
func ExampleFields() {
	// Two minimal 8-byte fields (introducer only): SFLength 8, type D3A8AD.
	stream := []byte{
		0x00, 0x08, 0xD3, 0xA8, 0xAD, 0x00, 0x00, 0x00,
		0x00, 0x08, 0xD3, 0xA8, 0xAD, 0x00, 0x00, 0x00,
	}
	count := 0
	for f, err := range afp.Fields(bytes.NewReader(stream)) {
		if err != nil {
			fmt.Println("error:", err)
			return
		}
		fmt.Printf("field %d at offset %d, %d bytes\n", count, f.Offset, len(f.Data))
		count++
	}
	fmt.Println("total:", count)
	// Output:
	// field 0 at offset 0, 8 bytes
	// field 1 at offset 8, 8 bytes
	// total: 2
}
