package svg

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"image"
	"io"
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

		isNonIdentity := math.Abs(scaleW-1.0) > 0.0001 || math.Abs(scaleH-1.0) > 0.0001 ||
			math.Abs(icon.Transform.E) > 0.0001 || math.Abs(icon.Transform.F) > 0.0001

		// Upstream oksvg transforms path coordinates by icon.Transform, but passes Identity to
		// rasterx.GetColorFunctionUS. For userSpaceOnUse gradients, this causes coordinates to remain
		// in document user-space while the rasterizer scans in screen pixel coordinates.
		// Pre-compose icon.Transform onto gradientTransform for all userSpaceOnUse gradients so they
		// scale and align accurately with image resolution (Issue #58).
		if isNonIdentity && bytes.Contains(svgData, []byte("userSpaceOnUse")) {
			scaledSVG := applyViewportTransformToUserGradients(svgData, icon.Transform)
			if scaledIcon, err := oksvg.ReadIconStream(bytes.NewReader(scaledSVG)); err == nil {
				scaledIcon.ViewBox = icon.ViewBox
				scaledIcon.Transform = icon.Transform
				icon = scaledIcon
			}
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

// applyViewportTransformToUserGradients composes the root viewport transformation matrix tm
// onto the gradientTransform of any <linearGradient> or <radialGradient> element that uses
// gradientUnits="userSpaceOnUse". This fixes upstream oksvg/rasterx where userSpaceOnUse gradients
// are evaluated against unscaled document coordinates instead of screen pixel coordinates (Issue #58).
func applyViewportTransformToUserGradients(svgData []byte, tm rasterx.Matrix2D) []byte {
	var buf bytes.Buffer
	dec := xml.NewDecoder(bytes.NewReader(svgData))
	enc := xml.NewEncoder(&buf)

	vpMatrix := Matrix2D{
		A: tm.A,
		B: tm.B,
		C: tm.C,
		D: tm.D,
		E: tm.E,
		F: tm.F,
	}

	for {
		tok, err := dec.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return svgData
		}

		if se, ok := tok.(xml.StartElement); ok {
			name := se.Name.Local
			if name == "linearGradient" || name == "radialGradient" {
				var isUserSpace bool
				var gradTransformIdx = -1
				var existingTransform = IdentityMatrix()

				for i, attr := range se.Attr {
					if attr.Name.Local == "gradientUnits" && strings.TrimSpace(attr.Value) == "userSpaceOnUse" {
						isUserSpace = true
					}
					if attr.Name.Local == "gradientTransform" {
						gradTransformIdx = i
						existingTransform = parseTransform(attr.Value)
					}
				}

				if isUserSpace {
					composed := vpMatrix.Multiply(existingTransform)
					matStr := fmt.Sprintf("matrix(%f %f %f %f %f %f)", composed.A, composed.B, composed.C, composed.D, composed.E, composed.F)
					if gradTransformIdx >= 0 {
						se.Attr[gradTransformIdx].Value = matStr
					} else {
						se.Attr = append(se.Attr, xml.Attr{
							Name:  xml.Name{Local: "gradientTransform"},
							Value: matStr,
						})
					}
				}
			}
			_ = enc.EncodeToken(se)
		} else {
			_ = enc.EncodeToken(tok)
		}
	}
	_ = enc.Flush()
	return buf.Bytes()
}


