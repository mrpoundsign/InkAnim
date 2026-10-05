package inksvg

import (
	"math"
	"os"
	"strings"
	"testing"
)

func TestEvaluatePathAt_Lines(t *testing.T) {
	// A simple path of 3 connected lines: (0,0) -> (100,0) -> (100,100) -> (0,100)
	// Total length: 300
	pathData := "M 0,0 L 100,0 L 100,100 L 0,100"

	// At t=0, we should be at (0,0)
	// At t=1/3, we should be at (100,0)
	// At t=2/3, we should be at (100,100)
	// At t=1, we should be at (0,100)

	tests := []struct {
		t       float64
		wantX   float64
		wantY   float64
		tol     float64
	}{
		{0.0, 0, 0, 0.001},
		{1.0 / 3.0, 100, 0, 0.001},
		{0.5, 100, 50, 0.001},
		{2.0 / 3.0, 100, 100, 0.001},
		{1.0, 0, 100, 0.001},
	}

	for _, tt := range tests {
		gotX, gotY, err := EvaluatePathAt(pathData, tt.t)
		if err != nil {
			t.Fatalf("EvaluatePathAt(%f) returned error: %v", tt.t, err)
		}
		if math.Abs(gotX-tt.wantX) > tt.tol || math.Abs(gotY-tt.wantY) > tt.tol {
			t.Errorf("EvaluatePathAt(%f) = (%f, %f), want (%f, %f)", tt.t, gotX, gotY, tt.wantX, tt.wantY)
		}
	}
}

func TestEvaluatePathAt_RelativeCommands(t *testing.T) {
	pathData := "m 10,10 l 90,0 l 0,100"

	gotX, gotY, err := EvaluatePathAt(pathData, 1.0)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	wantX, wantY := 90.0, 100.0
	if math.Abs(gotX-wantX) > 0.001 || math.Abs(gotY-wantY) > 0.001 {
		t.Errorf("EvaluatePathAt(1.0) = (%f, %f), want (%f, %f)", gotX, gotY, wantX, wantY)
	}
}

func TestParseMotionConfig_MoveAndRot(t *testing.T) {
	tests := []struct {
		label      string
		wantOK     bool
		wantType   string
		wantStart  int
		wantEnd    int
		wantAll    bool
		wantEase   string
		wantRev    bool
		wantAngle  float64
		wantDir    string
		wantOrient bool
		wantPivot      string
		wantEdge       float64
		wantNode       string
		wantScaleFromX float64
		wantScaleFromY float64
		wantScaleToX   float64
		wantScaleToY   float64
	}{
		{
			label:     "Move {f:1-15}",
			wantOK:    true,
			wantType:  "move",
			wantStart: 1,
			wantEnd:   15,
			wantEase:  "linear",
			wantDir:   "cw",
			wantPivot: "center",
		},
		{
			label:     "Move {f:1-15 rev}",
			wantOK:    true,
			wantType:  "move",
			wantStart: 1,
			wantEnd:   15,
			wantEase:  "linear",
			wantRev:   true,
			wantDir:   "cw",
			wantPivot: "center",
		},
		{
			label:     "Move {f:1-15 ease:in-out rev orient:true}",
			wantOK:    true,
			wantType:  "move",
			wantStart: 1,
			wantEnd:   15,
			wantEase:  "in-out",
			wantRev:   true,
			wantOrient: true,
			wantDir:   "cw",
			wantPivot: "center",
		},
		{
			label:     "Rot {f:1-60 angle:360 dir:ccw pivot:center}",
			wantOK:    true,
			wantType:  "rot",
			wantStart: 1,
			wantEnd:   60,
			wantEase:  "linear",
			wantAngle: 360,
			wantDir:   "ccw",
			wantPivot: "center",
		},
		{
			label:     "Rot {f:all ease:out angle:90 pivot:0}", // 0 = top edge
			wantOK:    true,
			wantType:  "rot",
			wantAll:   true,
			wantEase:  "out",
			wantAngle: 90,
			wantDir:   "cw",
			wantPivot: "edge",
			wantEdge:  0,
		},
		{
			label:     "Rot {angle:45 pivot:#arm_joint}", // omitted f defaults to all
			wantOK:    true,
			wantType:  "rot",
			wantAll:   true,
			wantEase:  "linear",
			wantAngle: 45,
			wantDir:   "cw",
			wantPivot: "node",
			wantNode:  "arm_joint",
		},
		{
			label:     "Rot {pivot:path-start}",
			wantOK:    true,
			wantType:  "rot",
			wantAll:   true,
			wantEase:  "linear",
			wantDir:   "cw",
			wantPivot: "path-start",
		},
		{
			label:          "Scale {f:1-30 scale:1.5}",
			wantOK:         true,
			wantType:       "scale",
			wantStart:      1,
			wantEnd:        30,
			wantEase:       "linear",
			wantDir:        "cw",
			wantPivot:      "center",
			wantScaleFromX: 1.0,
			wantScaleFromY: 1.0,
			wantScaleToX:   1.5,
			wantScaleToY:   1.5,
		},
		{
			label:          "Scal {f:1-30 ease:in-out from:0.5 to:1.5 pivot:180}",
			wantOK:         true,
			wantType:       "scale",
			wantStart:      1,
			wantEnd:        30,
			wantEase:       "in-out",
			wantDir:        "cw",
			wantPivot:      "edge",
			wantEdge:       180,
			wantScaleFromX: 0.5,
			wantScaleFromY: 0.5,
			wantScaleToX:   1.5,
			wantScaleToY:   1.5,
		},
		{
			label:          "Scale {scale-x:1.2 scale-y:0.8 pivot:center}",
			wantOK:         true,
			wantType:       "scale",
			wantAll:        true,
			wantEase:       "linear",
			wantDir:        "cw",
			wantPivot:      "center",
			wantScaleFromX: 1.0,
			wantScaleFromY: 1.0,
			wantScaleToX:   1.2,
			wantScaleToY:   0.8,
		},
		{
			label:          "Scale {from-x:0.8 to-x:1.4 from-y:1.2 to-y:0.6 pivot:#pin}",
			wantOK:         true,
			wantType:       "scale",
			wantAll:        true,
			wantEase:       "linear",
			wantDir:        "cw",
			wantPivot:      "node",
			wantNode:       "pin",
			wantScaleFromX: 0.8,
			wantScaleFromY: 1.2,
			wantScaleToX:   1.4,
			wantScaleToY:   0.6,
		},
		{
			label:  "Regular Label",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		cfg, ok := parseMotionConfig(tt.label)
		if ok != tt.wantOK {
			t.Errorf("[%s] parseMotionConfig ok = %v, want %v", tt.label, ok, tt.wantOK)
			continue
		}
		if !tt.wantOK {
			continue
		}
		if cfg.Type != tt.wantType {
			t.Errorf("[%s] type = %s, want %s", tt.label, cfg.Type, tt.wantType)
		}
		if cfg.StartFrame != tt.wantStart || cfg.EndFrame != tt.wantEnd {
			t.Errorf("[%s] frame range = %d-%d, want %d-%d", tt.label, cfg.StartFrame, cfg.EndFrame, tt.wantStart, tt.wantEnd)
		}
		if cfg.IsAll != tt.wantAll {
			t.Errorf("[%s] isAll = %v, want %v", tt.label, cfg.IsAll, tt.wantAll)
		}
		if cfg.Ease != tt.wantEase {
			t.Errorf("[%s] ease = %s, want %s", tt.label, cfg.Ease, tt.wantEase)
		}
		if cfg.Reverse != tt.wantRev {
			t.Errorf("[%s] reverse = %v, want %v", tt.label, cfg.Reverse, tt.wantRev)
		}
		if cfg.RotationAngle != tt.wantAngle {
			t.Errorf("[%s] angle = %f, want %f", tt.label, cfg.RotationAngle, tt.wantAngle)
		}
		if cfg.RotationDir != tt.wantDir {
			t.Errorf("[%s] dir = %s, want %s", tt.label, cfg.RotationDir, tt.wantDir)
		}
		if cfg.OrientPath != tt.wantOrient {
			t.Errorf("[%s] orient = %v, want %v", tt.label, cfg.OrientPath, tt.wantOrient)
		}
		if cfg.PivotType != tt.wantPivot {
			t.Errorf("[%s] pivot = %s, want %s", tt.label, cfg.PivotType, tt.wantPivot)
		}
		if cfg.PivotEdgeAngle != tt.wantEdge {
			t.Errorf("[%s] pivotEdge = %f, want %f", tt.label, cfg.PivotEdgeAngle, tt.wantEdge)
		}
		if cfg.PivotNodeID != tt.wantNode {
			t.Errorf("[%s] pivotNode = %s, want %s", tt.label, cfg.PivotNodeID, tt.wantNode)
		}
		expectedFromX := tt.wantScaleFromX
		if expectedFromX == 0 {
			expectedFromX = 1.0
		}
		expectedFromY := tt.wantScaleFromY
		if expectedFromY == 0 {
			expectedFromY = 1.0
		}
		expectedToX := tt.wantScaleToX
		if expectedToX == 0 {
			expectedToX = 1.0
		}
		expectedToY := tt.wantScaleToY
		if expectedToY == 0 {
			expectedToY = 1.0
		}
		if cfg.ScaleFromX != expectedFromX || cfg.ScaleFromY != expectedFromY ||
			cfg.ScaleToX != expectedToX || cfg.ScaleToY != expectedToY {
			t.Errorf("[%s] scale (from: %f,%f to: %f,%f), want (from: %f,%f to: %f,%f)",
				tt.label, cfg.ScaleFromX, cfg.ScaleFromY, cfg.ScaleToX, cfg.ScaleToY,
				expectedFromX, expectedFromY, expectedToX, expectedToY)
		}
	}
}

func TestCalculateEdgePivot(t *testing.T) {
	rect := Rect{X: 10, Y: 20, Width: 100, Height: 60}
	cx := 10.0 + 50.0 // 60
	cy := 20.0 + 30.0 // 50

	// 0 deg: top-center (cx, Y) -> (60, 20)
	px, py := CalculateEdgePivot(rect, 0)
	if math.Abs(px-cx) > 0.001 || math.Abs(py-20.0) > 0.001 {
		t.Errorf("pivot 0 = (%f, %f), want (%f, 20)", px, py, cx)
	}

	// 90 deg: right-center (X+W, cy) -> (110, 50)
	px, py = CalculateEdgePivot(rect, 90)
	if math.Abs(px-110.0) > 0.001 || math.Abs(py-cy) > 0.001 {
		t.Errorf("pivot 90 = (%f, %f), want (110, %f)", px, py, cy)
	}

	// 180 deg: bottom-center (cx, Y+H) -> (60, 80)
	px, py = CalculateEdgePivot(rect, 180)
	if math.Abs(px-cx) > 0.001 || math.Abs(py-80.0) > 0.001 {
		t.Errorf("pivot 180 = (%f, %f), want (%f, 80)", px, py, cx)
	}

	// 270 deg: left-center (X, cy) -> (10, 50)
	px, py = CalculateEdgePivot(rect, 270)
	if math.Abs(px-10.0) > 0.001 || math.Abs(py-cy) > 0.001 {
		t.Errorf("pivot 270 = (%f, %f), want (10, %f)", px, py, cy)
	}
}

func TestEvaluatePathTangentAngle(t *testing.T) {
	// Line going right (+X)
	pathRight := "M 0,0 L 100,0"
	ang, err := EvaluatePathTangentAngle(pathRight, 0.5)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if math.Abs(ang-0.0) > 0.001 {
		t.Errorf("pathRight angle = %f, want 0", ang)
	}

	// Line going down (+Y)
	pathDown := "M 0,0 L 0,100"
	ang, err = EvaluatePathTangentAngle(pathDown, 0.5)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if math.Abs(ang-90.0) > 0.001 {
		t.Errorf("pathDown angle = %f, want 90", ang)
	}
}

func TestBuildTimelineFrameSVG_Reverse(t *testing.T) {
	svgContent := `<svg width="100" height="100" viewBox="0 0 100 100" xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape">
  <g id="starGroup" inkscape:groupmode="layer" inkscape:label="Star">
    <rect id="star" x="0" y="0" width="10" height="10"/>
    <path id="motionPath" inkscape:label="Move {f:1-3 rev}" d="M 0,0 L 100,50"/>
  </g>
</svg>`

	doc, err := ParseSVG([]byte(svgContent))
	if err != nil {
		t.Fatalf("ParseSVG failed: %v", err)
	}

	if len(doc.MotionPaths) != 1 {
		t.Fatalf("expected 1 motion path, got %d", len(doc.MotionPaths))
	}
	if !doc.MotionPaths[0].Config.Reverse {
		t.Fatalf("expected Reverse=true")
	}

	// Frame 0 (1-based frame 1) should start at endpoint (100, 50)
	frame0, err := BuildTimelineFrameSVG(doc, 0, doc.GetDocumentRect())
	if err != nil {
		t.Fatalf("BuildTimelineFrameSVG frame 0 failed: %v", err)
	}
	if !strings.Contains(string(frame0), "translate(100.000000, 50.000000)") {
		t.Errorf("frame 0 expected translate(100, 50) for reverse start, got:\n%s", string(frame0))
	}

	// Frame 1 (1-based frame 2) should be at midpoint (50, 25)
	frame1, err := BuildTimelineFrameSVG(doc, 1, doc.GetDocumentRect())
	if err != nil {
		t.Fatalf("BuildTimelineFrameSVG frame 1 failed: %v", err)
	}
	if !strings.Contains(string(frame1), "translate(50.000000, 25.000000)") {
		t.Errorf("frame 1 expected translate(50, 25) for reverse midpoint, got:\n%s", string(frame1))
	}

	// Frame 2 (1-based frame 3) should arrive at resting position (0, 0)
	frame2, err := BuildTimelineFrameSVG(doc, 2, doc.GetDocumentRect())
	if err != nil {
		t.Fatalf("BuildTimelineFrameSVG frame 2 failed: %v", err)
	}
	if strings.Contains(string(frame2), "translate(") {
		t.Errorf("frame 2 (end of reverse) should not have non-zero translate, got:\n%s", string(frame2))
	}
}

func TestBuildTimelineFrameSVG_RotationAndMultiMotion(t *testing.T) {
	// A group with both Move and Rot
	svgContent := `<svg width="200" height="200" viewBox="0 0 200 200" xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape">
  <g id="boxGroup" inkscape:groupmode="layer" inkscape:label="Box">
    <rect id="box" x="10" y="10" width="20" height="20" fill="red"/>
    <path id="movePath" inkscape:label="Move {f:1-3}" d="M 0,0 L 40,0"/>
    <path id="rotPath" inkscape:label="Rot {f:1-3 angle:90 dir:cw pivot:0}" d="M 0,0 L 0,0"/>
  </g>
</svg>`

	doc, err := ParseSVG([]byte(svgContent))
	if err != nil {
		t.Fatalf("ParseSVG failed: %v", err)
	}

	if len(doc.MotionPaths) != 2 {
		t.Fatalf("expected 2 motion paths, got %d", len(doc.MotionPaths))
	}

	// Frame 0 (frame 1): progress = 0.0 -> translate 0, rot 0
	f0, err := BuildTimelineFrameSVG(doc, 0, doc.GetDocumentRect())
	if err != nil {
		t.Fatalf("BuildTimelineFrameSVG f0 failed: %v", err)
	}
	if strings.Contains(string(f0), "rotate(") || strings.Contains(string(f0), "translate(") {
		t.Errorf("f0 expected no transform at resting start, got:\n%s", string(f0))
	}

	// Frame 2 (frame 3): progress = 1.0 -> translate (40, 0), rot 90 around top edge (20, 10)
	f2, err := BuildTimelineFrameSVG(doc, 2, doc.GetDocumentRect())
	if err != nil {
		t.Fatalf("BuildTimelineFrameSVG f2 failed: %v", err)
	}
	f2Str := string(f2)
	if !strings.Contains(f2Str, "translate(40.000000, 0.000000)") {
		t.Errorf("f2 expected translate(40, 0), got:\n%s", f2Str)
	}
	if !strings.Contains(f2Str, "rotate(90.000000, 20.000000, 10.000000)") {
		t.Errorf("f2 expected rotate(90, 20, 10) for pivot 0 (top-center), got:\n%s", f2Str)
	}
}

func TestBuildTimelineFrameSVG_OrientPath(t *testing.T) {
	// Path curves from right (+X) to down (+Y)
	// Tangent at t=0 is 0 deg. Tangent at t=1 is 90 deg.
	// Relative rotation at t=1 should be 90 deg.
	svgContent := `<svg width="200" height="200" viewBox="0 0 200 200" xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape">
  <g id="arrowGroup" inkscape:groupmode="layer" inkscape:label="Arrow">
    <rect id="arrow" x="10" y="10" width="20" height="20" fill="blue"/>
    <path id="curvePath" inkscape:label="Move {f:1-3 orient:true}" d="M 0,0 C 50,0 50,50 50,50"/>
  </g>
</svg>`

	doc, err := ParseSVG([]byte(svgContent))
	if err != nil {
		t.Fatalf("ParseSVG failed: %v", err)
	}

	// Frame 0: relative orient = 0 deg
	f0, err := BuildTimelineFrameSVG(doc, 0, doc.GetDocumentRect())
	if err != nil {
		t.Fatalf("f0 failed: %v", err)
	}
	if strings.Contains(string(f0), "rotate(") {
		t.Errorf("f0 expected no rotation at start of path, got:\n%s", string(f0))
	}

	// Frame 2 (frame 3): endpoint tangent should be rotated
	f2, err := BuildTimelineFrameSVG(doc, 2, doc.GetDocumentRect())
	if err != nil {
		t.Fatalf("f2 failed: %v", err)
	}
	f2Str := string(f2)
	if !strings.Contains(f2Str, "rotate(") {
		t.Errorf("f2 expected rotate transform for orient:true, got:\n%s", f2Str)
	}
}

func TestBuildTimelineFrameSVG_NodePivot(t *testing.T) {
	// Pivot referencing #pin
	svgContent := `<svg width="200" height="200" viewBox="0 0 200 200" xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape">
  <rect id="pin" x="50" y="50" width="10" height="10"/>
  <g id="armGroup" inkscape:groupmode="layer" inkscape:label="Arm">
    <rect id="arm" x="0" y="0" width="100" height="20" fill="green"/>
    <path id="rot" inkscape:label="Rot {f:1-3 angle:45 pivot:#pin}" d="M 0,0 L 0,0"/>
  </g>
</svg>`

	doc, err := ParseSVG([]byte(svgContent))
	if err != nil {
		t.Fatalf("ParseSVG failed: %v", err)
	}

	// Frame 2: #pin center is (50+5, 50+5) = (55, 55)
	f2, err := BuildTimelineFrameSVG(doc, 2, doc.GetDocumentRect())
	if err != nil {
		t.Fatalf("f2 failed: %v", err)
	}
	f2Str := string(f2)
	if !strings.Contains(f2Str, "rotate(45.000000, 55.000000, 55.000000)") {
		t.Errorf("f2 expected rotate(45, 55, 55) for pivot:#pin, got:\n%s", f2Str)
	}
}

func TestIntegration_PendulumAndComplexMotion(t *testing.T) {
	// 1. Test testdata/pendulum.svg
	pendulumData, err := os.ReadFile("../../testdata/pendulum.svg")
	if err != nil {
		t.Fatalf("failed to read pendulum.svg: %v", err)
	}
	doc, err := ParseSVG(pendulumData)
	if err != nil {
		t.Fatalf("ParseSVG(pendulum.svg) failed: %v", err)
	}
	if doc.DefaultMode != ModeTimeline {
		t.Errorf("pendulum.svg DefaultMode = %s, want %s", doc.DefaultMode, ModeTimeline)
	}
	if len(doc.Layers) != 10 {
		t.Errorf("pendulum.svg frame layers count = %d, want 10", len(doc.Layers))
	}
	if len(doc.MotionPaths) != 1 {
		t.Fatalf("pendulum.svg motion paths count = %d, want 1", len(doc.MotionPaths))
	}
	mp := doc.MotionPaths[0]
	if mp.Config.Type != "rot" || mp.Config.RotationAngle != 48 || mp.Config.RotationDir != "ccw" || mp.Config.PivotType != "node" || mp.Config.PivotNodeID != "pivot_mount" {
		t.Errorf("pendulum.svg parsed config = %+v, want Rot 48 deg ccw pivot:#pivot_mount", mp.Config)
	}

	// Render all 10 frames to verify no panic or XML error
	for i := 0; i < len(doc.Layers); i++ {
		frameBytes, err := BuildTimelineFrameSVG(doc, i, doc.GetDrawingRect())
		if err != nil {
			t.Fatalf("BuildTimelineFrameSVG(pendulum, frame %d) failed: %v", i, err)
		}
		if len(frameBytes) == 0 {
			t.Fatalf("BuildTimelineFrameSVG(pendulum, frame %d) returned empty bytes", i)
		}
	}

	// 2. Test testdata/complex_motion.svg
	complexData, err := os.ReadFile("../../testdata/complex_motion.svg")
	if err != nil {
		t.Fatalf("failed to read complex_motion.svg: %v", err)
	}
	doc2, err := ParseSVG(complexData)
	if err != nil {
		t.Fatalf("ParseSVG(complex_motion.svg) failed: %v", err)
	}
	if doc2.DefaultMode != ModeTimeline {
		t.Errorf("complex_motion.svg DefaultMode = %s, want %s", doc2.DefaultMode, ModeTimeline)
	}
	if len(doc2.Layers) != 20 {
		t.Errorf("complex_motion.svg frame layers count = %d, want 20", len(doc2.Layers))
	}
	if len(doc2.MotionPaths) != 3 {
		t.Fatalf("complex_motion.svg motion paths count = %d, want 3", len(doc2.MotionPaths))
	}
	mpScale := doc2.MotionPaths[2]
	if mpScale.Config.Type != "scale" || mpScale.Config.ScaleFromX != 0.6 || mpScale.Config.ScaleToX != 1.4 {
		t.Errorf("complex_motion.svg scale path config = %+v, want scale from 0.6 to 1.4", mpScale.Config)
	}

	// Render all 20 frames to verify compositing across overlapping Move, Rot, and Scale
	for i := 0; i < len(doc2.Layers); i++ {
		frameBytes, err := BuildTimelineFrameSVG(doc2, i, doc2.GetDrawingRect())
		if err != nil {
			t.Fatalf("BuildTimelineFrameSVG(complex_motion, frame %d) failed: %v", i, err)
		}
		if len(frameBytes) == 0 {
			t.Fatalf("BuildTimelineFrameSVG(complex_motion, frame %d) returned empty bytes", i)
		}
	}
}

func TestScaleAnimation_FrameTransforms(t *testing.T) {
	svgContent := `<svg width="200" height="200" viewBox="0 0 200 200" xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape">
  <g id="boxGroup" inkscape:groupmode="layer" inkscape:label="Box">
    <rect id="rect" x="50" y="50" width="100" height="60" fill="blue"/>
    <path id="scale_ctrl" inkscape:label="Scale {f:1-3 scale:2.0 pivot:center}" d="M 0,0 L 0,0"/>
  </g>
</svg>`

	doc, err := ParseSVG([]byte(svgContent))
	if err != nil {
		t.Fatalf("ParseSVG failed: %v", err)
	}

	// Group bounding box center is (50 + 50, 50 + 30) = (100, 80)
	// Frame 0 (t=0.0): scale is 1.0 -> identity -> no scale transform injected
	f0, err := BuildTimelineFrameSVG(doc, 0, doc.GetDocumentRect())
	if err != nil {
		t.Fatalf("f0 failed: %v", err)
	}
	f0Str := string(f0)
	if strings.Contains(f0Str, "scale(") {
		t.Errorf("f0 expected no scale transform for identity scale 1.0, got:\n%s", f0Str)
	}

	// Frame 1 (t=0.5): scale is 1.0 + 0.5*(2.0 - 1.0) = 1.5
	f1, err := BuildTimelineFrameSVG(doc, 1, doc.GetDocumentRect())
	if err != nil {
		t.Fatalf("f1 failed: %v", err)
	}
	f1Str := string(f1)
	expectedF1 := "translate(100.000000, 80.000000) scale(1.500000, 1.500000) translate(-100.000000, -80.000000)"
	if !strings.Contains(f1Str, expectedF1) {
		t.Errorf("f1 expected %s, got:\n%s", expectedF1, f1Str)
	}

	// Frame 2 (t=1.0): scale is 2.0
	f2, err := BuildTimelineFrameSVG(doc, 2, doc.GetDocumentRect())
	if err != nil {
		t.Fatalf("f2 failed: %v", err)
	}
	f2Str := string(f2)
	expectedF2 := "translate(100.000000, 80.000000) scale(2.000000, 2.000000) translate(-100.000000, -80.000000)"
	if !strings.Contains(f2Str, expectedF2) {
		t.Errorf("f2 expected %s, got:\n%s", expectedF2, f2Str)
	}
}

func TestScaleAnimation_EdgePivot(t *testing.T) {
	// pivot: 180 (bottom edge: cx = 100, cy = 50 + 60 = 110)
	svgContent := `<svg width="200" height="200" viewBox="0 0 200 200" xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape">
  <g id="boxGroup" inkscape:groupmode="layer" inkscape:label="Box">
    <rect id="rect" x="50" y="50" width="100" height="60" fill="blue"/>
    <path id="scale_ctrl" inkscape:label="Scale {f:1-3 from-x:0.5 to-x:1.5 from-y:2.0 to-y:0.5 pivot:180}" d="M 0,0 L 0,0"/>
  </g>
</svg>`

	doc, err := ParseSVG([]byte(svgContent))
	if err != nil {
		t.Fatalf("ParseSVG failed: %v", err)
	}

	// Frame 2 (t=1.0): sx=1.5, sy=0.5, pivot=(100, 110)
	f2, err := BuildTimelineFrameSVG(doc, 2, doc.GetDocumentRect())
	if err != nil {
		t.Fatalf("f2 failed: %v", err)
	}
	f2Str := string(f2)
	expected := "translate(100.000000, 110.000000) scale(1.500000, 0.500000) translate(-100.000000, -110.000000)"
	if !strings.Contains(f2Str, expected) {
		t.Errorf("f2 expected %s, got:\n%s", expected, f2Str)
	}
}

func TestScaleAnimation_MultiMotionComposition(t *testing.T) {
	svgContent := `<svg width="200" height="200" viewBox="0 0 200 200" xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape">
  <g id="boxGroup" inkscape:groupmode="layer" inkscape:label="Box">
    <rect id="rect" x="50" y="50" width="100" height="60" fill="blue"/>
    <path id="m1" inkscape:label="Move {f:1-3}" d="M 0,0 L 40,20"/>
    <path id="m2" inkscape:label="Rot {f:1-3 angle:90 pivot:center}" d="M 0,0 L 0,0"/>
    <path id="m3" inkscape:label="Scale {f:1-3 scale:2.0 pivot:center}" d="M 0,0 L 0,0"/>
  </g>
</svg>`

	doc, err := ParseSVG([]byte(svgContent))
	if err != nil {
		t.Fatalf("ParseSVG failed: %v", err)
	}

	// Frame 2 (t=1.0):
	// translate(40, 20)
	// rotate(90, 100, 80)
	// translate(100, 80) scale(2, 2) translate(-100, -80)
	f2, err := BuildTimelineFrameSVG(doc, 2, doc.GetDocumentRect())
	if err != nil {
		t.Fatalf("f2 failed: %v", err)
	}
	f2Str := string(f2)
	expected := "translate(40.000000, 20.000000) rotate(90.000000, 100.000000, 80.000000) translate(100.000000, 80.000000) scale(2.000000, 2.000000) translate(-100.000000, -80.000000)"
	if !strings.Contains(f2Str, expected) {
		t.Errorf("f2 expected composed transform:\n%s\ngot:\n%s", expected, f2Str)
	}
}

func TestScaleAnimation_MultiplicativeScale(t *testing.T) {
	svgContent := `<svg width="200" height="200" viewBox="0 0 200 200" xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape">
  <g id="boxGroup" inkscape:groupmode="layer" inkscape:label="Box">
    <rect id="rect" x="50" y="50" width="100" height="60" fill="blue"/>
    <path id="s1" inkscape:label="Scale {f:1-3 scale:2.0 pivot:center}" d="M 0,0 L 0,0"/>
    <path id="s2" inkscape:label="Scale {f:1-3 scale-x:1.5 scale-y:0.5 pivot:center}" d="M 0,0 L 0,0"/>
  </g>
</svg>`

	doc, err := ParseSVG([]byte(svgContent))
	if err != nil {
		t.Fatalf("ParseSVG failed: %v", err)
	}

	// Frame 2 (t=1.0):
	// s1: sx=2.0, sy=2.0
	// s2: sx=1.5, sy=0.5
	// combined: totalSx = 2.0 * 1.5 = 3.0, totalSy = 2.0 * 0.5 = 1.0
	f2, err := BuildTimelineFrameSVG(doc, 2, doc.GetDocumentRect())
	if err != nil {
		t.Fatalf("f2 failed: %v", err)
	}
	f2Str := string(f2)
	expected := "scale(3.000000, 1.000000)"
	if !strings.Contains(f2Str, expected) {
		t.Errorf("f2 expected scale(3.0, 1.0), got:\n%s", f2Str)
	}
}

func TestIntegration_ScaleTestSVG(t *testing.T) {
	data, err := os.ReadFile("../../testdata/scale_test.svg")
	if err != nil {
		t.Fatalf("failed to read scale_test.svg: %v", err)
	}
	doc, err := ParseSVG(data)
	if err != nil {
		t.Fatalf("ParseSVG(scale_test.svg) failed: %v", err)
	}
	if doc.DefaultMode != ModeTimeline {
		t.Errorf("scale_test.svg DefaultMode = %s, want %s", doc.DefaultMode, ModeTimeline)
	}
	if len(doc.Layers) != 15 {
		t.Errorf("scale_test.svg frame count = %d, want 15", len(doc.Layers))
	}
	if len(doc.MotionPaths) != 2 {
		t.Fatalf("scale_test.svg motion paths count = %d, want 2", len(doc.MotionPaths))
	}

	// Render all 15 frames
	for i := 0; i < len(doc.Layers); i++ {
		frameBytes, err := BuildTimelineFrameSVG(doc, i, doc.GetDrawingRect())
		if err != nil {
			t.Fatalf("BuildTimelineFrameSVG(frame %d) failed: %v", i, err)
		}
		if len(frameBytes) == 0 {
			t.Fatalf("BuildTimelineFrameSVG(frame %d) returned empty bytes", i)
		}
	}
}

func TestQuadraticBezierSegment(t *testing.T) {
	// (0,0) -> Control (50, 100) -> End (100, 0)
	seg := NewQuadraticBezierSegment(0, 0, 50, 100, 100, 0)
	if seg.Length() <= 0 {
		t.Errorf("expected positive length, got %f", seg.Length())
	}

	// At u=0: (0, 0)
	x0, y0 := seg.EvaluateAt(0.0)
	if math.Abs(x0) > 1e-4 || math.Abs(y0) > 1e-4 {
		t.Errorf("at u=0, expected (0,0), got (%f, %f)", x0, y0)
	}

	// At u=0.5: x = 0.25*0 + 2*0.25*50 + 0.25*100 = 50, y = 0.25*0 + 2*0.25*100 + 0 = 50
	xMid, yMid := seg.EvaluateAt(0.5)
	if math.Abs(xMid-50) > 1e-4 || math.Abs(yMid-50) > 1e-4 {
		t.Errorf("at u=0.5, expected (50,50), got (%f, %f)", xMid, yMid)
	}

	// At u=1.0: (100, 0)
	x1, y1 := seg.EvaluateAt(1.0)
	if math.Abs(x1-100) > 1e-4 || math.Abs(y1) > 1e-4 {
		t.Errorf("at u=1.0, expected (100,0), got (%f, %f)", x1, y1)
	}

	// Tangents: at u=0, vector is 2*(cx-sx, cy-sy) = (100, 200) -> positive
	tx0, ty0 := seg.TangentAt(0.0)
	if tx0 <= 0 || ty0 <= 0 {
		t.Errorf("at u=0, expected positive tangent vector, got (%f, %f)", tx0, ty0)
	}

	// At u=1, vector is 2*(ex-cx, ey-cy) = (100, -200) -> positive dx, negative dy
	tx1, ty1 := seg.TangentAt(1.0)
	if tx1 <= 0 || ty1 >= 0 {
		t.Errorf("at u=1, expected positive dx and negative dy, got (%f, %f)", tx1, ty1)
	}

	// Degenerate tangent test (when dx==0 && dy==0)
	degen := NewQuadraticBezierSegment(10, 10, 10, 10, 20, 20)
	dtx, dty := degen.TangentAt(0.0)
	if dtx != 10 || dty != 10 {
		t.Errorf("expected fallback tangent (10, 10), got (%f, %f)", dtx, dty)
	}
}

func TestQuadraticPathParsing(t *testing.T) {
	// Q (absolute) and q (relative)
	pathData := "M 0 0 Q 50 100 100 0 q 50 -100 100 0"
	xEnd, yEnd, err := EvaluatePathAt(pathData, 1.0)
	if err != nil {
		t.Fatalf("EvaluatePathAt quadratic path failed: %v", err)
	}
	if math.Abs(xEnd-200) > 1e-2 || math.Abs(yEnd) > 1e-2 {
		t.Errorf("expected end at (200, 0), got (%f, %f)", xEnd, yEnd)
	}

	angle, err := EvaluatePathTangentAngle(pathData, 0.5)
	if err != nil {
		t.Fatalf("EvaluatePathTangentAngle failed: %v", err)
	}
	_ = angle
}

func TestGetPathStartPoint(t *testing.T) {
	// Absolute M
	x, y, err := GetPathStartPoint("M 15.5 42.8 L 100 200")
	if err != nil {
		t.Fatalf("GetPathStartPoint failed: %v", err)
	}
	if math.Abs(x-15.5) > 1e-4 || math.Abs(y-42.8) > 1e-4 {
		t.Errorf("expected (15.5, 42.8), got (%f, %f)", x, y)
	}

	// Relative m
	xRel, yRel, err := GetPathStartPoint("m 10 25 l 50 50")
	if err != nil {
		t.Fatalf("GetPathStartPoint relative failed: %v", err)
	}
	if math.Abs(xRel-10) > 1e-4 || math.Abs(yRel-25) > 1e-4 {
		t.Errorf("expected (10, 25), got (%f, %f)", xRel, yRel)
	}

	// Invalid path
	_, _, err = GetPathStartPoint("invalid path")
	if err == nil {
		t.Errorf("expected error on invalid path, got nil")
	}
}

