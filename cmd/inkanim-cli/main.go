package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"inkanim/internal/app"
	"inkanim/internal/gif"
	"inkanim/internal/svg"
)

func main() {
	inputFile := flag.String("i", "", "Input Inkscape SVG file path (required)")
	outputFile := flag.String("o", "", "Output animated GIF file path (default: input with .gif extension)")
	modeStr := flag.String("mode", "layers", "Frame extraction mode: 'layers' or 'pages'")
	square := flag.Bool("square", true, "Export Square: center graphic on max(width, height) with transparent padding")
	squareSize := flag.Int("size", 0, "Target square size (e.g. 512, max 4096). 0 uses max(width, height)")
	width := flag.Int("width", 0, "Custom target width (if not using square mode)")
	height := flag.Int("height", 0, "Custom target height (if not using square mode)")
	fps := flag.Int("fps", 10, "Frames per second (playback speed)")
	colors := flag.Int("colors", 256, "Max palette colors (2-256)")
	dither := flag.Bool("dither", false, "Apply Floyd-Steinberg dithering")
	checkTwitch := flag.Bool("check-twitch", true, "Validate output against Twitch animated emote specifications")

	flag.Parse()

	if *inputFile == "" {
		fmt.Println("InkAnim CLI — Convert Inkscape SVG layers/pages to a single animated GIF")
		fmt.Println("\nUsage:")
		flag.PrintDefaults()
		os.Exit(1)
	}

	outPath := *outputFile
	if outPath == "" {
		ext := filepath.Ext(*inputFile)
		outPath = strings.TrimSuffix(*inputFile, ext) + ".gif"
	}

	sess := app.NewSession()
	fmt.Printf("Loading SVG: %s ...\n", *inputFile)
	if err := sess.LoadSVG(*inputFile); err != nil {
		fmt.Fprintf(os.Stderr, "Error loading SVG: %v\n", err)
		os.Exit(1)
	}

	if strings.ToLower(*modeStr) == "pages" {
		if err := sess.SetMode(svg.ModePages); err != nil {
			fmt.Fprintf(os.Stderr, "Error setting pages mode: %v\n", err)
			os.Exit(1)
		}
	} else {
		if err := sess.SetMode(svg.ModeLayers); err != nil {
			fmt.Fprintf(os.Stderr, "Error setting layers mode: %v\n", err)
			os.Exit(1)
		}
	}

	frameCount := len(sess.RenderedFrames)
	fmt.Printf("Detected %d animation frames in %s mode.\n", frameCount, sess.CurrentMode)
	if frameCount == 0 {
		fmt.Fprintf(os.Stderr, "Error: No animation frames found in mode '%s'.\n", sess.CurrentMode)
		os.Exit(1)
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
	}
	sess.ExportOptions = opts

	fmt.Printf("Encoding single animated GIF to: %s ...\n", outPath)
	sizeBytes, err := sess.ExportGIF(outPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error exporting GIF: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Success! Exported %s (%0.2f KB)\n", outPath, float64(sizeBytes)/1024.0)

	if *checkTwitch {
		totalDurationMs := frameCount * frameDelayMs
		exportW := int(sess.Document.Width)
		exportH := int(sess.Document.Height)
		if *square {
			maxSide := exportW
			if exportH > maxSide {
				maxSide = exportH
			}
			if *squareSize > 0 {
				maxSide = *squareSize
			}
			exportW, exportH = maxSide, maxSide
		} else if *width > 0 && *height > 0 {
			exportW, exportH = *width, *height
		}

		res := gif.ValidateTwitchEmote(frameCount, totalDurationMs, exportW, exportH, sizeBytes)
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
}
