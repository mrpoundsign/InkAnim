package inksvg

import (
	"image"
)

// FrameMode specifies how frames are extracted from the SVG.
type FrameMode string

const (
	ModeLayers   FrameMode = "layers"
	ModePages    FrameMode = "pages"
	ModeTimeline FrameMode = "timeline"
)

// BoundaryMode specifies the crop boundary used for framing animation.
type BoundaryMode string

const (
	BoundaryDrawing  BoundaryMode = "drawing"
	BoundaryDocument BoundaryMode = "document"
	BoundaryPage     BoundaryMode = "page"
)

// Rect represents a 2D bounding rectangle in SVG user space.
type Rect struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}


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

// MotionConfig holds the parsed animation parameters from IAMS syntax.
type MotionConfig struct {
	StartFrame int
	EndFrame   int
	IsAll      bool
	Ease       string
	Type       string
}

// MotionPath represents a movement spline and its config found inside a group.
type MotionPath struct {
	ID       string
	GroupID  string
	PathData string
	Config   MotionConfig
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
	DrawingRect Rect
	Layers      []Layer
	Pages       []Page
	MotionPaths []MotionPath
	DefaultMode FrameMode
}

// GetDrawingRect returns the bounding rectangle of all rendered paths/elements in the SVG,
// falling back to GetDocumentRect if DrawingRect has non-positive dimensions.
func (d *SVGDocument) GetDrawingRect() Rect {
	if d == nil {
		return Rect{X: 0, Y: 0, Width: 512, Height: 512}
	}
	if d.DrawingRect.Width > 0 && d.DrawingRect.Height > 0 {
		return d.DrawingRect
	}
	return d.GetDocumentRect()
}

// GetDocumentRect returns the document's native viewBox or dimensions as a Rect.
func (d *SVGDocument) GetDocumentRect() Rect {
	if d == nil {
		return Rect{X: 0, Y: 0, Width: 512, Height: 512}
	}
	if d.ViewBoxW > 0 && d.ViewBoxH > 0 {
		return Rect{
			X:      d.ViewBoxX,
			Y:      d.ViewBoxY,
			Width:  d.ViewBoxW,
			Height: d.ViewBoxH,
		}
	}
	w := d.Width
	h := d.Height
	if w <= 0 {
		w = 512
	}
	if h <= 0 {
		h = 512
	}
	return Rect{X: 0, Y: 0, Width: w, Height: h}
}

// GetPageRect returns the bounding rectangle of the page at the given index.
func (d *SVGDocument) GetPageRect(index int) (Rect, bool) {
	if d == nil || index < 0 || index >= len(d.Pages) {
		return Rect{}, false
	}
	p := d.Pages[index]
	w := p.Width
	h := p.Height
	if w <= 0 {
		w = 512
	}
	if h <= 0 {
		h = 512
	}
	return Rect{
		X:      p.X,
		Y:      p.Y,
		Width:  w,
		Height: h,
	}, true
}


// RenderedFrame represents a single rasterized animation frame.
type RenderedFrame struct {
	Index      int
	Label      string
	Image      *image.RGBA
	DurationMs int
}

// EmbeddedImage represents an embedded raster graphic (<image> tag) decoded from a data URI.
type EmbeddedImage struct {
	Data      image.Image
	X, Y      float64
	Width     float64
	Height    float64
	Transform Matrix2D
	Opacity   float64
	PathIndex int
}

