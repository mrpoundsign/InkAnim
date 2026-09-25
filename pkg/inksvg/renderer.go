package inksvg

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"image"
	"image/color"
	"io"
	"math"
	"reflect"
	"strings"
	"unsafe"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
	"golang.org/x/image/math/fixed"
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
	scaleFactor := 1.0
	if icon.ViewBox.W > 0 && icon.ViewBox.H > 0 {
		par := parsePreserveAspectRatio(extractRootPreserveAspectRatio(svgData))

		var scaleW, scaleH, offsetX, offsetY float64

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
	clipPaths := extractClipPaths(svgData)
	pathClipIDs := extractPathClipIDs(svgData)
	dropShadows := extractFilterDefs(svgData)
	pathFilterIDs := extractPathFilterIDs(svgData)

	if len(embeddedImages) == 0 && len(clipPaths) == 0 && len(dropShadows) == 0 {
		for _, p := range icon.SVGPaths {
			drawPathTransformed(raster, p, icon.Transform, 1.0, scaleFactor)
		}
	} else {
		clipMasks := make(map[string]*image.RGBA)
		for id, content := range clipPaths {
			mask, err := renderClipMask(content, svgData, widthInt, heightInt, icon.Transform)
			if err == nil {
				clipMasks[id] = mask
			}
		}

		var layerImg *image.RGBA
		var layerRaster *rasterx.Dasher
		if len(clipPaths) > 0 {
			layerImg = image.NewRGBA(image.Rect(0, 0, widthInt, heightInt))
			layerScanner := rasterx.NewScannerGV(widthInt, heightInt, layerImg, layerImg.Bounds())
			layerRaster = rasterx.NewDasher(widthInt, heightInt, layerScanner)
		}

		activeClipID := ""
		imgIdx := 0
		for pathIdx := range icon.SVGPaths {
			targetClip := ""
			if pathIdx < len(pathClipIDs) {
				targetClip = pathClipIDs[pathIdx]
			}

			if targetClip != activeClipID {
				if activeClipID != "" && layerImg != nil {
					compositeLayerWithClip(img, layerImg, activeClipID, clipMasks)
				}
				activeClipID = targetClip
			}

			for imgIdx < len(embeddedImages) && embeddedImages[imgIdx].PathIndex <= pathIdx {
				target := img
				if activeClipID != "" && layerImg != nil {
					target = layerImg
				}
				drawEmbeddedImage(target, embeddedImages[imgIdx], icon.Transform)
				imgIdx++
			}

			targetImg := img
			targetRaster := raster
			if activeClipID != "" && layerRaster != nil {
				targetImg = layerImg
				targetRaster = layerRaster
			}

			if pathIdx < len(pathFilterIDs) {
				fID := pathFilterIDs[pathIdx]
				if filter, ok := dropShadows[fID]; ok {
					renderAndCompositeDropShadow(targetImg, icon.SVGPaths[pathIdx], icon.Transform, filter, widthInt, heightInt, scaleFactor)
				}
			}

			drawPathTransformed(targetRaster, icon.SVGPaths[pathIdx], icon.Transform, 1.0, scaleFactor)
		}

		if activeClipID != "" && layerImg != nil {
			compositeLayerWithClip(img, layerImg, activeClipID, clipMasks)
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

type transformScanner struct {
	rasterx.Scanner
	matrix rasterx.Matrix2D
}

func (ts *transformScanner) Start(a fixed.Point26_6) {
	ts.Scanner.Start(ts.matrix.TFixed(a))
}

func (ts *transformScanner) Line(b fixed.Point26_6) {
	ts.Scanner.Line(ts.matrix.TFixed(b))
}

var (
	mAdderOffset     uintptr
	linerColorOffset uintptr
	offsetsInit      bool
)

func initPathOffsets() {
	if offsetsInit {
		return
	}
	var dummy oksvg.SvgPath
	val := reflect.ValueOf(dummy)
	styleVal := val.FieldByName("PathStyle")
	styleTyp := styleVal.Type()
	if f, ok := styleTyp.FieldByName("mAdder"); ok {
		mAdderOffset = f.Offset
	}
	if f, ok := styleTyp.FieldByName("linerColor"); ok {
		linerColorOffset = f.Offset
	}
	offsetsInit = true
}

func getPathMatrix(p *oksvg.SvgPath) rasterx.Matrix2D {
	initPathOffsets()
	ptr := (*rasterx.MatrixAdder)(unsafe.Pointer(uintptr(unsafe.Pointer(p)) + mAdderOffset))
	return ptr.M
}

func getPathLinerColor(p *oksvg.SvgPath) interface{} {
	initPathOffsets()
	ptr := (*interface{})(unsafe.Pointer(uintptr(unsafe.Pointer(p)) + linerColorOffset))
	return *ptr
}

func setPathLinerColor(p *oksvg.SvgPath, c interface{}) {
	initPathOffsets()
	ptr := (*interface{})(unsafe.Pointer(uintptr(unsafe.Pointer(p)) + linerColorOffset))
	*ptr = c
}

// drawPathTransformed renders an SvgPath, correctly scaling strokes under anisotropic
// (non-uniform) 2D affine transformations where horizontal and vertical stroke widths differ.
func drawPathTransformed(r *rasterx.Dasher, svgp oksvg.SvgPath, t rasterx.Matrix2D, opacity float64, scaleFactor float64) {
	m := getPathMatrix(&svgp)
	linerColor := getPathLinerColor(&svgp)

	totalMatrix := t.Mult(m)
	sx := math.Hypot(totalMatrix.A, totalMatrix.B)
	sy := math.Hypot(totalMatrix.C, totalMatrix.D)
	isAnisotropic := (math.Abs(sx-sy) > 0.001 || math.Abs(totalMatrix.A*totalMatrix.C+totalMatrix.B*totalMatrix.D) > 0.001)

	if !isAnisotropic || linerColor == nil || svgp.LineWidth <= 0 {
		svgp.DrawTransformed(r, opacity, t)
		return
	}

	// 1. Draw fill first if present (by clearing linerColor on a copy)
	fillCopy := svgp
	setPathLinerColor(&fillCopy, nil)
	fillCopy.DrawTransformed(r, opacity, t)

	// 2. Draw stroke in local space through transformScanner
	r.Clear()
	ts := &transformScanner{
		Scanner: r.Scanner,
		matrix:  totalMatrix,
	}
	localDasher := rasterx.NewDasher(10000, 10000, ts)
	lineGap := svgp.LineGap
	if lineGap == nil {
		lineGap = rasterx.FlatGap
	}
	lineCap := svgp.LineCap
	if lineCap == nil {
		lineCap = rasterx.ButtCap
	}
	leadLineCap := lineCap
	if svgp.LeadLineCap != nil {
		leadLineCap = svgp.LeadLineCap
	}
	localLineWidth := svgp.LineWidth
	if scaleFactor > 0 {
		localLineWidth = svgp.LineWidth / scaleFactor
	}
	localDasher.SetStroke(
		fixed.Int26_6(localLineWidth * 64),
		fixed.Int26_6(svgp.MiterLimit * 64),
		leadLineCap, lineCap, lineGap, svgp.LineJoin,
		svgp.Dash, svgp.DashOffset,
	)
	svgp.Path.AddTo(localDasher)

	switch lc := linerColor.(type) {
	case color.Color:
		r.SetColor(rasterx.ApplyOpacity(lc, svgp.LineOpacity*opacity))
	case rasterx.Gradient:
		if lc.Units == rasterx.ObjectBoundingBox {
			fRect := r.GetPathExtent()
			mnx, mny := float64(fRect.Min.X)/64, float64(fRect.Min.Y)/64
			mxx, mxy := float64(fRect.Max.X)/64, float64(fRect.Max.Y)/64
			lc.Bounds.X, lc.Bounds.Y = mnx, mny
			lc.Bounds.W, lc.Bounds.H = mxx-mnx, mxy-mny
		}
		r.SetColor(lc.GetColorFunction(svgp.LineOpacity * opacity))
	}
	r.Draw()
}


