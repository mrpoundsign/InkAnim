package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
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
	var deleteCalled bool
	onDelete := func() {
		deleteCalled = true
	}

	cardObj := buildDirectiveWidgetCard(dir, 20, onModified, onDelete)
	card, ok := cardObj.(*fyne.Container)
	if !ok {
		t.Fatalf("expected card to be *fyne.Container")
	}

	// Find the angle entry inside card
	var angleEntry *widget.Entry
	var allFramesCheck *widget.Check
	var deleteBtn *widget.Button

	var walk func(co fyne.CanvasObject)
	walk = func(co fyne.CanvasObject) {
		if co == nil {
			return
		}
		if e, ok := co.(*widget.Entry); ok {
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

	// Test Depth: has frames, but no playback controls
	depthDir := &doctree.Directive{
		Type:   "Depth",
		Params: "f: 1-10; order: 2",
		Raw:    "Depth {f: 1-10; order: 2}",
	}
	depthCard := buildDirectiveWidgetCard(depthDir, 20, func() {}, func() {}).(*fyne.Container)
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
	distCard := buildDirectiveWidgetCard(distDir, 20, func() {}, func() {}).(*fyne.Container)
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



