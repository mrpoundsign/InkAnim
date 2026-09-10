package inksvg

import (
	"fmt"
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

// extractPathEffects scans XML for <inkscape:path-effect> definitions.
func extractPathEffects(data []byte) map[string]PathEffect {
	return extractDocumentMetadata(data).Effects
}

func isSVGCommand(b byte) bool {
	switch b {
	case 'M', 'm', 'L', 'l', 'H', 'h', 'V', 'v', 'C', 'c', 'S', 's', 'Q', 'q', 'T', 't', 'A', 'a', 'Z', 'z':
		return true
	}
	return false
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
