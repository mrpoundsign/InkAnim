package inksvg

import (
	"fmt"
	"image"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAtomicFixtures renders each synthetic, minimal SVG fixture and verifies it matches the golden render.
func TestAtomicFixtures(t *testing.T) {
	t.Parallel()

	fixturesDir := filepath.Join("..", "..", "testdata", "fixtures")
	entries, err := os.ReadDir(fixturesDir)
	if err != nil {
		t.Fatalf("failed to read fixtures dir %s: %v", fixturesDir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".svg") || strings.HasPrefix(entry.Name(), "multipage_") {
			continue
		}

		baseName := strings.TrimSuffix(entry.Name(), ".svg")
		t.Run(baseName, func(t *testing.T) {
			t.Parallel()

			svgPath := filepath.Join(fixturesDir, entry.Name())
			goldenPath := filepath.Join(fixturesDir, baseName+".golden.png")

			data, err := os.ReadFile(svgPath)
			if err != nil {
				t.Fatalf("failed to read fixture %s: %v", svgPath, err)
			}

			// Determine fixture render dimensions: match golden reference bounds if existing,
			// or derive from SVG root dimensions, defaulting to 128x128.
			renderWidth, renderHeight := 128, 128
			if gf, err := os.Open(goldenPath); err == nil {
				if cfg, _, err := image.DecodeConfig(gf); err == nil && cfg.Width > 0 && cfg.Height > 0 {
					renderWidth, renderHeight = cfg.Width, cfg.Height
				}
				_ = gf.Close()
			} else if doc, err := ParseSVG(data); err == nil && doc.Width > 0 && doc.Height > 0 {
				renderWidth, renderHeight = int(doc.Width), int(doc.Height)
			}

			// Golden reference must be committed; tests do not invoke Inkscape at runtime
			if _, err := os.Stat(goldenPath); os.IsNotExist(err) {
				t.Fatalf("missing golden reference %s; generate and commit it using headless inkscape before running tests: inkscape %s --export-type=png --export-filename=%s -w %d -h %d",
					goldenPath, svgPath, goldenPath, renderWidth, renderHeight)
			}

			doc, err := ParseSVG(data)
			if err != nil {
				t.Fatalf("ParseSVG failed for %s: %v", entry.Name(), err)
			}

			// Render through full session frame building pipeline (as used by GUI preview & export)
			layerID := ""
			if len(doc.Layers) > 0 {
				layerID = doc.Layers[0].ID
			}
			frameSVG, err := BuildLayerFrameSVG(doc, layerID, nil, doc.GetDocumentRect())
			if err != nil {
				t.Fatalf("BuildLayerFrameSVG failed for %s: %v", entry.Name(), err)
			}

			actualImg, err := RenderSVGToRGBA(frameSVG, renderWidth, renderHeight)
			if err != nil {
				t.Fatalf("RenderSVGToRGBA failed for %s: %v", entry.Name(), err)
			}

			// Also verify that scaling to standard GUI preview dimensions (512x512) completes cleanly
			if _, err := RenderSVGToRGBA(frameSVG, 512, 512); err != nil {
				t.Fatalf("Scaled GUI preview render (512x512) failed for %s: %v", entry.Name(), err)
			}

			maxMismatch := 0.5 // 99.5%+ compliance for all vector shapes and paths
			if strings.HasPrefix(baseName, "text_") {
				maxMismatch = 3.0 // allowance for cross-platform system font metrics & antialiasing
			}

			opts := GoldenCompareOptions{
				PerPixelTolerance:  35,
				MaxMismatchPercent: maxMismatch,
			}

			AssertImageMatchesGolden(t, actualImg, goldenPath, opts)
		})
	}
}

// TestMultiPageFixtures verifies that individual pages from a multi-page Inkscape SVG
// render with high fidelity to their respective golden ground-truth reference images.
func TestMultiPageFixtures(t *testing.T) {
	t.Parallel()

	fixturesDir := filepath.Join("..", "..", "testdata", "fixtures")
	svgPath := filepath.Join(fixturesDir, "multipage_sizes.svg")

	data, err := os.ReadFile(svgPath)
	if err != nil {
		t.Fatalf("failed to read fixture %s: %v", svgPath, err)
	}

	doc, err := ParseSVG(data)
	if err != nil {
		t.Fatalf("ParseSVG failed: %v", err)
	}

	if len(doc.Pages) < 2 {
		t.Fatalf("expected at least 2 pages, got %d", len(doc.Pages))
	}

	for i, page := range doc.Pages {
		pageIdx := i + 1
		t.Run(fmt.Sprintf("page_%d", pageIdx), func(t *testing.T) {
			goldenPath := filepath.Join(fixturesDir, fmt.Sprintf("multipage_sizes_page%d.golden.png", pageIdx))
			if _, err := os.Stat(goldenPath); os.IsNotExist(err) {
				t.Fatalf("missing golden reference %s", goldenPath)
			}

			pageRect := Rect{X: page.X, Y: page.Y, Width: page.Width, Height: page.Height}
			pageSVG, err := BuildPageFrameSVG(doc, page, pageRect)
			if err != nil {
				t.Fatalf("BuildPageFrameSVG failed for page %d: %v", pageIdx, err)
			}

			renderW, renderH := 512, 512
			actualImg, err := RenderSVGToRGBA(pageSVG, renderW, renderH)
			if err != nil {
				t.Fatalf("RenderSVGToRGBA failed for page %d: %v", pageIdx, err)
			}

			opts := GoldenCompareOptions{
				PerPixelTolerance:  35,
				MaxMismatchPercent: 0.5,
			}
			AssertImageMatchesGolden(t, actualImg, goldenPath, opts)
		})
	}
}
