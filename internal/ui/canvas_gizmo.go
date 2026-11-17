package ui

import (
	"fmt"
	"image"
	"image/color"
	"maps"
	"math"
	"slices"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"inkanim/internal/app"
	"inkanim/pkg/inksvg"
)

// GizmoSegment represents one segment of an editable motion path in SVG coordinates.
type GizmoSegment struct {
	StartX, StartY float64
	Ctrl1X, Ctrl1Y float64
	Ctrl2X, Ctrl2Y float64
	EndX, EndY     float64
	IsCurve        bool
}

// EvaluateAt returns the (x, y) point along the segment at parameter u in [0, 1].
func (s GizmoSegment) EvaluateAt(u float64) (float64, float64) {
	if !s.IsCurve {
		return s.StartX + (s.EndX-s.StartX)*u, s.StartY + (s.EndY-s.StartY)*u
	}
	inv := 1.0 - u
	inv2 := inv * inv
	inv3 := inv2 * inv
	u2 := u * u
	u3 := u2 * u
	x := inv3*s.StartX + 3*inv2*u*s.Ctrl1X + 3*inv*u2*s.Ctrl2X + u3*s.EndX
	y := inv3*s.StartY + 3*inv2*u*s.Ctrl1Y + 3*inv*u2*s.Ctrl2Y + u3*s.EndY
	return x, y
}

// ParseGizmoPath parses an SVG path string (M, L, H, V, C, S, Q, T, Z) into editable GizmoSegments.
// For straight lines (L), Ctrl1 and Ctrl2 are initialized at the 1/3 and 2/3 points so dragging
// either handle smoothly promotes the segment into a cubic Bézier curve (C).
func ParseGizmoPath(pathData string) []GizmoSegment {
	pathData = strings.TrimSpace(pathData)
	if pathData == "" {
		return nil
	}
	tokens := tokenizeSVGPath(pathData)
	if len(tokens) == 0 {
		return nil
	}

	var segs []GizmoSegment
	var startX, startY float64
	var currX, currY float64
	var lastCtrlX, lastCtrlY float64
	var currCmd, prevCmd string

	nextFloat := func(idx *int) float64 {
		if *idx >= len(tokens) {
			return 0
		}
		var v float64
		_, _ = fmt.Sscanf(tokens[*idx], "%f", &v)
		*idx++
		return v
	}

	addLine := func(sx, sy, ex, ey float64) {
		segs = append(segs, GizmoSegment{
			StartX:  sx,
			StartY:  sy,
			Ctrl1X:  sx + (ex-sx)/3.0,
			Ctrl1Y:  sy + (ey-sy)/3.0,
			Ctrl2X:  sx + 2.0*(ex-sx)/3.0,
			Ctrl2Y:  sy + 2.0*(ey-sy)/3.0,
			EndX:    ex,
			EndY:    ey,
			IsCurve: false,
		})
	}

	addCubic := func(sx, sy, c1x, c1y, c2x, c2y, ex, ey float64) {
		segs = append(segs, GizmoSegment{
			StartX:  sx,
			StartY:  sy,
			Ctrl1X:  c1x,
			Ctrl1Y:  c1y,
			Ctrl2X:  c2x,
			Ctrl2Y:  c2y,
			EndX:    ex,
			EndY:    ey,
			IsCurve: true,
		})
	}

	addQuad := func(sx, sy, qx, qy, ex, ey float64) {
		c1x := sx + (2.0/3.0)*(qx-sx)
		c1y := sy + (2.0/3.0)*(qy-sy)
		c2x := ex + (2.0/3.0)*(qx-ex)
		c2y := ey + (2.0/3.0)*(qy-ey)
		addCubic(sx, sy, c1x, c1y, c2x, c2y, ex, ey)
	}

	i := 0
	for i < len(tokens) {
		tok := tokens[i]
		if len(tok) > 0 && ((tok[0] >= 'A' && tok[0] <= 'Z') || (tok[0] >= 'a' && tok[0] <= 'z')) {
			currCmd = tok
			i++
		} else {
			switch currCmd {
			case "M":
				currCmd = "L"
			case "m":
				currCmd = "l"
			}
		}

		switch currCmd {
		case "M":
			currX = nextFloat(&i)
			currY = nextFloat(&i)
			if len(segs) == 0 {
				startX, startY = currX, currY
			}
			lastCtrlX, lastCtrlY = currX, currY
		case "m":
			currX += nextFloat(&i)
			currY += nextFloat(&i)
			if len(segs) == 0 {
				startX, startY = currX, currY
			}
			lastCtrlX, lastCtrlY = currX, currY
		case "L":
			nx := nextFloat(&i)
			ny := nextFloat(&i)
			addLine(currX, currY, nx, ny)
			currX, currY = nx, ny
			lastCtrlX, lastCtrlY = currX, currY
		case "l":
			nx := currX + nextFloat(&i)
			ny := currY + nextFloat(&i)
			addLine(currX, currY, nx, ny)
			currX, currY = nx, ny
			lastCtrlX, lastCtrlY = currX, currY
		case "H":
			nx := nextFloat(&i)
			addLine(currX, currY, nx, currY)
			currX = nx
			lastCtrlX, lastCtrlY = currX, currY
		case "h":
			nx := currX + nextFloat(&i)
			addLine(currX, currY, nx, currY)
			currX = nx
			lastCtrlX, lastCtrlY = currX, currY
		case "V":
			ny := nextFloat(&i)
			addLine(currX, currY, currX, ny)
			currY = ny
			lastCtrlX, lastCtrlY = currX, currY
		case "v":
			ny := currY + nextFloat(&i)
			addLine(currX, currY, currX, ny)
			currY = ny
			lastCtrlX, lastCtrlY = currX, currY
		case "C":
			c1x := nextFloat(&i)
			c1y := nextFloat(&i)
			c2x := nextFloat(&i)
			c2y := nextFloat(&i)
			nx := nextFloat(&i)
			ny := nextFloat(&i)
			addCubic(currX, currY, c1x, c1y, c2x, c2y, nx, ny)
			currX, currY = nx, ny
			lastCtrlX, lastCtrlY = c2x, c2y
		case "c":
			c1x := currX + nextFloat(&i)
			c1y := currY + nextFloat(&i)
			c2x := currX + nextFloat(&i)
			c2y := currY + nextFloat(&i)
			nx := currX + nextFloat(&i)
			ny := currY + nextFloat(&i)
			addCubic(currX, currY, c1x, c1y, c2x, c2y, nx, ny)
			currX, currY = nx, ny
			lastCtrlX, lastCtrlY = c2x, c2y
		case "S":
			c1x, c1y := currX, currY
			if strings.EqualFold(prevCmd, "c") || strings.EqualFold(prevCmd, "s") {
				c1x = 2*currX - lastCtrlX
				c1y = 2*currY - lastCtrlY
			}
			c2x := nextFloat(&i)
			c2y := nextFloat(&i)
			nx := nextFloat(&i)
			ny := nextFloat(&i)
			addCubic(currX, currY, c1x, c1y, c2x, c2y, nx, ny)
			currX, currY = nx, ny
			lastCtrlX, lastCtrlY = c2x, c2y
		case "s":
			c1x, c1y := currX, currY
			if strings.EqualFold(prevCmd, "c") || strings.EqualFold(prevCmd, "s") {
				c1x = 2*currX - lastCtrlX
				c1y = 2*currY - lastCtrlY
			}
			c2x := currX + nextFloat(&i)
			c2y := currY + nextFloat(&i)
			nx := currX + nextFloat(&i)
			ny := currY + nextFloat(&i)
			addCubic(currX, currY, c1x, c1y, c2x, c2y, nx, ny)
			currX, currY = nx, ny
			lastCtrlX, lastCtrlY = c2x, c2y
		case "Q":
			qx := nextFloat(&i)
			qy := nextFloat(&i)
			nx := nextFloat(&i)
			ny := nextFloat(&i)
			addQuad(currX, currY, qx, qy, nx, ny)
			currX, currY = nx, ny
			lastCtrlX, lastCtrlY = qx, qy
		case "q":
			qx := currX + nextFloat(&i)
			qy := currY + nextFloat(&i)
			nx := currX + nextFloat(&i)
			ny := currY + nextFloat(&i)
			addQuad(currX, currY, qx, qy, nx, ny)
			currX, currY = nx, ny
			lastCtrlX, lastCtrlY = qx, qy
		case "T":
			qx, qy := currX, currY
			if strings.EqualFold(prevCmd, "q") || strings.EqualFold(prevCmd, "t") {
				qx = 2*currX - lastCtrlX
				qy = 2*currY - lastCtrlY
			}
			nx := nextFloat(&i)
			ny := nextFloat(&i)
			addQuad(currX, currY, qx, qy, nx, ny)
			currX, currY = nx, ny
			lastCtrlX, lastCtrlY = qx, qy
		case "t":
			qx, qy := currX, currY
			if strings.EqualFold(prevCmd, "q") || strings.EqualFold(prevCmd, "t") {
				qx = 2*currX - lastCtrlX
				qy = 2*currY - lastCtrlY
			}
			nx := currX + nextFloat(&i)
			ny := currY + nextFloat(&i)
			addQuad(currX, currY, qx, qy, nx, ny)
			currX, currY = nx, ny
			lastCtrlX, lastCtrlY = qx, qy
		case "Z", "z":
			if math.Hypot(currX-startX, currY-startY) > 1e-4 {
				addLine(currX, currY, startX, startY)
			}
			currX, currY = startX, startY
			lastCtrlX, lastCtrlY = currX, currY
		default:
			i++
		}
		prevCmd = currCmd
	}
	return segs
}

// FormatGizmoPath serializes a slice of GizmoSegments back into a clean SVG path `d` string.
func FormatGizmoPath(segs []GizmoSegment) string {
	if len(segs) == 0 {
		return ""
	}
	clean := func(v float64) float64 {
		r := math.Round(v*10) / 10
		if math.Abs(r) < 1e-9 {
			return 0
		}
		return r
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "M %.1f %.1f", clean(segs[0].StartX), clean(segs[0].StartY))
	for _, seg := range segs {
		if seg.IsCurve {
			fmt.Fprintf(&sb, " C %.1f %.1f %.1f %.1f %.1f %.1f",
				clean(seg.Ctrl1X), clean(seg.Ctrl1Y), clean(seg.Ctrl2X), clean(seg.Ctrl2Y), clean(seg.EndX), clean(seg.EndY))
		} else {
			fmt.Fprintf(&sb, " L %.1f %.1f", clean(seg.EndX), clean(seg.EndY))
		}
	}
	return sb.String()
}

func tokenizeSVGPath(d string) []string {
	var tokens []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			tokens = append(tokens, cur.String())
			cur.Reset()
		}
	}
	for i := 0; i < len(d); i++ {
		c := d[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == ',':
			flush()
		case (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z'):
			if (c == 'e' || c == 'E') && cur.Len() > 0 {
				cur.WriteByte(c)
			} else {
				flush()
				tokens = append(tokens, string(c))
			}
		case c == '-' || c == '+':
			if cur.Len() > 0 && cur.String()[cur.Len()-1] != 'e' && cur.String()[cur.Len()-1] != 'E' {
				flush()
			}
			cur.WriteByte(c)
		case c == '.':
			if strings.Contains(cur.String(), ".") {
				flush()
			}
			cur.WriteByte(c)
		default:
			cur.WriteByte(c)
		}
	}
	flush()
	return tokens
}

// snapPointTo15Deg constrains (targetX, targetY) to the nearest 15-degree ray originating
// from (originX, originY), preserving the Euclidean distance between origin and target.
func snapPointTo15Deg(originX, originY, targetX, targetY float64) (float64, float64) {
	dx := targetX - originX
	dy := targetY - originY
	dist := math.Hypot(dx, dy)
	if dist < 1e-6 {
		return targetX, targetY
	}
	snappedDeg := math.Round((math.Atan2(dy, dx)*180.0/math.Pi)/15.0) * 15.0
	rad := snappedDeg * math.Pi / 180.0
	c, s := math.Cos(rad), math.Sin(rad)
	if math.Abs(c) < 1e-12 {
		c = 0
	}
	if math.Abs(s) < 1e-12 {
		s = 0
	}
	resX := originX + dist*c
	resY := originY + dist*s
	if math.Abs(resX) < 1e-12 {
		resX = 0
	}
	if math.Abs(resY) < 1e-12 {
		resY = 0
	}
	return resX, resY
}

type gizmoDragMode int

const (
	gizmoDragNone gizmoDragMode = iota
	gizmoDragStagePos
	gizmoDragPivot
	gizmoDragPathAnchor // dragSegIdx = anchor index (0 .. len(segs)), where 0 is segs[0].Start and k>0 is segs[k-1].End
	gizmoDragPathCtrl1  // dragSegIdx = segment index
	gizmoDragPathCtrl2  // dragSegIdx = segment index
	gizmoDragPan
)

// CanvasGizmoOverlay renders interactive stage placement, pivot crosshair, and motion path
// line/spline handles directly over the CenterPreviewPanel viewport.
type CanvasGizmoOverlay struct {
	widget.BaseWidget
	mw     *MainWindow
	raster *canvas.Raster

	dragMode     gizmoDragMode
	dragSegIdx   int
	dragNodeID   string
	dragObjKey   string
	dragActIdx   int
	dragStartSVG [2]float64
	dragInitPos  [2]float64
	dragSegs     []GizmoSegment
	dragOrigSegs []GizmoSegment
	lastDragPos  fyne.Position
	hasLastDrag  bool
	panStartOff  fyne.Position
	mouseButton  desktop.MouseButton
	ctrlHeld     bool
	dragDirty    bool
}

var (
	_ desktop.Mouseable = (*CanvasGizmoOverlay)(nil)
	_ fyne.Scrollable   = (*CanvasGizmoOverlay)(nil)
)

// MouseDown syncs the Ctrl modifier state and active mouse button at click/drag start.
func (g *CanvasGizmoOverlay) MouseDown(ev *desktop.MouseEvent) {
	if ev != nil {
		g.ctrlHeld = (ev.Modifier & fyne.KeyModifierControl) != 0
		g.mouseButton = ev.Button
	}
}

// MouseUp syncs the Ctrl modifier state on button release.
func (g *CanvasGizmoOverlay) MouseUp(ev *desktop.MouseEvent) {
	if ev != nil {
		g.ctrlHeld = (ev.Modifier & fyne.KeyModifierControl) != 0
	}
	g.mouseButton = 0
}

// Scrolled handles Inkscape-style Ctrl+MouseWheel custom zooming around the cursor,
// forwarding unmodified scroll events to the enclosing stageScroll container.
func (g *CanvasGizmoOverlay) Scrolled(ev *fyne.ScrollEvent) {
	if ev == nil || g.mw == nil || g.mw.centerPanel == nil {
		return
	}
	cp := g.mw.centerPanel
	if g.ctrlHeld {
		dy := ev.Scrolled.DY
		if dy == 0 {
			dy = ev.Scrolled.DX
		}
		if dy == 0 {
			return
		}

		curZoom := cp.CurrentZoomScale()
		factor := math.Pow(1.15, float64(dy)/10.0)
		if math.Abs(float64(dy)) < 5 {
			if dy > 0 {
				factor = 1.15
			} else {
				factor = 1.0 / 1.15
			}
		}
		newZoom := clampFloat(curZoom*factor, 0.25, 8.0)

		var anchorSVG [2]float64
		var vpPos fyne.Position
		hasAnchor := false
		if cp.stageScroll != nil && cp.stageStack != nil {
			sz := g.Size()
			if _, viewToSVG, ok := g.viewportTransform(float64(sz.Width), float64(sz.Height)); ok {
				anchorSVG[0], anchorSVG[1] = viewToSVG(float64(ev.Position.X), float64(ev.Position.Y))
				vpPos = ev.Position.Subtract(cp.stageScroll.Offset)
				hasAnchor = true
			}
		}

		cp.SetZoom(newZoom)

		if hasAnchor && cp.stageScroll != nil && cp.stageStack != nil {
			sz := g.Size()
			if svgToView, _, ok := g.viewportTransform(float64(sz.Width), float64(sz.Height)); ok {
				nx, ny := svgToView(anchorSVG[0], anchorSVG[1])
				ss := cp.stageStack.Size()
				scSz := cp.stageScroll.Size()
				maxX := max(float32(0), ss.Width-scSz.Width)
				maxY := max(float32(0), ss.Height-scSz.Height)
				ox := min(max(float32(0), float32(nx)-vpPos.X), maxX)
				oy := min(max(float32(0), float32(ny)-vpPos.Y), maxY)
				cp.stageScroll.Offset = fyne.NewPos(ox, oy)
				cp.stageScroll.Refresh()
			}
		}
		return
	}

	if cp.stageScroll != nil {
		cp.stageScroll.Scrolled(ev)
	}
}

// SetCtrlHeld updates live Ctrl angle-snapping state and immediately re-evaluates an active path-anchor drag.
func (g *CanvasGizmoOverlay) SetCtrlHeld(held bool) {
	if g.ctrlHeld == held {
		return
	}
	g.ctrlHeld = held
	if g.dragMode == gizmoDragPathAnchor && g.hasLastDrag {
		g.applyDragAtPosition(g.lastDragPos)
	}
}

// NewCanvasGizmoOverlay creates an interactive canvas gizmo overlay bound to mw.
func NewCanvasGizmoOverlay(mw *MainWindow) *CanvasGizmoOverlay {
	g := &CanvasGizmoOverlay{
		mw:         mw,
		dragActIdx: -1,
	}
	g.raster = canvas.NewRaster(g.draw)
	g.ExtendBaseWidget(g)
	return g
}

func (g *CanvasGizmoOverlay) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(g.raster)
}

func (g *CanvasGizmoOverlay) MinSize() fyne.Size {
	return fyne.NewSize(150, 150)
}

func (g *CanvasGizmoOverlay) Cursor() desktop.Cursor {
	if g.dragMode != gizmoDragNone {
		return desktop.PointerCursor
	}
	return desktop.DefaultCursor
}

// viewportTransform computes the affine transform between SVG document coordinates and
// widget pixel coordinates (accounting for ImageFillContain and ExportSquare centering).
func (g *CanvasGizmoOverlay) viewportTransform(viewW, viewH float64) (svgToView func(sx, sy float64) (float64, float64), viewToSVG func(vx, vy float64) (float64, float64), ok bool) {
	if g.mw == nil || g.mw.session == nil || g.mw.session.Document == nil || viewW <= 4 || viewH <= 4 {
		return nil, nil, false
	}
	sess := g.mw.session
	pRect := sess.GetPreviewBoundaryRect()
	if pRect.Width <= 0 || pRect.Height <= 0 {
		return nil, nil, false
	}

	boundW := pRect.Width
	boundH := pRect.Height
	maxDim := max(boundW, boundH)
	renderW := math.Round(boundW * (512.0 / maxDim))
	renderH := math.Round(boundH * (512.0 / maxDim))
	if renderW <= 0 {
		renderW = 512
	}
	if renderH <= 0 {
		renderH = 512
	}

	dispW, dispH := renderW, renderH
	var sqOffX, sqOffY float64
	if sess.ExportOptions.ExportSquare {
		sqSide := max(renderW, renderH)
		sqOffX = (sqSide - renderW) / 2.0
		sqOffY = (sqSide - renderH) / 2.0
		dispW, dispH = sqSide, sqSide
	}

	scale := min(viewW/dispW, viewH/dispH)
	if scale <= 0 {
		return nil, nil, false
	}
	drawW := dispW * scale
	drawH := dispH * scale
	padX := (viewW - drawW) / 2.0
	padY := (viewH - drawH) / 2.0

	if g.mw.centerPanel != nil && g.mw.centerPanel.stageStack != nil && g.mw.centerPanel.mainCanvasStack != nil {
		ss := g.mw.centerPanel.stageStack.Size()
		mc := g.mw.centerPanel.mainCanvasStack
		mcSize := mc.Size()
		if ss.Width > 4 && ss.Height > 4 && mcSize.Width > 4 && mcSize.Height > 4 &&
			math.Abs(viewW-float64(ss.Width)) < 1.0 && math.Abs(viewH-float64(ss.Height)) < 1.0 {
			mcPos := mc.Position()
			scale = min(float64(mcSize.Width)/dispW, float64(mcSize.Height)/dispH)
			if scale <= 0 {
				return nil, nil, false
			}
			drawW = dispW * scale
			drawH = dispH * scale
			padX = float64(mcPos.X) + (float64(mcSize.Width)-drawW)/2.0
			padY = float64(mcPos.Y) + (float64(mcSize.Height)-drawH)/2.0
		}
	}

	s2v := func(sx, sy float64) (float64, float64) {
		rx := ((sx - pRect.X) / pRect.Width) * renderW
		ry := ((sy - pRect.Y) / pRect.Height) * renderH
		dx := rx + sqOffX
		dy := ry + sqOffY
		return padX + dx*scale, padY + dy*scale
	}

	v2s := func(vx, vy float64) (float64, float64) {
		dx := (vx - padX) / scale
		dy := (vy - padY) / scale
		rx := dx - sqOffX
		ry := dy - sqOffY
		sx := pRect.X + (rx/renderW)*pRect.Width
		sy := pRect.Y + (ry/renderH)*pRect.Height
		return math.Round(sx*10) / 10, math.Round(sy*10) / 10
	}

	return s2v, v2s, true
}

func (g *CanvasGizmoOverlay) extraStageBoundsSVG() (inksvg.Rect, bool) {
	if g == nil || g.mw == nil || g.mw.session == nil || g.mw.session.Document == nil {
		return inksvg.Rect{}, false
	}
	minX, minY := math.MaxFloat64, math.MaxFloat64
	maxX, maxY := -math.MaxFloat64, -math.MaxFloat64
	hasAny := false
	includePt := func(x, y float64) {
		if x < minX {
			minX = x
		}
		if x > maxX {
			maxX = x
		}
		if y < minY {
			minY = y
		}
		if y > maxY {
			maxY = y
		}
		hasAny = true
	}

	nodeID, _, objCfg, _, moveActIdx, moveSegs := g.activeSelection()
	if nodeID != "" {
		if rect, pivX, pivY, hasBounds := g.resolveObjectBoundsAndPivot(nodeID, objCfg); hasBounds {
			includePt(rect.X, rect.Y)
			includePt(rect.X+rect.Width, rect.Y+rect.Height)
			includePt(pivX, pivY)
		}
	}
	if moveActIdx >= 0 {
		for _, seg := range moveSegs {
			includePt(seg.StartX, seg.StartY)
			includePt(seg.Ctrl1X, seg.Ctrl1Y)
			includePt(seg.Ctrl2X, seg.Ctrl2Y)
			includePt(seg.EndX, seg.EndY)
		}
	}
	if g.mw.session.Objects != nil {
		for _, cfg := range g.mw.session.Objects {
			for _, act := range cfg.Actions {
				if isEditablePathAction(act) {
					for _, seg := range ParseGizmoPath(act.PathD) {
						includePt(seg.StartX, seg.StartY)
						includePt(seg.Ctrl1X, seg.Ctrl1Y)
						includePt(seg.Ctrl2X, seg.Ctrl2Y)
						includePt(seg.EndX, seg.EndY)
					}
				}
			}
		}
	}
	if !hasAny || maxX < minX || maxY < minY {
		return inksvg.Rect{}, false
	}
	return inksvg.Rect{
		X:      minX,
		Y:      minY,
		Width:  maxX - minX,
		Height: maxY - minY,
	}, true
}

func isEditablePathAction(act app.ActionConfig) bool {
	t := strings.ToLower(act.Type)
	return t == "move" || t == "push" || (t == "gravity" && strings.TrimSpace(act.PathD) != "")
}

func (g *CanvasGizmoOverlay) activeSelection() (nodeID, objKey string, objCfg app.ObjectConfig, hasObj bool, moveActIdx int, moveSegs []GizmoSegment) {
	moveActIdx = -1
	if g.mw == nil || g.mw.session == nil || g.mw.session.Document == nil || g.mw.editorState == nil {
		return
	}
	state := g.mw.editorState

	loadMoveSegs := func(idx int, act app.ActionConfig) {
		moveActIdx = idx
		if g.dragMode >= gizmoDragPathAnchor && g.dragActIdx == idx && len(g.dragSegs) > 0 {
			moveSegs = g.dragSegs
		} else {
			moveSegs = ParseGizmoPath(act.PathD)
		}
	}

	// 1. Check if a specific action is selected (prioritizing ActiveNode first for deterministic resolution)
	if g.mw.selectedActionID != "" {
		if state.ActiveNode != nil {
			k, cfg, ok := g.mw.resolveNodeObjectConfig(state.ActiveNode)
			if ok {
				for idx, act := range cfg.Actions {
					if act.ID == g.mw.selectedActionID {
						nodeID = state.ActiveNode.ID
						objKey = k
						objCfg = cfg
						hasObj = true
						if isEditablePathAction(act) {
							loadMoveSegs(idx, act)
						}
						return
					}
				}
			}
		}

		for _, k := range slices.Sorted(maps.Keys(g.mw.session.Objects)) {
			cfg := g.mw.session.Objects[k]
			for idx, act := range cfg.Actions {
				if act.ID == g.mw.selectedActionID {
					nID := cfg.FallbackXMLID
					if nID == "" {
						nID = k
					}
					nodeID = nID
					objKey = k
					objCfg = cfg
					hasObj = true
					if isEditablePathAction(act) {
						loadMoveSegs(idx, act)
					}
					return
				}
			}
		}
	}

	// 2. Otherwise inspect ActiveNode (Track selection mode: Stage Placement, Pivot, and active track's Move/Push path)
	if state.ActiveNode != nil {
		nodeID = state.ActiveNode.ID
		objKey, objCfg, hasObj = g.mw.resolveNodeObjectConfig(state.ActiveNode)
		if objKey == "" {
			objKey = nodeID
		}
		if hasObj {
			for idx, act := range slices.Backward(objCfg.Actions) {
				if isEditablePathAction(act) {
					loadMoveSegs(idx, act)
					break
				}
			}
		}
	}
	return
}

func (g *CanvasGizmoOverlay) resolveObjectBoundsAndPivot(nodeID string, objCfg app.ObjectConfig) (rect inksvg.Rect, pivotX, pivotY float64, hasBounds bool) {
	if g.mw == nil || g.mw.session == nil || g.mw.session.Document == nil || nodeID == "" {
		return
	}
	doc := g.mw.session.Document
	native := doc.GetElementRect(nodeID)
	if native.Width <= 0 && native.Height <= 0 {
		return
	}
	hasBounds = true
	rect = native
	if g.dragMode == gizmoDragStagePos && g.dragNodeID == nodeID {
		rect.X = g.dragInitPos[0]
		rect.Y = g.dragInitPos[1]
	} else if objCfg.OverrideStartPos {
		rect.X = objCfg.StartX
		rect.Y = objCfg.StartY
	}

	if g.dragMode == gizmoDragPivot && g.dragNodeID == nodeID {
		pivotX = g.dragInitPos[0]
		pivotY = g.dragInitPos[1]
		return
	}

	switch objCfg.PivotType {
	case "custom":
		pivotX = objCfg.PivotX
		pivotY = objCfg.PivotY
	case "top":
		pivotX = rect.X + rect.Width/2.0
		pivotY = rect.Y
	case "bottom":
		pivotX = rect.X + rect.Width/2.0
		pivotY = rect.Y + rect.Height
	case "left":
		pivotX = rect.X
		pivotY = rect.Y + rect.Height/2.0
	case "right":
		pivotX = rect.X + rect.Width
		pivotY = rect.Y + rect.Height/2.0
	default:
		pivotX = rect.X + rect.Width/2.0
		pivotY = rect.Y + rect.Height/2.0
	}
	return
}

func (g *CanvasGizmoOverlay) draw(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, max(w, 1), max(h, 1)))
	sz := g.Size()
	if sz.Width <= 4 || sz.Height <= 4 {
		return img
	}
	scaleX := float64(w) / float64(sz.Width)
	scaleY := float64(h) / float64(sz.Height)

	svgToView, _, ok := g.viewportTransform(float64(sz.Width), float64(sz.Height))
	if !ok {
		return img
	}
	toPx := func(sx, sy float64) (int, int) {
		vx, vy := svgToView(sx, sy)
		return int(math.Round(vx * scaleX)), int(math.Round(vy * scaleY))
	}

	nodeID, _, objCfg, _, moveActIdx, moveSegs := g.activeSelection()
	if nodeID == "" && moveActIdx < 0 {
		return img
	}

	// 1. Draw Stage Object Bounding Box, Stage Move Handle (✥), and Pivot Crosshair (⊕)
	if nodeID != "" {
		rect, pivX, pivY, hasBounds := g.resolveObjectBoundsAndPivot(nodeID, objCfg)
		if hasBounds {
			x0, y0 := toPx(rect.X, rect.Y)
			x1, y1 := toPx(rect.X+rect.Width, rect.Y+rect.Height)
			if x0 > x1 {
				x0, x1 = x1, x0
			}
			if y0 > y1 {
				y0, y1 = y1, y0
			}
			boxCol := color.RGBA{R: 56, G: 189, B: 248, A: 200} // #38BDF8
			if objCfg.IsSolid {
				boxCol = color.RGBA{R: 16, G: 185, B: 129, A: 245} // Solid Emerald Collider #10B981
			} else if objCfg.OverrideStartPos || g.dragMode == gizmoDragStagePos {
				boxCol = color.RGBA{R: 52, G: 211, B: 153, A: 230} // Emerald when overridden
			}
			drawGizmoDashedRect(img, x0, y0, x1, y1, boxCol)
			if objCfg.IsSolid {
				drawGizmoDashedRect(img, x0-1, y0-1, x1+1, y1+1, boxCol)
			}

			// Stage position handle at top-left corner of bounding box
			drawGizmoSquareHandle(img, x0, y0, int(math.Round(5*scaleX)), boxCol, color.RGBA{R: 15, G: 23, B: 42, A: 240})

			// Pivot crosshair at (pivX, pivY)
			px, py := toPx(pivX, pivY)
			pivCol := color.RGBA{R: 250, G: 204, B: 21, A: 245} // Amber/Gold #FACC15
			if objCfg.PivotType == "custom" || g.dragMode == gizmoDragPivot {
				pivCol = color.RGBA{R: 249, G: 115, B: 22, A: 255} // Orange #F97316
			}
			drawGizmoCrosshair(img, px, py, int(math.Round(7*scaleX)), pivCol)
		}
	}

	// 2. Draw Interactive Motion Path Line / Cubic Bézier Spline / Physics Force Vector Gizmo
	if moveActIdx >= 0 && len(moveSegs) > 0 {
		actType := "move"
		if moveActIdx < len(objCfg.Actions) {
			actType = strings.ToLower(objCfg.Actions[moveActIdx].Type)
		}
		isForceVector := actType == "push" || actType == "gravity"

		pathCasing := color.RGBA{R: 15, G: 23, B: 42, A: 220}
		pathStroke := color.RGBA{R: 56, G: 189, B: 248, A: 245}    // Electric Sky Blue #38BDF8
		tangentCol := color.RGBA{R: 244, G: 114, B: 182, A: 200}   // Pink/Magenta #F472B6
		ctrlFill := color.RGBA{R: 244, G: 114, B: 182, A: 245}     // Curve handle diamond
		lineCtrlFill := color.RGBA{R: 148, G: 163, B: 184, A: 210} // Subtle handle on straight line
		startFill := color.RGBA{R: 34, G: 197, B: 94, A: 255}      // Green #22C55E
		endFill := color.RGBA{R: 56, G: 189, B: 248, A: 255}       // Sky Blue #38BDF8
		midFill := color.RGBA{R: 250, G: 204, B: 21, A: 255}       // Gold waypoint

		switch actType {
		case "push":
			pathStroke = color.RGBA{R: 251, G: 146, B: 60, A: 250} // Tangerine Orange #FB923C
			endFill = color.RGBA{R: 251, G: 146, B: 60, A: 255}
		case "gravity":
			pathStroke = color.RGBA{R: 45, G: 212, B: 191, A: 250} // Cyan/Teal #2DD4BF
			endFill = color.RGBA{R: 45, G: 212, B: 191, A: 255}
		}

		// Draw curve / line segments
		for _, seg := range moveSegs {
			const steps = 32
			prevX, prevY := toPx(seg.StartX, seg.StartY)
			for s := 1; s <= steps; s++ {
				u := float64(s) / float64(steps)
				sx, sy := seg.EvaluateAt(u)
				nx, ny := toPx(sx, sy)
				drawGizmoThickLine(img, prevX, prevY, nx, ny, 2, pathCasing)
				drawGizmoThickLine(img, prevX, prevY, nx, ny, 1, pathStroke)
				prevX, prevY = nx, ny
			}

			// Draw cubic Bézier tangent arms & control handles
			ax0, ay0 := toPx(seg.StartX, seg.StartY)
			ax1, ay1 := toPx(seg.EndX, seg.EndY)
			c1x, c1y := toPx(seg.Ctrl1X, seg.Ctrl1Y)
			c2x, c2y := toPx(seg.Ctrl2X, seg.Ctrl2Y)

			if seg.IsCurve {
				drawGizmoLine(img, ax0, ay0, c1x, c1y, tangentCol)
				drawGizmoLine(img, ax1, ay1, c2x, c2y, tangentCol)
				drawGizmoDiamond(img, c1x, c1y, int(math.Round(5*scaleX)), ctrlFill, pathCasing)
				drawGizmoDiamond(img, c2x, c2y, int(math.Round(5*scaleX)), ctrlFill, pathCasing)
			} else if !isForceVector {
				// On straight line segments, show subtle curve handles at 1/3 and 2/3 so the user can drag them to bend into a curve
				drawGizmoDiamond(img, c1x, c1y, int(math.Round(4*scaleX)), lineCtrlFill, pathCasing)
				drawGizmoDiamond(img, c2x, c2y, int(math.Round(4*scaleX)), lineCtrlFill, pathCasing)
			}
		}

		if isForceVector {
			lastSeg := moveSegs[len(moveSegs)-1]
			ex, ey := toPx(lastSeg.EndX, lastSeg.EndY)
			tx, ty := toPx(lastSeg.StartX, lastSeg.StartY)
			if lastSeg.IsCurve {
				u9 := 0.9
				px9, py9 := lastSeg.EvaluateAt(u9)
				tx, ty = toPx(px9, py9)
			}
			drawGizmoArrowhead(img, tx, ty, ex, ey, int(math.Round(11*scaleX)), pathStroke, pathCasing)
		}

		// Draw anchor nodes (Start = green, intermediate = gold, End = sky blue / orange)
		rAnchor := int(math.Round(6 * scaleX))
		sx0, sy0 := toPx(moveSegs[0].StartX, moveSegs[0].StartY)
		drawGizmoCircle(img, sx0, sy0, rAnchor, startFill, pathCasing)
		for idx, seg := range moveSegs {
			ex, ey := toPx(seg.EndX, seg.EndY)
			col := midFill
			if idx == len(moveSegs)-1 {
				col = endFill
			}
			drawGizmoCircle(img, ex, ey, rAnchor, col, pathCasing)
		}
	}

	return img
}

func (g *CanvasGizmoOverlay) hitTest(pos fyne.Position) (mode gizmoDragMode, segIdx int) {
	sz := g.Size()
	svgToView, _, ok := g.viewportTransform(float64(sz.Width), float64(sz.Height))
	if !ok {
		return gizmoDragNone, -1
	}
	vx, vy := float64(pos.X), float64(pos.Y)
	const hitRadius = 12.0

	nodeID, _, objCfg, _, moveActIdx, moveSegs := g.activeSelection()

	checkMoveHandles := func() (gizmoDragMode, int, bool) {
		if moveActIdx >= 0 && len(moveSegs) > 0 {
			sx, sy := svgToView(moveSegs[0].StartX, moveSegs[0].StartY)
			if math.Hypot(vx-sx, vy-sy) <= hitRadius {
				return gizmoDragPathAnchor, 0, true
			}
			for idx, seg := range moveSegs {
				ex, ey := svgToView(seg.EndX, seg.EndY)
				if math.Hypot(vx-ex, vy-ey) <= hitRadius {
					return gizmoDragPathAnchor, idx + 1, true
				}
			}
			for idx, seg := range moveSegs {
				c1x, c1y := svgToView(seg.Ctrl1X, seg.Ctrl1Y)
				if math.Hypot(vx-c1x, vy-c1y) <= hitRadius {
					return gizmoDragPathCtrl1, idx, true
				}
				c2x, c2y := svgToView(seg.Ctrl2X, seg.Ctrl2Y)
				if math.Hypot(vx-c2x, vy-c2y) <= hitRadius {
					return gizmoDragPathCtrl2, idx, true
				}
			}
		}
		return gizmoDragNone, -1, false
	}

	if g.mw != nil && g.mw.selectedActionID != "" {
		// When an action bar is selected in the timeline, prioritize its path handles first
		if m, idx, hit := checkMoveHandles(); hit {
			return m, idx
		}
	}

	// Check explicit Pivot Crosshair & top-left Stage Position Handle
	var hasBounds bool
	var bx0, by0, bx1, by1 float64
	if nodeID != "" {
		var rect inksvg.Rect
		var pivX, pivY float64
		rect, pivX, pivY, hasBounds = g.resolveObjectBoundsAndPivot(nodeID, objCfg)
		if hasBounds {
			px, py := svgToView(pivX, pivY)
			if math.Hypot(vx-px, vy-py) <= hitRadius {
				return gizmoDragPivot, -1
			}
			bx0, by0 = svgToView(rect.X, rect.Y)
			bx1, by1 = svgToView(rect.X+rect.Width, rect.Y+rect.Height)
			if math.Hypot(vx-bx0, vy-by0) <= hitRadius {
				return gizmoDragStagePos, -1
			}
		}
	}

	// Check Motion Path Anchors & Curve Handles (even when in Track selection mode)
	if m, idx, hit := checkMoveHandles(); hit {
		return m, idx
	}

	// Finally check interior of object bounding box for stage drag
	if hasBounds {
		minX, maxX := min(bx0, bx1), max(bx0, bx1)
		minY, maxY := min(by0, by1), max(by0, by1)
		if vx >= minX && vx <= maxX && vy >= minY && vy <= maxY {
			return gizmoDragStagePos, -1
		}
	}

	return gizmoDragNone, -1
}

// Dragged handles interactive dragging of motion path anchors, cubic Bézier handles, pivot crosshairs, and stage positions.
func (g *CanvasGizmoOverlay) Dragged(ev *fyne.DragEvent) {
	sz := g.Size()
	_, viewToSVG, ok := g.viewportTransform(float64(sz.Width), float64(sz.Height))
	if !ok {
		return
	}

	if g.dragMode == gizmoDragNone {
		startPos := fyne.NewPos(ev.Position.X-ev.Dragged.DX, ev.Position.Y-ev.Dragged.DY)
		if g.mouseButton == desktop.MouseButtonTertiary {
			if g.mw != nil && g.mw.centerPanel != nil && g.mw.centerPanel.stageScroll != nil && g.mw.centerPanel.stageStack != nil {
				sc := g.mw.centerPanel.stageScroll
				ss := g.mw.centerPanel.stageStack.Size()
				if ss.Width > sc.Size().Width+1 || ss.Height > sc.Size().Height+1 {
					g.dragMode = gizmoDragPan
					g.panStartOff = sc.Offset
					g.dragStartSVG = [2]float64{float64(startPos.X), float64(startPos.Y)}
				}
			}
			if g.dragMode == gizmoDragNone {
				return
			}
		} else {
			mode, segIdx := g.hitTest(startPos)
			if mode == gizmoDragNone {
				if g.mw != nil && g.mw.centerPanel != nil && g.mw.centerPanel.stageScroll != nil && g.mw.centerPanel.stageStack != nil {
					sc := g.mw.centerPanel.stageScroll
					ss := g.mw.centerPanel.stageStack.Size()
					if ss.Width > sc.Size().Width+1 || ss.Height > sc.Size().Height+1 {
						g.dragMode = gizmoDragPan
						g.panStartOff = sc.Offset
						g.dragStartSVG = [2]float64{float64(startPos.X), float64(startPos.Y)}
					}
				}
				if g.dragMode == gizmoDragNone {
					return
				}
			}
			if g.dragMode == gizmoDragNone {
				nodeID, objKey, objCfg, _, moveActIdx, moveSegs := g.activeSelection()
				g.dragMode = mode
				g.dragSegIdx = segIdx
				g.dragNodeID = nodeID
				g.dragObjKey = objKey
				g.dragActIdx = moveActIdx
				g.dragStartSVG[0], g.dragStartSVG[1] = viewToSVG(float64(startPos.X), float64(startPos.Y))
				g.dragDirty = false

				switch mode {
				case gizmoDragStagePos:
					rect, _, _, _ := g.resolveObjectBoundsAndPivot(nodeID, objCfg)
					g.dragInitPos = [2]float64{rect.X, rect.Y}
				case gizmoDragPivot:
					_, pivX, pivY, _ := g.resolveObjectBoundsAndPivot(nodeID, objCfg)
					g.dragInitPos = [2]float64{pivX, pivY}
				case gizmoDragPathAnchor, gizmoDragPathCtrl1, gizmoDragPathCtrl2:
					g.dragSegs = slices.Clone(moveSegs)
					g.dragOrigSegs = slices.Clone(moveSegs)
				}
			}
		}
	}

	if g.dragMode == gizmoDragPan {
		if g.mw != nil && g.mw.centerPanel != nil && g.mw.centerPanel.stageScroll != nil && g.mw.centerPanel.stageStack != nil {
			sc := g.mw.centerPanel.stageScroll
			ss := g.mw.centerPanel.stageStack.Size()
			scSz := sc.Size()
			maxX := max(float32(0), ss.Width-scSz.Width)
			maxY := max(float32(0), ss.Height-scSz.Height)
			nx := min(max(float32(0), g.panStartOff.X-(ev.Position.X-float32(g.dragStartSVG[0]))), maxX)
			ny := min(max(float32(0), g.panStartOff.Y-(ev.Position.Y-float32(g.dragStartSVG[1]))), maxY)
			sc.Offset = fyne.NewPos(nx, ny)
			sc.Refresh()
		}
		return
	}

	g.lastDragPos = ev.Position
	g.hasLastDrag = true
	g.applyDragAtPosition(ev.Position)
}

func (g *CanvasGizmoOverlay) applyDragAtPosition(pos fyne.Position) {
	sz := g.Size()
	_, viewToSVG, ok := g.viewportTransform(float64(sz.Width), float64(sz.Height))
	if !ok {
		return
	}
	curSVGX, curSVGY := viewToSVG(float64(pos.X), float64(pos.Y))

	switch g.dragMode {
	case gizmoDragStagePos:
		dx := curSVGX - g.dragStartSVG[0]
		dy := curSVGY - g.dragStartSVG[1]
		g.dragInitPos[0] = math.Round((g.dragInitPos[0]+dx)*10) / 10
		g.dragInitPos[1] = math.Round((g.dragInitPos[1]+dy)*10) / 10
		g.dragStartSVG = [2]float64{curSVGX, curSVGY}
		g.dragDirty = true
		g.raster.Refresh()

	case gizmoDragPivot:
		g.dragInitPos = [2]float64{curSVGX, curSVGY}
		g.dragDirty = true
		g.raster.Refresh()

	case gizmoDragPathAnchor:
		if len(g.dragSegs) == 0 || len(g.dragOrigSegs) == 0 {
			return
		}
		anchorIdx := g.dragSegIdx
		if anchorIdx == 0 {
			if g.ctrlHeld {
				curSVGX, curSVGY = snapPointTo15Deg(g.dragSegs[0].EndX, g.dragSegs[0].EndY, curSVGX, curSVGY)
				curSVGX = math.Round(curSVGX*10) / 10
				curSVGY = math.Round(curSVGY*10) / 10
				if math.Abs(curSVGX) < 1e-9 {
					curSVGX = 0
				}
				if math.Abs(curSVGY) < 1e-9 {
					curSVGY = 0
				}
			}
			totalDX := curSVGX - g.dragOrigSegs[0].StartX
			totalDY := curSVGY - g.dragOrigSegs[0].StartY
			g.dragSegs[0].StartX = curSVGX
			g.dragSegs[0].StartY = curSVGY
			if g.dragSegs[0].IsCurve {
				g.dragSegs[0].Ctrl1X = math.Round((g.dragOrigSegs[0].Ctrl1X+totalDX)*10) / 10
				g.dragSegs[0].Ctrl1Y = math.Round((g.dragOrigSegs[0].Ctrl1Y+totalDY)*10) / 10
			} else {
				syncStraightLineControls(&g.dragSegs[0])
			}
		} else if anchorIdx >= 1 && anchorIdx <= len(g.dragSegs) {
			prevSeg := &g.dragSegs[anchorIdx-1]
			if g.ctrlHeld {
				curSVGX, curSVGY = snapPointTo15Deg(prevSeg.StartX, prevSeg.StartY, curSVGX, curSVGY)
				curSVGX = math.Round(curSVGX*10) / 10
				curSVGY = math.Round(curSVGY*10) / 10
				if math.Abs(curSVGX) < 1e-9 {
					curSVGX = 0
				}
				if math.Abs(curSVGY) < 1e-9 {
					curSVGY = 0
				}
			}
			totalDX := curSVGX - g.dragOrigSegs[anchorIdx-1].EndX
			totalDY := curSVGY - g.dragOrigSegs[anchorIdx-1].EndY
			prevSeg.EndX = curSVGX
			prevSeg.EndY = curSVGY
			if prevSeg.IsCurve {
				prevSeg.Ctrl2X = math.Round((g.dragOrigSegs[anchorIdx-1].Ctrl2X+totalDX)*10) / 10
				prevSeg.Ctrl2Y = math.Round((g.dragOrigSegs[anchorIdx-1].Ctrl2Y+totalDY)*10) / 10
			} else {
				syncStraightLineControls(prevSeg)
			}
			if anchorIdx < len(g.dragSegs) {
				nextSeg := &g.dragSegs[anchorIdx]
				nextSeg.StartX = curSVGX
				nextSeg.StartY = curSVGY
				if nextSeg.IsCurve {
					nextSeg.Ctrl1X = math.Round((g.dragOrigSegs[anchorIdx].Ctrl1X+totalDX)*10) / 10
					nextSeg.Ctrl1Y = math.Round((g.dragOrigSegs[anchorIdx].Ctrl1Y+totalDY)*10) / 10
				} else {
					syncStraightLineControls(nextSeg)
				}
			}
		}
		g.dragDirty = true
		g.raster.Refresh()

	case gizmoDragPathCtrl1:
		if g.dragSegIdx >= 0 && g.dragSegIdx < len(g.dragSegs) {
			seg := &g.dragSegs[g.dragSegIdx]
			seg.IsCurve = true
			seg.Ctrl1X = curSVGX
			seg.Ctrl1Y = curSVGY
			g.dragDirty = true
			g.raster.Refresh()
		}

	case gizmoDragPathCtrl2:
		if g.dragSegIdx >= 0 && g.dragSegIdx < len(g.dragSegs) {
			seg := &g.dragSegs[g.dragSegIdx]
			seg.IsCurve = true
			seg.Ctrl2X = curSVGX
			seg.Ctrl2Y = curSVGY
			g.dragDirty = true
			g.raster.Refresh()
		}
	}
	if g.dragDirty && g.mw != nil && g.mw.centerPanel != nil && g.mw.centerPanel.stageStack != nil {
		if g.Size() == g.mw.centerPanel.stageStack.Size() {
			g.mw.centerPanel.RefreshStageLayout()
		}
	}
}

// DragEnd commits the modified stage position, custom pivot, or motion path to .inka Objects and triggers a preview update.
func (g *CanvasGizmoOverlay) DragEnd() {
	if g.dragMode == gizmoDragNone {
		return
	}
	if g.dragMode == gizmoDragPan {
		g.dragMode = gizmoDragNone
		return
	}
	mode := g.dragMode
	nodeID := g.dragNodeID
	objKey := g.dragObjKey
	actIdx := g.dragActIdx
	pos := g.dragInitPos
	segs := g.dragSegs
	dirty := g.dragDirty

	g.dragMode = gizmoDragNone
	g.dragSegIdx = -1
	g.dragSegs = nil
	g.dragOrigSegs = nil
	g.hasLastDrag = false
	g.dragDirty = false

	if !dirty || g.mw == nil || g.mw.session == nil || g.mw.timelinePanel == nil {
		g.raster.Refresh()
		return
	}

	if g.mw.session.Objects == nil {
		g.mw.session.Objects = make(map[string]app.ObjectConfig)
	}
	cfg := g.mw.session.Objects[objKey]
	if cfg.FallbackXMLID == "" {
		cfg.FallbackXMLID = nodeID
	}

	switch mode {
	case gizmoDragStagePos:
		cfg.OverrideStartPos = true
		cfg.StartX = pos[0]
		cfg.StartY = pos[1]
		g.mw.timelinePanel.CommitTrackConfig(nodeID, objKey, cfg)
		g.mw.timelinePanel.Refresh()

	case gizmoDragPivot:
		cfg.PivotType = "custom"
		cfg.PivotX = pos[0]
		cfg.PivotY = pos[1]
		g.mw.timelinePanel.CommitTrackConfig(nodeID, objKey, cfg)
		g.mw.timelinePanel.Refresh()

	case gizmoDragPathAnchor, gizmoDragPathCtrl1, gizmoDragPathCtrl2:
		if actIdx >= 0 && actIdx < len(cfg.Actions) && len(segs) > 0 {
			cfg.Actions[actIdx].PathD = FormatGizmoPath(segs)
			g.mw.timelinePanel.CommitTrackConfig(nodeID, objKey, cfg)
			g.mw.timelinePanel.Refresh()
		}
	}
	g.raster.Refresh()
}

// DoubleTapped allows inserting a new waypoint along a motion path segment, deleting an intermediate
// waypoint, toggling a curve handle back to a straight line, or resetting a custom pivot to center.
func (g *CanvasGizmoOverlay) DoubleTapped(ev *fyne.PointEvent) {
	if g.mw == nil || g.mw.session == nil || g.mw.timelinePanel == nil {
		return
	}
	sz := g.Size()
	svgToView, _, ok := g.viewportTransform(float64(sz.Width), float64(sz.Height))
	if !ok {
		return
	}
	nodeID, objKey, objCfg, _, moveActIdx, moveSegs := g.activeSelection()
	if nodeID == "" && moveActIdx < 0 {
		return
	}

	mode, segIdx := g.hitTest(ev.Position)
	switch mode {
	case gizmoDragPivot:
		// Reset custom pivot back to center
		objCfg.PivotType = "center"
		objCfg.PivotX = 0
		objCfg.PivotY = 0
		g.mw.timelinePanel.CommitTrackConfig(nodeID, objKey, objCfg)
		g.mw.timelinePanel.Refresh()
		g.raster.Refresh()
		return

	case gizmoDragPathCtrl1, gizmoDragPathCtrl2:
		// Double-clicking a curve handle straightens that segment back into a line (L)
		if moveActIdx >= 0 && segIdx >= 0 && segIdx < len(moveSegs) {
			moveSegs[segIdx].IsCurve = false
			syncStraightLineControls(&moveSegs[segIdx])
			objCfg.Actions[moveActIdx].PathD = FormatGizmoPath(moveSegs)
			g.mw.timelinePanel.CommitTrackConfig(nodeID, objKey, objCfg)
			g.mw.timelinePanel.Refresh()
			g.raster.Refresh()
			return
		}

	case gizmoDragPathAnchor:
		// Double-clicking an intermediate anchor (1 .. len(moveSegs)-1) removes that waypoint
		if moveActIdx >= 0 && len(moveSegs) > 1 && segIdx >= 1 && segIdx < len(moveSegs) {
			prev := moveSegs[segIdx-1]
			next := moveSegs[segIdx]
			merged := GizmoSegment{
				StartX:  prev.StartX,
				StartY:  prev.StartY,
				Ctrl1X:  prev.Ctrl1X,
				Ctrl1Y:  prev.Ctrl1Y,
				Ctrl2X:  next.Ctrl2X,
				Ctrl2Y:  next.Ctrl2Y,
				EndX:    next.EndX,
				EndY:    next.EndY,
				IsCurve: prev.IsCurve || next.IsCurve,
			}
			if !merged.IsCurve {
				syncStraightLineControls(&merged)
			}
			newSegs := make([]GizmoSegment, 0, len(moveSegs)-1)
			newSegs = append(newSegs, moveSegs[:segIdx-1]...)
			newSegs = append(newSegs, merged)
			newSegs = append(newSegs, moveSegs[segIdx+1:]...)
			objCfg.Actions[moveActIdx].PathD = FormatGizmoPath(newSegs)
			g.mw.timelinePanel.CommitTrackConfig(nodeID, objKey, objCfg)
			g.mw.timelinePanel.Refresh()
			g.raster.Refresh()
			return
		}
	}

	// Check if double-click is near a motion path segment -> split segment and insert a new waypoint
	if moveActIdx >= 0 && len(moveSegs) > 0 {
		vx, vy := float64(ev.Position.X), float64(ev.Position.Y)
		bestSeg := -1
		bestDist := 14.0
		var bestSVGX, bestSVGY float64
		for idx, seg := range moveSegs {
			const samples = 24
			for s := 1; s < samples; s++ {
				u := float64(s) / float64(samples)
				sx, sy := seg.EvaluateAt(u)
				px, py := svgToView(sx, sy)
				d := math.Hypot(vx-px, vy-py)
				if d < bestDist {
					bestDist = d
					bestSeg = idx
					bestSVGX = math.Round(sx*10) / 10
					bestSVGY = math.Round(sy*10) / 10
				}
			}
		}
		if bestSeg >= 0 {
			orig := moveSegs[bestSeg]
			segA := GizmoSegment{
				StartX: orig.StartX,
				StartY: orig.StartY,
				EndX:   bestSVGX,
				EndY:   bestSVGY,
			}
			segB := GizmoSegment{
				StartX: bestSVGX,
				StartY: bestSVGY,
				EndX:   orig.EndX,
				EndY:   orig.EndY,
			}
			syncStraightLineControls(&segA)
			syncStraightLineControls(&segB)
			newSegs := make([]GizmoSegment, 0, len(moveSegs)+1)
			newSegs = append(newSegs, moveSegs[:bestSeg]...)
			newSegs = append(newSegs, segA, segB)
			newSegs = append(newSegs, moveSegs[bestSeg+1:]...)
			objCfg.Actions[moveActIdx].PathD = FormatGizmoPath(newSegs)
			g.mw.timelinePanel.CommitTrackConfig(nodeID, objKey, objCfg)
			g.mw.timelinePanel.Refresh()
			g.raster.Refresh()
		}
	}
}

func syncStraightLineControls(seg *GizmoSegment) {
	seg.Ctrl1X = math.Round((seg.StartX+(seg.EndX-seg.StartX)/3.0)*10) / 10
	seg.Ctrl1Y = math.Round((seg.StartY+(seg.EndY-seg.StartY)/3.0)*10) / 10
	seg.Ctrl2X = math.Round((seg.StartX+2.0*(seg.EndX-seg.StartX)/3.0)*10) / 10
	seg.Ctrl2Y = math.Round((seg.StartY+2.0*(seg.EndY-seg.StartY)/3.0)*10) / 10
}

// Refresh redraws the gizmo overlay.
func (g *CanvasGizmoOverlay) Refresh() {
	if g == nil || g.raster == nil {
		return
	}
	g.raster.Refresh()
}

// ============================================================================
// 2D Pixel Drawing Helpers for Gizmo Primitives
// ============================================================================

func setGizmoPixel(img *image.RGBA, x, y int, c color.RGBA) {
	b := img.Bounds()
	if x < b.Min.X || x >= b.Max.X || y < b.Min.Y || y >= b.Max.Y {
		return
	}
	img.SetRGBA(x, y, c)
}

func drawGizmoLine(img *image.RGBA, x0, y0, x1, y1 int, c color.RGBA) {
	dx := int(math.Abs(float64(x1 - x0)))
	dy := -int(math.Abs(float64(y1 - y0)))
	sx := -1
	if x0 < x1 {
		sx = 1
	}
	sy := -1
	if y0 < y1 {
		sy = 1
	}
	err := dx + dy
	for {
		setGizmoPixel(img, x0, y0, c)
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

func drawGizmoThickLine(img *image.RGBA, x0, y0, x1, y1, radius int, c color.RGBA) {
	for oy := -radius; oy <= radius; oy++ {
		for ox := -radius; ox <= radius; ox++ {
			if ox*ox+oy*oy <= radius*radius {
				drawGizmoLine(img, x0+ox, y0+oy, x1+ox, y1+oy, c)
			}
		}
	}
}

func drawGizmoDashedRect(img *image.RGBA, x0, y0, x1, y1 int, c color.RGBA) {
	for x := x0; x <= x1; x++ {
		if ((x-x0)/5)%2 == 0 {
			setGizmoPixel(img, x, y0, c)
			setGizmoPixel(img, x, y1, c)
		}
	}
	for y := y0; y <= y1; y++ {
		if ((y-y0)/5)%2 == 0 {
			setGizmoPixel(img, x0, y, c)
			setGizmoPixel(img, x1, y, c)
		}
	}
}

func drawGizmoCircle(img *image.RGBA, cx, cy, r int, fill, stroke color.RGBA) {
	if r < 2 {
		r = 2
	}
	r2 := r * r
	inner2 := (r - 2) * (r - 2)
	for dy := -r; dy <= r; dy++ {
		for dx := -r; dx <= r; dx++ {
			d2 := dx*dx + dy*dy
			if d2 <= r2 {
				if d2 >= inner2 {
					setGizmoPixel(img, cx+dx, cy+dy, stroke)
				} else {
					setGizmoPixel(img, cx+dx, cy+dy, fill)
				}
			}
		}
	}
}

func drawGizmoDiamond(img *image.RGBA, cx, cy, r int, fill, stroke color.RGBA) {
	if r < 2 {
		r = 2
	}
	for dy := -r; dy <= r; dy++ {
		for dx := -r; dx <= r; dx++ {
			man := int(math.Abs(float64(dx)) + math.Abs(float64(dy)))
			if man <= r {
				if man >= r-1 {
					setGizmoPixel(img, cx+dx, cy+dy, stroke)
				} else {
					setGizmoPixel(img, cx+dx, cy+dy, fill)
				}
			}
		}
	}
}

func drawGizmoSquareHandle(img *image.RGBA, cx, cy, r int, fill, stroke color.RGBA) {
	if r < 2 {
		r = 2
	}
	for dy := -r; dy <= r; dy++ {
		for dx := -r; dx <= r; dx++ {
			if dx == -r || dx == r || dy == -r || dy == r {
				setGizmoPixel(img, cx+dx, cy+dy, stroke)
			} else {
				setGizmoPixel(img, cx+dx, cy+dy, fill)
			}
		}
	}
}

func drawGizmoCrosshair(img *image.RGBA, cx, cy, r int, c color.RGBA) {
	if r < 4 {
		r = 4
	}
	shadow := color.RGBA{R: 15, G: 23, B: 42, A: 220}
	for i := -r; i <= r; i++ {
		setGizmoPixel(img, cx+i, cy-1, shadow)
		setGizmoPixel(img, cx+i, cy+1, shadow)
		setGizmoPixel(img, cx-1, cy+i, shadow)
		setGizmoPixel(img, cx+1, cy+i, shadow)
	}
	for i := -r; i <= r; i++ {
		setGizmoPixel(img, cx+i, cy, c)
		setGizmoPixel(img, cx, cy+i, c)
	}
	drawGizmoCircle(img, cx, cy, r/2+1, color.RGBA{0, 0, 0, 0}, c)
}

func drawGizmoArrowhead(img *image.RGBA, fromX, fromY, tipX, tipY, length int, fill, casing color.RGBA) {
	dx := float64(tipX - fromX)
	dy := float64(tipY - fromY)
	dist := math.Hypot(dx, dy)
	if dist < 2 {
		return
	}
	if length < 6 {
		length = 6
	}
	ux, uy := dx/dist, dy/dist
	px, py := -uy, ux
	halfW := float64(length) * 0.55

	bx := float64(tipX) - ux*float64(length)
	by := float64(tipY) - uy*float64(length)

	lx := int(math.Round(bx + px*halfW))
	ly := int(math.Round(by + py*halfW))
	rx := int(math.Round(bx - px*halfW))
	ry := int(math.Round(by - py*halfW))

	drawGizmoThickLine(img, tipX, tipY, lx, ly, 2, casing)
	drawGizmoThickLine(img, tipX, tipY, rx, ry, 2, casing)
	drawGizmoThickLine(img, lx, ly, rx, ry, 2, casing)

	drawGizmoThickLine(img, tipX, tipY, lx, ly, 1, fill)
	drawGizmoThickLine(img, tipX, tipY, rx, ry, 1, fill)
	drawGizmoThickLine(img, lx, ly, rx, ry, 1, fill)
}
