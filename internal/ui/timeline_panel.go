package ui

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"slices"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"inkanim/internal/app"
	"inkanim/internal/ext/doctree"
)

const (
	timelineTrackLabelWidth = float32(275)
	timelineRulerHeight     = float32(30)
	timelineTrackRowHeight  = float32(34)
	timelineActionSubRowPx  = float32(24)
	timelineEdgeGripPx      = float32(14)
)

// TimelineTrackModel represents a single stage object row in the timeline panel.
type TimelineTrackModel struct {
	NodeID      string
	ObjectKey   string
	Title       string
	Depth       int
	HasChildren bool
	IsCollapsed bool
	IsSelected  bool
	Config      app.ObjectConfig
}

// TimelinePanel provides a seconds-first timeline with frame ticks, hover/scrub timestamp readouts,
// playhead scrubbing, collapsible layer/group hierarchy, double-click modals, and draggable/resizable bars.
type TimelinePanel struct {
	mw        *MainWindow
	container *fyne.Container

	PlayheadLabel    *widget.Label
	HoverLabel       *widget.Label
	SnapCheck        *widget.Check
	StripIAMSCheck   *widget.Check
	AddSVGBtn        *widget.Button
	UndoBtn          *widget.Button
	RedoBtn          *widget.Button
	ActionTypeSelect *widget.Select
	AddActionBtn     *widget.Button

	ruler          *timelineRulerWidget
	tracksVBox     *fyne.Container
	emptyNotice    *fyne.Container
	tracksStack    *fyne.Container
	lanes          []*timelineTrackLane
	collapsedNodes map[string]bool

	playheadFrame int
	totalFrames   int
	playheadSec   float64
	frameDurMs    int

	hoverActive bool
	hoverSec    float64
	hoverFrame  int

	snapToFrames bool
}

// NewTimelinePanel creates the interactive studio timeline panel.
func NewTimelinePanel(mw *MainWindow) *TimelinePanel {
	tp := &TimelinePanel{
		mw:             mw,
		snapToFrames:   true,
		collapsedNodes: make(map[string]bool),
	}

	titleLabel := widget.NewLabelWithStyle("Timeline", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	tp.PlayheadLabel = widget.NewLabelWithStyle("Frame 0 / 0 • 0.00s", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	tp.HoverLabel = widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Italic: true})

	tp.AddSVGBtn = widget.NewButtonWithIcon("Add SVG", theme.ContentAddIcon(), func() {
		if tp.mw != nil {
			tp.mw.promptAddSVG()
		}
	})
	tp.AddSVGBtn.Importance = widget.LowImportance

	tp.StripIAMSCheck = widget.NewCheck("Strip IAMS", func(checked bool) {
		if tp.mw != nil {
			tp.mw.stripIAMSOnImport = checked
			if tp.mw.stripIAMSCheck != nil && tp.mw.stripIAMSCheck.Checked != checked {
				tp.mw.stripIAMSCheck.SetChecked(checked)
			}
		}
	})

	tp.UndoBtn = widget.NewButtonWithIcon("", theme.ContentUndoIcon(), func() {
		if tp.mw != nil && tp.mw.doUndo != nil {
			tp.mw.doUndo()
		}
	})
	tp.UndoBtn.Importance = widget.LowImportance
	tp.UndoBtn.Disable()

	tp.RedoBtn = widget.NewButtonWithIcon("", theme.ContentRedoIcon(), func() {
		if tp.mw != nil && tp.mw.doRedo != nil {
			tp.mw.doRedo()
		}
	})
	tp.RedoBtn.Importance = widget.LowImportance
	tp.RedoBtn.Disable()

	tp.SnapCheck = widget.NewCheck("Snap", func(checked bool) {
		tp.snapToFrames = checked
	})
	tp.SnapCheck.Checked = true

	tp.ActionTypeSelect = widget.NewSelect([]string{
		"Move",
		"Rot",
		"Scale",
		"Fade",
		"Color",
		"Depth",
		"Preset: Float",
		"Preset: Pulse",
		"Preset: Spin",
		"Preset: Shake",
		"Preset: Pop In",
		"Preset: Fade In",
	}, nil)
	tp.ActionTypeSelect.SetSelected("Move")

	tp.AddActionBtn = widget.NewButtonWithIcon("Add Action", theme.ContentAddIcon(), func() {
		tp.AddActionToActiveNode(tp.ActionTypeSelect.Selected)
	})
	tp.AddActionBtn.Importance = widget.HighImportance

	readoutBox := container.New(&nonExpandingWidthLayout{}, container.NewHBox(
		widget.NewSeparator(),
		tp.PlayheadLabel,
		tp.HoverLabel,
	))

	headerRight := container.NewHBox(
		tp.AddSVGBtn,
		tp.StripIAMSCheck,
		tp.UndoBtn,
		tp.RedoBtn,
		widget.NewSeparator(),
		tp.SnapCheck,
		widget.NewSeparator(),
		tp.ActionTypeSelect,
		tp.AddActionBtn,
	)
	headerBar := container.New(&nonExpandingWidthLayout{}, container.NewBorder(nil, nil, titleLabel, headerRight, readoutBox))

	tp.ruler = newTimelineRulerWidget(tp)
	rulerHeaderLabel := widget.NewLabelWithStyle("Stage Object / Track", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	rulerRow := container.NewBorder(
		nil, nil,
		container.New(&fixedWidthLayout{width: timelineTrackLabelWidth}, rulerHeaderLabel),
		nil,
		tp.ruler,
	)

	tp.tracksVBox = container.NewVBox()
	tp.emptyNotice = container.NewCenter(
		widget.NewLabel("Add an SVG to your project to view and animate Layers & Groups on the timeline."),
	)
	scrollTracks := container.NewVScroll(tp.tracksVBox)
	tp.tracksStack = container.NewStack(scrollTracks, tp.emptyNotice)

	tp.container = container.NewBorder(
		container.NewVBox(
			headerBar,
			widget.NewSeparator(),
			rulerRow,
			widget.NewSeparator(),
		),
		nil, nil, nil,
		tp.tracksStack,
	)

	return tp
}

// Container returns the root Fyne canvas object for the timeline panel.
func (tp *TimelinePanel) Container() *fyne.Container {
	return tp.container
}

// TotalDurationSec returns the active project duration in seconds (minimum 0.1s).
func (tp *TimelinePanel) TotalDurationSec() float64 {
	if tp.mw != nil && tp.mw.session != nil && tp.mw.session.TotalDurationMs > 0 {
		return float64(tp.mw.session.TotalDurationMs) / 1000.0
	}
	return 1.0
}

// FPS returns the active project frame rate (minimum 1).
func (tp *TimelinePanel) FPS() int {
	if tp.mw != nil && tp.mw.session != nil && tp.mw.session.FPS > 0 {
		return tp.mw.session.FPS
	}
	return 10
}

// FrameCount returns the number of timeline frames in the current session.
func (tp *TimelinePanel) FrameCount() int {
	if tp.mw != nil && tp.mw.session != nil {
		if n := len(tp.mw.session.RenderedFrames); n > 0 {
			if tp.mw.session.ExportOptions.PingPong && n > 1 {
				return n*2 - 2
			}
			return n
		}
		if n := len(tp.mw.session.Frames); n > 0 {
			if tp.mw.session.ExportOptions.PingPong && n > 1 {
				return n*2 - 2
			}
			return n
		}
	}
	return max(int(math.Round(tp.TotalDurationSec()*float64(tp.FPS()))), 1)
}

// SnapTime rounds a timestamp in seconds to the nearest frame tick if Snap to Frames is active.
func (tp *TimelinePanel) SnapTime(sec float64) float64 {
	dur := tp.TotalDurationSec()
	if sec < 0 {
		sec = 0
	}
	if sec > dur {
		sec = dur
	}
	if tp.snapToFrames {
		fps := float64(tp.FPS())
		if fps > 0 {
			sec = math.Round(sec*fps) / fps
		}
	}
	return math.Round(sec*1000.0) / 1000.0
}

// TimeAndFrameAtX converts a horizontal pixel coordinate inside a track/ruler of width w
// into (timeSec, 0-based frameIdx, totalFrames).
func (tp *TimelinePanel) TimeAndFrameAtX(x, width float32) (float64, int, int) {
	dur := tp.TotalDurationSec()
	total := tp.FrameCount()
	if width <= 1 {
		return 0, 0, total
	}
	ratio := float64(x) / float64(width)
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	sec := ratio * dur
	idx := int(math.Floor(ratio*float64(total) + 1e-9))
	if idx >= total {
		idx = total - 1
	}
	if idx < 0 {
		idx = 0
	}
	return sec, idx, total
}

// UpdateHover updates the hover timestamp/frame readout and refreshes the hover cursor line.
func (tp *TimelinePanel) UpdateHover(x, width float32) {
	sec, idx, total := tp.TimeAndFrameAtX(x, width)
	tp.hoverActive = true
	tp.hoverSec = sec
	tp.hoverFrame = idx
	frameMs := int(math.Round(1000.0 / float64(tp.FPS())))
	if tp.mw != nil && tp.mw.session != nil && idx < len(tp.mw.session.RenderedFrames) {
		if d := tp.mw.session.RenderedFrames[idx].DurationMs; d > 0 {
			frameMs = d
		}
	}
	digits := len(strconv.Itoa(max(total, 1)))
	tp.HoverLabel.SetText(fmt.Sprintf("• Hover: Frame %0*d / %d @ %.2fs (%dms)", digits, idx+1, total, sec, frameMs))
	tp.refreshGuidesOnly()
}

// ClearHover clears the hover readout when the cursor leaves the timeline ruler/tracks.
func (tp *TimelinePanel) ClearHover() {
	if !tp.hoverActive {
		return
	}
	tp.hoverActive = false
	tp.HoverLabel.SetText("")
	tp.refreshGuidesOnly()
}

// ScrubAtX scrubs the preview playhead to the timestamp at horizontal coordinate x.
func (tp *TimelinePanel) ScrubAtX(x, width float32) {
	sec, idx, total := tp.TimeAndFrameAtX(x, width)
	tp.hoverActive = true
	tp.hoverSec = sec
	tp.hoverFrame = idx
	if tp.mw != nil && tp.mw.centerPanel != nil && len(tp.mw.session.RenderedFrames) > 0 {
		tp.mw.centerPanel.ScrubToTime(sec)
	} else {
		tp.UpdatePlayhead(idx, total, sec, int(math.Round(1000.0/float64(tp.FPS()))))
	}
}

// UpdatePlayhead updates the playhead position and readout label without rebuilding track widgets.
func (tp *TimelinePanel) UpdatePlayhead(frameIdx, totalFrames int, timeSec float64, frameDurMs int) {
	tp.playheadFrame = frameIdx
	tp.totalFrames = totalFrames
	tp.playheadSec = timeSec
	tp.frameDurMs = frameDurMs

	if totalFrames <= 0 {
		tp.PlayheadLabel.SetText("Frame 0 / 0 • 0.00s")
	} else {
		digits := len(strconv.Itoa(totalFrames))
		tp.PlayheadLabel.SetText(fmt.Sprintf("Frame %0*d / %d • %.2fs (%dms)", digits, frameIdx+1, totalFrames, timeSec, frameDurMs))
	}
	tp.refreshGuidesOnly()
}

func (tp *TimelinePanel) refreshGuidesOnly() {
	if tp.ruler != nil {
		tp.ruler.Refresh()
	}
	for _, lane := range tp.lanes {
		lane.Refresh()
	}
}

// CollectTracks gathers the ordered list of Stage Objects (Linked SVGs -> Layers -> Nested Groups)
// for display in the timeline track list, honoring collapsed parent tracks.
func (tp *TimelinePanel) CollectTracks() []TimelineTrackModel {
	if tp.mw == nil || tp.mw.session == nil || tp.mw.editorState == nil {
		return nil
	}
	if tp.collapsedNodes == nil {
		tp.collapsedNodes = make(map[string]bool)
	}
	state := tp.mw.editorState
	var tracks []TimelineTrackModel

	hasVisibleChild := func(n *doctree.DocNode) bool {
		for _, c := range n.Children {
			_, cCfg, cHas := tp.mw.resolveNodeObjectConfig(c)
			if tp.mw.isVisibleTreeNode(c) || (cHas && len(cCfg.Actions) > 0) {
				return true
			}
		}
		return false
	}

	var walk func(nodes []*doctree.DocNode, depth int)
	walk = func(nodes []*doctree.DocNode, depth int) {
		for _, n := range nodes {
			objKey, objCfg, hasCfg := tp.mw.resolveNodeObjectConfig(n)
			if !tp.mw.isVisibleTreeNode(n) && (!hasCfg || len(objCfg.Actions) == 0) {
				continue
			}
			if objKey == "" {
				objKey = n.ID
			}
			var prefix string
			switch {
			case n.IsLinkedFile:
				prefix = "SVG: "
			case n.IsLayer:
				prefix = "Layer: "
			case n.IsGroup:
				prefix = "Group: "
			default:
				prefix = "<" + n.Tag + "> "
			}
			isSelected := state.ActiveNode != nil && state.ActiveNode.ID == n.ID
			hasKids := hasVisibleChild(n)
			collapsed := tp.collapsedNodes[n.ID]
			tracks = append(tracks, TimelineTrackModel{
				NodeID:      n.ID,
				ObjectKey:   objKey,
				Title:       prefix + n.DisplayTitle(),
				Depth:       depth,
				HasChildren: hasKids,
				IsCollapsed: collapsed,
				IsSelected:  isSelected,
				Config:      objCfg,
			})
			if hasKids && !collapsed {
				walk(n.Children, depth+1)
			}
		}
	}
	walk(state.Roots, 0)
	return tracks
}

// SelectTrack selects the specified Stage Object node as active and refreshes the timeline.
func (tp *TimelinePanel) SelectTrack(nodeID string) {
	if tp.mw == nil || tp.mw.editorState == nil {
		return
	}
	if node, ok := tp.mw.editorState.NodeMap[nodeID]; ok {
		tp.mw.editorState.ActiveNode = node
		if tp.mw.tree != nil {
			tp.mw.tree.Select(nodeID)
		}
		if tp.mw.refreshInspectorFn != nil {
			tp.mw.refreshInspectorFn()
		}
		tp.Refresh()
	}
}

// timelineTrackHeaderBtn is a button for track rows that supports both single-click selection
// and double-click to open the Object & Stage Properties modal.
type timelineTrackHeaderBtn struct {
	widget.Button
	onDoubleTap func()
}

func newTimelineTrackHeaderBtn(label string, onTap, onDoubleTap func()) *timelineTrackHeaderBtn {
	b := &timelineTrackHeaderBtn{
		onDoubleTap: onDoubleTap,
	}
	b.Text = label
	b.OnTapped = onTap
	b.Alignment = widget.ButtonAlignLeading
	b.ExtendBaseWidget(b)
	return b
}

func (b *timelineTrackHeaderBtn) DoubleTapped(_ *fyne.PointEvent) {
	if b.onDoubleTap != nil {
		b.onDoubleTap()
	}
}

// Refresh rebuilds the track rows to reflect current session objects, actions, and selection.
func (tp *TimelinePanel) Refresh() {
	if tp == nil {
		return
	}
	tracks := tp.CollectTracks()
	tp.lanes = tp.lanes[:0]
	var rowObjs []fyne.CanvasObject

	for _, tr := range tracks {
		track := tr
		indent := strings.Repeat("  ", track.Depth)
		arrow := ""
		if track.HasChildren {
			if track.IsCollapsed {
				arrow = "▶ "
			} else {
				arrow = "▼ "
			}
		}

		btn := newTimelineTrackHeaderBtn(
			indent+arrow+track.Title,
			func() {
				tp.SelectTrack(track.NodeID)
			},
			func() {
				tp.SelectTrack(track.NodeID)
				tp.ShowTrackPropertiesModal(track.NodeID)
			},
		)
		switch {
		case track.IsSelected:
			btn.Importance = widget.HighImportance
		case len(track.Config.Actions) > 0 || track.Config.HasDisplayWindow || track.Config.OverrideStartPos:
			btn.Importance = widget.MediumImportance
		default:
			btn.Importance = widget.LowImportance
		}

		var leftAction fyne.CanvasObject
		if track.HasChildren {
			toggleLbl := "▼"
			if track.IsCollapsed {
				toggleLbl = "▶"
			}
			collapseBtn := widget.NewButton(toggleLbl, func() {
				tp.collapsedNodes[track.NodeID] = !tp.collapsedNodes[track.NodeID]
				tp.Refresh()
			})
			collapseBtn.Importance = widget.LowImportance
			leftAction = collapseBtn
		}

		propsBtn := widget.NewButtonWithIcon("", theme.SettingsIcon(), func() {
			tp.SelectTrack(track.NodeID)
			tp.ShowTrackPropertiesModal(track.NodeID)
		})
		propsBtn.Importance = widget.LowImportance

		var rightAction fyne.CanvasObject = propsBtn
		if tp.mw != nil {
			canUp, canDown := tp.mw.CanMoveTrackNode(track.NodeID)
			if canUp || canDown {
				upBtn := widget.NewButton("▲", func() {
					_ = tp.mw.MoveTrackNode(track.NodeID, -1)
				})
				upBtn.Importance = widget.LowImportance
				if !canUp {
					upBtn.Disable()
				}
				downBtn := widget.NewButton("▼", func() {
					_ = tp.mw.MoveTrackNode(track.NodeID, 1)
				})
				downBtn.Importance = widget.LowImportance
				if !canDown {
					downBtn.Disable()
				}
				rightAction = container.NewHBox(upBtn, downBtn, propsBtn)
			}
		}

		headerCell := container.NewBorder(nil, nil, leftAction, rightAction, btn)

		lane := newTimelineTrackLane(tp, track)
		tp.lanes = append(tp.lanes, lane)

		row := container.NewBorder(
			nil,
			canvas.NewLine(color.RGBA{R: 30, G: 41, B: 59, A: 180}),
			container.New(&fixedWidthLayout{width: timelineTrackLabelWidth}, headerCell),
			nil,
			lane,
		)
		rowObjs = append(rowObjs, row)
	}

	tp.tracksVBox.Objects = rowObjs
	tp.tracksVBox.Refresh()

	if len(rowObjs) == 0 {
		tp.emptyNotice.Show()
	} else {
		tp.emptyNotice.Hide()
	}

	if tp.mw != nil && tp.mw.editorState != nil && tp.mw.editorState.ActiveNode != nil {
		tp.AddActionBtn.Enable()
	} else {
		tp.AddActionBtn.Disable()
	}
	tp.SyncUndoRedo()

	if tp.ruler != nil {
		tp.ruler.Refresh()
	}
}

// SyncUndoRedo synchronizes the timeline toolbar's Undo/Redo button states with the active EditorState.
func (tp *TimelinePanel) SyncUndoRedo() {
	if tp == nil || tp.UndoBtn == nil || tp.RedoBtn == nil {
		return
	}
	if tp.mw == nil || tp.mw.editorState == nil {
		tp.UndoBtn.Disable()
		tp.RedoBtn.Disable()
		return
	}
	if tp.mw.editorState.CanUndo() {
		tp.UndoBtn.Enable()
	} else {
		tp.UndoBtn.Disable()
	}
	if tp.mw.editorState.CanRedo() {
		tp.RedoBtn.Enable()
	} else {
		tp.RedoBtn.Disable()
	}
}

// AddActionToActiveNode appends a new time-based .inka ActionConfig (or preset) to the currently selected Stage Object.
func (tp *TimelinePanel) AddActionToActiveNode(actionType string) {
	if tp.mw == nil || tp.mw.session == nil || tp.mw.editorState == nil || tp.mw.editorState.ActiveNode == nil {
		return
	}
	node := tp.mw.editorState.ActiveNode
	if tp.mw.session.Objects == nil {
		tp.mw.session.Objects = make(map[string]app.ObjectConfig)
	}
	objKey, objCfg, _ := tp.mw.resolveNodeObjectConfig(node)
	if objKey == "" {
		objKey = node.ID
	}
	objCfg.SourceID = node.SourceID
	objCfg.FallbackXMLID = node.ID

	durSec := tp.TotalDurationSec()
	nextActID := func(offset int) string {
		n := len(objCfg.Actions) + 1 + offset
		cand := fmt.Sprintf("act_%d", n)
		for slices.ContainsFunc(objCfg.Actions, func(a app.ActionConfig) bool { return a.ID == cand }) {
			n++
			cand = fmt.Sprintf("act_%d", n)
		}
		return cand
	}

	cx, cy := 64.0, 64.0
	if tp.mw.session.Document != nil {
		r := tp.mw.session.Document.GetElementRect(node.ID)
		if r.Width > 0 || r.Height > 0 {
			cx = r.X + r.Width/2.0
			cy = r.Y + r.Height/2.0
		}
	}

	actType := strings.ToLower(strings.TrimSpace(actionType))
	if actType == "" {
		actType = "move"
	}

	var newActs []app.ActionConfig
	switch actType {
	case "preset: float":
		newActs = append(newActs, app.ActionConfig{
			ID:       nextActID(0),
			Type:     "move",
			StartSec: 0.0,
			EndSec:   durSec,
			Ease:     "in-out",
			Repeat:   1,
			PingPong: true,
			PathD:    fmt.Sprintf("M %.1f %.1f L %.1f %.1f", cx, cy, cx, cy-20.0),
		})
	case "preset: pulse":
		newActs = append(newActs, app.ActionConfig{
			ID:         nextActID(0),
			Type:       "scale",
			StartSec:   0.0,
			EndSec:     durSec,
			Ease:       "in-out",
			Repeat:     1,
			PingPong:   true,
			ScaleFromX: 1.0,
			ScaleFromY: 1.0,
			ScaleToX:   1.25,
			ScaleToY:   1.25,
		})
	case "preset: spin":
		newActs = append(newActs, app.ActionConfig{
			ID:            nextActID(0),
			Type:          "rot",
			StartSec:      0.0,
			EndSec:        durSec,
			Ease:          "linear",
			Repeat:        1,
			RotationAngle: 360.0,
		})
	case "preset: shake":
		newActs = append(newActs, app.ActionConfig{
			ID:            nextActID(0),
			Type:          "rot",
			StartSec:      0.0,
			EndSec:        durSec,
			Ease:          "in-out",
			Repeat:        max(int(math.Round(durSec*4)), 2),
			PingPong:      true,
			RotationAngle: 8.0,
		})
	case "preset: pop in":
		popEnd := min(durSec, max(0.35, durSec*0.4))
		newActs = append(newActs,
			app.ActionConfig{
				ID:         nextActID(0),
				Type:       "scale",
				StartSec:   0.0,
				EndSec:     popEnd,
				Ease:       "out",
				Repeat:     1,
				ScaleFromX: 0.2,
				ScaleFromY: 0.2,
				ScaleToX:   1.0,
				ScaleToY:   1.0,
			},
			app.ActionConfig{
				ID:          nextActID(1),
				Type:        "fade",
				StartSec:    0.0,
				EndSec:      popEnd,
				Ease:        "out",
				Repeat:      1,
				OpacityFrom: 0.0,
				OpacityTo:   1.0,
				HasOpacity:  true,
			},
		)
	case "preset: fade in":
		newActs = append(newActs, app.ActionConfig{
			ID:          nextActID(0),
			Type:        "fade",
			StartSec:    0.0,
			EndSec:      durSec,
			Ease:        "in-out",
			Repeat:      1,
			OpacityFrom: 0.0,
			OpacityTo:   1.0,
			HasOpacity:  true,
		})
	default:
		act := app.ActionConfig{
			ID:       nextActID(0),
			Type:     actType,
			StartSec: 0.0,
			EndSec:   durSec,
			Ease:     "in-out",
			Repeat:   1,
		}
		switch actType {
		case "move":
			act.PathD = fmt.Sprintf("M %.1f %.1f L %.1f %.1f", cx, cy, cx+32.0, cy)
		case "rot":
			act.RotationAngle = 45.0
		case "scale":
			act.ScaleFromX = 1.0
			act.ScaleFromY = 1.0
			act.ScaleToX = 1.25
			act.ScaleToY = 1.25
		case "fade":
			act.OpacityFrom = 0.0
			act.OpacityTo = 1.0
			act.HasOpacity = true
		case "color":
			act.ColorTarget = "#38BDF8"
		case "depth":
			act.DepthOffset = 5
		}
		newActs = append(newActs, act)
	}

	objCfg.Actions = append(objCfg.Actions, newActs...)
	tp.mw.session.Objects[objKey] = objCfg
	if len(newActs) > 0 {
		tp.mw.selectedActionID = newActs[len(newActs)-1].ID
	}
	tp.mw.RecordStudioChange("Add timeline action: " + actionType)
	if tp.mw.refreshTreeFn != nil {
		tp.mw.refreshTreeFn()
	}
	if tp.mw.refreshInspectorFn != nil {
		tp.mw.refreshInspectorFn()
	}
	tp.Refresh()
	tp.mw.schedulePreviewUpdate()
}

// ShowTrackPropertiesModal opens a modal dialog for renaming a Layer/Group and configuring
// Stage Placement, Display Timing, Linked SVG actions, or deleting an empty group.
func (tp *TimelinePanel) ShowTrackPropertiesModal(nodeID string) {
	if tp.mw == nil || tp.mw.editorState == nil || tp.mw.session == nil {
		return
	}
	node := tp.mw.editorState.NodeMap[nodeID]
	if node == nil {
		return
	}
	if tp.mw.session.Objects == nil {
		tp.mw.session.Objects = make(map[string]app.ObjectConfig)
	}
	objKey, objCfg, _ := tp.mw.resolveNodeObjectConfig(node)
	if objKey == "" {
		objKey = node.ID
	}

	var kind string
	switch {
	case node.IsLinkedFile:
		kind = "Linked SVG"
	case node.IsLayer:
		kind = "Layer"
	case node.IsGroup:
		kind = "Group"
	default:
		kind = "Object"
	}

	nameEntry := widget.NewEntry()
	nameEntry.SetText(node.DisplayTitle())
	if node.IsLinkedFile {
		nameEntry.Disable()
	}

	startXEntry := widget.NewEntry()
	startXEntry.SetText(fmt.Sprintf("%g", objCfg.StartX))
	startYEntry := widget.NewEntry()
	startYEntry.SetText(fmt.Sprintf("%g", objCfg.StartY))

	updateStartPosEnables := func(enabled bool) {
		if enabled {
			startXEntry.Enable()
			startYEntry.Enable()
		} else {
			startXEntry.Disable()
			startYEntry.Disable()
		}
	}
	updateStartPosEnables(objCfg.OverrideStartPos)

	startPosCheck := widget.NewCheck("Custom Start Position (override SVG page position)", func(checked bool) {
		if checked && startXEntry.Text == "0" && startYEntry.Text == "0" && tp.mw.session.Document != nil {
			r := tp.mw.session.Document.GetElementRect(node.ID)
			if r.Width > 0 || r.Height > 0 {
				startXEntry.SetText(fmt.Sprintf("%.1f", r.X+r.Width/2.0))
				startYEntry.SetText(fmt.Sprintf("%.1f", r.Y+r.Height/2.0))
			}
		}
		updateStartPosEnables(checked)
	})
	startPosCheck.Checked = objCfg.OverrideStartPos

	totalDurSec := tp.TotalDurationSec()
	endSecInit := objCfg.EndTimeSec
	if endSecInit <= 0 {
		endSecInit = totalDurSec
	}

	appearEntry := widget.NewEntry()
	appearEntry.SetText(fmt.Sprintf("%g", objCfg.StartTimeSec))
	hideEntry := widget.NewEntry()
	hideEntry.SetText(fmt.Sprintf("%g", endSecInit))

	updateDisplayWinEnables := func(enabled bool) {
		if enabled {
			appearEntry.Enable()
			hideEntry.Enable()
		} else {
			appearEntry.Disable()
			hideEntry.Disable()
		}
	}
	updateDisplayWinEnables(objCfg.HasDisplayWindow)

	displayWinCheck := widget.NewCheck("Custom Display Time Window (seconds)", func(checked bool) {
		updateDisplayWinEnables(checked)
	})
	displayWinCheck.Checked = objCfg.HasDisplayWindow

	zOffsetEntry := widget.NewEntry()
	zOffsetEntry.SetText(strconv.Itoa(objCfg.ZOffset))

	pivotSelect := widget.NewSelect([]string{"center", "top", "bottom", "left", "right"}, nil)
	if objCfg.PivotType != "" {
		pivotSelect.SetSelected(objCfg.PivotType)
	} else {
		pivotSelect.SetSelected("center")
	}

	var d *dialog.CustomDialog

	var items []fyne.CanvasObject
	if !node.IsLinkedFile {
		items = append(items,
			widget.NewLabelWithStyle("Rename "+kind, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			nameEntry,
			widget.NewSeparator(),
		)
	} else if node.SourceID != "" {
		stripBtn := widget.NewButtonWithIcon("Strip Legacy IAMS", theme.ContentCutIcon(), func() {
			if d != nil {
				d.Hide()
			}
			if err := tp.mw.StripIAMSFromLinkedSVG(node.SourceID); err != nil {
				dialog.ShowError(err, tp.mw.window)
			}
		})
		stripBtn.Importance = widget.LowImportance

		unlinkBtn := widget.NewButtonWithIcon("Unlink SVG", theme.DeleteIcon(), func() {
			if d != nil {
				d.Hide()
			}
			if err := tp.mw.RemoveLinkedSVG(node.SourceID); err != nil {
				dialog.ShowError(err, tp.mw.window)
			}
		})
		unlinkBtn.Importance = widget.DangerImportance

		items = append(items,
			widget.NewLabelWithStyle("Linked Artwork File: "+node.DisplayTitle(), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			container.NewHBox(stripBtn, unlinkBtn),
			widget.NewSeparator(),
		)
	}

	items = append(items,
		widget.NewLabelWithStyle("Stage Placement & Display Timing", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		startPosCheck,
		container.NewGridWithColumns(4,
			widget.NewLabel("Start X:"), startXEntry,
			widget.NewLabel("Start Y:"), startYEntry,
		),
		displayWinCheck,
		container.NewGridWithColumns(4,
			widget.NewLabel("Appear (s):"), appearEntry,
			widget.NewLabel("Hide (s):"), hideEntry,
		),
		container.NewGridWithColumns(4,
			widget.NewLabel("Z-Offset:"), zOffsetEntry,
			widget.NewLabel("Pivot:"), pivotSelect,
		),
	)

	canDelete, deleteReason := tp.mw.editorState.CanDeleteElement(node.ID)
	var deleteBtn *widget.Button
	if !node.IsLinkedFile {
		deleteBtn = widget.NewButtonWithIcon("Delete "+kind, theme.DeleteIcon(), func() {
			if d != nil {
				d.Hide()
			}
			tp.SelectTrack(node.ID)
			if tp.mw.doDeleteActiveElement != nil {
				tp.mw.doDeleteActiveElement()
			}
			tp.Refresh()
		})
		deleteBtn.Importance = widget.DangerImportance
		if !canDelete {
			deleteBtn.Disable()
			if deleteReason != "" {
				items = append(items,
					widget.NewSeparator(),
					widget.NewLabelWithStyle(deleteReason, fyne.TextAlignLeading, fyne.TextStyle{Italic: true}),
				)
			}
		}
	}

	saveBtn := widget.NewButtonWithIcon("Save", theme.ConfirmIcon(), func() {
		objCfg.OverrideStartPos = startPosCheck.Checked
		if v, err := strconv.ParseFloat(strings.TrimSpace(startXEntry.Text), 64); err == nil {
			objCfg.StartX = v
		}
		if v, err := strconv.ParseFloat(strings.TrimSpace(startYEntry.Text), 64); err == nil {
			objCfg.StartY = v
		}
		objCfg.HasDisplayWindow = displayWinCheck.Checked
		if v, err := strconv.ParseFloat(strings.TrimSpace(appearEntry.Text), 64); err == nil && v >= 0 {
			objCfg.StartTimeSec = v
		}
		if v, err := strconv.ParseFloat(strings.TrimSpace(hideEntry.Text), 64); err == nil && v >= 0 {
			objCfg.EndTimeSec = v
		}
		if v, err := strconv.Atoi(strings.TrimSpace(zOffsetEntry.Text)); err == nil {
			objCfg.ZOffset = v
		}
		objCfg.PivotType = pivotSelect.Selected
		objCfg.SourceID = node.SourceID
		objCfg.FallbackXMLID = node.ID
		tp.mw.session.Objects[objKey] = objCfg

		renamed := false
		if !node.IsLinkedFile {
			newName := strings.TrimSpace(nameEntry.Text)
			if newName != "" && newName != node.DisplayTitle() {
				_ = tp.mw.RenameTrackNode(node.ID, newName)
				renamed = true
			}
		}
		if !renamed {
			tp.mw.RecordStudioChange("Update track properties: " + node.DisplayTitle())
		}

		if tp.mw.refreshTreeFn != nil {
			tp.mw.refreshTreeFn()
		}
		if tp.mw.refreshInspectorFn != nil {
			tp.mw.refreshInspectorFn()
		}
		tp.Refresh()
		tp.mw.schedulePreviewUpdate()
		if d != nil {
			d.Hide()
		}
	})
	saveBtn.Importance = widget.HighImportance

	cancelBtn := widget.NewButton("Cancel", func() {
		if d != nil {
			d.Hide()
		}
	})

	btnRow := container.NewHBox()
	if deleteBtn != nil {
		btnRow.Add(deleteBtn)
	}
	btnRow.Add(layout.NewSpacer())
	btnRow.Add(cancelBtn)
	btnRow.Add(saveBtn)

	items = append(items, widget.NewSeparator(), btnRow)
	content := container.NewVBox(items...)

	d = dialog.NewCustomWithoutButtons(fmt.Sprintf("%s Properties — %s", kind, node.DisplayTitle()), content, tp.mw.window)
	d.Resize(fyne.NewSize(480, 380))
	d.Show()
}

// ShowActionEditModal opens a modal dialog for editing or deleting a specific timeline ActionConfig.
func (tp *TimelinePanel) ShowActionEditModal(nodeID string, actionIdx int) {
	if tp.mw == nil || tp.mw.editorState == nil || tp.mw.session == nil {
		return
	}
	node := tp.mw.editorState.NodeMap[nodeID]
	if node == nil {
		return
	}
	objKey, objCfg, hasCfg := tp.mw.resolveNodeObjectConfig(node)
	if !hasCfg || actionIdx < 0 || actionIdx >= len(objCfg.Actions) {
		return
	}
	act := objCfg.Actions[actionIdx]
	tp.mw.selectedActionID = act.ID

	typeSelect := widget.NewSelect([]string{"move", "rot", "scale", "fade", "color", "depth"}, nil)
	actType := strings.ToLower(act.Type)
	if actType == "" {
		actType = "move"
	}
	typeSelect.SetSelected(actType)

	startEntry := widget.NewEntry()
	startEntry.SetText(fmt.Sprintf("%g", act.StartSec))
	endEntry := widget.NewEntry()
	endSecInit := act.EndSec
	if endSecInit <= 0 {
		endSecInit = tp.TotalDurationSec()
	}
	endEntry.SetText(fmt.Sprintf("%g", endSecInit))

	easeSelect := widget.NewSelect([]string{"linear", "in", "out", "in-out"}, nil)
	if act.Ease != "" {
		easeSelect.SetSelected(act.Ease)
	} else {
		easeSelect.SetSelected("in-out")
	}

	repeatEntry := widget.NewEntry()
	repInit := act.Repeat
	if repInit <= 0 {
		repInit = 1
	}
	repeatEntry.SetText(strconv.Itoa(repInit))

	pingPongCheck := widget.NewCheck("Ping-Pong", nil)
	pingPongCheck.Checked = act.PingPong

	reverseCheck := widget.NewCheck("Reverse", nil)
	reverseCheck.Checked = act.Reverse

	pivotSelect := widget.NewSelect([]string{"center", "top", "bottom", "left", "right"}, nil)
	if act.PivotType != "" {
		pivotSelect.SetSelected(act.PivotType)
	} else {
		pivotSelect.SetSelected("center")
	}

	// Type-specific fields
	pathEntry := widget.NewEntry()
	pathEntry.SetText(act.PathD)
	orientCheck := widget.NewCheck("Orient to Path", nil)
	orientCheck.Checked = act.OrientPath

	degEntry := widget.NewEntry()
	degEntry.SetText(fmt.Sprintf("%g", act.RotationAngle))
	rotDirSelect := widget.NewSelect([]string{"cw", "ccw"}, nil)
	if act.RotationDir != "" {
		rotDirSelect.SetSelected(act.RotationDir)
	} else {
		rotDirSelect.SetSelected("cw")
	}

	scaleFromXEntry := widget.NewEntry()
	sfX := act.ScaleFromX
	if sfX == 0 {
		sfX = 1.0
	}
	scaleFromXEntry.SetText(fmt.Sprintf("%g", sfX))

	scaleFromYEntry := widget.NewEntry()
	sfY := act.ScaleFromY
	if sfY == 0 {
		sfY = 1.0
	}
	scaleFromYEntry.SetText(fmt.Sprintf("%g", sfY))

	scaleToXEntry := widget.NewEntry()
	stX := act.ScaleToX
	if stX == 0 {
		stX = 1.25
	}
	scaleToXEntry.SetText(fmt.Sprintf("%g", stX))

	scaleToYEntry := widget.NewEntry()
	stY := act.ScaleToY
	if stY == 0 {
		stY = 1.25
	}
	scaleToYEntry.SetText(fmt.Sprintf("%g", stY))

	opFromEntry := widget.NewEntry()
	opFromEntry.SetText(fmt.Sprintf("%g", act.OpacityFrom))
	opToEntry := widget.NewEntry()
	opToEntry.SetText(fmt.Sprintf("%g", act.OpacityTo))

	colorTargetEntry := widget.NewEntry()
	colorTargetEntry.SetText(act.ColorTarget)
	colorAngleEntry := widget.NewEntry()
	colorAngleEntry.SetText(fmt.Sprintf("%g", act.ColorAngle))

	depthEntry := widget.NewEntry()
	depthEntry.SetText(strconv.Itoa(act.DepthOffset))

	specificBox := container.NewVBox()
	rebuildSpecific := func(t string) {
		switch strings.ToLower(t) {
		case "move":
			specificBox.Objects = []fyne.CanvasObject{
				widget.NewLabel("Motion Path (SVG d):"),
				pathEntry,
				orientCheck,
			}
		case "rot":
			specificBox.Objects = []fyne.CanvasObject{
				container.NewGridWithColumns(4,
					widget.NewLabel("Degrees:"), degEntry,
					widget.NewLabel("Direction:"), rotDirSelect,
				),
			}
		case "scale":
			specificBox.Objects = []fyne.CanvasObject{
				container.NewGridWithColumns(4,
					widget.NewLabel("From X:"), scaleFromXEntry,
					widget.NewLabel("From Y:"), scaleFromYEntry,
				),
				container.NewGridWithColumns(4,
					widget.NewLabel("To X:"), scaleToXEntry,
					widget.NewLabel("To Y:"), scaleToYEntry,
				),
			}
		case "fade":
			specificBox.Objects = []fyne.CanvasObject{
				container.NewGridWithColumns(4,
					widget.NewLabel("Alpha From (0-1):"), opFromEntry,
					widget.NewLabel("Alpha To (0-1):"), opToEntry,
				),
			}
		case "color":
			specificBox.Objects = []fyne.CanvasObject{
				container.NewGridWithColumns(4,
					widget.NewLabel("Target / Color:"), colorTargetEntry,
					widget.NewLabel("Sweep Angle:"), colorAngleEntry,
				),
			}
		case "depth":
			specificBox.Objects = []fyne.CanvasObject{
				container.NewGridWithColumns(2,
					widget.NewLabel("Z-Depth Offset:"), depthEntry,
				),
			}
		}
		specificBox.Refresh()
	}
	rebuildSpecific(actType)
	typeSelect.OnChanged = rebuildSpecific

	var d *dialog.CustomDialog

	deleteBtn := widget.NewButtonWithIcon("Delete Action", theme.DeleteIcon(), func() {
		if d != nil {
			d.Hide()
		}
		if tp.mw.selectedActionID == act.ID {
			tp.mw.selectedActionID = ""
		}
		objCfg.Actions = append(objCfg.Actions[:actionIdx], objCfg.Actions[actionIdx+1:]...)
		tp.CommitTrackConfig(node.ID, objKey, objCfg)
		tp.Refresh()
	})
	deleteBtn.Importance = widget.DangerImportance

	saveBtn := widget.NewButtonWithIcon("Save Action", theme.ConfirmIcon(), func() {
		act.Type = strings.ToLower(typeSelect.Selected)
		if v, err := strconv.ParseFloat(strings.TrimSpace(startEntry.Text), 64); err == nil && v >= 0 {
			act.StartSec = v
			act.IsAll = false
		}
		if v, err := strconv.ParseFloat(strings.TrimSpace(endEntry.Text), 64); err == nil && v > 0 {
			act.EndSec = v
			act.IsAll = false
		}
		act.Ease = easeSelect.Selected
		if v, err := strconv.Atoi(strings.TrimSpace(repeatEntry.Text)); err == nil && v > 0 {
			act.Repeat = v
		}
		act.PingPong = pingPongCheck.Checked
		act.Reverse = reverseCheck.Checked
		act.PivotType = pivotSelect.Selected

		switch act.Type {
		case "move":
			act.PathD = strings.TrimSpace(pathEntry.Text)
			act.OrientPath = orientCheck.Checked
		case "rot":
			if v, err := strconv.ParseFloat(strings.TrimSpace(degEntry.Text), 64); err == nil {
				act.RotationAngle = v
			}
			act.RotationDir = rotDirSelect.Selected
		case "scale":
			if v, err := strconv.ParseFloat(strings.TrimSpace(scaleFromXEntry.Text), 64); err == nil {
				act.ScaleFromX = v
			}
			if v, err := strconv.ParseFloat(strings.TrimSpace(scaleFromYEntry.Text), 64); err == nil {
				act.ScaleFromY = v
			}
			if v, err := strconv.ParseFloat(strings.TrimSpace(scaleToXEntry.Text), 64); err == nil {
				act.ScaleToX = v
			}
			if v, err := strconv.ParseFloat(strings.TrimSpace(scaleToYEntry.Text), 64); err == nil {
				act.ScaleToY = v
			}
		case "fade":
			if v, err := strconv.ParseFloat(strings.TrimSpace(opFromEntry.Text), 64); err == nil {
				act.OpacityFrom = v
				act.HasOpacity = true
			}
			if v, err := strconv.ParseFloat(strings.TrimSpace(opToEntry.Text), 64); err == nil {
				act.OpacityTo = v
				act.HasOpacity = true
			}
		case "color":
			act.ColorTarget = strings.TrimSpace(colorTargetEntry.Text)
			if v, err := strconv.ParseFloat(strings.TrimSpace(colorAngleEntry.Text), 64); err == nil {
				act.ColorAngle = v
				act.HasColorAngle = true
			}
		case "depth":
			if v, err := strconv.Atoi(strings.TrimSpace(depthEntry.Text)); err == nil {
				act.DepthOffset = v
			}
		}

		objCfg.Actions[actionIdx] = act
		tp.CommitTrackConfig(node.ID, objKey, objCfg)
		tp.Refresh()
		if d != nil {
			d.Hide()
		}
	})
	saveBtn.Importance = widget.HighImportance

	cancelBtn := widget.NewButton("Cancel", func() {
		if d != nil {
			d.Hide()
		}
	})

	actionBottomRow := container.NewHBox(deleteBtn)
	if len(objCfg.Actions) > 1 {
		moveUpBtn := widget.NewButton("▲ Up", func() {
			if d != nil {
				d.Hide()
			}
			tp.MoveAction(node.ID, actionIdx, -1)
		})
		moveUpBtn.Importance = widget.LowImportance
		if actionIdx <= 0 {
			moveUpBtn.Disable()
		}
		moveDownBtn := widget.NewButton("▼ Down", func() {
			if d != nil {
				d.Hide()
			}
			tp.MoveAction(node.ID, actionIdx, 1)
		})
		moveDownBtn.Importance = widget.LowImportance
		if actionIdx >= len(objCfg.Actions)-1 {
			moveDownBtn.Disable()
		}
		actionBottomRow.Add(moveUpBtn)
		actionBottomRow.Add(moveDownBtn)
	}
	actionBottomRow.Add(layout.NewSpacer())
	actionBottomRow.Add(cancelBtn)
	actionBottomRow.Add(saveBtn)

	content := container.NewVBox(
		container.NewGridWithColumns(4,
			widget.NewLabel("Action Type:"), typeSelect,
			widget.NewLabel("Ease:"), easeSelect,
		),
		container.NewGridWithColumns(4,
			widget.NewLabel("Start (s):"), startEntry,
			widget.NewLabel("End (s):"), endEntry,
		),
		container.NewGridWithColumns(4,
			widget.NewLabel("Repeat:"), repeatEntry,
			widget.NewLabel("Pivot:"), pivotSelect,
		),
		container.NewHBox(pingPongCheck, reverseCheck),
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Action Parameters", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		specificBox,
		widget.NewSeparator(),
		actionBottomRow,
	)

	d = dialog.NewCustomWithoutButtons(fmt.Sprintf("Edit Action (%s) — %s", strings.ToUpper(act.Type), node.DisplayTitle()), content, tp.mw.window)
	d.Resize(fyne.NewSize(480, 360))
	d.Show()
}

// DeleteSelectedAction removes the currently selected timeline action (if any) and returns true.
func (tp *TimelinePanel) DeleteSelectedAction() bool {
	if tp == nil || tp.mw == nil || tp.mw.session == nil || tp.mw.editorState == nil || tp.mw.selectedActionID == "" {
		return false
	}
	targetID := tp.mw.selectedActionID
	for objKey, objCfg := range tp.mw.session.Objects {
		for idx, act := range objCfg.Actions {
			if act.ID == targetID {
				objCfg.Actions = append(objCfg.Actions[:idx], objCfg.Actions[idx+1:]...)
				tp.mw.selectedActionID = ""
				nodeID := objCfg.FallbackXMLID
				if nodeID == "" {
					nodeID = objKey
				}
				tp.CommitTrackConfig(nodeID, objKey, objCfg)
				tp.Refresh()
				return true
			}
		}
	}
	return false
}

// MoveAction moves an action at actionIdx by delta (-1 for up, +1 for down) within the track's Actions slice.
func (tp *TimelinePanel) MoveAction(nodeID string, actionIdx, delta int) bool {
	if tp.mw == nil || tp.mw.editorState == nil || tp.mw.session == nil {
		return false
	}
	node := tp.mw.editorState.NodeMap[nodeID]
	if node == nil {
		return false
	}
	objKey, objCfg, hasCfg := tp.mw.resolveNodeObjectConfig(node)
	targetIdx := actionIdx + delta
	if !hasCfg || actionIdx < 0 || actionIdx >= len(objCfg.Actions) || targetIdx < 0 || targetIdx >= len(objCfg.Actions) {
		return false
	}
	objCfg.Actions[actionIdx], objCfg.Actions[targetIdx] = objCfg.Actions[targetIdx], objCfg.Actions[actionIdx]
	tp.CommitTrackConfig(node.ID, objKey, objCfg)
	tp.Refresh()
	return true
}

// CommitTrackConfig persists an updated ObjectConfig from a timeline drag/resize or modal operation.
func (tp *TimelinePanel) CommitTrackConfig(nodeID, objKey string, cfg app.ObjectConfig) {
	if tp.mw == nil || tp.mw.session == nil {
		return
	}
	if tp.mw.session.Objects == nil {
		tp.mw.session.Objects = make(map[string]app.ObjectConfig)
	}
	if objKey == "" {
		objKey = nodeID
	}
	cfg.FallbackXMLID = nodeID
	tp.mw.session.Objects[objKey] = cfg
	tp.mw.RecordStudioChange("Update timeline track")
	if tp.mw.refreshTreeFn != nil {
		tp.mw.refreshTreeFn()
	}
	if tp.mw.refreshInspectorFn != nil {
		tp.mw.refreshInspectorFn()
	}
	tp.mw.schedulePreviewUpdate()
}

// ============================================================================
// Timeline Ruler Widget (Seconds + Frame Ticks + Playhead Scrubber)
// ============================================================================

type timelineRulerWidget struct {
	widget.BaseWidget
	tp     *TimelinePanel
	raster *canvas.Raster
}

func newTimelineRulerWidget(tp *TimelinePanel) *timelineRulerWidget {
	r := &timelineRulerWidget{tp: tp}
	r.raster = canvas.NewRaster(r.draw)
	r.ExtendBaseWidget(r)
	return r
}

func (r *timelineRulerWidget) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(r.raster)
}

func (r *timelineRulerWidget) MinSize() fyne.Size {
	return fyne.NewSize(240, timelineRulerHeight)
}

func (r *timelineRulerWidget) Tapped(ev *fyne.PointEvent) {
	r.tp.ScrubAtX(ev.Position.X, r.Size().Width)
}

func (r *timelineRulerWidget) Dragged(ev *fyne.DragEvent) {
	r.tp.ScrubAtX(ev.Position.X, r.Size().Width)
}

func (r *timelineRulerWidget) DragEnd() {}

func (r *timelineRulerWidget) MouseIn(ev *desktop.MouseEvent) {
	r.tp.UpdateHover(ev.Position.X, r.Size().Width)
}

func (r *timelineRulerWidget) MouseMoved(ev *desktop.MouseEvent) {
	r.tp.UpdateHover(ev.Position.X, r.Size().Width)
}

func (r *timelineRulerWidget) MouseOut() {
	r.tp.ClearHover()
}

func (r *timelineRulerWidget) draw(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, max(w, 1), max(h, 1)))
	if w <= 1 || h <= 1 {
		return img
	}

	bg := color.RGBA{R: 15, G: 17, B: 21, A: 255}
	fillRectRGBA(img, 0, 0, w, h, bg)

	durSec := r.tp.TotalDurationSec()
	totalFrames := r.tp.FrameCount()

	// 1. Draw Frame Ticks along the bottom edge of the ruler
	frameTickColor := color.RGBA{R: 51, G: 65, B: 85, A: 220}
	for i := range totalFrames + 1 {
		x := int(math.Round(float64(i) / float64(max(totalFrames, 1)) * float64(w-1)))
		drawVLineRGBA(img, x, h-8, h-1, frameTickColor)
	}

	// 2. Draw Second Ticks (major & minor) along the top/full height of the ruler
	majorStep := chooseSecondTickStep(durSec)
	minorStep := majorStep / 2.0
	minorColor := color.RGBA{R: 100, G: 116, B: 139, A: 200}
	majorColor := color.RGBA{R: 203, G: 213, B: 225, A: 255}

	if minorStep > 0 {
		steps := int(math.Ceil(durSec/minorStep)) + 1
		for i := range steps {
			t := float64(i) * minorStep
			if t > durSec+1e-6 {
				break
			}
			x := int(math.Round((t / durSec) * float64(w-1)))
			if i%2 == 0 {
				drawVLineRGBA(img, x, 2, h-1, majorColor)
				lbl := fmt.Sprintf("%.2gs", t)
				lblW := len(lbl) * 6
				lblX := x + 3
				if lblX+lblW >= w {
					lblX = max(2, x-lblW-2)
				}
				drawTinyLabel(img, lblX, 4, lbl, majorColor)
			} else {
				drawVLineRGBA(img, x, h/2, h-1, minorColor)
			}
		}
	}

	// 3. Hover guide line
	if r.tp.hoverActive && durSec > 0 {
		hx := int(math.Round((r.tp.hoverSec / durSec) * float64(w-1)))
		drawVLineRGBA(img, hx, 0, h-1, color.RGBA{R: 148, G: 163, B: 184, A: 180})
	}

	// 4. Playhead line + cap (Electric Sky Blue #38BDF8)
	playheadRatio := 0.0
	if totalFrames > 0 {
		playheadRatio = float64(r.tp.playheadFrame) / float64(totalFrames)
	} else if durSec > 0 {
		playheadRatio = r.tp.playheadSec / durSec
	}
	px := int(math.Round(playheadRatio * float64(w-1)))
	skyBlue := color.RGBA{R: 56, G: 189, B: 248, A: 255}
	fillRectRGBA(img, px-4, 0, px+5, 6, skyBlue)
	drawVLineRGBA(img, px, 0, h-1, skyBlue)
	if px+1 < w {
		drawVLineRGBA(img, px+1, 0, h-1, skyBlue)
	}

	return img
}

func chooseSecondTickStep(durSec float64) float64 {
	switch {
	case durSec <= 0.5:
		return 0.1
	case durSec <= 1.5:
		return 0.25
	case durSec <= 4.0:
		return 0.5
	case durSec <= 10.0:
		return 1.0
	case durSec <= 25.0:
		return 2.0
	default:
		return 5.0
	}
}

// ============================================================================
// Timeline Track Lane Widget (Display Window Span + Draggable/Resizable Actions)
// ============================================================================

type timelineDragMode int

const (
	dragNone timelineDragMode = iota
	dragScrubPlayhead
	dragActionMove
	dragActionResizeStart
	dragActionResizeEnd
	dragDisplayResizeStart
	dragDisplayResizeEnd
)

type timelineTrackLane struct {
	widget.BaseWidget
	tp     *TimelinePanel
	track  TimelineTrackModel
	raster *canvas.Raster

	cursorMode     desktop.Cursor
	dragMode       timelineDragMode
	dragActionIdx  int
	dragStartX     float32
	origStartSec   float64
	origEndSec     float64
	activeDragmake bool
}

func newTimelineTrackLane(tp *TimelinePanel, track TimelineTrackModel) *timelineTrackLane {
	lane := &timelineTrackLane{
		tp:            tp,
		track:         track,
		cursorMode:    desktop.DefaultCursor,
		dragActionIdx: -1,
	}
	lane.raster = canvas.NewRaster(lane.draw)
	lane.ExtendBaseWidget(lane)
	return lane
}

func (l *timelineTrackLane) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(l.raster)
}

func (l *timelineTrackLane) Cursor() desktop.Cursor {
	if l.cursorMode != nil {
		return l.cursorMode
	}
	return desktop.DefaultCursor
}

func (l *timelineTrackLane) MinSize() fyne.Size {
	n := len(l.track.Config.Actions)
	h := timelineTrackRowHeight
	if n > 1 {
		h = float32(n)*timelineActionSubRowPx + 8
	}
	return fyne.NewSize(240, h)
}

func (l *timelineTrackLane) actionSubRowBounds(actIdx, totalActions int, totalHeight float32) (float32, float32) {
	if totalActions <= 1 {
		return 5, max(6, totalHeight-5)
	}
	availH := max(1, totalHeight-6)
	rowH := availH / float32(totalActions)
	y1 := 3 + float32(actIdx)*rowH + 1
	y2 := 3 + float32(actIdx+1)*rowH - 1
	if y2 <= y1 {
		y2 = y1 + 1
	}
	return y1, y2
}

func (l *timelineTrackLane) updateCursorAt(x, y float32) {
	sz := l.Size()
	_, mode := l.hitTestAction(x, y, sz.Width, sz.Height)
	var next desktop.Cursor
	switch mode {
	case dragActionResizeStart, dragActionResizeEnd, dragDisplayResizeStart, dragDisplayResizeEnd:
		next = desktop.HResizeCursor
	case dragActionMove:
		next = desktop.PointerCursor
	default:
		next = desktop.DefaultCursor
	}
	if l.cursorMode != next {
		l.cursorMode = next
		l.Refresh()
	}
}

func (l *timelineTrackLane) MouseIn(ev *desktop.MouseEvent) {
	l.updateCursorAt(ev.Position.X, ev.Position.Y)
	l.tp.UpdateHover(ev.Position.X, l.Size().Width)
}

func (l *timelineTrackLane) MouseMoved(ev *desktop.MouseEvent) {
	l.updateCursorAt(ev.Position.X, ev.Position.Y)
	l.tp.UpdateHover(ev.Position.X, l.Size().Width)
}

func (l *timelineTrackLane) MouseOut() {
	l.cursorMode = desktop.DefaultCursor
	l.tp.ClearHover()
}

func (l *timelineTrackLane) hitTestAction(x, y, width, height float32) (int, timelineDragMode) {
	dur := l.tp.TotalDurationSec()
	if width <= 1 || dur <= 0 {
		return -1, dragNone
	}
	totalActions := len(l.track.Config.Actions)
	for i := totalActions - 1; i >= 0; i-- {
		y1, y2 := l.actionSubRowBounds(i, totalActions, height)
		if totalActions > 1 && (y < y1-2 || y > y2+2) {
			continue
		}
		act := l.track.Config.Actions[i]
		startSec := clampFloat(act.StartSec, 0, dur)
		endSec := act.EndSec
		if endSec <= startSec {
			endSec = dur
		}
		endSec = clampFloat(endSec, startSec, dur)
		x1 := float32(startSec/dur) * width
		x2 := float32(endSec/dur) * width
		if x1 > width-18 {
			x1 = max(0, width-18)
		}
		if x2-x1 < 18 {
			x2 = min(width, x1+18)
		}
		grip := min(timelineEdgeGripPx, max(6, (x2-x1)/3))
		if x >= x1-6 && x <= x2+6 {
			if x <= x1+grip {
				return i, dragActionResizeStart
			}
			if x >= x2-grip {
				return i, dragActionResizeEnd
			}
			return i, dragActionMove
		}
	}

	if l.track.Config.HasDisplayWindow {
		startSec := clampFloat(l.track.Config.StartTimeSec, 0, dur)
		endSec := l.track.Config.EndTimeSec
		if endSec <= 0 {
			endSec = dur
		}
		endSec = clampFloat(endSec, startSec, dur)
		x1 := float32(startSec/dur) * width
		x2 := float32(endSec/dur) * width
		if math.Abs(float64(x-x1)) <= float64(timelineEdgeGripPx) {
			return -1, dragDisplayResizeStart
		}
		if math.Abs(float64(x-x2)) <= float64(timelineEdgeGripPx) {
			return -1, dragDisplayResizeEnd
		}
	}

	return -1, dragScrubPlayhead
}

func (l *timelineTrackLane) Tapped(ev *fyne.PointEvent) {
	sz := l.Size()
	w, h := sz.Width, sz.Height
	actIdx, _ := l.hitTestAction(ev.Position.X, ev.Position.Y, w, h)
	if actIdx >= 0 && actIdx < len(l.track.Config.Actions) {
		act := l.track.Config.Actions[actIdx]
		if l.tp.mw != nil {
			l.tp.mw.selectedActionID = act.ID
			l.tp.SelectTrack(l.track.NodeID)
		}
		l.Refresh()
		return
	}
	l.tp.SelectTrack(l.track.NodeID)
	l.tp.ScrubAtX(ev.Position.X, w)
}

func (l *timelineTrackLane) DoubleTapped(ev *fyne.PointEvent) {
	sz := l.Size()
	w, h := sz.Width, sz.Height
	l.tp.SelectTrack(l.track.NodeID)
	actIdx, _ := l.hitTestAction(ev.Position.X, ev.Position.Y, w, h)
	if actIdx >= 0 && actIdx < len(l.track.Config.Actions) {
		l.tp.ShowActionEditModal(l.track.NodeID, actIdx)
		return
	}
	l.tp.ShowTrackPropertiesModal(l.track.NodeID)
}

func (l *timelineTrackLane) Dragged(ev *fyne.DragEvent) {
	sz := l.Size()
	w, h := sz.Width, sz.Height
	dur := l.tp.TotalDurationSec()
	if w <= 1 || dur <= 0 {
		return
	}

	if !l.activeDragmake {
		l.activeDragmake = true
		l.dragStartX = ev.Position.X - ev.Dragged.DX
		startY := ev.Position.Y - ev.Dragged.DY
		idx, mode := l.hitTestAction(l.dragStartX, startY, w, h)
		l.dragActionIdx = idx
		l.dragMode = mode
		if idx >= 0 && idx < len(l.track.Config.Actions) {
			act := l.track.Config.Actions[idx]
			l.origStartSec = clampFloat(act.StartSec, 0, dur)
			l.origEndSec = act.EndSec
			if l.origEndSec <= l.origStartSec {
				l.origEndSec = dur
			}
			l.origEndSec = clampFloat(l.origEndSec, l.origStartSec, dur)
			if l.tp.mw != nil {
				l.tp.mw.selectedActionID = act.ID
			}
		} else if mode == dragDisplayResizeStart || mode == dragDisplayResizeEnd {
			l.origStartSec = clampFloat(l.track.Config.StartTimeSec, 0, dur)
			l.origEndSec = l.track.Config.EndTimeSec
			if l.origEndSec <= 0 {
				l.origEndSec = dur
			}
			l.origEndSec = clampFloat(l.origEndSec, l.origStartSec, dur)
		}
	}

	deltaSec := float64(ev.Position.X-l.dragStartX) / float64(w) * dur
	minSpan := max(1.0/float64(l.tp.FPS()), 0.05)

	switch l.dragMode {
	case dragActionMove:
		if l.dragActionIdx >= 0 && l.dragActionIdx < len(l.track.Config.Actions) {
			span := l.origEndSec - l.origStartSec
			newStart := l.tp.SnapTime(l.origStartSec + deltaSec)
			if newStart < 0 {
				newStart = 0
			}
			if newStart+span > dur {
				newStart = max(0, dur-span)
			}
			l.track.Config.Actions[l.dragActionIdx].StartSec = newStart
			l.track.Config.Actions[l.dragActionIdx].EndSec = math.Round((newStart+span)*1000.0) / 1000.0
			l.track.Config.Actions[l.dragActionIdx].IsAll = false
			l.Refresh()
		}
	case dragActionResizeStart:
		if l.dragActionIdx >= 0 && l.dragActionIdx < len(l.track.Config.Actions) {
			newStart := l.tp.SnapTime(l.origStartSec + deltaSec)
			if newStart < 0 {
				newStart = 0
			}
			if newStart > l.origEndSec-minSpan {
				newStart = max(0, l.origEndSec-minSpan)
			}
			l.track.Config.Actions[l.dragActionIdx].StartSec = newStart
			l.track.Config.Actions[l.dragActionIdx].EndSec = l.origEndSec
			l.track.Config.Actions[l.dragActionIdx].IsAll = false
			l.Refresh()
		}
	case dragActionResizeEnd:
		if l.dragActionIdx >= 0 && l.dragActionIdx < len(l.track.Config.Actions) {
			newEnd := l.tp.SnapTime(l.origEndSec + deltaSec)
			if newEnd > dur {
				newEnd = dur
			}
			if newEnd < l.origStartSec+minSpan {
				newEnd = min(dur, l.origStartSec+minSpan)
			}
			l.track.Config.Actions[l.dragActionIdx].StartSec = l.origStartSec
			l.track.Config.Actions[l.dragActionIdx].EndSec = newEnd
			l.track.Config.Actions[l.dragActionIdx].IsAll = false
			l.Refresh()
		}
	case dragDisplayResizeStart:
		newStart := l.tp.SnapTime(l.origStartSec + deltaSec)
		if newStart < 0 {
			newStart = 0
		}
		if newStart > l.origEndSec-minSpan {
			newStart = max(0, l.origEndSec-minSpan)
		}
		l.track.Config.StartTimeSec = newStart
		l.track.Config.EndTimeSec = l.origEndSec
		l.Refresh()
	case dragDisplayResizeEnd:
		newEnd := l.tp.SnapTime(l.origEndSec + deltaSec)
		if newEnd > dur {
			newEnd = dur
		}
		if newEnd < l.origStartSec+minSpan {
			newEnd = min(dur, l.origStartSec+minSpan)
		}
		l.track.Config.StartTimeSec = l.origStartSec
		l.track.Config.EndTimeSec = newEnd
		l.Refresh()
	default:
		l.tp.ScrubAtX(ev.Position.X, w)
	}
	l.tp.UpdateHover(ev.Position.X, w)
}

func (l *timelineTrackLane) DragEnd() {
	if !l.activeDragmake {
		return
	}
	mode := l.dragMode
	l.activeDragmake = false
	l.dragMode = dragNone
	l.dragActionIdx = -1

	if mode != dragNone && mode != dragScrubPlayhead {
		l.tp.CommitTrackConfig(l.track.NodeID, l.track.ObjectKey, l.track.Config)
	}
}

func (l *timelineTrackLane) draw(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, max(w, 1), max(h, 1)))
	if w <= 1 || h <= 1 {
		return img
	}

	bg := color.RGBA{R: 18, G: 21, B: 28, A: 255}
	if l.track.IsSelected {
		bg = color.RGBA{R: 26, G: 34, B: 48, A: 255}
	}
	fillRectRGBA(img, 0, 0, w, h, bg)

	durSec := l.tp.TotalDurationSec()
	totalFrames := l.tp.FrameCount()

	// 1. Frame grid lines
	gridColor := color.RGBA{R: 30, G: 41, B: 59, A: 140}
	for i := range totalFrames + 1 {
		x := int(math.Round(float64(i) / float64(max(totalFrames, 1)) * float64(w-1)))
		drawVLineRGBA(img, x, 0, h-1, gridColor)
	}

	// 2. Display Window Span
	dispStart := 0.0
	dispEnd := durSec
	if l.track.Config.HasDisplayWindow {
		dispStart = l.track.Config.StartTimeSec
		if l.track.Config.EndTimeSec > 0 {
			dispEnd = l.track.Config.EndTimeSec
		}
	}
	if durSec > 0 {
		dx1 := int(math.Round((dispStart / durSec) * float64(w-1)))
		dx2 := int(math.Round((dispEnd / durSec) * float64(w-1)))
		if l.track.Config.HasDisplayWindow {
			fillRectRGBA(img, dx1, 2, dx2, h-2, color.RGBA{R: 56, G: 189, B: 248, A: 32})
			fillRectRGBA(img, dx1, 2, dx1+3, h-2, color.RGBA{R: 56, G: 189, B: 248, A: 180})
			fillRectRGBA(img, dx2-3, 2, dx2, h-2, color.RGBA{R: 56, G: 189, B: 248, A: 180})
		}
	}

	// 3. Action Clip Bars (stacked vertically when a track has multiple actions)
	if durSec > 0 {
		totalActions := len(l.track.Config.Actions)
		for idx, act := range l.track.Config.Actions {
			sSec := act.StartSec
			eSec := act.EndSec
			if eSec <= sSec {
				eSec = durSec
			}
			visStart := clampFloat(sSec, 0, durSec)
			visEnd := clampFloat(eSec, visStart, durSec)
			ax1 := int(math.Round((visStart / durSec) * float64(w-1)))
			ax2 := int(math.Round((visEnd / durSec) * float64(w-1)))
			if ax1 > w-12 {
				ax1 = max(0, w-12)
			}
			if ax2-ax1 < 12 {
				ax2 = min(w-1, ax1+12)
			}
			if ax2 >= w {
				ax2 = w - 1
			}
			fy1, fy2 := l.actionSubRowBounds(idx, totalActions, float32(h))
			ay1 := int(math.Round(float64(fy1)))
			ay2 := int(math.Round(float64(fy2)))
			if ay2-ay1 < 10 {
				ay2 = ay1 + 10
			}
			barColor := actionClipColor(act.Type)
			fillRectRGBA(img, ax1, ay1, ax2, ay2, barColor)

			// Left & Right resize grip handles (6px wide caps with vertical grip notch)
			gripColor := color.RGBA{R: 255, G: 255, B: 255, A: 210}
			if l.tp.mw != nil && l.tp.mw.selectedActionID == act.ID {
				gripColor = color.RGBA{R: 255, G: 255, B: 255, A: 255}
				drawRectBorderRGBA(img, ax1, ay1, ax2, ay2, gripColor)
			}
			fillRectRGBA(img, ax1, ay1, min(ax1+6, ax2), ay2, gripColor)
			fillRectRGBA(img, max(ax2-6, ax1), ay1, ax2, ay2, gripColor)
			notchColor := color.RGBA{R: 15, G: 17, B: 21, A: 220}
			if ax2-ax1 > 16 {
				drawVLineRGBA(img, ax1+3, ay1+3, ay2-4, notchColor)
				drawVLineRGBA(img, ax2-4, ay1+3, ay2-4, notchColor)
			}

			lbl := fmt.Sprintf("%s %.2fs-%.2fs", strings.ToUpper(act.Type), sSec, eSec)
			if ax2-ax1 > 34 {
				labelY := ay1 + max(1, (ay2-ay1-7)/2)
				drawTinyLabel(img, ax1+9, labelY, lbl, color.RGBA{R: 15, G: 17, B: 21, A: 255})
			}
		}
	}

	// 4. Hover guide line
	if l.tp.hoverActive && durSec > 0 {
		hx := int(math.Round((l.tp.hoverSec / durSec) * float64(w-1)))
		drawVLineRGBA(img, hx, 0, h-1, color.RGBA{R: 148, G: 163, B: 184, A: 140})
	}

	// 5. Playhead line
	playheadRatio := 0.0
	if totalFrames > 0 {
		playheadRatio = float64(l.tp.playheadFrame) / float64(totalFrames)
	} else if durSec > 0 {
		playheadRatio = l.tp.playheadSec / durSec
	}
	px := int(math.Round(playheadRatio * float64(w-1)))
	skyBlue := color.RGBA{R: 56, G: 189, B: 248, A: 255}
	drawVLineRGBA(img, px, 0, h-1, skyBlue)

	return img
}

func actionClipColor(actionType string) color.RGBA {
	switch strings.ToLower(actionType) {
	case "move":
		return color.RGBA{R: 56, G: 189, B: 248, A: 230} // Electric Sky Blue (#38BDF8)
	case "rot":
		return color.RGBA{R: 245, G: 158, B: 11, A: 230} // Amber (#F59E0B)
	case "scale":
		return color.RGBA{R: 16, G: 185, B: 129, A: 230} // Emerald (#10B981)
	case "fade":
		return color.RGBA{R: 168, G: 85, B: 247, A: 230} // Violet (#A855F7)
	case "color":
		return color.RGBA{R: 244, G: 63, B: 94, A: 230} // Rose (#F43F5E)
	case "depth":
		return color.RGBA{R: 99, G: 102, B: 241, A: 230} // Indigo (#6366F1)
	default:
		return color.RGBA{R: 148, G: 163, B: 184, A: 230}
	}
}

// ============================================================================
// Lightweight RGBA Drawing Helpers & 5x7 Bitmap Font for Timeline Labels
// ============================================================================

func fillRectRGBA(img *image.RGBA, x1, y1, x2, y2 int, c color.RGBA) {
	b := img.Bounds()
	x1 = max(x1, b.Min.X)
	y1 = max(y1, b.Min.Y)
	x2 = min(x2, b.Max.X)
	y2 = min(y2, b.Max.Y)
	for y := y1; y < y2; y++ {
		for x := x1; x < x2; x++ {
			img.SetRGBA(x, y, c)
		}
	}
}

func drawVLineRGBA(img *image.RGBA, x, y1, y2 int, c color.RGBA) {
	b := img.Bounds()
	if x < b.Min.X || x >= b.Max.X {
		return
	}
	y1 = max(y1, b.Min.Y)
	y2 = min(y2, b.Max.Y-1)
	for y := y1; y <= y2; y++ {
		img.SetRGBA(x, y, c)
	}
}

func drawRectBorderRGBA(img *image.RGBA, x1, y1, x2, y2 int, c color.RGBA) {
	fillRectRGBA(img, x1, y1, x2, y1+1, c)
	fillRectRGBA(img, x1, y2-1, x2, y2, c)
	fillRectRGBA(img, x1, y1, x1+1, y2, c)
	fillRectRGBA(img, x2-1, y1, x2, y2, c)
}

var tinyGlyph5x7 = map[rune][7]uint8{
	'0': {0x0E, 0x11, 0x13, 0x15, 0x19, 0x11, 0x0E},
	'1': {0x04, 0x0C, 0x04, 0x04, 0x04, 0x04, 0x0E},
	'2': {0x0E, 0x11, 0x01, 0x02, 0x04, 0x08, 0x1F},
	'3': {0x1F, 0x02, 0x04, 0x02, 0x01, 0x11, 0x0E},
	'4': {0x02, 0x06, 0x0A, 0x12, 0x1F, 0x02, 0x02},
	'5': {0x1F, 0x10, 0x1E, 0x01, 0x01, 0x11, 0x0E},
	'6': {0x06, 0x08, 0x10, 0x1E, 0x11, 0x11, 0x0E},
	'7': {0x1F, 0x01, 0x02, 0x04, 0x08, 0x08, 0x08},
	'8': {0x0E, 0x11, 0x11, 0x0E, 0x11, 0x11, 0x0E},
	'9': {0x0E, 0x11, 0x11, 0x0F, 0x01, 0x02, 0x0C},
	'.': {0x00, 0x00, 0x00, 0x00, 0x00, 0x0C, 0x0C},
	'-': {0x00, 0x00, 0x00, 0x1F, 0x00, 0x00, 0x00},
	's': {0x00, 0x00, 0x0F, 0x10, 0x0E, 0x01, 0x1E},
	'A': {0x0E, 0x11, 0x11, 0x1F, 0x11, 0x11, 0x11},
	'C': {0x0E, 0x11, 0x10, 0x10, 0x10, 0x11, 0x0E},
	'D': {0x1E, 0x11, 0x11, 0x11, 0x11, 0x11, 0x1E},
	'E': {0x1F, 0x10, 0x10, 0x1E, 0x10, 0x10, 0x1F},
	'F': {0x1F, 0x10, 0x10, 0x1E, 0x10, 0x10, 0x10},
	'H': {0x11, 0x11, 0x11, 0x1F, 0x11, 0x11, 0x11},
	'L': {0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x1F},
	'M': {0x11, 0x1B, 0x15, 0x15, 0x11, 0x11, 0x11},
	'O': {0x0E, 0x11, 0x11, 0x11, 0x11, 0x11, 0x0E},
	'P': {0x1E, 0x11, 0x11, 0x1E, 0x10, 0x10, 0x10},
	'R': {0x1E, 0x11, 0x11, 0x1E, 0x14, 0x12, 0x11},
	'S': {0x0F, 0x10, 0x10, 0x0E, 0x01, 0x01, 0x1E},
	'T': {0x1F, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04},
	'V': {0x11, 0x11, 0x11, 0x11, 0x11, 0x0A, 0x04},
}

func drawTinyLabel(img *image.RGBA, startX, startY int, text string, c color.RGBA) {
	b := img.Bounds()
	cx := startX
	for _, ch := range text {
		if ch == ' ' {
			cx += 4
			continue
		}
		glyph, ok := tinyGlyph5x7[ch]
		if !ok {
			cx += 4
			continue
		}
		if cx+5 >= b.Max.X {
			break
		}
		for row := range 7 {
			py := startY + row
			if py < b.Min.Y || py >= b.Max.Y {
				continue
			}
			bits := glyph[row]
			for col := range 5 {
				if (bits & (1 << (4 - col))) != 0 {
					px := cx + col
					if px >= b.Min.X && px < b.Max.X {
						img.SetRGBA(px, py, c)
					}
				}
			}
		}
		cx += 6
	}
}

// nonExpandingWidthLayout allows a child container to fill whatever horizontal width is allocated
// without forcing parent containers wider than a small minimum width (preventing long dynamic
// labels or toolbars from pushing the timeline off the right edge of the window).
type nonExpandingWidthLayout struct{}

func (l *nonExpandingWidthLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	h := float32(0)
	for _, o := range objects {
		if o.Visible() {
			ms := o.MinSize()
			if ms.Height > h {
				h = ms.Height
			}
		}
	}
	return fyne.NewSize(120, h)
}

func (l *nonExpandingWidthLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objects {
		if o.Visible() {
			o.Resize(size)
			o.Move(fyne.NewPos(0, 0))
		}
	}
}
