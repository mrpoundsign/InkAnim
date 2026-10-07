package main

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"inkanim/internal/app"
	"inkanim/internal/ext/doctree"
	"inkanim/internal/ext/presets"
	"inkanim/internal/ext/svgpatch"
	"inkanim/internal/ui"
	"inkanim/pkg/inksvg"
)

var motionTypeDefaults = map[string]string{
	"Move":  "f: 1-20; ease: in-out",
	"Rot":   "f: 1-20; angle: 360",
	"Scale": "f: 1-20; from: 1.0; to: 0.5",
	"Fade":  "f: 1-20; from: 0; to: 1",
	"Show":  "f: 1-10",
	"Hide":  "f: 1-10",
	"Depth": "order: 1",
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
			if len(doc.Layers) > 0 {
				totalFrames = len(doc.Layers)
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
	if doc, err := inksvg.ParseSVG(s.ComputePatchedSVG()); err == nil && len(doc.Layers) > 0 {
		totalFrames = len(doc.Layers)
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

	return &EditorState{
		Roots:       roots,
		NodeMap:     nodeMap,
		ActiveNode:  active,
		ModifiedIDs: make(map[string]bool),
		SelectedIDs: selectedIDs,
		InputPath:   inputPath,
		SVGData:     data,
		Result:      data,
	}, nil
}

// ShowEditorWindow displays the interactive Motion Editor window with live preview.
func ShowEditorWindow(a fyne.App, state *EditorState) fyne.Window {
	w := a.NewWindow("InkAnim Motion Studio")
	w.Resize(fyne.NewSize(1150, 650))

	// Live Animation Preview Session
	sess := app.NewSession()
	_ = sess.LoadSVGData(state.SVGData, state.InputPath)
	preview := ui.NewCenterPreviewPanel(sess)
	preview.SetInspectorVisible(false)
	if len(sess.RenderedFrames) > 1 {
		preview.Play()
	}

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
				for i := range node.Directives {
					idx := i
					dir := &node.Directives[idx]

					typeSelect := widget.NewSelect(motionTypes, func(newType string) {
						if newType != dir.Type {
							dir.Type = newType
							if def, ok := motionTypeDefaults[newType]; ok {
								dir.Params = def
							}
							dir.Raw = fmt.Sprintf("%s {%s}", dir.Type, dir.Params)
							state.ModifiedIDs[node.ID] = true
							rebuildDirectivesList()
							updatePreview()
							updateStatus()
							refreshTree()
							schedulePreviewUpdate()
						}
					})
					typeSelect.SetSelected(dir.Type)

					paramsEntry := widget.NewEntry()
					paramsEntry.SetText(dir.Params)
					paramsEntry.OnChanged = func(val string) {
						dir.Params = val
						dir.Raw = fmt.Sprintf("%s {%s}", dir.Type, dir.Params)
						state.ModifiedIDs[node.ID] = true
						updatePreview()
						updateStatus()
						refreshTree()
						schedulePreviewUpdate()
					}

					deleteBtn := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
						node.Directives = append(node.Directives[:idx], node.Directives[idx+1:]...)
						state.ModifiedIDs[node.ID] = true
						rebuildDirectivesList()
						updatePreview()
						updateStatus()
						refreshTree()
						schedulePreviewUpdate()
					})
					deleteBtn.Importance = widget.DangerImportance

					row := container.NewBorder(nil, nil, typeSelect, deleteBtn, paramsEntry)
					directivesList.Add(row)
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
			return container.NewHBox(
				widget.NewIcon(theme.DocumentIcon()),
				widget.NewLabel("node template"),
			)
		},
		func(uid string, branch bool, obj fyne.CanvasObject) {
			box := obj.(*fyne.Container)
			icon := box.Objects[0].(*widget.Icon)
			lbl := box.Objects[1].(*widget.Label)

			node := state.NodeMap[uid]
			if node == nil {
				lbl.SetText(uid)
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
				lbl.TextStyle = fyne.TextStyle{Bold: true}
			} else {
				lbl.TextStyle = fyne.TextStyle{}
			}

			lbl.SetText(displayText)
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

	// 3-Pane Studio Layout: [Tree] | [Inspector] | [Live Canvas Preview]
	inspectorScroll := container.NewVScroll(inspectorCard)
	leftSplit := container.NewHSplit(tree, inspectorScroll)
	leftSplit.SetOffset(0.38)

	mainSplit := container.NewHSplit(leftSplit, preview.Container())
	mainSplit.SetOffset(0.55)

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
		cleanupPlayback()
		state.Result = state.SVGData
		state.Applied = false
		w.Close()
		a.Quit()
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
