package ui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"inkanim/internal/ext/doctree"
	"inkanim/internal/ext/presets"
	"inkanim/pkg/inksvg"
)

// setupLayersAndMotionTab builds the split view containing the Objects hierarchy tree on the left
// and the element/motion inspector on the right.
func setupLayersAndMotionTab(mw *MainWindow) fyne.CanvasObject {
	w := mw.window
	a := fyne.CurrentApp()

	// 1. Undo / Redo Toolbar Buttons
	mw.undoBtn = widget.NewButtonWithIcon("", theme.ContentUndoIcon(), func() { mw.doUndo() })
	mw.undoBtn.Importance = widget.LowImportance

	mw.redoBtn = widget.NewButtonWithIcon("", theme.ContentRedoIcon(), func() { mw.doRedo() })
	mw.redoBtn.Importance = widget.LowImportance

	mw.undoItem = fyne.NewMenuItem("Undo", func() { mw.doUndo() })
	mw.undoItem.Shortcut = &fyne.ShortcutUndo{}
	mw.undoItem.Icon = theme.ContentUndoIcon()

	mw.redoItem = fyne.NewMenuItem("Redo", func() { mw.doRedo() })
	mw.redoItem.Shortcut = &fyne.ShortcutRedo{}
	mw.redoItem.Icon = theme.ContentRedoIcon()

	mw.copyItem = fyne.NewMenuItem("Copy Motions", func() { mw.doCopy() })
	mw.copyItem.Shortcut = &fyne.ShortcutCopy{}
	mw.copyItem.Icon = theme.ContentCopyIcon()

	mw.pasteItem = fyne.NewMenuItem("Paste Motions", func() { mw.doPaste() })
	mw.pasteItem.Shortcut = &fyne.ShortcutPaste{}
	mw.pasteItem.Icon = theme.ContentPasteIcon()

	mw.deleteItem = fyne.NewMenuItem("Delete Element", func() { mw.doDeleteActiveElement() })
	mw.deleteItem.Icon = theme.DeleteIcon()
	mw.deleteItem.Shortcut = &desktop.CustomShortcut{KeyName: fyne.KeyDelete}

	mw.updateUndoRedo = func() {
		state := mw.editorState
		if state == nil {
			mw.undoBtn.Disable()
			mw.redoBtn.Disable()
			mw.undoItem.Disabled = true
			mw.redoItem.Disabled = true
			mw.copyItem.Disabled = true
			mw.pasteItem.Disabled = true
			mw.deleteItem.Disabled = true
			if mw.mainMenu != nil {
				mw.mainMenu.Refresh()
			}
			return
		}

		canUndo := state.CanUndo()
		canRedo := state.CanRedo()
		if canUndo {
			mw.undoBtn.Enable()
			mw.undoItem.Disabled = false
		} else {
			mw.undoBtn.Disable()
			mw.undoItem.Disabled = true
		}
		if canRedo {
			mw.redoBtn.Enable()
			mw.redoItem.Disabled = false
		} else {
			mw.redoBtn.Disable()
			mw.redoItem.Disabled = true
		}

		canCopy := state.ActiveNode != nil && len(state.ActiveNode.Directives) > 0
		mw.copyItem.Disabled = !canCopy

		clipHasText := state.ClipboardDirectives != ""
		if a != nil && a.Clipboard() != nil && strings.TrimSpace(a.Clipboard().Content()) != "" {
			clipHasText = true
		}
		mw.pasteItem.Disabled = state.ActiveNode == nil || !clipHasText

		var canDelete bool
		if state.ActiveNode != nil {
			canDelete, _ = state.CanDeleteElement(state.ActiveNode.ID)
		}
		mw.deleteItem.Disabled = !canDelete

		if mw.mainMenu != nil {
			mw.mainMenu.Refresh()
		}
	}

	mw.doUndo = func() {
		state := mw.editorState
		if state == nil || !state.CanUndo() {
			return
		}
		desc, err := state.Undo()
		if err != nil {
			return
		}
		mw.lastHistoryMsg = "Undid: " + desc
		mw.rebuildAfterHistoryChange()
	}

	mw.doRedo = func() {
		state := mw.editorState
		if state == nil || !state.CanRedo() {
			return
		}
		desc, err := state.Redo()
		if err != nil {
			return
		}
		mw.lastHistoryMsg = "Redid: " + desc
		mw.rebuildAfterHistoryChange()
	}

	mw.doCopy = func() {
		state := mw.editorState
		if state == nil || state.ActiveNode == nil || len(state.ActiveNode.Directives) == 0 {
			return
		}
		if _, isEntry := w.Canvas().Focused().(*editorCommitEntry); isEntry {
			return
		}
		if _, isEntry := w.Canvas().Focused().(*widget.Entry); isEntry {
			return
		}
		text := state.CopyMotions(state.ActiveNode)
		if a != nil && a.Clipboard() != nil {
			a.Clipboard().SetContent(text)
		}
		mw.statusLabel.SetText(fmt.Sprintf("Copied %d motion directive(s) to clipboard", len(state.ActiveNode.Directives)))
		mw.updateUndoRedo()
	}

	mw.doPaste = func() {
		state := mw.editorState
		if state == nil || state.ActiveNode == nil {
			return
		}
		if _, isEntry := w.Canvas().Focused().(*editorCommitEntry); isEntry {
			return
		}
		if _, isEntry := w.Canvas().Focused().(*widget.Entry); isEntry {
			return
		}
		clipText := ""
		if a != nil && a.Clipboard() != nil {
			clipText = a.Clipboard().Content()
		}
		if strings.TrimSpace(clipText) == "" {
			clipText = state.ClipboardDirectives
		}
		if strings.TrimSpace(clipText) == "" {
			mw.statusLabel.SetText("Clipboard is empty")
			return
		}
		resultingID, err := state.PasteMotions(clipText)
		if err != nil {
			mw.statusLabel.SetText("Paste error: " + err.Error())
			return
		}
		mw.lastHistoryMsg = ""
		mw.rebuildAfterHistoryChange()
		if mw.tree != nil && resultingID != "" {
			if n := state.NodeMap[resultingID]; n != nil && n.ParentID != "" {
				mw.tree.OpenBranch(n.ParentID)
			}
			mw.tree.Select(resultingID)
		}
	}

	mw.doDeleteActiveElement = func() {
		state := mw.editorState
		if state == nil || state.ActiveNode == nil {
			return
		}
		if _, isEntry := w.Canvas().Focused().(*editorCommitEntry); isEntry {
			return
		}
		if _, isEntry := w.Canvas().Focused().(*widget.Entry); isEntry {
			return
		}
		nodeID := state.ActiveNode.ID
		canDelete, _ := state.CanDeleteElement(nodeID)
		if !canDelete {
			return
		}
		if err := state.DeleteElement(nodeID); err != nil {
			mw.statusLabel.SetText("Delete error: " + err.Error())
			return
		}
		mw.lastHistoryMsg = ""
		mw.rebuildAfterHistoryChange()
	}

	// 2. Keyboard Shortcuts on Window Canvas
	w.Canvas().AddShortcut(&fyne.ShortcutUndo{}, func(_ fyne.Shortcut) { mw.doUndo() })
	w.Canvas().AddShortcut(&fyne.ShortcutRedo{}, func(_ fyne.Shortcut) { mw.doRedo() })
	w.Canvas().AddShortcut(&desktop.CustomShortcut{
		KeyName:  fyne.KeyZ,
		Modifier: fyne.KeyModifierControl,
	}, func(_ fyne.Shortcut) { mw.doUndo() })
	w.Canvas().AddShortcut(&desktop.CustomShortcut{
		KeyName:  fyne.KeyZ,
		Modifier: fyne.KeyModifierControl | fyne.KeyModifierShift,
	}, func(_ fyne.Shortcut) { mw.doRedo() })
	w.Canvas().AddShortcut(&desktop.CustomShortcut{
		KeyName:  fyne.KeyY,
		Modifier: fyne.KeyModifierControl,
	}, func(_ fyne.Shortcut) { mw.doRedo() })
	w.Canvas().AddShortcut(&fyne.ShortcutCopy{}, func(_ fyne.Shortcut) { mw.doCopy() })
	w.Canvas().AddShortcut(&fyne.ShortcutPaste{}, func(_ fyne.Shortcut) { mw.doPaste() })
	w.Canvas().AddShortcut(&desktop.CustomShortcut{
		KeyName:  fyne.KeyC,
		Modifier: fyne.KeyModifierControl,
	}, func(_ fyne.Shortcut) { mw.doCopy() })
	w.Canvas().AddShortcut(&desktop.CustomShortcut{
		KeyName:  fyne.KeyV,
		Modifier: fyne.KeyModifierControl,
	}, func(_ fyne.Shortcut) { mw.doPaste() })
	w.Canvas().AddShortcut(&desktop.CustomShortcut{
		KeyName: fyne.KeyDelete,
	}, func(_ fyne.Shortcut) { mw.doDeleteActiveElement() })
	w.Canvas().SetOnTypedKey(func(ev *fyne.KeyEvent) {
		if ev.Name == fyne.KeyDelete {
			mw.doDeleteActiveElement()
		}
	})

	// 3. Inspector Card
	mw.inspectorCard = container.NewStack()

	mw.refreshInspectorFn = func() {
		state := mw.editorState
		if state == nil || state.ActiveNode == nil {
			emptyNotice := container.NewCenter(widget.NewLabel("Select an object from the document tree to view and edit its motions."))
			mw.inspectorCard.Objects = []fyne.CanvasObject{emptyNotice}
			mw.inspectorCard.Refresh()
			mw.updateStatus()
			mw.updateUndoRedo()
			return
		}

		node := state.ActiveNode

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

		canDelete, deleteReason := state.CanDeleteElement(node.ID)
		deleteBtn := widget.NewButtonWithIcon("Delete", theme.DeleteIcon(), mw.doDeleteActiveElement)
		deleteBtn.Importance = widget.DangerImportance
		if !canDelete {
			deleteBtn.Disable()
		}

		headerTop := container.NewBorder(
			nil, nil,
			titleLabel,
			deleteBtn,
		)

		var headerBox *fyne.Container
		if !canDelete && deleteReason != "" {
			reasonLabel := widget.NewLabelWithStyle(deleteReason, fyne.TextAlignLeading, fyne.TextStyle{Italic: true})
			headerBox = container.NewVBox(
				headerTop,
				tagLabel,
				reasonLabel,
				widget.NewSeparator(),
			)
		} else {
			headerBox = container.NewVBox(
				headerTop,
				tagLabel,
				widget.NewSeparator(),
			)
		}

		updatePreview := func() {}

		// Label prefix entry (e.g. "Fade In: ")
		prefixEntry := newEditorCommitEntry(
			node.LabelPrefix,
			func(val string) {
				node.LabelPrefix = val
				state.ModifiedIDs[node.ID] = true
				updatePreview()
				mw.updateStatus()
				mw.updateFileLabel()
				mw.refreshTreeFn()
				mw.schedulePreviewUpdate()
			},
			func(val string) {
				state.RecordChange(fmt.Sprintf("Rename %s prefix to '%s'", node.DisplayTitle(), val))
				mw.updateUndoRedo()
				mw.updateStatus()
				mw.updateFileLabel()
			},
			mw.doUndo,
			mw.doRedo,
		)
		prefixEntry.SetPlaceHolder("Descriptive name or prefix (optional)")

		// Directives list container
		directivesList := container.NewVBox()

		var rebuildDirectivesList func()
		rebuildDirectivesList = func() {
			directivesList.Objects = nil
			if len(node.Directives) == 0 {
				directivesList.Add(widget.NewLabel("No motion directives configured on this element."))
			} else {
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
						mw.updateStatus()
						mw.updateFileLabel()
						mw.refreshTreeFn()
						mw.schedulePreviewUpdate()
					}

					onCommit := func(desc string) {
						state.ModifiedIDs[node.ID] = true
						state.RecordChange(fmt.Sprintf("%s on %s", desc, node.DisplayTitle()))
						mw.updateUndoRedo()
						mw.updateStatus()
						mw.updateFileLabel()
					}

					onDelete := func() {
						deletedType := dir.Type
						node.Directives = append(node.Directives[:idx], node.Directives[idx+1:]...)
						state.ModifiedIDs[node.ID] = true
						state.RecordChange(fmt.Sprintf("Delete %s from %s", deletedType, node.DisplayTitle()))
						rebuildDirectivesList()
						updatePreview()
						mw.updateUndoRedo()
						mw.updateStatus()
						mw.updateFileLabel()
						mw.refreshTreeFn()
						mw.schedulePreviewUpdate()
					}

					onCopy := func() {
						text := state.CopySingleDirective(*dir)
						if a != nil && a.Clipboard() != nil {
							a.Clipboard().SetContent(text)
						}
						mw.statusLabel.SetText(fmt.Sprintf("Copied %s directive to clipboard", dir.Type))
						mw.updateUndoRedo()
					}

					card := buildDirectiveWidgetCard(dir, totalDocFrames, onModified, onCommit, onCopy, onDelete, mw.doUndo, mw.doRedo)
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
			state.RecordChange(fmt.Sprintf("Add %s to %s", mType, node.DisplayTitle()))
			mw.updateUndoRedo()
			rebuildDirectivesList()
			updatePreview()
			mw.updateStatus()
			mw.updateFileLabel()
			mw.refreshTreeFn()
			mw.schedulePreviewUpdate()
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
		presetDescLabel.Wrapping = fyne.TextWrapWord

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
			mw.updateUndoRedo()
			updatePreview()
			mw.updateStatus()
			mw.updateFileLabel()
			mw.refreshTreeFn()
			if mw.tree != nil {
				if node.IsGroup || node.IsLayer {
					mw.tree.OpenBranch(node.ID)
				} else if node.ParentID != "" {
					mw.tree.OpenBranch(node.ParentID)
				}
				mw.tree.Select(anchorID)
			}
			mw.refreshInspectorFn()
			mw.schedulePreviewUpdate()
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

		updatePreview = func() {}

		copyAllBtn := widget.NewButtonWithIcon("Copy All", theme.ContentCopyIcon(), mw.doCopy)
		copyAllBtn.Importance = widget.LowImportance
		if len(node.Directives) == 0 {
			copyAllBtn.Disable()
		}

		pasteBtn := widget.NewButtonWithIcon("Paste", theme.ContentPasteIcon(), mw.doPaste)
		pasteBtn.Importance = widget.LowImportance
		clipHasText := state.ClipboardDirectives != ""
		if a != nil && a.Clipboard() != nil && strings.TrimSpace(a.Clipboard().Content()) != "" {
			clipHasText = true
		}
		if !clipHasText {
			pasteBtn.Disable()
		}

		dirHeader := container.NewBorder(
			nil, nil,
			widget.NewLabelWithStyle("Motion Directives", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			container.NewHBox(copyAllBtn, pasteBtn),
		)

		content := container.NewVBox(
			headerBox,
			widget.NewLabelWithStyle("Motion Presets", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			presetBox,
			widget.NewSeparator(),
			widget.NewLabelWithStyle("Object Name / Label Prefix", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			prefixEntry,
			widget.NewSeparator(),
			dirHeader,
			directivesList,
			widget.NewSeparator(),
			addBar,
		)

		scrollContent := container.NewVScroll(content)
		mw.inspectorCard.Objects = []fyne.CanvasObject{scrollContent}
		mw.inspectorCard.Refresh()
		mw.updateStatus()
		mw.updateUndoRedo()
	}

	// 4. Build Tree Widget
	mw.tree = newDeletableTree(
		func(uid string) []string {
			state := mw.editorState
			if state == nil {
				return nil
			}
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
			state := mw.editorState
			if state == nil {
				return false
			}
			if uid == "" {
				return len(state.Roots) > 0
			}
			node := state.NodeMap[uid]
			return node != nil && len(node.Children) > 0
		},
		func(branch bool) fyne.CanvasObject {
			icon := widget.NewIcon(theme.DocumentIcon())
			iconWrap := container.NewGridWrap(fyne.NewSize(16, 16), icon)
			txt := canvas.NewText("node template", theme.Color(theme.ColorNameForeground))
			txt.TextSize = theme.Size(theme.SizeNameText)
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
			txt.TextSize = theme.Size(theme.SizeNameText)

			state := mw.editorState
			if state == nil {
				txt.Text = uid
				txt.Refresh()
				return
			}
			node := state.NodeMap[uid]
			if node == nil {
				txt.Text = uid
				txt.Refresh()
				return
			}

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
				displayText = fmt.Sprintf("* %s [%s]", displayText, strings.Join(dirTypes, ", "))
				txt.TextStyle = fyne.TextStyle{Bold: true}
			} else {
				txt.TextStyle = fyne.TextStyle{}
			}

			txt.Text = displayText
			txt.Color = theme.Color(theme.ColorNameForeground)
			txt.Refresh()
		},
		mw.doDeleteActiveElement,
	)

	mw.tree.OnSelected = func(uid string) {
		if mw.editorState != nil {
			if node, ok := mw.editorState.NodeMap[uid]; ok {
				mw.editorState.ActiveNode = node
				mw.refreshInspectorFn()
			}
		}
	}

	mw.refreshTreeFn = func() {
		if mw.tree != nil {
			mw.tree.Refresh()
		}
	}

	treeHeader := container.NewHBox(
		widget.NewLabelWithStyle("Objects", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		layout.NewSpacer(),
		mw.undoBtn,
		mw.redoBtn,
	)
	treePanel := container.NewBorder(
		container.NewVBox(treeHeader, widget.NewSeparator()),
		nil, nil, nil,
		mw.tree,
	)

	motionSplit := container.NewHSplit(treePanel, mw.inspectorCard)
	motionSplit.SetOffset(0.35)

	mw.updateUndoRedo()
	mw.refreshInspectorFn()

	return motionSplit
}

// ShowEditorMainWindow is an exported entry point to run the studio window in extension mode,
// returning the *MainWindow for lifecycle management and testing.
func ShowEditorMainWindow(a fyne.App, state *EditorState) *MainWindow {
	mw := NewMainWindow(a)
	mw.EnableExtensionMode()
	mw.editorState = state
	mw.currentPath = state.InputPath
	mw.currentFilename = filepath.Base(state.InputPath)
	mw.loadData(state.SVGData, mw.currentFilename)
	mw.window.Show()
	return mw
}

// ShowEditorWindow is an exported entry point to run the studio window in extension mode.
func ShowEditorWindow(a fyne.App, state *EditorState) fyne.Window {
	return ShowEditorMainWindow(a, state).window
}

// rebuildAfterHistoryChange restores UI after undo/redo/paste/delete.
func (mw *MainWindow) rebuildAfterHistoryChange() {
	mw.updateUndoRedo()
	if mw.refreshTreeFn != nil {
		mw.refreshTreeFn()
	}
	if mw.editorState != nil && mw.editorState.ActiveNode != nil && mw.tree != nil {
		if mw.editorState.ActiveNode.ParentID != "" {
			mw.tree.OpenBranch(mw.editorState.ActiveNode.ParentID)
		}
		mw.tree.Select(mw.editorState.ActiveNode.ID)
	}
	if mw.refreshInspectorFn != nil {
		mw.refreshInspectorFn()
	}
	mw.schedulePreviewUpdate()
	mw.updateStatus()
	mw.updateFileLabel()
}

// schedulePreviewUpdate debounces live preview re-rendering when motion parameters change.
func (mw *MainWindow) schedulePreviewUpdate() {
	mw.previewMu.Lock()
	if mw.closed {
		mw.previewMu.Unlock()
		return
	}
	if mw.previewTimer != nil {
		mw.previewTimer.Stop()
	}
	mw.previewTimer = time.AfterFunc(200*time.Millisecond, func() {
		mw.previewMu.Lock()
		if mw.closed || mw.editorState == nil || mw.session == nil {
			mw.previewMu.Unlock()
			return
		}
		state := mw.editorState
		sess := mw.session
		filename := mw.currentFilename
		mw.previewMu.Unlock()

		patched := state.ComputePatchedSVG()
		fyne.Do(func() {
			mw.previewMu.Lock()
			if mw.closed {
				mw.previewMu.Unlock()
				return
			}
			mw.previewMu.Unlock()

			if err := sess.LoadSVGData(patched, filename); err == nil {
				if mw.centerPanel != nil {
					mw.centerPanel.Refresh()
					if len(sess.RenderedFrames) > 1 && !mw.centerPanel.IsPlaying() {
						mw.centerPanel.Play()
					}
				}
				if mw.leftPanel != nil {
					mw.leftPanel.Refresh()
				}
				if mw.rightPanel != nil {
					mw.rightPanel.syncOptions()
				}
			}
		})
	})
	mw.previewMu.Unlock()
}

// updateStatus updates the bottom status bar with the active object and change summary.
func (mw *MainWindow) updateStatus() {
	state := mw.editorState
	if state == nil {
		mw.statusLabel.SetText("Ready. Open an Inkscape SVG to begin.")
		return
	}

	var activeName string
	if state.ActiveNode != nil {
		activeName = state.ActiveNode.DisplayTitle()
	} else {
		activeName = "None"
	}

	var changeStatus string
	if state.IsDirty() {
		count := state.HistoryIndex + 1
		switch {
		case count == 1:
			changeStatus = fmt.Sprintf("Unapplied changes (%s)", state.History[state.HistoryIndex].Description)
		case count > 1:
			changeStatus = fmt.Sprintf("Unapplied changes (%d changes, latest: %s)", count, state.History[state.HistoryIndex].Description)
		default:
			changeStatus = "Unapplied changes"
		}
	} else {
		changeStatus = "No changes"
	}

	if mw.lastHistoryMsg != "" {
		changeStatus = fmt.Sprintf("%s (%s)", mw.lastHistoryMsg, changeStatus)
		mw.lastHistoryMsg = ""
	}
	mw.statusLabel.SetText(fmt.Sprintf("Active Object: %s | %s", activeName, changeStatus))
}

// updateFileLabel updates the top header filename with dimension and an asterisk if dirty.
func (mw *MainWindow) updateFileLabel() {
	if mw.currentFilename == "" {
		mw.fileLabel.SetText("No file loaded")
		return
	}
	dirtyMarker := ""
	if mw.editorState != nil && mw.editorState.IsDirty() {
		dirtyMarker = " *"
	}
	if mw.session != nil && mw.session.Document != nil {
		mw.fileLabel.SetText(fmt.Sprintf("%s (%0.0fx%0.0f)%s", mw.currentFilename, mw.session.Document.Width, mw.session.Document.Height, dirtyMarker))
	} else {
		mw.fileLabel.SetText(fmt.Sprintf("%s%s", mw.currentFilename, dirtyMarker))
	}
}

// handleCloseRequest prompts the user when closing with unapplied or unsaved changes.
func (mw *MainWindow) handleCloseRequest() {
	if mw.editorState == nil || !mw.editorState.IsDirty() {
		if mw.isExtMode && mw.editorState != nil {
			mw.editorState.Result = mw.editorState.OriginalSVG
			mw.editorState.Applied = false
		}
		mw.window.Close()
		return
	}

	state := mw.editorState
	count := state.HistoryIndex + 1

	if mw.isExtMode {
		var message string
		switch {
		case count == 1:
			message = fmt.Sprintf("You have unapplied motion changes (%s).\n\nWould you like to apply your changes to your Inkscape document before closing?", state.History[state.HistoryIndex].Description)
		case count > 1:
			message = fmt.Sprintf("You have %d unapplied motion changes (latest: %s).\n\nWould you like to apply your changes to your Inkscape document before closing?", count, state.History[state.HistoryIndex].Description)
		default:
			message = "You have unapplied motion changes.\n\nWould you like to apply your changes to your Inkscape document before closing?"
		}

		var d *dialog.CustomDialog
		applyAndClose := widget.NewButtonWithIcon("Apply & Close", theme.ConfirmIcon(), func() {
			d.Hide()
			state.Result = state.ComputePatchedSVG()
			state.Applied = true
			mw.window.Close()
		})
		applyAndClose.Importance = widget.HighImportance

		discardChanges := widget.NewButtonWithIcon("Discard Changes", theme.DeleteIcon(), func() {
			d.Hide()
			state.Result = state.OriginalSVG
			state.Applied = false
			mw.window.Close()
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
			dialogButtons,
		)

		d = dialog.NewCustomWithoutButtons("Unapplied Changes", dialogContent, mw.window)
		d.Show()
		return
	}

	// Standalone Desktop or Web
	message := fmt.Sprintf("You have %d unsaved change(s). Would you like to save before closing?", count)
	if count == 1 {
		message = fmt.Sprintf("You have unsaved changes (%s). Would you like to save before closing?", state.History[state.HistoryIndex].Description)
	}

	var d *dialog.CustomDialog
	saveAndClose := widget.NewButtonWithIcon("Save & Close", theme.DocumentSaveIcon(), func() {
		d.Hide()
		mw.Save()
		mw.window.Close()
	})
	saveAndClose.Importance = widget.HighImportance

	discardAndClose := widget.NewButtonWithIcon("Discard & Close", theme.DeleteIcon(), func() {
		d.Hide()
		mw.window.Close()
	})
	discardAndClose.Importance = widget.DangerImportance

	cancelBtn := widget.NewButton("Cancel", func() {
		d.Hide()
	})

	buttons := container.NewHBox(
		layout.NewSpacer(),
		cancelBtn,
		discardAndClose,
		saveAndClose,
	)

	dialogContent := container.NewVBox(
		widget.NewLabel(message),
		buttons,
	)

	d = dialog.NewCustomWithoutButtons("Unsaved Changes", dialogContent, mw.window)
	d.Show()
}
