package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"inkanim/assets"
	"inkanim/internal/app"
	"inkanim/internal/ext/doctree"
	"inkanim/internal/ext/params"
	"inkanim/internal/ext/presets"
	"inkanim/internal/ext/svgpatch"
	"inkanim/internal/ui"
	"inkanim/pkg/inksvg"
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

type EditorState struct {
	Roots       []*doctree.DocNode
	NodeMap     map[string]*doctree.DocNode
	ActiveNode  *doctree.DocNode
	ModifiedIDs map[string]bool
	SelectedIDs []string
	InputPath   string
	SVGData     []byte
	Result      []byte
	Applied     bool
}

// ComputePatchedSVG generates the updated SVG bytes with all modified labels applied.
func (s *EditorState) ComputePatchedSVG() []byte {
	patchedData := s.SVGData
	for id := range s.ModifiedIDs {
		node := s.NodeMap[id]
		if node == nil {
			continue
		}
		newLabel := node.FormatLabel()
		patched, err := svgpatch.SetAttr(patchedData, id, svgpatch.InkscapeNS, "label", newLabel)
		if err == nil {
			patchedData = patched
		}
	}
	return patchedData
}

// ApplyPreset applies a motion preset to the active node. If the active node is a group,
// or a child drawing shape without existing directives, an anchor element (dot or path)
// centered on the target is automatically generated and inserted into the group.
func (s *EditorState) ApplyPreset(p presets.Preset) (string, error) {
	if s.ActiveNode == nil {
		return "", errors.New("no active node selected")
	}

	targetNode := s.ActiveNode
	isGroup := targetNode.IsGroup || targetNode.IsLayer

	// If target is an ordinary non-group shape that doesn't have directives,
	// anchor to its parent group if available to avoid culling the user's artwork.
	if !isGroup && !targetNode.HasDirectives() && targetNode.ParentID != "" {
		if parent := s.NodeMap[targetNode.ParentID]; parent != nil {
			targetNode = parent
			isGroup = true
		}
	}

	if isGroup {
		// Calculate center of target from SVG geometry and detect total frames
		doc, err := inksvg.ParseSVG(s.ComputePatchedSVG())
		var cx, cy float64
		totalFrames := 20
		if err == nil {
			if doc.FrameCount() > 0 {
				totalFrames = doc.FrameCount()
			}
			measureID := s.ActiveNode.ID
			rect := doc.GetElementRect(measureID)
			cx = rect.X + rect.Width/2.0
			cy = rect.Y + rect.Height/2.0
		}

		// Generate unique anchor ID
		baseID := fmt.Sprintf("%s_%s", targetNode.ID, p.ID)
		anchorID := baseID
		counter := 2
		for s.NodeMap[anchorID] != nil {
			anchorID = fmt.Sprintf("%s_%d", baseID, counter)
			counter++
		}

		childXML := p.GenerateAnchorElement(anchorID, cx, cy, totalFrames)
		patched, err := svgpatch.InsertChild(s.ComputePatchedSVG(), targetNode.ID, childXML)
		if err != nil {
			return "", fmt.Errorf("failed to insert anchor element: %w", err)
		}

		s.SVGData = patched
		s.ModifiedIDs = make(map[string]bool)

		// Refresh doctree
		roots, nodeMap, err := doctree.ParseTree(s.SVGData)
		if err != nil {
			return "", fmt.Errorf("failed to re-parse tree: %w", err)
		}
		s.Roots = roots
		s.NodeMap = nodeMap
		if newAnchor := nodeMap[anchorID]; newAnchor != nil {
			s.ActiveNode = newAnchor
		}
		return anchorID, nil
	}

	// Applying preset directly to an existing anchor or directive-carrying shape
	totalFrames := 20
	if doc, err := inksvg.ParseSVG(s.ComputePatchedSVG()); err == nil && doc.FrameCount() > 0 {
		totalFrames = doc.FrameCount()
	}
	label := p.FormatLabelForFrames(totalFrames)
	prefix, directives, suffix := doctree.ParseDirectives(label)
	targetNode.LabelPrefix = prefix
	targetNode.Directives = directives
	targetNode.LabelSuffix = suffix
	s.ModifiedIDs[targetNode.ID] = true
	return targetNode.ID, nil
}


// NewEditorState initializes the editor model from SVG data and CLI parameters.
func NewEditorState(data []byte, inputPath string, selectedIDs []string) (*EditorState, error) {
	roots, nodeMap, err := doctree.ParseTree(data)
	if err != nil {
		return nil, err
	}

	var active *doctree.DocNode
	if len(selectedIDs) > 0 {
		active = nodeMap[selectedIDs[0]]
	}
	if active == nil && len(roots) > 0 {
		active = roots[0]
	}

	state := &EditorState{
		Roots:       roots,
		NodeMap:     nodeMap,
		ActiveNode:  active,
		ModifiedIDs: make(map[string]bool),
		SelectedIDs: selectedIDs,
		InputPath:   inputPath,
		SVGData:     data,
		Result:      data,
	}

	// Mark any nodes whose labels were migrated on load as modified
	for _, node := range nodeMap {
		if node.HasDirectives() {
			orig := node.Label
			migrated := node.FormatLabel()
			if orig != migrated && orig != "" {
				state.ModifiedIDs[node.ID] = true
			}
		}
	}

	return state, nil
}

// ShowEditorWindow displays the interactive Motion Editor window with live preview.
func ShowEditorWindow(a fyne.App, state *EditorState) fyne.Window {
	w := a.NewWindow("InkAnim Motion Studio")
	w.SetIcon(assets.AppIcon)
	w.Resize(fyne.NewSize(1200, 720))

	// Live Animation Preview Session
	sess := app.NewSession()
	_ = sess.LoadSVGData(state.ComputePatchedSVG(), state.InputPath)
	preview := ui.NewCenterPreviewPanel(sess)
	preview.SetInspectorVisible(false)
	if len(sess.RenderedFrames) > 1 {
		preview.Play()
	}

	var leftPanel *ui.LeftFramesPanel
	var rightPanel *ui.RightExportPanel

	leftPanel = ui.NewLeftFramesPanel(sess, func() {
		preview.Refresh()
		if rightPanel != nil {
			rightPanel.Refresh()
		}
	})

	rightPanel = ui.NewRightExportPanel(sess, w, func() {
		preview.Refresh()
	}, func() func() {
		wasPlaying := preview.IsPlaying()
		if wasPlaying {
			preview.Pause()
		}
		return func() {
			if wasPlaying {
				preview.Play()
			}
		}
	})

	hasExported := false
	rightPanel.SetOnExportSuccess(func(outputPath string) {
		hasExported = true
	})

	var previewTimer *time.Timer
	var previewMu sync.Mutex
	schedulePreviewUpdate := func() {
		previewMu.Lock()
		if previewTimer != nil {
			previewTimer.Stop()
		}
		previewTimer = time.AfterFunc(200*time.Millisecond, func() {
			patched := state.ComputePatchedSVG()
			fyne.Do(func() {
				if err := sess.LoadSVGData(patched, state.InputPath); err == nil {
					preview.Refresh()
					if leftPanel != nil {
						leftPanel.Refresh()
					}
					if rightPanel != nil {
						rightPanel.Refresh()
					}
					if len(sess.RenderedFrames) > 1 && !preview.IsPlaying() {
						preview.Play()
					}
				}
			})
		})
		previewMu.Unlock()
	}

	cleanupPlayback := func() {
		previewMu.Lock()
		if previewTimer != nil {
			previewTimer.Stop()
		}
		previewMu.Unlock()
		if preview.IsPlaying() {
			preview.Pause()
		}
	}
	w.SetOnClosed(cleanupPlayback)

	// Status label at bottom
	statusLabel := widget.NewLabel("")
	updateStatus := func() {
		var activeName string
		if state.ActiveNode != nil {
			activeName = state.ActiveNode.DisplayTitle()
		} else {
			activeName = "None"
		}
		statusLabel.SetText(fmt.Sprintf("Active Object: %s | Modified: %d",
			activeName, len(state.ModifiedIDs)))
	}

	// Inspector container
	inspectorCard := container.NewStack()

	var tree *widget.Tree
	var refreshTree func()
	var refreshInspector func()
	refreshInspector = func() {
		node := state.ActiveNode
		if node == nil {
			emptyNotice := container.NewCenter(widget.NewLabel("Select an object from the document tree to view and edit its motions."))
			inspectorCard.Objects = []fyne.CanvasObject{emptyNotice}
			inspectorCard.Refresh()
			updateStatus()
			return
		}

		// Header
		var kind string
		switch {
		case node.IsLayer:
			kind = "Layer"
		case node.IsGroup:
			kind = "Group"
		default:
			kind = strings.ToUpper(node.Tag)
		}

		titleLabel := widget.NewLabelWithStyle(fmt.Sprintf("%s: %s", kind, node.ID), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		tagLabel := widget.NewLabel(fmt.Sprintf("Tag: <%s>", node.Tag))

		headerBox := container.NewVBox(
			titleLabel,
			tagLabel,
			widget.NewSeparator(),
		)

		// Resulting label preview
		previewLabel := widget.NewLabelWithStyle("Label preview will appear here", fyne.TextAlignLeading, fyne.TextStyle{Italic: true})
		updatePreview := func() {
			lbl := node.FormatLabel()
			if lbl == "" {
				lbl = "(empty label)"
			}
			previewLabel.SetText("inkscape:label: " + lbl)
		}

		// Label prefix entry (e.g. "Fade In: ")
		prefixEntry := widget.NewEntry()
		prefixEntry.SetPlaceHolder("Descriptive name or prefix (optional)")
		prefixEntry.SetText(node.LabelPrefix)
		prefixEntry.OnChanged = func(val string) {
			node.LabelPrefix = val
			state.ModifiedIDs[node.ID] = true
			updatePreview()
			updateStatus()
			refreshTree()
			schedulePreviewUpdate()
		}

		// Directives list container
		directivesList := container.NewVBox()

		var rebuildDirectivesList func()
		rebuildDirectivesList = func() {
			directivesList.Objects = nil
			if len(node.Directives) == 0 {
				directivesList.Add(widget.NewLabel("No motion directives configured on this element."))
			} else {
				// Detect total doc frames
				doc, err := inksvg.ParseSVG(state.ComputePatchedSVG())
				totalDocFrames := 20
				if err == nil && doc.FrameCount() > 0 {
					totalDocFrames = doc.FrameCount()
				}

				for i := range node.Directives {
					idx := i
					dir := &node.Directives[idx]

					onModified := func() {
						state.ModifiedIDs[node.ID] = true
						updatePreview()
						updateStatus()
						refreshTree()
						schedulePreviewUpdate()
					}

					onDelete := func() {
						node.Directives = append(node.Directives[:idx], node.Directives[idx+1:]...)
						state.ModifiedIDs[node.ID] = true
						rebuildDirectivesList()
						updatePreview()
						updateStatus()
						refreshTree()
						schedulePreviewUpdate()
					}

					card := buildDirectiveWidgetCard(dir, totalDocFrames, onModified, onDelete)
					directivesList.Add(card)
				}
			}
			directivesList.Refresh()
		}

		rebuildDirectivesList()

		// Add Motion Button with Type Selector
		addMotionSelect := widget.NewSelect(motionTypes, nil)
		addMotionSelect.PlaceHolder = "Select motion type..."

		addMotionBtn := widget.NewButtonWithIcon("Add Motion", theme.ContentAddIcon(), func() {
			mType := addMotionSelect.Selected
			if mType == "" {
				mType = "Move"
			}
			params := motionTypeDefaults[mType]
			if len(node.Directives) == 0 && node.LabelPrefix == "" {
				node.LabelPrefix = node.ID
				prefixEntry.SetText(node.ID)
			}
			node.Directives = append(node.Directives, doctree.Directive{
				Type:   mType,
				Params: params,
				Raw:    fmt.Sprintf("%s {%s}", mType, params),
			})
			state.ModifiedIDs[node.ID] = true
			rebuildDirectivesList()
			updatePreview()
			updateStatus()
			refreshTree()
			schedulePreviewUpdate()
		})
		addMotionBtn.Importance = widget.MediumImportance

		addBar := container.NewBorder(nil, nil, nil, addMotionBtn, addMotionSelect)

		// Preset Selection
		presetList := presets.AllPresets()
		presetNames := make([]string, len(presetList))
		for i, p := range presetList {
			presetNames[i] = p.Name
		}

		presetSelect := widget.NewSelect(presetNames, nil)
		presetSelect.PlaceHolder = "Select a motion preset..."
		presetDescLabel := widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Italic: true})

		applyPresetBtn := widget.NewButtonWithIcon("Apply Preset", theme.MediaPlayIcon(), func() {
			sel := presetSelect.Selected
			if sel == "" {
				return
			}
			p, ok := presets.FindPreset(sel)
			if !ok {
				return
			}
			anchorID, err := state.ApplyPreset(p)
			if err != nil {
				return
			}
			updatePreview()
			updateStatus()
			refreshTree()
			if tree != nil {
				if node.IsGroup || node.IsLayer {
					tree.OpenBranch(node.ID)
				} else if node.ParentID != "" {
					tree.OpenBranch(node.ParentID)
				}
				tree.Select(anchorID)
			}
			refreshInspector()
			schedulePreviewUpdate()
		})
		applyPresetBtn.Importance = widget.HighImportance

		presetSelect.OnChanged = func(sel string) {
			if p, ok := presets.FindPreset(sel); ok {
				info := p.Description
				if node.IsGroup || node.IsLayer || (!node.HasDirectives() && node.ParentID != "") {
					info += " (auto-adds anchor object to group)"
				}
				presetDescLabel.SetText(info)
			} else {
				presetDescLabel.SetText("")
			}
		}

		presetBar := container.NewBorder(nil, nil, nil, applyPresetBtn, presetSelect)
		presetBox := container.NewVBox(
			presetBar,
			presetDescLabel,
		)

		updatePreview()

		content := container.NewVBox(
			headerBox,
			widget.NewLabelWithStyle("Motion Presets", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			presetBox,
			widget.NewSeparator(),
			widget.NewLabelWithStyle("Object Name / Label Prefix", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			prefixEntry,
			widget.NewSeparator(),
			widget.NewLabelWithStyle("Motion Directives", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			directivesList,
			widget.NewSeparator(),
			addBar,
			widget.NewSeparator(),
			previewLabel,
		)

		scrollContent := container.NewVScroll(content)
		inspectorCard.Objects = []fyne.CanvasObject{scrollContent}
		inspectorCard.Refresh()
		updateStatus()
	}

	// Build Tree widget
	tree = widget.NewTree(
		func(uid string) []string {
			if uid == "" {
				uids := make([]string, len(state.Roots))
				for i, r := range state.Roots {
					uids[i] = r.ID
				}
				return uids
			}
			node := state.NodeMap[uid]
			if node == nil {
				return nil
			}
			uids := make([]string, len(node.Children))
			for i, c := range node.Children {
				uids[i] = c.ID
			}
			return uids
		},
		func(uid string) bool {
			if uid == "" {
				return len(state.Roots) > 0
			}
			node := state.NodeMap[uid]
			return node != nil && len(node.Children) > 0
		},
		func(branch bool) fyne.CanvasObject {
			icon := widget.NewIcon(theme.DocumentIcon())
			iconWrap := container.NewGridWrap(fyne.NewSize(12, 12), icon)
			txt := canvas.NewText("node template", theme.Color(theme.ColorNameForeground))
			txt.TextSize = 7.0
			return container.NewHBox(
				iconWrap,
				txt,
			)
		},
		func(uid string, branch bool, obj fyne.CanvasObject) {
			box := obj.(*fyne.Container)
			iconWrap := box.Objects[0].(*fyne.Container)
			icon := iconWrap.Objects[0].(*widget.Icon)
			txt := box.Objects[1].(*canvas.Text)
			txt.TextSize = 7.0

			node := state.NodeMap[uid]
			if node == nil {
				txt.Text = uid
				txt.Refresh()
				return
			}

			// Choose appropriate icon
			switch {
			case node.IsLayer:
				icon.SetResource(theme.FolderOpenIcon())
			case node.IsGroup:
				icon.SetResource(theme.FolderIcon())
			default:
				icon.SetResource(theme.DocumentIcon())
			}

			displayText := node.DisplayTitle()
			switch {
			case node.IsLayer:
				displayText = "Layer: " + displayText
			case node.IsGroup:
				displayText = "Group: " + displayText
			default:
				displayText = "<" + node.Tag + "> " + displayText
			}

			if node.HasDirectives() {
				var dirTypes []string
				for _, d := range node.Directives {
					dirTypes = append(dirTypes, d.Type)
				}
				displayText = fmt.Sprintf("✦ %s [%s]", displayText, strings.Join(dirTypes, ", "))
				txt.TextStyle = fyne.TextStyle{Bold: true}
			} else {
				txt.TextStyle = fyne.TextStyle{}
			}

			txt.Text = displayText
			txt.Color = theme.Color(theme.ColorNameForeground)
			txt.Refresh()
		},
	)

	tree.OnSelected = func(uid string) {
		if node, ok := state.NodeMap[uid]; ok {
			state.ActiveNode = node
			refreshInspector()
		}
	}

	refreshTree = func() {
		tree.Refresh()
	}

	// Studio Layout:
	// Left side: Tabs [Layers & Motion | Frames | Export]
	// Right side: [Live Canvas Preview]
	inspectorScroll := container.NewVScroll(inspectorCard)
	motionSplit := container.NewHSplit(tree, inspectorScroll)
	motionSplit.SetOffset(0.38)

	sidebarTabs := container.NewAppTabs(
		container.NewTabItemWithIcon("Layers & Motion", theme.VisibilityIcon(), motionSplit),
		container.NewTabItemWithIcon("Frames", theme.ListIcon(), leftPanel.Container()),
		container.NewTabItemWithIcon("Export", theme.DownloadIcon(), rightPanel.Container()),
	)

	mainSplit := container.NewHSplit(sidebarTabs, preview.Container())
	mainSplit.SetOffset(0.55)

	handleCloseRequest := func() {
		if len(state.ModifiedIDs) == 0 {
			cleanupPlayback()
			state.Result = state.SVGData
			state.Applied = false
			w.Close()
			a.Quit()
			return
		}

		var message string
		if hasExported {
			message = "You have exported an animated GIF, but your motion changes have not been applied to your Inkscape document yet.\n\nWould you like to apply your changes to the document before closing?"
		} else {
			message = fmt.Sprintf("You have unapplied motion changes on %d element(s).\n\nWould you like to apply your changes to your Inkscape document before closing?", len(state.ModifiedIDs))
		}

		var d *dialog.CustomDialog

		applyAndClose := widget.NewButtonWithIcon("Apply & Close", theme.ConfirmIcon(), func() {
			d.Hide()
			cleanupPlayback()
			state.Result = state.ComputePatchedSVG()
			state.Applied = true
			w.Close()
			a.Quit()
		})
		applyAndClose.Importance = widget.HighImportance

		discardChanges := widget.NewButtonWithIcon("Discard Changes", theme.DeleteIcon(), func() {
			d.Hide()
			cleanupPlayback()
			state.Result = state.SVGData
			state.Applied = false
			w.Close()
			a.Quit()
		})
		discardChanges.Importance = widget.DangerImportance

		keepEditing := widget.NewButton("Keep Editing", func() {
			d.Hide()
		})

		dialogButtons := container.NewHBox(
			layout.NewSpacer(),
			keepEditing,
			discardChanges,
			applyAndClose,
		)

		dialogContent := container.NewVBox(
			widget.NewLabel(message),
			widget.NewSeparator(),
			dialogButtons,
		)

		d = dialog.NewCustomWithoutButtons("Unapplied Motion Changes", dialogContent, w)
		d.Show()
	}

	w.SetCloseIntercept(handleCloseRequest)

	// Bottom action buttons
	applyBtn := widget.NewButtonWithIcon("Apply Changes", theme.ConfirmIcon(), func() {
		cleanupPlayback()
		state.Result = state.ComputePatchedSVG()
		state.Applied = true
		w.Close()
		a.Quit()
	})
	applyBtn.Importance = widget.HighImportance

	cancelBtn := widget.NewButtonWithIcon("Cancel", theme.CancelIcon(), func() {
		handleCloseRequest()
	})

	bottomBar := container.NewHBox(
		statusLabel,
		layout.NewSpacer(),
		cancelBtn,
		applyBtn,
	)

	filePathLabel := widget.NewLabel("SVG: " + state.InputPath)
	filePathLabel.Truncation = fyne.TextTruncateClip

	rootContent := container.NewBorder(
		container.NewVBox(filePathLabel, widget.NewSeparator()),
		container.NewVBox(widget.NewSeparator(), bottomBar),
		nil,
		nil,
		mainSplit,
	)

	w.SetContent(container.NewPadded(rootContent))
	tree.OpenAllBranches()
	if state.ActiveNode != nil {
		tree.Select(state.ActiveNode.ID)
	}
	refreshInspector()
	w.Show()
	return w
}

func buildDirectiveWidgetCard(
	dir *doctree.Directive,
	totalDocFrames int,
	onModified func(),
	onDelete func(),
) fyne.CanvasObject {
	m := params.Parse(dir.Type, dir.Params, totalDocFrames)

	syncToDir := func() {
		dir.Params = m.Format()
		dir.Raw = fmt.Sprintf("%s {%s}", dir.Type, dir.Params)
		onModified()
	}

	rawPreviewLabel := widget.NewLabelWithStyle(dir.Raw, fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})
	rawPreviewLabel.Truncation = fyne.TextTruncateClip

	syncAndPreview := func() {
		syncToDir()
		rawPreviewLabel.SetText(dir.Raw)
	}

	// 1. Header: Directive Type Title & Delete Button
	typeTitle := widget.NewLabelWithStyle(dir.Type, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	delBtn := widget.NewButtonWithIcon("", theme.DeleteIcon(), onDelete)
	delBtn.Importance = widget.DangerImportance

	headerRow := container.NewBorder(nil, nil, nil, delBtn, typeTitle)

	// 2. Timing Controls (Frames)
	startEntry := widget.NewEntry()
	startEntry.SetText(strconv.Itoa(m.StartFrame))

	endEntry := widget.NewEntry()
	endEntry.SetText(strconv.Itoa(m.EndFrame))

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
		syncAndPreview()
	}

	startEntry.OnChanged = func(s string) {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			m.StartFrame = v
			syncAndPreview()
		}
	}
	endEntry.OnChanged = func(s string) {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			m.EndFrame = v
			syncAndPreview()
		}
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
		syncAndPreview()
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
			syncAndPreview()
		}
	})
	if m.Repeat > 1 {
		repeatSelect.SetSelected(fmt.Sprintf("%dx", m.Repeat))
	} else {
		repeatSelect.SetSelected("1x (Once)")
	}

	pingpongCheck := widget.NewCheck("Ping-Pong", func(checked bool) {
		m.PingPong = checked
		syncAndPreview()
	})
	pingpongCheck.SetChecked(m.PingPong)

	revCheck := widget.NewCheck("Reverse", func(checked bool) {
		m.Reverse = checked
		syncAndPreview()
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
		degEntry := widget.NewEntry()
		degVal := m.Angle
		if !m.HasAngle {
			degVal = 360
		}
		degEntry.SetText(fmt.Sprintf("%g", degVal))
		degEntry.OnChanged = func(s string) {
			if v, err := strconv.ParseFloat(s, 64); err == nil {
				m.Angle = v
				m.HasAngle = true
				syncAndPreview()
			}
		}

		dirSelect := widget.NewSelect([]string{"Clockwise (cw)", "Counter-Clockwise (ccw)"}, func(sel string) {
			if strings.Contains(sel, "ccw") {
				m.RotDir = "ccw"
			} else {
				m.RotDir = "cw"
			}
			syncAndPreview()
		})
		if m.RotDir == "ccw" {
			dirSelect.SetSelected("Counter-Clockwise (ccw)")
		} else {
			dirSelect.SetSelected("Clockwise (cw)")
		}

		pivotSelect := widget.NewSelect([]string{"center", "top", "bottom", "left", "right", "0", "90", "180", "270"}, func(sel string) {
			m.Pivot = sel
			syncAndPreview()
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
		fromEntry := widget.NewEntry()
		fromEntry.SetText(fmt.Sprintf("%g", m.ScaleFrom))
		fromEntry.OnChanged = func(s string) {
			if v, err := strconv.ParseFloat(s, 64); err == nil {
				m.ScaleFrom = v
				m.HasScale = true
				syncAndPreview()
			}
		}

		toEntry := widget.NewEntry()
		toEntry.SetText(fmt.Sprintf("%g", m.ScaleTo))
		toEntry.OnChanged = func(s string) {
			if v, err := strconv.ParseFloat(s, 64); err == nil {
				m.ScaleTo = v
				m.HasScale = true
				syncAndPreview()
			}
		}

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
			syncAndPreview()
		})
		orientCheck.SetChecked(m.Orient)

		pivotSelect := widget.NewSelect([]string{"center", "top", "bottom", "left", "right"}, func(sel string) {
			m.Pivot = sel
			syncAndPreview()
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
		fromEntry := widget.NewEntry()
		fromEntry.SetText(fmt.Sprintf("%g", m.OpacityFrom))
		fromEntry.OnChanged = func(s string) {
			if v, err := strconv.ParseFloat(s, 64); err == nil {
				m.OpacityFrom = v
				m.HasOpacity = true
				syncAndPreview()
			}
		}

		toEntry := widget.NewEntry()
		toEntry.SetText(fmt.Sprintf("%g", m.OpacityTo))
		toEntry.OnChanged = func(s string) {
			if v, err := strconv.ParseFloat(s, 64); err == nil {
				m.OpacityTo = v
				m.HasOpacity = true
				syncAndPreview()
			}
		}

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
			syncAndPreview()
		})
		if m.ColorTarget != "" {
			targetSelect.SetSelected(m.ColorTarget)
		} else {
			targetSelect.SetSelected("fill")
		}

		degEntry := widget.NewEntry()
		degEntry.SetText(fmt.Sprintf("%g", m.ColorAngle))
		degEntry.OnChanged = func(s string) {
			if v, err := strconv.ParseFloat(s, 64); err == nil {
				m.ColorAngle = v
				m.HasColorAngle = true
				syncAndPreview()
			}
		}

		colorRow := container.NewHBox(
			widget.NewLabel("Target:"),
			targetSelect,
			widget.NewLabel("Degrees (°):"),
			degEntry,
		)
		typeProps.Add(colorRow)

	case "Dist":
		factorEntry := widget.NewEntry()
		factorEntry.SetText(fmt.Sprintf("%g", m.ParallaxFactor))
		factorEntry.OnChanged = func(s string) {
			if v, err := strconv.ParseFloat(s, 64); err == nil {
				m.ParallaxFactor = v
				m.HasParallax = true
				syncAndPreview()
			}
		}

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
			syncAndPreview()
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
		orderEntry := widget.NewEntry()
		orderEntry.SetText(strconv.Itoa(m.DepthOffset))
		orderEntry.OnChanged = func(s string) {
			if v, err := strconv.Atoi(s); err == nil {
				m.DepthOffset = v
				syncAndPreview()
			}
		}

		depthRow := container.NewHBox(
			widget.NewLabel("Z-Order Offset:"),
			orderEntry,
		)
		typeProps.Add(depthRow)
	}

	// Directive Card Container
	cardItems := []fyne.CanvasObject{
		headerRow,
	}

	// Frames row: on all types except static parallax "Dist"
	if m.Type != "Dist" {
		cardItems = append(cardItems, framesRow)
	}

	// Playback row (ease, repeat, ping-pong, reverse): only on continuous animations
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

