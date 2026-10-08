package svgpatch

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"inkanim/pkg/inksvg"
)

func TestSetAttr_SplineTest(t *testing.T) {
	origPath := filepath.Join("..", "..", "..", "testdata", "migrations", "movement_current.svg")
	origBytes, err := os.ReadFile(origPath)
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}

	newLabel := "Move {f: 1-20; ease: in-out} · Spike"

	// (a) and (b): Update existing label on path_motion
	patchedBytes, err := SetAttr(origBytes, "path_motion", InkscapeNS, "label", newLabel)
	if err != nil {
		t.Fatalf("SetAttr failed: %v", err)
	}

	// (a) Assert only bytes inside that attribute changed
	// Locate old label in origBytes
	oldLabel := "Move {f: 1-20; ease: in-out}"
	oldIdx := bytes.Index(origBytes, []byte(oldLabel))
	if oldIdx == -1 {
		t.Fatalf("could not find old label %q in original bytes", oldLabel)
	}
	prefix := origBytes[:oldIdx]
	suffix := origBytes[oldIdx+len(oldLabel):]

	if !bytes.HasPrefix(patchedBytes, prefix) {
		t.Errorf("prefix before attribute value was modified")
	}
	if !bytes.HasSuffix(patchedBytes, suffix) {
		t.Errorf("suffix after attribute value was modified")
	}
	expectedPatched := append(append(append([]byte{}, prefix...), []byte(newLabel)...), suffix...)
	if !bytes.Equal(patchedBytes, expectedPatched) {
		t.Errorf("patched bytes do not exactly match prefix + newLabel + suffix")
	}

	// (b) Assert re-parsing yields the new label
	gotLabel, ok := GetAttr(patchedBytes, "path_motion", "label")
	if !ok || gotLabel != newLabel {
		t.Errorf("GetAttr on patchedBytes returned (%q, %v), want (%q, true)", gotLabel, ok, newLabel)
	}
	doc, err := inksvg.ParseSVG(patchedBytes)
	if err != nil {
		t.Fatalf("failed to parse patched SVG with inksvg: %v", err)
	}
	var foundPath bool
	for _, mp := range doc.MotionPaths {
		if mp.ID == "path_motion" {
			foundPath = true
			if mp.Config.Type != "move" || mp.Config.StartFrame != 1 || mp.Config.EndFrame != 20 {
				t.Errorf("unexpected motion config parsed from new label: %+v", mp.Config)
			}
		}
	}
	if !foundPath {
		t.Errorf("motion path path_motion not found in re-parsed doc")
	}

	// (c) Element without an existing label gets one inserted (e.g. circle 'path2')
	circleLabel, circleHadLabel := GetAttr(origBytes, "path2", "label")
	if circleHadLabel {
		t.Fatalf("expected circle 'path2' to not have a label initially, got %q", circleLabel)
	}
	insertedBytes, err := SetAttr(origBytes, "path2", InkscapeNS, "label", "Test Circle Label")
	if err != nil {
		t.Fatalf("failed to insert label on path2: %v", err)
	}
	gotCircleLabel, ok := GetAttr(insertedBytes, "path2", "label")
	if !ok || gotCircleLabel != "Test Circle Label" {
		t.Errorf("inserted label on path2 = (%q, %v), want ('Test Circle Label', true)", gotCircleLabel, ok)
	}
	if !strings.Contains(string(insertedBytes), `inkscape:label="Test Circle Label"`) {
		t.Errorf("expected insertedBytes to contain inkscape:label=\"Test Circle Label\"")
	}

	// (d) Unknown ID returns an error and input unchanged
	nonExistentBytes, err := SetAttr(origBytes, "non_existent_element_id_12345", InkscapeNS, "label", "abc")
	if err == nil {
		t.Errorf("expected error for non-existent ID, got nil")
	}
	if !bytes.Equal(nonExistentBytes, origBytes) {
		t.Errorf("expected output bytes to be unchanged on error")
	}
}

func TestSetAttr_SingleQuoteAndSelfClosing(t *testing.T) {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape"><rect id="r1" inkscape:label='old' width="10" height="10"/><circle id="c1" r="5"/></svg>`)
	patched, err := SetAttr(svg, "r1", InkscapeNS, "label", "new")
	if err != nil {
		t.Fatalf("SetAttr failed: %v", err)
	}
	if !strings.Contains(string(patched), "inkscape:label='new'") {
		t.Errorf("expected patched to preserve single quotes, got: %s", string(patched))
	}

	patchedC, err := SetAttr(svg, "c1", InkscapeNS, "label", "circle_label")
	if err != nil {
		t.Fatalf("SetAttr insert failed: %v", err)
	}
	val, ok := GetAttr(patchedC, "c1", "label")
	if !ok || val != "circle_label" {
		t.Errorf("GetAttr = (%q, %v), want ('circle_label', true)", val, ok)
	}
}

func TestSetAttr_CRLF(t *testing.T) {
	origPath := filepath.Join("..", "..", "..", "testdata", "spline_test.svg")
	origBytes, err := os.ReadFile(origPath)
	if err != nil {
		t.Fatalf("failed to read spline_test.svg: %v", err)
	}
	crlfBytes := bytes.ReplaceAll(origBytes, []byte("\n"), []byte("\r\n"))
	patched, err := SetAttr(crlfBytes, "path_motion", InkscapeNS, "label", "Move {f: 1-20; ease: in-out} · Spike")
	if err != nil {
		t.Fatalf("SetAttr on CRLF failed: %v", err)
	}
	val, ok := GetAttr(patched, "path_motion", "label")
	if !ok || val != "Move {f: 1-20; ease: in-out} · Spike" {
		t.Errorf("GetAttr on CRLF patched = (%q, %v)", val, ok)
	}
}

func TestInsertChild(t *testing.T) {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape">
  <g id="layer1" inkscape:label="Layer 1">
    <rect id="r1" width="10" height="10"/>
  </g>
  <g id="self_closing" inkscape:label="Empty Group"/>
</svg>`)

	childDot := `<circle id="layer1_anchor" cx="5" cy="5" r="1" style="fill:none;stroke:none" inkscape:label="Spin: Rot {f: 1-20; deg: 360}"/>`

	// Insert into regular group
	patched, err := InsertChild(svg, "layer1", childDot)
	if err != nil {
		t.Fatalf("InsertChild on layer1 failed: %v", err)
	}
	if !strings.Contains(string(patched), "layer1_anchor") {
		t.Fatalf("patched SVG does not contain inserted child ID: %s", string(patched))
	}
	// Parse with inksvg
	doc, err := inksvg.ParseSVG(patched)
	if err != nil {
		t.Fatalf("failed to parse patched SVG: %v", err)
	}
	var foundAnchor bool
	for _, mp := range doc.MotionPaths {
		if mp.ID == "layer1_anchor" {
			foundAnchor = true
			if mp.GroupID != "layer1" {
				t.Errorf("expected anchor GroupID 'layer1', got %q", mp.GroupID)
			}
			if mp.Config.Type != "rot" || mp.Config.RotationAngle != 360 {
				t.Errorf("unexpected anchor config: %+v", mp.Config)
			}
		}
	}
	if !foundAnchor {
		t.Errorf("inserted anchor motion path not found in parsed doc")
	}

	// Insert into self-closing group
	patchedSelfClosing, err := InsertChild(svg, "self_closing", childDot)
	if err != nil {
		t.Fatalf("InsertChild on self_closing failed: %v", err)
	}
	if !strings.Contains(string(patchedSelfClosing), "layer1_anchor") {
		t.Fatalf("patched SVG does not contain inserted child ID: %s", string(patchedSelfClosing))
	}
	if !strings.Contains(string(patchedSelfClosing), "</g>") {
		t.Fatalf("patched SVG does not contain closing </g>: %s", string(patchedSelfClosing))
	}

	// Non-existent parent ID
	_, err = InsertChild(svg, "non_existent_id", childDot)
	if err == nil {
		t.Errorf("expected error for non-existent parent, got nil")
	}
}

func TestRemoveElement_LeafAndGroup(t *testing.T) {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg">
  <g id="layer1">
    <!-- A comment -->
    <rect id="rect1" width="10" height="10"/>
    <circle id="circle1" r="5"/>
  </g>
  <g id="empty_group">
  </g>
</svg>`)

	// 1. Remove self-closing leaf element rect1
	patched1, err := RemoveElement(svg, "rect1")
	if err != nil {
		t.Fatalf("RemoveElement(rect1) failed: %v", err)
	}
	if strings.Contains(string(patched1), "rect1") {
		t.Errorf("expected rect1 to be removed from SVG: %s", string(patched1))
	}
	if !strings.Contains(string(patched1), "circle1") {
		t.Errorf("expected circle1 to remain in SVG: %s", string(patched1))
	}
	if !strings.Contains(string(patched1), "<!-- A comment -->") {
		t.Errorf("expected comment to remain in SVG")
	}
	// Verify no blank line between comment and circle1
	expected1 := `<svg xmlns="http://www.w3.org/2000/svg">
  <g id="layer1">
    <!-- A comment -->
    <circle id="circle1" r="5"/>
  </g>
  <g id="empty_group">
  </g>
</svg>`
	if string(patched1) != expected1 {
		t.Errorf("unexpected patched1 content:\nGot:\n%s\nWant:\n%s", string(patched1), expected1)
	}

	// 2. Remove open/close empty group
	patched2, err := RemoveElement(patched1, "empty_group")
	if err != nil {
		t.Fatalf("RemoveElement(empty_group) failed: %v", err)
	}
	if strings.Contains(string(patched2), "empty_group") {
		t.Errorf("expected empty_group to be removed from SVG: %s", string(patched2))
	}
	expected2 := `<svg xmlns="http://www.w3.org/2000/svg">
  <g id="layer1">
    <!-- A comment -->
    <circle id="circle1" r="5"/>
  </g>
</svg>`
	if string(patched2) != expected2 {
		t.Errorf("unexpected patched2 content:\nGot:\n%s\nWant:\n%s", string(patched2), expected2)
	}

	// 3. Remove group with children (nested subtree deletion)
	patched3, err := RemoveElement(svg, "layer1")
	if err != nil {
		t.Fatalf("RemoveElement(layer1) failed: %v", err)
	}
	if strings.Contains(string(patched3), "layer1") || strings.Contains(string(patched3), "rect1") || strings.Contains(string(patched3), "circle1") {
		t.Errorf("expected entire layer1 subtree to be removed: %s", string(patched3))
	}

	// 4. Missing ID
	_, err = RemoveElement(svg, "non_existent")
	if err == nil {
		t.Errorf("expected error for non_existent element ID, got nil")
	}
}

func TestRemoveElement_CRLF(t *testing.T) {
	crlfSVG := []byte("<svg xmlns=\"http://www.w3.org/2000/svg\">\r\n  <rect id=\"r1\" width=\"10\"/>\r\n  <rect id=\"r2\" width=\"20\"/>\r\n</svg>")
	patched, err := RemoveElement(crlfSVG, "r1")
	if err != nil {
		t.Fatalf("RemoveElement failed on CRLF SVG: %v", err)
	}
	expected := "<svg xmlns=\"http://www.w3.org/2000/svg\">\r\n  <rect id=\"r2\" width=\"20\"/>\r\n</svg>"
	if string(patched) != expected {
		t.Errorf("unexpected CRLF patched output:\nGot:\n%q\nWant:\n%q", string(patched), expected)
	}
}

func TestRemoveElement_SingleQuotes(t *testing.T) {
	sqSVG := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><rect id='r1' width='10'/><rect id="r2"/></svg>`)
	patched, err := RemoveElement(sqSVG, "r1")
	if err != nil {
		t.Fatalf("RemoveElement failed with single quotes: %v", err)
	}
	expected := `<svg xmlns="http://www.w3.org/2000/svg"><rect id="r2"/></svg>`
	if string(patched) != expected {
		t.Errorf("unexpected output:\nGot: %q\nWant: %q", string(patched), expected)
	}
}

func TestChildElementCount_And_HasChildElements(t *testing.T) {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg">
  <g id="empty_pair"></g>
  <g id="empty_whitespace">
  </g>
  <g id="self_closing_group" />
  <g id="group_with_clip">
    <clipPath id="cp1">
      <rect width="10" height="10"/>
    </clipPath>
  </g>
  <g id="group_with_visible">
    <path id="p1" d="M 0,0 L 10,10"/>
    <circle id="c1" r="5"/>
  </g>
  <rect id="leaf_rect" width="5" height="5"/>
</svg>`)

	// 1. Leaf element
	count, err := ChildElementCount(svg, "leaf_rect")
	if err != nil || count != 0 {
		t.Errorf("leaf_rect count = (%d, %v), want (0, nil)", count, err)
	}
	hasChildren, err := HasChildElements(svg, "leaf_rect")
	if err != nil || hasChildren {
		t.Errorf("leaf_rect HasChildElements = (%v, %v), want (false, nil)", hasChildren, err)
	}

	// 2. Empty group tags
	count, err = ChildElementCount(svg, "empty_pair")
	if err != nil || count != 0 {
		t.Errorf("empty_pair count = (%d, %v), want (0, nil)", count, err)
	}
	count, err = ChildElementCount(svg, "empty_whitespace")
	if err != nil || count != 0 {
		t.Errorf("empty_whitespace count = (%d, %v), want (0, nil)", count, err)
	}
	count, err = ChildElementCount(svg, "self_closing_group")
	if err != nil || count != 0 {
		t.Errorf("self_closing_group count = (%d, %v), want (0, nil)", count, err)
	}

	// 3. Group containing only invisible <clipPath>
	count, err = ChildElementCount(svg, "group_with_clip")
	if err != nil || count != 1 {
		t.Errorf("group_with_clip count = (%d, %v), want (1, nil)", count, err)
	}
	hasChildren, err = HasChildElements(svg, "group_with_clip")
	if err != nil || !hasChildren {
		t.Errorf("group_with_clip HasChildElements = (%v, %v), want (true, nil)", hasChildren, err)
	}

	// 4. Group with visible elements
	count, err = ChildElementCount(svg, "group_with_visible")
	if err != nil || count != 2 {
		t.Errorf("group_with_visible count = (%d, %v), want (2, nil)", count, err)
	}
	hasChildren, err = HasChildElements(svg, "group_with_visible")
	if err != nil || !hasChildren {
		t.Errorf("group_with_visible HasChildElements = (%v, %v), want (true, nil)", hasChildren, err)
	}

	// 5. Non-existent ID
	_, err = ChildElementCount(svg, "does_not_exist")
	if err == nil {
		t.Errorf("expected error for non-existent ID, got nil")
	}
}


