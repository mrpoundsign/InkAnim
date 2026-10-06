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

