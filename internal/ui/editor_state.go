package ui

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"slices"

	"inkanim/internal/app"
	"inkanim/internal/ext/doctree"
	"inkanim/internal/ext/svgpatch"
)

// StudioSnapshot captures the full .inka project and timeline state at a point in history.
type StudioSnapshot struct {
	Objects          map[string]app.ObjectConfig
	LinkedSources    []app.LinkedSVG
	RawSourceBytes   map[string][]byte
	TotalDurationMs  int
	FPS              int
	PingPong         bool
	SelectedActionID string
}

// Clone returns a deep copy of the StudioSnapshot so subsequent mutations do not alias history entries.
func (ss StudioSnapshot) Clone() StudioSnapshot {
	out := StudioSnapshot{
		Objects:          cloneObjectsMap(ss.Objects),
		LinkedSources:    slices.Clone(ss.LinkedSources),
		RawSourceBytes:   make(map[string][]byte, len(ss.RawSourceBytes)),
		TotalDurationMs:  ss.TotalDurationMs,
		FPS:              ss.FPS,
		PingPong:         ss.PingPong,
		SelectedActionID: ss.SelectedActionID,
	}
	for k, raw := range ss.RawSourceBytes {
		out.RawSourceBytes[k] = bytes.Clone(raw)
	}
	return out
}

func cloneObjectsMap(src map[string]app.ObjectConfig) map[string]app.ObjectConfig {
	out := make(map[string]app.ObjectConfig, len(src))
	for k, cfg := range src {
		cfgCopy := cfg
		cfgCopy.Actions = slices.Clone(cfg.Actions)
		out[k] = cfgCopy
	}
	return out
}

// Change records a document mutation with a human-readable description, full SVG snapshot,
// active node ID, and optional .inka studio/timeline snapshot at the time of the change.
type Change struct {
	Description  string
	SVGData      []byte
	ActiveNodeID string
	Studio       StudioSnapshot
	HasStudio    bool
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
	OriginalStudio      StudioSnapshot
	HasStudioSnapshot   bool
	CaptureStudio       func() StudioSnapshot
	ApplyStudio         func(StudioSnapshot)
	History             []Change
	HistoryIndex        int
}

const maxHistory = 100

// ComputePatchedSVG generates the updated SVG bytes with all modified labels applied.
func (s *EditorState) ComputePatchedSVG() []byte {
	patchedData := s.SVGData
	if len(s.ModifiedIDs) == 0 {
		return patchedData
	}
	for _, id := range slices.Sorted(maps.Keys(s.ModifiedIDs)) {
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
	ch := Change{
		Description:  description,
		SVGData:      snapshot,
		ActiveNodeID: activeID,
	}
	if s.CaptureStudio != nil {
		ch.Studio = s.CaptureStudio().Clone()
		ch.HasStudio = true
	}
	s.History = append(s.History, ch)
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
		if err := s.RestoreSnapshot(prev.SVGData, prev.ActiveNodeID); err != nil {
			return undoneDesc, err
		}
		if prev.HasStudio && s.ApplyStudio != nil {
			s.ApplyStudio(prev.Studio.Clone())
		}
		return undoneDesc, nil
	}
	if err := s.RestoreSnapshot(s.OriginalSVG, s.OriginalActiveID); err != nil {
		return undoneDesc, err
	}
	if s.HasStudioSnapshot && s.ApplyStudio != nil {
		s.ApplyStudio(s.OriginalStudio.Clone())
	}
	return undoneDesc, nil
}

// Redo steps forward one action in history and restores the subsequent document state.
func (s *EditorState) Redo() (string, error) {
	if !s.CanRedo() {
		return "", errors.New("nothing to redo")
	}
	s.HistoryIndex++
	next := s.History[s.HistoryIndex]
	if err := s.RestoreSnapshot(next.SVGData, next.ActiveNodeID); err != nil {
		return next.Description, err
	}
	if next.HasStudio && s.ApplyStudio != nil {
		s.ApplyStudio(next.Studio.Clone())
	}
	return next.Description, nil
}

// IsDirty reports whether the current document state differs from the original SVG loaded.
func (s *EditorState) IsDirty() bool {
	return !bytes.Equal(s.ComputePatchedSVG(), s.OriginalSVG)
}

// MarkClean resets the baseline original SVG and studio state to the current patched state.
func (s *EditorState) MarkClean() {
	s.SVGData = s.ComputePatchedSVG()
	s.OriginalSVG = bytes.Clone(s.SVGData)
	if s.CaptureStudio != nil {
		s.OriginalStudio = s.CaptureStudio().Clone()
		s.HasStudioSnapshot = true
	}
	s.ModifiedIDs = make(map[string]bool)
}

// CanDeleteElement checks whether the element identified by nodeID can be safely deleted.
func (s *EditorState) CanDeleteElement(nodeID string) (bool, string) {
	node := s.NodeMap[nodeID]
	if node == nil {
		return false, "Element not found"
	}
	if node.IsLinkedFile {
		return false, "Use Unlink SVG to remove a linked artwork file"
	}
	if node.SyntheticID {
		return false, "Cannot delete element without an SVG id attribute"
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
	}

	return state, nil
}
