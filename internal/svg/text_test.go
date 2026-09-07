package svg

import (
	"image"
	"image/png"
	"os"
	"strings"
	"testing"

	"golang.org/x/image/font/sfnt"
)

func TestFontManager_EmbeddedFallback(t *testing.T) {
	fm := newFontManager()

	// Test regular font fallback
	fReg := fm.ResolveFont("NonExistentFontFamily123", false, false)
	if fReg == nil {
		t.Fatal("expected non-nil fallback font for regular text")
	}

	// Test bold font fallback
	fBold := fm.ResolveFont("NonExistentFontFamily123", true, false)
	if fBold == nil {
		t.Fatal("expected non-nil fallback font for bold text")
	}

	if fReg.UnitsPerEm() <= 0 || fBold.UnitsPerEm() <= 0 {
		t.Errorf("invalid UnitsPerEm: reg=%d, bold=%d", fReg.UnitsPerEm(), fBold.UnitsPerEm())
	}
}

func TestFontResolution_SegoeUIVariable(t *testing.T) {
	fm := newFontManager()
	f := fm.ResolveFont("Segoe UI Variable", true, false)
	if f == nil {
		t.Fatal("failed to resolve font")
	}
	var b sfnt.Buffer
	family, _ := f.Name(&b, sfnt.NameIDFamily)
	full, _ := f.Name(&b, sfnt.NameIDFull)
	if !strings.Contains(strings.ToLower(family), "segoe") {
		t.Errorf("expected Segoe font, got family %q (full %q)", family, full)
	}
}

func TestGenerateGlyphPathD(t *testing.T) {
	fm := newFontManager()
	f := fm.ResolveFont("sans-serif", true, false)
	if f == nil {
		t.Fatal("failed to resolve font")
	}

	d := GenerateGlyphPathD(f, "!!!", 100, 200, 120, 10, "start")
	if d == "" {
		t.Fatal("expected non-empty SVG path d string")
	}

	// Verify standard path commands are present
	if !strings.Contains(d, "M") || !strings.Contains(d, "Z") {
		t.Errorf("expected M and Z commands in path d, got: %s", d)
	}

	// Verify middle text-anchor shifts starting position to the left
	dMiddle := GenerateGlyphPathD(f, "!!!", 100, 200, 120, 10, "middle")
	if dMiddle == d {
		t.Error("middle anchor should offset path coordinates compared to start anchor")
	}
}

func TestConvertTextToPaths_Basic(t *testing.T) {
	svgInput := `<svg viewBox="0 0 500 500" xmlns="http://www.w3.org/2000/svg">
		<text id="txt1" x="100" y="200" style="font-size:24px;font-family:sans-serif;fill:#ff0000">Hello World</text>
	</svg>`

	out, err := ConvertTextToPaths([]byte(svgInput))
	if err != nil {
		t.Fatalf("ConvertTextToPaths failed: %v", err)
	}

	outStr := string(out)
	if strings.Contains(outStr, "<text") {
		t.Errorf("output still contains <text> element: %s", outStr)
	}
	if !strings.Contains(outStr, "<path") {
		t.Errorf("output does not contain <path> element: %s", outStr)
	}
	if !strings.Contains(outStr, "fill:#ff0000") {
		t.Errorf("output did not preserve fill style: %s", outStr)
	}
}

func TestConvertTextToPaths_TspanFlowed(t *testing.T) {
	svgInput := `<svg viewBox="0 0 500 500" xmlns="http://www.w3.org/2000/svg">
		<text id="text1" transform="matrix(0.78,0,0,0.70,-409,-252)" style="font-weight:bold;font-size:120px;font-family:'Segoe UI Variable';fill:#1f1f1f;stroke:#a72222;stroke-width:13.4;paint-order:stroke fill markers">
			<tspan x="775.37" y="837.21" id="tspan2">!!!</tspan>
		</text>
	</svg>`

	out, err := ConvertTextToPaths([]byte(svgInput))
	if err != nil {
		t.Fatalf("ConvertTextToPaths failed: %v", err)
	}

	outStr := string(out)
	if strings.Contains(outStr, "<text") || strings.Contains(outStr, "<tspan") {
		t.Errorf("output still contains text elements: %s", outStr)
	}
	if !strings.Contains(outStr, `<g id="text1"`) {
		t.Errorf("expected wrapping <g id=\"text1\">, got: %s", outStr)
	}
	if !strings.Contains(outStr, `<path id="tspan2"`) {
		t.Errorf("expected <path id=\"tspan2\">, got: %s", outStr)
	}
	if !strings.Contains(outStr, "tspan2_stroke") && !strings.Contains(outStr, "paint-order") {
		t.Errorf("expected desugared stroke path or preserved paint-order, got: %s", outStr)
	}
}

func TestAlertIcon_TextRenderValidation(t *testing.T) {
	data, err := os.ReadFile("../../testdata/Alert Icon.svg")
	if err != nil {
		t.Fatalf("failed to read Alert Icon.svg: %v", err)
	}

	// Verify PreprocessSVG transforms the text into path
	preprocessed, err := PreprocessSVG(data)
	if err != nil {
		t.Fatalf("PreprocessSVG failed: %v", err)
	}

	if strings.Contains(string(preprocessed), "<text") {
		t.Errorf("PreprocessSVG output should not contain unconverted <text> tags")
	}

	doc, err := ParseSVG(data)
	if err != nil {
		t.Fatalf("ParseSVG failed: %v", err)
	}

	// Build layer 1 frame
	frameBytes, err := BuildLayerFrameSVG(doc, "layer1", nil, Rect{X: 0, Y: 0, Width: 500, Height: 500})
	if err != nil {
		t.Fatalf("BuildLayerFrameSVG failed: %v", err)
	}

	// Render layer 1 frame at 521x521
	img, err := RenderSVGToRGBA(frameBytes, 521, 521)
	if err != nil {
		t.Fatalf("RenderSVGToRGBA failed: %v", err)
	}

	// Load golden comparison image
	goldenFile, err := os.Open("../../testdata/Alert Icon-golden.png")
	if err != nil {
		t.Fatalf("failed to open Alert Icon-golden.png: %v", err)
	}
	defer func() {
		_ = goldenFile.Close()
	}()

	goldenImg, err := png.Decode(goldenFile)
	if err != nil {
		t.Fatalf("failed to decode golden png: %v", err)
	}

	// Check pixel sampling at the exclamation mark region (approx x: 170..230, y: 250..340)
	// In the golden image, this region contains non-transparent red stroke (#a72222) and dark fill (#1f1f1f).
	var renderedTextPixels, goldenTextPixels int
	for y := 260; y < 340; y++ {
		for x := 170; x < 230; x++ {
			r1, g1, b1, a1 := img.At(x, y).RGBA()
			r2, g2, b2, a2 := goldenImg.At(x, y).RGBA()

			// Check if pixel is part of the exclamation mark (red stroke or dark fill)
			if a1 > 0x8000 && (r1 > 0x8000 || (r1 < 0x4000 && g1 < 0x4000 && b1 < 0x4000)) {
				renderedTextPixels++
			}
			if a2 > 0x8000 && (r2 > 0x8000 || (r2 < 0x4000 && g2 < 0x4000 && b2 < 0x4000)) {
				goldenTextPixels++
			}
		}
	}

	t.Logf("Exclamation mark region pixels: rendered=%d, golden=%d", renderedTextPixels, goldenTextPixels)
	if renderedTextPixels < 500 {
		t.Errorf("expected at least 500 text pixels in rendered Alert Icon exclamation region, got %d", renderedTextPixels)
	}

	// Verify file on disk is unchanged
	diskData, err := os.ReadFile("../../testdata/Alert Icon.svg")
	if err != nil {
		t.Fatalf("failed to re-read Alert Icon.svg: %v", err)
	}
	if len(diskData) != len(data) {
		t.Errorf("testdata/Alert Icon.svg was modified on disk! expected %d bytes, got %d", len(data), len(diskData))
	}
}

// Ensure unused import warnings are prevented
var _ image.Image
