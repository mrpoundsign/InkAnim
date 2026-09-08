package svg

import (
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
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".svg") {
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

			// Preprocess SVG (runs font conversion, LPE, rect rx/ry, paint-order, transform stroke scaling)
			preprocessed, err := PreprocessSVG(data)
			if err != nil {
				t.Fatalf("PreprocessSVG failed for %s: %v", entry.Name(), err)
			}

			actualImg, err := RenderSVGToRGBA(preprocessed, renderWidth, renderHeight)
			if err != nil {
				t.Fatalf("RenderSVGToRGBA failed for %s: %v", entry.Name(), err)
			}

			opts := GoldenCompareOptions{
				PerPixelTolerance:  35,
				MaxMismatchPercent: 3.0,
			}

			AssertImageMatchesGolden(t, actualImg, goldenPath, opts)
		})
	}
}
