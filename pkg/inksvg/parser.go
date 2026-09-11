package inksvg

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"image"
	"io"
	"math"
	"strconv"
	"strings"

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
		RawContent:  processedData,
		DefaultMode: ModeLayers,
	}

	decoder := xml.NewDecoder(bytes.NewReader(processedData))
	var inSVGTag bool
	var layerIdx, pageIdx int
	var activeGroups []string

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

			// Check for Inkscape layer: <g inkscape:groupmode="layer" ...>
			if name == "g" {
				var isLayer bool
				var id, label, style string
				for _, attr := range elem.Attr {
					if attr.Name.Local == "groupmode" && attr.Value == "layer" {
						isLayer = true
					}
					if attr.Name.Local == "id" {
						id = attr.Value
					}
					if attr.Name.Local == "label" {
						label = attr.Value
					}
					if attr.Name.Local == "style" {
						style = attr.Value
					}
				}

				if isLayer {
					if label == "" {
						label = id
					}
					if label == "" {
						label = fmt.Sprintf("Layer %d", layerIdx+1)
					}
					visible := !strings.Contains(style, "display:none")
					layer := Layer{
						ID:         id,
						Label:      label,
						Index:      layerIdx,
						Visible:    visible,
						IsActive:   true,
						DurationMs: 100, // 10 fps default
					}
					doc.Layers = append(doc.Layers, layer)
					layerIdx++
				}
				activeGroups = append(activeGroups, id)
			}

			// Check for IAMS Motion Path: <path inkscape:label="Movement {...}" d="...">
			if name == "path" {
				var id, label, d string
				for _, attr := range elem.Attr {
					if attr.Name.Local == "id" {
						id = attr.Value
					}
					if attr.Name.Local == "label" {
						label = attr.Value
					}
					if attr.Name.Local == "d" {
						d = attr.Value
					}
				}
				if cfg, ok := parseMotionConfig(label); ok && len(activeGroups) > 0 {
					doc.MotionPaths = append(doc.MotionPaths, MotionPath{
						ID:       id,
						GroupID:  activeGroups[len(activeGroups)-1],
						PathData: d,
						Config:   cfg,
					})
				}
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
		}
	}

	// Default mode is ModeLayers (pages serve as artboard crop boundaries)
	doc.DefaultMode = ModeLayers

	// Check if this is a timeline animation
	maxFrame := 0
	for _, mp := range doc.MotionPaths {
		if mp.Config.EndFrame > maxFrame {
			maxFrame = mp.Config.EndFrame
		}
	}

	if maxFrame > 0 {
		doc.DefaultMode = ModeTimeline
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
	} else if len(doc.Layers) == 0 {
		// Fallback: if no explicit layers were found, treat the document as a single layer
		doc.Layers = []Layer{
			{
				ID:         "layer_default",
				Label:      "Layer 1",
				Index:      0,
				Visible:    true,
				IsActive:   true,
				DurationMs: 100,
			},
		}
	}

	doc.DrawingRect = ComputeDrawingRect(processedData)

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
	icon.Draw(dasher, 1.0)

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
	idx := strings.Index(label, "Motion {")
	if idx == -1 {
		return MotionConfig{}, false
	}
	endIdx := strings.Index(label[idx:], "}")
	if endIdx == -1 {
		return MotionConfig{}, false
	}
	configStr := label[idx+8 : idx+endIdx]
	parts := strings.Split(configStr, ";")

	config := MotionConfig{
		Ease: "linear",
	}

	for _, part := range parts {
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
		case "f":
			if v == "all" {
				config.IsAll = true
			} else {
				rangeParts := strings.Split(v, "-")
				if len(rangeParts) == 2 {
					start, _ := strconv.Atoi(rangeParts[0])
					end, _ := strconv.Atoi(rangeParts[1])
					config.StartFrame = start
					config.EndFrame = end
				}
			}
		}
	}
	return config, true
}
