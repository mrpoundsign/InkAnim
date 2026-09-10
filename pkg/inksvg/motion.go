package inksvg

import (
	"errors"
	"math"
	"strconv"
	"strings"
	"unicode"
)

type PathSegment interface {
	Length() float64
	EvaluateAt(u float64) (x, y float64)
}

type LineSegment struct {
	startX, startY float64
	endX, endY     float64
	len            float64
}

func NewLineSegment(sx, sy, ex, ey float64) *LineSegment {
	dx := ex - sx
	dy := ey - sy
	return &LineSegment{sx, sy, ex, ey, math.Sqrt(dx*dx + dy*dy)}
}
func (s *LineSegment) Length() float64 { return s.len }
func (s *LineSegment) EvaluateAt(u float64) (float64, float64) {
	return s.startX + (s.endX-s.startX)*u, s.startY + (s.endY-s.startY)*u
}

type CubicBezierSegment struct {
	sx, sy float64
	cx1, cy1 float64
	cx2, cy2 float64
	ex, ey float64
	len float64
}

func NewCubicBezierSegment(sx, sy, cx1, cy1, cx2, cy2, ex, ey float64) *CubicBezierSegment {
	s := &CubicBezierSegment{sx, sy, cx1, cy1, cx2, cy2, ex, ey, 0}
	s.len = s.approximateLength(50)
	return s
}
func (s *CubicBezierSegment) approximateLength(steps int) float64 {
	var len float64
	px, py := s.sx, s.sy
	for i := 1; i <= steps; i++ {
		u := float64(i) / float64(steps)
		nx, ny := s.EvaluateAt(u)
		dx := nx - px
		dy := ny - py
		len += math.Sqrt(dx*dx + dy*dy)
		px, py = nx, ny
	}
	return len
}
func (s *CubicBezierSegment) Length() float64 { return s.len }
func (s *CubicBezierSegment) EvaluateAt(u float64) (float64, float64) {
	inv := 1.0 - u
	inv2 := inv * inv
	inv3 := inv2 * inv
	u2 := u * u
	u3 := u2 * u

	x := inv3*s.sx + 3*inv2*u*s.cx1 + 3*inv*u2*s.cx2 + u3*s.ex
	y := inv3*s.sy + 3*inv2*u*s.cy1 + 3*inv*u2*s.cy2 + u3*s.ey
	return x, y
}

type QuadraticBezierSegment struct {
	sx, sy float64
	cx, cy float64
	ex, ey float64
	len float64
}

func NewQuadraticBezierSegment(sx, sy, cx, cy, ex, ey float64) *QuadraticBezierSegment {
	s := &QuadraticBezierSegment{sx, sy, cx, cy, ex, ey, 0}
	s.len = s.approximateLength(50)
	return s
}
func (s *QuadraticBezierSegment) approximateLength(steps int) float64 {
	var len float64
	px, py := s.sx, s.sy
	for i := 1; i <= steps; i++ {
		u := float64(i) / float64(steps)
		nx, ny := s.EvaluateAt(u)
		dx := nx - px
		dy := ny - py
		len += math.Sqrt(dx*dx + dy*dy)
		px, py = nx, ny
	}
	return len
}
func (s *QuadraticBezierSegment) Length() float64 { return s.len }
func (s *QuadraticBezierSegment) EvaluateAt(u float64) (float64, float64) {
	inv := 1.0 - u
	inv2 := inv * inv
	u2 := u * u

	x := inv2*s.sx + 2*inv*u*s.cx + u2*s.ex
	y := inv2*s.sy + 2*inv*u*s.cy + u2*s.ey
	return x, y
}

func tokenizePath(pathData string) []string {
	var tokens []string
	var currentToken strings.Builder
	for _, r := range pathData {
		switch {
		case unicode.IsSpace(r) || r == ',':
			if currentToken.Len() > 0 {
				tokens = append(tokens, currentToken.String())
				currentToken.Reset()
			}
		case unicode.IsLetter(r) && r != 'e' && r != 'E': // not exp notation
			if currentToken.Len() > 0 {
				tokens = append(tokens, currentToken.String())
				currentToken.Reset()
			}
			tokens = append(tokens, string(r))
		case r == '-':
			if currentToken.Len() > 0 && currentToken.String() != "e" && currentToken.String() != "E" {
				tokens = append(tokens, currentToken.String())
				currentToken.Reset()
			}
			currentToken.WriteRune(r)
		default:
			currentToken.WriteRune(r)
		}
	}
	if currentToken.Len() > 0 {
		tokens = append(tokens, currentToken.String())
	}
	return tokens
}

func parseFloatCmd(tokens []string, i *int) float64 {
	if *i >= len(tokens) {
		return 0
	}
	val, _ := strconv.ParseFloat(tokens[*i], 64)
	*i++
	return val
}

func parsePathSegments(pathData string) ([]PathSegment, float64, float64, error) {
	tokens := tokenizePath(pathData)
	if len(tokens) == 0 {
		return nil, 0, 0, errors.New("empty path data")
	}

	var segments []PathSegment
	var startX, startY float64
	var currX, currY float64
	var lastCtrlX, lastCtrlY float64
	var lastCmd string

	i := 0
	for i < len(tokens) {
		cmd := tokens[i]
		if unicode.IsLetter(rune(cmd[0])) {
			lastCmd = cmd
			i++
		} else {
			// Implicit command, reuse lastCmd but if lastCmd was M/m, it becomes L/l
			switch lastCmd {
			case "M":
				lastCmd = "L"
			case "m":
				lastCmd = "l"
			}
		}

		switch lastCmd {
		case "M":
			currX = parseFloatCmd(tokens, &i)
			currY = parseFloatCmd(tokens, &i)
			if len(segments) == 0 {
				startX, startY = currX, currY
			}
			lastCtrlX, lastCtrlY = currX, currY
		case "m":
			currX += parseFloatCmd(tokens, &i)
			currY += parseFloatCmd(tokens, &i)
			if len(segments) == 0 {
				startX, startY = currX, currY
			}
			lastCtrlX, lastCtrlY = currX, currY
		case "L":
			nx := parseFloatCmd(tokens, &i)
			ny := parseFloatCmd(tokens, &i)
			segments = append(segments, NewLineSegment(currX, currY, nx, ny))
			currX, currY = nx, ny
			lastCtrlX, lastCtrlY = currX, currY
		case "l":
			nx := currX + parseFloatCmd(tokens, &i)
			ny := currY + parseFloatCmd(tokens, &i)
			segments = append(segments, NewLineSegment(currX, currY, nx, ny))
			currX, currY = nx, ny
			lastCtrlX, lastCtrlY = currX, currY
		case "H":
			nx := parseFloatCmd(tokens, &i)
			segments = append(segments, NewLineSegment(currX, currY, nx, currY))
			currX = nx
			lastCtrlX, lastCtrlY = currX, currY
		case "h":
			nx := currX + parseFloatCmd(tokens, &i)
			segments = append(segments, NewLineSegment(currX, currY, nx, currY))
			currX = nx
			lastCtrlX, lastCtrlY = currX, currY
		case "V":
			ny := parseFloatCmd(tokens, &i)
			segments = append(segments, NewLineSegment(currX, currY, currX, ny))
			currY = ny
			lastCtrlX, lastCtrlY = currX, currY
		case "v":
			ny := currY + parseFloatCmd(tokens, &i)
			segments = append(segments, NewLineSegment(currX, currY, currX, ny))
			currY = ny
			lastCtrlX, lastCtrlY = currX, currY
		case "C":
			cx1 := parseFloatCmd(tokens, &i)
			cy1 := parseFloatCmd(tokens, &i)
			cx2 := parseFloatCmd(tokens, &i)
			cy2 := parseFloatCmd(tokens, &i)
			nx := parseFloatCmd(tokens, &i)
			ny := parseFloatCmd(tokens, &i)
			segments = append(segments, NewCubicBezierSegment(currX, currY, cx1, cy1, cx2, cy2, nx, ny))
			currX, currY = nx, ny
			lastCtrlX, lastCtrlY = cx2, cy2
		case "c":
			cx1 := currX + parseFloatCmd(tokens, &i)
			cy1 := currY + parseFloatCmd(tokens, &i)
			cx2 := currX + parseFloatCmd(tokens, &i)
			cy2 := currY + parseFloatCmd(tokens, &i)
			nx := currX + parseFloatCmd(tokens, &i)
			ny := currY + parseFloatCmd(tokens, &i)
			segments = append(segments, NewCubicBezierSegment(currX, currY, cx1, cy1, cx2, cy2, nx, ny))
			currX, currY = nx, ny
			lastCtrlX, lastCtrlY = cx2, cy2
		case "S":
			cx1, cy1 := currX, currY
			if strings.ToLower(lastCmd) == "c" || strings.ToLower(lastCmd) == "s" {
				cx1 = 2*currX - lastCtrlX
				cy1 = 2*currY - lastCtrlY
			}
			cx2 := parseFloatCmd(tokens, &i)
			cy2 := parseFloatCmd(tokens, &i)
			nx := parseFloatCmd(tokens, &i)
			ny := parseFloatCmd(tokens, &i)
			segments = append(segments, NewCubicBezierSegment(currX, currY, cx1, cy1, cx2, cy2, nx, ny))
			currX, currY = nx, ny
			lastCtrlX, lastCtrlY = cx2, cy2
		case "s":
			cx1, cy1 := currX, currY
			if strings.ToLower(lastCmd) == "c" || strings.ToLower(lastCmd) == "s" {
				cx1 = 2*currX - lastCtrlX
				cy1 = 2*currY - lastCtrlY
			}
			cx2 := currX + parseFloatCmd(tokens, &i)
			cy2 := currY + parseFloatCmd(tokens, &i)
			nx := currX + parseFloatCmd(tokens, &i)
			ny := currY + parseFloatCmd(tokens, &i)
			segments = append(segments, NewCubicBezierSegment(currX, currY, cx1, cy1, cx2, cy2, nx, ny))
			currX, currY = nx, ny
			lastCtrlX, lastCtrlY = cx2, cy2
		case "Q":
			cx := parseFloatCmd(tokens, &i)
			cy := parseFloatCmd(tokens, &i)
			nx := parseFloatCmd(tokens, &i)
			ny := parseFloatCmd(tokens, &i)
			segments = append(segments, NewQuadraticBezierSegment(currX, currY, cx, cy, nx, ny))
			currX, currY = nx, ny
			lastCtrlX, lastCtrlY = cx, cy
		case "q":
			cx := currX + parseFloatCmd(tokens, &i)
			cy := currY + parseFloatCmd(tokens, &i)
			nx := currX + parseFloatCmd(tokens, &i)
			ny := currY + parseFloatCmd(tokens, &i)
			segments = append(segments, NewQuadraticBezierSegment(currX, currY, cx, cy, nx, ny))
			currX, currY = nx, ny
			lastCtrlX, lastCtrlY = cx, cy
		case "Z", "z":
			if currX != startX || currY != startY {
				segments = append(segments, NewLineSegment(currX, currY, startX, startY))
			}
			currX, currY = startX, startY
			lastCtrlX, lastCtrlY = currX, currY
		default:
			// Unhandled command, skip its arguments
			return nil, 0, 0, errors.New("unsupported command: " + lastCmd)
		}
	}

	return segments, startX, startY, nil
}

func EvaluatePathAt(pathData string, t float64) (float64, float64, error) {
	if t <= 0.0 { t = 0.0 }
	if t >= 1.0 { t = 1.0 }

	segments, startX, startY, err := parsePathSegments(pathData)
	if err != nil {
		return 0, 0, err
	}
	if len(segments) == 0 {
		return 0, 0, nil
	}

	var totalLen float64
	for _, seg := range segments {
		totalLen += seg.Length()
	}

	if totalLen == 0 {
		return 0, 0, nil
	}

	targetLen := t * totalLen
	var currentLen float64

	for _, seg := range segments {
		segLen := seg.Length()
		if currentLen + segLen >= targetLen {
			u := 0.0
			if segLen > 0 {
				u = (targetLen - currentLen) / segLen
			}
			x, y := seg.EvaluateAt(u)
			return x - startX, y - startY, nil
		}
		currentLen += segLen
	}

	// Fallback to end of last segment
	x, y := segments[len(segments)-1].EvaluateAt(1.0)
	return x - startX, y - startY, nil
}

func ApplyEasing(t float64, easeType string) float64 {
	switch easeType {
	case "in":
		return t * t
	case "out":
		return t * (2 - t)
	case "in-out":
		if t < 0.5 {
			return 2 * t * t
		}
		return -1 + (4 - 2*t)*t
	default:
		return t
	}
}
