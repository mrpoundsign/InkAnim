package inksvg

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"image"
	"io"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
	"golang.org/x/image/math/fixed"
)

// ParseSVG parses an Inkscape SVG document from raw bytes.
func ParseSVG(data []byte) (*SVGDocument, error) {
	// Preprocess SVG: apply in-memory fillet_chamfer LPEs and desugar paint-order
	processedData, err := PreprocessSVG(data)
	if err != nil {
		processedData = data
	}

	doc := &SVGDocument{
		RawContent:   processedData,
		DefaultMode:  ModeTimeline,
		Gradients:    make(map[string]SVGGradient),
		ElementRects: make(map[string]Rect),
	}

	decoder := xml.NewDecoder(bytes.NewReader(processedData))
	var inSVGTag bool
	var pageIdx int
	var activeGroups []string
	var curGrad *SVGGradient
	gradHrefs := make(map[string]string)

	for {
		token, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("xml decode error: %w", err)
		}

		if elem, ok := token.(xml.StartElement); ok {
			name := elem.Name.Local

			if name == "svg" && !inSVGTag {
				inSVGTag = true
				parseSVGAttributes(elem.Attr, doc)
			}

			if name == "g" {
				var id, label string
				for _, attr := range elem.Attr {
					if attr.Name.Local == "id" {
						id = attr.Value
					}
					if attr.Name.Local == "label" {
						label = attr.Value
					}
				}

				if cfg, ok := parseMotionConfig(label); ok && cfg.HasParallax {
					doc.MotionPaths = append(doc.MotionPaths, MotionPath{
						ID:      id + "_dist",
						GroupID: id,
						Config:  cfg,
					})
				}
				activeGroups = append(activeGroups, id)
			}

			// Check for IAMS Motion Path or marker element: <path inkscape:label="..." d="...">
			if name == "path" || name == "circle" || name == "rect" || name == "ellipse" || name == "line" || name == "polygon" || name == "polyline" {
				var id, label, d, fill, style string
				for _, attr := range elem.Attr {
					switch attr.Name.Local {
					case "id":
						id = attr.Value
					case "label":
						label = attr.Value
					case "d":
						d = attr.Value
					case "fill":
						fill = attr.Value
					case "style":
						style = attr.Value
					}
				}
				if cfg, ok := parseMotionConfig(label); ok {
					var fillURL string
					if cfg.IsColor {
						gradRef := fill
						if gradRef == "" || !strings.Contains(gradRef, "url(") {
							gradRef = extractCSSProp(style, "fill")
						}
						if strings.Contains(gradRef, "url(") {
							start := strings.Index(gradRef, "url(") + 4
							end := strings.Index(gradRef[start:], ")")
							if end != -1 {
								ref := strings.TrimSpace(gradRef[start : start+end])
								ref = strings.Trim(ref, `"'#`)
								fillURL = ref
							}
						}
					}
					var bounds Rect
					switch name {
					case "rect":
						xVal := parseDimension(extractAttr(elem.Attr, "x"))
						yVal := parseDimension(extractAttr(elem.Attr, "y"))
						wVal := parseDimension(extractAttr(elem.Attr, "width"))
						hVal := parseDimension(extractAttr(elem.Attr, "height"))
						bounds = Rect{X: xVal, Y: yVal, Width: wVal, Height: hVal}
					case "circle":
						cxVal := parseDimension(extractAttr(elem.Attr, "cx"))
						cyVal := parseDimension(extractAttr(elem.Attr, "cy"))
						rVal := parseDimension(extractAttr(elem.Attr, "r"))
						bounds = Rect{X: cxVal - rVal, Y: cyVal - rVal, Width: 2 * rVal, Height: 2 * rVal}
					case "ellipse":
						cxVal := parseDimension(extractAttr(elem.Attr, "cx"))
						cyVal := parseDimension(extractAttr(elem.Attr, "cy"))
						rxVal := parseDimension(extractAttr(elem.Attr, "rx"))
						ryVal := parseDimension(extractAttr(elem.Attr, "ry"))
						bounds = Rect{X: cxVal - rxVal, Y: cyVal - ryVal, Width: 2 * rxVal, Height: 2 * ryVal}
					case "line":
						x1Val := parseDimension(extractAttr(elem.Attr, "x1"))
						y1Val := parseDimension(extractAttr(elem.Attr, "y1"))
						x2Val := parseDimension(extractAttr(elem.Attr, "x2"))
						y2Val := parseDimension(extractAttr(elem.Attr, "y2"))
						bounds = Rect{X: math.Min(x1Val, x2Val), Y: math.Min(y1Val, y2Val), Width: math.Abs(x2Val - x1Val), Height: math.Abs(y2Val - y1Val)}
					}
					if trStr := extractAttr(elem.Attr, "transform"); trStr != "" && (bounds.Width > 0 || bounds.Height > 0) {
						m := parseTransform(trStr)
						if m != IdentityMatrix() {
							pts := [4][2]float64{
								{bounds.X, bounds.Y},
								{bounds.X + bounds.Width, bounds.Y},
								{bounds.X + bounds.Width, bounds.Y + bounds.Height},
								{bounds.X, bounds.Y + bounds.Height},
							}
							x0, y0 := m.Transform(pts[0][0], pts[0][1])
							minX, maxX := x0, x0
							minY, maxY := y0, y0
							for i := 1; i < 4; i++ {
								xi, yi := m.Transform(pts[i][0], pts[i][1])
								if xi < minX {
									minX = xi
								}
								if xi > maxX {
									maxX = xi
								}
								if yi < minY {
									minY = yi
								}
								if yi > maxY {
									maxY = yi
								}
							}
							bounds = Rect{X: minX, Y: minY, Width: maxX - minX, Height: maxY - minY}
						}
					}
					if id == "" && cfg.IsColor {
						id = fmt.Sprintf("color_mp_%d", len(doc.MotionPaths)+1)
					}
					mp := MotionPath{
						ID:       id,
						PathData: d,
						Config:   cfg,
						FillURL:  fillURL,
						Bounds:   bounds,
					}
					if len(activeGroups) > 0 {
						mp.GroupID = activeGroups[len(activeGroups)-1]
					}
					if cfg.IsCamera {
						doc.CameraPath = &mp
					}
					if len(activeGroups) > 0 {
						doc.MotionPaths = append(doc.MotionPaths, mp)
					}
				}
			}

			// Check for linearGradient / radialGradient definitions and stops
			if name == "linearGradient" || name == "radialGradient" {
				var id, href string
				for _, attr := range elem.Attr {
					switch attr.Name.Local {
					case "id":
						id = attr.Value
					case "href":
						href = strings.TrimPrefix(attr.Value, "#")
					}
				}
				curGrad = &SVGGradient{
					ID: id,
				}
				if href != "" {
					gradHrefs[id] = href
				}
			}
			if name == "stop" && curGrad != nil {
				var offsetStr, stopColor, stopOpacityStr, styleStr string
				for _, attr := range elem.Attr {
					switch attr.Name.Local {
					case "offset":
						offsetStr = attr.Value
					case "stop-color":
						stopColor = attr.Value
					case "stop-opacity":
						stopOpacityStr = attr.Value
					case "style":
						styleStr = attr.Value
					}
				}
				if stopColor == "" {
					stopColor = extractCSSProp(styleStr, "stop-color")
				}
				if stopOpacityStr == "" {
					stopOpacityStr = extractCSSProp(styleStr, "stop-opacity")
				}
				offset := 0.0
				offsetStr = strings.TrimSpace(offsetStr)
				if before, ok0 := strings.CutSuffix(offsetStr, "%"); ok0 {
					if val, err := strconv.ParseFloat(before, 64); err == nil {
						offset = val / 100.0
					}
				} else if val, err := strconv.ParseFloat(offsetStr, 64); err == nil {
					offset = val
				}
				opacity := 1.0
				if stopOpacityStr != "" {
					if val, err := strconv.ParseFloat(strings.TrimSpace(stopOpacityStr), 64); err == nil {
						opacity = val
					}
				}
				if stopColor == "" {
					stopColor = "#000000"
				}
				curGrad.Stops = append(curGrad.Stops, GradientStop{
					Offset:  offset,
					Color:   stopColor,
					Opacity: opacity,
				})
			}

			// Check for Inkscape 1.2+ page: <inkscape:page ...>
			if name == "page" && elem.Name.Space == "http://www.inkscape.org/namespaces/inkscape" || name == "page" {
				var id, label string
				var x, y, w, h float64
				for _, attr := range elem.Attr {
					switch attr.Name.Local {
					case "id":
						id = attr.Value
					case "label":
						label = attr.Value
					case "x":
						x = parseDimension(attr.Value)
					case "y":
						y = parseDimension(attr.Value)
					case "width":
						w = parseDimension(attr.Value)
					case "height":
						h = parseDimension(attr.Value)
					}
				}

				if w > 0 && h > 0 {
					if label == "" {
						label = fmt.Sprintf("Page %d", pageIdx+1)
					}
					page := Page{
						ID:         id,
						Label:      label,
						Index:      pageIdx,
						X:          x,
						Y:          y,
						Width:      w,
						Height:     h,
						IsActive:   true,
						DurationMs: 100,
					}
					doc.Pages = append(doc.Pages, page)
					pageIdx++
				}
			}
		}

		if endElem, ok := token.(xml.EndElement); ok {
			if endElem.Name.Local == "g" && len(activeGroups) > 0 {
				activeGroups = activeGroups[:len(activeGroups)-1]
			}
			if endElem.Name.Local == "linearGradient" || endElem.Name.Local == "radialGradient" {
				if curGrad != nil {
					if curGrad.ID != "" {
						doc.Gradients[curGrad.ID] = *curGrad
					}
					curGrad = nil
				}
			}
		}
	}

	// Resolve gradient stop inheritance (for gradients linking to template gradients)
	for id, href := range gradHrefs {
		grad, exists := doc.Gradients[id]
		if exists && len(grad.Stops) == 0 {
			targetHref := href
			for range 10 {
				parent, ok := doc.Gradients[targetHref]
				if !ok {
					break
				}
				if len(parent.Stops) > 0 {
					grad.Stops = make([]GradientStop, len(parent.Stops))
					copy(grad.Stops, parent.Stops)
					doc.Gradients[id] = grad
					break
				}
				targetHref = gradHrefs[targetHref]
				if targetHref == "" {
					break
				}
			}
		}
	}

	// Ensure gradient stops are sorted by offset
	for id, grad := range doc.Gradients {
		sort.SliceStable(grad.Stops, func(i, j int) bool {
			return grad.Stops[i].Offset < grad.Stops[j].Offset
		})
		doc.Gradients[id] = grad
	}

	// Default mode is ModeTimeline
	doc.DefaultMode = ModeTimeline

	// Check if this is a timeline animation
	maxFrame := 0
	for _, mp := range doc.MotionPaths {
		if mp.Config.EndFrame > maxFrame {
			maxFrame = mp.Config.EndFrame
		}
	}
	if doc.CameraPath != nil && doc.CameraPath.Config.EndFrame > maxFrame {
		maxFrame = doc.CameraPath.Config.EndFrame
	}
	if maxFrame == 0 && len(doc.MotionPaths) > 0 {
		maxFrame = 15
	}

	if maxFrame > 0 {
		doc.Layers = make([]Layer, maxFrame)
		for i := 0; i < maxFrame; i++ {
			doc.Layers[i] = Layer{
				ID:         fmt.Sprintf("timeline_frame_%d", i+1),
				Label:      fmt.Sprintf("Frame %d", i+1),
				Index:      i,
				Visible:    true,
				IsActive:   true,
				DurationMs: 100,
			}
		}
	} else {
		// Single static frame representing the entire document
		doc.Layers = []Layer{
			{
				ID:         "timeline_frame_1",
				Label:      "Frame 1",
				Index:      0,
				Visible:    true,
				IsActive:   true,
				DurationMs: 100,
			},
		}
	}

	doc.DrawingRect = ComputeDrawingRect(processedData)

	if tpl, err := parseTimelineTemplate(processedData); err == nil {
		doc.timelineTpl = tpl
	}

	return doc, nil
}

type bboxScanner struct {
	minX, minY float64
	maxX, maxY float64
	found      bool
}

func (s *bboxScanner) addPoint(p fixed.Point26_6) {
	x := float64(p.X) / 64.0
	y := float64(p.Y) / 64.0
	if !s.found {
		s.minX, s.maxX = x, x
		s.minY, s.maxY = y, y
		s.found = true
		return
	}
	if x < s.minX {
		s.minX = x
	}
	if x > s.maxX {
		s.maxX = x
	}
	if y < s.minY {
		s.minY = y
	}
	if y > s.maxY {
		s.maxY = y
	}
}

func (s *bboxScanner) Start(a fixed.Point26_6) {
	s.addPoint(a)
}

func (s *bboxScanner) Line(b fixed.Point26_6) {
	s.addPoint(b)
}

func (s *bboxScanner) Draw() {}

func (s *bboxScanner) GetPathExtent() fixed.Rectangle26_6 {
	if !s.found {
		return fixed.Rectangle26_6{}
	}
	return fixed.Rectangle26_6{
		Min: fixed.Point26_6{X: fixed.Int26_6(s.minX * 64), Y: fixed.Int26_6(s.minY * 64)},
		Max: fixed.Point26_6{X: fixed.Int26_6(s.maxX * 64), Y: fixed.Int26_6(s.maxY * 64)},
	}
}

func (s *bboxScanner) SetBounds(w, h int)                {}
func (s *bboxScanner) SetColor(color any)                {}
func (s *bboxScanner) SetWinding(useNonZeroWinding bool) {}
func (s *bboxScanner) Clear()                            {}
func (s *bboxScanner) SetClip(rect image.Rectangle)      {}

// ComputeElementRect extracts the SVG subtree of the element with the given ID
// (preserving its ancestor groups and transforms, but excluding motion path guides),
// wraps it in a root SVG with the document's viewBox, and computes its visual bounding box.
func ComputeElementRect(data []byte, elementID string) (Rect, bool) {
	if len(data) == 0 || elementID == "" {
		return Rect{}, false
	}

	decoder := xml.NewDecoder(bytes.NewReader(data))
	var rootElem xml.StartElement
	var rootFound bool

	var ancestorStack []xml.StartElement
	var targetTokens []xml.Token
	var inTarget bool
	var targetDepth int

	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}

		switch elem := tok.(type) {
		case xml.StartElement:
			if !rootFound && elem.Name.Local == "svg" {
				rootFound = true
				rootElem = elem.Copy()
			}

			if inTarget {
				targetDepth++
				// Skip internal motion path guides so they don't expand the element's bounding box
				if elem.Name.Local == "path" || elem.Name.Local == "circle" || elem.Name.Local == "rect" || elem.Name.Local == "ellipse" || elem.Name.Local == "line" {
					for _, attr := range elem.Attr {
						if attr.Name.Local == "label" {
							if _, ok := parseMotionConfig(attr.Value); ok {
								_ = decoder.Skip()
								targetDepth--
								goto nextToken
							}
						}
					}
				}
				targetTokens = append(targetTokens, xml.CopyToken(elem))
			} else {
				var id string
				for _, attr := range elem.Attr {
					if attr.Name.Local == "id" {
						id = attr.Value
						break
					}
				}
				if id == elementID {
					inTarget = true
					targetDepth = 1
					targetTokens = append(targetTokens, xml.CopyToken(elem))
				} else {
					ancestorStack = append(ancestorStack, elem.Copy())
				}
			}

		case xml.EndElement:
			if inTarget {
				targetTokens = append(targetTokens, xml.CopyToken(elem))
				targetDepth--
				if targetDepth == 0 {
					goto finishScan
				}
			} else if len(ancestorStack) > 0 {
				ancestorStack = ancestorStack[:len(ancestorStack)-1]
			}
		case xml.CharData, xml.Comment, xml.ProcInst, xml.Directive:
			if inTarget {
				targetTokens = append(targetTokens, xml.CopyToken(tok))
			}
		}
	nextToken:
	}

finishScan:
	if len(targetTokens) == 0 {
		return Rect{}, false
	}

	var buf bytes.Buffer
	enc := xml.NewEncoder(&buf)

	if !rootFound {
		rootElem = xml.StartElement{
			Name: xml.Name{Local: "svg"},
			Attr: []xml.Attr{
				{Name: xml.Name{Local: "xmlns"}, Value: "http://www.w3.org/2000/svg"},
			},
		}
	}
	if err := enc.EncodeToken(rootElem); err != nil {
		return Rect{}, false
	}

	for _, a := range ancestorStack {
		if a.Name.Local == "svg" {
			continue
		}
		if err := enc.EncodeToken(a); err != nil {
			return Rect{}, false
		}
	}

	for _, t := range targetTokens {
		if err := enc.EncodeToken(t); err != nil {
			return Rect{}, false
		}
	}

	for _, a := range slices.Backward(ancestorStack) {
		if a.Name.Local == "svg" {
			continue
		}
		if err := enc.EncodeToken(a.End()); err != nil {
			return Rect{}, false
		}
	}

	if err := enc.EncodeToken(rootElem.End()); err != nil {
		return Rect{}, false
	}
	if err := enc.Flush(); err != nil {
		return Rect{}, false
	}

	rect := ComputeDrawingRect(buf.Bytes())
	return rect, true
}

// ComputeDrawingRect calculates the bounding box of all paths in the SVG drawing,
// applying element and group transformation matrices, stroke widths, and joins.
// Returns a Rect with 0 width/height if no paths are present or if an error occurs.
func ComputeDrawingRect(data []byte) Rect {
	icon, err := oksvg.ReadIconStream(bytes.NewReader(data))
	if err != nil || len(icon.SVGPaths) == 0 {
		return Rect{}
	}

	scanner := &bboxScanner{
		minX: math.MaxFloat64,
		minY: math.MaxFloat64,
		maxX: -math.MaxFloat64,
		maxY: -math.MaxFloat64,
	}

	w := int(math.Ceil(icon.ViewBox.W))
	h := int(math.Ceil(icon.ViewBox.H))
	if w <= 0 {
		w = 512
	}
	if h <= 0 {
		h = 512
	}

	dasher := rasterx.NewDasher(w, h, scanner)
	for _, p := range icon.SVGPaths {
		drawPathTransformed(dasher, p, rasterx.Identity, 1.0, 1.0)
	}

	if scanner.found && scanner.maxX > scanner.minX && scanner.maxY > scanner.minY {
		return Rect{
			X:      scanner.minX,
			Y:      scanner.minY,
			Width:  scanner.maxX - scanner.minX,
			Height: scanner.maxY - scanner.minY,
		}
	}

	// Fallback to iterating raw nodes if scanner found no points (e.g. degenerated un-stroked/un-filled paths)
	minX, minY := math.MaxFloat64, math.MaxFloat64
	maxX, maxY := -math.MaxFloat64, -math.MaxFloat64
	var found bool

	for _, p := range icon.SVGPaths {
		strokeMargin := 0.0
		if p.LineWidth > 0 {
			strokeMargin = p.LineWidth / 2.0
			if (p.LineJoin == rasterx.Miter || p.LineJoin == rasterx.MiterClip) && p.MiterLimit > 1.0 {
				strokeMargin = p.LineWidth * (p.MiterLimit / 2.0)
			}
		}

		for i := 0; i < len(p.Path); {
			cmd := rasterx.PathCommand(p.Path[i])
			i++
			var numPts int
			switch cmd {
			case rasterx.PathMoveTo, rasterx.PathLineTo:
				numPts = 1
			case rasterx.PathQuadTo:
				numPts = 2
			case rasterx.PathCubicTo:
				numPts = 3
			case rasterx.PathClose:
				numPts = 0
			default:
				numPts = 0
			}
			for pIdx := 0; pIdx < numPts && i+1 < len(p.Path); pIdx++ {
				x := float64(p.Path[i]) / 64.0
				y := float64(p.Path[i+1]) / 64.0
				i += 2
				if x-strokeMargin < minX {
					minX = x - strokeMargin
				}
				if x+strokeMargin > maxX {
					maxX = x + strokeMargin
				}
				if y-strokeMargin < minY {
					minY = y - strokeMargin
				}
				if y+strokeMargin > maxY {
					maxY = y + strokeMargin
				}
				found = true
			}
		}
	}

	if !found || maxX <= minX || maxY <= minY {
		return Rect{}
	}

	return Rect{
		X:      minX,
		Y:      minY,
		Width:  maxX - minX,
		Height: maxY - minY,
	}
}

func parseSVGAttributes(attrs []xml.Attr, doc *SVGDocument) {
	for _, attr := range attrs {
		switch attr.Name.Local {
		case "width":
			doc.Width = parseDimension(attr.Value)
		case "height":
			doc.Height = parseDimension(attr.Value)
		case "viewBox":
			vbParts := strings.Fields(attr.Value)
			if len(vbParts) == 4 {
				doc.ViewBoxX, _ = strconv.ParseFloat(vbParts[0], 64)
				doc.ViewBoxY, _ = strconv.ParseFloat(vbParts[1], 64)
				doc.ViewBoxW, _ = strconv.ParseFloat(vbParts[2], 64)
				doc.ViewBoxH, _ = strconv.ParseFloat(vbParts[3], 64)
			}
		}
	}

	// Fallback dimensions from viewBox if width/height not explicitly specified
	if doc.Width <= 0 && doc.ViewBoxW > 0 {
		doc.Width = doc.ViewBoxW
	}
	if doc.Height <= 0 && doc.ViewBoxH > 0 {
		doc.Height = doc.ViewBoxH
	}
	// Fallback viewBox from width/height if viewBox not specified
	if doc.ViewBoxW <= 0 && doc.Width > 0 {
		doc.ViewBoxW = doc.Width
	}
	if doc.ViewBoxH <= 0 && doc.Height > 0 {
		doc.ViewBoxH = doc.Height
	}
}

// parseDimension parses strings with CSS/SVG length units ("px", "pt", "mm", "cm", "in", "pc")
// and converts them to standard user space pixels at 96 DPI.
func parseDimension(s string) float64 {
	s = strings.TrimSpace(s)
	scale := 1.0
	switch {
	case strings.HasSuffix(s, "in"):
		scale = 96.0
		s = strings.TrimSuffix(s, "in")
	case strings.HasSuffix(s, "mm"):
		scale = 96.0 / 25.4
		s = strings.TrimSuffix(s, "mm")
	case strings.HasSuffix(s, "cm"):
		scale = 96.0 / 2.54
		s = strings.TrimSuffix(s, "cm")
	case strings.HasSuffix(s, "pt"):
		scale = 96.0 / 72.0
		s = strings.TrimSuffix(s, "pt")
	case strings.HasSuffix(s, "pc"):
		scale = 16.0
		s = strings.TrimSuffix(s, "pc")
	case strings.HasSuffix(s, "px"):
		scale = 1.0
		s = strings.TrimSuffix(s, "px")
	}
	v, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return v * scale
}

func parseMotionConfig(label string) (MotionConfig, bool) {
	var configType string
	var contentStart int

	lower := strings.ToLower(label)
	if idx := strings.Index(lower, "move"); idx != -1 {
		rest := strings.TrimLeft(label[idx+4:], " \t")
		if strings.HasPrefix(rest, "{") {
			configType = "move"
			contentStart = idx + 4 + (len(label[idx+4:]) - len(rest)) + 1
		}
	}
	if configType == "" {
		if idx := strings.Index(lower, "rot"); idx != -1 {
			rest := strings.TrimLeft(label[idx+3:], " \t")
			if strings.HasPrefix(rest, "{") {
				configType = "rot"
				contentStart = idx + 3 + (len(label[idx+3:]) - len(rest)) + 1
			}
		}
	}
	if configType == "" {
		if idx := strings.Index(lower, "scale"); idx != -1 {
			rest := strings.TrimLeft(label[idx+5:], " \t")
			if strings.HasPrefix(rest, "{") {
				configType = "scale"
				contentStart = idx + 5 + (len(label[idx+5:]) - len(rest)) + 1
			}
		}
	}
	if configType == "" {
		if idx := strings.Index(lower, "scal"); idx != -1 {
			rest := strings.TrimLeft(label[idx+4:], " \t")
			if strings.HasPrefix(rest, "{") {
				configType = "scale"
				contentStart = idx + 4 + (len(label[idx+4:]) - len(rest)) + 1
			}
		}
	}
	if configType == "" {
		if idx := strings.Index(lower, "fade"); idx != -1 {
			rest := strings.TrimLeft(label[idx+4:], " \t")
			if strings.HasPrefix(rest, "{") {
				configType = "fade"
				contentStart = idx + 4 + (len(label[idx+4:]) - len(rest)) + 1
			}
		}
	}
	if configType == "" {
		if idx := strings.Index(lower, "show"); idx != -1 {
			rest := strings.TrimLeft(label[idx+4:], " \t")
			if strings.HasPrefix(rest, "{") {
				configType = "show"
				contentStart = idx + 4 + (len(label[idx+4:]) - len(rest)) + 1
			}
		}
	}
	if configType == "" {
		if idx := strings.Index(lower, "hide"); idx != -1 {
			rest := strings.TrimLeft(label[idx+4:], " \t")
			if strings.HasPrefix(rest, "{") {
				configType = "hide"
				contentStart = idx + 4 + (len(label[idx+4:]) - len(rest)) + 1
			}
		}
	}
	if configType == "" {
		if idx := strings.Index(lower, "depth"); idx != -1 {
			rest := strings.TrimLeft(label[idx+5:], " \t")
			if strings.HasPrefix(rest, "{") {
				configType = "depth"
				contentStart = idx + 5 + (len(label[idx+5:]) - len(rest)) + 1
			}
		}
	}
	if configType == "" {
		if idx := strings.Index(lower, "camera"); idx != -1 {
			rest := strings.TrimLeft(label[idx+6:], " \t")
			if strings.HasPrefix(rest, "{") {
				configType = "camera"
				contentStart = idx + 6 + (len(label[idx+6:]) - len(rest)) + 1
			}
		}
	}
	if configType == "" {
		if idx := strings.Index(lower, "distance"); idx != -1 {
			rest := strings.TrimLeft(label[idx+8:], " \t")
			if strings.HasPrefix(rest, "{") {
				configType = "dist"
				contentStart = idx + 8 + (len(label[idx+8:]) - len(rest)) + 1
			}
		}
	}
	if configType == "" {
		if idx := strings.Index(lower, "dist"); idx != -1 {
			rest := strings.TrimLeft(label[idx+4:], " \t")
			if strings.HasPrefix(rest, "{") {
				configType = "dist"
				contentStart = idx + 4 + (len(label[idx+4:]) - len(rest)) + 1
			}
		}
	}
	if configType == "" {
		if idx := strings.Index(lower, "color"); idx != -1 {
			rest := strings.TrimLeft(label[idx+5:], " \t")
			if strings.HasPrefix(rest, "{") {
				configType = "color"
				contentStart = idx + 5 + (len(label[idx+5:]) - len(rest)) + 1
			}
		}
	}
	if configType == "" {
		return MotionConfig{}, false
	}

	endIdx := strings.Index(label[contentStart:], "}")
	if endIdx == -1 {
		return MotionConfig{}, false
	}
	configStr := label[contentStart : contentStart+endIdx]

	// Normalize whitespace around colons so "k : v" becomes "k:v"
	var cleaned strings.Builder
	runes := []rune(configStr)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == ':' {
			str := strings.TrimRight(cleaned.String(), " \t")
			cleaned.Reset()
			cleaned.WriteString(str)
			cleaned.WriteRune(':')
			for i+1 < len(runes) && (runes[i+1] == ' ' || runes[i+1] == '\t') {
				i++
			}
		} else {
			cleaned.WriteRune(r)
		}
	}
	configStr = cleaned.String()

	parts := strings.FieldsFunc(configStr, func(r rune) bool {
		return r == ';' || r == ',' || unicode.IsSpace(r)
	})

	config := DefaultMotionConfig(configType)

	var foundF bool
	var hasExplicitFrom bool

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if part == "rev" || part == "reverse" {
			config.Reverse = true
			continue
		}
		if part == "pingpong" {
			config.IsPingPong = true
			continue
		}
		if part == "fixed" {
			config.ParallaxFactor = 0.0
			config.HasParallax = true
			continue
		}
		kv := strings.SplitN(part, ":", 2)
		if len(kv) != 2 {
			continue
		}
		k := strings.TrimSpace(kv[0])
		v := strings.TrimSpace(kv[1])
		switch k {
		case "ease":
			config.Ease = v
		case "t":
			config.Type = v
		case "pingpong":
			config.IsPingPong = (v == "true" || v == "1" || v == "yes")
		case "target":
			vLower := strings.ToLower(v)
			if vLower == "stroke" || vLower == "all" {
				config.ColorTarget = vLower
			} else {
				config.ColorTarget = "fill"
			}
		case "r", "repeat":
			if vInt, err := strconv.Atoi(v); err == nil && vInt > 0 {
				config.ColorRepeat = vInt
			}
		case "rev", "reverse":
			config.Reverse = (v == "true" || v == "1" || v == "yes")
		case "factor":
			if vFloat, err := strconv.ParseFloat(v, 64); err == nil {
				config.ParallaxFactor = vFloat
				config.HasParallax = true
			}
		case "fixed":
			vLower := strings.ToLower(v)
			if vLower == "true" || vLower == "yes" || vLower == "1" || vLower == "" {
				config.ParallaxFactor = 0.0
				config.HasParallax = true
			}
		case "opacity":
			if vFloat, err := strconv.ParseFloat(strings.TrimSuffix(v, "%"), 64); err == nil {
				if vFloat > 1.0 {
					vFloat /= 100.0
				}
				if vFloat < 0.0 {
					vFloat = 0.0
				} else if vFloat > 1.0 {
					vFloat = 1.0
				}
				config.OpacityTo = vFloat
				if !hasExplicitFrom {
					config.OpacityFrom = vFloat
				}
				config.HasOpacity = true
			}
		case "visibility", "state":
			vLower := strings.ToLower(v)
			if vLower == "show" || vLower == "hide" {
				config.VisibilityState = vLower
				config.HasVisibility = true
			}
		case "z", "depth":
			if configType == "dist" {
				if vFloat, err := strconv.ParseFloat(v, 64); err == nil {
					config.ParallaxFactor = 1.0 / (1.0 + vFloat*0.01)
					config.HasParallax = true
				}
			} else {
				cleanV := strings.TrimPrefix(v, "+")
				if zInt, err := strconv.Atoi(cleanV); err == nil {
					config.DepthOffset = zInt
					config.HasDepth = true
				}
			}
		case "scale":
			if vFloat, err := strconv.ParseFloat(v, 64); err == nil {
				config.ScaleToX = vFloat
				config.ScaleToY = vFloat
			}
		case "scale-x", "x":
			if vFloat, err := strconv.ParseFloat(v, 64); err == nil {
				config.ScaleToX = vFloat
			}
		case "scale-y", "y":
			if vFloat, err := strconv.ParseFloat(v, 64); err == nil {
				config.ScaleToY = vFloat
			}
		case "from":
			switch configType {
			case "fade":
				if vFloat, err := strconv.ParseFloat(strings.TrimSuffix(v, "%"), 64); err == nil {
					if vFloat > 1.0 {
						vFloat /= 100.0
					}
					if vFloat < 0.0 {
						vFloat = 0.0
					} else if vFloat > 1.0 {
						vFloat = 1.0
					}
					config.OpacityFrom = vFloat
					hasExplicitFrom = true
					config.HasOpacity = true
				}
			case "rot":
				if vFloat, err := strconv.ParseFloat(strings.TrimSuffix(v, "deg"), 64); err == nil {
					config.RotationFrom = vFloat
					config.HasRotationRange = true
				}
			default:
				if vFloat, err := strconv.ParseFloat(v, 64); err == nil {
					config.ScaleFromX = vFloat
					config.ScaleFromY = vFloat
				}
			}
		case "from-x":
			if vFloat, err := strconv.ParseFloat(v, 64); err == nil {
				config.ScaleFromX = vFloat
			}
		case "from-y":
			if vFloat, err := strconv.ParseFloat(v, 64); err == nil {
				config.ScaleFromY = vFloat
			}
		case "to":
			switch configType {
			case "fade":
				if vFloat, err := strconv.ParseFloat(strings.TrimSuffix(v, "%"), 64); err == nil {
					if vFloat > 1.0 {
						vFloat /= 100.0
					}
					if vFloat < 0.0 {
						vFloat = 0.0
					} else if vFloat > 1.0 {
						vFloat = 1.0
					}
					config.OpacityTo = vFloat
					config.HasOpacity = true
				}
			case "rot":
				if vFloat, err := strconv.ParseFloat(strings.TrimSuffix(v, "deg"), 64); err == nil {
					config.RotationTo = vFloat
					config.HasRotationRange = true
				}
			default:
				if vFloat, err := strconv.ParseFloat(v, 64); err == nil {
					config.ScaleToX = vFloat
					config.ScaleToY = vFloat
				}
			}
		case "to-x":
			if vFloat, err := strconv.ParseFloat(v, 64); err == nil {
				config.ScaleToX = vFloat
			}
		case "to-y":
			if vFloat, err := strconv.ParseFloat(v, 64); err == nil {
				config.ScaleToY = vFloat
			}
		case "angle":
			if deg, err := strconv.ParseFloat(v, 64); err == nil {
				if configType == "color" {
					config.ColorAngle = deg
					config.HasColorAngle = true
				} else {
					config.RotationAngle = deg
				}
			}
		case "dir":
			config.RotationDir = strings.ToLower(v)
		case "orient":
			config.OrientPath = (v == "true" || v == "1" || v == "yes")
		case "pivot":
			vLower := strings.ToLower(v)
			if vLower == "center" || vLower == "" {
				config.PivotType = "center"
			} else if vLower == "path-start" {
				config.PivotType = "path-start"
			} else if strings.HasPrefix(v, "#") {
				config.PivotType = "node"
				config.PivotNodeID = strings.TrimPrefix(v, "#")
			} else if angle, err := strconv.ParseFloat(v, 64); err == nil {
				config.PivotType = "edge"
				config.PivotEdgeAngle = angle
			}
		case "f":
			foundF = true
			if v == "all" {
				config.IsAll = true
			} else {
				rangeParts := strings.Split(v, "-")
				if len(rangeParts) == 2 {
					start, _ := strconv.Atoi(rangeParts[0])
					end, _ := strconv.Atoi(rangeParts[1])
					config.StartFrame = start
					config.EndFrame = end
				} else if len(rangeParts) == 1 {
					singleF, err := strconv.Atoi(rangeParts[0])
					if err == nil && singleF > 0 {
						config.StartFrame = singleF
						config.EndFrame = singleF
					}
				}
			}
		}
	}

	if !foundF {
		config.IsAll = true
	}

	return config, true
}
