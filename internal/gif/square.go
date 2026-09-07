package gif

import (
	"image"
	"math"

	"golang.org/x/image/draw"
)

// MakeSquare takes an image and centers it on a square canvas with transparent padding.
// If targetSize <= 0, the square size is set to the maximum of width and height: max(W, H).
// The source graphic is centered with transparent borders.
func MakeSquare(src *image.RGBA, targetSize int) *image.RGBA {
	bounds := src.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()

	if srcW <= 0 || srcH <= 0 {
		return src
	}

	maxSide := max(srcH, srcW)

	if targetSize <= 0 {
		targetSize = maxSide
	}

	// Cap to 4096 max (Twitch limit)
	if targetSize > 4096 {
		targetSize = 4096
	}

	dst := image.NewRGBA(image.Rect(0, 0, targetSize, targetSize))
	// By default, dst is filled with transparent pixels (0, 0, 0, 0).

	// Calculate fitted scaled dimensions inside targetSize x targetSize
	scale := float64(targetSize) / float64(maxSide)
	scaledW := int(math.Round(float64(srcW) * scale))
	scaledH := int(math.Round(float64(srcH) * scale))

	if scaledW < 1 {
		scaledW = 1
	}
	if scaledH < 1 {
		scaledH = 1
	}

	// Calculate center offset
	offsetX := (targetSize - scaledW) / 2
	offsetY := (targetSize - scaledH) / 2

	dstRect := image.Rect(offsetX, offsetY, offsetX+scaledW, offsetY+scaledH)

	if scaledW == srcW && scaledH == srcH {
		// No scaling needed, just direct draw at offset
		draw.Draw(dst, dstRect, src, bounds.Min, draw.Src)
	} else {
		// High quality scaling
		draw.BiLinear.Scale(dst, dstRect, src, bounds, draw.Src, nil)
	}

	return dst
}
