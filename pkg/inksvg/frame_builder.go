package inksvg

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"strings"
)

// BuildLayerFrameSVG generates an SVG where only the target layer and any pinned layers are visible,
// cropped to the specified boundary rectangle. If boundary has zero dimensions, the document's native rect is used.
// Pinned background layers are always placed BEFORE the target layer in the SVG DOM,
// guaranteeing that they render in the background regardless of their position in the source document.
func BuildLayerFrameSVG(doc *SVGDocument, targetLayerID string, pinnedLayerIDs map[string]bool, boundary Rect) ([]byte, error) {
	if boundary.Width <= 0 || boundary.Height <= 0 {
		boundary = doc.GetDocumentRect()
	}

	decoder := xml.NewDecoder(bytes.NewReader(doc.RawContent))

	var beforeLayers []xml.Token
	var pinnedLayers [][]xml.Token
	var targetLayer []xml.Token
	var afterLayers []xml.Token

	var currentLayerTokens []xml.Token
	var currentLayerID string
	var inLayer bool
	var layerDepth int
	var firstLayerSeen bool
	var processedRoot bool

	for {
		token, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("xml transform error: %w", err)
		}

		if !inLayer {
			switch elem := token.(type) {
			case xml.StartElement:
				if elem.Name.Local == "svg" && !processedRoot {
					processedRoot = true
					modifiedElem := elem.Copy()
					applyBoundaryToSVG(&modifiedElem, boundary)
					beforeLayers = append(beforeLayers, modifiedElem)
					continue
				}

				if elem.Name.Local == "g" {
					var isLayer bool
					var layerID string
					for _, attr := range elem.Attr {
						if attr.Name.Local == "groupmode" && attr.Value == "layer" {
							isLayer = true
						}
						if attr.Name.Local == "id" {
							layerID = attr.Value
						}
					}

					if isLayer {
						inLayer = true
						layerDepth = 1
						currentLayerID = layerID
						firstLayerSeen = true

						// Ensure layer is visible (override display:none if present)
						modifiedElem := elem.Copy()
						ensureLayerVisible(&modifiedElem)
						currentLayerTokens = []xml.Token{modifiedElem}
						continue
					}
				}

				if !firstLayerSeen {
					beforeLayers = append(beforeLayers, xml.CopyToken(token))
				} else {
					afterLayers = append(afterLayers, xml.CopyToken(token))
				}

			default:
				if !firstLayerSeen {
					beforeLayers = append(beforeLayers, xml.CopyToken(token))
				} else {
					afterLayers = append(afterLayers, xml.CopyToken(token))
				}
			}
		} else {
			// Inside a layer subtree
			switch elem := token.(type) {
			case xml.StartElement:
				layerDepth++
				currentLayerTokens = append(currentLayerTokens, xml.CopyToken(elem))
			case xml.EndElement:
				layerDepth--
				currentLayerTokens = append(currentLayerTokens, xml.CopyToken(elem))
				if layerDepth == 0 {
					inLayer = false
					if currentLayerID == targetLayerID {
						targetLayer = currentLayerTokens
					} else if pinnedLayerIDs != nil && pinnedLayerIDs[currentLayerID] {
						pinnedLayers = append(pinnedLayers, currentLayerTokens)
					}
					currentLayerTokens = nil
					currentLayerID = ""
				}
			default:
				currentLayerTokens = append(currentLayerTokens, xml.CopyToken(token))
			}
		}
	}

	// Now serialize the resulting SVG in correct painter's order:
	// 1. Tokens before the layers (SVG header, defs, metadata)
	// 2. All pinned background layers
	// 3. The active target layer
	// 4. Tokens after the layers (closing </svg>, etc.)
	var buf bytes.Buffer
	encoder := xml.NewEncoder(&buf)

	for _, tok := range beforeLayers {
		if err := encoder.EncodeToken(tok); err != nil {
			return nil, err
		}
	}

	for _, layerToks := range pinnedLayers {
		for _, tok := range layerToks {
			if err := encoder.EncodeToken(tok); err != nil {
				return nil, err
			}
		}
	}

	for _, tok := range targetLayer {
		if err := encoder.EncodeToken(tok); err != nil {
			return nil, err
		}
	}

	for _, tok := range afterLayers {
		if err := encoder.EncodeToken(tok); err != nil {
			return nil, err
		}
	}

	if err := encoder.Flush(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func ensureLayerVisible(elem *xml.StartElement) {
	var foundStyle bool
	for i, attr := range elem.Attr {
		if attr.Name.Local == "style" {
			foundStyle = true
			cleaned := removeStyleProp(attr.Value, "display")
			if cleaned != "" {
				elem.Attr[i].Value = cleaned + ";display:inline"
			} else {
				elem.Attr[i].Value = "display:inline"
			}
		} else if attr.Name.Local == "display" && attr.Value == "none" {
			elem.Attr[i].Value = "inline"
		}
	}
	if !foundStyle {
		elem.Attr = append(elem.Attr, xml.Attr{
			Name:  xml.Name{Local: "style"},
			Value: "display:inline",
		})
	}
}

// BuildPageFrameSVG generates an SVG where the viewBox and dimensions correspond to the given boundary.
// If boundary has zero dimensions, the page's native (x, y, width, height) is used.
func BuildPageFrameSVG(doc *SVGDocument, page Page, boundary Rect) ([]byte, error) {
	if boundary.Width <= 0 || boundary.Height <= 0 {
		boundary = Rect{
			X:      page.X,
			Y:      page.Y,
			Width:  page.Width,
			Height: page.Height,
		}
	}
	if boundary.Width <= 0 {
		boundary.Width = 512
	}
	if boundary.Height <= 0 {
		boundary.Height = 512
	}

	decoder := xml.NewDecoder(bytes.NewReader(doc.RawContent))
	var buf bytes.Buffer
	encoder := xml.NewEncoder(&buf)
	var processedRoot bool

	for {
		token, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("xml transform error: %w", err)
		}

		switch elem := token.(type) {
		case xml.StartElement:
			if elem.Name.Local == "svg" && !processedRoot {
				processedRoot = true
				applyBoundaryToSVG(&elem, boundary)
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

// applyBoundaryToSVG applies viewBox, width, height, and overflow:hidden to the root SVG element.
func applyBoundaryToSVG(elem *xml.StartElement, boundary Rect) {
	newViewBox := fmt.Sprintf("%f %f %f %f", boundary.X, boundary.Y, boundary.Width, boundary.Height)
	newW := fmt.Sprintf("%f", boundary.Width)
	newH := fmt.Sprintf("%f", boundary.Height)

	var vbFound, wFound, hFound, styleFound bool
	for i := range elem.Attr {
		switch elem.Attr[i].Name.Local {
		case "viewBox":
			elem.Attr[i].Value = newViewBox
			vbFound = true
		case "width":
			elem.Attr[i].Value = newW
			wFound = true
		case "height":
			elem.Attr[i].Value = newH
			hFound = true
		case "style":
			styleFound = true
			cleaned := removeStyleProp(elem.Attr[i].Value, "overflow")
			if cleaned != "" {
				elem.Attr[i].Value = cleaned + ";overflow:hidden"
			} else {
				elem.Attr[i].Value = "overflow:hidden"
			}
		}
	}
	if !vbFound {
		elem.Attr = append(elem.Attr, xml.Attr{Name: xml.Name{Local: "viewBox"}, Value: newViewBox})
	}
	if !wFound {
		elem.Attr = append(elem.Attr, xml.Attr{Name: xml.Name{Local: "width"}, Value: newW})
	}
	if !hFound {
		elem.Attr = append(elem.Attr, xml.Attr{Name: xml.Name{Local: "height"}, Value: newH})
	}
	if !styleFound {
		elem.Attr = append(elem.Attr, xml.Attr{Name: xml.Name{Local: "style"}, Value: "overflow:hidden"})
	}
}


// removeStyleProp removes a CSS property from a style string, e.g. "display:none;opacity:1" -> "opacity:1"
func removeStyleProp(style, prop string) string {
	parts := strings.Split(style, ";")
	var kept []string
	prefix := prop + ":"
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" || strings.HasPrefix(trimmed, prefix) {
			continue
		}
		kept = append(kept, trimmed)
	}
	return strings.Join(kept, ";")
}

func setElementHidden(elem *xml.StartElement) {
	var foundStyle bool
	var filtered []xml.Attr
	for _, attr := range elem.Attr {
		if attr.Name.Local == "groupmode" && attr.Value == "layer" {
			continue
		}
		switch attr.Name.Local {
		case "style":
			foundStyle = true
			cleaned := removeStyleProp(attr.Value, "display")
			if cleaned != "" {
				attr.Value = cleaned + ";display:none"
			} else {
				attr.Value = "display:none"
			}
		case "display":
			attr.Value = "none"
		}
		filtered = append(filtered, attr)
	}
	if !foundStyle {
		filtered = append(filtered, xml.Attr{
			Name:  xml.Name{Local: "style"},
			Value: "display:none",
		})
	}
	elem.Attr = filtered
}

func setElementVisible(elem *xml.StartElement) {
	for i, attr := range elem.Attr {
		if attr.Name.Local == "style" {
			cleaned := removeStyleProp(attr.Value, "display")
			elem.Attr[i].Value = cleaned
		} else if attr.Name.Local == "display" && attr.Value == "none" {
			elem.Attr[i].Value = "inline"
		}
	}
}

func setElementOpacity(elem *xml.StartElement, opacity float64) {
	for i, attr := range elem.Attr {
		if attr.Name.Local == "style" {
			elem.Attr[i].Value = removeStyleProp(attr.Value, "opacity")
		}
	}

	if math.Abs(opacity-1.0) < 1e-4 {
		var filtered []xml.Attr
		for _, attr := range elem.Attr {
			if attr.Name.Local != "opacity" {
				filtered = append(filtered, attr)
			}
		}
		elem.Attr = filtered
		return
	}

	opacityStr := fmt.Sprintf("%.4f", opacity)
	var found bool
	for i, attr := range elem.Attr {
		if attr.Name.Local == "opacity" {
			elem.Attr[i].Value = opacityStr
			found = true
			break
		}
	}
	if !found {
		elem.Attr = append(elem.Attr, xml.Attr{
			Name:  xml.Name{Local: "opacity"},
			Value: opacityStr,
		})
	}
}

// BuildTimelineFrameSVG generates an SVG frame for a timeline animation.
// It hides any path with a "Movement" label and injects translate transforms into animated groups.
func BuildTimelineFrameSVG(doc *SVGDocument, frameIndex int, boundary Rect) ([]byte, error) {
	if boundary.Width <= 0 || boundary.Height <= 0 {
		boundary = doc.GetDocumentRect()
	}

	decoder := xml.NewDecoder(bytes.NewReader(doc.RawContent))
	var buf bytes.Buffer
	encoder := xml.NewEncoder(&buf)
	var processedRoot bool

	// Map GroupID to slice of MotionPaths for multi-motion support
	motionMap := make(map[string][]MotionPath)
	for _, mp := range doc.MotionPaths {
		motionMap[mp.GroupID] = append(motionMap[mp.GroupID], mp)
	}

	for {
		token, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("xml transform error: %w", err)
		}

		switch elem := token.(type) {
		case xml.StartElement:
			if elem.Name.Local == "svg" && !processedRoot {
				processedRoot = true
				applyBoundaryToSVG(&elem, boundary)
				if err := encoder.EncodeToken(elem); err != nil {
					return nil, err
				}
				continue
			}

			// Hide the motion paths or markers themselves
			if elem.Name.Local == "path" || elem.Name.Local == "circle" || elem.Name.Local == "rect" || elem.Name.Local == "ellipse" || elem.Name.Local == "line" {
				var isMotionPath bool
				for _, attr := range elem.Attr {
					if attr.Name.Local == "label" {
						if _, ok := parseMotionConfig(attr.Value); ok {
							isMotionPath = true
							break
						}
					}
				}
				if isMotionPath {
					if err := decoder.Skip(); err != nil {
						return nil, err
					}
					continue
				}
			}

			// Apply translation, rotation, scaling, and visibility to animated groups
			if elem.Name.Local == "g" {
				var id string
				for _, attr := range elem.Attr {
					if attr.Name.Local == "id" {
						id = attr.Value
						break
					}
				}
				if paths, ok := motionMap[id]; ok && len(paths) > 0 {
					frame1Idx := frameIndex + 1 // 1-based index for logic
					maxF := len(doc.Layers)

					var activePaths []MotionPath
					var hasShowRules bool
					var isShown bool
					var hasHideRules bool
					var isHidden bool

					for _, mp := range paths {
						startF := mp.Config.StartFrame
						endF := mp.Config.EndFrame
						if mp.Config.IsAll {
							startF = 1
							endF = maxF
						}
						inRange := frame1Idx >= startF && frame1Idx <= endF
						if inRange {
							activePaths = append(activePaths, mp)
						}

						switch mp.Config.VisibilityState {
						case "show":
							hasShowRules = true
							if inRange {
								isShown = true
							}
						case "hide":
							hasHideRules = true
							if inRange {
								isHidden = true
							}
						}
					}

					groupHidden := false
					switch {
					case hasShowRules && !isShown:
						groupHidden = true
					case isHidden:
						groupHidden = true
					case !hasShowRules && !hasHideRules:
						// Group has no explicit Show/Hide rules:
						// Hide if outside active range of all defined motion paths
						if len(activePaths) == 0 {
							groupHidden = true
						}
					}

					if groupHidden {
						setElementHidden(&elem)
						if err := encoder.EncodeToken(elem); err != nil {
							return nil, err
						}
						if err := decoder.Skip(); err != nil {
							return nil, err
						}
						if err := encoder.EncodeToken(elem.End()); err != nil {
							return nil, err
						}
						continue
					}

					if hasShowRules || hasHideRules {
						setElementVisible(&elem)
					}

					var totalDx, totalDy, totalRot float64
					var rotPivotX, rotPivotY float64
					var rotPivotDetermined bool

					totalSx := 1.0
					totalSy := 1.0
					var scalePivotX, scalePivotY float64
					var scalePivotDetermined bool

					totalOpacity := 1.0
					var hasActiveFade bool

					for _, mp := range activePaths {
						startF := mp.Config.StartFrame
						endF := mp.Config.EndFrame
						if mp.Config.IsAll {
							startF = 1
							endF = maxF
						}

						duration := endF - startF
						progress := 0.0
						if duration > 0 {
							progress = float64(frame1Idx-startF) / float64(duration)
						}
						easedProgress := ApplyEasing(progress, mp.Config.Ease)
						t := easedProgress
						if mp.Config.Reverse {
							t = 1.0 - easedProgress
						}

						// Evaluate translation (only for Move, not Rot or Scale)
						if mp.Config.Type == "move" && mp.PathData != "" {
							dx, dy, err := EvaluatePathAt(mp.PathData, t)
							if err == nil {
								totalDx += dx
								totalDy += dy
							}
						}

						// Evaluate explicit rotation
						if mp.Config.RotationAngle != 0 {
							rot := mp.Config.RotationAngle * t
							if mp.Config.RotationDir == "ccw" {
								rot = -rot
							}
							totalRot += rot
						}

						// Evaluate orient: true (relative delta from path tangent at t=0)
						if mp.Config.OrientPath && mp.PathData != "" {
							thetaT, errT := EvaluatePathTangentAngle(mp.PathData, t)
							theta0, err0 := EvaluatePathTangentAngle(mp.PathData, 0.0)
							if errT == nil && err0 == nil {
								diff := math.Mod(thetaT-theta0+180.0, 360.0)
								if diff < 0 {
									diff += 360.0
								}
								orientRot := diff - 180.0
								totalRot += orientRot
							}
						}

						// Rotation pivot point determined by first evaluated active configuration with a rotational effect
						hasRotEffect := mp.Config.Type == "rot" || mp.Config.RotationAngle != 0 || mp.Config.OrientPath
						if !rotPivotDetermined && hasRotEffect {
							rotPivotDetermined = true
							rotPivotX, rotPivotY = resolvePivot(doc, id, mp.Config, mp.PathData)
						}

						// Evaluate scale effect
						hasScaleEffect := mp.Config.Type == "scale" ||
							mp.Config.ScaleFromX != 1.0 || mp.Config.ScaleToX != 1.0 ||
							mp.Config.ScaleFromY != 1.0 || mp.Config.ScaleToY != 1.0

						if hasScaleEffect {
							currSx := mp.Config.ScaleFromX + t*(mp.Config.ScaleToX-mp.Config.ScaleFromX)
							currSy := mp.Config.ScaleFromY + t*(mp.Config.ScaleToY-mp.Config.ScaleFromY)
							totalSx *= currSx
							totalSy *= currSy

							if !scalePivotDetermined {
								scalePivotDetermined = true
								scalePivotX, scalePivotY = resolvePivot(doc, id, mp.Config, mp.PathData)
							}
						}

						// Evaluate opacity (Fade)
						if mp.Config.Type == "fade" || mp.Config.HasOpacity {
							currOpacity := mp.Config.OpacityFrom + t*(mp.Config.OpacityTo-mp.Config.OpacityFrom)
							currOpacity = math.Max(0.0, math.Min(1.0, currOpacity))
							totalOpacity *= currOpacity
							hasActiveFade = true
						}
					}

					if hasActiveFade {
						totalOpacity = math.Max(0.0, math.Min(1.0, totalOpacity))
						setElementOpacity(&elem, totalOpacity)
					}

					var transformParts []string
					if totalDx != 0 || totalDy != 0 {
						transformParts = append(transformParts, fmt.Sprintf("translate(%f, %f)", totalDx, totalDy))
					}
					if totalRot != 0 {
						transformParts = append(transformParts, fmt.Sprintf("rotate(%f, %f, %f)", totalRot, rotPivotX, rotPivotY))
					}
					if math.Abs(totalSx-1.0) > 1e-4 || math.Abs(totalSy-1.0) > 1e-4 {
						transformParts = append(transformParts, fmt.Sprintf("translate(%f, %f) scale(%f, %f) translate(%f, %f)", scalePivotX, scalePivotY, totalSx, totalSy, -scalePivotX, -scalePivotY))
					}

					if len(transformParts) > 0 {
						newTransform := strings.Join(transformParts, " ")
						transformFound := false
						for i, attr := range elem.Attr {
							if attr.Name.Local == "transform" {
								elem.Attr[i].Value = newTransform + " " + attr.Value
								transformFound = true
								break
							}
						}
						if !transformFound {
							elem.Attr = append(elem.Attr, xml.Attr{
								Name:  xml.Name{Local: "transform"},
								Value: newTransform,
							})
						}
					}
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

func resolvePivot(doc *SVGDocument, groupID string, config MotionConfig, pathData string) (float64, float64) {
	switch config.PivotType {
	case "node":
		if config.PivotNodeID != "" {
			nodeRect := doc.GetElementRect(config.PivotNodeID)
			return nodeRect.X + nodeRect.Width/2.0, nodeRect.Y + nodeRect.Height/2.0
		}
		groupRect := doc.GetElementRect(groupID)
		return groupRect.X + groupRect.Width/2.0, groupRect.Y + groupRect.Height/2.0
	case "edge":
		groupRect := doc.GetElementRect(groupID)
		return CalculateEdgePivot(groupRect, config.PivotEdgeAngle)
	case "path-start":
		if sx, sy, err := GetPathStartPoint(pathData); err == nil {
			return sx, sy
		}
		groupRect := doc.GetElementRect(groupID)
		return groupRect.X + groupRect.Width/2.0, groupRect.Y + groupRect.Height/2.0
	default: // "center"
		groupRect := doc.GetElementRect(groupID)
		return groupRect.X + groupRect.Width/2.0, groupRect.Y + groupRect.Height/2.0
	}
}


