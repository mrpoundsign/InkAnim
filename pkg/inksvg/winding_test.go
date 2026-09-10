package inksvg

import (
	"strings"
	"testing"
)

func TestNormalizeEvenOddPath_ConcentricSquares(t *testing.T) {
	// Both outer and inner squares are defined clockwise
	d := "M 0 0 L 100 0 L 100 100 L 0 100 Z M 20 20 L 80 20 L 80 80 L 20 80 Z"

	normalized := NormalizeEvenOddPath(d)

	// Outer square should remain in original direction
	if !strings.HasPrefix(normalized, "M 0 0 L 100 0") {
		t.Errorf("expected outer square to retain start coordinates, got: %s", normalized)
	}

	// Inner square should have been reversed from (20,20)->(80,20) to (20,20)->(20,80)
	if !strings.Contains(normalized, "M 20 80") && !strings.Contains(normalized, "L 20 80") {
		t.Errorf("expected inner square to be reversed, got: %s", normalized)
	}
}

func TestNormalizeEvenOddPath_SingleSubpathNoop(t *testing.T) {
	d := "M 10 10 L 50 10 L 50 50 Z"
	normalized := NormalizeEvenOddPath(d)
	if normalized != d {
		t.Errorf("expected single subpath to remain unmodified, got: %s", normalized)
	}
}

func TestReverseSubpath_CubicAndQuadCurves(t *testing.T) {
	d := "M 10 10 C 20 5 40 5 50 10 Q 70 30 50 50 Z"
	subpaths := ParseSubpaths(d)
	if len(subpaths) != 1 {
		t.Fatalf("expected 1 subpath, got %d", len(subpaths))
	}

	reversed := ReverseSubpath(subpaths[0])
	serialized := SerializeSubpaths([]Subpath{reversed})

	// Reversed path should start at (10, 10) because it closed at start
	if !strings.HasPrefix(serialized, "M 10 10") {
		t.Errorf("expected reversed path to start at 10 10, got: %s", serialized)
	}
	if !strings.HasSuffix(serialized, "Z") {
		t.Errorf("expected reversed path to close with Z, got: %s", serialized)
	}
}
