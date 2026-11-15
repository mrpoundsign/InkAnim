package ui

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"inkanim/internal/app"
	"inkanim/internal/ext/doctree"
	"inkanim/internal/ext/svgpatch"
)

// setupStudioHistoryAndShortcuts initializes menu items, history callbacks, and keyboard shortcuts for the studio window.
func setupStudioHistoryAndShortcuts(mw *MainWindow) {
	w := mw.window

	mw.undoItem = fyne.NewMenuItem("Undo", func() { mw.doUndo() })
	mw.undoItem.Shortcut = &fyne.ShortcutUndo{}
	mw.undoItem.Icon = theme.ContentUndoIcon()

	mw.redoItem = fyne.NewMenuItem("Redo", func() { mw.doRedo() })
	mw.redoItem.Shortcut = &fyne.ShortcutRedo{}
	mw.redoItem.Icon = theme.ContentRedoIcon()

	mw.deleteItem = fyne.NewMenuItem("Delete Element", func() { mw.doDeleteActiveElement() })
	mw.deleteItem.Icon = theme.DeleteIcon()
	mw.deleteItem.Shortcut = &desktop.CustomShortcut{KeyName: fyne.KeyDelete}

	mw.updateUndoRedo = func() {
		state := mw.editorState
		if state == nil {
			mw.undoItem.Disabled = true
			mw.redoItem.Disabled = true
			mw.deleteItem.Disabled = true
			if mw.timelinePanel != nil {
				mw.timelinePanel.SyncUndoRedo()
			}
			if mw.mainMenu != nil {
				mw.mainMenu.Refresh()
			}
			return
		}

		mw.undoItem.Disabled = !state.CanUndo()
		mw.redoItem.Disabled = !state.CanRedo()
		if mw.timelinePanel != nil {
			mw.timelinePanel.SyncUndoRedo()
		}

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

	mw.doDeleteActiveElement = func() {
		switch w.Canvas().Focused().(type) {
		case *widget.Entry, *commitEntry:
			return
		}
		if mw.timelinePanel != nil && mw.timelinePanel.DeleteSelectedAction() {
			return
		}
		state := mw.editorState
		if state == nil || state.ActiveNode == nil {
			return
		}
		if err := mw.DeleteTrackNode(state.ActiveNode.ID); err != nil {
			mw.statusLabel.SetText("Delete error: " + err.Error())
		}
	}

	// Keyboard Shortcuts on Window Canvas
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
	w.Canvas().AddShortcut(&desktop.CustomShortcut{
		KeyName: fyne.KeyDelete,
	}, func(_ fyne.Shortcut) { mw.doDeleteActiveElement() })
	w.Canvas().SetOnTypedKey(func(ev *fyne.KeyEvent) {
		if ev.Name == fyne.KeyDelete {
			mw.doDeleteActiveElement()
		}
	})

	if deskCanvas, ok := w.Canvas().(desktop.Canvas); ok {
		deskCanvas.SetOnKeyDown(func(ev *fyne.KeyEvent) {
			if ev != nil && (ev.Name == desktop.KeyControlLeft || ev.Name == desktop.KeyControlRight) {
				if mw.centerPanel != nil && mw.centerPanel.gizmoOverlay != nil {
					mw.centerPanel.gizmoOverlay.SetCtrlHeld(true)
				}
			}
		})
		deskCanvas.SetOnKeyUp(func(ev *fyne.KeyEvent) {
			if ev != nil && (ev.Name == desktop.KeyControlLeft || ev.Name == desktop.KeyControlRight) {
				if mw.centerPanel != nil && mw.centerPanel.gizmoOverlay != nil {
					mw.centerPanel.gizmoOverlay.SetCtrlHeld(false)
				}
			}
		})
	}

	mw.updateUndoRedo()
}

// rebuildAfterHistoryChange restores UI after undo/redo/delete.
func (mw *MainWindow) rebuildAfterHistoryChange() {
	mw.updateUndoRedo()
	if mw.timelinePanel != nil {
		mw.timelinePanel.Refresh()
	}
	if mw.centerPanel != nil {
		mw.centerPanel.RefreshGizmos()
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
		mw.runPreviewUpdateForGen(gen)
	})
	mw.previewMu.Unlock()
}

func (mw *MainWindow) flushPreviewUpdate() {
	mw.previewMu.Lock()
	if mw.closed || mw.previewTimer == nil {
		mw.previewMu.Unlock()
		return
	}
	mw.previewTimer.Stop()
	mw.previewTimer = nil
	gen := mw.previewGen
	mw.previewMu.Unlock()
	mw.runPreviewUpdateForGen(gen)
}

func (mw *MainWindow) runPreviewUpdateForGen(gen int) {
	mw.previewMu.Lock()
	if mw.closed || mw.editorState == nil || mw.session == nil || mw.previewGen != gen {
		mw.previewMu.Unlock()
		return
	}
	mw.previewTimer = nil
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
		}
		if mw.timelinePanel != nil {
			mw.timelinePanel.refreshGuidesOnly()
		}
		if mw.rightPanel != nil {
			mw.rightPanel.Refresh()
		}
	})
}

func (mw *MainWindow) isVisibleTreeNode(n *doctree.DocNode) bool {
	if n == nil {
		return false
	}
	if _, cfg, ok := mw.resolveNodeObjectConfig(n); ok && (len(cfg.Actions) > 0 || cfg.HasDisplayWindow || cfg.OverrideStartPos) {
		return true
	}
	if n.IsHidden {
		return false
	}
	if n.IsLinkedFile || n.IsLayer {
		return true
	}
	if !n.HasDrawableContent {
		return false
	}
	return n.IsGroup || doctree.IsDrawableLeafTag(n.Tag)
}

func (mw *MainWindow) resolveNodeObjectConfig(n *doctree.DocNode) (string, app.ObjectConfig, bool) {
	if mw.session == nil || mw.session.Objects == nil || n == nil {
		return "", app.ObjectConfig{}, false
	}
	if cfg, ok := mw.session.Objects[n.ID]; ok {
		return n.ID, cfg, true
	}
	keys := slices.Sorted(maps.Keys(mw.session.Objects))
	for _, k := range keys {
		cfg := mw.session.Objects[k]
		if cfg.FallbackXMLID == n.ID {
			return k, cfg, true
		}
	}
	if n.Label != "" {
		if n.SourceID != "" {
			cand := n.SourceID + "/" + n.Label
			if cfg, ok := mw.session.Objects[cand]; ok && (cfg.FallbackXMLID == "" || cfg.FallbackXMLID == n.ID) {
				return cand, cfg, true
			}
		}
		if cfg, ok := mw.session.Objects[n.Label]; ok && (cfg.FallbackXMLID == "" || cfg.FallbackXMLID == n.ID) {
			return n.Label, cfg, true
		}
	}
	return n.ID, app.ObjectConfig{
		SourceID:      n.SourceID,
		FallbackXMLID: n.ID,
	}, false
}

func (mw *MainWindow) consolidateNodeObjectConfig(nodeID, keepKey string, cfg app.ObjectConfig) {
	if mw.session == nil {
		return
	}
	if mw.session.Objects == nil {
		mw.session.Objects = make(map[string]app.ObjectConfig)
	}
	if keepKey == "" {
		keepKey = nodeID
	}
	cfg.FallbackXMLID = nodeID
	for k, existing := range mw.session.Objects {
		if k != keepKey && existing.FallbackXMLID == nodeID {
			delete(mw.session.Objects, k)
		}
	}
	mw.session.Objects[keepKey] = cfg
	if mw.session.Document != nil {
		durSec := float64(max(mw.session.TotalDurationMs, 100)) / 1000.0
		mw.session.Document.TotalDurationSec = durSec
		app.ApplyObjectsToDocument(mw.session.Document, mw.session.Objects, durSec)
	}
}

// RenameTrackNode renames a Layer or Group in the document tree, updates its inkscape:label
// in the source SVG and EditorState, and preserves its bound .inka ObjectConfig.
func (mw *MainWindow) RenameTrackNode(nodeID, newName string) error {
	newName = strings.TrimSpace(newName)
	if newName == "" {
		return errors.New("layer/group name cannot be empty")
	}
	if mw.editorState == nil {
		return errors.New("no active document")
	}
	node := mw.editorState.NodeMap[nodeID]
	if node == nil {
		return fmt.Errorf("object %q not found", nodeID)
	}
	if node.IsLinkedFile {
		return errors.New("linked SVG filenames cannot be renamed here")
	}

	oldTitle := node.DisplayTitle()
	if oldTitle == newName {
		return nil
	}

	node.LabelPrefix = newName
	mw.editorState.ModifiedIDs[node.ID] = true

	// Also patch rawSourceBytes for the linked SVG source so saving/re-composing preserves the renamed label
	if node.SourceID != "" && mw.rawSourceBytes != nil {
		if raw, ok := mw.rawSourceBytes[node.SourceID]; ok && len(raw) > 0 {
			origID := strings.TrimPrefix(node.ID, node.SourceID+"__")
			if patched, err := svgpatch.SetAttr(raw, origID, svgpatch.InkscapeNS, "label", node.FormatLabel()); err == nil {
				mw.rawSourceBytes[node.SourceID] = patched
			}
		}
	}

	// Update .inka ObjectConfig LabelPath / key while keeping FallbackXMLID intact
	if mw.session != nil && mw.session.Objects != nil {
		objKey, objCfg, hasCfg := mw.resolveNodeObjectConfig(node)
		if hasCfg {
			delete(mw.session.Objects, objKey)
			if objCfg.LabelPath != "" {
				parts := strings.Split(objCfg.LabelPath, "/")
				parts[len(parts)-1] = newName
				objCfg.LabelPath = strings.Join(parts, "/")
			} else {
				objCfg.LabelPath = newName
			}
			newKey := objCfg.LabelPath
			if objCfg.SourceID != "" {
				newKey = objCfg.SourceID + "/" + objCfg.LabelPath
			}
			objCfg.FallbackXMLID = node.ID
			mw.session.Objects[newKey] = objCfg
		}
	}

	mw.RecordStudioChange(fmt.Sprintf("Rename %s to '%s'", oldTitle, newName))
	if mw.timelinePanel != nil {
		mw.timelinePanel.Refresh()
	}
	mw.schedulePreviewUpdate()
	return nil
}

func (mw *MainWindow) visibleSiblingsForNode(node *doctree.DocNode) ([]*doctree.DocNode, int) {
	if mw.editorState == nil || node == nil {
		return nil, -1
	}
	var rawSibs []*doctree.DocNode
	if node.Parent == nil {
		rawSibs = mw.editorState.Roots
	} else {
		rawSibs = node.Parent.Children
	}
	var vis []*doctree.DocNode
	idx := -1
	for _, s := range rawSibs {
		if mw.isVisibleTreeNode(s) {
			if s.ID == node.ID {
				idx = len(vis)
			}
			vis = append(vis, s)
		}
	}
	return vis, idx
}

// CanMoveTrackNode reports whether the specified track node can be moved up or down among its siblings.
func (mw *MainWindow) CanMoveTrackNode(nodeID string) (canUp, canDown bool) {
	if mw.editorState == nil {
		return false, false
	}
	node := mw.editorState.NodeMap[nodeID]
	if node == nil {
		return false, false
	}
	vis, idx := mw.visibleSiblingsForNode(node)
	if idx < 0 || len(vis) <= 1 {
		return false, false
	}
	return idx > 0, idx < len(vis)-1
}

// MoveTrackNode moves a track node up (delta = -1) or down (delta = +1) relative to its siblings.
func (mw *MainWindow) MoveTrackNode(nodeID string, delta int) error {
	if mw.editorState == nil {
		return errors.New("no active document")
	}
	node := mw.editorState.NodeMap[nodeID]
	if node == nil {
		return fmt.Errorf("object %q not found", nodeID)
	}
	vis, idx := mw.visibleSiblingsForNode(node)
	targetIdx := idx + delta
	if idx < 0 || targetIdx < 0 || targetIdx >= len(vis) {
		return errors.New("cannot move track further in that direction")
	}
	other := vis[targetIdx]

	currentSVG := mw.editorState.ComputePatchedSVG()
	swapped, err := svgpatch.SwapElements(currentSVG, node.ID, other.ID)
	if err != nil {
		return err
	}

	if node.IsLinkedFile && other.IsLinkedFile && mw.session != nil {
		idxA, idxB := -1, -1
		for i, src := range mw.session.LinkedSources {
			if src.ID == node.SourceID {
				idxA = i
			}
			if src.ID == other.SourceID {
				idxB = i
			}
		}
		if idxA >= 0 && idxB >= 0 {
			mw.session.LinkedSources[idxA], mw.session.LinkedSources[idxB] = mw.session.LinkedSources[idxB], mw.session.LinkedSources[idxA]
		}
	} else if !node.IsLinkedFile && node.SourceID != "" && node.SourceID == other.SourceID && mw.rawSourceBytes != nil {
		if raw, ok := mw.rawSourceBytes[node.SourceID]; ok && len(raw) > 0 {
			origA := strings.TrimPrefix(node.ID, node.SourceID+"__")
			origB := strings.TrimPrefix(other.ID, other.SourceID+"__")
			if patchedRaw, rawErr := svgpatch.SwapElements(raw, origA, origB); rawErr == nil {
				mw.rawSourceBytes[node.SourceID] = patchedRaw
			}
		}
	}

	if err := mw.editorState.RestoreSnapshot(swapped, node.ID); err != nil {
		return err
	}
	dirLabel := "down"
	if delta < 0 {
		dirLabel = "up"
	}
	mw.RecordStudioChange(fmt.Sprintf("Move %s %s", node.DisplayTitle(), dirLabel))

	if mw.timelinePanel != nil {
		mw.timelinePanel.Refresh()
	}
	mw.schedulePreviewUpdate()
	return nil
}

// DeleteTrackNode removes the specified Layer, Group, or SVG element and all of its
// descendants from EditorState, rawSourceBytes, and .inka session.Objects with Undo/Redo support.
func (mw *MainWindow) DeleteTrackNode(nodeID string) error {
	if mw.editorState == nil {
		return errors.New("no active document")
	}
	canDelete, reason := mw.editorState.CanDeleteElement(nodeID)
	if !canDelete {
		return errors.New(reason)
	}
	node := mw.editorState.NodeMap[nodeID]
	if node == nil {
		return fmt.Errorf("object %q not found", nodeID)
	}

	var deletedNodes []*doctree.DocNode
	deletedIDs := make(map[string]bool)
	var collectSubtree func(n *doctree.DocNode)
	collectSubtree = func(n *doctree.DocNode) {
		if n == nil || deletedIDs[n.ID] {
			return
		}
		deletedIDs[n.ID] = true
		deletedNodes = append(deletedNodes, n)
		for _, child := range n.Children {
			collectSubtree(child)
		}
	}
	collectSubtree(node)

	if mw.session != nil && mw.session.Objects != nil {
		for _, delNode := range deletedNodes {
			if objKey, objCfg, hasCfg := mw.resolveNodeObjectConfig(delNode); hasCfg {
				for _, act := range objCfg.Actions {
					if mw.selectedActionID == act.ID {
						mw.selectedActionID = ""
					}
				}
				delete(mw.session.Objects, objKey)
			}
		}
		for _, k := range slices.Sorted(maps.Keys(mw.session.Objects)) {
			cfg := mw.session.Objects[k]
			if deletedIDs[k] || deletedIDs[cfg.FallbackXMLID] {
				for _, act := range cfg.Actions {
					if mw.selectedActionID == act.ID {
						mw.selectedActionID = ""
					}
				}
				delete(mw.session.Objects, k)
			}
		}
	}

	if !node.IsLinkedFile && node.SourceID != "" && mw.rawSourceBytes != nil {
		if raw, ok := mw.rawSourceBytes[node.SourceID]; ok && len(raw) > 0 {
			origID := strings.TrimPrefix(node.ID, node.SourceID+"__")
			if patchedRaw, rawErr := svgpatch.RemoveElement(raw, origID); rawErr == nil {
				mw.rawSourceBytes[node.SourceID] = patchedRaw
			}
		}
	}

	mw.projectDirty = true
	mw.bindEditorStateStudioHooks(mw.editorState, false)

	if err := mw.editorState.DeleteElement(nodeID); err != nil {
		return err
	}

	if mw.session != nil && mw.session.Document != nil {
		durSec := float64(max(mw.session.TotalDurationMs, 100)) / 1000.0
		mw.session.Document.TotalDurationSec = durSec
		app.ApplyObjectsToDocument(mw.session.Document, mw.session.Objects, durSec)
	}

	mw.lastHistoryMsg = ""
	mw.rebuildAfterHistoryChange()
	return nil
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
			changeStatus = fmt.Sprintf("Unsaved changes (%s)", state.History[state.HistoryIndex].Description)
		case count > 1:
			changeStatus = fmt.Sprintf("Unsaved changes (%d changes, latest: %s)", count, state.History[state.HistoryIndex].Description)
		default:
			changeStatus = "Unsaved changes"
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
	name := mw.projectDisplayName()
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

// handleCloseRequest prompts the user when closing with unsaved changes.
func (mw *MainWindow) handleCloseRequest() {
	isDirty := (mw.editorState != nil && mw.editorState.IsDirty()) || mw.projectDirty
	if !isDirty {
		mw.Close()
		return
	}

	state := mw.editorState
	count := 0
	if state != nil {
		count = state.HistoryIndex + 1
	}

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
