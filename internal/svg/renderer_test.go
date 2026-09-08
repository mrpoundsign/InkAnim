package svg

import (
	"image/png"
	"math"
	"os"
	"testing"
)

func TestRenderSVGToRGBAStrokeResolutionScaling(t *testing.T) {
	testSVG := `<svg viewBox="0 0 100 100" xmlns="http://www.w3.org/2000/svg">
		<path d="M 10,50 L 90,50" stroke="#000" stroke-width="10" fill="none" />
	</svg>`

	// Render at 100x100 (scaleFactor = 1.0)
	img100, err := RenderSVGToRGBA([]byte(testSVG), 100, 100)
	if err != nil {
		t.Fatalf("RenderSVGToRGBA 100x100 failed: %v", err)
	}

	// Count vertical stroke thickness at x=50
	var count100 int
	for y := range 100 {
		_, _, _, a := img100.At(50, y).RGBA()
		if a > 1000 {
			count100++
		}
	}

	// Render at 200x200 (scaleFactor = 2.0)
	img200, err := RenderSVGToRGBA([]byte(testSVG), 200, 200)
	if err != nil {
		t.Fatalf("RenderSVGToRGBA 200x200 failed: %v", err)
	}

	var count200 int
	for y := range 200 {
		_, _, _, a := img200.At(100, y).RGBA()
		if a > 1000 {
			count200++
		}
	}

	t.Logf("Stroke thickness at 100x100: %d px, at 200x200: %d px", count100, count200)
	if count100 < 9 || count100 > 11 {
		t.Errorf("expected ~10px stroke at 100x100, got %d", count100)
	}
	if count200 < 19 || count200 > 21 {
		t.Errorf("expected ~20px stroke at 200x200 (scaled by 2x), got %d", count200)
	}
}

func TestHydrateGoldenStrokeMatch(t *testing.T) {
	svgBytes, err := os.ReadFile("../../testdata/hydrate.svg")
	if err != nil {
		t.Fatalf("failed to read hydrate.svg: %v", err)
	}

	doc, err := ParseSVG(svgBytes)
	if err != nil {
		t.Fatalf("failed to parse hydrate.svg: %v", err)
	}

	frame, err := BuildLayerFrameSVG(doc, "layer1", nil, doc.GetDocumentRect())
	if err != nil {
		t.Fatalf("failed to build frame: %v", err)
	}

	// Render at 794x1123 matching hydrate-golden.png resolution
	img, err := RenderSVGToRGBA(frame, 794, 1123)
	if err != nil {
		t.Fatalf("failed to render frame: %v", err)
	}

	// Load golden image
	fGold, err := os.Open("../../testdata/hydrate-golden.png")
	if err != nil {
		t.Fatalf("failed to open hydrate-golden.png: %v", err)
	}
	defer func() { _ = fGold.Close() }()

	imgGold, err := png.Decode(fGold)
	if err != nil {
		t.Fatalf("failed to decode hydrate-golden.png: %v", err)
	}

	// Verify stroke thickness on the right droplet around x=555, y=992..1019
	centerX := 794 * 7 / 10
	var strokeCountRender, strokeCountGolden int
	for y := 985; y <= 1025; y++ {
		// Rendered image
		_, _, blR, aR := img.At(centerX, y).RGBA()
		blR8, aR8 := uint8(blR>>8), uint8(aR>>8)
		if blR8 > 80 && blR8 < 160 && aR8 > 200 {
			strokeCountRender++
		}

		// Golden image
		_, _, blG, aG := imgGold.At(centerX, y).RGBA()
		blG8, aG8 := uint8(blG>>8), uint8(aG>>8)
		if blG8 > 80 && blG8 < 160 && aG8 > 200 {
			strokeCountGolden++
		}
	}

	t.Logf("Stroke thickness at x=%d: Rendered=%d, Golden=%d", centerX, strokeCountRender, strokeCountGolden)
	if strokeCountRender < 25 {
		t.Errorf("expected rendered stroke >= 25 px, got %d", strokeCountRender)
	}
	if math.Abs(float64(strokeCountRender-strokeCountGolden)) > 2 {
		t.Errorf("rendered stroke thickness differs from golden: got %d, golden %d", strokeCountRender, strokeCountGolden)
	}
}

func TestParsePreserveAspectRatio(t *testing.T) {
	tests := []struct {
		input       string
		expected    PreserveAspectRatio
	}{
		{"", PreserveAspectRatio{Align: "xMidYMid", MeetOrSlice: "meet"}},
		{"none", PreserveAspectRatio{Align: "none", MeetOrSlice: "meet"}},
		{"xMinYMin meet", PreserveAspectRatio{Align: "xMinYMin", MeetOrSlice: "meet"}},
		{"xMaxYMax slice", PreserveAspectRatio{Align: "xMaxYMax", MeetOrSlice: "slice"}},
		{"defer xMidYMid slice", PreserveAspectRatio{Align: "xMidYMid", MeetOrSlice: "slice"}},
		{"  xMinYMid   meet  ", PreserveAspectRatio{Align: "xMinYMid", MeetOrSlice: "meet"}},
	}

	for _, tc := range tests {
		got := parsePreserveAspectRatio(tc.input)
		if got != tc.expected {
			t.Errorf("parsePreserveAspectRatio(%q) = %+v; want %+v", tc.input, got, tc.expected)
		}
	}
}

func TestRenderPreserveAspectRatioModes(t *testing.T) {
	// 200x100 rectangle filled with red, rendered into 100x100 square
	svgMeet := `<svg xmlns="http://www.w3.org/2000/svg" width="100" height="100" viewBox="0 0 200 100" preserveAspectRatio="xMidYMid meet">
		<rect width="200" height="100" fill="#ff0000" />
	</svg>`

	imgMeet, err := RenderSVGToRGBA([]byte(svgMeet), 100, 100)
	if err != nil {
		t.Fatalf("RenderSVGToRGBA meet failed: %v", err)
	}

	// In meet mode, 200x100 scaled to 100 wide -> height is 50.
	// Centered: y=0..24 is transparent, y=25..74 is red, y=75..99 is transparent.
	_, _, _, alphaTop := imgMeet.At(50, 10).RGBA()
	if alphaTop != 0 {
		t.Errorf("expected transparent top pillarbox in meet mode, got alpha %d", alphaTop)
	}
	rCenter, _, _, alphaCenter := imgMeet.At(50, 50).RGBA()
	if alphaCenter == 0 || (rCenter>>8) < 200 {
		t.Errorf("expected red content at center in meet mode, got RGBA (%d, _, _, %d)", rCenter>>8, alphaCenter)
	}

	// In none mode, 200x100 stretches to fill entire 100x100
	svgNone := `<svg xmlns="http://www.w3.org/2000/svg" width="100" height="100" viewBox="0 0 200 100" preserveAspectRatio="none">
		<rect width="200" height="100" fill="#ff0000" />
	</svg>`

	imgNone, err := RenderSVGToRGBA([]byte(svgNone), 100, 100)
	if err != nil {
		t.Fatalf("RenderSVGToRGBA none failed: %v", err)
	}
	rNone, _, _, aNone := imgNone.At(50, 10).RGBA()
	if aNone == 0 || (rNone>>8) < 200 {
		t.Errorf("expected filled content across top in none mode, got RGBA (%d, _, _, %d)", rNone>>8, aNone)
	}
}

