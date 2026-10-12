package ui

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"inkanim/internal/ext/doctree"
	"inkanim/internal/ext/presets"
	"inkanim/internal/ext/svgpatch"
	"inkanim/pkg/inksvg"
)

// Change records a document mutation with a human-readable description, full SVG snapshot,
// and the ID of the active node at the time of the change.
type Change struct {
	Description  string
	SVGData      []byte
	ActiveNodeID string
}

type EditorState struct {
	Roots               []*doctree.DocNode
	NodeMap             map[string]*doctree.DocNode
	ActiveNode          *doctree.DocNode
	ModifiedIDs         map[string]bool
	SelectedIDs         []string
	InputPath           string
	SVGData             []byte
	OriginalSVG         []byte
	OriginalActiveID    string
	History             []Change
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

// MarkClean resets the baseline original SVG to the current patched state.
func (s *EditorState) MarkClean() {
	s.SVGData = s.ComputePatchedSVG()
	s.OriginalSVG = bytes.Clone(s.SVGData)
	s.ModifiedIDs = make(map[string]bool)
}

// CanDeleteElement checks whether the element identified by nodeID can be safely deleted.
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
func (s *EditorState) DeleteElement(nodeID string) error {
	canDelete, reason := s.CanDeleteElement(nodeID)
	if !canDelete {
		return errors.New(reason)
	}

	node := s.NodeMap[nodeID]
	title := node.DisplayTitle()
	parentID := node.ParentID

	s.SVGData = s.ComputePatchedSVG()
	s.ModifiedIDs = make(map[string]bool)

	patched, err := svgpatch.RemoveElement(s.SVGData, nodeID)
	if err != nil {
		return fmt.Errorf("failed to remove element: %w", err)
	}

	if err := s.RestoreSnapshot(patched, parentID); err != nil {
		return fmt.Errorf("failed to restore snapshot after delete: %w", err)
	}

	s.RecordChange("Delete " + title)
	return nil
}

// ApplyPreset applies a motion preset to the active node.
func (s *EditorState) ApplyPreset(p presets.Preset) (string, error) {
	if s.ActiveNode == nil {
		return "", errors.New("no active node selected")
	}

	targetNode := s.ActiveNode
	isGroup := targetNode.IsGroup || targetNode.IsLayer

	if !isGroup && !targetNode.HasDirectives() && targetNode.ParentID != "" {
		if parent := s.NodeMap[targetNode.ParentID]; parent != nil {
			targetNode = parent
			isGroup = true
		}
	}

	if isGroup {
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

// PasteMotions parses rawDirectives and appends them to the active element.
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

	if !isGroup && !targetNode.HasDirectives() && targetNode.ParentID != "" {
		if parent := s.NodeMap[targetNode.ParentID]; parent != nil {
			targetNode = parent
			isGroup = true
		}
	}

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
				insertGroupID := targetNode.ID
				if !isGroup && targetNode.ParentID != "" {
					insertGroupID = targetNode.ParentID
				}

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
					doc, _ = inksvg.ParseSVG(s.SVGData)
				}
			}

			if newPivotID != "" {
				d.Params = strings.Replace(d.Params, matchedFull, "pivot: #"+newPivotID, 1)
				d.Raw = fmt.Sprintf("%s {%s}", d.Type, d.Params)
			}
		}
	}

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

// NewEditorState initializes the editor model from SVG data.
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
