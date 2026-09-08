package svg

import (
	"bytes"
	"fmt"
	"image"
	"math"

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

	// Fix upstream oksvg bug: icon.SetTarget translates by (x-ViewBox.X, y-ViewBox.Y) BEFORE
	// scaling, but rasterx matrix multiplication applies translation without scaling it.
	// This results in x' = x*scale + (targetX - ViewBox.X) instead of (x - ViewBox.X)*scale + targetX.
	// For any SVG with non-zero ViewBox.X or ViewBox.Y (e.g. Drawing boundary or multi-page offsets),
	// this caused an unintended shift of ViewBox * (scale - 1), shoving the drawing down/right
	// and clipping shapes against the bottom/right canvas edges.
	if icon.ViewBox.W > 0 && icon.ViewBox.H > 0 {
		scaleW := w / icon.ViewBox.W
		scaleH := h / icon.ViewBox.H
		icon.Transform = rasterx.Matrix2D{
			A: scaleW,
			D: scaleH,
			E: -icon.ViewBox.X * scaleW,
			F: -icon.ViewBox.Y * scaleH,
		}

		// oksvg draws strokes using raw LineWidth in user units without scaling by the viewBox-to-canvas
		// transformation matrix. Scale each path's LineWidth proportionally so strokes scale with image resolution.
		scaleFactor := math.Sqrt(scaleW * scaleH)
		for i := range icon.SVGPaths {
			icon.SVGPaths[i].LineWidth *= scaleFactor
		}
	}

	widthInt := int(w)
	heightInt := int(h)

	img := image.NewRGBA(image.Rect(0, 0, widthInt, heightInt))
	scanner := rasterx.NewScannerGV(widthInt, heightInt, img, img.Bounds())
	raster := rasterx.NewDasher(widthInt, heightInt, scanner)

	embeddedImages := extractEmbeddedImages(svgData)
	if len(embeddedImages) == 0 {
		icon.Draw(raster, 1.0)
	} else {
		imgIdx := 0
		for pathIdx := range icon.SVGPaths {
			for imgIdx < len(embeddedImages) && embeddedImages[imgIdx].PathIndex <= pathIdx {
				drawEmbeddedImage(img, embeddedImages[imgIdx], icon.Transform)
				imgIdx++
			}
			icon.SVGPaths[pathIdx].DrawTransformed(raster, 1.0, icon.Transform)
		}
		for imgIdx < len(embeddedImages) {
			drawEmbeddedImage(img, embeddedImages[imgIdx], icon.Transform)
			imgIdx++
		}
	}

	return img, nil
}
