package inksvg

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractClipPaths(t *testing.T) {
	svgXML := `<svg xmlns="http://www.w3.org/2000/svg">
		<defs>
			<clipPath id="testClip">
				<circle cx="50" cy="50" r="25" />
			</clipPath>
		</defs>
		<rect width="100" height="100" />
	</svg>`

	clips := extractClipPaths([]byte(svgXML))
	if len(clips) != 1 {
		t.Fatalf("expected 1 clipPath, got %d", len(clips))
	}
	content, ok := clips["testClip"]
	if !ok {
		t.Fatalf("expected clip 'testClip' not found")
	}
	if len(content) == 0 {
		t.Errorf("clip content is empty")
	}
}

func TestExtractPathClipIDs(t *testing.T) {
	svgXML := `<svg xmlns="http://www.w3.org/2000/svg">
		<defs>
			<clipPath id="clip1"><rect width="50" height="50" /></clipPath>
		</defs>
		<rect id="r1" width="10" height="10" />
		<g clip-path="url(#clip1)">
			<rect id="r2" width="20" height="20" />
			<path id="p1" d="M 0 0 L 10 10" />
		</g>
		<circle id="c1" cx="5" cy="5" r="5" />
	</svg>`

	clipIDs := extractPathClipIDs([]byte(svgXML))
	// Expected shapes in order: r1 (no clip), r2 (clip1), p1 (clip1), c1 (no clip)
	expected := []string{"", "clip1", "clip1", ""}
	if len(clipIDs) != len(expected) {
		t.Fatalf("expected %d shapes, got %d", len(expected), len(clipIDs))
	}
	for i, exp := range expected {
		if clipIDs[i] != exp {
			t.Errorf("shape %d clipID mismatch: got %q, want %q", i, clipIDs[i], exp)
		}
	}
}

func TestApplyAlphaMask(t *testing.T) {
	dst := image.NewRGBA(image.Rect(0, 0, 2, 2))
	// Set dst to opaque red
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			dst.SetRGBA(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
		}
	}

	mask := image.NewRGBA(image.Rect(0, 0, 2, 2))
	// Top-left: opaque, Top-right: 0 alpha, Bottom-left: 128 alpha, Bottom-right: 0 alpha
	mask.SetRGBA(0, 0, color.RGBA{A: 255})
	mask.SetRGBA(1, 0, color.RGBA{A: 0})
	mask.SetRGBA(0, 1, color.RGBA{A: 128})
	mask.SetRGBA(1, 1, color.RGBA{A: 0})

	applyAlphaMask(dst, mask)

	// Check top-left (unchanged)
	if c := dst.RGBAAt(0, 0); c.R != 255 || c.A != 255 {
		t.Errorf("top-left mismatch: %v", c)
	}
	// Check top-right (zeroed)
	if c := dst.RGBAAt(1, 0); c.A != 0 || c.R != 0 {
		t.Errorf("top-right mismatch: %v", c)
	}
	// Check bottom-left (half alpha & red)
	if c := dst.RGBAAt(0, 1); c.A != 128 || c.R != 128 {
		t.Errorf("bottom-left mismatch: %v", c)
	}
}

func TestClipPathMaskRendering(t *testing.T) {
	svgPath := filepath.Join("..", "..", "testdata", "fixtures", "clip_path_mask.svg")
	goldenPath := filepath.Join("..", "..", "testdata", "fixtures", "clip_path_mask.golden.png")

	data, err := os.ReadFile(svgPath)
	if err != nil {
		t.Fatalf("failed to read svg: %v", err)
	}

	doc, err := ParseSVG(data)
	if err != nil {
		t.Fatalf("failed to parse svg: %v", err)
	}

	frameSVG, err := BuildLayerFrameSVG(doc, doc.Layers[0].ID, nil, doc.GetDocumentRect())
	if err != nil {
		t.Fatalf("failed to build frame: %v", err)
	}

	img, err := RenderSVGToRGBA(frameSVG, 128, 128)
	if err != nil {
		t.Fatalf("failed to render: %v", err)
	}

	opts := GoldenCompareOptions{
		PerPixelTolerance:  35,
		MaxMismatchPercent: 3.0,
	}
	AssertImageMatchesGolden(t, img, goldenPath, opts)
}
