package main

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
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

// Change records a document mutation with a human-readable description, full SVG snapshot,
// and the ID of the active node at the time of the change.
type Change struct {
	Description  string
	SVGData      []byte
	ActiveNodeID string
}

type EditorState struct {
	Roots            []*doctree.DocNode
	NodeMap          map[string]*doctree.DocNode
	ActiveNode       *doctree.DocNode
	ModifiedIDs      map[string]bool
	SelectedIDs      []string
	InputPath        string
	SVGData          []byte
	OriginalSVG      []byte
	OriginalActiveID string
	History          []Change
	HistoryIndex        int
	ClipboardDirectives string
	Result              []byte
	Applied             bool
}

const maxHistory = 100

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

// RecordChange captures a snapshot of the current document state and appends it to History,
// pruning any redo history if changes were previously undone.
func (s *EditorState) RecordChange(description string) {
	snapshot := s.ComputePatchedSVG()
	if s.HistoryIndex < len(s.History)-1 {
		s.History = s.History[:s.HistoryIndex+1]
	}
	var activeID string
	if s.ActiveNode != nil {
		activeID = s.ActiveNode.ID
	}
	s.History = append(s.History, Change{
		Description:  description,
		SVGData:      snapshot,
		ActiveNodeID: activeID,
	})
	s.HistoryIndex = len(s.History) - 1
	if len(s.History) > maxHistory {
		excess := len(s.History) - maxHistory
		s.History = s.History[excess:]
		s.HistoryIndex -= excess
		if s.HistoryIndex < 0 {
			s.HistoryIndex = 0
		}
	}
}

// CanUndo reports whether there are actions in history that can be undone.
func (s *EditorState) CanUndo() bool {
	return s.HistoryIndex >= 0
}

// CanRedo reports whether there are previously undone actions that can be redone.
func (s *EditorState) CanRedo() bool {
	return s.HistoryIndex < len(s.History)-1
}

// RestoreSnapshot restores the document tree and node selections from SVG data.
func (s *EditorState) RestoreSnapshot(svgData []byte, targetNodeID string) error {
	roots, nodeMap, err := doctree.ParseTree(svgData)
	if err != nil {
		return err
	}
	s.Roots = roots
	s.NodeMap = nodeMap
	s.SVGData = svgData
	s.ModifiedIDs = make(map[string]bool)

	// Restore active node selection
	switch {
	case targetNodeID != "" && nodeMap[targetNodeID] != nil:
		s.ActiveNode = nodeMap[targetNodeID]
	case s.ActiveNode != nil && nodeMap[s.ActiveNode.ID] != nil:
		s.ActiveNode = nodeMap[s.ActiveNode.ID]
	case s.ActiveNode != nil && s.ActiveNode.ParentID != "" && nodeMap[s.ActiveNode.ParentID] != nil:
		s.ActiveNode = nodeMap[s.ActiveNode.ParentID]
	case len(roots) > 0:
		s.ActiveNode = roots[0]
	default:
		s.ActiveNode = nil
	}
	return nil
}

// Undo steps back one action in history and restores the previous document state.
func (s *EditorState) Undo() (string, error) {
	if !s.CanUndo() {
		return "", errors.New("nothing to undo")
	}
	undoneDesc := s.History[s.HistoryIndex].Description
	s.HistoryIndex--
	if s.HistoryIndex >= 0 {
		prev := s.History[s.HistoryIndex]
		return undoneDesc, s.RestoreSnapshot(prev.SVGData, prev.ActiveNodeID)
	}
	return undoneDesc, s.RestoreSnapshot(s.OriginalSVG, s.OriginalActiveID)
}

// Redo steps forward one action in history and restores the subsequent document state.
func (s *EditorState) Redo() (string, error) {
	if !s.CanRedo() {
		return "", errors.New("nothing to redo")
	}
	s.HistoryIndex++
	next := s.History[s.HistoryIndex]
	return next.Description, s.RestoreSnapshot(next.SVGData, next.ActiveNodeID)
}

// IsDirty reports whether the current document state differs from the original SVG loaded.
func (s *EditorState) IsDirty() bool {
	return !bytes.Equal(s.ComputePatchedSVG(), s.OriginalSVG)
}

// CanDeleteElement checks whether the element identified by nodeID can be safely deleted.
// Leaf elements with real SVG IDs are deletable.
// Groups and layers are deletable only if they contain no raw XML child elements.
// Elements without a real SVG ID (synthetic IDs) are not deletable.
func (s *EditorState) CanDeleteElement(nodeID string) (bool, string) {
	node := s.NodeMap[nodeID]
	if node == nil {
		return false, "Element not found"
	}
	if node.SyntheticID {
		return false, "Cannot delete element without an SVG id attribute"
	}
	if node.IsGroup || node.IsLayer {
		childCount, err := svgpatch.ChildElementCount(s.ComputePatchedSVG(), node.ID)
		if err != nil {
			return false, err.Error()
		}
		if childCount > 0 {
			if childCount == 1 {
				return false, "Group contains 1 element — empty it in Inkscape first"
			}
			return false, fmt.Sprintf("Group contains %d elements — empty it in Inkscape first", childCount)
		}
	}
	return true, ""
}

// DeleteElement removes the element identified by nodeID from the document.
// It byte-splices out the element, updates tree hierarchy, selects parent/sibling,
// and records an undoable history snapshot.
func (s *EditorState) DeleteElement(nodeID string) error {
	canDelete, reason := s.CanDeleteElement(nodeID)
	if !canDelete {
		return errors.New(reason)
	}

	node := s.NodeMap[nodeID]
	title := node.DisplayTitle()
	parentID := node.ParentID

	// Flush any in-flight label modifications
	s.SVGData = s.ComputePatchedSVG()
	s.ModifiedIDs = make(map[string]bool)

	patched, err := svgpatch.RemoveElement(s.SVGData, nodeID)
	if err != nil {
		return fmt.Errorf("failed to remove element: %w", err)
	}

	// Restore snapshot into state and select parent if available
	if err := s.RestoreSnapshot(patched, parentID); err != nil {
		return fmt.Errorf("failed to restore snapshot after delete: %w", err)
	}

	s.RecordChange("Delete " + title)
	return nil
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
		s.RecordChange(fmt.Sprintf("Apply preset '%s' to %s", p.Name, targetNode.DisplayTitle()))
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
	s.RecordChange(fmt.Sprintf("Apply preset '%s' to %s", p.Name, targetNode.DisplayTitle()))
	return targetNode.ID, nil
}

var pivotRefRe = regexp.MustCompile(`(?i)\bpivot:\s*#([a-zA-Z0-9_\-\.:]+)`)

// CopyMotions extracts all directives on node into an IAMS plaintext string and caches it in s.ClipboardDirectives.
func (s *EditorState) CopyMotions(node *doctree.DocNode) string {
	if node == nil || len(node.Directives) == 0 {
		return ""
	}
	var parts []string
	for _, d := range node.Directives {
		raw := strings.TrimSpace(d.Raw)
		if raw == "" {
			raw = fmt.Sprintf("%s {%s}", d.Type, d.Params)
		}
		parts = append(parts, raw)
	}
	text := strings.Join(parts, " ")
	s.ClipboardDirectives = text
	return text
}

// CopySingleDirective caches a single directive's IAMS string in s.ClipboardDirectives and returns it.
func (s *EditorState) CopySingleDirective(d doctree.Directive) string {
	raw := strings.TrimSpace(d.Raw)
	if raw == "" {
		raw = fmt.Sprintf("%s {%s}", d.Type, d.Params)
	}
	s.ClipboardDirectives = raw
	return raw
}

// PasteMotions parses rawDirectives (falling back to s.ClipboardDirectives) and appends
// the resulting motion directives to the active element. If the active element is a group
// or an un-animated child shape in a group, a new motion anchor circle is created in the group.
// If any directive references an object pivot (#oldID), a new pivot anchor element is generated
// at the same location in the target's group and the directive is rewritten to reference it.
func (s *EditorState) PasteMotions(rawDirectives string) (string, error) {
	if s.ActiveNode == nil {
		return "", errors.New("no active node selected")
	}
	if strings.TrimSpace(rawDirectives) == "" {
		rawDirectives = s.ClipboardDirectives
	}
	if strings.TrimSpace(rawDirectives) == "" {
		return "", errors.New("clipboard is empty")
	}

	_, parsedDirectives, _ := doctree.ParseDirectives(rawDirectives)
	if len(parsedDirectives) == 0 {
		return "", errors.New("no valid motion directives found to paste")
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

	// 1. Pivot ID handling:
	// If any pasted directive references an object pivot (pivot: #source_pivot),
	// measure its position and create a new anchor pivot element in the target's group,
	// then rewrite the directive to reference the new pivot ID.
	createdPivots := make(map[string]string)
	doc, docErr := inksvg.ParseSVG(s.ComputePatchedSVG())

	for i := range parsedDirectives {
		d := &parsedDirectives[i]
		matches := pivotRefRe.FindAllStringSubmatch(d.Params, -1)
		for _, m := range matches {
			if len(m) < 2 {
				continue
			}
			matchedFull := m[0]
			oldPivotID := m[1]

			newPivotID, exists := createdPivots[oldPivotID]
			if !exists {
				// Determine parent group where the pivot should be inserted
				insertGroupID := targetNode.ID
				if !isGroup && targetNode.ParentID != "" {
					insertGroupID = targetNode.ParentID
				}

				// Find location for new pivot
				var cx, cy float64
				foundLocation := false
				if docErr == nil && doc != nil {
					rect, ok := inksvg.ComputeElementRect(s.ComputePatchedSVG(), oldPivotID)
					if ok {
						cx = rect.X + rect.Width/2.0
						cy = rect.Y + rect.Height/2.0
						foundLocation = true
					}
					if !foundLocation {
						// Fallback to active node center
						tRect, tOk := inksvg.ComputeElementRect(s.ComputePatchedSVG(), s.ActiveNode.ID)
						if tOk {
							cx = tRect.X + tRect.Width/2.0
							cy = tRect.Y + tRect.Height/2.0
							foundLocation = true
						}
					}
				}
				if !foundLocation && doc != nil {
					dRect := doc.GetDrawingRect()
					cx = dRect.X + dRect.Width/2.0
					cy = dRect.Y + dRect.Height/2.0
				}

				basePivotID := targetNode.ID + "_pivot"
				newPivotID = basePivotID
				counter := 2
				for s.NodeMap[newPivotID] != nil {
					newPivotID = fmt.Sprintf("%s_%d", basePivotID, counter)
					counter++
				}

				pivotXML := fmt.Sprintf(`<circle id="%s" cx="%.2f" cy="%.2f" r="1.5" style="fill:#38bdf8;stroke:none" inkscape:label="Pivot: %s" />`,
					newPivotID, cx, cy, newPivotID)

				patched, err := svgpatch.InsertChild(s.ComputePatchedSVG(), insertGroupID, pivotXML)
				if err == nil {
					s.SVGData = patched
					s.ModifiedIDs = make(map[string]bool)
					if roots, nodeMap, pErr := doctree.ParseTree(s.SVGData); pErr == nil {
						s.Roots = roots
						s.NodeMap = nodeMap
						if refreshedTarget := s.NodeMap[targetNode.ID]; refreshedTarget != nil {
							targetNode = refreshedTarget
						}
					}
					createdPivots[oldPivotID] = newPivotID
					// Refresh doc for subsequent measurements
					doc, _ = inksvg.ParseSVG(s.SVGData)
				}
			}

			if newPivotID != "" {
				d.Params = strings.Replace(d.Params, matchedFull, "pivot: #"+newPivotID, 1)
				d.Raw = fmt.Sprintf("%s {%s}", d.Type, d.Params)
			}
		}
	}

	// 2. Attach directives to target
	var resultingID string
	if isGroup {
		var cx, cy float64
		if doc != nil {
			measureID := s.ActiveNode.ID
			rect, ok := inksvg.ComputeElementRect(s.ComputePatchedSVG(), measureID)
			if ok {
				cx = rect.X + rect.Width/2.0
				cy = rect.Y + rect.Height/2.0
			} else {
				dRect := doc.GetDrawingRect()
				cx = dRect.X + dRect.Width/2.0
				cy = dRect.Y + dRect.Height/2.0
			}
		}

		baseID := targetNode.ID + "_motion"
		anchorID := baseID
		counter := 2
		for s.NodeMap[anchorID] != nil {
			anchorID = fmt.Sprintf("%s_%d", baseID, counter)
			counter++
		}

		var dirStrings []string
		for _, d := range parsedDirectives {
			dirStrings = append(dirStrings, d.Raw)
		}
		label := fmt.Sprintf("%s: %s", anchorID, strings.Join(dirStrings, " "))
		childXML := fmt.Sprintf(`<circle id="%s" cx="%.2f" cy="%.2f" r="1.5" style="fill:#38bdf8;stroke:none" inkscape:label="%s" />`,
			anchorID, cx, cy, label)

		patched, err := svgpatch.InsertChild(s.ComputePatchedSVG(), targetNode.ID, childXML)
		if err != nil {
			return "", fmt.Errorf("failed to insert motion anchor element: %w", err)
		}

		s.SVGData = patched
		s.ModifiedIDs = make(map[string]bool)
		roots, nodeMap, err := doctree.ParseTree(s.SVGData)
		if err != nil {
			return "", fmt.Errorf("failed to re-parse tree: %w", err)
		}
		s.Roots = roots
		s.NodeMap = nodeMap
		if newAnchor := nodeMap[anchorID]; newAnchor != nil {
			s.ActiveNode = newAnchor
		}
		resultingID = anchorID
	} else {
		if len(targetNode.Directives) == 0 && targetNode.LabelPrefix == "" {
			targetNode.LabelPrefix = targetNode.ID
		}
		targetNode.Directives = append(targetNode.Directives, parsedDirectives...)
		s.ModifiedIDs[targetNode.ID] = true
		resultingID = targetNode.ID
	}

	desc := fmt.Sprintf("Paste %d motion(s) onto %s", len(parsedDirectives), targetNode.DisplayTitle())
	if len(parsedDirectives) == 1 {
		desc = fmt.Sprintf("Paste %s motion onto %s", parsedDirectives[0].Type, targetNode.DisplayTitle())
	}
	s.RecordChange(desc)
	return resultingID, nil
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

	var activeID string
	if active != nil {
		activeID = active.ID
	}

	state := &EditorState{
		Roots:            roots,
		NodeMap:          nodeMap,
		ActiveNode:       active,
		ModifiedIDs:      make(map[string]bool),
		SelectedIDs:      selectedIDs,
		InputPath:        inputPath,
		SVGData:          data,
		OriginalSVG:      bytes.Clone(data),
		OriginalActiveID: activeID,
		History:          nil,
		HistoryIndex:     -1,
		Result:           data,
	}

	return state, nil
}

// commitEntry is an Entry widget that coalesces live typing modifications for smooth real-time preview,
// and only commits an undo snapshot when submitted (Enter key) or when focus is lost.
// It also catches Ctrl+Z and Ctrl+Y shortcuts when focused so entry widgets never swallow global undo/redo.
type commitEntry struct {
	widget.Entry
	lastCommitted string
	onCommit      func(val string)
	onUndo        func()
	onRedo        func()
}

func newCommitEntry(initialText string, onLiveChange func(string), onCommit func(string), onUndo func(), onRedo func()) *commitEntry {
	e := &commitEntry{
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

func (e *commitEntry) FocusLost() {
	e.Entry.FocusLost()
	e.Commit()
}

func (e *commitEntry) Commit() {
	current := e.Text
	if current != e.lastCommitted {
		e.lastCommitted = current
		if e.onCommit != nil {
			e.onCommit(current)
		}
	}
}

func (e *commitEntry) SetText(text string) {
	e.lastCommitted = text
	e.Entry.SetText(text)
}

func (e *commitEntry) TypedShortcut(shortcut fyne.Shortcut) {
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

	// Undo/Redo Controls
	undoBtn := widget.NewButtonWithIcon("", theme.ContentUndoIcon(), nil)
	undoBtn.Importance = widget.LowImportance

	redoBtn := widget.NewButtonWithIcon("", theme.ContentRedoIcon(), nil)
	redoBtn.Importance = widget.LowImportance

	undoItem := fyne.NewMenuItem("Undo", nil)
	undoItem.Shortcut = &fyne.ShortcutUndo{}
	undoItem.Icon = theme.ContentUndoIcon()

	redoItem := fyne.NewMenuItem("Redo", nil)
	redoItem.Shortcut = &fyne.ShortcutRedo{}
	redoItem.Icon = theme.ContentRedoIcon()

	copyItem := fyne.NewMenuItem("Copy Motions", nil)
	copyItem.Shortcut = &fyne.ShortcutCopy{}
	copyItem.Icon = theme.ContentCopyIcon()

	pasteItem := fyne.NewMenuItem("Paste Motions", nil)
	pasteItem.Shortcut = &fyne.ShortcutPaste{}
	pasteItem.Icon = theme.ContentPasteIcon()

	deleteItem := fyne.NewMenuItem("Delete Element", nil)
	deleteItem.Icon = theme.DeleteIcon()
	deleteItem.Shortcut = &desktop.CustomShortcut{KeyName: fyne.KeyDelete}

	editMenu := fyne.NewMenu("Edit",
		undoItem,
		redoItem,
		fyne.NewMenuItemSeparator(),
		copyItem,
		pasteItem,
		fyne.NewMenuItemSeparator(),
		deleteItem,
	)
	mainMenu := fyne.NewMainMenu(editMenu)
	w.SetMainMenu(mainMenu)

	updateUndoRedo := func() {
		canUndo := state.CanUndo()
		canRedo := state.CanRedo()
		if canUndo {
			undoBtn.Enable()
			undoItem.Disabled = false
		} else {
			undoBtn.Disable()
			undoItem.Disabled = true
		}
		if canRedo {
			redoBtn.Enable()
			redoItem.Disabled = false
		} else {
			redoBtn.Disable()
			redoItem.Disabled = true
		}

		canCopy := state.ActiveNode != nil && len(state.ActiveNode.Directives) > 0
		copyItem.Disabled = !canCopy

		clipHasText := state.ClipboardDirectives != ""
		if a != nil && a.Clipboard() != nil && strings.TrimSpace(a.Clipboard().Content()) != "" {
			clipHasText = true
		}
		pasteItem.Disabled = state.ActiveNode == nil || !clipHasText

		var canDelete bool
		if state.ActiveNode != nil {
			canDelete, _ = state.CanDeleteElement(state.ActiveNode.ID)
		}
		deleteItem.Disabled = !canDelete

		mainMenu.Refresh()
	}
	updateUndoRedo()

	var lastHistoryMsg string
	var tree *deletableTree
	var refreshTree func()
	var refreshInspector func()

	updateStatus := func() {
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
		if lastHistoryMsg != "" {
			changeStatus = fmt.Sprintf("%s (%s)", lastHistoryMsg, changeStatus)
			lastHistoryMsg = ""
		}
		statusLabel.SetText(fmt.Sprintf("Active Object: %s | %s", activeName, changeStatus))
	}

	rebuildAfterHistoryChange := func() {
		updateUndoRedo()
		refreshTree()
		if state.ActiveNode != nil && tree != nil {
			if state.ActiveNode.ParentID != "" {
				tree.OpenBranch(state.ActiveNode.ParentID)
			}
			tree.Select(state.ActiveNode.ID)
		}
		refreshInspector()
		schedulePreviewUpdate()
		updateStatus()
	}

	doUndo := func() {
		if !state.CanUndo() {
			return
		}
		desc, err := state.Undo()
		if err != nil {
			return
		}
		lastHistoryMsg = "Undid: " + desc
		rebuildAfterHistoryChange()
	}

	doRedo := func() {
		if !state.CanRedo() {
			return
		}
		desc, err := state.Redo()
		if err != nil {
			return
		}
		lastHistoryMsg = "Redid: " + desc
		rebuildAfterHistoryChange()
	}

	undoBtn.OnTapped = doUndo
	redoBtn.OnTapped = doRedo
	undoItem.Action = doUndo
	redoItem.Action = doRedo

	doCopy := func() {
		if _, isEntry := w.Canvas().Focused().(*commitEntry); isEntry {
			return
		}
		if _, isEntry := w.Canvas().Focused().(*widget.Entry); isEntry {
			return
		}
		if state.ActiveNode == nil || len(state.ActiveNode.Directives) == 0 {
			return
		}
		text := state.CopyMotions(state.ActiveNode)
		if a != nil && a.Clipboard() != nil {
			a.Clipboard().SetContent(text)
		}
		statusLabel.SetText(fmt.Sprintf("Copied %d motion directive(s) to clipboard", len(state.ActiveNode.Directives)))
		updateUndoRedo()
	}

	doPaste := func() {
		if _, isEntry := w.Canvas().Focused().(*commitEntry); isEntry {
			return
		}
		if _, isEntry := w.Canvas().Focused().(*widget.Entry); isEntry {
			return
		}
		if state.ActiveNode == nil {
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
			statusLabel.SetText("Clipboard is empty")
			return
		}
		resultingID, err := state.PasteMotions(clipText)
		if err != nil {
			statusLabel.SetText("Paste error: " + err.Error())
			return
		}
		lastHistoryMsg = ""
		rebuildAfterHistoryChange()
		if tree != nil && resultingID != "" {
			if n := state.NodeMap[resultingID]; n != nil && n.ParentID != "" {
				tree.OpenBranch(n.ParentID)
			}
			tree.Select(resultingID)
		}
	}

	copyItem.Action = doCopy
	pasteItem.Action = doPaste

	doDeleteActiveElement := func() {
		if state.ActiveNode == nil {
			return
		}
		if _, isEntry := w.Canvas().Focused().(*commitEntry); isEntry {
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
			statusLabel.SetText("Delete error: " + err.Error())
			return
		}
		lastHistoryMsg = ""
		rebuildAfterHistoryChange()
	}
	deleteItem.Action = doDeleteActiveElement

	w.Canvas().AddShortcut(&fyne.ShortcutUndo{}, func(_ fyne.Shortcut) {
		doUndo()
	})
	w.Canvas().AddShortcut(&fyne.ShortcutRedo{}, func(_ fyne.Shortcut) {
		doRedo()
	})
	w.Canvas().AddShortcut(&desktop.CustomShortcut{
		KeyName:  fyne.KeyZ,
		Modifier: fyne.KeyModifierControl,
	}, func(_ fyne.Shortcut) {
		doUndo()
	})
	w.Canvas().AddShortcut(&desktop.CustomShortcut{
		KeyName:  fyne.KeyZ,
		Modifier: fyne.KeyModifierControl | fyne.KeyModifierShift,
	}, func(_ fyne.Shortcut) {
		doRedo()
	})
	w.Canvas().AddShortcut(&desktop.CustomShortcut{
		KeyName:  fyne.KeyY,
		Modifier: fyne.KeyModifierControl,
	}, func(_ fyne.Shortcut) {
		doRedo()
	})
	w.Canvas().AddShortcut(&fyne.ShortcutCopy{}, func(_ fyne.Shortcut) {
		doCopy()
	})
	w.Canvas().AddShortcut(&fyne.ShortcutPaste{}, func(_ fyne.Shortcut) {
		doPaste()
	})
	w.Canvas().AddShortcut(&desktop.CustomShortcut{
		KeyName:  fyne.KeyC,
		Modifier: fyne.KeyModifierControl,
	}, func(_ fyne.Shortcut) {
		doCopy()
	})
	w.Canvas().AddShortcut(&desktop.CustomShortcut{
		KeyName:  fyne.KeyV,
		Modifier: fyne.KeyModifierControl,
	}, func(_ fyne.Shortcut) {
		doPaste()
	})
	w.Canvas().AddShortcut(&desktop.CustomShortcut{
		KeyName: fyne.KeyDelete,
	}, func(_ fyne.Shortcut) {
		doDeleteActiveElement()
	})
	w.Canvas().SetOnTypedKey(func(ev *fyne.KeyEvent) {
		if ev.Name == fyne.KeyDelete {
			doDeleteActiveElement()
		}
	})

	// Inspector container
	inspectorCard := container.NewStack()

	refreshInspector = func() {
		node := state.ActiveNode
		if node == nil {
			emptyNotice := container.NewCenter(widget.NewLabel("Select an object from the document tree to view and edit its motions."))
			inspectorCard.Objects = []fyne.CanvasObject{emptyNotice}
			inspectorCard.Refresh()
			updateStatus()
			updateUndoRedo()
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

		canDelete, deleteReason := state.CanDeleteElement(node.ID)
		deleteBtn := widget.NewButtonWithIcon("Delete", theme.DeleteIcon(), doDeleteActiveElement)
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
		prefixEntry := newCommitEntry(
			node.LabelPrefix,
			func(val string) {
				node.LabelPrefix = val
				state.ModifiedIDs[node.ID] = true
				updatePreview()
				updateStatus()
				refreshTree()
				schedulePreviewUpdate()
			},
			func(val string) {
				state.RecordChange(fmt.Sprintf("Rename %s prefix to '%s'", node.DisplayTitle(), val))
				updateUndoRedo()
				updateStatus()
			},
			doUndo,
			doRedo,
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

					onCommit := func(desc string) {
						state.ModifiedIDs[node.ID] = true
						state.RecordChange(fmt.Sprintf("%s on %s", desc, node.DisplayTitle()))
						updateUndoRedo()
						updateStatus()
					}

					onDelete := func() {
						deletedType := dir.Type
						node.Directives = append(node.Directives[:idx], node.Directives[idx+1:]...)
						state.ModifiedIDs[node.ID] = true
						state.RecordChange(fmt.Sprintf("Delete %s from %s", deletedType, node.DisplayTitle()))
						rebuildDirectivesList()
						updatePreview()
						updateUndoRedo()
						updateStatus()
						refreshTree()
						schedulePreviewUpdate()
					}

					onCopy := func() {
						text := state.CopySingleDirective(*dir)
						if a != nil && a.Clipboard() != nil {
							a.Clipboard().SetContent(text)
						}
						statusLabel.SetText(fmt.Sprintf("Copied %s directive to clipboard", dir.Type))
						updateUndoRedo()
					}

					card := buildDirectiveWidgetCard(dir, totalDocFrames, onModified, onCommit, onCopy, onDelete, doUndo, doRedo)
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
			updateUndoRedo()
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
			updateUndoRedo()
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

		copyAllBtn := widget.NewButtonWithIcon("Copy All", theme.ContentCopyIcon(), doCopy)
		copyAllBtn.Importance = widget.LowImportance
		if len(node.Directives) == 0 {
			copyAllBtn.Disable()
		}

		pasteBtn := widget.NewButtonWithIcon("Paste", theme.ContentPasteIcon(), doPaste)
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
		inspectorCard.Objects = []fyne.CanvasObject{scrollContent}
		inspectorCard.Refresh()
		updateStatus()
		updateUndoRedo()
	}

	// Build Tree widget
	tree = newDeletableTree(
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
		doDeleteActiveElement,
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

	// Tree panel with toolbar header for Undo/Redo
	treeHeader := container.NewHBox(
		widget.NewLabelWithStyle("Objects", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		layout.NewSpacer(),
		undoBtn,
		redoBtn,
	)
	treePanel := container.NewBorder(
		container.NewVBox(treeHeader, widget.NewSeparator()),
		nil, nil, nil,
		tree,
	)

	// Studio Layout:
	// Left side: Tabs [Layers & Motion | Frames | Export]
	// Right side: [Live Canvas Preview]
	motionSplit := container.NewHSplit(treePanel, inspectorCard)
	motionSplit.SetOffset(0.35)

	sidebarTabs := container.NewAppTabs(
		container.NewTabItemWithIcon("Layers & Motion", theme.VisibilityIcon(), motionSplit),
		container.NewTabItemWithIcon("Frames", theme.ListIcon(), leftPanel.Container()),
		container.NewTabItemWithIcon("Export", theme.DownloadIcon(), rightPanel.Container()),
	)

	mainSplit := container.NewHSplit(sidebarTabs, preview.Container())
	mainSplit.SetOffset(0.55)

	handleCloseRequest := func() {
		if !state.IsDirty() {
			cleanupPlayback()
			state.Result = state.OriginalSVG
			state.Applied = false
			w.Close()
			a.Quit()
			return
		}

		var message string
		count := state.HistoryIndex + 1
		switch {
		case hasExported:
			message = "You have exported an animated GIF, but your motion changes have not been applied to your Inkscape document yet.\n\nWould you like to apply your changes to the document before closing?"
		case count == 1:
			message = fmt.Sprintf("You have unapplied motion changes (%s).\n\nWould you like to apply your changes to your Inkscape document before closing?", state.History[state.HistoryIndex].Description)
		case count > 1:
			message = fmt.Sprintf("You have %d unapplied motion changes (latest: %s).\n\nWould you like to apply your changes to your Inkscape document before closing?", count, state.History[state.HistoryIndex].Description)
		case len(state.ModifiedIDs) > 0:
			message = fmt.Sprintf("You have unapplied motion changes on %d element(s).\n\nWould you like to apply your changes to your Inkscape document before closing?", len(state.ModifiedIDs))
		default:
			message = "You have unapplied motion changes.\n\nWould you like to apply your changes to your Inkscape document before closing?"
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
			state.Result = state.OriginalSVG
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
		if state.IsDirty() {
			state.Result = state.ComputePatchedSVG()
			state.Applied = true
		} else {
			state.Applied = false
		}
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
	onCommit func(description string),
	onCopy func(),
	onDelete func(),
	onUndo func(),
	onRedo func(),
) fyne.CanvasObject {
	m := params.Parse(dir.Type, dir.Params, totalDocFrames)
	initializing := true

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

	syncAndCommit := func(desc string) {
		syncToDir()
		rawPreviewLabel.SetText(dir.Raw)
		if !initializing && onCommit != nil {
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
	startEntry := newCommitEntry(
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

	endEntry := newCommitEntry(
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
		degEntry := newCommitEntry(
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
		fromEntry := newCommitEntry(
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

		toEntry := newCommitEntry(
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
		fromEntry := newCommitEntry(
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

		toEntry := newCommitEntry(
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

		degEntry := newCommitEntry(
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
		factorEntry := newCommitEntry(
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
		orderEntry := newCommitEntry(
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

