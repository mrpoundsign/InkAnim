package inksvg

import (
	"math"
	"strconv"
	"strings"
	"unicode"
)

// PathCommand represents a single absolute SVG path command.
type PathCommand struct {
	Type byte      // 'M', 'L', 'C', 'Q', 'A', 'Z'
	Args []float64 // Parameters associated with the command
}

// Subpath represents a contiguous contour starting with M and ending before the next M or at Z.
type Subpath struct {
	Commands []PathCommand
	Points   []Point2D
	Closed   bool
	MinX     float64
	MinY     float64
	MaxX     float64
	MaxY     float64
}

// SignedArea returns the signed polygon area of the subpath's vertices via the Shoelace formula.
func (sp *Subpath) SignedArea() float64 {
	pts := sp.Points
	n := len(pts)
	if n < 3 {
		return 0
	}
	var area float64
	for i := 0; i < n; i++ {
		j := (i + 1) % n
		area += pts[i].X * pts[j].Y
		area -= pts[j].X * pts[i].Y
	}
	return area * 0.5
}

// ContainsPoint tests if point (x, y) is inside the subpath polygon using ray casting.
func (sp *Subpath) ContainsPoint(x, y float64) bool {
	if x < sp.MinX || x > sp.MaxX || y < sp.MinY || y > sp.MaxY {
		return false
	}
	pts := sp.Points
	n := len(pts)
	if n < 3 {
		return false
	}
	inside := false
	for i, j := 0, n-1; i < n; j, i = i, i+1 {
		xi, yi := pts[i].X, pts[i].Y
		xj, yj := pts[j].X, pts[j].Y
		intersect := ((yi > y) != (yj > y)) && (x < (xj-xi)*(y-yi)/(yj-yi)+xi)
		if intersect {
			inside = !inside
		}
	}
	return inside
}

// InteriorPoint returns an approximate interior point of the subpath polygon.
func (sp *Subpath) InteriorPoint() Point2D {
	if len(sp.Points) == 0 {
		return Point2D{}
	}
	var sumX, sumY float64
	for _, p := range sp.Points {
		sumX += p.X
		sumY += p.Y
	}
	centroid := Point2D{X: sumX / float64(len(sp.Points)), Y: sumY / float64(len(sp.Points))}
	if sp.ContainsPoint(centroid.X, centroid.Y) {
		return centroid
	}
	// Fallback to average of first two points nudged slightly
	p0 := sp.Points[0]
	p1 := sp.Points[len(sp.Points)/2]
	return Point2D{X: (p0.X + p1.X) / 2.0, Y: (p0.Y + p1.Y) / 2.0}
}

// NormalizeEvenOddPath converts an SVG path d string authored under evenodd fill rules
// into equivalent NonZero winding subpaths by reversing the contour direction of internal
// cutouts (Issue #62).
func NormalizeEvenOddPath(d string) string {
	subpaths := ParseSubpaths(d)
	if len(subpaths) <= 1 {
		return d
	}

	// Calculate nesting depth and container for each subpath
	type subpathInfo struct {
		depth int
		root  int // Index of outermost root ancestor
	}
	info := make([]subpathInfo, len(subpaths))

	for i := range subpaths {
		depth := 0
		root := i
		for j := range subpaths {
			if i == j {
				continue
			}
			// Subpath j can only contain subpath i if j's bounds enclose i and j's area is larger
			if subpaths[j].MinX <= subpaths[i].MinX && subpaths[j].MaxX >= subpaths[i].MaxX &&
				subpaths[j].MinY <= subpaths[i].MinY && subpaths[j].MaxY >= subpaths[i].MaxY &&
				math.Abs(subpaths[j].SignedArea()) > math.Abs(subpaths[i].SignedArea()) {
				// Test a representative point of subpath i against subpath j
				testPt := subpaths[i].Points[0]
				if len(subpaths[i].Points) > 1 {
					testPt = Point2D{
						X: (subpaths[i].Points[0].X + subpaths[i].Points[1].X) / 2.0,
						Y: (subpaths[i].Points[0].Y + subpaths[i].Points[1].Y) / 2.0,
					}
				}
				if subpaths[j].ContainsPoint(testPt.X, testPt.Y) ||
					(subpaths[j].MinX < subpaths[i].MinX && subpaths[j].MaxX > subpaths[i].MaxX) {
					depth++
					root = j
				}
			}
		}
		info[i] = subpathInfo{depth: depth, root: root}
	}

	var anyModified bool
	for i := range subpaths {
		if info[i].depth%2 == 1 {
			// Odd nesting depth: this subpath is a cutout/hole.
			// Its winding must be opposite of the outermost root container.
			rootIdx := info[i].root
			rootArea := subpaths[rootIdx].SignedArea()
			holeArea := subpaths[i].SignedArea()

			// If winding direction matches root (both positive or both negative), reverse it!
			if (rootArea > 0 && holeArea > 0) || (rootArea < 0 && holeArea < 0) {
				subpaths[i] = ReverseSubpath(subpaths[i])
				anyModified = true
			}
		}
	}

	if !anyModified {
		return d
	}

	return SerializeSubpaths(subpaths)
}

// ParseSubpaths parses an SVG path d string into individual subpaths with absolute coordinates.
func ParseSubpaths(d string) []Subpath {
	tokens := tokenizePathD(d)
	if len(tokens) == 0 {
		return nil
	}

	var subpaths []Subpath
	var curSubpath *Subpath
	var curX, curY float64
	var startX, startY float64
	var lastCtrlX, lastCtrlY float64
	var lastCmd byte

	addPt := func(x, y float64) {
		if curSubpath == nil {
			return
		}
		curSubpath.Points = append(curSubpath.Points, Point2D{X: x, Y: y})
		if x < curSubpath.MinX {
			curSubpath.MinX = x
		}
		if x > curSubpath.MaxX {
			curSubpath.MaxX = x
		}
		if y < curSubpath.MinY {
			curSubpath.MinY = y
		}
		if y > curSubpath.MaxY {
			curSubpath.MaxY = y
		}
	}

	startNewSubpath := func(x, y float64) {
		if curSubpath != nil && len(curSubpath.Commands) > 0 {
			subpaths = append(subpaths, *curSubpath)
		}
		curSubpath = &Subpath{
			MinX: x, MaxX: x,
			MinY: y, MaxY: y,
		}
		startX, startY = x, y
		curX, curY = x, y
		addPt(x, y)
	}

	i := 0
	for i < len(tokens) {
		token := tokens[i]
		if len(token) == 1 && unicode.IsLetter(rune(token[0])) {
			cmd := token[0]
			i++

			for i < len(tokens) {
				if len(tokens[i]) == 1 && unicode.IsLetter(rune(tokens[i][0])) {
					break
				}

				switch cmd {
				case 'M', 'm':
					if i+1 >= len(tokens) {
						i = len(tokens)
						break
					}
					x := parseFloat(tokens[i])
					y := parseFloat(tokens[i+1])
					i += 2
					if cmd == 'm' {
						x += curX
						y += curY
					}
					startNewSubpath(x, y)
					curSubpath.Commands = append(curSubpath.Commands, PathCommand{Type: 'M', Args: []float64{x, y}})
					// Subsequent coordinate pairs after M/m are treated as L/l per SVG spec
					if cmd == 'm' {
						cmd = 'l'
					} else {
						cmd = 'L'
					}

				case 'L', 'l':
					if i+1 >= len(tokens) {
						i = len(tokens)
						break
					}
					x := parseFloat(tokens[i])
					y := parseFloat(tokens[i+1])
					i += 2
					if cmd == 'l' {
						x += curX
						y += curY
					}
					curX, curY = x, y
					addPt(x, y)
					curSubpath.Commands = append(curSubpath.Commands, PathCommand{Type: 'L', Args: []float64{x, y}})

				case 'H', 'h':
					x := parseFloat(tokens[i])
					i++
					if cmd == 'h' {
						x += curX
					}
					curX = x
					addPt(curX, curY)
					curSubpath.Commands = append(curSubpath.Commands, PathCommand{Type: 'L', Args: []float64{curX, curY}})

				case 'V', 'v':
					y := parseFloat(tokens[i])
					i++
					if cmd == 'v' {
						y += curY
					}
					curY = y
					addPt(curX, curY)
					curSubpath.Commands = append(curSubpath.Commands, PathCommand{Type: 'L', Args: []float64{curX, curY}})

				case 'C', 'c':
					if i+5 >= len(tokens) {
						i = len(tokens)
						break
					}
					x1 := parseFloat(tokens[i])
					y1 := parseFloat(tokens[i+1])
					x2 := parseFloat(tokens[i+2])
					y2 := parseFloat(tokens[i+3])
					x := parseFloat(tokens[i+4])
					y := parseFloat(tokens[i+5])
					i += 6
					if cmd == 'c' {
						x1 += curX
						y1 += curY
						x2 += curX
						y2 += curY
						x += curX
						y += curY
					}
					lastCtrlX, lastCtrlY = x2, y2
					curX, curY = x, y
					addPt(x1, y1)
					addPt(x2, y2)
					addPt(x, y)
					curSubpath.Commands = append(curSubpath.Commands, PathCommand{Type: 'C', Args: []float64{x1, y1, x2, y2, x, y}})

				case 'S', 's':
					if i+3 >= len(tokens) {
						i = len(tokens)
						break
					}
					var x1, y1 float64
					if lastCmd == 'C' || lastCmd == 'c' || lastCmd == 'S' || lastCmd == 's' {
						x1 = 2*curX - lastCtrlX
						y1 = 2*curY - lastCtrlY
					} else {
						x1, y1 = curX, curY
					}
					x2 := parseFloat(tokens[i])
					y2 := parseFloat(tokens[i+1])
					x := parseFloat(tokens[i+2])
					y := parseFloat(tokens[i+3])
					i += 4
					if cmd == 's' {
						x2 += curX
						y2 += curY
						x += curX
						y += curY
					}
					lastCtrlX, lastCtrlY = x2, y2
					curX, curY = x, y
					addPt(x1, y1)
					addPt(x2, y2)
					addPt(x, y)
					curSubpath.Commands = append(curSubpath.Commands, PathCommand{Type: 'C', Args: []float64{x1, y1, x2, y2, x, y}})

				case 'Q', 'q':
					if i+3 >= len(tokens) {
						i = len(tokens)
						break
					}
					x1 := parseFloat(tokens[i])
					y1 := parseFloat(tokens[i+1])
					x := parseFloat(tokens[i+2])
					y := parseFloat(tokens[i+3])
					i += 4
					if cmd == 'q' {
						x1 += curX
						y1 += curY
						x += curX
						y += curY
					}
					lastCtrlX, lastCtrlY = x1, y1
					curX, curY = x, y
					addPt(x1, y1)
					addPt(x, y)
					curSubpath.Commands = append(curSubpath.Commands, PathCommand{Type: 'Q', Args: []float64{x1, y1, x, y}})

				case 'T', 't':
					if i+1 >= len(tokens) {
						i = len(tokens)
						break
					}
					var x1, y1 float64
					if lastCmd == 'Q' || lastCmd == 'q' || lastCmd == 'T' || lastCmd == 't' {
						x1 = 2*curX - lastCtrlX
						y1 = 2*curY - lastCtrlY
					} else {
						x1, y1 = curX, curY
					}
					x := parseFloat(tokens[i])
					y := parseFloat(tokens[i+1])
					i += 2
					if cmd == 't' {
						x += curX
						y += curY
					}
					lastCtrlX, lastCtrlY = x1, y1
					curX, curY = x, y
					addPt(x1, y1)
					addPt(x, y)
					curSubpath.Commands = append(curSubpath.Commands, PathCommand{Type: 'Q', Args: []float64{x1, y1, x, y}})

				case 'A', 'a':
					if i+6 >= len(tokens) {
						i = len(tokens)
						break
					}
					rx := parseFloat(tokens[i])
					ry := parseFloat(tokens[i+1])
					rot := parseFloat(tokens[i+2])
					large := parseFloat(tokens[i+3])
					sweep := parseFloat(tokens[i+4])
					x := parseFloat(tokens[i+5])
					y := parseFloat(tokens[i+6])
					i += 7
					if cmd == 'a' {
						x += curX
						y += curY
					}
					curX, curY = x, y
					addPt(x, y)
					curSubpath.Commands = append(curSubpath.Commands, PathCommand{Type: 'A', Args: []float64{rx, ry, rot, large, sweep, x, y}})

				default:
					i++
				}
				lastCmd = cmd
			}

			if cmd == 'Z' || cmd == 'z' {
				if curSubpath != nil {
					curSubpath.Closed = true
					curSubpath.Commands = append(curSubpath.Commands, PathCommand{Type: 'Z'})
					curX, curY = startX, startY
				}
			}
		} else {
			i++
		}
	}

	if curSubpath != nil && len(curSubpath.Commands) > 0 {
		subpaths = append(subpaths, *curSubpath)
	}

	return subpaths
}

// ReverseSubpath inverts the traversal direction of a subpath.
func ReverseSubpath(sp Subpath) Subpath {
	cmds := sp.Commands
	if len(cmds) == 0 {
		return sp
	}

	// Compute start and end points of each command
	type segInfo struct {
		cmd        PathCommand
		startPt    Point2D
		endPt      Point2D
	}

	var segs []segInfo
	var curPt Point2D
	var startPt Point2D
	var hasClose bool

	for _, c := range cmds {
		switch c.Type {
		case 'M':
			curPt = Point2D{X: c.Args[0], Y: c.Args[1]}
			startPt = curPt
		case 'L':
			endPt := Point2D{X: c.Args[0], Y: c.Args[1]}
			segs = append(segs, segInfo{cmd: c, startPt: curPt, endPt: endPt})
			curPt = endPt
		case 'C':
			endPt := Point2D{X: c.Args[4], Y: c.Args[5]}
			segs = append(segs, segInfo{cmd: c, startPt: curPt, endPt: endPt})
			curPt = endPt
		case 'Q':
			endPt := Point2D{X: c.Args[2], Y: c.Args[3]}
			segs = append(segs, segInfo{cmd: c, startPt: curPt, endPt: endPt})
			curPt = endPt
		case 'A':
			endPt := Point2D{X: c.Args[5], Y: c.Args[6]}
			segs = append(segs, segInfo{cmd: c, startPt: curPt, endPt: endPt})
			curPt = endPt
		case 'Z':
			hasClose = true
			if math.Abs(curPt.X-startPt.X) > 0.001 || math.Abs(curPt.Y-startPt.Y) > 0.001 {
				segs = append(segs, segInfo{
					cmd:     PathCommand{Type: 'L', Args: []float64{startPt.X, startPt.Y}},
					startPt: curPt,
					endPt:   startPt,
				})
			}
			curPt = startPt
		}
	}

	if len(segs) == 0 {
		return sp
	}

	// The new start point is the end of the last segment
	newStart := segs[len(segs)-1].endPt
	var revCmds []PathCommand
	revCmds = append(revCmds, PathCommand{Type: 'M', Args: []float64{newStart.X, newStart.Y}})

	for k := len(segs) - 1; k >= 0; k-- {
		s := segs[k]
		switch s.cmd.Type {
		case 'L':
			revCmds = append(revCmds, PathCommand{
				Type: 'L',
				Args: []float64{s.startPt.X, s.startPt.Y},
			})
		case 'C':
			// Reverse cubic: control points swapped C2 then C1, ending at s.startPt
			revCmds = append(revCmds, PathCommand{
				Type: 'C',
				Args: []float64{s.cmd.Args[2], s.cmd.Args[3], s.cmd.Args[0], s.cmd.Args[1], s.startPt.X, s.startPt.Y},
			})
		case 'Q':
			// Reverse quad: control point remains same, ending at s.startPt
			revCmds = append(revCmds, PathCommand{
				Type: 'Q',
				Args: []float64{s.cmd.Args[0], s.cmd.Args[1], s.startPt.X, s.startPt.Y},
			})
		case 'A':
			// Reverse arc: toggle sweep flag
			sweep := s.cmd.Args[4]
			if sweep > 0.5 {
				sweep = 0
			} else {
				sweep = 1
			}
			revCmds = append(revCmds, PathCommand{
				Type: 'A',
				Args: []float64{s.cmd.Args[0], s.cmd.Args[1], s.cmd.Args[2], s.cmd.Args[3], sweep, s.startPt.X, s.startPt.Y},
			})
		}
	}

	if hasClose {
		revCmds = append(revCmds, PathCommand{Type: 'Z'})
	}

	// Reverse point vertices
	revPts := make([]Point2D, len(sp.Points))
	for idx, p := range sp.Points {
		revPts[len(sp.Points)-1-idx] = p
	}

	return Subpath{
		Commands: revCmds,
		Points:   revPts,
		Closed:   sp.Closed,
		MinX:     sp.MinX, MaxX: sp.MaxX,
		MinY:     sp.MinY, MaxY: sp.MaxY,
	}
}

// SerializeSubpaths formats a slice of Subpaths into standard SVG path d syntax.
func SerializeSubpaths(subpaths []Subpath) string {
	var b strings.Builder
	for idx, sp := range subpaths {
		if idx > 0 {
			b.WriteByte(' ')
		}
		for cIdx, cmd := range sp.Commands {
			if cIdx > 0 {
				b.WriteByte(' ')
			}
			b.WriteByte(cmd.Type)
			for _, a := range cmd.Args {
				b.WriteByte(' ')
				b.WriteString(formatCoord(a))
			}
		}
	}
	return b.String()
}

func formatCoord(v float64) string {
	if math.Abs(v-math.Round(v)) < 0.0001 {
		return strconv.Itoa(int(math.Round(v)))
	}
	return strconv.FormatFloat(v, 'f', 4, 64)
}

func parseFloat(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}
