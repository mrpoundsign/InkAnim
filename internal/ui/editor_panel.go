package ui

import (
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
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

	"inkanim/internal/app"
	"inkanim/internal/ext/doctree"
	"inkanim/internal/ext/presets"
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
		case node.IsLinkedFile:
			kind = "Linked SVG"
		case node.IsLayer:
			kind = "Layer"
		case node.IsGroup:
			kind = "Group"
		default:
			kind = strings.ToUpper(node.Tag)
		}

		titleLabel := widget.NewLabelWithStyle(fmt.Sprintf("%s: %s", kind, node.DisplayTitle()), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		tagLabel := widget.NewLabel(fmt.Sprintf("ID: %s • Tag: <%s>", node.ID, node.Tag))

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
				totalDocFrames := 20
				if mw.session != nil {
					if mw.session.Document != nil && mw.session.Document.FrameCount() > 0 {
						totalDocFrames = mw.session.Document.FrameCount()
					} else if len(mw.session.Frames) > 0 {
						totalDocFrames = len(mw.session.Frames)
					}
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

		inspectorItems := []fyne.CanvasObject{headerBox}
		if !mw.isExtMode && (node.IsLinkedFile || node.IsLayer || node.IsGroup) {
			inspectorItems = append(inspectorItems,
				mw.buildStagePropertiesCard(node),
				widget.NewSeparator(),
			)
		}
		inspectorItems = append(inspectorItems,
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
		content := container.NewVBox(inspectorItems...)

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
				var uids []string
				for _, r := range state.Roots {
					if mw.isVisibleTreeNode(r) {
						uids = append(uids, r.ID)
					}
				}
				return uids
			}
			node := state.NodeMap[uid]
			if node == nil {
				return nil
			}
			var uids []string
			for _, c := range node.Children {
				if mw.isVisibleTreeNode(c) {
					uids = append(uids, c.ID)
				}
			}
			return uids
		},
		func(uid string) bool {
			state := mw.editorState
			if state == nil {
				return false
			}
			if uid == "" {
				return slices.ContainsFunc(state.Roots, mw.isVisibleTreeNode)
			}
			node := state.NodeMap[uid]
			if node == nil {
				return false
			}
			return slices.ContainsFunc(node.Children, mw.isVisibleTreeNode)
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
			case node.IsLinkedFile:
				icon.SetResource(theme.FileImageIcon())
			case node.IsLayer:
				icon.SetResource(theme.FolderOpenIcon())
			case node.IsGroup:
				icon.SetResource(theme.FolderIcon())
			default:
				icon.SetResource(theme.DocumentIcon())
			}

			displayText := node.DisplayTitle()
			switch {
			case node.IsLinkedFile:
				displayText = "SVG: " + displayText
			case node.IsLayer:
				displayText = "Layer: " + displayText
			case node.IsGroup:
				displayText = "Group: " + displayText
			default:
				displayText = "<" + node.Tag + "> " + displayText
			}

			_, objCfg, hasObjCfg := mw.resolveNodeObjectConfig(node)
			hasStageOverrides := hasObjCfg && (objCfg.OverrideStartPos || objCfg.HasDisplayWindow || objCfg.ZOffset != 0)
			hasDirectives := node.HasDirectives() || (hasObjCfg && len(objCfg.Actions) > 0) || hasStageOverrides
			isModified := state.ModifiedIDs[node.ID]

			var badges []string
			for _, d := range node.Directives {
				badges = append(badges, d.Type)
			}
			if hasObjCfg {
				for _, a := range objCfg.Actions {
					badges = append(badges, strings.ToUpper(a.Type[:1])+a.Type[1:])
				}
				if hasStageOverrides {
					badges = append(badges, "Stage")
				}
			}
			if len(badges) > 0 {
				displayText = fmt.Sprintf("%s [%s]", displayText, strings.Join(badges, ", "))
			}

			txt.TextStyle = fyne.TextStyle{
				Bold:   hasDirectives,
				Italic: isModified,
			}

			txt.Text = displayText
			switch {
			case isModified:
				txt.Color = theme.Color(theme.ColorNameWarning)
			case hasDirectives:
				txt.Color = theme.Color(theme.ColorNamePrimary)
			default:
				txt.Color = theme.Color(theme.ColorNameForeground)
			}
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

	addSVGBtn := widget.NewButtonWithIcon("Add SVG", theme.ContentAddIcon(), func() {
		mw.promptAddSVG()
	})
	addSVGBtn.Importance = widget.LowImportance

	treeHeader := container.NewHBox(
		widget.NewLabelWithStyle("Objects", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		layout.NewSpacer(),
		addSVGBtn,
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
	mw.previewGen++
	gen := mw.previewGen
	mw.previewTimer = time.AfterFunc(200*time.Millisecond, func() {
		mw.previewMu.Lock()
		if mw.closed || mw.editorState == nil || mw.session == nil || mw.previewGen != gen {
			mw.previewMu.Unlock()
			return
		}
		state := mw.editorState
		sess := mw.session
		filename := mw.currentFilename
		mw.previewMu.Unlock()

		patched := state.ComputePatchedSVG()
		if err := sess.LoadSVGData(patched, filename); err != nil {
			return
		}

		dispatchUI(func() {
			mw.previewMu.Lock()
			if mw.closed || mw.previewGen != gen {
				mw.previewMu.Unlock()
				return
			}
			mw.previewMu.Unlock()

			if mw.centerPanel != nil {
				mw.centerPanel.Refresh()
				if len(sess.RenderedFrames) > 1 && !mw.centerPanel.IsPlaying() {
					mw.centerPanel.Play()
				}
			}
			if mw.rightPanel != nil {
				mw.rightPanel.Refresh()
			}
		})
	})
	mw.previewMu.Unlock()
}

func (mw *MainWindow) isVisibleTreeNode(n *doctree.DocNode) bool {
	if n == nil {
		return false
	}
	if mw.isExtMode {
		return true
	}
	return n.IsLinkedFile || n.IsLayer || n.IsGroup
}

func (mw *MainWindow) resolveNodeObjectConfig(n *doctree.DocNode) (string, app.ObjectConfig, bool) {
	if mw.session == nil || mw.session.Objects == nil || n == nil {
		return "", app.ObjectConfig{}, false
	}
	if cfg, ok := mw.session.Objects[n.ID]; ok {
		return n.ID, cfg, true
	}
	for k, cfg := range mw.session.Objects {
		if cfg.FallbackXMLID == n.ID {
			return k, cfg, true
		}
	}
	return n.ID, app.ObjectConfig{
		SourceID:      n.SourceID,
		FallbackXMLID: n.ID,
	}, false
}

func (mw *MainWindow) buildStagePropertiesCard(node *doctree.DocNode) fyne.CanvasObject {
	if mw.session.Objects == nil {
		mw.session.Objects = make(map[string]app.ObjectConfig)
	}
	objKey, objCfg, _ := mw.resolveNodeObjectConfig(node)
	if objKey == "" {
		objKey = node.ID
	}

	saveStageObj := func() {
		objCfg.SourceID = node.SourceID
		objCfg.FallbackXMLID = node.ID
		mw.session.Objects[objKey] = objCfg
		mw.projectDirty = true
		mw.updateStatus()
		mw.updateFileLabel()
		if mw.refreshTreeFn != nil {
			mw.refreshTreeFn()
		}
		mw.schedulePreviewUpdate()
	}

	var items []fyne.CanvasObject

	if node.IsLinkedFile && node.SourceID != "" {
		stripBtn := widget.NewButtonWithIcon("Strip Legacy IAMS", theme.ContentCutIcon(), func() {
			if err := mw.StripIAMSFromLinkedSVG(node.SourceID); err != nil {
				dialog.ShowError(err, mw.window)
			}
		})
		stripBtn.Importance = widget.LowImportance

		unlinkBtn := widget.NewButtonWithIcon("Unlink SVG", theme.DeleteIcon(), func() {
			if err := mw.RemoveLinkedSVG(node.SourceID); err != nil {
				dialog.ShowError(err, mw.window)
			}
		})
		unlinkBtn.Importance = widget.DangerImportance

		items = append(items,
			widget.NewLabelWithStyle("Linked Artwork File", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			container.NewHBox(stripBtn, unlinkBtn),
			widget.NewSeparator(),
		)
	}

	items = append(items, widget.NewLabelWithStyle("Stage Placement & Display Timing", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))

	// 1. Start Position Override
	startXEntry := newEditorCommitEntry(
		fmt.Sprintf("%g", objCfg.StartX),
		func(s string) {
			if v, err := strconv.ParseFloat(s, 64); err == nil {
				objCfg.StartX = v
				saveStageObj()
			}
		},
		nil,
		mw.doUndo,
		mw.doRedo,
	)
	startYEntry := newEditorCommitEntry(
		fmt.Sprintf("%g", objCfg.StartY),
		func(s string) {
			if v, err := strconv.ParseFloat(s, 64); err == nil {
				objCfg.StartY = v
				saveStageObj()
			}
		},
		nil,
		mw.doUndo,
		mw.doRedo,
	)

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

	startPosCheck := widget.NewCheck("Custom Start Position (override SVG page position)", nil)
	startPosCheck.Checked = objCfg.OverrideStartPos
	startPosCheck.OnChanged = func(checked bool) {
		objCfg.OverrideStartPos = checked
		if checked && objCfg.StartX == 0 && objCfg.StartY == 0 && mw.session != nil && mw.session.Document != nil {
			r := mw.session.Document.GetElementRect(node.ID)
			if r.Width > 0 || r.Height > 0 {
				objCfg.StartX = r.X + r.Width/2.0
				objCfg.StartY = r.Y + r.Height/2.0
				startXEntry.SetText(fmt.Sprintf("%.1f", objCfg.StartX))
				startYEntry.SetText(fmt.Sprintf("%.1f", objCfg.StartY))
			}
		}
		updateStartPosEnables(checked)
		saveStageObj()
	}

	startPosRow := container.NewHBox(
		widget.NewLabel("Start X:"),
		startXEntry,
		widget.NewLabel("Start Y:"),
		startYEntry,
	)

	// 2. Display Time Window (Seconds)
	totalDurSec := 1.0
	if mw.session != nil && mw.session.TotalDurationMs > 0 {
		totalDurSec = float64(mw.session.TotalDurationMs) / 1000.0
	}
	endSecInit := objCfg.EndTimeSec
	if endSecInit <= 0 {
		endSecInit = totalDurSec
	}

	startSecEntry := newEditorCommitEntry(
		fmt.Sprintf("%g", objCfg.StartTimeSec),
		func(s string) {
			if v, err := strconv.ParseFloat(s, 64); err == nil && v >= 0 {
				objCfg.StartTimeSec = v
				saveStageObj()
			}
		},
		nil,
		mw.doUndo,
		mw.doRedo,
	)
	endSecEntry := newEditorCommitEntry(
		fmt.Sprintf("%g", endSecInit),
		func(s string) {
			if v, err := strconv.ParseFloat(s, 64); err == nil && v >= 0 {
				objCfg.EndTimeSec = v
				saveStageObj()
			}
		},
		nil,
		mw.doUndo,
		mw.doRedo,
	)

	updateDisplayWinEnables := func(enabled bool) {
		if enabled {
			startSecEntry.Enable()
			endSecEntry.Enable()
		} else {
			startSecEntry.Disable()
			endSecEntry.Disable()
		}
	}
	updateDisplayWinEnables(objCfg.HasDisplayWindow)

	displayWinCheck := widget.NewCheck("Custom Display Time Window (seconds)", nil)
	displayWinCheck.Checked = objCfg.HasDisplayWindow
	displayWinCheck.OnChanged = func(checked bool) {
		objCfg.HasDisplayWindow = checked
		if checked && objCfg.EndTimeSec <= 0 {
			objCfg.EndTimeSec = totalDurSec
			endSecEntry.SetText(fmt.Sprintf("%g", objCfg.EndTimeSec))
		}
		updateDisplayWinEnables(checked)
		saveStageObj()
	}

	displayWinRow := container.NewHBox(
		widget.NewLabel("Appear (s):"),
		startSecEntry,
		widget.NewLabel("Hide (s):"),
		endSecEntry,
	)

	// 3. Z-Order & Pivot
	zOffsetEntry := newEditorCommitEntry(
		strconv.Itoa(objCfg.ZOffset),
		func(s string) {
			if v, err := strconv.Atoi(s); err == nil {
				objCfg.ZOffset = v
				saveStageObj()
			}
		},
		nil,
		mw.doUndo,
		mw.doRedo,
	)

	pivotSelect := widget.NewSelect([]string{"center", "top", "bottom", "left", "right"}, nil)
	if objCfg.PivotType != "" {
		pivotSelect.SetSelected(objCfg.PivotType)
	} else {
		pivotSelect.SetSelected("center")
	}
	pivotSelect.OnChanged = func(sel string) {
		objCfg.PivotType = sel
		saveStageObj()
	}

	zAndPivotRow := container.NewHBox(
		widget.NewLabel("Z-Offset:"),
		zOffsetEntry,
		widget.NewLabel("Pivot:"),
		pivotSelect,
	)

	items = append(items,
		startPosCheck,
		startPosRow,
		displayWinCheck,
		displayWinRow,
		zAndPivotRow,
	)

	return container.NewVBox(items...)
}

// updateStatus updates the bottom status bar with the active object and change summary.
func (mw *MainWindow) updateStatus() {
	state := mw.editorState
	if state == nil {
		mw.statusLabel.SetText("New project ready. Add an SVG or open a .inka project.")
		return
	}

	var activeName string
	if state.ActiveNode != nil {
		activeName = state.ActiveNode.DisplayTitle()
	} else {
		activeName = "None"
	}

	var changeStatus string
	switch {
	case state.IsDirty():
		count := state.HistoryIndex + 1
		switch {
		case count == 1:
			changeStatus = fmt.Sprintf("Unapplied changes (%s)", state.History[state.HistoryIndex].Description)
		case count > 1:
			changeStatus = fmt.Sprintf("Unapplied changes (%d changes, latest: %s)", count, state.History[state.HistoryIndex].Description)
		default:
			changeStatus = "Unapplied changes"
		}
	case mw.projectDirty:
		changeStatus = "Unsaved project changes"
	default:
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
	name := mw.currentFilename
	if !mw.isExtMode {
		name = mw.projectDisplayName()
	}
	if name == "" {
		mw.fileLabel.SetText("Untitled Project")
		return
	}
	dirtyMarker := ""
	if (mw.editorState != nil && mw.editorState.IsDirty()) || mw.projectDirty {
		dirtyMarker = " *"
	}
	if mw.session != nil && mw.session.Document != nil {
		mw.fileLabel.SetText(fmt.Sprintf("%s (%0.0fx%0.0f)%s", name, mw.session.Document.Width, mw.session.Document.Height, dirtyMarker))
	} else {
		mw.fileLabel.SetText(fmt.Sprintf("%s%s", name, dirtyMarker))
	}
}

// handleCloseRequest prompts the user when closing with unapplied or unsaved changes.
func (mw *MainWindow) handleCloseRequest() {
	isDirty := (mw.editorState != nil && mw.editorState.IsDirty()) || mw.projectDirty
	if !isDirty {
		if mw.isExtMode && mw.editorState != nil {
			mw.editorState.Result = mw.editorState.OriginalSVG
			mw.editorState.Applied = false
		}
		mw.Close()
		return
	}

	state := mw.editorState
	count := 0
	if state != nil {
		count = state.HistoryIndex + 1
	}

	if mw.isExtMode && state != nil {
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
			mw.Close()
		})
		applyAndClose.Importance = widget.HighImportance

		discardChanges := widget.NewButtonWithIcon("Discard Changes", theme.DeleteIcon(), func() {
			d.Hide()
			state.Result = state.OriginalSVG
			state.Applied = false
			mw.Close()
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
	message := "You have unsaved project changes. Would you like to save before closing?"
	if state != nil && count == 1 {
		message = fmt.Sprintf("You have unsaved changes (%s). Would you like to save before closing?", state.History[state.HistoryIndex].Description)
	} else if count > 1 {
		message = fmt.Sprintf("You have %d unsaved change(s). Would you like to save before closing?", count)
	}

	var d *dialog.CustomDialog
	saveAndClose := widget.NewButtonWithIcon("Save & Close", theme.DocumentSaveIcon(), func() {
		d.Hide()
		mw.SaveWithCallback(func() {
			mw.Close()
		})
	})
	saveAndClose.Importance = widget.HighImportance

	discardAndClose := widget.NewButtonWithIcon("Discard & Close", theme.DeleteIcon(), func() {
		d.Hide()
		mw.Close()
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
