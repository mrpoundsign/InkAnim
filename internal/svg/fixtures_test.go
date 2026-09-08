package svg

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// ExportWithInkscape runs the installed headless inkscape CLI to export an SVG to PNG at width x height.
func ExportWithInkscape(svgPath, outPNGPath string, width, height int) error {
	inkscapeBin, err := exec.LookPath("inkscape")
	if err != nil {
		return fmt.Errorf("inkscape CLI not found: %w", err)
	}

	args := []string{
		svgPath,
		"--export-type=png",
		"--export-filename=" + outPNGPath,
		"-w", strconv.Itoa(width),
		"-h", strconv.Itoa(height),
	}

	cmd := exec.Command(inkscapeBin, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("inkscape failed (%w): %s", err, string(out))
	}
	return nil
}

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

			if baseName == "lpe_fillet_chamfer" {
				t.Skip("Skipping lpe_fillet_chamfer pending Issue #41 fix")
			}
			if baseName == "text_tspan_basic" {
				t.Skip("Skipping text_tspan_basic pending Issue #42 CI font resolution")
			}

			svgPath := filepath.Join(fixturesDir, entry.Name())
			goldenPath := filepath.Join(fixturesDir, baseName+".golden.png")

			data, err := os.ReadFile(svgPath)
			if err != nil {
				t.Fatalf("failed to read fixture %s: %v", svgPath, err)
			}

			// If golden does not exist, export reference using headless Inkscape
			if _, err := os.Stat(goldenPath); os.IsNotExist(err) {
				if err := ExportWithInkscape(svgPath, goldenPath, 128, 128); err != nil {
					t.Fatalf("failed to generate reference golden with inkscape: %v", err)
				}
				t.Logf("Generated reference golden via Inkscape: %s", goldenPath)
			}

			// Preprocess SVG (runs font conversion, LPE, rect rx/ry, paint-order, transform stroke scaling)
			preprocessed, err := PreprocessSVG(data)
			if err != nil {
				t.Fatalf("PreprocessSVG failed for %s: %v", entry.Name(), err)
			}

			actualImg, err := RenderSVGToRGBA(preprocessed, 128, 128)
			if err != nil {
				t.Fatalf("RenderSVGToRGBA failed for %s: %v", entry.Name(), err)
			}

			// Tolerance options tailored for vector rasterizer antialiasing vs Inkscape cairo/skia
			opts := GoldenCompareOptions{
				PerPixelTolerance:  35,
				MaxMismatchPercent: 3.0,
			}

			AssertImageMatchesGolden(t, actualImg, goldenPath, opts)
		})
	}
}
