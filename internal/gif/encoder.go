package gif

import (
	"bytes"
	"fmt"
	"image"
	"image/gif"
	"io"
	"os"

	"golang.org/x/image/draw"
)

// EncodeAnimatedGIF processes frame inputs and encodes them into a single animated GIF.
func EncodeAnimatedGIF(frames []FrameInput, opts ExportOptions) (*gif.GIF, error) {
	if len(frames) == 0 {
		return nil, fmt.Errorf("no frames provided to encode")
	}

	processedFrames := make([]*image.RGBA, len(frames))
	delays := make([]int, len(frames))
	disposals := make([]byte, len(frames))

	for i, f := range frames {
		frameImg := f.Image

		// 1. If ExportSquare is enabled, center source inside square canvas
		if opts.ExportSquare {
			frameImg = MakeSquare(frameImg, opts.SquareSize)
		} else if opts.TargetWidth > 0 && opts.TargetHeight > 0 {
			// Custom resize if explicitly specified and different from source bounds
			b := frameImg.Bounds()
			if b.Dx() != opts.TargetWidth || b.Dy() != opts.TargetHeight {
				scaled := image.NewRGBA(image.Rect(0, 0, opts.TargetWidth, opts.TargetHeight))
				draw.BiLinear.Scale(scaled, scaled.Bounds(), frameImg, b, draw.Src, nil)
				frameImg = scaled
			}
		}

		processedFrames[i] = frameImg

		// Calculate delay in 100ths of a second (1 unit = 10ms)
		durationMs := f.DurationMs
		if durationMs <= 0 {
			durationMs = opts.DefaultDurationMs
		}
		if durationMs <= 0 {
			durationMs = 100 // fallback 10 fps
		}
		delayUnits := durationMs / 10
		if delayUnits < 1 {
			delayUnits = 1 // minimum gif delay
		}
		delays[i] = delayUnits

		// DisposalBackground ensures transparent pixels clear the previous frame properly
		disposals[i] = gif.DisposalBackground
	}

	// 2. Generate unified palette
	numColors := opts.NumColors
	if numColors <= 0 || numColors > 256 {
		numColors = 256
	}
	alphaThreshold := opts.AlphaThreshold
	if alphaThreshold == 0 {
		alphaThreshold = 128
	}

	palette := GeneratePalette(processedFrames, numColors, alphaThreshold)

	// 3. Quantize frames
	palettedList := make([]*image.Paletted, len(processedFrames))
	for i, frameImg := range processedFrames {
		palettedList[i] = QuantizeFrame(frameImg, palette, alphaThreshold, opts.Dither)
	}

	animGIF := &gif.GIF{
		Image:     palettedList,
		Delay:     delays,
		LoopCount: opts.LoopCount,
		Disposal:  disposals,
	}

	return animGIF, nil
}

// WriteGIFToFile encodes the animation directly to a destination file path.
func WriteGIFToFile(outputPath string, frames []FrameInput, opts ExportOptions) (int64, error) {
	anim, err := EncodeAnimatedGIF(frames, opts)
	if err != nil {
		return 0, err
	}

	f, err := os.Create(outputPath)
	if err != nil {
		return 0, fmt.Errorf("failed to create output file: %w", err)
	}
	defer f.Close()

	var buf bytes.Buffer
	mw := io.MultiWriter(f, &buf)

	if err := gif.EncodeAll(mw, anim); err != nil {
		return 0, fmt.Errorf("failed to encode gif stream: %w", err)
	}

	return int64(buf.Len()), nil
}
