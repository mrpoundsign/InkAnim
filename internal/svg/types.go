package svg

import (
	"image"
)

// FrameMode specifies how frames are extracted from the SVG.
type FrameMode string

const (
	ModeLayers FrameMode = "layers"
	ModePages  FrameMode = "pages"
)

// Layer represents an Inkscape layer group in the SVG.
type Layer struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Index       int    `json:"index"`
	Visible     bool   `json:"visible"`     // original visibility in SVG
	IsActive    bool   `json:"isActive"`    // included in current animation
	IsPinned    bool   `json:"isPinned"`    // if true, rendered across all frames as background
	HasOverride bool   `json:"hasOverride"` // if true, uses OverrideMs instead of global duration
	OverrideMs  int    `json:"overrideMs"`  // per-frame override duration in milliseconds
	DurationMs  int    `json:"durationMs"`  // effective duration in milliseconds
}

// EffectiveDuration returns the override duration if set, otherwise the global default.
func (l Layer) EffectiveDuration(globalDefault int) int {
	if l.HasOverride && l.OverrideMs > 0 {
		return l.OverrideMs
	}
	if globalDefault > 0 {
		return globalDefault
	}
	if l.DurationMs > 0 {
		return l.DurationMs
	}
	return 100
}

// Page represents an Inkscape 1.2+ multi-page artboard.
type Page struct {
	ID          string  `json:"id"`
	Label       string  `json:"label"`
	Index       int     `json:"index"`
	X           float64 `json:"x"`
	Y           float64 `json:"y"`
	Width       float64 `json:"width"`
	Height      float64 `json:"height"`
	IsActive    bool    `json:"isActive"`
	HasOverride bool    `json:"hasOverride"`
	OverrideMs  int     `json:"overrideMs"`
	DurationMs  int     `json:"durationMs"`
}

// EffectiveDuration returns the override duration if set, otherwise the global default.
func (p Page) EffectiveDuration(globalDefault int) int {
	if p.HasOverride && p.OverrideMs > 0 {
		return p.OverrideMs
	}
	if globalDefault > 0 {
		return globalDefault
	}
	if p.DurationMs > 0 {
		return p.DurationMs
	}
	return 100
}

// SVGDocument holds parsed SVG metadata and elements.
type SVGDocument struct {
	RawContent  []byte
	Width       float64
	Height      float64
	ViewBoxX    float64
	ViewBoxY    float64
	ViewBoxW    float64
	ViewBoxH    float64
	Layers      []Layer
	Pages       []Page
	DefaultMode FrameMode
}

// RenderedFrame represents a single rasterized animation frame.
type RenderedFrame struct {
	Index      int
	Label      string
	Image      *image.RGBA
	DurationMs int
}
