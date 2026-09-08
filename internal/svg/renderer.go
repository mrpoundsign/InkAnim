package svg

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"image"
	"math"
	"strings"

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
		par := parsePreserveAspectRatio(extractRootPreserveAspectRatio(svgData))

		var scaleW, scaleH, offsetX, offsetY, scaleFactor float64

		if par.Align == "none" {
			scaleW = w / icon.ViewBox.W
			scaleH = h / icon.ViewBox.H
			scaleFactor = math.Sqrt(scaleW * scaleH)
		} else {
			scaleX := w / icon.ViewBox.W
			scaleY := h / icon.ViewBox.H
			var s float64
			if par.MeetOrSlice == "slice" {
				s = math.Max(scaleX, scaleY)
			} else {
				s = math.Min(scaleX, scaleY)
			}
			scaleW = s
			scaleH = s
			scaleFactor = s

			deltaX := w - icon.ViewBox.W*s
			deltaY := h - icon.ViewBox.H*s

			switch {
			case strings.HasPrefix(par.Align, "xMin"):
				offsetX = 0
			case strings.HasPrefix(par.Align, "xMax"):
				offsetX = deltaX
			default: // xMid
				offsetX = deltaX / 2.0
			}

			switch {
			case strings.HasSuffix(par.Align, "YMin"):
				offsetY = 0
			case strings.HasSuffix(par.Align, "YMax"):
				offsetY = deltaY
			default: // YMid
				offsetY = deltaY / 2.0
			}
		}

		icon.Transform = rasterx.Matrix2D{
			A: scaleW,
			D: scaleH,
			E: -icon.ViewBox.X*scaleW + offsetX,
			F: -icon.ViewBox.Y*scaleH + offsetY,
		}

		// Scale each path's LineWidth proportionally so strokes scale with image resolution.
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

// PreserveAspectRatio defines viewBox aspect ratio preservation rules.
type PreserveAspectRatio struct {
	Align       string // "none", "xMinYMin", "xMidYMin", "xMaxYMin", "xMinYMid", "xMidYMid", etc.
	MeetOrSlice string // "meet" or "slice"
}

// parsePreserveAspectRatio parses standard SVG preserveAspectRatio attribute syntax.
// Defaults to xMidYMid meet if empty or omitted per SVG specification.
func parsePreserveAspectRatio(attr string) PreserveAspectRatio {
	attr = strings.TrimSpace(attr)
	if attr == "" {
		return PreserveAspectRatio{
			Align:       "xMidYMid",
			MeetOrSlice: "meet",
		}
	}

	parts := strings.Fields(attr)
	idx := 0
	if parts[0] == "defer" {
		idx++
		if idx >= len(parts) {
			return PreserveAspectRatio{
				Align:       "xMidYMid",
				MeetOrSlice: "meet",
			}
		}
	}

	align := parts[idx]
	idx++

	meetOrSlice := "meet"
	if idx < len(parts) && parts[idx] == "slice" {
		meetOrSlice = "slice"
	}

	return PreserveAspectRatio{
		Align:       align,
		MeetOrSlice: meetOrSlice,
	}
}

// extractRootPreserveAspectRatio extracts the preserveAspectRatio attribute from the root <svg> tag.
func extractRootPreserveAspectRatio(svgData []byte) string {
	decoder := xml.NewDecoder(bytes.NewReader(svgData))
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		if start, ok := tok.(xml.StartElement); ok {
			if start.Name.Local == "svg" {
				for _, a := range start.Attr {
					if a.Name.Local == "preserveAspectRatio" {
						return a.Value
					}
				}
				return ""
			}
		}
	}
	return ""
}

