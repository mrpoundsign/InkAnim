package svg

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var updateGolden = flag.Bool("update-golden", false, "update golden reference PNG files")

// GoldenCompareOptions specifies tolerances for image comparison.
type GoldenCompareOptions struct {
	// PerPixelTolerance is the maximum allowed difference (0-255) for any color channel (R, G, B, A).
	// Pixel differences within this tolerance are considered matching (accounts for rasterizer antialiasing).
	PerPixelTolerance uint8

	// MaxMismatchPercent is the maximum allowed percentage of total pixels that may exceed PerPixelTolerance.
	// For example, 1.0 means up to 1.0% of pixels can differ before the test fails.
	MaxMismatchPercent float64
}

// DefaultGoldenCompareOptions returns balanced defaults for SVG rasterization comparison.
func DefaultGoldenCompareOptions() GoldenCompareOptions {
	return GoldenCompareOptions{
		PerPixelTolerance:  20,
		MaxMismatchPercent: 1.0,
	}
}

// DiffStats records metrics from a golden image comparison.
type DiffStats struct {
	TotalPixels      int
	MismatchedPixels int
	MismatchPercent  float64
	MaxChannelDiff   uint8
	MinX, MinY       int
	MaxX, MaxY       int
}

// AssertImageMatchesGolden compares actual against the golden image file at goldenRelPath.
// If -update-golden is set, actual is saved to goldenRelPath.
// If the comparison fails, an annotated diff image is saved alongside the golden image.
func AssertImageMatchesGolden(t *testing.T, actual image.Image, goldenRelPath string, opts GoldenCompareOptions) {
	t.Helper()

	if *updateGolden {
		if err := savePNG(goldenRelPath, actual); err != nil {
			t.Fatalf("failed to update golden file %s: %v", goldenRelPath, err)
		}
		t.Logf("Updated golden reference image: %s", goldenRelPath)
		return
	}

	goldenFile, err := os.Open(goldenRelPath)
	if err != nil {
		if os.IsNotExist(err) {
			t.Fatalf("golden file %s does not exist; run tests with -update-golden to generate it", goldenRelPath)
		}
		t.Fatalf("failed to open golden file %s: %v", goldenRelPath, err)
	}
	defer func() { _ = goldenFile.Close() }()

	golden, err := png.Decode(goldenFile)
	if err != nil {
		t.Fatalf("failed to decode golden png %s: %v", goldenRelPath, err)
	}

	actualBounds := actual.Bounds()
	goldenBounds := golden.Bounds()

	actualW, actualH := actualBounds.Dx(), actualBounds.Dy()
	goldenW, goldenH := goldenBounds.Dx(), goldenBounds.Dy()

	if actualW != goldenW || actualH != goldenH {
		t.Fatalf("image dimensions mismatch for %s: actual %dx%d, golden %dx%d",
			goldenRelPath, actualW, actualH, goldenW, goldenH)
	}

	stats, diffImg := computeDiff(actual, golden, opts.PerPixelTolerance)

	t.Logf("Golden comparison [%s]: %d/%d mismatched pixels (%.2f%%, bounds: [%d,%d .. %d,%d], max channel diff: %d, threshold: %.2f%%)",
		filepath.Base(goldenRelPath), stats.MismatchedPixels, stats.TotalPixels, stats.MismatchPercent,
		stats.MinX, stats.MinY, stats.MaxX, stats.MaxY, stats.MaxChannelDiff, opts.MaxMismatchPercent)

	if stats.MismatchPercent > opts.MaxMismatchPercent {
		ext := filepath.Ext(goldenRelPath)
		base := strings.TrimSuffix(goldenRelPath, ext)
		diffPath := base + "_diff.png"
		actualPath := base + "_actual.png"

		_ = savePNG(diffPath, diffImg)
		_ = savePNG(actualPath, actual)

		t.Errorf("image %s exceeds mismatch threshold: %.2f%% > %.2f%% (%d pixels differ). Diff saved to %s",
			goldenRelPath, stats.MismatchPercent, opts.MaxMismatchPercent, stats.MismatchedPixels, diffPath)
	}
}

func computeDiff(actual, golden image.Image, tolerance uint8) (DiffStats, *image.RGBA) {
	b := actual.Bounds()
	w, h := b.Dx(), b.Dy()

	diffImg := image.NewRGBA(image.Rect(0, 0, w, h))
	var stats DiffStats
	stats.TotalPixels = w * h

	for y := range h {
		for x := range w {
			actPt := image.Pt(b.Min.X+x, b.Min.Y+y)
			goldPt := image.Pt(golden.Bounds().Min.X+x, golden.Bounds().Min.Y+y)

			ar, ag, ab, aa := actual.At(actPt.X, actPt.Y).RGBA()
			gr, gg, gb, ga := golden.At(goldPt.X, goldPt.Y).RGBA()

			// Scale down from 16-bit RGBA (0-65535) to 8-bit (0-255)
			a8 := [4]uint8{uint8(ar >> 8), uint8(ag >> 8), uint8(ab >> 8), uint8(aa >> 8)}
			g8 := [4]uint8{uint8(gr >> 8), uint8(gg >> 8), uint8(gb >> 8), uint8(ga >> 8)}

			var maxDiff uint8
			for c := range 4 {
				diff := uint8(math.Abs(float64(int(a8[c]) - int(g8[c]))))
				if diff > maxDiff {
					maxDiff = diff
				}
			}

			if maxDiff > stats.MaxChannelDiff {
				stats.MaxChannelDiff = maxDiff
			}

			if maxDiff > tolerance {
				stats.MismatchedPixels++
				if stats.MismatchedPixels == 1 {
					stats.MinX, stats.MaxX = x, x
					stats.MinY, stats.MaxY = y, y
				} else {
					if x < stats.MinX {
						stats.MinX = x
					}
					if x > stats.MaxX {
						stats.MaxX = x
					}
					if y < stats.MinY {
						stats.MinY = y
					}
					if y > stats.MaxY {
						stats.MaxY = y
					}
				}
				// Highlight mismatched pixel in vivid neon pink/magenta
				diffImg.Set(x, y, color.RGBA{R: 255, G: 0, B: 100, A: 255})
			} else {
				// Dim matching pixel in grayscale for context
				gray := uint8(0.299*float64(a8[0]) + 0.587*float64(a8[1]) + 0.114*float64(a8[2]))
				dimGray := gray / 4
				diffImg.Set(x, y, color.RGBA{R: dimGray, G: dimGray, B: dimGray, A: 255})
			}
		}
	}

	if stats.TotalPixels > 0 {
		stats.MismatchPercent = (float64(stats.MismatchedPixels) / float64(stats.TotalPixels)) * 100.0
	}

	return stats, diffImg
}

func savePNG(path string, img image.Image) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create dir %s: %w", dir, err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	if err := png.Encode(f, img); err != nil {
		return fmt.Errorf("failed to encode png: %w", err)
	}
	return nil
}

func TestGolden_AlertIcon(t *testing.T) {
	svgBytes, err := os.ReadFile("../../testdata/Alert Icon.svg")
	if err != nil {
		t.Fatalf("failed to read Alert Icon.svg: %v", err)
	}

	doc, err := ParseSVG(svgBytes)
	if err != nil {
		t.Fatalf("failed to parse Alert Icon.svg: %v", err)
	}

	frame, err := BuildLayerFrameSVG(doc, "layer1", nil, doc.GetDocumentRect())
	if err != nil {
		t.Fatalf("failed to build frame: %v", err)
	}

	img, err := RenderSVGToRGBA(frame, 521, 521)
	if err != nil {
		t.Fatalf("failed to render frame: %v", err)
	}

	// Alert Icon includes text glyphs from Segoe UI and LPE curves.
	// Cairo/Inkscape vs rasterx produces minor curve rasterization differences (2.18% on Windows).
	// On Linux/non-Windows platforms, Segoe UI falls back to system Noto Sans / DejaVu Sans,
	// producing ~2.99% font contour variance. Allow 3.5% cross-platform.
	opts := GoldenCompareOptions{
		PerPixelTolerance:  25,
		MaxMismatchPercent: 3.5,
	}
	AssertImageMatchesGolden(t, img, "../../testdata/Alert Icon-golden.png", opts)
}

func TestGolden_Hydrate(t *testing.T) {
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

	img, err := RenderSVGToRGBA(frame, 794, 1123)
	if err != nil {
		t.Fatalf("failed to render frame: %v", err)
	}

	// hydrate golden was exported from Inkscape at 794x1123.
	opts := GoldenCompareOptions{
		PerPixelTolerance:  25,
		MaxMismatchPercent: 1.5,
	}
	AssertImageMatchesGolden(t, img, "../../testdata/hydrate-golden.png", opts)
}

func TestGolden_CharacterWalk(t *testing.T) {
	svgBytes, err := os.ReadFile("../../testdata/character_walk.svg")
	if err != nil {
		t.Fatalf("failed to read character_walk.svg: %v", err)
	}

	doc, err := ParseSVG(svgBytes)
	if err != nil {
		t.Fatalf("failed to parse character_walk.svg: %v", err)
	}

	layers := []string{"layer_frame1", "layer_frame2", "layer_frame3"}
	opts := DefaultGoldenCompareOptions()

	for i, layerID := range layers {
		frame, err := BuildLayerFrameSVG(doc, layerID, nil, doc.GetDocumentRect())
		if err != nil {
			t.Fatalf("failed to build frame for %s: %v", layerID, err)
		}

		img, err := RenderSVGToRGBA(frame, 256, 256)
		if err != nil {
			t.Fatalf("failed to render frame %s: %v", layerID, err)
		}

		goldenPath := fmt.Sprintf("../../testdata/golden/character_walk_f%d.png", i+1)
		AssertImageMatchesGolden(t, img, goldenPath, opts)
	}
}

func TestGolden_BouncingWalker(t *testing.T) {
	svgBytes, err := os.ReadFile("../../testdata/bouncing_walker.svg")
	if err != nil {
		t.Fatalf("failed to read bouncing_walker.svg: %v", err)
	}

	doc, err := ParseSVG(svgBytes)
	if err != nil {
		t.Fatalf("failed to parse bouncing_walker.svg: %v", err)
	}

	// Frame 1 with pinned background layer
	frame, err := BuildLayerFrameSVG(doc, "frame1_left_wall", map[string]bool{"layer_bg_grid": true}, doc.GetDocumentRect())
	if err != nil {
		t.Fatalf("failed to build frame for bouncing_walker: %v", err)
	}

	img, err := RenderSVGToRGBA(frame, 256, 256)
	if err != nil {
		t.Fatalf("failed to render frame: %v", err)
	}

	opts := DefaultGoldenCompareOptions()
	AssertImageMatchesGolden(t, img, "../../testdata/golden/bouncing_walker_f1.png", opts)
}

func TestGolden_MultipageWalk(t *testing.T) {
	svgBytes, err := os.ReadFile("../../testdata/multipage_walk.svg")
	if err != nil {
		t.Fatalf("failed to read multipage_walk.svg: %v", err)
	}

	doc, err := ParseSVG(svgBytes)
	if err != nil {
		t.Fatalf("failed to parse multipage_walk.svg: %v", err)
	}

	opts := DefaultGoldenCompareOptions()

	for i, page := range doc.Pages {
		frame, err := BuildPageFrameSVG(doc, page, Rect{})
		if err != nil {
			t.Fatalf("failed to build page frame for %s: %v", page.ID, err)
		}

		img, err := RenderSVGToRGBA(frame, 256, 256)
		if err != nil {
			t.Fatalf("failed to render page frame %s: %v", page.ID, err)
		}

		goldenPath := fmt.Sprintf("../../testdata/golden/multipage_walk_p%d.png", i+1)
		AssertImageMatchesGolden(t, img, goldenPath, opts)
	}
}

func TestComputeDiff_AntialiasingToleranceAndFailureDiff(t *testing.T) {
	// Create two 10x10 test images
	actual := image.NewRGBA(image.Rect(0, 0, 10, 10))
	golden := image.NewRGBA(image.Rect(0, 0, 10, 10))

	// Base background: white
	for y := range 10 {
		for x := range 10 {
			actual.Set(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 255})
			golden.Set(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}

	// 1 pixel with minor antialiasing difference: delta = 10 (<= tolerance of 20)
	actual.Set(2, 2, color.RGBA{R: 245, G: 255, B: 255, A: 255})

	// 1 pixel with intentional major mismatch: delta = 255 (> tolerance of 20)
	actual.Set(5, 5, color.RGBA{R: 0, G: 0, B: 0, A: 255})

	stats, diffImg := computeDiff(actual, golden, 20)

	if stats.TotalPixels != 100 {
		t.Errorf("expected 100 total pixels, got %d", stats.TotalPixels)
	}
	if stats.MismatchedPixels != 1 {
		t.Errorf("expected exactly 1 mismatched pixel (major diff), got %d", stats.MismatchedPixels)
	}
	if stats.MismatchPercent != 1.0 {
		t.Errorf("expected 1.0%% mismatch, got %f", stats.MismatchPercent)
	}
	if stats.MinX != 5 || stats.MinY != 5 || stats.MaxX != 5 || stats.MaxY != 5 {
		t.Errorf("expected mismatch bounds [5,5..5,5], got [%d,%d..%d,%d]", stats.MinX, stats.MinY, stats.MaxX, stats.MaxY)
	}

	// Verify diff image has neon magenta at (5, 5)
	diffColor := diffImg.RGBAAt(5, 5)
	if diffColor.R != 255 || diffColor.G != 0 || diffColor.B != 100 {
		t.Errorf("expected neon pink/magenta diff color at (5,5), got %+v", diffColor)
	}

	// Verify pixel (2, 2) with minor delta <= tolerance is treated as matching (dim gray)
	matchColor := diffImg.RGBAAt(2, 2)
	if matchColor.R == 255 && matchColor.G == 0 && matchColor.B == 100 {
		t.Errorf("pixel with minor diff <= tolerance should not be marked as mismatch")
	}
}
