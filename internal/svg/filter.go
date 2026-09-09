package svg

import (
	"bytes"
	"encoding/xml"
	"image"
	"image/color"
	"math"
	"strconv"
	"strings"

	"github.com/anthonynsimon/bild/blur"
	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
)

// DropShadowFilter represents extracted SVG drop shadow parameters.
type DropShadowFilter struct {
	ID           string
	Dx           float64
	Dy           float64
	StdDeviation float64
	FloodColor   color.NRGBA
}

// extractFilterDefs extracts drop shadow filter definitions from the SVG XML.
func extractFilterDefs(svgData []byte) map[string]DropShadowFilter {
	filters := make(map[string]DropShadowFilter)
	dec := xml.NewDecoder(bytes.NewReader(svgData))

	var curFilter *DropShadowFilter
	var hasBlur, hasOffset, hasFlood bool

	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}

		switch elem := tok.(type) {
		case xml.StartElement:
			name := elem.Name.Local
			if name == "filter" {
				id := extractAttr(elem.Attr, "id")
				if id != "" {
					curFilter = &DropShadowFilter{
						ID:         id,
						FloodColor: color.NRGBA{R: 0, G: 0, B: 0, A: 255},
					}
					hasBlur = false
					hasOffset = false
					hasFlood = false
				}
			} else if curFilter != nil {
				switch name {
				case "feDropShadow":
					if dxVal := extractAttr(elem.Attr, "dx"); dxVal != "" {
						curFilter.Dx, _ = strconv.ParseFloat(strings.TrimSpace(dxVal), 64)
					}
					if dyVal := extractAttr(elem.Attr, "dy"); dyVal != "" {
						curFilter.Dy, _ = strconv.ParseFloat(strings.TrimSpace(dyVal), 64)
					}
					if stdVal := extractAttr(elem.Attr, "stdDeviation"); stdVal != "" {
						curFilter.StdDeviation, _ = parseStdDeviation(stdVal)
					}
					fCol := extractAttr(elem.Attr, "flood-color")
					fOp := extractAttr(elem.Attr, "flood-opacity")
					if fCol != "" || fOp != "" {
						curFilter.FloodColor = parseFloodColor(fCol, fOp)
					}
					filters[curFilter.ID] = *curFilter

				case "feGaussianBlur":
					if stdVal := extractAttr(elem.Attr, "stdDeviation"); stdVal != "" {
						curFilter.StdDeviation, _ = parseStdDeviation(stdVal)
						hasBlur = true
					}

				case "feOffset":
					if dxVal := extractAttr(elem.Attr, "dx"); dxVal != "" {
						curFilter.Dx, _ = strconv.ParseFloat(strings.TrimSpace(dxVal), 64)
					}
					if dyVal := extractAttr(elem.Attr, "dy"); dyVal != "" {
						curFilter.Dy, _ = strconv.ParseFloat(strings.TrimSpace(dyVal), 64)
					}
					hasOffset = true

				case "feFlood":
					fCol := extractAttr(elem.Attr, "flood-color")
					fOp := extractAttr(elem.Attr, "flood-opacity")
					if style := extractAttr(elem.Attr, "style"); style != "" {
						if sc := extractCSSProp(style, "flood-color"); sc != "" {
							fCol = sc
						}
						if so := extractCSSProp(style, "flood-opacity"); so != "" {
							fOp = so
						}
					}
					if fCol != "" || fOp != "" {
						curFilter.FloodColor = parseFloodColor(fCol, fOp)
						hasFlood = true
					}
				}
			}

		case xml.EndElement:
			if elem.Name.Local == "filter" && curFilter != nil {
				if hasBlur || hasOffset || hasFlood {
					filters[curFilter.ID] = *curFilter
				}
				curFilter = nil
			}
		}
	}
	return filters
}

// extractPathFilterIDs maps each shape element in the document to its active filter ID.
func extractPathFilterIDs(svgData []byte) []string {
	dec := xml.NewDecoder(bytes.NewReader(svgData))
	filterStack := []string{""}
	var pathFilterIDs []string
	var inDefs int

	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}

		switch elem := tok.(type) {
		case xml.StartElement:
			name := elem.Name.Local
			if name == "defs" {
				inDefs++
			}

			curFilter := filterStack[len(filterStack)-1]
			for _, a := range elem.Attr {
				if a.Name.Local == "filter" {
					if fID := parseFilterURL(a.Value); fID != "" {
						curFilter = fID
					}
				}
				if a.Name.Local == "style" {
					if fProp := extractCSSProp(a.Value, "filter"); fProp != "" {
						if fID := parseFilterURL(fProp); fID != "" {
							curFilter = fID
						}
					}
				}
			}

			if name == "g" {
				filterStack = append(filterStack, curFilter)
			}

			if inDefs == 0 && isShapeElement(name) {
				pathFilterIDs = append(pathFilterIDs, curFilter)
			}

		case xml.EndElement:
			name := elem.Name.Local
			if name == "defs" && inDefs > 0 {
				inDefs--
			}
			if name == "g" && len(filterStack) > 1 {
				filterStack = filterStack[:len(filterStack)-1]
			}
		}
	}
	return pathFilterIDs
}

func parseFilterURL(val string) string {
	val = strings.TrimSpace(val)
	if strings.HasPrefix(val, "url(#") && strings.HasSuffix(val, ")") {
		return strings.TrimSuffix(strings.TrimPrefix(val, "url(#"), ")")
	}
	if strings.HasPrefix(val, "url('#") && strings.HasSuffix(val, "')") {
		return strings.TrimSuffix(strings.TrimPrefix(val, "url('#"), "')")
	}
	if strings.HasPrefix(val, "url(\"#") && strings.HasSuffix(val, "\")") {
		return strings.TrimSuffix(strings.TrimPrefix(val, "url(\"#"), "\")")
	}
	return ""
}

func extractAttr(attrs []xml.Attr, name string) string {
	for _, a := range attrs {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

func parseStdDeviation(val string) (float64, error) {
	fields := strings.Fields(val)
	if len(fields) == 0 {
		return 0, nil
	}
	// If two values provided (stdDeviationX, stdDeviationY), average them for isotropic blur
	if len(fields) >= 2 {
		sx, err1 := strconv.ParseFloat(fields[0], 64)
		sy, err2 := strconv.ParseFloat(fields[1], 64)
		if err1 == nil && err2 == nil {
			return (sx + sy) / 2.0, nil
		}
	}
	return strconv.ParseFloat(fields[0], 64)
}

func parseFloodColor(colStr, opStr string) color.NRGBA {
	col := color.NRGBA{R: 0, G: 0, B: 0, A: 255}
	if colStr != "" {
		if c, err := oksvg.ParseSVGColor(colStr); err == nil {
			r, g, b, _ := c.RGBA()
			col.R = uint8(r >> 8)
			col.G = uint8(g >> 8)
			col.B = uint8(b >> 8)
		}
	}
	if opStr != "" {
		if op, err := strconv.ParseFloat(strings.TrimSpace(opStr), 64); err == nil {
			if op < 0 {
				op = 0
			}
			if op > 1 {
				op = 1
			}
			col.A = uint8(math.Round(op * 255.0))
		}
	}
	return col
}

func renderAndCompositeDropShadow(dst *image.RGBA, path oksvg.SvgPath, tm rasterx.Matrix2D, f DropShadowFilter, w, h int, scaleFactor float64) {
	shadowBuf := image.NewRGBA(image.Rect(0, 0, w, h))
	shadowScanner := rasterx.NewScannerGV(w, h, shadowBuf, shadowBuf.Bounds())
	shadowRaster := rasterx.NewDasher(w, h, shadowScanner)

	path.DrawTransformed(shadowRaster, 1.0, tm)

	b := shadowBuf.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			_, _, _, a := shadowBuf.At(x, y).RGBA()
			if a > 0 {
				alphaFrac := float64(a>>8) / 255.0
				finalA := uint8(math.Round(alphaFrac * float64(f.FloodColor.A)))
				shadowBuf.SetRGBA(x, y, color.RGBA{
					R: f.FloodColor.R,
					G: f.FloodColor.G,
					B: f.FloodColor.B,
					A: finalA,
				})
			}
		}
	}

	radius := f.StdDeviation * scaleFactor
	if radius < 0.1 {
		radius = 0.1
	}
	blurred := blur.Gaussian(shadowBuf, radius)

	dx := int(math.Round(f.Dx * scaleFactor))
	dy := int(math.Round(f.Dy * scaleFactor))

	compositeOver(dst, blurred, dx, dy)
}

func compositeOver(dst *image.RGBA, src *image.RGBA, offsetX, offsetY int) {
	b := src.Bounds()
	w, h := dst.Bounds().Dx(), dst.Bounds().Dy()

	for y := b.Min.Y; y < b.Max.Y; y++ {
		dy := y + offsetY
		if dy < 0 || dy >= h {
			continue
		}
		for x := b.Min.X; x < b.Max.X; x++ {
			dx := x + offsetX
			if dx < 0 || dx >= w {
				continue
			}

			sr, sg, sb, sa := src.At(x, y).RGBA()
			if sa == 0 {
				continue
			}

			dr, dg, db, da := dst.At(dx, dy).RGBA()
			if da == 0 {
				dst.SetRGBA(dx, dy, color.RGBA{
					R: uint8(sr >> 8),
					G: uint8(sg >> 8),
					B: uint8(sb >> 8),
					A: uint8(sa >> 8),
				})
				continue
			}

			srcAlpha := float64(sa) / 65535.0
			dstAlpha := float64(da) / 65535.0
			outAlpha := srcAlpha + dstAlpha*(1.0-srcAlpha)

			if outAlpha > 0 {
				outR := (float64(sr)/65535.0 + (float64(dr)/65535.0)*dstAlpha*(1.0-srcAlpha)) / outAlpha
				outG := (float64(sg)/65535.0 + (float64(dg)/65535.0)*dstAlpha*(1.0-srcAlpha)) / outAlpha
				outB := (float64(sb)/65535.0 + (float64(db)/65535.0)*dstAlpha*(1.0-srcAlpha)) / outAlpha

				dst.SetRGBA(dx, dy, color.RGBA{
					R: uint8(math.Round(outR * 255.0)),
					G: uint8(math.Round(outG * 255.0)),
					B: uint8(math.Round(outB * 255.0)),
					A: uint8(math.Round(outAlpha * 255.0)),
				})
			}
		}
	}
}
