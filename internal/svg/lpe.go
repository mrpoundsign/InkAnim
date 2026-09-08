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

// PathEffect represents an Inkscape Live Path Effect definition from <defs>.
type PathEffect struct {
	ID        string
	Effect    string
	Radius    float64
	Unit      string
	Mode      string // "F" for fillet, "C" for chamfer
	Steps     int
	UseKnot   bool
	RawParams string
}

// Point2D represents a 2D coordinate in user space.
type Point2D struct {
	X, Y float64
}

// SubPathSegment represents a discrete segment in an SVG path.
type SubPathSegment struct {
	Type     byte // 'M', 'L', 'A', 'C', 'Z'
	Start    Point2D
	End      Point2D
	P1       Point2D // control point 1 for cubic
	P2       Point2D // control point 2 for cubic
	Rx, Ry   float64 // radii for arc
	XRot     float64 // x-axis rotation for arc
	LargeArc bool
	Sweep    bool
}

// PreprocessSVG applies in-memory transformations to raw SVG data:
//  1. Converts SVG <text> and <tspan> elements into standard <path> vector glyph contours (Issue #21).
//  2. Evaluates Inkscape fillet_chamfer Live Path Effects on paths referencing them (when unbaked).
//  3. Normalizes rect rx/ry arcs.
//  4. Scales shape stroke-width by cumulative group transform scaling.
//  5. Desugars paint-order: stroke fill (and stroke fill markers) into consecutive stroke-then-fill elements
//     so renderers like oksvg (which lack native paint-order support) render strokes under fills correctly.
func PreprocessSVG(data []byte) ([]byte, error) {
	// First pass: extract LPE definitions from <defs>
	effects := extractPathEffects(data)

	// Second pass: stream transform XML tokens
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var buf bytes.Buffer
	encoder := xml.NewEncoder(&buf)

	transformStack := []Matrix2D{IdentityMatrix()}

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

			// Stage 1: Convert SVG <text> elements into <path> vectors (Issue #21)
			if name == "text" {
				if err := ProcessTextElementToPaths(elem, decoder, encoder, &transformStack); err != nil {
					return nil, err
				}
				continue
			}

			// Track group transforms
			if name == "g" {
				curM := IdentityMatrix()
				for _, attr := range elem.Attr {
					if attr.Name.Local == "transform" {
						curM = parseTransform(attr.Value)
						break
					}
				}
				top := transformStack[len(transformStack)-1]
				transformStack = append(transformStack, top.Multiply(curM))
			}

			// Check for LPE application on path elements
			if name == "path" {
				var effectRef string
				var dAttrIdx = -1

				for i, attr := range elem.Attr {
					if attr.Name.Local == "path-effect" {
						effectRef = strings.TrimPrefix(attr.Value, "#")
					}
					if attr.Name.Local == "d" {
						dAttrIdx = i
					}
				}

				// If path references a fillet_chamfer effect and has a d attribute, evaluate LPE if unbaked
				if effectRef != "" && dAttrIdx >= 0 {
					if eff, ok := effects[effectRef]; ok && eff.Effect == "fillet_chamfer" && eff.Radius > 0 {
						newD := ApplyFilletChamferToPath(elem.Attr[dAttrIdx].Value, eff)
						if newD != "" {
							elem.Attr[dAttrIdx].Value = newD
						}
					}
				}
			}

			// Normalize rx/ry for rect elements: if only one is specified, SVG spec requires copying to the other.
			// oksvg only inspects rx; if rx is omitted, oksvg incorrectly renders square corners.
			if name == "rect" {
				var rxVal, ryVal string
				for _, attr := range elem.Attr {
					if attr.Name.Local == "rx" {
						rxVal = attr.Value
					}
					if attr.Name.Local == "ry" {
						ryVal = attr.Value
					}
				}
				if rxVal == "" && ryVal != "" {
					elem.Attr = append(elem.Attr, xml.Attr{Name: xml.Name{Local: "rx"}, Value: ryVal})
				} else if ryVal == "" && rxVal != "" {
					elem.Attr = append(elem.Attr, xml.Attr{Name: xml.Name{Local: "ry"}, Value: rxVal})
				}
			}

			// For shapes inside transformed groups, oksvg applies the CTM to path coordinates
			// but ignores the transform when rasterizing stroke-width. Scale stroke-width by the
			// ancestor group scale factor so strokes render at the true visual thickness.
			if isShapeElement(name) {
				ancestorScale := transformStack[len(transformStack)-1].ScaleFactor()
				if ancestorScale > 0 && math.Abs(ancestorScale-1.0) > 0.001 {
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
								newSW := fmt.Sprintf("%.4f%s", swNum*ancestorScale, unit)
								elem.Attr[styleAttrIdx].Value = setStyleProp(elem.Attr[styleAttrIdx].Value, "stroke-width", newSW)
							}
						}
					}
					for i := range elem.Attr {
						if elem.Attr[i].Name.Local == "stroke-width" {
							if swNum, unit := parseStrokeWidth(elem.Attr[i].Value); swNum > 0 {
								elem.Attr[i].Value = fmt.Sprintf("%.4f%s", swNum*ancestorScale, unit)
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

// extractPathEffects scans XML for <inkscape:path-effect> definitions.
func extractPathEffects(data []byte) map[string]PathEffect {
	effects := make(map[string]PathEffect)
	decoder := xml.NewDecoder(bytes.NewReader(data))

	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}

		if elem, ok := token.(xml.StartElement); ok {
			if elem.Name.Local == "path-effect" {
				var eff PathEffect
				for _, attr := range elem.Attr {
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
					effects[eff.ID] = eff
				}
			}
		}
	}

	return effects
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

func isSVGCommand(b byte) bool {
	switch b {
	case 'M', 'm', 'L', 'l', 'H', 'h', 'V', 'v', 'C', 'c', 'S', 's', 'Q', 'q', 'T', 't', 'A', 'a', 'Z', 'z':
		return true
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

// ApplyFilletChamferToPath evaluates fillet_chamfer Live Path Effect on an SVG path string d.
// If the path contains sharp corners that have not yet been filleted, it fillets them to the target radius.
func ApplyFilletChamferToPath(d string, eff PathEffect) string {
	segments := parseSVGPathSegments(d)
	if len(segments) == 0 {
		return d
	}

	// If path already contains arcs, Inkscape has already baked the fillet into d
	for _, s := range segments {
		if s.Type == 'A' {
			return d
		}
	}

	radius := eff.Radius
	if radius <= 0 {
		return d
	}

	filleted := filletSegments(segments, radius)
	if len(filleted) == 0 {
		return d
	}

	return serializeSegmentsToD(filleted)
}

// parseSVGPathSegments converts an SVG path d string into discrete linear/curved segments with absolute coordinates.
func parseSVGPathSegments(d string) []SubPathSegment {
	tokens := tokenizePathD(d)
	if len(tokens) == 0 {
		return nil
	}

	var segments []SubPathSegment
	var current, subPathStart Point2D
	var lastCmd byte

	i := 0
	for i < len(tokens) {
		tok := tokens[i]
		cmd := tok[0]
		isCmd := len(tok) == 1 && isSVGCommand(cmd)

		if isCmd {
			lastCmd = cmd
			i++
		} else {
			switch lastCmd {
			case 'M':
				cmd = 'L'
			case 'm':
				cmd = 'l'
			default:
				cmd = lastCmd
			}
		}

		switch cmd {
		case 'M', 'm':
			if i+1 >= len(tokens) {
				break
			}
			x, _ := strconv.ParseFloat(tokens[i], 64)
			y, _ := strconv.ParseFloat(tokens[i+1], 64)
			i += 2
			if cmd == 'm' {
				current.X += x
				current.Y += y
			} else {
				current.X = x
				current.Y = y
			}
			subPathStart = current
			segments = append(segments, SubPathSegment{
				Type:  'M',
				Start: current,
				End:   current,
			})

		case 'L', 'l':
			if i+1 >= len(tokens) {
				break
			}
			x, _ := strconv.ParseFloat(tokens[i], 64)
			y, _ := strconv.ParseFloat(tokens[i+1], 64)
			i += 2
			target := Point2D{X: x, Y: y}
			if cmd == 'l' {
				target.X += current.X
				target.Y += current.Y
			}
			segments = append(segments, SubPathSegment{
				Type:  'L',
				Start: current,
				End:   target,
			})
			current = target

		case 'H', 'h':
			if i >= len(tokens) {
				break
			}
			x, _ := strconv.ParseFloat(tokens[i], 64)
			i++
			target := Point2D{X: x, Y: current.Y}
			if cmd == 'h' {
				target.X += current.X
			}
			segments = append(segments, SubPathSegment{
				Type:  'L',
				Start: current,
				End:   target,
			})
			current = target

		case 'V', 'v':
			if i >= len(tokens) {
				break
			}
			y, _ := strconv.ParseFloat(tokens[i], 64)
			i++
			target := Point2D{X: current.X, Y: y}
			if cmd == 'v' {
				target.Y += current.Y
			}
			segments = append(segments, SubPathSegment{
				Type:  'L',
				Start: current,
				End:   target,
			})
			current = target

		case 'A', 'a':
			if i+6 >= len(tokens) {
				break
			}
			rx, _ := strconv.ParseFloat(tokens[i], 64)
			ry, _ := strconv.ParseFloat(tokens[i+1], 64)
			xRot, _ := strconv.ParseFloat(tokens[i+2], 64)
			largeArc := tokens[i+3] == "1"
			sweep := tokens[i+4] == "1"
			x, _ := strconv.ParseFloat(tokens[i+5], 64)
			y, _ := strconv.ParseFloat(tokens[i+6], 64)
			i += 7

			target := Point2D{X: x, Y: y}
			if cmd == 'a' {
				target.X += current.X
				target.Y += current.Y
			}
			segments = append(segments, SubPathSegment{
				Type:     'A',
				Start:    current,
				End:      target,
				Rx:       rx,
				Ry:       ry,
				XRot:     xRot,
				LargeArc: largeArc,
				Sweep:    sweep,
			})
			current = target

		case 'C', 'c':
			if i+5 >= len(tokens) {
				break
			}
			x1, _ := strconv.ParseFloat(tokens[i], 64)
			y1, _ := strconv.ParseFloat(tokens[i+1], 64)
			x2, _ := strconv.ParseFloat(tokens[i+2], 64)
			y2, _ := strconv.ParseFloat(tokens[i+3], 64)
			x, _ := strconv.ParseFloat(tokens[i+4], 64)
			y, _ := strconv.ParseFloat(tokens[i+5], 64)
			i += 6

			p1 := Point2D{X: x1, Y: y1}
			p2 := Point2D{X: x2, Y: y2}
			target := Point2D{X: x, Y: y}
			if cmd == 'c' {
				p1.X += current.X
				p1.Y += current.Y
				p2.X += current.X
				p2.Y += current.Y
				target.X += current.X
				target.Y += current.Y
			}
			segments = append(segments, SubPathSegment{
				Type:  'C',
				Start: current,
				End:   target,
				P1:    p1,
				P2:    p2,
			})
			current = target

		case 'Z', 'z':
			segments = append(segments, SubPathSegment{
				Type:  'Z',
				Start: current,
				End:   subPathStart,
			})
			current = subPathStart

		default:
			i++
		}
	}

	return segments
}

// tokenizePathD splits an SVG path string into numbers and valid SVG command characters.
func tokenizePathD(d string) []string {
	var tokens []string
	var cur strings.Builder

	flush := func() {
		if cur.Len() > 0 {
			tokens = append(tokens, cur.String())
			cur.Reset()
		}
	}

	for i := 0; i < len(d); i++ {
		ch := d[i]
		if isSVGCommand(ch) {
			flush()
			tokens = append(tokens, string(ch))
			continue
		}
		if unicode.IsSpace(rune(ch)) || ch == ',' {
			flush()
			continue
		}
		if ch == '-' || ch == '+' {
			// Check if part of scientific notation like 1e-7 or 2E+3
			if cur.Len() > 0 {
				lastChar := cur.String()[cur.Len()-1]
				if lastChar != 'e' && lastChar != 'E' {
					flush()
				}
			}
			cur.WriteByte(ch)
			continue
		}
		cur.WriteByte(ch)
	}
	flush()
	return tokens
}

// filletSegments applies the fillet radius to corners formed by adjacent line segments.
func filletSegments(segs []SubPathSegment, radius float64) []SubPathSegment {
	if len(segs) < 3 || radius <= 0 {
		return segs
	}

	// Group segments into discrete subpaths beginning with 'M'
	var subpaths [][]SubPathSegment
	var curSub []SubPathSegment
	for _, s := range segs {
		if s.Type == 'M' && len(curSub) > 0 {
			subpaths = append(subpaths, curSub)
			curSub = nil
		}
		curSub = append(curSub, s)
	}
	if len(curSub) > 0 {
		subpaths = append(subpaths, curSub)
	}

	var result []SubPathSegment
	for _, sp := range subpaths {
		result = append(result, filletSingleSubPath(sp, radius)...)
	}
	return result
}

func filletSingleSubPath(sp []SubPathSegment, radius float64) []SubPathSegment {
	if len(sp) < 3 || sp[0].Type != 'M' {
		return sp
	}

	// Check if subpath consists exclusively of linear segments
	for _, s := range sp {
		if s.Type != 'M' && s.Type != 'L' && s.Type != 'Z' {
			return sp
		}
	}

	// Extract vertices
	var vertices []Point2D
	vertices = append(vertices, sp[0].End)
	isClosed := false

	for _, s := range sp[1:] {
		switch s.Type {
		case 'Z':
			isClosed = true
		case 'L':
			vertices = append(vertices, s.End)
		}
	}

	// If closed and last vertex is duplicate of first vertex, drop duplicate
	if isClosed && len(vertices) > 1 {
		last := vertices[len(vertices)-1]
		first := vertices[0]
		if math.Hypot(last.X-first.X, last.Y-first.Y) < 1e-4 {
			vertices = vertices[:len(vertices)-1]
		}
	}

	m := len(vertices)
	if m < 3 {
		return sp
	}

	type cornerInfo struct {
		hasArc    bool
		t1, t2    Point2D
		effRadius float64
		sweep     bool
	}

	corners := make([]cornerInfo, m)

	for i := range m {
		if !isClosed && (i == 0 || i == m-1) {
			continue
		}

		prevIdx := (i - 1 + m) % m
		nextIdx := (i + 1) % m

		vCurr := vertices[i]
		vPrev := vertices[prevIdx]
		vNext := vertices[nextIdx]

		vin := Point2D{X: vCurr.X - vPrev.X, Y: vCurr.Y - vPrev.Y}
		vout := Point2D{X: vNext.X - vCurr.X, Y: vNext.Y - vCurr.Y}

		u1 := normalizeVec(vin)
		u2 := normalizeVec(vout)

		dot := u1.X*u2.X + u1.Y*u2.Y
		if dot < -1.0 {
			dot = -1.0
		}
		if dot > 1.0 {
			dot = 1.0
		}

		theta := math.Acos(dot)
		if theta < 0.01 || theta > math.Pi-0.01 {
			continue
		}

		d := radius * math.Tan(theta/2.0)
		len1 := math.Hypot(vin.X, vin.Y)
		len2 := math.Hypot(vout.X, vout.Y)
		maxD := math.Min(len1, len2) * 0.49
		effRadius := radius
		if d > maxD {
			d = maxD
			effRadius = d / math.Tan(theta/2.0)
		}

		t1 := Point2D{X: vCurr.X - d*u1.X, Y: vCurr.Y - d*u1.Y}
		t2 := Point2D{X: vCurr.X + d*u2.X, Y: vCurr.Y + d*u2.Y}

		cross := u1.X*u2.Y - u1.Y*u2.X
		sweep := cross > 0

		corners[i] = cornerInfo{
			hasArc:    true,
			t1:        t1,
			t2:        t2,
			effRadius: effRadius,
			sweep:     sweep,
		}
	}

	var res []SubPathSegment
	if isClosed {
		// Closed polygon: start at t2 of vertex 0
		startPt := vertices[0]
		if corners[0].hasArc {
			startPt = corners[0].t2
		}
		res = append(res, SubPathSegment{
			Type:  'M',
			Start: startPt,
			End:   startPt,
		})

		for i := 1; i < m; i++ {
			c := corners[i]
			if c.hasArc {
				res = append(res, SubPathSegment{
					Type:  'L',
					Start: res[len(res)-1].End,
					End:   c.t1,
				})
				res = append(res, SubPathSegment{
					Type:  'A',
					Start: c.t1,
					End:   c.t2,
					Rx:    c.effRadius,
					Ry:    c.effRadius,
					Sweep: c.sweep,
				})
			} else {
				res = append(res, SubPathSegment{
					Type:  'L',
					Start: res[len(res)-1].End,
					End:   vertices[i],
				})
			}
		}

		// Connect to vertex 0
		c0 := corners[0]
		if c0.hasArc {
			res = append(res, SubPathSegment{
				Type:  'L',
				Start: res[len(res)-1].End,
				End:   c0.t1,
			})
			res = append(res, SubPathSegment{
				Type:  'A',
				Start: c0.t1,
				End:   c0.t2,
				Rx:    c0.effRadius,
				Ry:    c0.effRadius,
				Sweep: c0.sweep,
			})
		} else {
			res = append(res, SubPathSegment{
				Type:  'L',
				Start: res[len(res)-1].End,
				End:   vertices[0],
			})
		}
		res = append(res, SubPathSegment{
			Type:  'Z',
			Start: res[len(res)-1].End,
			End:   startPt,
		})
	} else {
		// Open polyline: endpoints stay sharp
		res = append(res, SubPathSegment{
			Type:  'M',
			Start: vertices[0],
			End:   vertices[0],
		})
		for i := 1; i < m-1; i++ {
			c := corners[i]
			if c.hasArc {
				res = append(res, SubPathSegment{
					Type:  'L',
					Start: res[len(res)-1].End,
					End:   c.t1,
				})
				res = append(res, SubPathSegment{
					Type:  'A',
					Start: c.t1,
					End:   c.t2,
					Rx:    c.effRadius,
					Ry:    c.effRadius,
					Sweep: c.sweep,
				})
			} else {
				res = append(res, SubPathSegment{
					Type:  'L',
					Start: res[len(res)-1].End,
					End:   vertices[i],
				})
			}
		}
		res = append(res, SubPathSegment{
			Type:  'L',
			Start: res[len(res)-1].End,
			End:   vertices[m-1],
		})
	}

	return res
}

func normalizeVec(p Point2D) Point2D {
	l := math.Hypot(p.X, p.Y)
	if l == 0 {
		return Point2D{}
	}
	return Point2D{X: p.X / l, Y: p.Y / l}
}

// serializeSegmentsToD formats segments back into SVG path d syntax.
func serializeSegmentsToD(segs []SubPathSegment) string {
	var b strings.Builder
	for i, s := range segs {
		if i > 0 {
			b.WriteByte(' ')
		}
		switch s.Type {
		case 'M':
			fmt.Fprintf(&b, "M %f %f", s.End.X, s.End.Y)
		case 'L':
			fmt.Fprintf(&b, "L %f %f", s.End.X, s.End.Y)
		case 'A':
			sweep := 0
			if s.Sweep {
				sweep = 1
			}
			large := 0
			if s.LargeArc {
				large = 1
			}
			fmt.Fprintf(&b, "A %f %f %f %d %d %f %f", s.Rx, s.Ry, s.XRot, large, sweep, s.End.X, s.End.Y)
		case 'C':
			fmt.Fprintf(&b, "C %f %f, %f %f, %f %f", s.P1.X, s.P1.Y, s.P2.X, s.P2.Y, s.End.X, s.End.Y)
		case 'Z':
			b.WriteString("Z")
		}
	}
	return b.String()
}
