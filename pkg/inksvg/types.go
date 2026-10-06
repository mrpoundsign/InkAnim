package inksvg

import (
	"image"
	"sync"
)

// FrameMode specifies how frames are extracted from the SVG.
type FrameMode string

const (
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


// Layer represents an animation frame in the timeline.
type Layer struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Index       int    `json:"index"`
	Visible     bool   `json:"visible"`     // visibility in SVG
	IsActive    bool   `json:"isActive"`    // included in current animation
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
	StartFrame     int
	EndFrame       int
	IsAll          bool
	Ease           string
	Type           string  // "move" or "rot"
	Reverse        bool
	RotationAngle    float64 // degrees
	RotationFrom     float64 // degrees start for range interpolation
	RotationTo       float64 // degrees end for range interpolation
	HasRotationRange bool    // true if from/to specified for Rot
	RotationDir      string  // "cw" or "ccw"
	OrientPath       bool    // true if orient: true
	PivotType      string  // "center", "edge", "node", "path-start"
	PivotEdgeAngle float64 // clock degrees (0 = top, 90 = right, 180 = bottom, 270 = left)
	PivotNodeID    string  // element ID for node pivot (without '#')
	ScaleFromX      float64 // starting horizontal scale multiplier (default 1.0)
	ScaleFromY      float64 // starting vertical scale multiplier (default 1.0)
	ScaleToX        float64 // target horizontal scale multiplier (default 1.0)
	ScaleToY        float64 // target vertical scale multiplier (default 1.0)
	OpacityFrom     float64 // starting opacity (0.0 - 1.0, default 1.0)
	OpacityTo       float64 // target opacity (0.0 - 1.0, default 0.0 for fade out)
	HasOpacity      bool    // true if Fade directive parsed
	VisibilityState string  // "show" or "hide"
	HasVisibility   bool    // true if Show or Hide directive parsed
	DepthOffset     int     // signed integer modifier applied to base Z-index (e.g. -1, +1)
	HasDepth        bool    // true if Depth directive parsed
	IsCamera        bool    // true if Camera directive parsed
	ParallaxFactor  float64 // parallax multiplier relative to camera (default 1.0, or 0.0 for fixed)
	HasParallax     bool    // true if Dist directive parsed
	IsColor         bool    // true if Color directive parsed
	IsPingPong      bool    // true if pingpong: true
	ColorRepeat     int     // repetitions across frame range (default 1)
	HasColorAngle   bool    // true if angle specified for Color
	ColorAngle      float64 // degrees for gradient sweep
	ColorTarget     string  // "fill", "stroke", or "all" (default "fill")
}

// LayerRenderOrder tracks the document and effective Z-index for frame reordering.
type LayerRenderOrder struct {
	Index      int
	ID         string
	BaseZ      int
	EffectiveZ int
}

// DefaultMotionConfig returns a MotionConfig with standard defaults.
func DefaultMotionConfig(configType string) MotionConfig {
	cfg := MotionConfig{
		Type:           configType,
		Ease:           "linear",
		RotationDir:    "cw",
		PivotType:      "center",
		ScaleFromX:     1.0,
		ScaleFromY:     1.0,
		ScaleToX:       1.0,
		ScaleToY:       1.0,
		OpacityFrom:    1.0,
		OpacityTo:      1.0,
		ParallaxFactor: 1.0,
		ColorTarget:    "fill",
		ColorRepeat:    1,
	}
	switch configType {
	case "camera":
		cfg.IsCamera = true
	case "dist", "distance":
		cfg.HasParallax = true
		cfg.ParallaxFactor = 1.0
	case "fade":
		cfg.OpacityFrom = 1.0
		cfg.OpacityTo = 0.0
		cfg.HasOpacity = true
	case "show":
		cfg.VisibilityState = "show"
		cfg.HasVisibility = true
	case "hide":
		cfg.VisibilityState = "hide"
		cfg.HasVisibility = true
	case "depth":
		cfg.HasDepth = true
	case "color":
		cfg.IsColor = true
		cfg.ColorTarget = "fill"
		cfg.ColorRepeat = 1
	}
	return cfg
}

// GradientStop represents a color stop within an SVG linear or radial gradient.
type GradientStop struct {
	Offset  float64
	Color   string // hex #rrggbb or rgb(...)
	Opacity float64
}

// SVGGradient represents an SVG gradient definition extracted from <defs>.
type SVGGradient struct {
	ID    string
	Stops []GradientStop
}

// MotionPath represents a movement spline and its config found inside a group.
type MotionPath struct {
	ID       string
	GroupID  string
	PathData string
	Config   MotionConfig
	FillURL  string // gradient ID from url(#...) on modifier object
	Bounds   Rect   // geometric bounding box of modifier object
}

// SVGDocument holds parsed SVG metadata and elements.
type SVGDocument struct {
	RawContent   []byte
	Width        float64
	Height       float64
	ViewBoxX     float64
	ViewBoxY     float64
	ViewBoxW     float64
	ViewBoxH     float64
	DrawingRect  Rect
	Layers       []Layer
	Pages        []Page
	MotionPaths  []MotionPath
	CameraPath   *MotionPath
	Gradients    map[string]SVGGradient
	DefaultMode     FrameMode
	ElementRects    map[string]Rect
	Migrations      []MigrationHit
	ShowMotionLines bool
	timelineTpl     any
	mu              sync.RWMutex
}

// GetElementRect returns the bounding rectangle of the specified element by ID,
// falling back to GetDrawingRect if the element is not found or has empty bounds.
func (d *SVGDocument) GetElementRect(id string) Rect {
	if d == nil {
		return Rect{X: 0, Y: 0, Width: 512, Height: 512}
	}
	d.mu.RLock()
	if d.ElementRects != nil {
		if r, ok := d.ElementRects[id]; ok {
			d.mu.RUnlock()
			return r
		}
	}
	d.mu.RUnlock()

	r, ok := ComputeElementRect(d.RawContent, id)
	if ok && (r.Width > 0 || r.Height > 0) {
		d.mu.Lock()
		if d.ElementRects == nil {
			d.ElementRects = make(map[string]Rect)
		}
		d.ElementRects[id] = r
		d.mu.Unlock()
		return r
	}
	return d.GetDrawingRect()
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

