package svg

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"unicode"
)

// PreprocessSVG applies in-memory transformations to raw SVG data:
//  1. Normalizes root <svg> dimensions and viewBox to user-space pixels (Issue #45).
//  2. Injects canvas background rect if sodipodi:namedview specifies pagecolor with opacity (Issue #43).
//  3. Expands and inlines SVG <use> element clones into <g> groups (Issue #46).
//  4. Converts SVG <text> and <tspan> elements into standard <path> vector glyph contours (Issue #21).
//  5. Evaluates Inkscape fillet_chamfer Live Path Effects on paths referencing them (when unbaked, Issue #41).
//  6. Normalizes rect rx/ry arcs.
//  7. Scales shape stroke-width by cumulative group transform scaling.
//  8. Desugars paint-order: stroke fill (and stroke fill markers) into consecutive stroke-then-fill elements
//     so renderers like oksvg (which lack native paint-order support) render strokes under fills correctly.
func PreprocessSVG(data []byte) ([]byte, error) {
	// First pass: extract document metadata, namedview background, and LPE definitions
	meta := extractDocumentMetadata(data)

	// Expand SVG <use> elements early so inlined clones benefit from all downstream stages
	if len(meta.ElementsByID) > 0 {
		expanded, err := expandUseElements(data, meta.ElementsByID)
		if err == nil {
			data = expanded
		}
	}

	effects := meta.Effects

	// Second pass: stream transform XML tokens
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var buf bytes.Buffer
	encoder := xml.NewEncoder(&buf)

	transformStack := []Matrix2D{IdentityMatrix()}
	var rootSVGSeen bool

	for {
		token, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("xml preprocess error: %w", err)
		}

		switch elem := token.(type) {
		case xml.StartElement:
			name := elem.Name.Local

			// Stage 0: Normalize root SVG dimensions/viewBox and inject canvas background rect
			if name == "svg" && !rootSVGSeen {
				rootSVGSeen = true

				var newAttrs []xml.Attr
				var hasVB bool
				for _, a := range elem.Attr {
					switch a.Name.Local {
					case "width":
						if meta.Width > 0 {
							newAttrs = append(newAttrs, xml.Attr{Name: a.Name, Value: fmt.Sprintf("%f", meta.Width)})
						} else {
							newAttrs = append(newAttrs, a)
						}
					case "height":
						if meta.Height > 0 {
							newAttrs = append(newAttrs, xml.Attr{Name: a.Name, Value: fmt.Sprintf("%f", meta.Height)})
						} else {
							newAttrs = append(newAttrs, a)
						}
					case "viewBox":
						hasVB = true
						newAttrs = append(newAttrs, a)
					default:
						newAttrs = append(newAttrs, a)
					}
				}
				if !hasVB && meta.Width > 0 && meta.Height > 0 {
					newAttrs = append(newAttrs, xml.Attr{
						Name:  xml.Name{Local: "viewBox"},
						Value: fmt.Sprintf("0 0 %f %f", meta.Width, meta.Height),
					})
				}
				elem.Attr = newAttrs

				if err := encoder.EncodeToken(elem); err != nil {
					return nil, err
				}
				if meta.HasPageColor {
					bgX := meta.VBX
					bgY := meta.VBY
					bgW := meta.VBW
					bgH := meta.VBH
					if bgW <= 0 && bgH <= 0 {
						bgX = 0
						bgY = 0
						bgW = meta.Width
						bgH = meta.Height
					}
					if bgW <= 0 {
						bgW = 512
					}
					if bgH <= 0 {
						bgH = 512
					}

					bgRect := xml.StartElement{
						Name: xml.Name{Local: "rect"},
						Attr: []xml.Attr{
							{Name: xml.Name{Local: "id"}, Value: "inkanim_page_background"},
							{Name: xml.Name{Local: "x"}, Value: fmt.Sprintf("%f", bgX)},
							{Name: xml.Name{Local: "y"}, Value: fmt.Sprintf("%f", bgY)},
							{Name: xml.Name{Local: "width"}, Value: fmt.Sprintf("%f", bgW)},
							{Name: xml.Name{Local: "height"}, Value: fmt.Sprintf("%f", bgH)},
							{Name: xml.Name{Local: "fill"}, Value: meta.PageColor},
							{Name: xml.Name{Local: "fill-opacity"}, Value: fmt.Sprintf("%f", meta.PageOpacity)},
							{Name: xml.Name{Local: "style"}, Value: "stroke:none;"},
						},
					}
					if err := encoder.EncodeToken(bgRect); err != nil {
						return nil, err
					}
					if err := encoder.EncodeToken(xml.EndElement{Name: bgRect.Name}); err != nil {
						return nil, err
					}
				}
				continue
			}

			// Inline inherited stops and normalize gradientTransform on gradients (Issue #49)
			if name == "linearGradient" || name == "radialGradient" {
				var gradID string
				for _, a := range elem.Attr {
					if a.Name.Local == "id" {
						gradID = a.Value
						break
					}
				}

				gradDef := meta.Gradients[gradID]
				var newAttrs []xml.Attr
				for _, a := range elem.Attr {
					if a.Name.Local == "href" {
						// Strip href/xlink:href since stops are inlined directly
						continue
					}
					if a.Name.Local == "gradientTransform" {
						parsed := parseTransform(a.Value)
						if parsed != IdentityMatrix() {
							matStr := fmt.Sprintf("matrix(%f %f %f %f %f %f)", parsed.A, parsed.B, parsed.C, parsed.D, parsed.E, parsed.F)
							newAttrs = append(newAttrs, xml.Attr{Name: a.Name, Value: matStr})
							continue
						}
					}
					newAttrs = append(newAttrs, a)
				}
				elem.Attr = newAttrs

				if err := encoder.EncodeToken(elem); err != nil {
					return nil, err
				}

				if gradDef != nil && gradDef.OriginalStopCount == 0 && len(gradDef.Stops) > 0 {
					for _, st := range gradDef.Stops {
						if err := encoder.EncodeToken(st); err != nil {
							return nil, err
						}
					}
				}
				continue
			}

			// Stage 1: Convert SVG <text> elements into <path> vectors (Issue #21)
			if name == "text" {
				if err := ProcessTextElementToPaths(elem, decoder, encoder, &transformStack); err != nil {
					return nil, fmt.Errorf("text conversion failed: %w", err)
				}
				continue
			}

			// Track 2D affine transformation matrices on nested <g> elements
			if name == "g" {
				curMatrix := transformStack[len(transformStack)-1]
				for _, attr := range elem.Attr {
					if attr.Name.Local == "transform" {
						parsed := parseTransform(attr.Value)
						curMatrix = curMatrix.Multiply(parsed)
					}
				}
				transformStack = append(transformStack, curMatrix)
			}

			// Evaluate fillet_chamfer Live Path Effects on <path> elements
			if name == "path" {
				var pathEffectID string
				var dAttrIdx = -1

				for i, attr := range elem.Attr {
					if attr.Name.Local == "path-effect" {
						pathEffectID = strings.TrimPrefix(attr.Value, "#")
					}
					if attr.Name.Local == "d" {
						dAttrIdx = i
					}
				}

				if pathEffectID != "" && dAttrIdx >= 0 {
					if eff, ok := effects[pathEffectID]; ok && eff.Effect == "fillet_chamfer" {
						originalD := elem.Attr[dAttrIdx].Value
						filletedD := ApplyFilletChamferToPath(originalD, eff)
						elem.Attr[dAttrIdx].Value = filletedD
					}
				}
			}

			// Normalize rx/ry for rect elements: if only one is specified or one is zero while the
			// other is positive, mirror the non-zero radius. oksvg only inspects rx; if rx is omitted
			// or zero, oksvg incorrectly renders square corners.
			if name == "rect" {
				var rxVal, ryVal string
				var rxIdx, ryIdx = -1, -1
				for i, attr := range elem.Attr {
					if attr.Name.Local == "rx" {
						rxVal = attr.Value
						rxIdx = i
					}
					if attr.Name.Local == "ry" {
						ryVal = attr.Value
						ryIdx = i
					}
				}

				rxNum := parseDimension(rxVal)
				ryNum := parseDimension(ryVal)

				if (rxVal == "" || rxNum <= 0) && (ryVal != "" && ryNum > 0) {
					if rxIdx >= 0 {
						elem.Attr[rxIdx].Value = ryVal
					} else {
						elem.Attr = append(elem.Attr, xml.Attr{Name: xml.Name{Local: "rx"}, Value: ryVal})
					}
				} else if (ryVal == "" || ryNum <= 0) && (rxVal != "" && rxNum > 0) {
					if ryIdx >= 0 {
						elem.Attr[ryIdx].Value = rxVal
					} else {
						elem.Attr = append(elem.Attr, xml.Attr{Name: xml.Name{Local: "ry"}, Value: rxVal})
					}
				}
			}

			// For shapes inside transformed groups or with direct element transforms, oksvg
			// applies the CTM to path coordinates but ignores the transform when rasterizing stroke-width.
			// Scale stroke-width by the combined ancestor and element scale factor so strokes render at true visual thickness.
			if isShapeElement(name) {
				scale := transformStack[len(transformStack)-1].ScaleFactor()
				for _, attr := range elem.Attr {
					if attr.Name.Local == "transform" {
						scale *= parseTransform(attr.Value).ScaleFactor()
						break
					}
				}
				if scale > 0 && math.Abs(scale-1.0) > 0.001 {
					var styleAttrIdx = -1
					for i, attr := range elem.Attr {
						if attr.Name.Local == "style" {
							styleAttrIdx = i
							break
						}
					}
					if styleAttrIdx >= 0 {
						swVal := extractCSSProp(elem.Attr[styleAttrIdx].Value, "stroke-width")
						if swVal != "" {
							if swNum, unit := parseStrokeWidth(swVal); swNum > 0 {
								newSW := fmt.Sprintf("%.4f%s", swNum*scale, unit)
								elem.Attr[styleAttrIdx].Value = setStyleProp(elem.Attr[styleAttrIdx].Value, "stroke-width", newSW)
							}
						}
					}
					for i := range elem.Attr {
						if elem.Attr[i].Name.Local == "stroke-width" {
							if swNum, unit := parseStrokeWidth(elem.Attr[i].Value); swNum > 0 {
								elem.Attr[i].Value = fmt.Sprintf("%.4f%s", swNum*scale, unit)
							}
						}
					}
				}
			}

			// Desugar paint-order: stroke fill on all shape elements (path, rect, circle, polygon, etc.)
			if isShapeElement(name) {
				var styleAttrIdx = -1
				var paintOrderVal string

				for i, attr := range elem.Attr {
					if attr.Name.Local == "style" {
						styleAttrIdx = i
					}
					if attr.Name.Local == "paint-order" {
						paintOrderVal = attr.Value
					}
				}

				if styleAttrIdx >= 0 {
					po := extractCSSProp(elem.Attr[styleAttrIdx].Value, "paint-order")
					if po != "" {
						paintOrderVal = po
					}
				}

				if needsPaintOrderDesugar(paintOrderVal, elem.Attr, styleAttrIdx) {
					strokeElem, fillElem := desugarPaintOrder(&elem, styleAttrIdx)
					if err := encoder.EncodeToken(strokeElem); err != nil {
						return nil, err
					}
					if err := encoder.EncodeToken(xml.EndElement{Name: elem.Name}); err != nil {
						return nil, err
					}
					if err := encoder.EncodeToken(fillElem); err != nil {
						return nil, err
					}
					continue
				}
			}

			if err := encoder.EncodeToken(elem); err != nil {
				return nil, err
			}

		case xml.EndElement:
			if elem.Name.Local == "g" && len(transformStack) > 1 {
				transformStack = transformStack[:len(transformStack)-1]
			}
			if err := encoder.EncodeToken(elem); err != nil {
				return nil, err
			}

		default:
			if err := encoder.EncodeToken(token); err != nil {
				return nil, err
			}
		}
	}

	if err := encoder.Flush(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// GradientDef stores parsed gradient definitions and their color stop tokens.
type GradientDef struct {
	ID                string
	IsRadial          bool
	Href              string
	Attrs             []xml.Attr
	Stops             []xml.Token
	OriginalStopCount int
}

// DocumentMetadata collects document-level metadata during the first XML pass.
type DocumentMetadata struct {
	Effects      map[string]PathEffect
	ElementsByID map[string][]xml.Token
	Gradients    map[string]*GradientDef
	VBX, VBY     float64
	VBW, VBH     float64
	Width        float64
	Height       float64
	PageColor    string
	PageOpacity  float64
	HasPageColor bool
}

type idRecording struct {
	id     string
	depth  int
	tokens []xml.Token
}

// extractDocumentMetadata scans XML for root dimensions, namedview pagecolor, LPE definitions, and elements with IDs.
func extractDocumentMetadata(data []byte) DocumentMetadata {
	meta := DocumentMetadata{
		Effects:      make(map[string]PathEffect),
		ElementsByID: make(map[string][]xml.Token),
		Gradients:    make(map[string]*GradientDef),
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var activeRecordings []*idRecording
	var curGrad *GradientDef
	var curStopDepth int

	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}

		switch tok := token.(type) {
		case xml.StartElement:
			name := tok.Name.Local
			var elemID string
			for _, a := range tok.Attr {
				if a.Name.Local == "id" && a.Value != "" {
					elemID = a.Value
					break
				}
			}
			if elemID != "" {
				activeRecordings = append(activeRecordings, &idRecording{id: elemID})
			}
			for _, rec := range activeRecordings {
				rec.tokens = append(rec.tokens, tok.Copy())
				rec.depth++
			}

			if name == "linearGradient" || name == "radialGradient" {
				var id, href string
				for _, a := range tok.Attr {
					switch a.Name.Local {
					case "id":
						id = a.Value
					case "href":
						href = strings.TrimPrefix(a.Value, "#")
					}
				}
				curGrad = &GradientDef{
					ID:       id,
					IsRadial: name == "radialGradient",
					Href:     href,
					Attrs:    tok.Attr,
				}
				if id != "" {
					meta.Gradients[id] = curGrad
				}
			}

			if curGrad != nil {
				if name == "stop" {
					curStopDepth++
					curGrad.Stops = append(curGrad.Stops, tok.Copy())
				} else if curStopDepth > 0 {
					curGrad.Stops = append(curGrad.Stops, tok.Copy())
				}
			}

			switch name {
			case "svg":
				if meta.VBW == 0 && meta.Width == 0 {
					for _, a := range tok.Attr {
						switch a.Name.Local {
						case "viewBox":
							parts := strings.Fields(a.Value)
							if len(parts) == 4 {
								meta.VBX, _ = strconv.ParseFloat(parts[0], 64)
								meta.VBY, _ = strconv.ParseFloat(parts[1], 64)
								meta.VBW, _ = strconv.ParseFloat(parts[2], 64)
								meta.VBH, _ = strconv.ParseFloat(parts[3], 64)
							}
						case "width":
							meta.Width = parseDimension(a.Value)
						case "height":
							meta.Height = parseDimension(a.Value)
						}
					}
				}
			case "namedview":
				var pageColor, pageOpacityStr string
				for _, a := range tok.Attr {
					if a.Name.Local == "pagecolor" {
						pageColor = a.Value
					}
					if a.Name.Local == "pageopacity" {
						pageOpacityStr = a.Value
					}
				}
				if pageColor != "" && pageOpacityStr != "" {
					if op, err := strconv.ParseFloat(pageOpacityStr, 64); err == nil && op > 0 {
						meta.HasPageColor = true
						meta.PageColor = pageColor
						meta.PageOpacity = op
					}
				}
			case "path-effect":
				var eff PathEffect
				for _, attr := range tok.Attr {
					switch attr.Name.Local {
					case "id":
						eff.ID = attr.Value
					case "effect":
						eff.Effect = attr.Value
					case "radius":
						eff.Radius, _ = strconv.ParseFloat(attr.Value, 64)
					case "unit":
						eff.Unit = attr.Value
					case "mode":
						eff.Mode = attr.Value
					case "chamfer_steps":
						eff.Steps, _ = strconv.Atoi(attr.Value)
					case "use_knot_distance":
						eff.UseKnot = attr.Value == "true"
					case "nodesatellites_param":
						eff.RawParams = attr.Value
					}
				}
				if eff.ID != "" {
					meta.Effects[eff.ID] = eff
				}
			}

		case xml.EndElement:
			if curGrad != nil {
				switch tok.Name.Local {
				case "stop":
					curGrad.Stops = append(curGrad.Stops, tok)
					curStopDepth--
					curGrad.OriginalStopCount++
				case "linearGradient", "radialGradient":
					curGrad = nil
				}
			}

			var remaining []*idRecording
			for _, rec := range activeRecordings {
				rec.tokens = append(rec.tokens, tok)
				rec.depth--
				if rec.depth == 0 {
					meta.ElementsByID[rec.id] = rec.tokens
				} else {
					remaining = append(remaining, rec)
				}
			}
			activeRecordings = remaining

		default:
			if curGrad != nil && curStopDepth > 0 {
				curGrad.Stops = append(curGrad.Stops, xml.CopyToken(tok))
			}
			for _, rec := range activeRecordings {
				rec.tokens = append(rec.tokens, xml.CopyToken(tok))
			}
		}
	}

	// Resolve gradient stop inheritance (xlink:href / href template references)
	for _, grad := range meta.Gradients {
		curr := grad
		for depth := 0; grad.OriginalStopCount == 0 && curr.Href != "" && depth < 10; depth++ {
			parent, ok := meta.Gradients[curr.Href]
			if !ok {
				break
			}
			if len(parent.Stops) > 0 {
				grad.Stops = make([]xml.Token, len(parent.Stops))
				for i, st := range parent.Stops {
					grad.Stops[i] = xml.CopyToken(st)
				}
				break
			}
			curr = parent
		}
	}

	return meta
}

// expandUseElements recursively inlines <use> element clones into <g> groups up to 10 iterations.
func expandUseElements(data []byte, elementsByID map[string][]xml.Token) ([]byte, error) {
	current := data
	for iter := 0; iter < 10; iter++ {
		expanded, changed, err := expandUseElementsOnce(current, elementsByID)
		if err != nil {
			return nil, err
		}
		if !changed {
			break
		}
		current = expanded
	}
	return current, nil
}

func expandUseElementsOnce(data []byte, elementsByID map[string][]xml.Token) ([]byte, bool, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var buf bytes.Buffer
	encoder := xml.NewEncoder(&buf)
	var changed bool

	for {
		token, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, false, err
		}

		switch tok := token.(type) {
		case xml.StartElement:
			if tok.Name.Local == "use" {
				var targetID string
				var xStr, yStr, origTransform, useID string
				var passAttrs []xml.Attr

				for _, a := range tok.Attr {
					switch a.Name.Local {
					case "href", "xlink:href":
						targetID = strings.TrimPrefix(a.Value, "#")
					case "x":
						xStr = a.Value
					case "y":
						yStr = a.Value
					case "transform":
						origTransform = a.Value
					case "id":
						useID = a.Value
					case "width", "height":
						// SVG spec: width/height on <use> only apply to <svg> or <symbol>
					default:
						passAttrs = append(passAttrs, a)
					}
				}

				// Consume any child tokens inside <use> until its matching </use>
				depth := 1
				for depth > 0 {
					childTok, err := decoder.Token()
					if err != nil {
						break
					}
					switch childTok.(type) {
					case xml.StartElement:
						depth++
					case xml.EndElement:
						depth--
					}
				}

				targetTokens, ok := elementsByID[targetID]
				if !ok || len(targetTokens) == 0 {
					// Referenced element not found; output as original <use></use>
					if err := encoder.EncodeToken(tok); err != nil {
						return nil, false, err
					}
					if err := encoder.EncodeToken(xml.EndElement{Name: tok.Name}); err != nil {
						return nil, false, err
					}
					continue
				}

				changed = true

				origM := IdentityMatrix()
				if origTransform != "" {
					origM = parseTransform(origTransform)
				}
				xVal := parseDimension(xStr)
				yVal := parseDimension(yStr)
				transM := Matrix2D{A: 1, D: 1, E: xVal, F: yVal}
				finalM := origM.Multiply(transM)

				var wrapperAttrs []xml.Attr
				if useID != "" {
					wrapperAttrs = append(wrapperAttrs, xml.Attr{Name: xml.Name{Local: "id"}, Value: useID})
				}
				if finalM != IdentityMatrix() {
					matStr := fmt.Sprintf("matrix(%f %f %f %f %f %f)", finalM.A, finalM.B, finalM.C, finalM.D, finalM.E, finalM.F)
					wrapperAttrs = append(wrapperAttrs, xml.Attr{Name: xml.Name{Local: "transform"}, Value: matStr})
				}
				wrapperAttrs = append(wrapperAttrs, passAttrs...)

				wrapperStart := xml.StartElement{
					Name: xml.Name{Local: "g"},
					Attr: wrapperAttrs,
				}
				if err := encoder.EncodeToken(wrapperStart); err != nil {
					return nil, false, err
				}

				// Clone target tokens, stripping the 'id' attribute on the root target element
				cloned := make([]xml.Token, len(targetTokens))
				for i, t := range targetTokens {
					cloned[i] = xml.CopyToken(t)
				}
				if rootStart, ok := cloned[0].(xml.StartElement); ok {
					var nonIDAttrs []xml.Attr
					for _, a := range rootStart.Attr {
						if a.Name.Local != "id" {
							nonIDAttrs = append(nonIDAttrs, a)
						}
					}
					rootStart.Attr = nonIDAttrs
					cloned[0] = rootStart
				}

				for _, t := range cloned {
					if err := encoder.EncodeToken(t); err != nil {
						return nil, false, err
					}
				}

				if err := encoder.EncodeToken(xml.EndElement{Name: wrapperStart.Name}); err != nil {
					return nil, false, err
				}
				continue
			}

			if err := encoder.EncodeToken(tok); err != nil {
				return nil, false, err
			}

		default:
			if err := encoder.EncodeToken(tok); err != nil {
				return nil, false, err
			}
		}
	}

	if err := encoder.Flush(); err != nil {
		return nil, false, err
	}

	return buf.Bytes(), changed, nil
}

// needsPaintOrderDesugar returns true if stroke is ordered before fill and both stroke and fill are present.
func needsPaintOrderDesugar(paintOrder string, attrs []xml.Attr, styleAttrIdx int) bool {
	if paintOrder == "" {
		return false
	}

	tokens := strings.Fields(strings.ToLower(paintOrder))
	strokeIdx := -1
	fillIdx := -1
	for i, t := range tokens {
		t = strings.Trim(t, ",;")
		if t == "stroke" && strokeIdx == -1 {
			strokeIdx = i
		}
		if t == "fill" && fillIdx == -1 {
			fillIdx = i
		}
	}

	if strokeIdx == -1 || fillIdx == -1 || strokeIdx > fillIdx {
		return false
	}

	var fillVal, strokeVal, strokeWidthVal string
	if styleAttrIdx >= 0 {
		style := attrs[styleAttrIdx].Value
		fillVal = extractCSSProp(style, "fill")
		strokeVal = extractCSSProp(style, "stroke")
		strokeWidthVal = extractCSSProp(style, "stroke-width")
	}
	for _, attr := range attrs {
		switch attr.Name.Local {
		case "fill":
			if fillVal == "" {
				fillVal = attr.Value
			}
		case "stroke":
			if strokeVal == "" {
				strokeVal = attr.Value
			}
		case "stroke-width":
			if strokeWidthVal == "" {
				strokeWidthVal = attr.Value
			}
		}
	}

	if fillVal == "none" || strokeVal == "" || strokeVal == "none" {
		return false
	}
	if strokeWidthVal != "" {
		sw, _ := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSuffix(strokeWidthVal, "px"), "mm"), 64)
		if sw <= 0 {
			return false
		}
	}

	return true
}

// desugarPaintOrder creates two element clones: one stroke-only, one fill-only.
func desugarPaintOrder(elem *xml.StartElement, styleAttrIdx int) (xml.StartElement, xml.StartElement) {
	strokeElem := elem.Copy()
	fillElem := elem.Copy()

	if styleAttrIdx >= 0 {
		sStyle := removeStyleProp(strokeElem.Attr[styleAttrIdx].Value, "paint-order")
		sStyle = removeStyleProp(sStyle, "fill")
		sStyle += ";fill:none"
		strokeElem.Attr[styleAttrIdx].Value = sStyle

		fStyle := removeStyleProp(fillElem.Attr[styleAttrIdx].Value, "paint-order")
		fStyle = removeStyleProp(fStyle, "stroke")
		fStyle = removeStyleProp(fStyle, "stroke-width")
		fStyle += ";stroke:none"
		fillElem.Attr[styleAttrIdx].Value = fStyle
	} else {
		setOrAppendAttr(&strokeElem, "fill", "none")
		setOrAppendAttr(&fillElem, "stroke", "none")
	}

	for i := range strokeElem.Attr {
		if strokeElem.Attr[i].Name.Local == "id" {
			strokeElem.Attr[i].Value += "_stroke"
		}
	}

	return strokeElem, fillElem
}

func setOrAppendAttr(elem *xml.StartElement, name, val string) {
	for i := range elem.Attr {
		if elem.Attr[i].Name.Local == name {
			elem.Attr[i].Value = val
			return
		}
	}
	elem.Attr = append(elem.Attr, xml.Attr{
		Name:  xml.Name{Local: name},
		Value: val,
	})
}

func extractCSSProp(style, prop string) string {
	for part := range strings.SplitSeq(style, ";") {
		kv := strings.SplitN(strings.TrimSpace(part), ":", 2)
		if len(kv) == 2 && strings.TrimSpace(kv[0]) == prop {
			return strings.TrimSpace(kv[1])
		}
	}
	return ""
}

func isShapeElement(name string) bool {
	switch name {
	case "path", "rect", "circle", "ellipse", "polygon", "polyline", "line":
		return true
	}
	return false
}

// Matrix2D represents a 2D affine transformation matrix:
// [ A C E ]
// [ B D F ]
// [ 0 0 1 ]
type Matrix2D struct {
	A, B, C, D, E, F float64
}

// IdentityMatrix returns the 2D identity transformation matrix.
func IdentityMatrix() Matrix2D {
	return Matrix2D{A: 1, D: 1}
}

// Multiply computes the product of two 2D affine transformation matrices (m1 * m2).
func (m1 Matrix2D) Multiply(m2 Matrix2D) Matrix2D {
	return Matrix2D{
		A: m1.A*m2.A + m1.C*m2.B,
		B: m1.B*m2.A + m1.D*m2.B,
		C: m1.A*m2.C + m1.C*m2.D,
		D: m1.B*m2.C + m1.D*m2.D,
		E: m1.A*m2.E + m1.C*m2.F + m1.E,
		F: m1.B*m2.E + m1.D*m2.F + m1.F,
	}
}

// ScaleFactor returns the effective isotropic scale factor represented by the matrix.
func (m Matrix2D) ScaleFactor() float64 {
	sx := math.Hypot(m.A, m.B)
	sy := math.Hypot(m.C, m.D)
	return math.Sqrt(sx * sy)
}

// parseTransform parses an SVG transform attribute string into a cumulative Matrix2D.
func parseTransform(s string) Matrix2D {
	res := IdentityMatrix()
	s = strings.TrimSpace(s)
	for len(s) > 0 {
		idx := strings.IndexByte(s, '(')
		if idx == -1 {
			break
		}
		cmd := strings.TrimSpace(s[:idx])
		endIdx := strings.IndexByte(s[idx:], ')')
		if endIdx == -1 {
			break
		}
		argsStr := s[idx+1 : idx+endIdx]
		s = strings.TrimSpace(s[idx+endIdx+1:])

		fields := strings.FieldsFunc(argsStr, func(r rune) bool {
			return unicode.IsSpace(r) || r == ','
		})
		var nums []float64
		for _, f := range fields {
			if v, err := strconv.ParseFloat(f, 64); err == nil {
				nums = append(nums, v)
			}
		}

		var cur Matrix2D
		switch strings.ToLower(cmd) {
		case "matrix":
			if len(nums) >= 6 {
				cur = Matrix2D{A: nums[0], B: nums[1], C: nums[2], D: nums[3], E: nums[4], F: nums[5]}
			} else {
				cur = IdentityMatrix()
			}
		case "scale":
			switch {
			case len(nums) == 1:
				cur = Matrix2D{A: nums[0], D: nums[0]}
			case len(nums) >= 2:
				cur = Matrix2D{A: nums[0], D: nums[1]}
			default:
				cur = IdentityMatrix()
			}
		case "translate":
			switch {
			case len(nums) == 1:
				cur = Matrix2D{A: 1, D: 1, E: nums[0]}
			case len(nums) >= 2:
				cur = Matrix2D{A: 1, D: 1, E: nums[0], F: nums[1]}
			default:
				cur = IdentityMatrix()
			}
		case "rotate":
			if len(nums) >= 1 {
				rad := nums[0] * math.Pi / 180.0
				cosA := math.Cos(rad)
				sinA := math.Sin(rad)
				if len(nums) >= 3 {
					cx, cy := nums[1], nums[2]
					t1 := Matrix2D{A: 1, D: 1, E: cx, F: cy}
					rot := Matrix2D{A: cosA, B: sinA, C: -sinA, D: cosA}
					t2 := Matrix2D{A: 1, D: 1, E: -cx, F: -cy}
					cur = t1.Multiply(rot).Multiply(t2)
				} else {
					cur = Matrix2D{A: cosA, B: sinA, C: -sinA, D: cosA}
				}
			} else {
				cur = IdentityMatrix()
			}
		case "skewx":
			if len(nums) >= 1 {
				rad := nums[0] * math.Pi / 180.0
				cur = Matrix2D{A: 1, D: 1, C: math.Tan(rad)}
			} else {
				cur = IdentityMatrix()
			}
		case "skewy":
			if len(nums) >= 1 {
				rad := nums[0] * math.Pi / 180.0
				cur = Matrix2D{A: 1, D: 1, B: math.Tan(rad)}
			} else {
				cur = IdentityMatrix()
			}
		default:
			cur = IdentityMatrix()
		}

		res = res.Multiply(cur)
	}
	return res
}

func parseStrokeWidth(s string) (float64, string) {
	s = strings.TrimSpace(s)
	units := []string{"px", "pt", "mm", "cm", "in", "pc"}
	unit := ""
	for _, u := range units {
		if strings.HasSuffix(s, u) {
			unit = u
			s = strings.TrimSuffix(s, u)
			break
		}
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, ""
	}
	return v, unit
}

func setStyleProp(style, prop, val string) string {
	cleaned := removeStyleProp(style, prop)
	if cleaned == "" {
		return prop + ":" + val
	}
	return cleaned + ";" + prop + ":" + val
}
