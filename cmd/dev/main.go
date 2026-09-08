package main

import (
	"bytes"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"inkanim/internal/svg"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	subcmd := os.Args[1]
	subArgs := os.Args[2:]

	switch subcmd {
	case "golden":
		runGolden(subArgs)
	case "inspect":
		runInspect(subArgs)
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown subcommand: %s\n\n", subcmd)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`InkAnim Developer Tool (cmd/dev)

Usage:
  go run ./cmd/dev <command> [arguments]

Commands:
  golden   Compare SVG fixtures against golden PNGs or generate ground truth
  inspect  Inspect pixel bounds, centroids, and colors of an image

Examples:
  go run ./cmd/dev golden use_element_clone
  go run ./cmd/dev golden --all
  go run ./cmd/dev golden --generate use_element_clone
  go run ./cmd/dev inspect testdata/fixtures/use_element_clone.golden.png
  go run ./cmd/dev inspect testdata/fixtures/use_element_clone.golden.png --color "#facc15"`)
}

// runGolden handles fixture comparison and ground-truth generation via headless Inkscape CLI.
func runGolden(args []string) {
	fs := flag.NewFlagSet("golden", flag.ExitOnError)
	generate := fs.Bool("generate", false, "Generate golden PNG(s) using headless Inkscape CLI")
	all := fs.Bool("all", false, "Process all fixtures in testdata/fixtures")
	saveDiff := fs.Bool("save-diff", true, "Save diff PNG on comparison failure")
	tolerance := fs.Int("tol", 35, "Per-pixel color channel tolerance (0-255)")
	maxMismatch := fs.Float64("max-mismatch", 3.0, "Max allowed mismatch percentage (0-100)")

	_ = fs.Parse(reorderArgs(args, map[string]bool{"generate": true, "all": true, "save-diff": true}))

	fixturesDir := filepath.Join("testdata", "fixtures")
	if _, err := os.Stat(fixturesDir); os.IsNotExist(err) {
		fixturesDir = filepath.Join("..", "..", "testdata", "fixtures")
	}

	var targets []string
	if *all || fs.NArg() == 0 {
		entries, err := os.ReadDir(fixturesDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", fixturesDir, err)
			os.Exit(1)
		}
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".svg") {
				targets = append(targets, strings.TrimSuffix(e.Name(), ".svg"))
			}
		}
	} else {
		for _, arg := range fs.Args() {
			name := strings.TrimSuffix(filepath.Base(arg), ".svg")
			targets = append(targets, name)
		}
	}

	if len(targets) == 0 {
		fmt.Println("No fixtures found.")
		return
	}

	hasFailure := false
	for _, name := range targets {
		svgPath := filepath.Join(fixturesDir, name+".svg")
		goldenPath := filepath.Join(fixturesDir, name+".golden.png")

		if _, err := os.Stat(svgPath); os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "[%s] SVG file not found: %s\n", name, svgPath)
			hasFailure = true
			continue
		}

		w, h := resolveFixtureDimensions(svgPath, goldenPath)

		if *generate {
			err := generateGoldenWithInkscape(svgPath, goldenPath, w, h)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[%s] FAILED generating golden: %v\n", name, err)
				hasFailure = true
			} else {
				fmt.Printf("[%s] Golden generated: %s (%dx%d)\n", name, goldenPath, w, h)
			}
			continue
		}

		// Comparison mode
		data, err := os.ReadFile(svgPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[%s] Failed reading SVG: %v\n", name, err)
			hasFailure = true
			continue
		}

		preprocessed, err := svg.PreprocessSVG(data)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[%s] PreprocessSVG failed: %v\n", name, err)
			hasFailure = true
			continue
		}

		actualImg, err := svg.RenderSVGToRGBA(preprocessed, w, h)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[%s] RenderSVGToRGBA failed: %v\n", name, err)
			hasFailure = true
			continue
		}

		gf, err := os.Open(goldenPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[%s] Golden PNG missing: %s (run with --generate to create)\n", name, goldenPath)
			hasFailure = true
			continue
		}
		goldenImg, err := png.Decode(gf)
		_ = gf.Close()
		if err != nil {
			fmt.Fprintf(os.Stderr, "[%s] Failed decoding golden PNG: %v\n", name, err)
			hasFailure = true
			continue
		}

		stats, diffImg := diffImages(actualImg, goldenImg, uint8(*tolerance))

		if stats.MismatchPercent > *maxMismatch {
			hasFailure = true
			fmt.Printf("❌ [%s] MISMATCH: %.2f%% (%d/%d px differ, max channel diff: %d, bbox: [%d,%d .. %d,%d])\n",
				name, stats.MismatchPercent, stats.MismatchedPixels, stats.TotalPixels, stats.MaxChannelDiff,
				stats.MinX, stats.MinY, stats.MaxX, stats.MaxY)

			if *saveDiff {
				scratchDir := filepath.Join("testdata", "scratch")
				_ = os.MkdirAll(scratchDir, 0755)
				diffPath := filepath.Join(scratchDir, name+"_diff.png")
				if err := savePNGFile(diffPath, diffImg); err == nil {
					fmt.Printf("   Diff saved to: %s\n", diffPath)
				}
			}
		} else {
			fmt.Printf("✅ [%s] PASS: %.2f%% mismatch (%d/%d px, threshold: %.2f%%)\n",
				name, stats.MismatchPercent, stats.MismatchedPixels, stats.TotalPixels, *maxMismatch)
		}
	}

	if hasFailure {
		os.Exit(1)
	}
}

func resolveFixtureDimensions(svgPath, goldenPath string) (int, int) {
	if gf, err := os.Open(goldenPath); err == nil {
		cfg, _, err := image.DecodeConfig(gf)
		_ = gf.Close()
		if err == nil && cfg.Width > 0 && cfg.Height > 0 {
			return cfg.Width, cfg.Height
		}
	}
	if data, err := os.ReadFile(svgPath); err == nil {
		if doc, err := svg.ParseSVG(data); err == nil && doc.Width > 0 && doc.Height > 0 {
			return int(doc.Width), int(doc.Height)
		}
	}
	return 128, 128
}

func generateGoldenWithInkscape(svgPath, goldenPath string, w, h int) error {
	inkscapeBin, err := exec.LookPath("inkscape")
	if err != nil {
		return fmt.Errorf("inkscape CLI not found in PATH: %w", err)
	}

	cmd := exec.Command(inkscapeBin,
		svgPath,
		"--export-type=png",
		"--export-filename=" + goldenPath,
		"-w", strconv.Itoa(w),
		"-h", strconv.Itoa(h),
	)
	var outBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &outBuf

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("inkscape failed: %w (output: %s)", err, outBuf.String())
	}
	return nil
}

type diffStats struct {
	TotalPixels      int
	MismatchedPixels int
	MismatchPercent  float64
	MaxChannelDiff   uint8
	MinX, MinY       int
	MaxX, MaxY       int
}

func diffImages(actual, golden image.Image, tolerance uint8) (diffStats, *image.RGBA) {
	b := actual.Bounds()
	w, h := b.Dx(), b.Dy()

	diffImg := image.NewRGBA(image.Rect(0, 0, w, h))
	var stats diffStats
	stats.TotalPixels = w * h
	stats.MinX, stats.MinY = w, h
	stats.MaxX, stats.MaxY = 0, 0

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			actPt := image.Pt(b.Min.X+x, b.Min.Y+y)
			goldPt := image.Pt(golden.Bounds().Min.X+x, golden.Bounds().Min.Y+y)

			ar, ag, ab, aa := actual.At(actPt.X, actPt.Y).RGBA()
			gr, gg, gb, ga := golden.At(goldPt.X, goldPt.Y).RGBA()

			a8 := [4]uint8{uint8(ar >> 8), uint8(ag >> 8), uint8(ab >> 8), uint8(aa >> 8)}
			g8 := [4]uint8{uint8(gr >> 8), uint8(gg >> 8), uint8(gb >> 8), uint8(ga >> 8)}

			var maxDiff uint8
			for c := 0; c < 4; c++ {
				diff := uint8(math.Abs(float64(int(a8[c]) - int(g8[c]))))
				if diff > maxDiff {
					maxDiff = diff
				}
			}

			if maxDiff > tolerance {
				stats.MismatchedPixels++
				if maxDiff > stats.MaxChannelDiff {
					stats.MaxChannelDiff = maxDiff
				}
				if x < stats.MinX {
					stats.MinX = x
				}
				if y < stats.MinY {
					stats.MinY = y
				}
				if x > stats.MaxX {
					stats.MaxX = x
				}
				if y > stats.MaxY {
					stats.MaxY = y
				}
				diffImg.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
			} else {
				diffImg.Set(x, y, color.RGBA{R: g8[0] / 3, G: g8[1] / 3, B: g8[2] / 3, A: 255})
			}
		}
	}

	if stats.MismatchedPixels > 0 {
		stats.MismatchPercent = (float64(stats.MismatchedPixels) / float64(stats.TotalPixels)) * 100.0
	} else {
		stats.MinX, stats.MinY = 0, 0
	}
	return stats, diffImg
}

func savePNGFile(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return png.Encode(f, img)
}

// runInspect inspects pixel properties, non-transparent bounding boxes, centroids, or color matches.
func runInspect(args []string) {
	fs := flag.NewFlagSet("inspect", flag.ExitOnError)
	colorHex := fs.String("color", "", "Hex color to match and locate centroid for (e.g. '#facc15' or 'facc15')")
	tolerance := fs.Int("tol", 35, "Color distance tolerance (0-255)")
	_ = fs.Parse(reorderArgs(args, nil))

	if fs.NArg() == 0 {
		fmt.Fprintf(os.Stderr, "Usage: go run ./cmd/dev inspect <image-path> [--color #hex]\n")
		os.Exit(1)
	}

	imgPath := fs.Arg(0)
	f, err := os.Open(imgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open image %s: %v\n", imgPath, err)
		os.Exit(1)
	}

	img, err := png.Decode(f)
	_ = f.Close()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to decode PNG %s: %v\n", imgPath, err)
		os.Exit(1)
	}

	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	fmt.Printf("Image: %s\nDimensions: %dx%d\n", imgPath, w, h)

	if *colorHex != "" {
		targetR, targetG, targetB, err := parseHexColor(*colorHex)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid hex color %s: %v\n", *colorHex, err)
			os.Exit(1)
		}

		matchGrid := make([][]bool, h)
		for i := range matchGrid {
			matchGrid[i] = make([]bool, w)
		}

		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				r, g, b, a := img.At(x, y).RGBA()
				if a == 0 {
					continue
				}
				r8, g8, b8 := uint8(r>>8), uint8(g>>8), uint8(b>>8)

				dr := math.Abs(float64(int(r8) - int(targetR)))
				dg := math.Abs(float64(int(g8) - int(targetG)))
				db := math.Abs(float64(int(b8) - int(targetB)))

				if dr <= float64(*tolerance) && dg <= float64(*tolerance) && db <= float64(*tolerance) {
					matchGrid[y-bounds.Min.Y][x-bounds.Min.X] = true
				}
			}
		}

		type cluster struct {
			count      int
			minX, minY int
			maxX, maxY int
			cx, cy     float64
		}

		visited := make([][]bool, h)
		for i := range visited {
			visited[i] = make([]bool, w)
		}

		var clusters []cluster
		var totalCount, totalSumX, totalSumY int
		totalMinX, totalMinY := w, h
		totalMaxX, totalMaxY := 0, 0

		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				if !matchGrid[y][x] || visited[y][x] {
					continue
				}

				// BFS 8-way flood fill
				var q [][2]int
				q = append(q, [2]int{x, y})
				visited[y][x] = true

				var cCount int
				var cSumX, cSumY int
				cMinX, cMinY := x, y
				cMaxX, cMaxY := x, y

				for len(q) > 0 {
					curr := q[0]
					q = q[1:]
					cx, cy := curr[0], curr[1]
					cCount++
					cSumX += cx + bounds.Min.X
					cSumY += cy + bounds.Min.Y

					if cx+bounds.Min.X < cMinX {
						cMinX = cx + bounds.Min.X
					}
					if cy+bounds.Min.Y < cMinY {
						cMinY = cy + bounds.Min.Y
					}
					if cx+bounds.Min.X > cMaxX {
						cMaxX = cx + bounds.Min.X
					}
					if cy+bounds.Min.Y > cMaxY {
						cMaxY = cy + bounds.Min.Y
					}

					for dy := -1; dy <= 1; dy++ {
						for dx := -1; dx <= 1; dx++ {
							nx, ny := cx+dx, cy+dy
							if nx >= 0 && nx < w && ny >= 0 && ny < h {
								if matchGrid[ny][nx] && !visited[ny][nx] {
									visited[ny][nx] = true
									q = append(q, [2]int{nx, ny})
								}
							}
						}
					}
				}

				totalCount += cCount
				totalSumX += cSumX
				totalSumY += cSumY
				if cMinX < totalMinX {
					totalMinX = cMinX
				}
				if cMinY < totalMinY {
					totalMinY = cMinY
				}
				if cMaxX > totalMaxX {
					totalMaxX = cMaxX
				}
				if cMaxY > totalMaxY {
					totalMaxY = cMaxY
				}

				clusters = append(clusters, cluster{
					count: cCount,
					minX:  cMinX,
					minY:  cMinY,
					maxX:  cMaxX,
					maxY:  cMaxY,
					cx:    float64(cSumX) / float64(cCount),
					cy:    float64(cSumY) / float64(cCount),
				})
			}
		}

		if totalCount == 0 {
			fmt.Printf("No pixels found matching %s (tol %d)\n", *colorHex, *tolerance)
		} else {
			fmt.Printf("Color %s match: %d pixels (%.2f%% of image)\n", *colorHex, totalCount, float64(totalCount)/float64(w*h)*100.0)
			fmt.Printf("Overall Bounding Box: [%d, %d .. %d, %d] (size %dx%d)\n", totalMinX, totalMinY, totalMaxX, totalMaxY, totalMaxX-totalMinX+1, totalMaxY-totalMinY+1)
			if len(clusters) > 1 {
				fmt.Printf("Detected %d separated regions/clusters:\n", len(clusters))
				for idx, c := range clusters {
					fmt.Printf("  #%d: %d px, centroid: (%.2f, %.2f), bbox: [%d, %d .. %d, %d] (size %dx%d)\n",
						idx+1, c.count, c.cx, c.cy, c.minX, c.minY, c.maxX, c.maxY, c.maxX-c.minX+1, c.maxY-c.minY+1)
				}
			} else {
				fmt.Printf("Centroid: (%.2f, %.2f)\n", float64(totalSumX)/float64(totalCount), float64(totalSumY)/float64(totalCount))
			}
		}
		return
	}

	// General non-transparent inspection
	var opaqueCount int
	var sumX, sumY int
	minX, minY := w, h
	maxX, maxY := 0, 0

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			if a > 0 {
				opaqueCount++
				sumX += x
				sumY += y
				if x < minX {
					minX = x
				}
				if y < minY {
					minY = y
				}
				if x > maxX {
					maxX = x
				}
				if y > maxY {
					maxY = y
				}
			}
		}
	}

	if opaqueCount == 0 {
		fmt.Println("Image is entirely transparent.")
	} else {
		cx := float64(sumX) / float64(opaqueCount)
		cy := float64(sumY) / float64(opaqueCount)
		fmt.Printf("Visible pixels (alpha > 0): %d (%.2f%% of image)\n", opaqueCount, float64(opaqueCount)/float64(w*h)*100.0)
		fmt.Printf("Centroid: (%.2f, %.2f)\n", cx, cy)
		fmt.Printf("Bounding Box: [%d, %d .. %d, %d] (size %dx%d)\n", minX, minY, maxX, maxY, maxX-minX+1, maxY-minY+1)
	}
}

func parseHexColor(s string) (uint8, uint8, uint8, error) {
	s = strings.TrimPrefix(s, "#")
	if len(s) == 3 {
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	if len(s) != 6 {
		return 0, 0, 0, fmt.Errorf("expected 3 or 6 hex digits, got %q", s)
	}
	val, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return 0, 0, 0, err
	}
	return uint8(val >> 16), uint8((val >> 8) & 0xff), uint8(val & 0xff), nil
}

func reorderArgs(args []string, boolFlags map[string]bool) []string {
	var flags []string
	var nonFlags []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flags = append(flags, arg)
			name := strings.TrimLeft(arg, "-")
			if strings.Contains(name, "=") {
				// Flag with inline value like --color=#hex
			} else if !boolFlags[name] && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				flags = append(flags, args[i])
			}
		} else {
			nonFlags = append(nonFlags, arg)
		}
	}
	return append(flags, nonFlags...)
}

