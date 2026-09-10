package inksvg

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"regexp"
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
	fillRuleStack := []string{"nonzero"}
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

			// Prune hidden elements, groups, and layers (Issue #61)
			if isElementHidden(name, elem.Attr) {
				if err := decoder.Skip(); err != nil {
					return nil, fmt.Errorf("xml skip hidden element %s: %w", name, err)
				}
				continue
			}

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

				// Emit specialized gradient variants for group transforms (Issue #56)
				if specs, ok := meta.SpecializedByParent[gradID]; ok && gradDef != nil {
					for _, spec := range specs {
						var specAttrs []xml.Attr
						for _, a := range gradDef.Attrs {
							switch a.Name.Local {
							case "id":
								specAttrs = append(specAttrs, xml.Attr{Name: a.Name, Value: spec.NewID})
							case "gradientTransform", "href":
								continue
							default:
								specAttrs = append(specAttrs, a)
							}
						}
						matStr := fmt.Sprintf("matrix(%f %f %f %f %f %f)", spec.Transform.A, spec.Transform.B, spec.Transform.C, spec.Transform.D, spec.Transform.E, spec.Transform.F)
						specAttrs = append(specAttrs, xml.Attr{Name: xml.Name{Local: "gradientTransform"}, Value: matStr})
						specStart := xml.StartElement{Name: elem.Name, Attr: specAttrs}
						if err := encoder.EncodeToken(specStart); err != nil {
							return nil, err
						}
						for _, st := range gradDef.Stops {
							if err := encoder.EncodeToken(st); err != nil {
								return nil, err
							}
						}
						if err := encoder.EncodeToken(xml.EndElement{Name: elem.Name}); err != nil {
							return nil, err
						}
					}
				}
				continue
			}

			// Normalize gradient stop style attributes to XML attributes (Issue #55)
			if name == "stop" {
				elem.Attr = normalizeStopAttrs(elem.Attr)
			}

			// Stage 1: Convert SVG <text> elements into <path> vectors (Issue #21)
			if name == "text" {
				if err := ProcessTextElementToPaths(elem, decoder, encoder, &transformStack); err != nil {
					return nil, fmt.Errorf("text conversion failed: %w", err)
				}
				continue
			}

			// Track 2D affine transformation matrices and fill-rules on nested <g> elements
			if name == "g" {
				curMatrix := transformStack[len(transformStack)-1]
				curRule := fillRuleStack[len(fillRuleStack)-1]
				for i, attr := range elem.Attr {
					if attr.Name.Local == "transform" {
						parsed := parseTransform(attr.Value)
						curMatrix = curMatrix.Multiply(parsed)
						// Normalize transform to canonical matrix to avoid oksvg single-param scale(s, 0) bug
						elem.Attr[i].Value = fmt.Sprintf("matrix(%f %f %f %f %f %f)", parsed.A, parsed.B, parsed.C, parsed.D, parsed.E, parsed.F)
					}
					if attr.Name.Local == "fill-rule" {
						curRule = strings.ToLower(strings.TrimSpace(attr.Value))
					}
					if attr.Name.Local == "style" {
						if fr := extractCSSProp(attr.Value, "fill-rule"); fr != "" {
							curRule = strings.ToLower(strings.TrimSpace(fr))
						}
					}
				}
				transformStack = append(transformStack, curMatrix)
				fillRuleStack = append(fillRuleStack, curRule)
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

				// Desugar implicit repeated arc commands in path d data (Issue #57)
				if dAttrIdx >= 0 {
					elem.Attr[dAttrIdx].Value = DesugarPathArcs(elem.Attr[dAttrIdx].Value)
				}

				// Normalize evenodd fill-rules to NonZero winding (Issue #62)
				pathRule := fillRuleStack[len(fillRuleStack)-1]
				var fillRuleAttrIdx = -1
				var pathStyleAttrIdx = -1
				for i, attr := range elem.Attr {
					if attr.Name.Local == "fill-rule" {
						fillRuleAttrIdx = i
						pathRule = strings.ToLower(strings.TrimSpace(attr.Value))
					}
					if attr.Name.Local == "style" {
						pathStyleAttrIdx = i
						if fr := extractCSSProp(attr.Value, "fill-rule"); fr != "" {
							pathRule = strings.ToLower(strings.TrimSpace(fr))
						}
					}
				}
				if pathRule == "evenodd" && dAttrIdx >= 0 {
					elem.Attr[dAttrIdx].Value = NormalizeEvenOddPath(elem.Attr[dAttrIdx].Value)
					if fillRuleAttrIdx >= 0 {
						elem.Attr[fillRuleAttrIdx].Value = "nonzero"
					}
					if pathStyleAttrIdx >= 0 {
						elem.Attr[pathStyleAttrIdx].Value = setStyleProp(elem.Attr[pathStyleAttrIdx].Value, "fill-rule", "nonzero")
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

			// Default stroke-linejoin="miter" on stroked shapes and groups (Issue #64).
			// Upstream oksvg defaults omitted stroke-linejoin to rasterx.Bevel (4) instead of
			// rasterx.Miter (2), causing 45° beveled corners instead of standard SVG 90° miter joins.
			if isShapeElement(name) || name == "g" {
				var hasStroke bool
				var hasLineJoin bool
				for _, attr := range elem.Attr {
					if attr.Name.Local == "stroke" && attr.Value != "" && attr.Value != "none" {
						hasStroke = true
					}
					if attr.Name.Local == "stroke-linejoin" && attr.Value != "" {
						hasLineJoin = true
					}
					if attr.Name.Local == "style" {
						sVal := extractCSSProp(attr.Value, "stroke")
						if sVal != "" && sVal != "none" {
							hasStroke = true
						}
						if extractCSSProp(attr.Value, "stroke-linejoin") != "" {
							hasLineJoin = true
						}
					}
				}
				if hasStroke && !hasLineJoin {
					elem.Attr = append(elem.Attr, xml.Attr{
						Name:  xml.Name{Local: "stroke-linejoin"},
						Value: "miter",
					})
				}
			}

			// Rewrite userSpaceOnUse gradient references for group transforms (Issue #56)
			elemMatrix := transformStack[len(transformStack)-1]
			for _, a := range elem.Attr {
				if a.Name.Local == "transform" {
					elemMatrix = elemMatrix.Multiply(parseTransform(a.Value))
				}
			}
			if elemMatrix != IdentityMatrix() {
				rewrites := make(map[string]string)
				for _, a := range elem.Attr {
					for _, match := range reURLGrad.FindAllStringSubmatch(a.Value, -1) {
						gradID := match[1]
						if g, ok := meta.Gradients[gradID]; ok && g.IsUserSpaceOnUse(meta.Gradients) {
							eff := elemMatrix.Multiply(g.GetGradientTransform(meta.Gradients))
							key := fmt.Sprintf("%.4f_%.4f_%.4f_%.4f_%.4f_%.4f", eff.A, eff.B, eff.C, eff.D, eff.E, eff.F)
							if newID, exists := meta.SpecializedLookup[gradID][key]; exists {
								rewrites[gradID] = newID
							}
						}
					}
				}
				if len(rewrites) > 0 {
					for i, a := range elem.Attr {
						elem.Attr[i].Value = reURLGrad.ReplaceAllStringFunc(a.Value, func(m string) string {
							sub := reURLGrad.FindStringSubmatch(m)
							if len(sub) == 2 {
								if newID, ok := rewrites[sub[1]]; ok {
									return fmt.Sprintf("url(#%s)", newID)
								}
							}
							return m
						})
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
			if elem.Name.Local == "g" {
				if len(transformStack) > 1 {
					transformStack = transformStack[:len(transformStack)-1]
				}
				if len(fillRuleStack) > 1 {
					fillRuleStack = fillRuleStack[:len(fillRuleStack)-1]
				}
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

// SpecializedGradient represents a cloned gradient with an ancestor group transform applied (Issue #56).
type SpecializedGradient struct {
	NewID     string
	Parent    *GradientDef
	Transform Matrix2D
}

// DocumentMetadata collects document-level metadata during the first XML pass.
type DocumentMetadata struct {
	Effects             map[string]PathEffect
	ElementsByID        map[string][]xml.Token
	Gradients           map[string]*GradientDef
	SpecializedByParent map[string][]*SpecializedGradient
	SpecializedLookup   map[string]map[string]string // gradID -> matrixKey -> newID
	VBX, VBY            float64
	VBW, VBH            float64
	Width               float64
	Height              float64
	PageColor           string
	PageOpacity         float64
	HasPageColor        bool
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
					normalized := tok.Copy()
					normalized.Attr = normalizeStopAttrs(normalized.Attr)
					curGrad.Stops = append(curGrad.Stops, normalized)
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

	// Resolve specialized gradients for userSpaceOnUse in transformed groups (Issue #56)
	meta.resolveSpecializedGradients(data)

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

// isElementHidden reports whether an SVG element or group is styled or configured
// to be non-rendering via display="none", visibility="hidden", or corresponding inline styles (Issue #61).
func isElementHidden(name string, attrs []xml.Attr) bool {
	switch name {
	case "svg", "defs", "linearGradient", "radialGradient", "pattern", "clipPath", "mask", "filter", "style":
		return false
	}
	// Never prune Inkscape animation layers; InkAnim's frame builder dynamically
	// manages layer visibility when synthesizing animation frames.
	for _, a := range attrs {
		if a.Name.Local == "groupmode" && a.Value == "layer" {
			return false
		}
	}
	for _, a := range attrs {
		switch a.Name.Local {
		case "display":
			val := strings.ToLower(strings.TrimSpace(a.Value))
			if strings.HasPrefix(val, "none") {
				return true
			}
		case "visibility":
			val := strings.ToLower(strings.TrimSpace(a.Value))
			if strings.HasPrefix(val, "hidden") || strings.HasPrefix(val, "collapse") {
				return true
			}
		case "style":
			d := strings.ToLower(extractCSSProp(a.Value, "display"))
			if strings.HasPrefix(d, "none") {
				return true
			}
			v := strings.ToLower(extractCSSProp(a.Value, "visibility"))
			if strings.HasPrefix(v, "hidden") || strings.HasPrefix(v, "collapse") {
				return true
			}
		}
	}
	return false
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

// normalizeStopAttrs extracts stop-color and stop-opacity from style="..." on <stop> elements
// and ensures they are set as explicit XML attributes for oksvg compatibility (Issue #55).
func normalizeStopAttrs(attrs []xml.Attr) []xml.Attr {
	var hasStopColor, hasStopOpacity bool
	var styleVal string
	for _, a := range attrs {
		switch a.Name.Local {
		case "stop-color":
			hasStopColor = true
		case "stop-opacity":
			hasStopOpacity = true
		case "style":
			styleVal = a.Value
		}
	}

	if styleVal == "" || (hasStopColor && hasStopOpacity) {
		return attrs
	}

	var extractedColor, extractedOpacity string
	for _, part := range strings.Split(styleVal, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, ":", 2)
		if len(kv) != 2 {
			continue
		}
		k := strings.TrimSpace(kv[0])
		v := strings.TrimSpace(kv[1])
		if k == "stop-color" && !hasStopColor {
			extractedColor = v
		} else if k == "stop-opacity" && !hasStopOpacity {
			extractedOpacity = v
		}
	}

	result := attrs
	if extractedColor != "" {
		result = append(result, xml.Attr{Name: xml.Name{Local: "stop-color"}, Value: extractedColor})
	}
	if extractedOpacity != "" {
		result = append(result, xml.Attr{Name: xml.Name{Local: "stop-opacity"}, Value: extractedOpacity})
	}
	return result
}

var reURLGrad = regexp.MustCompile(`url\(#([a-zA-Z0-9_\-\.:]+)\)`)

// IsUserSpaceOnUse checks if the gradient has gradientUnits="userSpaceOnUse", inheriting from Href if needed.
func (g *GradientDef) IsUserSpaceOnUse(gradients map[string]*GradientDef) bool {
	curr := g
	for depth := 0; depth < 10 && curr != nil; depth++ {
		for _, a := range curr.Attrs {
			if a.Name.Local == "gradientUnits" {
				return a.Value == "userSpaceOnUse"
			}
		}
		if curr.Href != "" {
			curr = gradients[curr.Href]
		} else {
			break
		}
	}
	return false
}

// GetGradientTransform retrieves the gradient's gradientTransform matrix, inheriting from Href if needed.
func (g *GradientDef) GetGradientTransform(gradients map[string]*GradientDef) Matrix2D {
	curr := g
	for depth := 0; depth < 10 && curr != nil; depth++ {
		for _, a := range curr.Attrs {
			if a.Name.Local == "gradientTransform" {
				return parseTransform(a.Value)
			}
		}
		if curr.Href != "" {
			curr = gradients[curr.Href]
		} else {
			break
		}
	}
	return IdentityMatrix()
}

// resolveSpecializedGradients scans elements to identify userSpaceOnUse gradients referenced within transformed groups.
func (meta *DocumentMetadata) resolveSpecializedGradients(data []byte) {
	meta.SpecializedByParent = make(map[string][]*SpecializedGradient)
	meta.SpecializedLookup = make(map[string]map[string]string)
	specCount := 0

	dec := xml.NewDecoder(bytes.NewReader(data))
	transformStack := []Matrix2D{IdentityMatrix()}

	for {
		token, err := dec.Token()
		if err != nil {
			break
		}
		switch tok := token.(type) {
		case xml.StartElement:
			name := tok.Name.Local
			curMatrix := transformStack[len(transformStack)-1]
			if name == "g" {
				for _, a := range tok.Attr {
					if a.Name.Local == "transform" {
						curMatrix = curMatrix.Multiply(parseTransform(a.Value))
					}
				}
				transformStack = append(transformStack, curMatrix)
			} else {
				elemMatrix := curMatrix
				for _, a := range tok.Attr {
					if a.Name.Local == "transform" {
						elemMatrix = elemMatrix.Multiply(parseTransform(a.Value))
					}
				}
				if elemMatrix != IdentityMatrix() {
					for _, a := range tok.Attr {
						for _, match := range reURLGrad.FindAllStringSubmatch(a.Value, -1) {
							gradID := match[1]
							if g, ok := meta.Gradients[gradID]; ok && g.IsUserSpaceOnUse(meta.Gradients) {
								eff := elemMatrix.Multiply(g.GetGradientTransform(meta.Gradients))
								key := fmt.Sprintf("%.4f_%.4f_%.4f_%.4f_%.4f_%.4f", eff.A, eff.B, eff.C, eff.D, eff.E, eff.F)
								if meta.SpecializedLookup[gradID] == nil {
									meta.SpecializedLookup[gradID] = make(map[string]string)
								}
								if _, exists := meta.SpecializedLookup[gradID][key]; !exists {
									specCount++
									newID := fmt.Sprintf("%s__t%d", gradID, specCount)
									meta.SpecializedLookup[gradID][key] = newID
									spec := &SpecializedGradient{
										NewID:     newID,
										Parent:    g,
										Transform: eff,
									}
									meta.SpecializedByParent[gradID] = append(meta.SpecializedByParent[gradID], spec)
								}
							}
						}
					}
				}
			}
		case xml.EndElement:
			if tok.Name.Local == "g" && len(transformStack) > 1 {
				transformStack = transformStack[:len(transformStack)-1]
			}
		}
	}
}

func isPathCommand(r rune) bool {
	switch r {
	case 'M', 'm', 'L', 'l', 'H', 'h', 'V', 'v',
		'C', 'c', 'S', 's', 'Q', 'q', 'T', 't',
		'A', 'a', 'Z', 'z':
		return true
	}
	return false
}

// tokenizeArcParams extracts individual numeric parameter strings for an arc command body.
// It properly handles commas, whitespace, signs, multiple decimal points, and concatenated arc flags.
func tokenizeArcParams(s string) []string {
	var tokens []string
	var cur strings.Builder

	flush := func() {
		if cur.Len() > 0 {
			tokens = append(tokens, cur.String())
			cur.Reset()
		}
	}

	runes := []rune(s)
	n := len(runes)

	for i := 0; i < n; i++ {
		r := runes[i]
		if r == ',' || unicode.IsSpace(r) {
			flush()
			continue
		}
		if r == '-' || r == '+' {
			prev := ' '
			if cur.Len() > 0 {
				prevR := []rune(cur.String())
				prev = prevR[len(prevR)-1]
			}
			if prev != 'e' && prev != 'E' {
				flush()
			}
			cur.WriteRune(r)
			continue
		}
		if r == '.' {
			if strings.ContainsRune(cur.String(), '.') {
				flush()
			}
			cur.WriteRune(r)
			continue
		}
		// Check if we are expecting an arc flag (param index 3 or 4 within a 7-param group)
		// and the current token starts with '0' or '1'.
		paramIdx := len(tokens) % 7
		if (paramIdx == 3 || paramIdx == 4) && cur.Len() == 1 {
			c := cur.String()
			if c == "0" || c == "1" {
				flush()
			}
		}

		cur.WriteRune(r)
	}
	flush()
	return tokens
}

// DesugarPathArcs converts implicit repeated arc commands in SVG path data into explicit arc commands (Issue #57).
// For example: "M 0 0 a 1 2 3 4 5 6 7 8 9 10 11 12 13 14 Z" -> "M 0 0 a 1 2 3 4 5 6 7 a 8 9 10 11 12 13 14 Z"
func DesugarPathArcs(d string) string {
	type segment struct {
		cmd  rune
		body string
	}
	var segs []segment
	lastIdx := -1
	runes := []rune(d)

	for i, r := range runes {
		if isPathCommand(r) {
			if lastIdx != -1 {
				segs = append(segs, segment{
					cmd:  runes[lastIdx],
					body: string(runes[lastIdx+1 : i]),
				})
			}
			lastIdx = i
		}
	}
	if lastIdx != -1 {
		segs = append(segs, segment{
			cmd:  runes[lastIdx],
			body: string(runes[lastIdx+1:]),
		})
	}

	hasImplicitArcs := false
	for _, seg := range segs {
		if seg.cmd == 'a' || seg.cmd == 'A' {
			nums := tokenizeArcParams(seg.body)
			if len(nums) > 7 && len(nums)%7 == 0 {
				hasImplicitArcs = true
				break
			}
		}
	}
	if !hasImplicitArcs {
		return d
	}

	var sb strings.Builder
	for _, seg := range segs {
		if seg.cmd == 'a' || seg.cmd == 'A' {
			nums := tokenizeArcParams(seg.body)
			if len(nums) > 7 && len(nums)%7 == 0 {
				for i := 0; i < len(nums); i += 7 {
					if sb.Len() > 0 {
						sb.WriteByte(' ')
					}
					sb.WriteRune(seg.cmd)
					sb.WriteByte(' ')
					sb.WriteString(strings.Join(nums[i:i+7], " "))
				}
				continue
			}
		}
		if sb.Len() > 0 {
			sb.WriteByte(' ')
		}
		sb.WriteRune(seg.cmd)
		sb.WriteString(seg.body)
	}
	return sb.String()
}

