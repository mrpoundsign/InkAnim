package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"

	"inkanim/internal/ext/doctree"
)

func TestEditorState_ComputePatchedSVG(t *testing.T) {
	splinePath := filepath.Join("..", "..", "testdata", "spline_test.svg")
	data, err := os.ReadFile(splinePath)
	if err != nil {
		t.Fatalf("failed to read spline_test.svg: %v", err)
	}

	state, err := NewEditorState(data, splinePath, nil)
	if err != nil {
		t.Fatalf("NewEditorState failed: %v", err)
	}

	rectNode := state.NodeMap["rect_rocket"]
	if rectNode == nil {
		t.Fatalf("expected rect_rocket in NodeMap")
	}

	// Add a directive
	rectNode.Directives = append(rectNode.Directives, doctree.Directive{
		Type:   "Scale",
		Params: "f: 1-10; from: 1; to: 2",
		Raw:    "Scale {f: 1-10; from: 1; to: 2}",
	})
	state.ModifiedIDs["rect_rocket"] = true

	patched := state.ComputePatchedSVG()
	patchedStr := string(patched)

	if !strings.Contains(patchedStr, `inkscape:label="rect_rocket Scale {f: 1-10; from: 1; to: 2}"`) {
		t.Errorf("patched SVG does not contain expected label, got:\n%s", patchedStr)
	}
}

func TestEditorState_IsDirty_LabelRevert(t *testing.T) {
	cleanSVG := `<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape" width="100" height="100">
  <g id="layer1" inkscape:groupmode="layer" inkscape:label="Layer 1">
    <g id="g_rocket" inkscape:label="Rocket">
      <rect id="rect_rocket" width="10" height="20" inkscape:label="Body" />
    </g>
  </g>
</svg>`
	data := []byte(cleanSVG)

	state, err := NewEditorState(data, "clean.svg", nil)
	if err != nil {
		t.Fatalf("NewEditorState failed: %v", err)
	}

	rectNode := state.NodeMap["rect_rocket"]
	if rectNode == nil {
		t.Fatalf("expected rect_rocket in NodeMap")
	}

	// 1. Initially clean
	if state.IsDirty() {
		t.Errorf("expected state to be clean initially")
	}

	// 2. Add directive -> becomes dirty
	rectNode.Directives = append(rectNode.Directives, doctree.Directive{
		Type:   "Scale",
		Params: "f: 1-10; from: 1; to: 2",
		Raw:    "Scale {f: 1-10; from: 1; to: 2}",
	})
	state.ModifiedIDs["rect_rocket"] = true
	state.RecordChange("Add Scale")

	if !state.IsDirty() {
		t.Errorf("expected state.IsDirty() == true after adding directive")
	}

	// 3. Revert directives back to original -> becomes clean
	rectNode.Directives = nil
	if state.IsDirty() {
		t.Errorf("expected state.IsDirty() == false after reverting label back to original")
	}
}

func TestEditorState_InitialStateCleanOnLoad(t *testing.T) {
	for _, name := range []string{"pendulum.svg", "complex_motion.svg", "scale_test.svg", "color_test.svg", "spline_test.svg"} {
		p := filepath.Join("..", "..", "testdata", name)
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		st, err := NewEditorState(data, p, nil)
		if err != nil {
			t.Errorf("%s: NewEditorState err: %v", name, err)
			continue
		}
		if st.IsDirty() {
			t.Errorf("%s: expected clean state on load, but IsDirty() == true", name)
		}
		if len(st.ModifiedIDs) > 0 {
			t.Errorf("%s: expected 0 ModifiedIDs on load, got %d: %v", name, len(st.ModifiedIDs), st.ModifiedIDs)
		}
		if len(st.History) > 0 {
			t.Errorf("%s: expected empty History on load, got %d changes", name, len(st.History))
		}
	}
}

func TestEditorState_UndoRedo_DirectiveAddDelete(t *testing.T) {
	cleanSVG := `<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape" width="100" height="100">
  <rect id="rect1" inkscape:label="MyBox" x="10" y="10" width="20" height="20" />
</svg>`
	data := []byte(cleanSVG)

	state, err := NewEditorState(data, "clean.svg", []string{"rect1"})
	if err != nil {
		t.Fatalf("NewEditorState failed: %v", err)
	}

	node := state.ActiveNode
	if node == nil {
		t.Fatalf("expected rect1 active")
	}

	// Add directive
	node.Directives = append(node.Directives, doctree.Directive{
		Type:   "Move",
		Params: "f: 1-10",
		Raw:    "Move {f: 1-10}",
	})
	state.ModifiedIDs[node.ID] = true
	state.RecordChange("Add Move to rect1")

	if state.HistoryIndex != 0 || len(state.History) != 1 {
		t.Fatalf("expected 1 history entry, got index %d len %d", state.HistoryIndex, len(state.History))
	}

	// Delete directive
	node.Directives = nil
	state.ModifiedIDs[node.ID] = true
	state.RecordChange("Delete Move from rect1")

	if state.HistoryIndex != 1 || len(state.History) != 2 {
		t.Fatalf("expected 2 history entries, got index %d len %d", state.HistoryIndex, len(state.History))
	}

	// Undo delete
	_, err = state.Undo()
	if err != nil {
		t.Fatalf("Undo delete failed: %v", err)
	}
	if len(state.ActiveNode.Directives) != 1 {
		t.Fatalf("expected 1 directive restored after undoing delete, got %d", len(state.ActiveNode.Directives))
	}
	if state.ActiveNode.Directives[0].Type != "Move" {
		t.Errorf("expected restored directive to be Move, got %s", state.ActiveNode.Directives[0].Type)
	}

	// Undo add
	_, err = state.Undo()
	if err != nil {
		t.Fatalf("Undo add failed: %v", err)
	}
	if len(state.ActiveNode.Directives) != 0 {
		t.Fatalf("expected 0 directives after undoing add, got %d", len(state.ActiveNode.Directives))
	}
	if state.IsDirty() {
		t.Errorf("expected clean state after undoing back to initial")
	}
}

func TestEditorState_UndoRedo_TruncateOnNewEdit(t *testing.T) {
	cleanSVG := `<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape" width="100" height="100">
  <rect id="rect1" inkscape:label="MyBox" x="10" y="10" width="20" height="20" />
</svg>`
	data := []byte(cleanSVG)

	state, err := NewEditorState(data, "clean.svg", []string{"rect1"})
	if err != nil {
		t.Fatalf("NewEditorState failed: %v", err)
	}

	node := state.ActiveNode

	// Edit 1
	node.LabelPrefix = "Box Alpha"
	state.ModifiedIDs[node.ID] = true
	state.RecordChange("Rename prefix to Box Alpha")

	// Edit 2
	node.LabelPrefix = "Box Beta"
	state.ModifiedIDs[node.ID] = true
	state.RecordChange("Rename prefix to Box Beta")

	if len(state.History) != 2 || state.HistoryIndex != 1 {
		t.Fatalf("expected 2 history entries, got index %d len %d", state.HistoryIndex, len(state.History))
	}

	// Undo Edit 2
	_, _ = state.Undo()
	if state.HistoryIndex != 0 {
		t.Fatalf("expected index 0 after undo, got %d", state.HistoryIndex)
	}
	if !state.CanRedo() {
		t.Fatalf("expected CanRedo() == true")
	}

	// New Edit 3 while at index 0 should truncate Edit 2 from redo stack
	node = state.ActiveNode
	node.LabelPrefix = "Box Gamma"
	state.ModifiedIDs[node.ID] = true
	state.RecordChange("Rename prefix to Box Gamma")

	if len(state.History) != 2 {
		t.Fatalf("expected history truncated and appended to length 2, got %d", len(state.History))
	}
	if state.HistoryIndex != 1 {
		t.Fatalf("expected index 1, got %d", state.HistoryIndex)
	}
	if state.CanRedo() {
		t.Errorf("expected CanRedo() == false after branching new edit")
	}
	if state.History[1].Description != "Rename prefix to Box Gamma" {
		t.Errorf("expected latest history to be Box Gamma, got %q", state.History[1].Description)
	}
}

func TestCanDeleteElement(t *testing.T) {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg">
  <g id="layer1">
    <rect id="rect1" width="10" height="10"/>
    <circle r="5"/>
  </g>
  <g id="group_with_clip">
    <clipPath id="cp1">
      <rect width="10" height="10"/>
    </clipPath>
  </g>
  <g id="empty_group">
  </g>
</svg>`)

	state, err := NewEditorState(svg, "test.svg", nil)
	if err != nil {
		t.Fatalf("NewEditorState failed: %v", err)
	}

	// 1. Leaf element with ID: deletable
	canDel, reason := state.CanDeleteElement("rect1")
	if !canDel {
		t.Errorf("expected rect1 to be deletable, got reason: %s", reason)
	}

	// 2. Element with synthetic ID (circle without id): not deletable
	circleNode := state.NodeMap["circle_1"]
	if circleNode == nil || !circleNode.SyntheticID {
		t.Fatalf("expected synthetic ID node for circle")
	}
	canDel, _ = state.CanDeleteElement("circle_1")
	if canDel {
		t.Errorf("expected circle with synthetic ID to NOT be deletable")
	}

	// 3. Non-empty layer (layer1 contains rect1 and circle): deletable
	canDel, reason = state.CanDeleteElement("layer1")
	if !canDel {
		t.Errorf("expected non-empty layer1 to be deletable, got reason: %s", reason)
	}

	// 4. Group containing <clipPath>: deletable
	canDel, reason = state.CanDeleteElement("group_with_clip")
	if !canDel {
		t.Errorf("expected group_with_clip to be deletable, got reason: %s", reason)
	}

	// 5. Empty group: deletable
	canDel, reason = state.CanDeleteElement("empty_group")
	if !canDel {
		t.Errorf("expected empty_group to be deletable, got reason: %s", reason)
	}

	// 6. Linked source wrapper node: not deletable (uses Unlink SVG)
	state.NodeMap["src_1"] = &doctree.DocNode{ID: "src_1", IsLinkedFile: true}
	canDel, reason = state.CanDeleteElement("src_1")
	if canDel || !strings.Contains(reason, "Unlink SVG") {
		t.Errorf("expected linked source src_1 not deletable with Unlink SVG reason, got canDel=%v reason=%q", canDel, reason)
	}
}

func TestDeleteElement_UndoRedo(t *testing.T) {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg">
  <g id="layer1">
    <rect id="rect1" width="10" height="10"/>
    <rect id="rect2" width="20" height="20"/>
  </g>
</svg>`)

	state, err := NewEditorState(svg, "test.svg", nil)
	if err != nil {
		t.Fatalf("NewEditorState failed: %v", err)
	}

	if state.IsDirty() {
		t.Errorf("expected clean state initially")
	}

	// Select rect1
	state.ActiveNode = state.NodeMap["rect1"]

	// Delete rect1
	err = state.DeleteElement("rect1")
	if err != nil {
		t.Fatalf("DeleteElement failed: %v", err)
	}

	// Verify rect1 is removed from SVGData and NodeMap
	if state.NodeMap["rect1"] != nil {
		t.Errorf("expected rect1 to be removed from NodeMap")
	}
	if strings.Contains(string(state.SVGData), "rect1") {
		t.Errorf("expected rect1 to be removed from SVGData: %s", string(state.SVGData))
	}
	if !state.IsDirty() {
		t.Errorf("expected state to be dirty after delete")
	}
	// ActiveNode should now be parent "layer1"
	if state.ActiveNode == nil || state.ActiveNode.ID != "layer1" {
		t.Errorf("expected ActiveNode to be parent 'layer1', got %+v", state.ActiveNode)
	}
	if !state.CanUndo() {
		t.Errorf("expected CanUndo() == true after delete")
	}

	// Test Undo
	desc, err := state.Undo()
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}
	if !strings.Contains(desc, "Delete rect1") {
		t.Errorf("expected undo description to mention Delete rect1, got %q", desc)
	}
	if state.NodeMap["rect1"] == nil {
		t.Errorf("expected rect1 restored in NodeMap after undo")
	}
	if !strings.Contains(string(state.SVGData), "rect1") {
		t.Errorf("expected rect1 restored in SVGData after undo")
	}
	if state.IsDirty() {
		t.Errorf("expected clean state after undoing back to initial state")
	}

	// Test Redo
	desc, err = state.Redo()
	if err != nil {
		t.Fatalf("Redo failed: %v", err)
	}
	if !strings.Contains(desc, "Delete rect1") {
		t.Errorf("expected redo description to mention Delete rect1, got %q", desc)
	}
	if state.NodeMap["rect1"] != nil {
		t.Errorf("expected rect1 removed in NodeMap after redo")
	}
	if strings.Contains(string(state.SVGData), "rect1") {
		t.Errorf("expected rect1 removed from SVGData after redo")
	}
	if !state.IsDirty() {
		t.Errorf("expected dirty state after redo")
	}
}

func TestStudioMainWindow_DeleteAndUndoRedoShortcuts(t *testing.T) {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="100" height="100">
  <g id="layer1">
    <rect id="rect1" width="10" height="10"/>
    <rect id="rect2" width="20" height="20"/>
  </g>
</svg>`)

	a := test.NewApp()
	defer a.Quit()
	mw := NewMainWindow(a)
	defer func() {
		waitForPreview(mw)
		mw.Close()
	}()

	mw.loadData(svg, "delete_test.svg")
	waitForPreview(mw)

	if mw.editorState == nil {
		t.Fatalf("expected editorState to be initialized")
	}

	mw.timelinePanel.SelectTrack("rect1")
	if mw.editorState.ActiveNode == nil || mw.editorState.ActiveNode.ID != "rect1" {
		t.Fatalf("expected active node rect1, got %+v", mw.editorState.ActiveNode)
	}
	if mw.deleteItem == nil || mw.deleteItem.Disabled {
		t.Fatalf("expected Delete Element menu item to be enabled for rect1")
	}

	mw.doDeleteActiveElement()
	if mw.editorState.NodeMap["rect1"] != nil {
		t.Errorf("expected rect1 deleted after doDeleteActiveElement")
	}
	if mw.undoItem == nil || mw.undoItem.Disabled {
		t.Errorf("expected Undo menu item enabled after delete")
	}

	mw.doUndo()
	if mw.editorState.NodeMap["rect1"] == nil {
		t.Errorf("expected rect1 restored after doUndo")
	}
	if mw.redoItem == nil || mw.redoItem.Disabled {
		t.Errorf("expected Redo menu item enabled after undo")
	}

	mw.doRedo()
	if mw.editorState.NodeMap["rect1"] != nil {
		t.Errorf("expected rect1 deleted again after doRedo")
	}
}

func TestSchedulePreviewUpdateDebounce(t *testing.T) {
	splinePath := filepath.Join("..", "..", "testdata", "spline_test.svg")
	data, err := os.ReadFile(splinePath)
	if err != nil {
		t.Fatalf("failed to read spline_test.svg: %v", err)
	}

	a := test.NewApp()
	defer a.Quit()
	mw := NewMainWindow(a)
	defer func() {
		waitForPreview(mw)
		mw.Close()
	}()

	mw.loadData(data, "spline_test.svg")
	waitForPreview(mw)

	if mw.session.Document == nil || mw.session.Document.FrameCount() == 0 {
		t.Fatalf("expected session document to be loaded with frames")
	}

	initialGen := mw.previewGen
	mw.schedulePreviewUpdate()
	if mw.previewGen != initialGen+1 {
		t.Errorf("expected previewGen to increment, got %d want %d", mw.previewGen, initialGen+1)
	}

	// Triggering another schedule immediately should increment previewGen again and cancel previous timer
	mw.schedulePreviewUpdate()
	if mw.previewGen != initialGen+2 {
		t.Errorf("expected previewGen to increment to %d, got %d", initialGen+2, mw.previewGen)
	}
}
