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
	ID        string `json:"id"`
	Label     string `json:"label"`
	Index     int    `json:"index"`
	Visible   bool   `json:"visible"`   // original visibility in SVG
	IsActive  bool   `json:"isActive"`  // included in current animation
	IsPinned  bool   `json:"isPinned"`  // if true, rendered across all frames as background
	DurationMs int   `json:"durationMs"` // per-frame duration in milliseconds
}

// Page represents an Inkscape 1.2+ multi-page artboard.
type Page struct {
	ID         string  `json:"id"`
	Label      string  `json:"label"`
	Index      int     `json:"index"`
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	Width      float64 `json:"width"`
	Height     float64 `json:"height"`
	IsActive   bool    `json:"isActive"`
	DurationMs int     `json:"durationMs"`
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
