package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"inkanim/internal/ext/doctree"
	"inkanim/internal/ext/presets"
	"inkanim/pkg/inksvg"
)

func TestTreeActualRendering(t *testing.T) {
	splinePath := filepath.Join("..", "..", "testdata", "spline_test.svg")
	data, err := os.ReadFile(splinePath)
	if err != nil {
		t.Fatalf("failed to read spline_test.svg: %v", err)
	}

	state, err := NewEditorState(data, splinePath, []string{"path_motion"})
	if err != nil {
		t.Fatalf("NewEditorState failed: %v", err)
	}

	a := test.NewApp()
	w := ShowEditorWindow(a, state)

	var tree *widget.Tree
	var findTree func(co fyne.CanvasObject)
	findTree = func(co fyne.CanvasObject) {
		if tr, ok := co.(*deletableTree); ok {
			tree = &tr.Tree
			return
		}
		if tr, ok := co.(*widget.Tree); ok {
			tree = tr
			return
		}
		if s, ok := co.(*container.Split); ok {
			findTree(s.Leading)
			findTree(s.Trailing)
		}
		if tabs, ok := co.(*container.AppTabs); ok {
			for _, item := range tabs.Items {
				findTree(item.Content)
			}
		}
		if c, ok := co.(*fyne.Container); ok {
			for _, child := range c.Objects {
				findTree(child)
			}
		}
	}
	findTree(w.Content())

	if tree == nil {
		t.Fatalf("tree is nil")
	}

	t.Logf("Tree roots: %v", tree.ChildUIDs(""))
	for _, root := range tree.ChildUIDs("") {
		t.Logf("Root: %s, isBranch=%v, children=%v", root, tree.IsBranch(root), tree.ChildUIDs(root))
	}

	roots := tree.ChildUIDs("")
	if len(roots) != 2 {
		t.Fatalf("expected 2 root nodes in tree, got %d", len(roots))
	}

	if !tree.IsBranch("layer1") || !tree.IsBranchOpen("layer1") {
		t.Errorf("expected layer1 to be an open branch")
	}
	if !tree.IsBranch("layer2") || !tree.IsBranchOpen("layer2") {
		t.Errorf("expected layer2 to be an open branch")
	}
	if !tree.IsBranch("g_rocket") || !tree.IsBranchOpen("g_rocket") {
		t.Errorf("expected g_rocket to be an open branch")
	}
}

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

func TestEditorState_ApplyPreset(t *testing.T) {
	splinePath := filepath.Join("..", "..", "testdata", "spline_test.svg")
	data, err := os.ReadFile(splinePath)
	if err != nil {
		t.Fatalf("failed to read spline_test.svg: %v", err)
	}

	state, err := NewEditorState(data, splinePath, []string{"g_rocket"})
	if err != nil {
		t.Fatalf("NewEditorState failed: %v", err)
	}

	// 1. Apply Spin preset to group "g_rocket"
	spinPreset, ok := presets.FindPreset("spin")
	if !ok {
		t.Fatalf("spin preset not found")
	}

	anchorID, err := state.ApplyPreset(spinPreset)
	if err != nil {
		t.Fatalf("ApplyPreset failed: %v", err)
	}
	if anchorID != "g_rocket_spin" {
		t.Errorf("expected anchor ID 'g_rocket_spin', got %q", anchorID)
	}

	// Verify new anchor node in tree
	anchorNode := state.NodeMap[anchorID]
	if anchorNode == nil {
		t.Fatalf("anchor node %q not found in state.NodeMap", anchorID)
	}
	if state.ActiveNode != anchorNode {
		t.Errorf("expected state.ActiveNode to be new anchor node")
	}
	if anchorNode.LabelPrefix != "Spin:" {
		t.Errorf("expected LabelPrefix 'Spin:', got %q", anchorNode.LabelPrefix)
	}

	// Verify anchor circle in SVGData
	svgStr := string(state.SVGData)
	if !strings.Contains(svgStr, `<circle id="g_rocket_spin"`) {
		t.Errorf("SVG does not contain anchor circle: %s", svgStr)
	}
	if !strings.Contains(svgStr, `inkscape:label="Spin: Rot {f: 1-20; deg: 360}"`) {
		t.Errorf("SVG does not contain prefixed preset label: %s", svgStr)
	}

	// Parse with inksvg to confirm motion path evaluation
	doc, err := inksvg.ParseSVG(state.SVGData)
	if err != nil {
		t.Fatalf("ParseSVG on SVGData failed: %v", err)
	}
	var foundRot bool
	for _, mp := range doc.MotionPaths {
		if mp.ID == "g_rocket_spin" {
			foundRot = true
			if mp.GroupID != "g_rocket" {
				t.Errorf("expected mp.GroupID == 'g_rocket', got %q", mp.GroupID)
			}
			if mp.Config.Type != "rot" || mp.Config.RotationAngle != 360 {
				t.Errorf("unexpected mp.Config: %+v", mp.Config)
			}
		}
	}
	if !foundRot {
		t.Errorf("anchor motion path g_rocket_spin not found in parsed doc")
	}

	// 2. Apply Float preset (creates vertical bob path)
	state.ActiveNode = state.NodeMap["g_rocket"]
	floatPreset, ok := presets.FindPreset("float")
	if !ok {
		t.Fatalf("float preset not found")
	}
	floatAnchorID, err := state.ApplyPreset(floatPreset)
	if err != nil {
		t.Fatalf("ApplyPreset float failed: %v", err)
	}
	if floatAnchorID != "g_rocket_float" {
		t.Errorf("expected float anchor ID 'g_rocket_float', got %q", floatAnchorID)
	}
	if !strings.Contains(string(state.SVGData), `<path id="g_rocket_float"`) {
		t.Errorf("SVG does not contain float motion path")
	}

	// 3. Apply preset when selecting a child artwork shape inside layer1 (e.g. "path6")
	state.ActiveNode = state.NodeMap["path6"]
	if state.ActiveNode == nil {
		t.Fatalf("path6 not found in state.NodeMap")
	}
	pulsePreset, ok := presets.FindPreset("pulse")
	if !ok {
		t.Fatalf("pulse preset not found")
	}
	pulseAnchorID, err := state.ApplyPreset(pulsePreset)
	if err != nil {
		t.Fatalf("ApplyPreset pulse on shape failed: %v", err)
	}
	// It should anchor to layer1
	if !strings.HasPrefix(pulseAnchorID, "layer1_") {
		t.Errorf("expected anchor on parent layer1, got %q", pulseAnchorID)
	}
	// Artwork path6 must retain its own ID and not be overwritten
	if state.NodeMap["path6"] == nil {
		t.Errorf("path6 was removed or corrupted")
	}

	// 4. Apply Shake preset: must adapt to 20 frames with pingpong and repeat
	state.ActiveNode = state.NodeMap["g_rocket"]
	shakePreset, ok := presets.FindPreset("shake")
	if !ok {
		t.Fatalf("shake preset not found")
	}
	shakeAnchorID, err := state.ApplyPreset(shakePreset)
	if err != nil {
		t.Fatalf("ApplyPreset shake failed: %v", err)
	}
	shakeNode := state.NodeMap[shakeAnchorID]
	if shakeNode == nil {
		t.Fatalf("expected shake anchor node %q", shakeAnchorID)
	}
	shakeLabel := shakeNode.FormatLabel()
	if !strings.Contains(shakeLabel, "f: 1-20") {
		t.Errorf("expected shake to adapt to 20 frames, got %q", shakeLabel)
	}
	if !strings.Contains(shakeLabel, "pingpong") {
		t.Errorf("expected shake to have pingpong, got %q", shakeLabel)
	}
	if !strings.Contains(shakeLabel, "r: 4") {
		t.Errorf("expected shake to have r: 4 for 20 frames, got %q", shakeLabel)
	}
}

func TestBuildDirectiveWidgetCard(t *testing.T) {
	_ = test.NewApp()

	dir := &doctree.Directive{
		Type:   "Rot",
		Params: "f: 1-20; angle: 45; ease: in-out; pingpong",
		Raw:    "Rot {f: 1-20; angle: 45; ease: in-out; pingpong}",
	}

	var modifiedCalled bool
	onModified := func() {
		modifiedCalled = true
	}
	var copyCalled bool
	onCopy := func() {
		copyCalled = true
	}
	var deleteCalled bool
	onDelete := func() {
		deleteCalled = true
	}

	cardObj := buildDirectiveWidgetCard(dir, 20, onModified, nil, onCopy, onDelete, nil, nil)
	card, ok := cardObj.(*fyne.Container)
	if !ok {
		t.Fatalf("expected card to be *fyne.Container")
	}

	// Find the angle entry inside card
	var angleEntry *widget.Entry
	var allFramesCheck *widget.Check
	var copyBtn *widget.Button
	var deleteBtn *widget.Button

	var walk func(co fyne.CanvasObject)
	walk = func(co fyne.CanvasObject) {
		if co == nil {
			return
		}
		if e, ok := co.(*commitEntry); ok {
			if e.Text == "45" {
				angleEntry = &e.Entry
			}
		} else if e, ok := co.(*widget.Entry); ok {
			if e.Text == "45" {
				angleEntry = e
			}
		}
		if ch, ok := co.(*widget.Check); ok {
			if ch.Text == "All Frames" {
				allFramesCheck = ch
			}
		}
		if btn, ok := co.(*widget.Button); ok {
			if btn.Importance == widget.DangerImportance {
				deleteBtn = btn
			}
			if btn.Icon == theme.ContentCopyIcon() {
				copyBtn = btn
			}
		}
		if c, ok := co.(*fyne.Container); ok {
			for _, child := range c.Objects {
				walk(child)
			}
		}
		if b, ok := co.(*container.Scroll); ok {
			walk(b.Content)
		}
	}
	walk(card)

	if angleEntry == nil {
		t.Fatalf("angle entry not found")
	}
	if allFramesCheck == nil {
		t.Fatalf("allFramesCheck not found")
	}
	if deleteBtn == nil {
		t.Fatalf("deleteBtn not found")
	}

	// Test modifying degrees
	angleEntry.SetText("90")
	if !modifiedCalled {
		t.Errorf("expected onModified to be called when angle changed")
	}
	if !strings.Contains(dir.Params, "deg: 90") {
		t.Errorf("expected dir.Params to contain 'deg: 90', got %q", dir.Params)
	}

	// Test checking All Frames
	modifiedCalled = false
	allFramesCheck.SetChecked(true)
	if !modifiedCalled {
		t.Errorf("expected onModified to be called when All Frames toggled")
	}
	if !strings.Contains(dir.Params, "f: all") {
		t.Errorf("expected dir.Params to contain 'f: all', got %q", dir.Params)
	}

	// Test delete button
	deleteBtn.Tapped(&fyne.PointEvent{})
	if !deleteCalled {
		t.Errorf("expected onDelete to be called when delete button tapped")
	}

	// Test copy button
	if copyBtn == nil {
		t.Fatalf("expected to find copy button in card")
	}
	copyBtn.Tapped(&fyne.PointEvent{})
	if !copyCalled {
		t.Errorf("expected onCopy to be called when copy button tapped")
	}

	// Test Depth: has frames, but no playback controls
	depthDir := &doctree.Directive{
		Type:   "Depth",
		Params: "f: 1-10; order: 2",
		Raw:    "Depth {f: 1-10; order: 2}",
	}
	depthCard := buildDirectiveWidgetCard(depthDir, 20, func() {}, nil, func() {}, func() {}, nil, nil).(*fyne.Container)
	var depthHasFrames, depthHasEase bool
	walk = func(co fyne.CanvasObject) {
		if co == nil {
			return
		}
		if ch, ok := co.(*widget.Check); ok && ch.Text == "All Frames" {
			depthHasFrames = true
		}
		if ch, ok := co.(*widget.Check); ok && ch.Text == "Ping-Pong" {
			depthHasEase = true
		}
		if c, ok := co.(*fyne.Container); ok {
			for _, child := range c.Objects {
				walk(child)
			}
		}
	}
	walk(depthCard)
	if !depthHasFrames {
		t.Errorf("expected Depth card to include frames row")
	}
	if depthHasEase {
		t.Errorf("expected Depth card to NOT include playback/ease row")
	}

	// Test Dist: has neither frames nor playback controls
	distDir := &doctree.Directive{
		Type:   "Dist",
		Params: "factor: 0.5",
		Raw:    "Dist {factor: 0.5}",
	}
	distCard := buildDirectiveWidgetCard(distDir, 20, func() {}, nil, func() {}, func() {}, nil, nil).(*fyne.Container)
	var distHasFrames, distHasEase bool
	walk = func(co fyne.CanvasObject) {
		if co == nil {
			return
		}
		if ch, ok := co.(*widget.Check); ok && ch.Text == "All Frames" {
			distHasFrames = true
		}
		if ch, ok := co.(*widget.Check); ok && ch.Text == "Ping-Pong" {
			distHasEase = true
		}
		if c, ok := co.(*fyne.Container); ok {
			for _, child := range c.Objects {
				walk(child)
			}
		}
	}
	walk(distCard)
	if distHasFrames {
		t.Errorf("expected Dist card to NOT include frames row")
	}
	if distHasEase {
		t.Errorf("expected Dist card to NOT include playback/ease row")
	}
}

func TestEditorState_IsDirty_ApplyPreset(t *testing.T) {
	cleanSVG := `<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape" width="100" height="100">
  <g id="layer1" inkscape:groupmode="layer" inkscape:label="Layer 1">
    <g id="g_rocket" inkscape:label="Rocket">
      <rect id="rect_rocket" width="10" height="20" inkscape:label="Body" />
    </g>
  </g>
</svg>`
	data := []byte(cleanSVG)

	state, err := NewEditorState(data, "clean.svg", []string{"g_rocket"})
	if err != nil {
		t.Fatalf("NewEditorState failed: %v", err)
	}

	if state.IsDirty() {
		t.Fatalf("expected state to be clean initially")
	}
	if len(state.History) != 0 {
		t.Fatalf("expected empty history initially, got %d", len(state.History))
	}

	spinPreset, ok := presets.FindPreset("spin")
	if !ok {
		t.Fatalf("spin preset not found")
	}

	_, err = state.ApplyPreset(spinPreset)
	if err != nil {
		t.Fatalf("ApplyPreset failed: %v", err)
	}

	if !state.IsDirty() {
		t.Errorf("expected state.IsDirty() == true after applying preset to group")
	}
	if len(state.History) != 1 {
		t.Fatalf("expected 1 history change, got %d", len(state.History))
	}
	if !strings.Contains(state.History[0].Description, "Spin") {
		t.Errorf("expected history description to mention Spin, got %q", state.History[0].Description)
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

func TestEditorState_UndoRedo_ApplyPreset(t *testing.T) {
	cleanSVG := `<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape" width="100" height="100">
  <g id="layer1" inkscape:groupmode="layer" inkscape:label="Layer 1">
    <g id="g_rocket" inkscape:label="Rocket">
      <rect id="rect_body" x="10" y="10" width="20" height="40" />
    </g>
  </g>
</svg>`
	data := []byte(cleanSVG)

	state, err := NewEditorState(data, "clean.svg", []string{"g_rocket"})
	if err != nil {
		t.Fatalf("NewEditorState failed: %v", err)
	}

	if state.CanUndo() {
		t.Errorf("expected CanUndo() == false initially")
	}
	if state.CanRedo() {
		t.Errorf("expected CanRedo() == false initially")
	}
	if state.HistoryIndex != -1 {
		t.Errorf("expected HistoryIndex == -1 initially, got %d", state.HistoryIndex)
	}

	spinPreset, ok := presets.FindPreset("spin")
	if !ok {
		t.Fatalf("spin preset not found")
	}

	anchorID, err := state.ApplyPreset(spinPreset)
	if err != nil {
		t.Fatalf("ApplyPreset failed: %v", err)
	}

	if !state.CanUndo() {
		t.Errorf("expected CanUndo() == true after preset")
	}
	if state.CanRedo() {
		t.Errorf("expected CanRedo() == false after preset")
	}
	if state.HistoryIndex != 0 {
		t.Errorf("expected HistoryIndex == 0, got %d", state.HistoryIndex)
	}
	if state.ActiveNode == nil || state.ActiveNode.ID != anchorID {
		t.Errorf("expected active node to be anchor %s, got %v", anchorID, state.ActiveNode)
	}
	if !state.IsDirty() {
		t.Errorf("expected state to be dirty after preset")
	}

	// Undo the preset
	desc, err := state.Undo()
	if err != nil {
		t.Fatalf("Undo failed: %v", err)
	}
	if !strings.Contains(desc, "Spin") {
		t.Errorf("expected undone description to mention Spin, got %q", desc)
	}
	if state.CanUndo() {
		t.Errorf("expected CanUndo() == false after undoing only change")
	}
	if !state.CanRedo() {
		t.Errorf("expected CanRedo() == true after undoing")
	}
	if state.HistoryIndex != -1 {
		t.Errorf("expected HistoryIndex == -1 after undo, got %d", state.HistoryIndex)
	}
	if state.IsDirty() {
		t.Errorf("expected state to be clean after undoing back to initial state")
	}
	if state.NodeMap[anchorID] != nil {
		t.Errorf("expected anchor %s to be removed from NodeMap after undo", anchorID)
	}
	if state.ActiveNode == nil || state.ActiveNode.ID != "g_rocket" {
		t.Errorf("expected active node restored to g_rocket, got %v", state.ActiveNode)
	}

	// Redo the preset
	redoneDesc, err := state.Redo()
	if err != nil {
		t.Fatalf("Redo failed: %v", err)
	}
	if !strings.Contains(redoneDesc, "Spin") {
		t.Errorf("expected redone description to mention Spin, got %q", redoneDesc)
	}
	if !state.CanUndo() {
		t.Errorf("expected CanUndo() == true after redo")
	}
	if state.CanRedo() {
		t.Errorf("expected CanRedo() == false after redo")
	}
	if state.HistoryIndex != 0 {
		t.Errorf("expected HistoryIndex == 0 after redo, got %d", state.HistoryIndex)
	}
	if !state.IsDirty() {
		t.Errorf("expected state to be dirty after redo")
	}
	if state.NodeMap[anchorID] == nil {
		t.Errorf("expected anchor %s restored in NodeMap after redo", anchorID)
	}
	if state.ActiveNode == nil || state.ActiveNode.ID != anchorID {
		t.Errorf("expected active node to be restored to %s, got %v", anchorID, state.ActiveNode)
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

func TestCommitEntry_Coalescing(t *testing.T) {
	var liveCalls int
	var committedVals []string

	onLive := func(s string) {
		liveCalls++
	}
	onCommit := func(s string) {
		committedVals = append(committedVals, s)
	}

	var undoCalls, redoCalls int
	onUndo := func() {
		undoCalls++
	}
	onRedo := func() {
		redoCalls++
	}

	entry := newCommitEntry("10", onLive, onCommit, onUndo, onRedo)
	if entry.Text != "10" {
		t.Errorf("expected initial text 10, got %q", entry.Text)
	}
	if liveCalls != 0 {
		t.Errorf("expected 0 live calls during construction, got %d", liveCalls)
	}
	if len(committedVals) != 0 {
		t.Errorf("expected 0 commit calls during construction, got %d", len(committedVals))
	}

	// Simulate user typing: "11", "12", "15"
	entry.SetText("11")
	entry.SetText("12")
	entry.SetText("15")

	if len(committedVals) != 0 {
		t.Errorf("expected no commits while typing, got %v", committedVals)
	}

	// Now simulate user typing via OnChanged without updating lastCommitted
	entry.Text = "20"
	entry.OnChanged("20")
	entry.Text = "25"
	entry.OnChanged("25")

	if len(committedVals) != 0 {
		t.Errorf("expected no commits during live typing, got %v", committedVals)
	}

	// Test Undo while entry has uncommitted text: reverts back to lastCommitted ("15")
	entry.TypedShortcut(&fyne.ShortcutUndo{})
	if entry.Text != "15" {
		t.Errorf("expected uncommitted text to revert to 15, got %q", entry.Text)
	}
	if undoCalls != 0 {
		t.Errorf("expected document undo NOT called when entry was dirty, got %d", undoCalls)
	}

	// FocusLost should commit once with current text ("15")
	entry.FocusLost()
	if len(committedVals) != 0 {
		// entry was reverted to "15" which was lastCommitted so no commit should be fired
		t.Logf("committedVals: %v", committedVals)
	}

	// Now modify text and commit via FocusLost
	entry.Text = "25"
	entry.FocusLost()
	if len(committedVals) != 1 || committedVals[0] != "25" {
		t.Fatalf("expected 1 commit with '25', got %v", committedVals)
	}

	// Subsequent FocusLost without edits should not duplicate commit
	entry.FocusLost()
	if len(committedVals) != 1 {
		t.Fatalf("expected still 1 commit, got %v", committedVals)
	}

	// Enter submission with edit
	entry.Text = "30"
	entry.OnSubmitted("30")
	if len(committedVals) != 2 || committedVals[1] != "30" {
		t.Fatalf("expected second commit with '30', got %v", committedVals)
	}

	// Test Undo when clean (text == lastCommitted): delegates to document onUndo
	entry.TypedShortcut(&fyne.ShortcutUndo{})
	if undoCalls != 1 {
		t.Errorf("expected document undo called once, got %d", undoCalls)
	}

	// Test Redo: delegates to document onRedo
	entry.TypedShortcut(&fyne.ShortcutRedo{})
	if redoCalls != 1 {
		t.Errorf("expected document redo called once, got %d", redoCalls)
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
	circleNode := state.NodeMap["circle"]
	if circleNode == nil || !circleNode.SyntheticID {
		t.Fatalf("expected synthetic ID node for circle")
	}
	canDel, _ = state.CanDeleteElement("circle")
	if canDel {
		t.Errorf("expected circle with synthetic ID to NOT be deletable")
	}

	// 3. Non-empty group (layer1 contains rect1 and circle): not deletable
	canDel, reason = state.CanDeleteElement("layer1")
	if canDel {
		t.Errorf("expected non-empty layer1 to NOT be deletable")
	}
	if !strings.Contains(reason, "Group contains") {
		t.Errorf("expected reason to mention child elements, got %q", reason)
	}

	// 4. Group containing only invisible <clipPath>: not deletable
	canDel, reason = state.CanDeleteElement("group_with_clip")
	if canDel {
		t.Errorf("expected group_with_clip to NOT be deletable")
	}
	if !strings.Contains(reason, "Group contains 1 element") {
		t.Errorf("expected reason to mention 1 element, got %q", reason)
	}

	// 5. Empty group: deletable
	canDel, reason = state.CanDeleteElement("empty_group")
	if !canDel {
		t.Errorf("expected empty_group to be deletable, got reason: %s", reason)
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

func TestEditorWindow_DeleteElementUI(t *testing.T) {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg">
  <g id="layer1">
    <rect id="rect1" width="10" height="10"/>
    <rect id="rect2" width="20" height="20"/>
  </g>
</svg>`)

	state, err := NewEditorState(svg, "test.svg", []string{"rect1"})
	if err != nil {
		t.Fatalf("NewEditorState failed: %v", err)
	}

	a := test.NewApp()
	w := ShowEditorWindow(a, state)
	defer w.Close()

	// Find the Delete button in the window
	var findDeleteBtn func(co fyne.CanvasObject) *widget.Button
	findDeleteBtn = func(co fyne.CanvasObject) *widget.Button {
		if btn, ok := co.(*widget.Button); ok && btn.Text == "Delete" && btn.Importance == widget.DangerImportance {
			return btn
		}
		if s, ok := co.(*container.Split); ok {
			if b := findDeleteBtn(s.Leading); b != nil {
				return b
			}
			return findDeleteBtn(s.Trailing)
		}
		if tabs, ok := co.(*container.AppTabs); ok {
			for _, item := range tabs.Items {
				if b := findDeleteBtn(item.Content); b != nil {
					return b
				}
			}
		}
		if c, ok := co.(*fyne.Container); ok {
			for _, child := range c.Objects {
				if b := findDeleteBtn(child); b != nil {
					return b
				}
			}
		}
		if scr, ok := co.(*container.Scroll); ok {
			return findDeleteBtn(scr.Content)
		}
		return nil
	}

	delBtn := findDeleteBtn(w.Content())
	if delBtn == nil {
		t.Fatalf("Delete button not found in UI inspector")
	}
	if delBtn.Disabled() {
		t.Errorf("expected Delete button to be enabled for leaf rect1")
	}

	// Trigger Delete button
	delBtn.Tapped(&fyne.PointEvent{})

	// Verify rect1 is deleted and layer1 is selected
	if state.NodeMap["rect1"] != nil {
		t.Errorf("expected rect1 deleted from state after clicking Delete")
	}
	if state.ActiveNode == nil || state.ActiveNode.ID != "layer1" {
		t.Errorf("expected layer1 to become active node, got %+v", state.ActiveNode)
	}

	// After deletion, active node is layer1 which still has child rect2, so Delete should be disabled!
	delBtn2 := findDeleteBtn(w.Content())
	if delBtn2 == nil {
		t.Fatalf("Delete button not found after update")
	}
	if !delBtn2.Disabled() {
		t.Errorf("expected Delete button to be disabled for non-empty layer1")
	}

	// Find deletableTree
	var delTree *deletableTree
	var findDelTree func(co fyne.CanvasObject)
	findDelTree = func(co fyne.CanvasObject) {
		if tr, ok := co.(*deletableTree); ok {
			delTree = tr
			return
		}
		if s, ok := co.(*container.Split); ok {
			findDelTree(s.Leading)
			findDelTree(s.Trailing)
		}
		if tabs, ok := co.(*container.AppTabs); ok {
			for _, item := range tabs.Items {
				findDelTree(item.Content)
			}
		}
		if c, ok := co.(*fyne.Container); ok {
			for _, child := range c.Objects {
				findDelTree(child)
			}
		}
	}
	findDelTree(w.Content())
	if delTree == nil {
		t.Fatalf("deletableTree not found in window")
	}

	// Select remaining leaf rect2
	delTree.Select("rect2")

	// Trigger Delete key on the tree!
	delTree.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDelete})

	// Verify rect2 was deleted via keyboard shortcut on tree
	if state.NodeMap["rect2"] != nil {
		t.Errorf("expected rect2 deleted after TypedKey(KeyDelete)")
	}
}

func TestEditorWindow_ApplyChanges_CleanVsDirty(t *testing.T) {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><rect id="r1" width="10"/></svg>`)

	// 1. Clean document: clicking Apply Changes sets Applied = false
	stateClean, err := NewEditorState(svg, "test.svg", []string{"r1"})
	if err != nil {
		t.Fatalf("NewEditorState failed: %v", err)
	}

	a := test.NewApp()
	wClean := ShowEditorWindow(a, stateClean)

	var findApplyBtn func(co fyne.CanvasObject) *widget.Button
	findApplyBtn = func(co fyne.CanvasObject) *widget.Button {
		if btn, ok := co.(*widget.Button); ok && btn.Text == "Apply Changes" {
			return btn
		}
		if c, ok := co.(*fyne.Container); ok {
			for _, child := range c.Objects {
				if b := findApplyBtn(child); b != nil {
					return b
				}
			}
		}
		return nil
	}

	applyBtnClean := findApplyBtn(wClean.Content())
	if applyBtnClean == nil {
		t.Fatalf("applyBtn not found in clean window")
	}

	applyBtnClean.Tapped(&fyne.PointEvent{})
	if stateClean.Applied {
		t.Errorf("expected Applied == false when clicking Apply Changes on clean document")
	}

	// 2. Dirty document: clicking Apply Changes sets Applied = true
	stateDirty, err := NewEditorState(svg, "test.svg", []string{"r1"})
	if err != nil {
		t.Fatalf("NewEditorState failed: %v", err)
	}
	stateDirty.NodeMap["r1"].LabelPrefix = "Dirty Prefix"
	stateDirty.ModifiedIDs["r1"] = true

	wDirty := ShowEditorWindow(a, stateDirty)
	applyBtnDirty := findApplyBtn(wDirty.Content())
	if applyBtnDirty == nil {
		t.Fatalf("applyBtn not found in dirty window")
	}

	applyBtnDirty.Tapped(&fyne.PointEvent{})
	if !stateDirty.Applied {
		t.Errorf("expected Applied == true when clicking Apply Changes on dirty document")
	}
}

func TestCopyMotions(t *testing.T) {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape">
  <path id="p1" inkscape:label="p1: Rot {f: 1-10; deg: 45} Scale {f: 1-10; scale: 1.5}" d="M0 0 L10 10" />
  <path id="p2" d="M0 0 L10 10" />
</svg>`)

	state, err := NewEditorState(svg, "test.svg", []string{"p1"})
	if err != nil {
		t.Fatalf("NewEditorState failed: %v", err)
	}

	// 1. CopyMotions returns all directives
	copied := state.CopyMotions(state.ActiveNode)
	if !strings.Contains(copied, "Rot {f: 1-10; deg: 45}") || !strings.Contains(copied, "Scale {f: 1-10; scale: 1.5}") {
		t.Errorf("unexpected copied directives: %q", copied)
	}
	if state.ClipboardDirectives != copied {
		t.Errorf("expected ClipboardDirectives to match copied string, got %q", state.ClipboardDirectives)
	}

	// 2. CopySingleDirective
	single := state.CopySingleDirective(state.ActiveNode.Directives[0])
	if single != "Rot {f: 1-10; deg: 45}" {
		t.Errorf("expected single directive 'Rot {f: 1-10; deg: 45}', got %q", single)
	}
	if state.ClipboardDirectives != single {
		t.Errorf("expected ClipboardDirectives to be single directive, got %q", state.ClipboardDirectives)
	}

	// 3. Copy on node without directives returns empty string
	p2 := state.NodeMap["p2"]
	if p2 == nil {
		t.Fatalf("p2 not found")
	}
	if res := state.CopyMotions(p2); res != "" {
		t.Errorf("expected empty string when copying from p2, got %q", res)
	}
}

func TestPasteMotions_Append(t *testing.T) {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape">
  <path id="p1" inkscape:label="p1: Rot {f: 1-10; deg: 45}" d="M0 0 L10 10" />
</svg>`)

	state, err := NewEditorState(svg, "test.svg", []string{"p1"})
	if err != nil {
		t.Fatalf("NewEditorState failed: %v", err)
	}

	// Paste a Scale directive
	resID, err := state.PasteMotions("Scale {f: 1-20; scale: 2.0}")
	if err != nil {
		t.Fatalf("PasteMotions failed: %v", err)
	}
	if resID != "p1" {
		t.Errorf("expected resulting ID 'p1', got %q", resID)
	}

	node := state.NodeMap["p1"]
	if len(node.Directives) != 2 {
		t.Fatalf("expected 2 directives on p1, got %d", len(node.Directives))
	}
	if node.Directives[0].Type != "Rot" || node.Directives[1].Type != "Scale" {
		t.Errorf("expected [Rot, Scale], got [%s, %s]", node.Directives[0].Type, node.Directives[1].Type)
	}

	// Check undo restores 1 directive
	if !state.CanUndo() {
		t.Fatalf("expected CanUndo == true after paste")
	}
	if _, err := state.Undo(); err != nil {
		t.Fatalf("Undo failed: %v", err)
	}
	node = state.NodeMap["p1"]
	if len(node.Directives) != 1 {
		t.Errorf("expected 1 directive after Undo, got %d", len(node.Directives))
	}
}

func TestPasteMotions_GroupAnchor(t *testing.T) {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape">
  <g id="layer1" inkscape:groupmode="layer">
    <rect id="r1" x="10" y="10" width="100" height="100" />
  </g>
</svg>`)

	state, err := NewEditorState(svg, "test.svg", []string{"r1"})
	if err != nil {
		t.Fatalf("NewEditorState failed: %v", err)
	}

	// Active node is r1 (child drawing shape in layer1 without directives).
	// Pasting should anchor to layer1 to avoid destroying r1.
	resID, err := state.PasteMotions("Move {f: 1-20; ease: in-out}")
	if err != nil {
		t.Fatalf("PasteMotions failed: %v", err)
	}
	if !strings.HasPrefix(resID, "layer1_motion") {
		t.Errorf("expected anchor ID starting with 'layer1_motion', got %q", resID)
	}
	if state.ActiveNode == nil || state.ActiveNode.ID != resID {
		t.Errorf("expected ActiveNode to be the newly created anchor %q", resID)
	}

	// Verify the anchor element exists in layer1
	patched := state.ComputePatchedSVG()
	if !strings.Contains(string(patched), resID) {
		t.Errorf("expected patched SVG to contain anchor %q", resID)
	}

	// Undo removes the anchor
	if _, err := state.Undo(); err != nil {
		t.Fatalf("Undo failed: %v", err)
	}
	if state.NodeMap[resID] != nil {
		t.Errorf("expected anchor %q to be removed after Undo", resID)
	}
}

func TestPasteMotions_PivotObject(t *testing.T) {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape">
  <g id="layer1" inkscape:groupmode="layer">
    <circle id="orig_pin" cx="50" cy="50" r="5" />
    <path id="arm" inkscape:label="arm: Rot {f: 1-20; deg: 90; pivot: #orig_pin}" d="M 50 50 L 100 50" />
  </g>
  <g id="layer2" inkscape:groupmode="layer">
    <path id="leg" d="M 0 0 L 20 20" />
  </g>
</svg>`)

	state, err := NewEditorState(svg, "test.svg", []string{"arm"})
	if err != nil {
		t.Fatalf("NewEditorState failed: %v", err)
	}

	// Copy directives from arm
	copied := state.CopyMotions(state.ActiveNode)
	if !strings.Contains(copied, "pivot: #orig_pin") {
		t.Fatalf("expected copied motions to contain 'pivot: #orig_pin', got %q", copied)
	}

	// Select leg in layer2
	state.ActiveNode = state.NodeMap["leg"]

	// Paste onto leg (child in layer2 without directives -> anchors in layer2)
	anchorID, err := state.PasteMotions(copied)
	if err != nil {
		t.Fatalf("PasteMotions failed: %v", err)
	}

	anchorNode := state.NodeMap[anchorID]
	if anchorNode == nil {
		t.Fatalf("expected anchor node %q to exist", anchorID)
	}
	if len(anchorNode.Directives) != 1 {
		t.Fatalf("expected 1 directive on anchor, got %d", len(anchorNode.Directives))
	}

	pastedDir := anchorNode.Directives[0]
	// Directive should reference the newly created pivot in layer2, NOT orig_pin!
	if strings.Contains(pastedDir.Params, "#orig_pin") {
		t.Errorf("expected pivot reference to be rewritten, still contains '#orig_pin': %s", pastedDir.Params)
	}
	if !strings.Contains(pastedDir.Params, "pivot: #layer2_pivot") {
		t.Errorf("expected pivot reference to point to new layer2 pivot, got: %s", pastedDir.Params)
	}

	// Verify the new pivot element exists in the tree at the measured coordinates
	pivotNode := state.NodeMap["layer2_pivot"]
	if pivotNode == nil {
		t.Fatalf("expected new pivot node 'layer2_pivot' to exist in nodeMap")
	}

	// Undo should remove both the new pivot and the new anchor in one step
	if _, err := state.Undo(); err != nil {
		t.Fatalf("Undo failed: %v", err)
	}
	if state.NodeMap[anchorID] != nil {
		t.Errorf("expected anchor %q to be gone after Undo", anchorID)
	}
	if state.NodeMap["layer2_pivot"] != nil {
		t.Errorf("expected pivot 'layer2_pivot' to be gone after Undo", )
	}
}

func TestCopyPasteWindowShortcuts(t *testing.T) {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape">
  <path id="p1" inkscape:label="p1: Rot {f: 1-10; deg: 45}" d="M0 0 L10 10" />
  <path id="p2" inkscape:label="p2: Fade {f: 1-10; from: 0; to: 1}" d="M0 0 L10 10" />
</svg>`)

	a := test.NewApp()
	state, err := NewEditorState(svg, "test.svg", []string{"p1"})
	if err != nil {
		t.Fatalf("NewEditorState failed: %v", err)
	}

	w := ShowEditorWindow(a, state)
	mainMenu := w.MainMenu()
	if mainMenu == nil || len(mainMenu.Items) == 0 {
		t.Fatalf("expected mainMenu")
	}

	editMenu := mainMenu.Items[0]
	var copyItem, pasteItem *fyne.MenuItem
	for _, item := range editMenu.Items {
		if item.Label == "Copy Motions" {
			copyItem = item
		}
		if item.Label == "Paste Motions" {
			pasteItem = item
		}
	}

	if copyItem == nil {
		t.Fatalf("expected Copy Motions item in editMenu")
	}
	if pasteItem == nil {
		t.Fatalf("expected Paste Motions item in editMenu")
	}

	// 1. Trigger Copy Motions menu item
	copyItem.Action()
	if !strings.Contains(state.ClipboardDirectives, "Rot {f: 1-10; deg: 45}") {
		t.Errorf("expected ClipboardDirectives to contain copied Rot directive, got %q", state.ClipboardDirectives)
	}

	// 2. Select p2 and trigger Paste Motions menu item
	state.ActiveNode = state.NodeMap["p2"]
	pasteItem.Action()

	p2 := state.NodeMap["p2"]
	if len(p2.Directives) != 2 {
		t.Fatalf("expected p2 to have 2 directives (Fade + Rot), got %d", len(p2.Directives))
	}
	if p2.Directives[0].Type != "Fade" || p2.Directives[1].Type != "Rot" {
		t.Errorf("expected [Fade, Rot], got [%s, %s]", p2.Directives[0].Type, p2.Directives[1].Type)
	}
}





