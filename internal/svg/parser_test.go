package svg

import (
	"strings"
	"testing"
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
	frame2Bytes, err := BuildLayerFrameSVG(doc, "layer_frame2", pinned)
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
	frame1Bytes, err := BuildLayerFrameSVG(doc, "layer_frame1", pinned)
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

	page2Bytes, err := BuildPageFrameSVG(doc, doc.Pages[1])
	if err != nil {
		t.Fatalf("BuildPageFrameSVG failed: %v", err)
	}

	page2Str := string(page2Bytes)
	if !strings.Contains(page2Str, `viewBox="100.000000 0.000000 100.000000 100.000000"`) {
		t.Errorf("unexpected viewBox in page frame: %s", page2Str)
	}
}
