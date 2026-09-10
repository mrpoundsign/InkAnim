package inksvg

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
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

	// Map GroupID to MotionPath for quick lookup
	motionMap := make(map[string]MotionPath)
	for _, mp := range doc.MotionPaths {
		motionMap[mp.GroupID] = mp
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

			// Hide the motion paths themselves
			if elem.Name.Local == "path" {
				var isMotionPath bool
				for _, attr := range elem.Attr {
					if attr.Name.Local == "label" && strings.HasPrefix(attr.Value, "Movement {") {
						isMotionPath = true
						break
					}
				}
				if isMotionPath {
					if err := decoder.Skip(); err != nil {
						return nil, err
					}
					continue
				}
			}

			// Apply translation and visibility to animated groups
			if elem.Name.Local == "g" {
				var id string
				for _, attr := range elem.Attr {
					if attr.Name.Local == "id" {
						id = attr.Value
						break
					}
				}
				if mp, ok := motionMap[id]; ok {
					startF := mp.Config.StartFrame
					endF := mp.Config.EndFrame
					maxF := len(doc.Layers)
					if mp.Config.IsAll {
						startF = 1
						endF = maxF
					}

					// Hide if outside range
					frame1Idx := frameIndex + 1 // 1-based index for logic
					if frame1Idx < startF || frame1Idx > endF {
						if err := decoder.Skip(); err != nil {
							return nil, err
						}
						continue
					} else {
						// Calculate t
						duration := endF - startF
						t := 0.0
						if duration > 0 {
							t = float64(frame1Idx-startF) / float64(duration)
						}
						t = ApplyEasing(t, mp.Config.Ease)

						dx, dy, err := EvaluatePathAt(mp.PathData, t)
						if err == nil && (dx != 0 || dy != 0) {
							// Inject transform
							transformFound := false
							newTransform := fmt.Sprintf("translate(%f, %f)", dx, dy)
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

func ensureStyleProp(elem *xml.StartElement, prop, value string) {
	var foundStyle bool
	for i, attr := range elem.Attr {
		if attr.Name.Local == "style" {
			foundStyle = true
			cleaned := removeStyleProp(attr.Value, prop)
			if cleaned != "" {
				elem.Attr[i].Value = cleaned + ";" + prop + ":" + value
			} else {
				elem.Attr[i].Value = prop + ":" + value
			}
		}
	}
	if !foundStyle {
		elem.Attr = append(elem.Attr, xml.Attr{
			Name:  xml.Name{Local: "style"},
			Value: prop + ":" + value,
		})
	}
}
