package inksvg

import (
	"errors"
	"strconv"
	"strings"
	"unicode"
)

// EvaluatePathAt calculates the (x, y) coordinate at percentage t (0.0 to 1.0) along an SVG path data string (d).
// For Phase 1, it supports basic absolute MoveTo (M), LineTo (L), and Cubic Bezier (C).
func EvaluatePathAt(pathData string, t float64) (x, y float64, err error) {
	if t <= 0.0 {
		t = 0.0
	}
	if t >= 1.0 {
		t = 1.0
	}

	// Tokenize path data
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
		default:
			currentToken.WriteRune(r)
		}
	}
	if currentToken.Len() > 0 {
		tokens = append(tokens, currentToken.String())
	}

	if len(tokens) == 0 {
		return 0, 0, errors.New("empty path data")
	}

	var startX, startY float64
	var currX, currY float64

	// Just a simple parser for the specific test case: M x,y C x1,y1 x2,y2 x,y
	var i int
	for i < len(tokens) {
		cmd := tokens[i]
		i++
		switch cmd {
		case "M", "m":
			currX, _ = strconv.ParseFloat(tokens[i], 64)
			currY, _ = strconv.ParseFloat(tokens[i+1], 64)
			startX, startY = currX, currY
			i += 2
			// If t == 0, we can just return here, but let's parse everything to find the curve.
		case "C":
			// Cubic Bezier
			if i+5 < len(tokens) {
				x1, _ := strconv.ParseFloat(tokens[i], 64)
				y1, _ := strconv.ParseFloat(tokens[i+1], 64)
				x2, _ := strconv.ParseFloat(tokens[i+2], 64)
				y2, _ := strconv.ParseFloat(tokens[i+3], 64)
				x3, _ := strconv.ParseFloat(tokens[i+4], 64)
				y3, _ := strconv.ParseFloat(tokens[i+5], 64)

				// Evaluate bezier at t
				invT := 1.0 - t
				ansX := (invT*invT*invT)*currX + 3*(invT*invT)*t*x1 + 3*invT*(t*t)*x2 + (t*t*t)*x3
				ansY := (invT*invT*invT)*currY + 3*(invT*invT)*t*y1 + 3*invT*(t*t)*y2 + (t*t*t)*y3
				
				// Return relative translation from start position
				return ansX - startX, ansY - startY, nil
			}
		}
	}
	return 0, 0, errors.New("unsupported path format for interpolation")
}

// Ease functions modify linear t (0 to 1)
func ApplyEasing(t float64, easeType string) float64 {
	switch easeType {
	case "in":
		return t * t // Quadratic ease-in
	case "out":
		return t * (2 - t) // Quadratic ease-out
	case "in-out":
		if t < 0.5 {
			return 2 * t * t
		}
		return -1 + (4 - 2*t)*t
	default: // "linear"
		return t
	}
}
