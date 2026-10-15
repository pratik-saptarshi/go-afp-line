package afptoline

import "testing"

// TestMedium_Geometry: contracted fixed geometry (US Letter, 612x792pt) is the
// v1 fallback per subset-contract §7 — fixtures carry no AEG/PTD, so every page
// resolves to it.
func TestMedium_Geometry(t *testing.T) {
	w, h := resolvePageGeometry()
	if w != 612 || h != 792 {
		t.Fatalf("resolvePageGeometry() = (%v, %v), want (612, 792)", w, h)
	}
}

// TestMedium_FlipY: PTOCA object space has Y increasing downward; PDF user
// space has Y increasing upward. The flip is anchored on the page height.
func TestMedium_FlipY(t *testing.T) {
	cases := []struct {
		ptocaY, pageH, want float64
	}{
		{36, 792, 756},
		{72, 792, 720},
		{0, 792, 792},
		{792, 792, 0},
	}
	for _, c := range cases {
		if got := flipY(c.ptocaY, c.pageH); got != c.want {
			t.Errorf("flipY(%v, %v) = %v, want %v", c.ptocaY, c.pageH, got, c.want)
		}
	}
}
