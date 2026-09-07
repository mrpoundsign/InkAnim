package svg

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
)

// ParseSVG parses an Inkscape SVG document from raw bytes.
func ParseSVG(data []byte) (*SVGDocument, error) {
	doc := &SVGDocument{
		RawContent:  data,
		DefaultMode: ModeLayers,
	}

	decoder := xml.NewDecoder(bytes.NewReader(data))
	var inSVGTag bool
	var layerIdx, pageIdx int

	for {
		token, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("xml decode error: %w", err)
		}

		switch elem := token.(type) {
		case xml.StartElement:
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
	}

	// Determine default mode based on what's found
	if len(doc.Pages) > 1 && len(doc.Layers) <= 1 {
		doc.DefaultMode = ModePages
	} else {
		doc.DefaultMode = ModeLayers
	}

	doc.DrawingRect = ComputeDrawingRect(data)

	return doc, nil
}

// ComputeDrawingRect calculates the bounding box of all paths in the SVG drawing.
// Returns a Rect with 0 width/height if no paths are present or if an error occurs.
func ComputeDrawingRect(data []byte) Rect {
	icon, err := oksvg.ReadIconStream(bytes.NewReader(data))
	if err != nil || len(icon.SVGPaths) == 0 {
		return Rect{}
	}

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

// parseDimension parses strings like "100", "100px", "100mm", "100pt" into float64.
func parseDimension(s string) float64 {
	s = strings.TrimSpace(s)
	// Strip known units
	units := []string{"px", "pt", "mm", "cm", "in", "pc"}
	for _, u := range units {
		if strings.HasSuffix(s, u) {
			s = strings.TrimSuffix(s, u)
			break
		}
	}
	v, _ := strconv.ParseFloat(s, 64)
	return v
}
