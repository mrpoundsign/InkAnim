package svg

import (
	"bytes"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/srwiley/oksvg"
)

const sampleInkscapeSVG = `<?xml version="1.0" encoding="UTF-8" standalone="no"?>
<svg
   width="200"
   height="100"
   viewBox="0 0 200 100"
   version="1.1"
   id="svg5"
   xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape"
   xmlns:sodipodi="http://sodipodi.sourceforge.net/DTD/sodipodi-0.dtd"
   xmlns="http://www.w3.org/2000/svg">
  <sodipodi:namedview id="namedview7">
    <inkscape:page x="0" y="0" width="100" height="100" id="page1" inkscape:label="Page 1" />
    <inkscape:page x="100" y="0" width="100" height="100" id="page2" inkscape:label="Page 2" />
  </sodipodi:namedview>
  <g
     inkscape:groupmode="layer"
     id="layer_bg"
     inkscape:label="Background"
     style="display:inline">
    <rect width="200" height="100" fill="#202020" id="rect_bg" />
  </g>
  <g
     inkscape:groupmode="layer"
     id="layer_frame1"
     inkscape:label="Frame 1"
     style="display:inline">
    <circle cx="50" cy="50" r="30" fill="#ff0000" id="circle1" />
  </g>
  <g
     inkscape:groupmode="layer"
     id="layer_frame2"
     inkscape:label="Frame 2"
     style="display:none">
    <circle cx="50" cy="50" r="30" fill="#00ff00" id="circle2" />
  </g>
</svg>`

func TestParseSVG(t *testing.T) {
	doc, err := ParseSVG([]byte(sampleInkscapeSVG))
	if err != nil {
		t.Fatalf("ParseSVG failed: %v", err)
	}

	if doc.Width != 200 || doc.Height != 100 {
		t.Errorf("expected 200x100, got %fx%f", doc.Width, doc.Height)
	}

	if len(doc.Layers) != 3 {
		t.Fatalf("expected 3 layers, got %d", len(doc.Layers))
	}
	if doc.Layers[0].ID != "layer_bg" || doc.Layers[0].Label != "Background" {
		t.Errorf("unexpected layer 0: %+v", doc.Layers[0])
	}
	if doc.Layers[1].ID != "layer_frame1" || doc.Layers[1].Label != "Frame 1" {
		t.Errorf("unexpected layer 1: %+v", doc.Layers[1])
	}
	if doc.Layers[2].ID != "layer_frame2" || doc.Layers[2].Label != "Frame 2" {
		t.Errorf("unexpected layer 2: %+v", doc.Layers[2])
	}

	if len(doc.Pages) != 2 {
		t.Fatalf("expected 2 pages, got %d", len(doc.Pages))
	}
	if doc.Pages[0].Width != 100 || doc.Pages[1].X != 100 {
		t.Errorf("unexpected pages: %+v, %+v", doc.Pages[0], doc.Pages[1])
	}
}

func TestBuildLayerFrameSVG(t *testing.T) {
	doc, err := ParseSVG([]byte(sampleInkscapeSVG))
	if err != nil {
		t.Fatalf("ParseSVG failed: %v", err)
	}

	// Make frame 2 active, pin background
	pinned := map[string]bool{"layer_bg": true}
	frame2Bytes, err := BuildLayerFrameSVG(doc, "layer_frame2", pinned, Rect{})
	if err != nil {
		t.Fatalf("BuildLayerFrameSVG failed: %v", err)
	}

	frame2Str := string(frame2Bytes)
	if !strings.Contains(frame2Str, `id="layer_frame2"`) {
		t.Errorf("expected layer_frame2 in output")
	}
	if !strings.Contains(frame2Str, "display:inline") {
		t.Errorf("expected display:inline in output")
	}

	// Render frame 2 to RGBA
	img, err := RenderSVGToRGBA(frame2Bytes, 100, 100)
	if err != nil {
		t.Fatalf("RenderSVGToRGBA failed: %v", err)
	}
	if img.Bounds().Dx() != 100 || img.Bounds().Dy() != 100 {
		t.Errorf("expected 100x100, got %dx%d", img.Bounds().Dx(), img.Bounds().Dy())
	}

	// Also build frame 1 (layer_frame1 should show red circle, layer_frame2 green circle should NOT be visible)
	frame1Bytes, err := BuildLayerFrameSVG(doc, "layer_frame1", pinned, Rect{})
	if err != nil {
		t.Fatalf("BuildLayerFrameSVG frame 1 failed: %v", err)
	}
	img1, err := RenderSVGToRGBA(frame1Bytes, 100, 100)
	if err != nil {
		t.Fatalf("RenderSVGToRGBA frame 1 failed: %v", err)
	}
	c1 := img1.RGBAAt(50, 50)
	t.Logf("Frame 1 pixel at (50, 50): R=%d, G=%d, B=%d, A=%d", c1.R, c1.G, c1.B, c1.A)
}

func TestOksvgRenderDirect(t *testing.T) {
	testSVG := `<svg width="100" height="100" viewBox="0 0 100 100" xmlns="http://www.w3.org/2000/svg">
		<circle cx="50" cy="50" r="30" fill="#ff0000" />
	</svg>`

	img, err := RenderSVGToRGBA([]byte(testSVG), 100, 100)
	if err != nil {
		t.Fatalf("RenderSVGToRGBA error: %v", err)
	}

	c := img.RGBAAt(50, 50)
	t.Logf("Direct render pixel at (50, 50): R=%d, G=%d, B=%d, A=%d", c.R, c.G, c.B, c.A)
}

func TestBuildPageFrameSVG(t *testing.T) {
	doc, err := ParseSVG([]byte(sampleInkscapeSVG))
	if err != nil {
		t.Fatalf("ParseSVG failed: %v", err)
	}

	page2Bytes, err := BuildPageFrameSVG(doc, doc.Pages[1], Rect{})
	if err != nil {
		t.Fatalf("BuildPageFrameSVG failed: %v", err)
	}

	page2Str := string(page2Bytes)
	if !strings.Contains(page2Str, `viewBox="100.000000 0.000000 100.000000 100.000000"`) {
		t.Errorf("unexpected viewBox in page frame: %s", page2Str)
	}
}

func TestBuildLayerFrameSVG_PinnedBackgroundStackingOrder(t *testing.T) {
	// An SVG where the background layer is positioned AFTER the animation frame layer in XML
	svgData := `<?xml version="1.0" encoding="UTF-8"?>
<svg width="100" height="100" viewBox="0 0 100 100" xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape">
  <!-- Animation layer defined first -->
  <g inkscape:groupmode="layer" id="layer_walk1" inkscape:label="Walk 1">
    <rect x="25" y="25" width="50" height="50" fill="#ff0000" />
  </g>
  <!-- Other animation layer -->
  <g inkscape:groupmode="layer" id="layer_walk2" inkscape:label="Walk 2">
    <rect x="25" y="25" width="50" height="50" fill="#00ff00" />
  </g>
  <!-- Background layer defined AFTER animation layers in source XML -->
  <g inkscape:groupmode="layer" id="layer_bg" inkscape:label="Background">
    <rect x="0" y="0" width="100" height="100" fill="#0000ff" />
  </g>
</svg>`

	doc, err := ParseSVG([]byte(svgData))
	if err != nil {
		t.Fatalf("ParseSVG failed: %v", err)
	}

	pinned := map[string]bool{"layer_bg": true}
	frameBytes, err := BuildLayerFrameSVG(doc, "layer_walk1", pinned, Rect{})
	if err != nil {
		t.Fatalf("BuildLayerFrameSVG failed: %v", err)
	}

	frameStr := string(frameBytes)

	// 1. Structural check: layer_bg MUST appear before layer_walk1 in the resulting XML
	bgIdx := strings.Index(frameStr, `id="layer_bg"`)
	walkIdx := strings.Index(frameStr, `id="layer_walk1"`)

	if bgIdx == -1 {
		t.Fatalf("expected layer_bg in generated SVG")
	}
	if walkIdx == -1 {
		t.Fatalf("expected layer_walk1 in generated SVG")
	}
	if bgIdx >= walkIdx {
		t.Fatalf("expected pinned background layer (idx %d) to appear BEFORE target animation layer (idx %d)", bgIdx, walkIdx)
	}

	// 2. Visual rendering check: The red rectangle at center (50, 50) must be drawn on TOP of the blue background
	img, err := RenderSVGToRGBA(frameBytes, 100, 100)
	if err != nil {
		t.Fatalf("RenderSVGToRGBA failed: %v", err)
	}

	centerPixel := img.RGBAAt(50, 50)
	// Must be red (#ff0000), not blue (#0000ff)
	if centerPixel.R < 200 || centerPixel.B > 50 {
		t.Errorf("center pixel was obscured by background! Expected red, got R=%d G=%d B=%d A=%d",
			centerPixel.R, centerPixel.G, centerPixel.B, centerPixel.A)
	}

	// Corner pixel (10, 10) must be blue background (#0000ff)
	cornerPixel := img.RGBAAt(10, 10)
	if cornerPixel.B < 200 || cornerPixel.R > 50 {
		t.Errorf("corner pixel expected blue background, got R=%d G=%d B=%d A=%d",
			cornerPixel.R, cornerPixel.G, cornerPixel.B, cornerPixel.A)
	}
}

func TestCharacterWalkPinned(t *testing.T) {
	data, err := os.ReadFile("../../testdata/character_walk.svg")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := ParseSVG(data)
	if err != nil {
		t.Fatal(err)
	}
	pinned := map[string]bool{"layer_frame1": true}
	f2, err := BuildLayerFrameSVG(doc, "layer_frame2", pinned, Rect{})
	if err != nil {
		t.Fatal(err)
	}
	f3, err := BuildLayerFrameSVG(doc, "layer_frame3", pinned, Rect{})
	if err != nil {
		t.Fatal(err)
	}
	img2, err := RenderSVGToRGBA(f2, 256, 256)
	if err != nil {
		t.Fatal(err)
	}
	img3, err := RenderSVGToRGBA(f3, 256, 256)
	if err != nil {
		t.Fatal(err)
	}
	diff := 0
	for y := range 256 {
		for x := range 256 {
			if img2.RGBAAt(x, y) != img3.RGBAAt(x, y) {
				diff++
			}
		}
	}
	t.Logf("Diff between f2 and f3 with f1 pinned: %d", diff)
	if diff == 0 {
		t.Errorf("expected frames 2 and 3 to be different with f1 pinned, but they are identical!")
	}
}

func TestBoundaryCroppingAndClipping(t *testing.T) {
	// 200x100 SVG with a circle at (50, 50) and a circle at (150, 50)
	svgData := `<?xml version="1.0" encoding="UTF-8"?>
<svg width="200" height="100" viewBox="0 0 200 100" xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape">
  <inkscape:page x="0" y="0" width="100" height="100" id="page1" inkscape:label="Page Left" />
  <inkscape:page x="100" y="0" width="100" height="100" id="page2" inkscape:label="Page Right" />
  <g inkscape:groupmode="layer" id="layer_action" inkscape:label="Action">
    <!-- Circle centered at (50, 50), r=40 (within page 1) -->
    <circle cx="50" cy="50" r="40" fill="#ff0000" />
    <!-- Circle centered at (150, 50), r=40 (within page 2) -->
    <circle cx="150" cy="50" r="40" fill="#0000ff" />
  </g>
</svg>`

	doc, err := ParseSVG([]byte(svgData))
	if err != nil {
		t.Fatalf("ParseSVG error: %v", err)
	}

	// 1. Crop to Page 1: boundary = (0, 0, 100, 100)
	page1Rect, ok := doc.GetPageRect(0)
	if !ok || page1Rect.Width != 100 {
		t.Fatalf("expected page 1 rect of 100x100, got %+v", page1Rect)
	}
	fPage1, err := BuildLayerFrameSVG(doc, "layer_action", nil, page1Rect)
	if err != nil {
		t.Fatalf("BuildLayerFrameSVG error: %v", err)
	}

	imgPage1, err := RenderSVGToRGBA(fPage1, 100, 100)
	if err != nil {
		t.Fatalf("RenderSVGToRGBA error: %v", err)
	}

	// Center of page 1 (50, 50) must be red
	pCenter1 := imgPage1.RGBAAt(50, 50)
	if pCenter1.R < 200 || pCenter1.B > 50 {
		t.Errorf("Page 1 center pixel expected red, got R=%d B=%d", pCenter1.R, pCenter1.B)
	}

	// 2. Crop to Page 2: boundary = (100, 0, 100, 100)
	page2Rect, ok := doc.GetPageRect(1)
	if !ok || page2Rect.X != 100 {
		t.Fatalf("expected page 2 rect with X=100, got %+v", page2Rect)
	}
	fPage2, err := BuildLayerFrameSVG(doc, "layer_action", nil, page2Rect)
	if err != nil {
		t.Fatalf("BuildLayerFrameSVG error: %v", err)
	}

	imgPage2, err := RenderSVGToRGBA(fPage2, 100, 100)
	if err != nil {
		t.Fatalf("RenderSVGToRGBA error: %v", err)
	}

	// In page 2 viewport (100..200 maps to 0..100), (150, 50) maps to (50, 50) in the raster image
	// and should be blue (#0000ff), not red
	pCenter2 := imgPage2.RGBAAt(50, 50)
	if pCenter2.B < 200 || pCenter2.R > 50 {
		t.Errorf("Page 2 center pixel expected blue, got R=%d B=%d", pCenter2.R, pCenter2.B)
	}

	// 3. Document Boundary: 200x100
	docRect := doc.GetDocumentRect()
	fDoc, err := BuildLayerFrameSVG(doc, "layer_action", nil, docRect)
	if err != nil {
		t.Fatalf("BuildLayerFrameSVG error: %v", err)
	}
	imgDoc, err := RenderSVGToRGBA(fDoc, 200, 100)
	if err != nil {
		t.Fatalf("RenderSVGToRGBA error: %v", err)
	}
	if imgDoc.Bounds().Dx() != 200 || imgDoc.Bounds().Dy() != 100 {
		t.Errorf("expected 200x100 document image, got %dx%d", imgDoc.Bounds().Dx(), imgDoc.Bounds().Dy())
	}
	// At (50, 50) is red, at (150, 50) is blue
	if imgDoc.RGBAAt(50, 50).R < 200 {
		t.Errorf("expected red at (50, 50)")
	}
	if imgDoc.RGBAAt(150, 50).B < 200 {
		t.Errorf("expected blue at (150, 50)")
	}
}

func TestBouncingWalkerSVGLoadAndCrop(t *testing.T) {
	data, err := os.ReadFile("../../testdata/bouncing_walker.svg")
	if err != nil {
		t.Fatalf("failed to read bouncing_walker.svg: %v", err)
	}

	doc, err := ParseSVG(data)
	if err != nil {
		t.Fatalf("failed to parse bouncing_walker.svg: %v", err)
	}

	if len(doc.Layers) != 7 {
		t.Errorf("expected 7 layers (1 bg + 6 frames), got %d", len(doc.Layers))
	}
	if len(doc.Pages) != 2 {
		t.Errorf("expected 2 pages, got %d", len(doc.Pages))
	}

	// Test Frame 1 (cx=20, rx=48, extending to x = -28)
	pinned := map[string]bool{"layer_bg_grid": true}
	docRect := doc.GetDocumentRect()
	f1Doc, err := BuildLayerFrameSVG(doc, "frame1_left_wall", pinned, docRect)
	if err != nil {
		t.Fatalf("BuildLayerFrameSVG f1 error: %v", err)
	}

	imgF1, err := RenderSVGToRGBA(f1Doc, 256, 256)
	if err != nil {
		t.Fatalf("RenderSVGToRGBA f1 error: %v", err)
	}

	// Leftmost pixel inside image bounds (0, 170) should be colored (purple slime)
	pLeftEdge := imgF1.RGBAAt(0, 170)
	if pLeftEdge.A == 0 {
		t.Errorf("expected slime to touch left edge (x=0) where cx=20, rx=48, but got transparent pixel")
	}

	// Test Frame 5 (cx=236, rx=48, extending to x = 284)
	f5Doc, err := BuildLayerFrameSVG(doc, "frame5_right_wall", pinned, docRect)
	if err != nil {
		t.Fatalf("BuildLayerFrameSVG f5 error: %v", err)
	}

	imgF5, err := RenderSVGToRGBA(f5Doc, 256, 256)
	if err != nil {
		t.Fatalf("RenderSVGToRGBA f5 error: %v", err)
	}

	// Rightmost pixel inside image bounds (255, 170) should be colored (purple slime)
	pRightEdge := imgF5.RGBAAt(255, 170)
	if pRightEdge.A == 0 {
		t.Errorf("expected slime to touch right edge (x=255) where cx=236, rx=48, but got transparent pixel")
	}

	// Test Center Focus Crop (Page 2: 160x160)
	page2Rect, ok := doc.GetPageRect(1)
	if !ok || page2Rect.Width != 160 {
		t.Fatalf("expected page 2 rect 160x160, got %+v", page2Rect)
	}

	f3Page2, err := BuildLayerFrameSVG(doc, "frame3_apex", pinned, page2Rect)
	if err != nil {
		t.Fatalf("BuildLayerFrameSVG f3 error: %v", err)
	}

	imgF3, err := RenderSVGToRGBA(f3Page2, 160, 160)
	if err != nil {
		t.Fatalf("RenderSVGToRGBA f3 error: %v", err)
	}
	if imgF3.Bounds().Dx() != 160 || imgF3.Bounds().Dy() != 160 {
		t.Errorf("expected 160x160 cropped image, got %dx%d", imgF3.Bounds().Dx(), imgF3.Bounds().Dy())
	}

	// Test Drawing Rect (unclipped bounding box encompassing ground line from -50 to 306, plus 3px stroke margin)
	drawingRect := doc.GetDrawingRect()
	if drawingRect.X > -50 || drawingRect.X < -53 {
		t.Errorf("expected drawing min X around -51.5 (including stroke), got %f", drawingRect.X)
	}
	if drawingRect.Width < 350 {
		t.Errorf("expected drawing width >= 350, got %f", drawingRect.Width)
	}
}

func TestHydrateDrawingStrokeWidth(t *testing.T) {
	data, err := os.ReadFile("../../testdata/hydrate.svg")
	if err != nil {
		t.Fatalf("failed to read hydrate.svg: %v", err)
	}

	doc, err := ParseSVG(data)
	if err != nil {
		t.Fatalf("failed to parse hydrate.svg: %v", err)
	}

	icon, err := oksvg.ReadIconStream(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("ReadIconStream error: %v", err)
	}
	t.Logf("oksvg icon.ViewBox: %+v", icon.ViewBox)
	t.Logf("oksvg icon.Transform: %+v", icon.Transform)

	// DrawingRect should encompass the 15px stroke (7.5px margin on each side)
	drawingRect := doc.GetDrawingRect()
	// Node max Y is ~263.1, so with 7.5px stroke margin, max Y should reach >= 270.0
	maxY := drawingRect.Y + drawingRect.Height
	if maxY < 270.0 {
		t.Errorf("expected drawing maxY to reach at least 270.0 to encompass 15px stroke, got %f", maxY)
	}
	// Node min Y is ~23.4, so with angled 15px stroke, min Y should reach <= 18.0
	if drawingRect.Y > 18.0 {
		t.Errorf("expected drawing minY <= 18.0 to encompass 15px stroke, got %f", drawingRect.Y)
	}

	fBytes, err := BuildLayerFrameSVG(doc, "g6", nil, drawingRect)
	if err != nil {
		t.Fatalf("BuildLayerFrameSVG failed: %v", err)
	}

	// Render through RenderSVGToRGBA with proportional dimensions
	renderW := int(math.Round(drawingRect.Width * 512.0 / drawingRect.Height))
	img, err := RenderSVGToRGBA(fBytes, renderW, 512)
	if err != nil {
		t.Fatalf("RenderSVGToRGBA failed: %v", err)
	}

	b := img.Bounds()
	var bottomRowFill int
	for x := b.Min.X; x < b.Max.X; x++ {
		c := img.RGBAAt(x, b.Max.Y-1)
		if c.B > 200 && c.R < 50 && c.G < 50 {
			bottomRowFill++
		}
	}
	if bottomRowFill > 0 {
		t.Errorf("expected 0 fill pixels on bottom row (stroke encompasses fill without clipping), got %d", bottomRowFill)
	}

	var topRowColored int
	for x := b.Min.X; x < b.Max.X; x++ {
		if img.RGBAAt(x, b.Min.Y).A > 0 {
			topRowColored++
		}
	}
	if topRowColored > 0 {
		t.Errorf("expected 0 colored pixels on top row (no clipping), got %d", topRowColored)
	}
}

func TestAlertIconDrawingBounds(t *testing.T) {
	data, err := os.ReadFile("../../testdata/Alert Icon.svg")
	if err != nil {
		t.Fatalf("failed to read Alert Icon.svg: %v", err)
	}

	doc, err := ParseSVG(data)
	if err != nil {
		t.Fatalf("failed to parse Alert Icon.svg: %v", err)
	}

	drawingRect := doc.GetDrawingRect()
	t.Logf("Alert Icon drawingRect: %+v", drawingRect)

	// In Alert Icon.svg, the alert triangle (with 60px stroke and bevel joins) and exclamation marks
	// span approximately [3.9 .. 496.1] horizontally and [29.0 .. 464.9] vertically after transforms.
	// DrawingRect must not have negative X or Y, and should span approximately 492x436.
	if drawingRect.X < 0 || drawingRect.X > 10 {
		t.Errorf("expected drawingRect.X between 0 and 10, got %f", drawingRect.X)
	}
	if drawingRect.Y < 20 || drawingRect.Y > 35 {
		t.Errorf("expected drawingRect.Y between 20 and 35, got %f", drawingRect.Y)
	}
	if drawingRect.Width < 480 || drawingRect.Width > 500 {
		t.Errorf("expected drawingRect.Width between 480 and 500, got %f", drawingRect.Width)
	}
	if drawingRect.Height < 425 || drawingRect.Height > 445 {
		t.Errorf("expected drawingRect.Height between 425 and 445, got %f", drawingRect.Height)
	}

	// Build frame for layer1 and render to verify no flat clipping
	fBytes, err := BuildLayerFrameSVG(doc, "layer1", nil, drawingRect)
	if err != nil {
		t.Fatalf("BuildLayerFrameSVG failed: %v", err)
	}

	img, err := RenderSVGToRGBA(fBytes, 512, 512)
	if err != nil {
		t.Fatalf("RenderSVGToRGBA failed: %v", err)
	}

	b := img.Bounds()
	// Check bottom row to ensure no flat clipping against the edge
	var bottomRowColored int
	for x := b.Min.X; x < b.Max.X; x++ {
		if img.RGBAAt(x, b.Max.Y-1).A > 0 {
			bottomRowColored++
		}
	}
	if bottomRowColored > 0 {
		t.Errorf("expected 0 colored pixels on bottom row (no clipping), got %d", bottomRowColored)
	}
}

func TestParseSVG_NoExplicitLayersFallback(t *testing.T) {
	raw := `<svg xmlns="http://www.w3.org/2000/svg" width="100" height="100" viewBox="0 0 100 100">
		<circle cx="50" cy="50" r="25" fill="#ff0000" />
	</svg>`
	doc, err := ParseSVG([]byte(raw))
	if err != nil {
		t.Fatalf("ParseSVG failed: %v", err)
	}
	if len(doc.Layers) != 1 {
		t.Fatalf("expected 1 fallback layer, got %d", len(doc.Layers))
	}
	if doc.Layers[0].Label != "Layer 1" {
		t.Errorf("expected layer label 'Layer 1', got %q", doc.Layers[0].Label)
	}

	frameSVG, err := BuildLayerFrameSVG(doc, doc.Layers[0].ID, nil, doc.GetDocumentRect())
	if err != nil {
		t.Fatalf("BuildLayerFrameSVG failed: %v", err)
	}
	img, err := RenderSVGToRGBA(frameSVG, 100, 100)
	if err != nil {
		t.Fatalf("RenderSVGToRGBA failed: %v", err)
	}
	c := img.At(50, 50)
	r, _, _, a := c.RGBA()
	if r == 0 || a == 0 {
		t.Errorf("expected non-zero red pixel at center, got %v", c)
	}
}

