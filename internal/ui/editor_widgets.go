package ui

import (
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"inkanim/internal/ext/doctree"
	"inkanim/internal/ext/params"
)

var motionTypeDefaults = map[string]string{
	"Move":  "f: 1-20; ease: in-out",
	"Rot":   "f: 1-20; deg: 360",
	"Scale": "f: 1-20; from: 1.0; to: 0.5",
	"Fade":  "f: 1-20; from: 0; to: 1",
	"Show":  "f: 1-10",
	"Hide":  "f: 1-10",
	"Depth": "f: 1-20; order: 1",
	"Dist":  "factor: 0.5",
	"Color": "f: 1-20; from: #ffffff; to: #38bdf8",
}

var motionTypes = []string{
	"Move",
	"Rot",
	"Scale",
	"Fade",
	"Show",
	"Hide",
	"Depth",
	"Dist",
	"Color",
}

// commitEntry is an Entry widget that coalesces live typing modifications for smooth real-time preview,
// and only commits an undo snapshot when submitted (Enter key) or when focus is lost.
type editorCommitEntry struct {
	widget.Entry
	lastCommitted string
	onCommit      func(val string)
	onUndo        func()
	onRedo        func()
}

func newEditorCommitEntry(initialText string, onLiveChange func(string), onCommit func(string), onUndo func(), onRedo func()) *editorCommitEntry {
	e := &editorCommitEntry{
		Entry:         widget.Entry{Wrapping: fyne.TextWrap(fyne.TextTruncateClip)},
		lastCommitted: initialText,
		onCommit:      onCommit,
		onUndo:        onUndo,
		onRedo:        onRedo,
	}
	e.ExtendBaseWidget(e)
	e.Text = initialText
	e.OnChanged = onLiveChange
	e.OnSubmitted = func(_ string) {
		e.Commit()
	}
	return e
}

func (e *editorCommitEntry) FocusLost() {
	e.Entry.FocusLost()
	e.Commit()
}

func (e *editorCommitEntry) Commit() {
	current := e.Text
	if current != e.lastCommitted {
		e.lastCommitted = current
		if e.onCommit != nil {
			e.onCommit(current)
		}
	}
}

func (e *editorCommitEntry) SetText(text string) {
	e.lastCommitted = text
	e.Entry.SetText(text)
}

func (e *editorCommitEntry) TypedShortcut(shortcut fyne.Shortcut) {
	if _, ok := shortcut.(*fyne.ShortcutUndo); ok {
		if e.Text != e.lastCommitted {
			e.SetText(e.lastCommitted)
			return
		}
		if e.onUndo != nil {
			e.onUndo()
		}
		return
	}
	if _, ok := shortcut.(*fyne.ShortcutRedo); ok {
		if e.onRedo != nil {
			e.onRedo()
		}
		return
	}
	if cs, ok := shortcut.(*desktop.CustomShortcut); ok {
		ctrlOrCmd := cs.Modifier&fyne.KeyModifierControl != 0 || cs.Modifier&fyne.KeyModifierSuper != 0
		if cs.KeyName == fyne.KeyZ && ctrlOrCmd {
			if cs.Modifier&fyne.KeyModifierShift != 0 {
				if e.onRedo != nil {
					e.onRedo()
				}
				return
			}
			if e.Text != e.lastCommitted {
				e.SetText(e.lastCommitted)
				return
			}
			if e.onUndo != nil {
				e.onUndo()
			}
			return
		}
		if cs.KeyName == fyne.KeyY && ctrlOrCmd {
			if e.onRedo != nil {
				e.onRedo()
			}
			return
		}
	}
	e.Entry.TypedShortcut(shortcut)
}

type deletableTree struct {
	widget.Tree
	onDelete func()
}

func newDeletableTree(
	childUIDs func(string) []string,
	isBranch func(string) bool,
	create func(bool) fyne.CanvasObject,
	update func(string, bool, fyne.CanvasObject),
	onDelete func(),
) *deletableTree {
	t := &deletableTree{
		Tree: widget.Tree{
			ChildUIDs:  childUIDs,
			IsBranch:   isBranch,
			CreateNode: create,
			UpdateNode: update,
		},
		onDelete: onDelete,
	}
	t.ExtendBaseWidget(t)
	return t
}

func (t *deletableTree) TypedKey(ev *fyne.KeyEvent) {
	if ev.Name == fyne.KeyDelete {
		if t.onDelete != nil {
			t.onDelete()
			return
		}
	}
	t.Tree.TypedKey(ev)
}

func buildDirectiveWidgetCard(
	dir *doctree.Directive,
	totalDocFrames int,
	onModified func(),
	onCommit func(description string),
	onCopy func(),
	onDelete func(),
	onUndo func(),
	onRedo func(),
) fyne.CanvasObject {
	m := params.Parse(dir.Type, dir.Params, totalDocFrames)
	initializing := true

	syncToDir := func() {
		if initializing {
			return
		}
		dir.Params = m.Format()
		dir.Raw = fmt.Sprintf("%s {%s}", dir.Type, dir.Params)
		onModified()
	}

	rawPreviewLabel := widget.NewLabelWithStyle(dir.Raw, fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})
	rawPreviewLabel.Truncation = fyne.TextTruncateClip

	syncAndPreview := func() {
		if initializing {
			return
		}
		syncToDir()
		rawPreviewLabel.SetText(dir.Raw)
	}

	syncAndCommit := func(desc string) {
		if initializing {
			return
		}
		syncToDir()
		rawPreviewLabel.SetText(dir.Raw)
		if onCommit != nil {
			onCommit(desc)
		}
	}

	// 1. Header: Directive Type Title, Copy & Delete Buttons
	typeTitle := widget.NewLabelWithStyle(dir.Type, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	copyBtn := widget.NewButtonWithIcon("", theme.ContentCopyIcon(), onCopy)
	copyBtn.Importance = widget.LowImportance

	delBtn := widget.NewButtonWithIcon("", theme.DeleteIcon(), onDelete)
	delBtn.Importance = widget.DangerImportance

	actions := container.NewHBox(copyBtn, delBtn)
	headerRow := container.NewBorder(nil, nil, nil, actions, typeTitle)

	// 2. Timing Controls (Frames)
	startEntry := newEditorCommitEntry(
		strconv.Itoa(m.StartFrame),
		func(s string) {
			if v, err := strconv.Atoi(s); err == nil && v > 0 {
				m.StartFrame = v
				syncAndPreview()
			}
		},
		func(s string) {
			if v, err := strconv.Atoi(s); err == nil && v > 0 && onCommit != nil {
				onCommit(fmt.Sprintf("Change %s start frame to %d", dir.Type, v))
			}
		},
		onUndo,
		onRedo,
	)

	endEntry := newEditorCommitEntry(
		strconv.Itoa(m.EndFrame),
		func(s string) {
			if v, err := strconv.Atoi(s); err == nil && v > 0 {
				m.EndFrame = v
				syncAndPreview()
			}
		},
		func(s string) {
			if v, err := strconv.Atoi(s); err == nil && v > 0 && onCommit != nil {
				onCommit(fmt.Sprintf("Change %s end frame to %d", dir.Type, v))
			}
		},
		onUndo,
		onRedo,
	)

	allFramesCheck := widget.NewCheck("All Frames", nil)
	allFramesCheck.SetChecked(m.IsAll)

	updateFrameEnables := func(isAll bool) {
		if isAll {
			startEntry.Disable()
			endEntry.Disable()
		} else {
			startEntry.Enable()
			endEntry.Enable()
		}
	}
	updateFrameEnables(m.IsAll)

	allFramesCheck.OnChanged = func(checked bool) {
		m.IsAll = checked
		updateFrameEnables(checked)
		syncAndCommit("Toggle all frames on " + dir.Type)
	}

	framesRow := container.NewHBox(
		widget.NewLabel("Frames:"),
		container.NewGridWithColumns(2, startEntry, endEntry),
		allFramesCheck,
	)

	// 3. Playback Controls (Easing, Repeat, Ping-Pong, Reverse)
	easingOptions := []string{"linear", "in", "out", "in-out", "bounce"}
	easeSelect := widget.NewSelect(easingOptions, func(sel string) {
		m.Ease = sel
		syncAndCommit(fmt.Sprintf("Change %s easing to %s", dir.Type, sel))
	})
	if m.Ease == "" {
		easeSelect.SetSelected("linear")
	} else {
		easeSelect.SetSelected(m.Ease)
	}

	repeatOptions := []string{"1x (Once)", "2x", "3x", "4x", "5x", "6x", "8x", "10x"}
	repeatSelect := widget.NewSelect(repeatOptions, func(sel string) {
		var r int
		if _, err := fmt.Sscanf(sel, "%dx", &r); err == nil && r > 0 {
			m.Repeat = r
			syncAndCommit(fmt.Sprintf("Change %s repeat to %s", dir.Type, sel))
		}
	})
	if m.Repeat > 1 {
		repeatSelect.SetSelected(fmt.Sprintf("%dx", m.Repeat))
	} else {
		repeatSelect.SetSelected("1x (Once)")
	}

	pingpongCheck := widget.NewCheck("Ping-Pong", func(checked bool) {
		m.PingPong = checked
		syncAndCommit("Toggle ping-pong on " + dir.Type)
	})
	pingpongCheck.SetChecked(m.PingPong)

	revCheck := widget.NewCheck("Reverse", func(checked bool) {
		m.Reverse = checked
		syncAndCommit("Toggle reverse on " + dir.Type)
	})
	revCheck.SetChecked(m.Reverse)

	playbackRow := container.NewHBox(
		widget.NewLabel("Ease:"),
		easeSelect,
		widget.NewLabel("Repeat:"),
		repeatSelect,
		pingpongCheck,
		revCheck,
	)

	// 4. Type-Specific Parameter Widgets
	typeProps := container.NewVBox()

	switch m.Type {
	case "Rot":
		degVal := m.Angle
		if !m.HasAngle {
			degVal = 360
		}
		degEntry := newEditorCommitEntry(
			fmt.Sprintf("%g", degVal),
			func(s string) {
				if v, err := strconv.ParseFloat(s, 64); err == nil {
					m.Angle = v
					m.HasAngle = true
					syncAndPreview()
				}
			},
			func(s string) {
				if v, err := strconv.ParseFloat(s, 64); err == nil && onCommit != nil {
					onCommit(fmt.Sprintf("Set Rot angle to %g°", v))
				}
			},
			onUndo,
			onRedo,
		)

		dirSelect := widget.NewSelect([]string{"Clockwise (cw)", "Counter-Clockwise (ccw)"}, func(sel string) {
			if strings.Contains(sel, "ccw") {
				m.RotDir = "ccw"
			} else {
				m.RotDir = "cw"
			}
			syncAndCommit("Set Rot direction to " + m.RotDir)
		})
		if m.RotDir == "ccw" {
			dirSelect.SetSelected("Counter-Clockwise (ccw)")
		} else {
			dirSelect.SetSelected("Clockwise (cw)")
		}

		pivotSelect := widget.NewSelect([]string{"center", "top", "bottom", "left", "right", "0", "90", "180", "270"}, func(sel string) {
			m.Pivot = sel
			syncAndCommit("Set Rot pivot to " + sel)
		})
		if m.Pivot != "" {
			pivotSelect.SetSelected(m.Pivot)
		} else {
			pivotSelect.SetSelected("center")
		}

		rotRow := container.NewHBox(
			widget.NewLabel("Degrees (°):"),
			degEntry,
			widget.NewLabel("Direction:"),
			dirSelect,
			widget.NewLabel("Pivot:"),
			pivotSelect,
		)
		typeProps.Add(rotRow)

	case "Scale":
		fromEntry := newEditorCommitEntry(
			fmt.Sprintf("%g", m.ScaleFrom),
			func(s string) {
				if v, err := strconv.ParseFloat(s, 64); err == nil {
					m.ScaleFrom = v
					m.HasScale = true
					syncAndPreview()
				}
			},
			func(s string) {
				if v, err := strconv.ParseFloat(s, 64); err == nil && onCommit != nil {
					onCommit(fmt.Sprintf("Set Scale 'from' to %g", v))
				}
			},
			onUndo,
			onRedo,
		)

		toEntry := newEditorCommitEntry(
			fmt.Sprintf("%g", m.ScaleTo),
			func(s string) {
				if v, err := strconv.ParseFloat(s, 64); err == nil {
					m.ScaleTo = v
					m.HasScale = true
					syncAndPreview()
				}
			},
			func(s string) {
				if v, err := strconv.ParseFloat(s, 64); err == nil && onCommit != nil {
					onCommit(fmt.Sprintf("Set Scale 'to' to %g", v))
				}
			},
			onUndo,
			onRedo,
		)

		scaleRow := container.NewHBox(
			widget.NewLabel("From Scale:"),
			fromEntry,
			widget.NewLabel("To Scale:"),
			toEntry,
		)
		typeProps.Add(scaleRow)

	case "Move":
		orientCheck := widget.NewCheck("Orient along path", func(checked bool) {
			m.Orient = checked
			syncAndCommit("Toggle Move orientation")
		})
		orientCheck.SetChecked(m.Orient)

		pivotSelect := widget.NewSelect([]string{"center", "top", "bottom", "left", "right"}, func(sel string) {
			m.Pivot = sel
			syncAndCommit("Set Move pivot to " + sel)
		})
		if m.Pivot != "" {
			pivotSelect.SetSelected(m.Pivot)
		} else {
			pivotSelect.SetSelected("center")
		}

		moveRow := container.NewHBox(
			orientCheck,
			widget.NewLabel("Pivot:"),
			pivotSelect,
		)
		typeProps.Add(moveRow)

	case "Fade":
		fromEntry := newEditorCommitEntry(
			fmt.Sprintf("%g", m.OpacityFrom),
			func(s string) {
				if v, err := strconv.ParseFloat(s, 64); err == nil {
					m.OpacityFrom = v
					m.HasOpacity = true
					syncAndPreview()
				}
			},
			func(s string) {
				if v, err := strconv.ParseFloat(s, 64); err == nil && onCommit != nil {
					onCommit(fmt.Sprintf("Set Fade 'from' to %g", v))
				}
			},
			onUndo,
			onRedo,
		)

		toEntry := newEditorCommitEntry(
			fmt.Sprintf("%g", m.OpacityTo),
			func(s string) {
				if v, err := strconv.ParseFloat(s, 64); err == nil {
					m.OpacityTo = v
					m.HasOpacity = true
					syncAndPreview()
				}
			},
			func(s string) {
				if v, err := strconv.ParseFloat(s, 64); err == nil && onCommit != nil {
					onCommit(fmt.Sprintf("Set Fade 'to' to %g", v))
				}
			},
			onUndo,
			onRedo,
		)

		fadeRow := container.NewHBox(
			widget.NewLabel("From Opacity (0-1):"),
			fromEntry,
			widget.NewLabel("To Opacity (0-1):"),
			toEntry,
		)
		typeProps.Add(fadeRow)

	case "Color":
		targetSelect := widget.NewSelect([]string{"fill", "stroke", "all"}, func(sel string) {
			m.ColorTarget = sel
			syncAndCommit("Set Color target to " + sel)
		})
		if m.ColorTarget != "" {
			targetSelect.SetSelected(m.ColorTarget)
		} else {
			targetSelect.SetSelected("fill")
		}

		degEntry := newEditorCommitEntry(
			fmt.Sprintf("%g", m.ColorAngle),
			func(s string) {
				if v, err := strconv.ParseFloat(s, 64); err == nil {
					m.ColorAngle = v
					m.HasColorAngle = true
					syncAndPreview()
				}
			},
			func(s string) {
				if v, err := strconv.ParseFloat(s, 64); err == nil && onCommit != nil {
					onCommit(fmt.Sprintf("Set Color angle to %g°", v))
				}
			},
			onUndo,
			onRedo,
		)

		colorRow := container.NewHBox(
			widget.NewLabel("Target:"),
			targetSelect,
			widget.NewLabel("Degrees (°):"),
			degEntry,
		)
		typeProps.Add(colorRow)

	case "Dist":
		factorEntry := newEditorCommitEntry(
			fmt.Sprintf("%g", m.ParallaxFactor),
			func(s string) {
				if v, err := strconv.ParseFloat(s, 64); err == nil {
					m.ParallaxFactor = v
					m.HasParallax = true
					syncAndPreview()
				}
			},
			func(s string) {
				if v, err := strconv.ParseFloat(s, 64); err == nil && onCommit != nil {
					onCommit(fmt.Sprintf("Set Dist parallax factor to %g", v))
				}
			},
			onUndo,
			onRedo,
		)

		fixedCheck := widget.NewCheck("Fixed Parallax (factor 0)", func(checked bool) {
			if checked {
				m.ParallaxFactor = 0.0
				factorEntry.SetText("0")
				factorEntry.Disable()
			} else {
				if m.ParallaxFactor == 0.0 {
					m.ParallaxFactor = 1.0
					factorEntry.SetText("1")
				}
				factorEntry.Enable()
			}
			m.HasParallax = true
			syncAndCommit("Toggle fixed parallax")
		})
		if m.ParallaxFactor == 0.0 {
			fixedCheck.SetChecked(true)
			factorEntry.Disable()
		}

		distRow := container.NewHBox(
			widget.NewLabel("Parallax Factor:"),
			factorEntry,
			fixedCheck,
		)
		typeProps.Add(distRow)

	case "Depth":
		orderEntry := newEditorCommitEntry(
			strconv.Itoa(m.DepthOffset),
			func(s string) {
				if v, err := strconv.Atoi(s); err == nil {
					m.DepthOffset = v
					syncAndPreview()
				}
			},
			func(s string) {
				if v, err := strconv.Atoi(s); err == nil && onCommit != nil {
					onCommit(fmt.Sprintf("Set Depth order to %d", v))
				}
			},
			onUndo,
			onRedo,
		)

		depthRow := container.NewHBox(
			widget.NewLabel("Z-Order Offset:"),
			orderEntry,
		)
		typeProps.Add(depthRow)
	}

	initializing = false

	cardItems := []fyne.CanvasObject{
		headerRow,
	}

	if m.Type != "Dist" {
		cardItems = append(cardItems, framesRow)
	}

	isContinuous := m.Type == "Move" || m.Type == "Rot" || m.Type == "Scale" || m.Type == "Fade" || m.Type == "Color"
	if isContinuous {
		cardItems = append(cardItems, playbackRow)
	}

	if len(typeProps.Objects) > 0 {
		cardItems = append(cardItems, typeProps)
	}
	cardItems = append(cardItems, rawPreviewLabel, widget.NewSeparator())

	return container.NewVBox(cardItems...)
}
