package afptoline

// Medium resolution: page geometry for reduction.
//
// v1 implements the subset contract's fallback: contracted fixed geometry.
// The contract (§7) resolves geometry from the Active Environment Group's text
// descriptors (PTD) when present; fixtures carry no AEG, so every page in the
// v1 corpus resolves here. A later task derives geometry from BAG/EAG+PTD in
// this file without changing Reduce's call shape.

// DefaultPageWidth and DefaultPageHeight are the contracted fixed page
// dimensions in points: US Letter (8.5 x 11 in), per subset-contract §7
// ("contracted fixed geometry is the v1 fallback").
const (
	DefaultPageWidth  = 612.0
	DefaultPageHeight = 792.0
)

// ResolvePageGeometry returns the page dimensions in points for the page
// currently being reduced. v1 always returns the contracted fixed geometry;
// AEG/PTD-derived geometry arrives here in a later task.
func ResolvePageGeometry() (width, height float64) {
	return DefaultPageWidth, DefaultPageHeight
}

// FlipY converts a PTOCA object-space Y coordinate (origin at the top of the
// page, increasing downward) to PDF user-space Y (origin at the bottom-left,
// increasing upward), anchored on the page height.
func FlipY(ptocaY, pageHeight float64) float64 {
	return pageHeight - ptocaY
}
