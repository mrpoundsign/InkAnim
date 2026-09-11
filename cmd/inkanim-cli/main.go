package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"inkanim/internal/app"
	"inkanim/internal/gif"
	"inkanim/internal/prof"
	"inkanim/pkg/inksvg"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	showVersion := flag.Bool("v", false, "Show version information")
	inputFile := flag.String("i", "", "Input Inkscape SVG file path (required)")
	outputFile := flag.String("o", "", "Output animated GIF file path (default: input with .gif extension)")
	modeStr := flag.String("mode", "layers", "Frame extraction mode: 'layers' (default; 'pages' is deprecated)")
	square := flag.Bool("square", true, "Export Square: center graphic on max(width, height) with transparent padding")
	squareSize := flag.Int("size", 0, "Target square size (e.g. 512, max 4096). 0 uses max(width, height)")
	width := flag.Int("width", 0, "Custom target width (if not using square mode)")
	height := flag.Int("height", 0, "Custom target height (if not using square mode)")
	fps := flag.Int("fps", 10, "Frames per second (playback speed)")
	colors := flag.Int("colors", 256, "Max palette colors (2-256)")
	dither := flag.Bool("dither", false, "Apply Floyd-Steinberg dithering")
	pingpong := flag.Bool("pingpong", false, "Enable Ping-Pong (bounce/reverse) loop playback")
	checkTwitch := flag.Bool("check-twitch", true, "Validate output against Twitch animated emote specifications")
	cpuprofile := flag.String("cpuprofile", "", "Write cpu profile to file")
	memprofile := flag.String("memprofile", "", "Write memory profile to file")
	pprofAddr := flag.String("pprof", "", "Serve live pprof HTTP endpoint at address (e.g. :6060)")

	flag.Parse()

	if *showVersion {
		fmt.Printf("inkanim-cli version %s (commit %s, built %s)\n", version, commit, date)
		return nil
	}

	if *inputFile == "" {
		fmt.Println("InkAnim CLI — Convert Inkscape SVG layers to a single animated GIF")
		fmt.Println("\nUsage:")
		flag.PrintDefaults()
		return errors.New("missing required input file (-i)")
	}

	cleanup, err := prof.SetupProfiler(*cpuprofile, *memprofile, *pprofAddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Profiler error: %v\n", err)
	} else if cleanup != nil {
		defer cleanup()
	}

	outPath := *outputFile
	if outPath == "" {
		ext := filepath.Ext(*inputFile)
		outPath = strings.TrimSuffix(*inputFile, ext) + ".gif"
	}

	sess := app.NewSession()
	fmt.Printf("Loading SVG: %s ...\n", *inputFile)
	if err := sess.LoadSVG(*inputFile); err != nil {
		return fmt.Errorf("loading SVG: %w", err)
	}

	if strings.ToLower(*modeStr) == "pages" {
		fmt.Println("Warning: -mode pages is deprecated; animation frames are extracted from layers. Pages serve as artboard crop boundaries.")
	}
	if err := sess.SetMode(inksvg.ModeLayers); err != nil {
		return fmt.Errorf("setting layers mode: %w", err)
	}

	frameCount := len(sess.RenderedFrames)
	fmt.Printf("Detected %d animation frames in %s mode.\n", frameCount, sess.CurrentMode)
	if frameCount == 0 {
		return fmt.Errorf("no animation frames found in mode '%s'", sess.CurrentMode)
	}

	frameDelayMs := 100
	if *fps > 0 {
		frameDelayMs = 1000 / *fps
	}

	opts := gif.ExportOptions{
		TargetWidth:       *width,
		TargetHeight:      *height,
		ExportSquare:      *square,
		SquareSize:        *squareSize,
		DefaultDurationMs: frameDelayMs,
		LoopCount:         0,
		NumColors:         *colors,
		AlphaThreshold:    128,
		Dither:            *dither,
		PingPong:          *pingpong,
	}
	sess.ExportOptions = opts

	fmt.Printf("Encoding single animated GIF to: %s ...\n", outPath)
	sizeBytes, err := sess.ExportGIF(outPath)
	if err != nil {
		return fmt.Errorf("exporting GIF: %w", err)
	}

	fmt.Printf("Success! Exported %s (%0.2f KB)\n", outPath, float64(sizeBytes)/1024.0)

	if *checkTwitch {
		effectiveFrames := frameCount
		if *pingpong && frameCount >= 3 {
			effectiveFrames = frameCount*2 - 2
		}
		totalDurationMs := effectiveFrames * frameDelayMs
		exportW := int(sess.Document.Width)
		exportH := int(sess.Document.Height)
		if *square {
			maxSide := max(exportH, exportW)
			if *squareSize > 0 {
				maxSide = *squareSize
			}
			exportW, exportH = maxSide, maxSide
		} else if *width > 0 && *height > 0 {
			exportW, exportH = *width, *height
		}

		res := gif.ValidateTwitchEmote(effectiveFrames, totalDurationMs, exportW, exportH)
		fmt.Println("\n--- Twitch Emote Compatibility Check ---")
		if res.IsValid && len(res.Warnings) == 0 {
			fmt.Println("[OK] Fully compatible with Twitch Animated Emote requirements!")
		} else {
			for _, errStr := range res.Errors {
				fmt.Printf("[FAIL] %s\n", errStr)
			}
			for _, warnStr := range res.Warnings {
				fmt.Printf("[WARN] %s\n", warnStr)
			}
		}
	}
	return nil
}
