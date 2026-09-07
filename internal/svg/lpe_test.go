package svg

import (
	"image"
	"image/png"
	"math"
	"os"
	"strings"
	"testing"
)

func TestLPEExtractPathEffects(t *testing.T) {
	svgData := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape" xmlns="http://www.w3.org/2000/svg">
  <defs>
    <inkscape:path-effect
       effect="fillet_chamfer"
       id="path-effect3"
       radius="42"
       unit="px"
       mode="F"
       chamfer_steps="1"
       use_knot_distance="true"
       nodesatellites_param="F,0,0,1,0,11.1125,0,1 @ F,0,0,1,0,11.1125,0,1" />
  </defs>
</svg>`)

	effects := extractPathEffects(svgData)
	if len(effects) != 1 {
		t.Fatalf("expected 1 effect, got %d", len(effects))
	}

	eff, ok := effects["path-effect3"]
	if !ok {
		t.Fatalf("path-effect3 not found")
	}
	if eff.Effect != "fillet_chamfer" {
		t.Errorf("expected effect fillet_chamfer, got %s", eff.Effect)
	}
	if eff.Radius != 42 {
		t.Errorf("expected radius 42, got %f", eff.Radius)
	}
	if eff.Unit != "px" {
		t.Errorf("expected unit px, got %s", eff.Unit)
	}
	if eff.Mode != "F" {
		t.Errorf("expected mode F, got %s", eff.Mode)
	}
	if !eff.UseKnot {
		t.Errorf("expected use_knot_distance true")
	}
}

func TestApplyFilletChamferToPath(t *testing.T) {
	// Sharp equilateral triangle: Apex at (50, 13.4), base corners at (0, 100) and (100, 100)
	// Base length 100, sides 100
	sharpTriangleD := "M 0 100 L 50 13.4 L 100 100 Z"

	eff := PathEffect{
		ID:     "effect1",
		Effect: "fillet_chamfer",
		Radius: 10.0,
		Mode:   "F",
	}

	filletedD := ApplyFilletChamferToPath(sharpTriangleD, eff)
	if !strings.Contains(filletedD, "A 10.000000 10.000000") {
		t.Fatalf("expected arc command with radius 10 in filleted path, got: %s", filletedD)
	}

	// Verify the original sharp apex (50, 13.4) was replaced with tangent setback and arc
	segs := parseSVGPathSegments(filletedD)
	var arcCount int
	for _, s := range segs {
		if s.Type == 'A' {
			arcCount++
			if s.Rx != 10.0 || s.Ry != 10.0 {
				t.Errorf("expected arc radius 10, got %f, %f", s.Rx, s.Ry)
			}
		}
	}
	if arcCount == 0 {
		t.Errorf("expected at least 1 filleted corner arc, got 0")
	}
}

func TestPaintOrderDesugar(t *testing.T) {
	svgData := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg">
  <path id="star1" d="M 10 10 L 90 10 L 50 90 Z"
        style="fill:#1f1f1f;stroke:#bb2222;stroke-width:20;paint-order:stroke fill markers" />
</svg>`)

	processed, err := PreprocessSVG(svgData)
	if err != nil {
		t.Fatalf("PreprocessSVG failed: %v", err)
	}

	procStr := string(processed)
	// Should have two paths now: one stroke-only, one fill-only
	if !strings.Contains(procStr, "id=\"star1_stroke\"") {
		t.Errorf("expected star1_stroke element in output, got: %s", procStr)
	}
	if !strings.Contains(procStr, "fill:none") {
		t.Errorf("expected fill:none on stroke element, got: %s", procStr)
	}
	if !strings.Contains(procStr, "stroke:none") {
		t.Errorf("expected stroke:none on fill element, got: %s", procStr)
	}

	// Stroke path should appear before fill path
	strokeIdx := strings.Index(procStr, "star1_stroke")
	fillIdx := strings.Index(procStr, "id=\"star1\"")
	if strokeIdx >= fillIdx {
		t.Errorf("expected stroke element before fill element in DOM")
	}
}

func TestRectRxRyNormalization(t *testing.T) {
	svgData := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg">
  <rect id="rect_ry_only" x="10" y="10" width="100" height="80" ry="15" />
  <rect id="rect_rx_only" x="10" y="10" width="100" height="80" rx="12" />
</svg>`)

	processed, err := PreprocessSVG(svgData)
	if err != nil {
		t.Fatalf("PreprocessSVG failed: %v", err)
	}

	procStr := string(processed)
	if !strings.Contains(procStr, `rx="15"`) {
		t.Errorf("expected rx to be synthesized from ry=15, got: %s", procStr)
	}
	if !strings.Contains(procStr, `ry="12"`) {
		t.Errorf("expected ry to be synthesized from rx=12, got: %s", procStr)
	}
}

func TestAlertIconGoldenMatch(t *testing.T) {
	data, err := os.ReadFile("../../testdata/Alert Icon.svg")
	if err != nil {
		t.Fatalf("failed to read Alert Icon.svg: %v", err)
	}

	doc, err := ParseSVG(data)
	if err != nil {
		t.Fatalf("failed to parse Alert Icon.svg: %v", err)
	}

	// Build layer1 frame at native 500x500 document bounds
	frameBytes, err := BuildLayerFrameSVG(doc, "layer1", nil, Rect{X: 0, Y: 0, Width: 500, Height: 500})
	if err != nil {
		t.Fatalf("BuildLayerFrameSVG failed: %v", err)
	}

	// Render at 521x521 matching Inkscape export DPI
	img, err := RenderSVGToRGBA(frameBytes, 521, 521)
	if err != nil {
		t.Fatalf("RenderSVGToRGBA failed: %v", err)
	}

	// Load golden image
	goldenFile, err := os.Open("../../testdata/Alert Icon-golden.png")
	if err != nil {
		t.Fatalf("failed to read Alert Icon-golden.png: %v", err)
	}
	defer func() {
		_ = goldenFile.Close()
	}()

	goldenImg, err := png.Decode(goldenFile)
	if err != nil {
		t.Fatalf("failed to decode golden png: %v", err)
	}

	// Helper color extractors
	isRed := func(c [4]uint8) bool {
		return c[0] > 150 && c[1] < 50 && c[2] < 50 && c[3] > 200
	}
	isBlack := func(c [4]uint8) bool {
		return c[0] < 50 && c[1] < 50 && c[2] < 50 && c[3] > 200
	}

	getBounds := func(img image.Image) (minRedX, maxRedX, minRedY, maxRedY, minBlackX, maxBlackX, minBlackY, maxBlackY int) {
		minRedX, minRedY = 9999, 9999
		minBlackX, minBlackY = 9999, 9999
		maxRedX, maxRedY = -1, -1
		maxBlackX, maxBlackY = -1, -1

		b := img.Bounds()
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				r, g, bl, a := img.At(x, y).RGBA()
				c := [4]uint8{uint8(r >> 8), uint8(g >> 8), uint8(bl >> 8), uint8(a >> 8)}
				if isRed(c) {
					if x < minRedX {
						minRedX = x
					}
					if x > maxRedX {
						maxRedX = x
					}
					if y < minRedY {
						minRedY = y
					}
					if y > maxRedY {
						maxRedY = y
					}
				}
				if isBlack(c) {
					if x < minBlackX {
						minBlackX = x
					}
					if x > maxBlackX {
						maxBlackX = x
					}
					if y < minBlackY {
						minBlackY = y
					}
					if y > maxBlackY {
						maxBlackY = y
					}
				}
			}
		}
		return
	}

	gMinRedX, gMaxRedX, gMinRedY, gMaxRedY, gMinBlackX, gMaxBlackX, gMinBlackY, gMaxBlackY := getBounds(goldenImg)
	rMinRedX, rMaxRedX, rMinRedY, rMaxRedY, rMinBlackX, rMaxBlackX, rMinBlackY, rMaxBlackY := getBounds(img)

	t.Logf("Golden Red bounds: X:[%d..%d] Y:[%d..%d], Black bounds: X:[%d..%d] Y:[%d..%d]",
		gMinRedX, gMaxRedX, gMinRedY, gMaxRedY, gMinBlackX, gMaxBlackX, gMinBlackY, gMaxBlackY)
	t.Logf("Render Red bounds: X:[%d..%d] Y:[%d..%d], Black bounds: X:[%d..%d] Y:[%d..%d]",
		rMinRedX, rMaxRedX, rMinRedY, rMaxRedY, rMinBlackX, rMaxBlackX, rMinBlackY, rMaxBlackY)

	// Verify black fill apex reaches up to ~62 (smooth dome) rather than cut off at ~116
	if rMinBlackY > 65 {
		t.Errorf("expected black apex minBlackY <= 65 (smooth dome), got %d", rMinBlackY)
	}
	if math.Abs(float64(rMinBlackY-gMinBlackY)) > 2 {
		t.Errorf("black apex min Y differs from golden by > 2px: got %d, golden %d", rMinBlackY, gMinBlackY)
	}
	if math.Abs(float64(rMaxBlackY-gMaxBlackY)) > 2 {
		t.Errorf("black base max Y differs from golden by > 2px: got %d, golden %d", rMaxBlackY, gMaxBlackY)
	}

	// Verify outer red stroke bounds match golden within 2px
	if math.Abs(float64(rMinRedX-gMinRedX)) > 2 || math.Abs(float64(rMaxRedX-gMaxRedX)) > 2 {
		t.Errorf("red stroke X bounds mismatch golden: got [%d..%d], golden [%d..%d]", rMinRedX, rMaxRedX, gMinRedX, gMaxRedX)
	}
	if math.Abs(float64(rMinRedY-gMinRedY)) > 2 || math.Abs(float64(rMaxRedY-gMaxRedY)) > 2 {
		t.Errorf("red stroke Y bounds mismatch golden: got [%d..%d], golden [%d..%d]", rMinRedY, rMaxRedY, gMinRedY, gMaxRedY)
	}
}

func TestTransformGroupStrokeWidthScaling(t *testing.T) {
	testSVG := `<svg viewBox="0 0 100 100" xmlns="http://www.w3.org/2000/svg">
		<g transform="matrix(2,0,0,2,10,20)">
			<rect style="fill:#000;stroke:#f00;stroke-width:5px" width="20" height="20" />
			<path stroke-width="10" stroke="#0f0" d="M 0,0 L 10,10" />
		</g>
	</svg>`

	processed, err := PreprocessSVG([]byte(testSVG))
	if err != nil {
		t.Fatalf("PreprocessSVG failed: %v", err)
	}

	str := string(processed)
	// The 5px stroke-width inside scale(2) group should be scaled to 10.0000px
	if !strings.Contains(str, "10.0000px") {
		t.Errorf("expected rect stroke-width scaled to 10.0000px, got:\n%s", str)
	}
	// The 10 stroke-width attribute inside scale(2) group should be scaled to 20.0000
	if !strings.Contains(str, "20.0000") {
		t.Errorf("expected path stroke-width scaled to 20.0000, got:\n%s", str)
	}
}

func TestAlertIconMiddleBoxesMatch(t *testing.T) {
	data, err := os.ReadFile("../../testdata/Alert Icon.svg")
	if err != nil {
		t.Fatalf("failed to read Alert Icon.svg: %v", err)
	}

	doc, err := ParseSVG(data)
	if err != nil {
		t.Fatalf("failed to parse Alert Icon.svg: %v", err)
	}

	frameBytes, err := BuildLayerFrameSVG(doc, "layer1", nil, Rect{})
	if err != nil {
		t.Fatalf("BuildLayerFrameSVG failed: %v", err)
	}

	img, err := RenderSVGToRGBA(frameBytes, 521, 521)
	if err != nil {
		t.Fatalf("RenderSVGToRGBA failed: %v", err)
	}

	// At x=250, verify the outer box top stroke starts at y=247 and ends at y=257 (11 pixels)
	var outerStrokeCount int
	for y := 247; y <= 257; y++ {
		r, g, b, a := img.At(250, y).RGBA()
		r8, g8, b8, a8 := uint8(r>>8), uint8(g>>8), uint8(b>>8), uint8(a>>8)
		if r8 > 50 && g8 < 40 && b8 < 40 && a8 > 200 {
			outerStrokeCount++
		}
	}
	if outerStrokeCount < 10 {
		t.Errorf("expected outer box stroke to span >= 10 pixels around y=247..257, got %d", outerStrokeCount)
	}

	// At x=250, verify the inner box top stroke starts at y=267 and ends at y=276 (10 pixels)
	var innerStrokeCount int
	for y := 267; y <= 276; y++ {
		r, g, b, a := img.At(250, y).RGBA()
		r8, g8, b8, a8 := uint8(r>>8), uint8(g>>8), uint8(b>>8), uint8(a>>8)
		if r8 > 50 && g8 < 40 && b8 < 40 && a8 > 200 {
			innerStrokeCount++
		}
	}
	if innerStrokeCount < 9 {
		t.Errorf("expected inner box stroke to span >= 9 pixels around y=267..276, got %d", innerStrokeCount)
	}

	// At x=260, verify the diagonal slash spans from y=316 to y=339 (24 pixels)
	var slashCount int
	for y := 316; y <= 339; y++ {
		r, g, b, a := img.At(260, y).RGBA()
		r8, g8, b8, a8 := uint8(r>>8), uint8(g>>8), uint8(b>>8), uint8(a>>8)
		if r8 > 50 && g8 < 40 && b8 < 40 && a8 > 200 {
			slashCount++
		}
	}
	if slashCount < 22 {
		t.Errorf("expected diagonal slash to span >= 22 pixels around y=316..339, got %d", slashCount)
	}
}

