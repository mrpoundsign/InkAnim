package params

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
)

// DirectiveModel holds structured settings for any motion directive.
type DirectiveModel struct {
	Type string // "Move", "Rot", "Scale", "Fade", "Show", "Hide", "Depth", "Dist", "Color"

	// Common Timing & Playback
	StartFrame int
	EndFrame   int
	IsAll      bool
	Ease       string // "linear", "in", "out", "in-out", "bounce", etc.
	Repeat     int    // default 1
	PingPong   bool
	Reverse    bool

	// Rot
	Angle    float64
	HasAngle bool
	RotDir   string // "cw" or "ccw"
	Pivot    string // "center", "0", "90", "180", "270", or "#nodeID"

	// Scale
	ScaleFrom float64
	ScaleTo   float64
	HasScale  bool

	// Move
	Orient bool

	// Fade
	OpacityFrom float64
	OpacityTo   float64
	HasOpacity  bool

	// Color
	ColorTarget   string // "fill", "stroke", "all"
	ColorAngle    float64
	HasColorAngle bool

	// Depth / Dist
	DepthOffset    int
	ParallaxFactor float64
	HasParallax    bool

	// Unknown tokens preserved
	Extra []string
}

// Parse extracts a structured DirectiveModel from a raw parameter string.
func Parse(dType string, paramsStr string, defaultEndFrame int) DirectiveModel {
	if defaultEndFrame <= 0 {
		defaultEndFrame = 20
	}

	m := DirectiveModel{
		Type:           dType,
		StartFrame:     1,
		EndFrame:       defaultEndFrame,
		Ease:           "linear",
		Repeat:         1,
		RotDir:         "cw",
		Pivot:          "center",
		ScaleFrom:      1.0,
		ScaleTo:        1.0,
		OpacityFrom:    1.0,
		OpacityTo:      0.0,
		ColorTarget:    "fill",
		ParallaxFactor: 1.0,
	}

	switch dType {
	case "Rot":
		m.Angle = 360
		m.HasAngle = true
	case "Scale":
		m.ScaleFrom = 1.0
		m.ScaleTo = 1.25
		m.HasScale = true
	case "Fade":
		m.OpacityFrom = 1.0
		m.OpacityTo = 0.0
		m.HasOpacity = true
	}

	// Normalize colons and split tokens
	configStr := paramsStr
	var cleaned strings.Builder
	runes := []rune(configStr)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == ':' {
			str := strings.TrimRight(cleaned.String(), " \t")
			cleaned.Reset()
			cleaned.WriteString(str)
			cleaned.WriteRune(':')
			for i+1 < len(runes) && (runes[i+1] == ' ' || runes[i+1] == '\t') {
				i++
			}
		} else {
			cleaned.WriteRune(r)
		}
	}
	configStr = cleaned.String()

	parts := strings.FieldsFunc(configStr, func(r rune) bool {
		return r == ';' || r == ',' || unicode.IsSpace(r)
	})

	var rotFrom, rotTo float64
	var hasRotFrom, hasRotTo bool

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		switch part {
		case "pingpong":
			m.PingPong = true
			continue
		case "rev", "reverse":
			m.Reverse = true
			continue
		case "orient":
			m.Orient = true
			continue
		case "fixed":
			m.ParallaxFactor = 0.0
			m.HasParallax = true
			continue
		}

		kv := strings.SplitN(part, ":", 2)
		if len(kv) != 2 {
			m.Extra = append(m.Extra, part)
			continue
		}

		k := strings.ToLower(strings.TrimSpace(kv[0]))
		v := strings.TrimSpace(kv[1])

		switch k {
		case "f", "frame", "frames":
			if strings.ToLower(v) == "all" {
				m.IsAll = true
			} else {
				sub := strings.SplitN(v, "-", 2)
				if len(sub) == 2 {
					if sf, err := strconv.Atoi(sub[0]); err == nil {
						m.StartFrame = sf
					}
					if ef, err := strconv.Atoi(sub[1]); err == nil {
						m.EndFrame = ef
					}
				} else if sf, err := strconv.Atoi(v); err == nil {
					m.StartFrame = sf
					m.EndFrame = sf
				}
			}

		case "ease":
			m.Ease = v
		case "pingpong":
			m.PingPong = (v == "true" || v == "1" || v == "yes")
		case "rev", "reverse":
			m.Reverse = (v == "true" || v == "1" || v == "yes")
		case "orient":
			m.Orient = (v == "true" || v == "1" || v == "yes")
		case "r", "repeat":
			if rInt, err := strconv.Atoi(v); err == nil && rInt > 0 {
				m.Repeat = rInt
			}

		// Rot & Color Angle
		case "deg", "angle":
			if deg, err := strconv.ParseFloat(strings.TrimSuffix(v, "deg"), 64); err == nil {
				if dType == "Color" {
					m.ColorAngle = deg
					m.HasColorAngle = true
				} else {
					m.Angle = deg
					m.HasAngle = true
				}
			}
		case "dir", "direction":
			m.RotDir = strings.ToLower(v)
		case "pivot":
			m.Pivot = v

		// Range / Scale / Opacity
		case "from":
			switch dType {
			case "Fade":
				if op, err := strconv.ParseFloat(strings.TrimSuffix(v, "%"), 64); err == nil {
					if op > 1.0 {
						op /= 100.0
					}
					m.OpacityFrom = math.Max(0.0, math.Min(1.0, op))
					m.HasOpacity = true
				}
			case "Rot":
				if deg, err := strconv.ParseFloat(strings.TrimSuffix(v, "deg"), 64); err == nil {
					rotFrom = deg
					hasRotFrom = true
				}
			default:
				if sc, err := strconv.ParseFloat(v, 64); err == nil {
					m.ScaleFrom = sc
					m.HasScale = true
				}
			}

		case "to":
			switch dType {
			case "Fade":
				if op, err := strconv.ParseFloat(strings.TrimSuffix(v, "%"), 64); err == nil {
					if op > 1.0 {
						op /= 100.0
					}
					m.OpacityTo = math.Max(0.0, math.Min(1.0, op))
					m.HasOpacity = true
				}
			case "Rot":
				if deg, err := strconv.ParseFloat(strings.TrimSuffix(v, "deg"), 64); err == nil {
					rotTo = deg
					hasRotTo = true
				}
			default:
				if sc, err := strconv.ParseFloat(v, 64); err == nil {
					m.ScaleTo = sc
					m.HasScale = true
				}
			}

		case "opacity":
			if op, err := strconv.ParseFloat(strings.TrimSuffix(v, "%"), 64); err == nil {
				if op > 1.0 {
					op /= 100.0
				}
				m.OpacityTo = math.Max(0.0, math.Min(1.0, op))
				m.OpacityFrom = m.OpacityTo
				m.HasOpacity = true
			}

		// Color
		case "target":
			m.ColorTarget = strings.ToLower(v)

		// Dist / Depth
		case "factor":
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				m.ParallaxFactor = f
				m.HasParallax = true
			}
		case "order", "z", "depth":
			if z, err := strconv.Atoi(strings.TrimPrefix(v, "+")); err == nil {
				m.DepthOffset = z
			}

		default:
			m.Extra = append(m.Extra, part)
		}
	}

	if dType == "Rot" && (hasRotFrom || hasRotTo) {
		m.Angle = rotTo - rotFrom
		m.HasAngle = true
	}

	return m
}

// Format converts a DirectiveModel back into canonical IAMS parameter syntax.
func (m DirectiveModel) Format() string {
	var parts []string

	// 1. Frame Range
	if m.IsAll {
		parts = append(parts, "f: all")
	} else if m.StartFrame > 0 && m.EndFrame > 0 {
		parts = append(parts, fmt.Sprintf("f: %d-%d", m.StartFrame, m.EndFrame))
	}

	// 2. Type-specific properties
	switch m.Type {
	case "Rot":
		parts = append(parts, fmt.Sprintf("deg: %g", m.Angle))
		if m.RotDir == "ccw" {
			parts = append(parts, "dir: ccw")
		}
		if m.Pivot != "" && m.Pivot != "center" {
			parts = append(parts, "pivot: "+m.Pivot)
		}

	case "Scale":
		parts = append(parts, fmt.Sprintf("from: %g", m.ScaleFrom))
		parts = append(parts, fmt.Sprintf("to: %g", m.ScaleTo))

	case "Move":
		if m.Orient {
			parts = append(parts, "orient")
		}
		if m.Pivot != "" && m.Pivot != "center" {
			parts = append(parts, "pivot: "+m.Pivot)
		}

	case "Fade":
		parts = append(parts, fmt.Sprintf("from: %g", m.OpacityFrom))
		parts = append(parts, fmt.Sprintf("to: %g", m.OpacityTo))

	case "Color":
		if m.ColorTarget != "" && m.ColorTarget != "fill" {
			parts = append(parts, "target: "+m.ColorTarget)
		}
		if m.HasColorAngle {
			parts = append(parts, fmt.Sprintf("deg: %g", m.ColorAngle))
		}

	case "Dist":
		if m.ParallaxFactor == 0.0 {
			parts = append(parts, "fixed")
		} else {
			parts = append(parts, fmt.Sprintf("factor: %g", m.ParallaxFactor))
		}

	case "Depth":
		parts = append(parts, fmt.Sprintf("order: %d", m.DepthOffset))
	}

	// 3. Easing & Modifiers (only for continuous motion types)
	isContinuous := m.Type == "Move" || m.Type == "Rot" || m.Type == "Scale" || m.Type == "Fade" || m.Type == "Color"
	if isContinuous {
		if m.Ease != "" && m.Ease != "linear" {
			parts = append(parts, "ease: "+m.Ease)
		}
		if m.PingPong {
			parts = append(parts, "pingpong")
		}
		if m.Reverse {
			parts = append(parts, "rev")
		}
		if m.Repeat > 1 {
			parts = append(parts, fmt.Sprintf("r: %d", m.Repeat))
		}
	}

	// 4. Preserved extra tokens
	parts = append(parts, m.Extra...)

	return strings.Join(parts, "; ")
}
