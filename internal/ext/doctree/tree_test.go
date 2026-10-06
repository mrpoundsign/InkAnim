package doctree

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseTree_SplineTest(t *testing.T) {
	splinePath := filepath.Join("..", "..", "..", "testdata", "spline_test.svg")
	data, err := os.ReadFile(splinePath)
	if err != nil {
		t.Fatalf("failed to read spline_test.svg: %v", err)
	}

	roots, nodeMap, err := ParseTree(data)
	if err != nil {
		t.Fatalf("ParseTree failed: %v", err)
	}

	if len(roots) == 0 {
		t.Fatalf("expected root nodes, got 0")
	}

	// Verify layer1 exists and is marked as layer
	layer1, ok := nodeMap["layer1"]
	if !ok {
		t.Fatalf("expected node layer1 in nodeMap")
	}
	if !layer1.IsLayer {
		t.Errorf("expected layer1.IsLayer to be true")
	}

	// Verify path_motion exists and has directive
	pathMotion, ok := nodeMap["path_motion"]
	if !ok {
		t.Fatalf("expected path_motion in nodeMap")
	}
	if len(pathMotion.Directives) != 1 {
		t.Fatalf("expected 1 directive on path_motion, got %d", len(pathMotion.Directives))
	}
	d := pathMotion.Directives[0]
	if d.Type != "Move" {
		t.Errorf("expected directive Type 'Move', got %q", d.Type)
	}
	if d.Params != "f: 1-20; ease: in-out" {
		t.Errorf("expected directive Params 'f: 1-20; ease: in-out', got %q", d.Params)
	}

	// Verify layer2 and path3
	path3, ok := nodeMap["path3"]
	if !ok {
		t.Fatalf("expected path3 in nodeMap")
	}
	if len(path3.Directives) != 1 {
		t.Fatalf("expected 1 directive on path3, got %d", len(path3.Directives))
	}
	if path3.Directives[0].Params != "f:all" {
		t.Errorf("expected 'f:all', got %q", path3.Directives[0].Params)
	}
}

func TestParseDirectives(t *testing.T) {
	label := "Intro: Fade {f: 1-10; from: 0; to: 1} Rot {f: 1-20; angle: 360} · Tag"
	prefix, directives, suffix := ParseDirectives(label)

	if prefix != "Intro:" {
		t.Errorf("expected prefix 'Intro:', got %q", prefix)
	}
	if suffix != "· Tag" {
		t.Errorf("expected suffix '· Tag', got %q", suffix)
	}
	if len(directives) != 2 {
		t.Fatalf("expected 2 directives, got %d", len(directives))
	}
	if directives[0].Type != "Fade" || directives[0].Params != "f: 1-10; from: 0; to: 1" {
		t.Errorf("unexpected directive 0: %+v", directives[0])
	}
	if directives[1].Type != "Rot" || directives[1].Params != "f: 1-20; angle: 360" {
		t.Errorf("unexpected directive 1: %+v", directives[1])
	}
}

func TestFormatLabel(t *testing.T) {
	node := &DocNode{
		LabelPrefix: "Hero",
		Directives: []Directive{
			{Raw: "Move {f: 1-30; ease: in-out}"},
			{Raw: "Scale {f: 1-15; from: 1; to: 2}"},
		},
		LabelSuffix: "· Highlight",
	}

	formatted := node.FormatLabel()
	expected := "Hero Move {f: 1-30; ease: in-out} Scale {f: 1-15; from: 1; to: 2} · Highlight"
	if formatted != expected {
		t.Errorf("expected %q, got %q", expected, formatted)
	}
}

func TestNamePreservation_UnlabeledObject(t *testing.T) {
	svgContent := `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape">
  <g id="layer1" inkscape:groupmode="layer" inkscape:label="Layer 1">
    <rect id="rect_rocket" x="10" y="10" width="50" height="50" />
  </g>
</svg>`

	_, nodeMap, err := ParseTree([]byte(svgContent))
	if err != nil {
		t.Fatalf("ParseTree failed: %v", err)
	}

	rectNode, ok := nodeMap["rect_rocket"]
	if !ok {
		t.Fatalf("expected rect_rocket in nodeMap")
	}

	// Should default prefix to object ID when unlabeled
	if rectNode.LabelPrefix != "rect_rocket" {
		t.Errorf("expected LabelPrefix 'rect_rocket', got %q", rectNode.LabelPrefix)
	}
	if rectNode.DisplayTitle() != "rect_rocket" {
		t.Errorf("expected DisplayTitle 'rect_rocket', got %q", rectNode.DisplayTitle())
	}

	// When 1st motion is added, formatted label preserves the name
	rectNode.Directives = append(rectNode.Directives, Directive{
		Type:   "Move",
		Params: "f: 1-20; ease: in-out",
		Raw:    "Move {f: 1-20; ease: in-out}",
	})

	formatted := rectNode.FormatLabel()
	expected := "rect_rocket Move {f: 1-20; ease: in-out}"
	if formatted != expected {
		t.Errorf("expected label %q, got %q", expected, formatted)
	}
	if rectNode.DisplayTitle() != "rect_rocket" {
		t.Errorf("DisplayTitle should remain 'rect_rocket', got %q", rectNode.DisplayTitle())
	}
}

func TestDisplayTitle_ExistingMotions(t *testing.T) {
	node := &DocNode{
		ID:    "path_motion",
		Label: "Move {f: 1-20}",
		Directives: []Directive{
			{Type: "Move", Params: "f: 1-20", Raw: "Move {f: 1-20}"},
		},
	}
	// Object whose label was purely motion should still show its ID in the tree
	if node.DisplayTitle() != "path_motion" {
		t.Errorf("expected DisplayTitle 'path_motion', got %q", node.DisplayTitle())
	}
}

func TestVisualZOrder(t *testing.T) {
	splinePath := filepath.Join("..", "..", "..", "testdata", "spline_test.svg")
	data, err := os.ReadFile(splinePath)
	if err != nil {
		t.Fatalf("failed to read spline_test.svg: %v", err)
	}

	roots, _, err := ParseTree(data)
	if err != nil {
		t.Fatalf("ParseTree failed: %v", err)
	}

	if len(roots) != 2 {
		t.Fatalf("expected 2 roots, got %d", len(roots))
	}

	// Layer 2 is top-most in z-order, Layer 1 is bottom-most
	if roots[0].ID != "layer2" {
		t.Errorf("expected top-most layer 'layer2' at root[0], got %q", roots[0].ID)
	}
	if roots[1].ID != "layer1" {
		t.Errorf("expected bottom-most layer 'layer1' at root[1], got %q", roots[1].ID)
	}
}


