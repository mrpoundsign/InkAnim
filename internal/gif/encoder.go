package gif

import (
	"errors"
	"fmt"
	"image"
	"image/gif"
	"io"
	"os"

	"golang.org/x/image/draw"

	"inkanim/internal/parallel"
)

// EncodeAnimatedGIF processes frame inputs and encodes them into a single animated GIF.
func EncodeAnimatedGIF(frames []FrameInput, opts ExportOptions) (*gif.GIF, error) {
	if len(frames) == 0 {
		return nil, errors.New("no frames provided to encode")
	}

	if opts.PingPong && len(frames) >= 3 {
		bounced := make([]FrameInput, 0, len(frames)*2-2)
		bounced = append(bounced, frames...)
		for i := len(frames) - 2; i >= 1; i-- {
			bounced = append(bounced, frames[i])
		}
		frames = bounced
	}

	processedFrames := make([]*image.RGBA, len(frames))
	delays := make([]int, len(frames))
	disposals := make([]byte, len(frames))

	_ = parallel.Run(len(frames), func(i int) error {
		f := frames[i]
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
		delayUnits := max(durationMs/10,
			// minimum gif delay
			1)
		delays[i] = delayUnits

		// DisposalBackground ensures transparent pixels clear the previous frame properly
		disposals[i] = gif.DisposalBackground
		return nil
	})

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

	// 3. Quantize frames in parallel
	palettedList := make([]*image.Paletted, len(processedFrames))
	_ = parallel.Run(len(processedFrames), func(i int) error {
		palettedList[i] = QuantizeFrame(processedFrames[i], palette, alphaThreshold, opts.Dither)
		return nil
	})

	animGIF := &gif.GIF{
		Image:     palettedList,
		Delay:     delays,
		LoopCount: opts.LoopCount,
		Disposal:  disposals,
	}

	return animGIF, nil
}

type countingWriter struct {
	w io.Writer
	n int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

// WriteGIFToWriter encodes the animation directly to an io.Writer.
func WriteGIFToWriter(w io.Writer, frames []FrameInput, opts ExportOptions) (int64, error) {
	anim, err := EncodeAnimatedGIF(frames, opts)
	if err != nil {
		return 0, err
	}

	cw := &countingWriter{w: w}
	if err := gif.EncodeAll(cw, anim); err != nil {
		return 0, fmt.Errorf("failed to encode gif stream: %w", err)
	}

	return cw.n, nil
}

// WriteGIFToFile encodes the animation directly to a destination file path.
func WriteGIFToFile(outputPath string, frames []FrameInput, opts ExportOptions) (int64, error) {
	f, err := os.Create(outputPath)
	if err != nil {
		return 0, fmt.Errorf("failed to create output file: %w", err)
	}
	defer func() { _ = f.Close() }()

	n, err := WriteGIFToWriter(f, frames, opts)
	if err != nil {
		return n, err
	}
	return n, f.Close()
}
