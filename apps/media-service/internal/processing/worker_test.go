package processing

import "testing"

func TestResizeDims_preservesAspect(t *testing.T) {
	// Landscape thumbnail: longest edge (4000) scales to 320, height scales
	// proportionally (3000 * 320/4000 = 240).
	w, h := ResizeDims(4000, 3000, 320)
	if w != 320 || h != 240 {
		t.Fatalf("thumbnail dims: want (320,240), got (%d,%d)", w, h)
	}
}

func TestResizeDims_portrait(t *testing.T) {
	// Portrait: longest edge is height (4000) → height becomes 1280, width
	// scales (3000 * 1280/4000 = 960).
	w, h := ResizeDims(3000, 4000, 1280)
	if w != 960 || h != 1280 {
		t.Fatalf("display dims: want (960,1280), got (%d,%d)", w, h)
	}
}

func TestResizeDims_neverUpscales(t *testing.T) {
	// Both dims already <= maxEdge → return original (no upscaling).
	w, h := ResizeDims(100, 80, 320)
	if w != 100 || h != 80 {
		t.Fatalf("must not upscale: want (100,80), got (%d,%d)", w, h)
	}
}

func TestResizeDims_squareExactEdge(t *testing.T) {
	// Square exactly at maxEdge → unchanged.
	w, h := ResizeDims(320, 320, 320)
	if w != 320 || h != 320 {
		t.Fatalf("square at edge: want (320,320), got (%d,%d)", w, h)
	}
}
