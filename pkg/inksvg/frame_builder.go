package inksvg

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"

	"github.com/srwiley/oksvg"
)


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

type topLevelNode struct {
	id         string
	isDefs     bool
	baseZ      int
	effectiveZ int
	tokens     []xml.Token
}

type timelineTemplate struct {
	preamble   []xml.Token
	rootElem   xml.StartElement
	rootEnd    xml.EndElement
	postTokens []xml.Token
	nodes      []topLevelNode
}

func parseTimelineTemplate(rawContent []byte) (*timelineTemplate, error) {
	decoder := xml.NewDecoder(bytes.NewReader(rawContent))
	var preamble []xml.Token
	var rootElem xml.StartElement
	var rootFound bool
	var rootEnd xml.EndElement
	var postTokens []xml.Token

	var nodes []topLevelNode
	var currentNodeTokens []xml.Token
	var currentDepth int
	var baseZCounter int

	for {
		token, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("xml transform error: %w", err)
		}

		if !rootFound {
			switch elem := token.(type) {
			case xml.StartElement:
				if elem.Name.Local == "svg" {
					rootFound = true
					rootElem = elem
					currentDepth = 1
				} else {
					preamble = append(preamble, xml.CopyToken(token))
				}
			default:
				preamble = append(preamble, xml.CopyToken(token))
			}
			continue
		}

		switch {
		case currentDepth == 1:
			switch elem := token.(type) {
			case xml.StartElement:
				currentDepth++
				currentNodeTokens = []xml.Token{xml.CopyToken(elem)}
			case xml.EndElement:
				if elem.Name.Local == "svg" {
					rootEnd = elem
					currentDepth = 0
				}
			default:
				if len(nodes) == 0 {
					preamble = append(preamble, xml.CopyToken(token))
				} else {
					nodes[len(nodes)-1].tokens = append(nodes[len(nodes)-1].tokens, xml.CopyToken(token))
				}
			}
		case currentDepth > 1:
			currentNodeTokens = append(currentNodeTokens, xml.CopyToken(token))
			switch token.(type) {
			case xml.StartElement:
				currentDepth++
			case xml.EndElement:
				currentDepth--
				if currentDepth == 1 {
					startElem := currentNodeTokens[0].(xml.StartElement)
					var id string
					for _, attr := range startElem.Attr {
						if attr.Name.Local == "id" {
							id = attr.Value
							break
						}
					}
					isDefs := startElem.Name.Local == "defs" ||
						startElem.Name.Local == "metadata" ||
						startElem.Name.Local == "style" ||
						startElem.Name.Space == "http://sodipodi.sourceforge.net/DTD/sodipodi-0.dtd"

					nodes = append(nodes, topLevelNode{
						id:     id,
						isDefs: isDefs,
						baseZ:  baseZCounter,
						tokens: currentNodeTokens,
					})
					baseZCounter++
					currentNodeTokens = nil
				}
			}
		default:
			postTokens = append(postTokens, xml.CopyToken(token))
		}
	}

	return &timelineTemplate{
		preamble:   preamble,
		rootElem:   rootElem,
		rootEnd:    rootEnd,
		postTokens: postTokens,
		nodes:      nodes,
	}, nil
}

// BuildTimelineFrameSVG generates an SVG frame for a timeline animation.
// It hides any motion paths/markers, dynamically reorders top-level groups according to Depth directives,
// and injects translate/rotate/scale/opacity/visibility into animated groups.
func BuildTimelineFrameSVG(doc *SVGDocument, frameIndex int, boundary Rect) ([]byte, error) {
	if boundary.Width <= 0 || boundary.Height <= 0 {
		boundary = doc.GetDocumentRect()
	}

	var tpl *timelineTemplate
	doc.mu.RLock()
	if doc.timelineTpl != nil {
		tpl = doc.timelineTpl.(*timelineTemplate)
	}
	doc.mu.RUnlock()

	if tpl == nil {
		parsedTpl, err := parseTimelineTemplate(doc.RawContent)
		if err != nil {
			return nil, err
		}
		doc.mu.Lock()
		if doc.timelineTpl != nil {
			tpl = doc.timelineTpl.(*timelineTemplate)
		} else {
			doc.timelineTpl = parsedTpl
			tpl = parsedTpl
		}
		doc.mu.Unlock()
	}

	rootElem := tpl.rootElem
	rootElem.Attr = append([]xml.Attr(nil), tpl.rootElem.Attr...)
	applyBoundaryToSVG(&rootElem, boundary)

	preamble := tpl.preamble
	rootEnd := tpl.rootEnd
	postTokens := tpl.postTokens

	nodes := make([]topLevelNode, len(tpl.nodes))
	copy(nodes, tpl.nodes)

	frame1Idx := frameIndex + 1
	maxF := len(doc.Layers)
	if maxF == 0 {
		for _, mp := range doc.MotionPaths {
			if mp.Config.EndFrame > maxF {
				maxF = mp.Config.EndFrame
			}
		}
	}
	if doc.CameraPath != nil && doc.CameraPath.Config.EndFrame > maxF {
		maxF = doc.CameraPath.Config.EndFrame
	}
	if maxF == 0 {
		maxF = 15
	}

	var camX, camY float64
	var cameraActive bool
	if doc.CameraPath != nil && doc.CameraPath.PathData != "" {
		cameraActive = true
		cStartF := doc.CameraPath.Config.StartFrame
		cEndF := doc.CameraPath.Config.EndFrame
		if doc.CameraPath.Config.IsAll {
			cStartF = 1
			cEndF = maxF
		}
		duration := cEndF - cStartF
		var progress float64
		switch {
		case frame1Idx <= cStartF:
			progress = 0.0
		case frame1Idx >= cEndF:
			progress = 1.0
		case duration > 0:
			progress = float64(frame1Idx-cStartF) / float64(duration)
		}
		t := ApplyEasing(progress, doc.CameraPath.Config.Ease)
		if doc.CameraPath.Config.Reverse {
			t = 1.0 - t
		}
		if doc.CameraPath.Config.IsPingPong {
			if t <= 0.5 {
				t *= 2.0
			} else {
				t = (1.0 - t) * 2.0
			}
		}
		if x, y, err := EvaluatePathAt(doc.CameraPath.PathData, t); err == nil {
			camX = x
			camY = y
		}
	}

	// Map GroupID to slice of MotionPaths
	motionMap := make(map[string][]MotionPath)
	for _, mp := range doc.MotionPaths {
		motionMap[mp.GroupID] = append(motionMap[mp.GroupID], mp)
	}

	// Calculate EffectiveZ for each node
	for i := range nodes {
		if nodes[i].isDefs {
			nodes[i].effectiveZ = -999999
			continue
		}
		activeDepth := 0
		if paths, ok := motionMap[nodes[i].id]; ok {
			for _, mp := range paths {
				if !mp.Config.HasDepth {
					continue
				}
				startF := mp.Config.StartFrame
				endF := mp.Config.EndFrame
				if mp.Config.IsAll {
					startF = 1
					endF = maxF
				}
				if frame1Idx >= startF && frame1Idx <= endF {
					activeDepth = mp.Config.DepthOffset
				}
			}
		}
		switch {
		case activeDepth == 0:
			nodes[i].effectiveZ = nodes[i].baseZ * 2
		case activeDepth < 0:
			nodes[i].effectiveZ = (nodes[i].baseZ + activeDepth)*2 - 1
		default:
			nodes[i].effectiveZ = (nodes[i].baseZ + activeDepth)*2 + 1
		}
	}

	// Stable sort nodes to establish Painter's rendering order
	sort.SliceStable(nodes, func(i, j int) bool {
		if nodes[i].isDefs != nodes[j].isDefs {
			return nodes[i].isDefs
		}
		if nodes[i].effectiveZ != nodes[j].effectiveZ {
			return nodes[i].effectiveZ < nodes[j].effectiveZ
		}
		return nodes[i].baseZ < nodes[j].baseZ
	})

	var buf bytes.Buffer
	encoder := xml.NewEncoder(&buf)
	dynamicGradients, groupSweepMap := generateSweepGradients(doc, frame1Idx, maxF)

	for _, tok := range preamble {
		if err := encoder.EncodeToken(tok); err != nil {
			return nil, err
		}
	}
	if err := encoder.EncodeToken(rootElem); err != nil {
		return nil, err
	}

	if len(dynamicGradients) > 0 {
		defsStart := xml.StartElement{Name: xml.Name{Local: "defs"}}
		if err := encoder.EncodeToken(defsStart); err != nil {
			return nil, err
		}
		for _, dg := range dynamicGradients {
			gradStart := xml.StartElement{
				Name: xml.Name{Local: "linearGradient"},
				Attr: []xml.Attr{
					{Name: xml.Name{Local: "id"}, Value: dg.ID},
					{Name: xml.Name{Local: "gradientUnits"}, Value: "userSpaceOnUse"},
					{Name: xml.Name{Local: "x1"}, Value: fmt.Sprintf("%.2f", dg.X1)},
					{Name: xml.Name{Local: "y1"}, Value: fmt.Sprintf("%.2f", dg.Y1)},
					{Name: xml.Name{Local: "x2"}, Value: fmt.Sprintf("%.2f", dg.X2)},
					{Name: xml.Name{Local: "y2"}, Value: fmt.Sprintf("%.2f", dg.Y2)},
				},
			}
			if err := encoder.EncodeToken(gradStart); err != nil {
				return nil, err
			}
			for _, st := range dg.Stops {
				stopElem := xml.StartElement{
					Name: xml.Name{Local: "stop"},
					Attr: []xml.Attr{
						{Name: xml.Name{Local: "offset"}, Value: fmt.Sprintf("%.2f%%", st.Offset*100.0)},
						{Name: xml.Name{Local: "stop-color"}, Value: st.Color},
						{Name: xml.Name{Local: "stop-opacity"}, Value: fmt.Sprintf("%.3f", st.Opacity)},
					},
				}
				if err := encoder.EncodeToken(stopElem); err != nil {
					return nil, err
				}
				if err := encoder.EncodeToken(stopElem.End()); err != nil {
					return nil, err
				}
			}
			if err := encoder.EncodeToken(gradStart.End()); err != nil {
				return nil, err
			}
		}
		if err := encoder.EncodeToken(defsStart.End()); err != nil {
			return nil, err
		}
	}

	for _, node := range nodes {
		if err := serializeNodeTokens(encoder, node.tokens, doc, frame1Idx, maxF, motionMap, cameraActive, camX, camY, groupSweepMap); err != nil {
			return nil, err
		}
	}

	if doc.ShowMotionLines {
		if err := serializeMotionGuides(encoder, doc); err != nil {
			return nil, err
		}
	}

	if err := encoder.EncodeToken(rootEnd); err != nil {
		return nil, err
	}
	for _, tok := range postTokens {
		if err := encoder.EncodeToken(tok); err != nil {
			return nil, err
		}
	}

	if err := encoder.Flush(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func serializeNodeTokens(encoder *xml.Encoder, tokens []xml.Token, doc *SVGDocument, frame1Idx, maxF int, motionMap map[string][]MotionPath, cameraActive bool, camX, camY float64, groupSweepMap map[string]string) error {
	var currentGDepth int
	var parallaxActiveDepth int

	type activeColorState struct {
		depth   int
		hex     string
		opacity float64
		gradURL string
		target  string
	}
	var colorStack []activeColorState

	for i := 0; i < len(tokens); i++ {
		token := tokens[i]
		switch elem := token.(type) {
		case xml.StartElement:
			elem.Attr = append([]xml.Attr(nil), elem.Attr...)
			// Hide the motion paths or markers themselves
			if elem.Name.Local == "path" || elem.Name.Local == "circle" || elem.Name.Local == "rect" || elem.Name.Local == "ellipse" || elem.Name.Local == "line" || elem.Name.Local == "polygon" || elem.Name.Local == "polyline" {
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
					depth := 1
					for i+1 < len(tokens) && depth > 0 {
						i++
						switch tokens[i].(type) {
						case xml.StartElement:
							depth++
						case xml.EndElement:
							depth--
						}
					}
					continue
				}

				// Apply active group color modifier
				if len(colorStack) > 0 {
					topColor := colorStack[len(colorStack)-1]
					applyColorOverride(&elem, topColor.hex, topColor.opacity, topColor.target, topColor.gradURL)
				}
			}

			// Apply translation, rotation, scaling, and visibility to animated groups
			if elem.Name.Local == "g" {
				currentGDepth++
				var id string
				for _, attr := range elem.Attr {
					if attr.Name.Local == "id" {
						id = attr.Value
						break
					}
				}
				paths := motionMap[id]
				hasPaths := len(paths) > 0
				applyCamera := cameraActive && parallaxActiveDepth == 0

				if hasPaths || applyCamera {
					factor := 1.0
					for _, mp := range paths {
						if mp.Config.HasParallax {
							factor = mp.Config.ParallaxFactor
							break
						}
					}

					var totalDx, totalDy float64
					if applyCamera {
						parallaxActiveDepth = currentGDepth
						totalDx = -camX * factor
						totalDy = -camY * factor
					}

					var activePaths []MotionPath
					var hasShowRules bool
					var isShown bool
					var hasHideRules bool
					var isHidden bool
					var hasSpatialOrFadeRules bool

					for _, mp := range paths {
						if mp.Config.Type == "move" || mp.Config.Type == "rot" || mp.Config.Type == "scale" || mp.Config.Type == "fade" {
							hasSpatialOrFadeRules = true
						}
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
						// Hide if outside active range of defined spatial/fade motion paths.
						// If group only has Depth/Dist rules or camera, it remains visible across all frames.
						if hasSpatialOrFadeRules && len(activePaths) == 0 {
							groupHidden = true
						}
					}

					if groupHidden {
						if parallaxActiveDepth == currentGDepth {
							parallaxActiveDepth = 0
						}
						currentGDepth--
						setElementHidden(&elem)
						if err := encoder.EncodeToken(elem); err != nil {
							return err
						}
						depth := 1
						for i+1 < len(tokens) && depth > 0 {
							i++
							switch tokens[i].(type) {
							case xml.StartElement:
								depth++
							case xml.EndElement:
								depth--
							}
						}
						if err := encoder.EncodeToken(elem.End()); err != nil {
							return err
						}
						continue
					}

					if hasShowRules || hasHideRules {
						setElementVisible(&elem)
					}

					var totalRot float64
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

						r := mp.Config.Repeat
						if r < 1 {
							r = mp.Config.ColorRepeat
						}
						if r < 1 {
							r = 1
						}

						cycleP := progress * float64(r)
						var fraction float64
						if progress >= 1.0 {
							fraction = 1.0
						} else {
							fraction = cycleP - math.Floor(cycleP)
						}

						easedProgress := ApplyEasing(fraction, mp.Config.Ease)
						t := easedProgress
						if mp.Config.Reverse {
							t = 1.0 - easedProgress
						}
						if mp.Config.IsPingPong {
							if t <= 0.5 {
								t *= 2.0
							} else {
								t = (1.0 - t) * 2.0
							}
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
						if mp.Config.HasRotationRange {
							rot := mp.Config.RotationFrom + t*(mp.Config.RotationTo-mp.Config.RotationFrom)
							if mp.Config.RotationDir == "ccw" {
								rot = -rot
							}
							totalRot += rot
						} else if mp.Config.RotationAngle != 0 {
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
						hasRotEffect := mp.Config.Type == "rot" || mp.Config.RotationAngle != 0 || mp.Config.HasRotationRange || mp.Config.OrientPath
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
					if math.Abs(totalDx) > 1e-6 || math.Abs(totalDy) > 1e-6 {
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
						for iAttr, attr := range elem.Attr {
							if attr.Name.Local == "transform" {
								elem.Attr[iAttr].Value = newTransform + " " + attr.Value
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

					// Check for Color sweep modifier attached to this group (Mode 2)
					var colorSweepPushed bool
					if sweepID, ok := groupSweepMap[id]; ok {
						for _, mp := range paths {
							if mp.Config.IsColor && mp.Config.HasColorAngle {
								colorStack = append(colorStack, activeColorState{
									depth:   currentGDepth,
									gradURL: sweepID,
									target:  mp.Config.ColorTarget,
								})
								colorSweepPushed = true
								break
							}
						}
					}

					// Check for Color solid shift modifier attached to this group (Mode 1)
					if !colorSweepPushed {
						for _, mp := range paths {
							if mp.Config.IsColor && !mp.Config.HasColorAngle {
								startF := mp.Config.StartFrame
								endF := mp.Config.EndFrame
								if mp.Config.IsAll {
									startF = 1
									endF = maxF
								}
								if frame1Idx >= startF && frame1Idx <= endF {
									duration := endF - startF
									p := 0.0
									if duration > 0 {
										p = float64(frame1Idx-startF) / float64(duration)
									}
									r := mp.Config.ColorRepeat
									if r <= 0 {
										r = 1
									}
									cycleP := p * float64(r)
									fraction := 0.0
									if p >= 1.0 {
										fraction = 1.0
									} else {
										fraction = cycleP - math.Floor(cycleP)
									}
									eased := ApplyEasing(fraction, mp.Config.Ease)
									if mp.Config.Reverse {
										eased = 1.0 - eased
									}
									var t float64
									if mp.Config.IsPingPong {
										if eased <= 0.5 {
											t = eased * 2.0
										} else {
											t = (1.0 - eased) * 2.0
										}
									} else {
										t = eased
									}
									if grad, ok := doc.Gradients[mp.FillURL]; ok && len(grad.Stops) > 0 {
										hex, op := InterpolateGradientColor(grad, t)
										colorStack = append(colorStack, activeColorState{
											depth:   currentGDepth,
											hex:     hex,
											opacity: op,
											target:  mp.Config.ColorTarget,
										})
									}
								}
								break
							}
						}
					}
				}
			}

			if err := encoder.EncodeToken(elem); err != nil {
				return err
			}

		case xml.EndElement:
			if elem.Name.Local == "g" {
				for len(colorStack) > 0 && colorStack[len(colorStack)-1].depth >= currentGDepth {
					colorStack = colorStack[:len(colorStack)-1]
				}
				if parallaxActiveDepth == currentGDepth {
					parallaxActiveDepth = 0
				}
				currentGDepth--
			}
			if err := encoder.EncodeToken(elem); err != nil {
				return err
			}

		default:
			if err := encoder.EncodeToken(token); err != nil {
				return err
			}
		}
	}
	return nil
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

func serializeMotionGuides(encoder *xml.Encoder, doc *SVGDocument) error {
	if len(doc.MotionPaths) == 0 {
		return nil
	}

	guidesGroup := xml.StartElement{
		Name: xml.Name{Local: "g"},
		Attr: []xml.Attr{
			{Name: xml.Name{Local: "id"}, Value: "inkanim_motion_guides"},
			{Name: xml.Name{Local: "style"}, Value: "pointer-events:none"},
		},
	}
	if err := encoder.EncodeToken(guidesGroup); err != nil {
		return err
	}

	seenPaths := make(map[string]bool)
	for _, mp := range doc.MotionPaths {
		if mp.PathData != "" {
			if seenPaths[mp.PathData] {
				continue
			}
			seenPaths[mp.PathData] = true

			// 1. Dark under-casing for high contrast on light backgrounds
			underCasing := xml.StartElement{
				Name: xml.Name{Local: "path"},
				Attr: []xml.Attr{
					{Name: xml.Name{Local: "d"}, Value: mp.PathData},
					{Name: xml.Name{Local: "fill"}, Value: "none"},
					{Name: xml.Name{Local: "stroke"}, Value: "#0f172a"},
					{Name: xml.Name{Local: "stroke-width"}, Value: "3.5"},
					{Name: xml.Name{Local: "stroke-linecap"}, Value: "round"},
					{Name: xml.Name{Local: "stroke-linejoin"}, Value: "round"},
					{Name: xml.Name{Local: "opacity"}, Value: "0.6"},
				},
			}
			if err := encoder.EncodeToken(underCasing); err != nil {
				return err
			}
			if err := encoder.EncodeToken(underCasing.End()); err != nil {
				return err
			}

			// 2. Vibrant Electric Sky Blue dashed guide line
			dashedGuide := xml.StartElement{
				Name: xml.Name{Local: "path"},
				Attr: []xml.Attr{
					{Name: xml.Name{Local: "d"}, Value: mp.PathData},
					{Name: xml.Name{Local: "fill"}, Value: "none"},
					{Name: xml.Name{Local: "stroke"}, Value: "#38BDF8"},
					{Name: xml.Name{Local: "stroke-width"}, Value: "2"},
					{Name: xml.Name{Local: "stroke-dasharray"}, Value: "6,4"},
					{Name: xml.Name{Local: "stroke-linecap"}, Value: "round"},
					{Name: xml.Name{Local: "stroke-linejoin"}, Value: "round"},
					{Name: xml.Name{Local: "opacity"}, Value: "0.95"},
				},
			}
			if err := encoder.EncodeToken(dashedGuide); err != nil {
				return err
			}
			if err := encoder.EncodeToken(dashedGuide.End()); err != nil {
				return err
			}

			// 3. Start point indicator dot
			if sx, sy, err := GetPathStartPoint(mp.PathData); err == nil {
				startDot := xml.StartElement{
					Name: xml.Name{Local: "circle"},
					Attr: []xml.Attr{
						{Name: xml.Name{Local: "cx"}, Value: fmt.Sprintf("%.2f", sx)},
						{Name: xml.Name{Local: "cy"}, Value: fmt.Sprintf("%.2f", sy)},
						{Name: xml.Name{Local: "r"}, Value: "3.5"},
						{Name: xml.Name{Local: "fill"}, Value: "#38BDF8"},
						{Name: xml.Name{Local: "stroke"}, Value: "#0f172a"},
						{Name: xml.Name{Local: "stroke-width"}, Value: "1.5"},
					},
				}
				if err := encoder.EncodeToken(startDot); err != nil {
					return err
				}
				if err := encoder.EncodeToken(startDot.End()); err != nil {
					return err
				}
			}
		} else if mp.Bounds.Width > 0 && mp.Bounds.Height > 0 {
			shapeKey := fmt.Sprintf("%.2f,%.2f,%.2f,%.2f", mp.Bounds.X, mp.Bounds.Y, mp.Bounds.Width, mp.Bounds.Height)
			if seenPaths[shapeKey] {
				continue
			}
			seenPaths[shapeKey] = true

			shapeGuide := xml.StartElement{
				Name: xml.Name{Local: "rect"},
				Attr: []xml.Attr{
					{Name: xml.Name{Local: "x"}, Value: fmt.Sprintf("%.2f", mp.Bounds.X)},
					{Name: xml.Name{Local: "y"}, Value: fmt.Sprintf("%.2f", mp.Bounds.Y)},
					{Name: xml.Name{Local: "width"}, Value: fmt.Sprintf("%.2f", mp.Bounds.Width)},
					{Name: xml.Name{Local: "height"}, Value: fmt.Sprintf("%.2f", mp.Bounds.Height)},
					{Name: xml.Name{Local: "fill"}, Value: "none"},
					{Name: xml.Name{Local: "stroke"}, Value: "#38BDF8"},
					{Name: xml.Name{Local: "stroke-width"}, Value: "2"},
					{Name: xml.Name{Local: "stroke-dasharray"}, Value: "6,4"},
					{Name: xml.Name{Local: "opacity"}, Value: "0.85"},
				},
			}
			if err := encoder.EncodeToken(shapeGuide); err != nil {
				return err
			}
			if err := encoder.EncodeToken(shapeGuide.End()); err != nil {
				return err
			}
		}
	}

	return encoder.EncodeToken(guidesGroup.End())
}

type dynamicSweepGradient struct {
	ID    string
	X1    float64
	Y1    float64
	X2    float64
	Y2    float64
	Stops []GradientStop
}

func generateSweepGradients(doc *SVGDocument, frame1Idx, maxF int) ([]dynamicSweepGradient, map[string]string) {
	var dynGradients []dynamicSweepGradient
	groupSweepMap := make(map[string]string)

	for _, mp := range doc.MotionPaths {
		if !mp.Config.IsColor || !mp.Config.HasColorAngle {
			continue
		}
		startF := mp.Config.StartFrame
		endF := mp.Config.EndFrame
		if mp.Config.IsAll {
			startF = 1
			endF = maxF
		}
		if frame1Idx < startF || frame1Idx > endF {
			continue
		}
		duration := endF - startF
		p := 0.0
		if duration > 0 {
			p = float64(frame1Idx-startF) / float64(duration)
		}
		r := mp.Config.ColorRepeat
		if r <= 0 {
			r = 1
		}
		cycleP := p * float64(r)
		fraction := 0.0
		if p >= 1.0 {
			fraction = 1.0
		} else {
			fraction = cycleP - math.Floor(cycleP)
		}
		eased := ApplyEasing(fraction, mp.Config.Ease)
		if mp.Config.Reverse {
			eased = 1.0 - eased
		}
		var t float64
		if mp.Config.IsPingPong {
			if eased <= 0.5 {
				t = eased * 2.0
			} else {
				t = (1.0 - eased) * 2.0
			}
		} else {
			t = eased
		}

		rect := mp.Bounds
		if rect.Width <= 0 || rect.Height <= 0 {
			if mp.ID != "" {
				rect = doc.GetElementRect(mp.ID)
			}
		}
		if rect.Width <= 0 || rect.Height <= 0 {
			if mp.GroupID != "" {
				rect = doc.GetElementRect(mp.GroupID)
			}
		}
		if rect.Width <= 0 || rect.Height <= 0 {
			rect = doc.GetDocumentRect()
		}

		rad := mp.Config.ColorAngle * math.Pi / 180.0
		ux := math.Cos(rad)
		uy := math.Sin(rad)

		cx := rect.X + rect.Width/2.0
		cy := rect.Y + rect.Height/2.0

		corners := [4][2]float64{
			{rect.X, rect.Y},
			{rect.X + rect.Width, rect.Y},
			{rect.X + rect.Width, rect.Y + rect.Height},
			{rect.X, rect.Y + rect.Height},
		}
		pMin := corners[0][0]*ux + corners[0][1]*uy
		pMax := pMin
		for i := 1; i < 4; i++ {
			proj := corners[i][0]*ux + corners[i][1]*uy
			if proj < pMin {
				pMin = proj
			}
			if proj > pMax {
				pMax = proj
			}
		}
		span := pMax - pMin
		if span < 1e-4 {
			span = 1.0
		}

		pCenter := pMin + t*span
		pStart := pCenter - span/2.0
		pEnd := pCenter + span/2.0

		cProj := cx*ux + cy*uy
		x1 := cx + (pStart-cProj)*ux
		y1 := cy + (pStart-cProj)*uy
		x2 := cx + (pEnd-cProj)*ux
		y2 := cy + (pEnd-cProj)*uy

		var stops []GradientStop
		if grad, ok := doc.Gradients[mp.FillURL]; ok && len(grad.Stops) > 0 {
			stops = grad.Stops
		} else {
			stops = []GradientStop{
				{Offset: 0.0, Color: "#ffffff", Opacity: 1.0},
				{Offset: 1.0, Color: "#000000", Opacity: 1.0},
			}
		}

		sweepID := fmt.Sprintf("inkanim_sweep_%s_%d", mp.ID, frame1Idx)
		if mp.ID == "" {
			sweepID = fmt.Sprintf("inkanim_sweep_%s_%d", mp.GroupID, frame1Idx)
		}

		if mp.GroupID != "" {
			groupSweepMap[mp.GroupID] = sweepID
		}
		dynGradients = append(dynGradients, dynamicSweepGradient{
			ID:    sweepID,
			X1:    x1,
			Y1:    y1,
			X2:    x2,
			Y2:    y2,
			Stops: stops,
		})
	}
	return dynGradients, groupSweepMap
}

// InterpolateGradientColor evaluates a gradient at progress t (0.0 to 1.0) and returns
// the interpolated color formatted as a #rrggbb hex string and opacity (0.0 to 1.0).
func InterpolateGradientColor(grad SVGGradient, t float64) (string, float64) {
	if len(grad.Stops) == 0 {
		return "#ffffff", 1.0
	}
	if t < 0.0 {
		t = 0.0
	} else if t > 1.0 {
		t = 1.0
	}

	if len(grad.Stops) == 1 || t <= grad.Stops[0].Offset {
		r, g, b := parseColorRGB(grad.Stops[0].Color)
		return fmt.Sprintf("#%02x%02x%02x", r, g, b), grad.Stops[0].Opacity
	}
	lastIdx := len(grad.Stops) - 1
	if t >= grad.Stops[lastIdx].Offset {
		r, g, b := parseColorRGB(grad.Stops[lastIdx].Color)
		return fmt.Sprintf("#%02x%02x%02x", r, g, b), grad.Stops[lastIdx].Opacity
	}

	// Find the two stops surrounding t
	var s0, s1 GradientStop
	for i := 0; i < len(grad.Stops)-1; i++ {
		if grad.Stops[i].Offset <= t && t <= grad.Stops[i+1].Offset {
			s0 = grad.Stops[i]
			s1 = grad.Stops[i+1]
			break
		}
	}

	span := s1.Offset - s0.Offset
	factor := 0.0
	if span > 1e-9 {
		factor = (t - s0.Offset) / span
	}

	r0, g0, b0 := parseColorRGB(s0.Color)
	r1, g1, b1 := parseColorRGB(s1.Color)

	r := uint8(math.Round(float64(r0) + factor*float64(int(r1)-int(r0))))
	g := uint8(math.Round(float64(g0) + factor*float64(int(g1)-int(g0))))
	b := uint8(math.Round(float64(b0) + factor*float64(int(b1)-int(b0))))

	op := s0.Opacity + factor*(s1.Opacity-s0.Opacity)
	op = math.Max(0.0, math.Min(1.0, op))

	return fmt.Sprintf("#%02x%02x%02x", r, g, b), op
}

func parseColorRGB(colorStr string) (uint8, uint8, uint8) {
	colorStr = strings.TrimSpace(colorStr)
	if r, g, b, err := oksvg.ParseSVGColorNum(colorStr); err == nil {
		return r, g, b
	}
	if c, err := oksvg.ParseSVGColor(colorStr); err == nil && c != nil {
		r, g, b, _ := c.RGBA()
		return uint8(r >> 8), uint8(g >> 8), uint8(b >> 8)
	}
	return 0, 0, 0
}

func applyColorOverride(elem *xml.StartElement, hex string, opacity float64, target string, gradURL string) {
	var styleVal string
	var styleAttrIdx = -1
	var fillAttrIdx = -1
	var strokeAttrIdx = -1
	var fillOpacityAttrIdx = -1
	var strokeOpacityAttrIdx = -1

	for i, attr := range elem.Attr {
		switch attr.Name.Local {
		case "style":
			styleVal = attr.Value
			styleAttrIdx = i
		case "fill":
			fillAttrIdx = i
		case "stroke":
			strokeAttrIdx = i
		case "fill-opacity":
			fillOpacityAttrIdx = i
		case "stroke-opacity":
			strokeOpacityAttrIdx = i
		}
	}

	styleFill := extractCSSProp(styleVal, "fill")
	styleStroke := extractCSSProp(styleVal, "stroke")

	attrFill := ""
	if fillAttrIdx >= 0 {
		attrFill = elem.Attr[fillAttrIdx].Value
	}
	attrStroke := ""
	if strokeAttrIdx >= 0 {
		attrStroke = elem.Attr[strokeAttrIdx].Value
	}

	colorVal := hex
	if gradURL != "" {
		colorVal = fmt.Sprintf("url(#%s)", gradURL)
	}

	// 1. Fill Override
	if target == "fill" || target == "all" {
		isFillNone := (styleFill == "none") || (styleFill == "" && attrFill == "none")
		if !isFillNone {
			if styleAttrIdx >= 0 && styleFill != "" {
				styleVal = setStyleProp(styleVal, "fill", colorVal)
				if gradURL == "" {
					if opacity < 1.0 {
						styleVal = setStyleProp(styleVal, "fill-opacity", fmt.Sprintf("%.3f", opacity))
					} else if extractCSSProp(styleVal, "fill-opacity") != "" {
						styleVal = setStyleProp(styleVal, "fill-opacity", "1")
					}
				}
			}
			if fillAttrIdx >= 0 {
				elem.Attr[fillAttrIdx].Value = colorVal
			} else if styleAttrIdx < 0 || styleFill == "" {
				elem.Attr = append(elem.Attr, xml.Attr{
					Name:  xml.Name{Local: "fill"},
					Value: colorVal,
				})
			}
			if gradURL == "" {
				if opacity < 1.0 {
					if fillOpacityAttrIdx >= 0 {
						elem.Attr[fillOpacityAttrIdx].Value = fmt.Sprintf("%.3f", opacity)
					} else if styleAttrIdx < 0 || styleFill == "" {
						elem.Attr = append(elem.Attr, xml.Attr{
							Name:  xml.Name{Local: "fill-opacity"},
							Value: fmt.Sprintf("%.3f", opacity),
						})
					}
				} else if fillOpacityAttrIdx >= 0 {
					elem.Attr[fillOpacityAttrIdx].Value = "1"
				}
			}
		}
	}

	// 2. Stroke Override
	if target == "stroke" || target == "all" {
		hasStroke := (styleStroke != "" && styleStroke != "none") || (styleStroke == "" && attrStroke != "" && attrStroke != "none")
		if hasStroke {
			if styleAttrIdx >= 0 && styleStroke != "" {
				styleVal = setStyleProp(styleVal, "stroke", colorVal)
				if gradURL == "" {
					if opacity < 1.0 {
						styleVal = setStyleProp(styleVal, "stroke-opacity", fmt.Sprintf("%.3f", opacity))
					} else if extractCSSProp(styleVal, "stroke-opacity") != "" {
						styleVal = setStyleProp(styleVal, "stroke-opacity", "1")
					}
				}
			}
			if strokeAttrIdx >= 0 {
				elem.Attr[strokeAttrIdx].Value = colorVal
			} else if styleAttrIdx < 0 || styleStroke == "" {
				elem.Attr = append(elem.Attr, xml.Attr{
					Name:  xml.Name{Local: "stroke"},
					Value: colorVal,
				})
			}
			if gradURL == "" {
				if opacity < 1.0 {
					if strokeOpacityAttrIdx >= 0 {
						elem.Attr[strokeOpacityAttrIdx].Value = fmt.Sprintf("%.3f", opacity)
					} else if styleAttrIdx < 0 || styleStroke == "" {
						elem.Attr = append(elem.Attr, xml.Attr{
							Name:  xml.Name{Local: "stroke-opacity"},
							Value: fmt.Sprintf("%.3f", opacity),
						})
					}
				} else if strokeOpacityAttrIdx >= 0 {
					elem.Attr[strokeOpacityAttrIdx].Value = "1"
				}
			}
		}
	}

	if styleAttrIdx >= 0 {
		elem.Attr[styleAttrIdx].Value = styleVal
	}
}


