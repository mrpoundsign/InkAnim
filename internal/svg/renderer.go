package svg

import (
	"bytes"
	"fmt"
	"image"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
)

// RenderSVGToRGBA renders an SVG byte buffer to an in-memory RGBA image at the requested width and height.
// If width or height is <= 0, the native SVG dimensions are used.
func RenderSVGToRGBA(svgData []byte, targetW, targetH int) (*image.RGBA, error) {
	icon, err := oksvg.ReadIconStream(bytes.NewReader(svgData))
	if err != nil {
		return nil, fmt.Errorf("failed to parse vector icon: %w", err)
	}

	w := float64(targetW)
	h := float64(targetH)

	if w <= 0 {
		w = icon.ViewBox.W
	}
	if h <= 0 {
		h = icon.ViewBox.H
	}

	if w <= 0 {
		w = 512
	}
	if h <= 0 {
		h = 512
	}

	icon.SetTarget(0, 0, w, h)

	widthInt := int(w)
	heightInt := int(h)

	img := image.NewRGBA(image.Rect(0, 0, widthInt, heightInt))
	scanner := rasterx.NewScannerGV(widthInt, heightInt, img, img.Bounds())
	raster := rasterx.NewDasher(widthInt, heightInt, scanner)

	icon.Draw(raster, 1.0)

	return img, nil
}
