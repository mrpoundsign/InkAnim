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
	origPath := filepath.Join("..", "..", "..", "testdata", "spline_test.svg")
	origBytes, err := os.ReadFile(origPath)
	if err != nil {
		t.Fatalf("failed to read spline_test.svg: %v", err)
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
