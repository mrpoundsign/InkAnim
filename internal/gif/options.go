package gif

import (
	"image"
)

// ExportOptions configures the animated GIF encoding pipeline.
type ExportOptions struct {
	TargetWidth      int   `json:"targetWidth"`      // 0 to preserve native/source width
	TargetHeight     int   `json:"targetHeight"`     // 0 to preserve native/source height
	ExportSquare     bool  `json:"exportSquare"`     // center on max(width, height) with transparent padding
	SquareSize       int   `json:"squareSize"`       // target square resolution (e.g. 512, up to 4096). 0 = source max side
	DefaultDurationMs int  `json:"defaultDurationMs"` // fallback frame delay in ms (e.g. 100ms = 10fps)
	LoopCount        int   `json:"loopCount"`        // 0 = infinite loop
	NumColors        int   `json:"numColors"`        // palette size (up to 256)
	AlphaThreshold   uint8 `json:"alphaThreshold"`   // pixels with alpha < threshold become transparent (default 128)
	Dither           bool  `json:"dither"`           // whether to apply Floyd-Steinberg dithering
}

// DefaultOptions provides standard defaults tailored for high-quality animated emotes.
func DefaultOptions() ExportOptions {
	return ExportOptions{
		TargetWidth:       0,
		TargetHeight:      0,
		ExportSquare:      true,
		SquareSize:        0, // automatically max(w, h)
		DefaultDurationMs: 100,
		LoopCount:         0, // infinite
		NumColors:         256,
		AlphaThreshold:    128,
		Dither:            false,
	}
}

// FrameInput represents an RGBA frame with individual duration.
type FrameInput struct {
	Index      int
	Label      string
	Image      *image.RGBA
	DurationMs int
}
