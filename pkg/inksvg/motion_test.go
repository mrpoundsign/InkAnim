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
		wantRotFrom    float64
		wantRotTo      float64
		wantRotRange   bool
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
			label:        "Rot {f:13-24 ease:in-out from:0 to:180 pivot:center}",
			wantOK:       true,
			wantType:     "rot",
			wantStart:    13,
			wantEnd:      24,
			wantEase:     "in-out",
			wantDir:      "cw",
			wantPivot:    "center",
			wantRotFrom:  0,
			wantRotTo:    180,
			wantRotRange: true,
		},
		{
			label:        "Rot {f:25-36 from:180 to:180 pivot:center}",
			wantOK:       true,
			wantType:     "rot",
			wantStart:    25,
			wantEnd:      36,
			wantEase:     "linear",
			wantDir:      "cw",
			wantPivot:    "center",
			wantRotFrom:  180,
			wantRotTo:    180,
			wantRotRange: true,
		},
		{
			label:        "Rot {f:37-48 ease:in-out from:180 to:0 pivot:center}",
			wantOK:       true,
			wantType:     "rot",
			wantStart:    37,
			wantEnd:      48,
			wantEase:     "in-out",
			wantDir:      "cw",
			wantPivot:    "center",
			wantRotFrom:  180,
			wantRotTo:    0,
			wantRotRange: true,
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
		if cfg.HasRotationRange != tt.wantRotRange {
			t.Errorf("[%s] hasRotRange = %v, want %v", tt.label, cfg.HasRotationRange, tt.wantRotRange)
		}
		if cfg.RotationFrom != tt.wantRotFrom || cfg.RotationTo != tt.wantRotTo {
			t.Errorf("[%s] rotRange = (%f to %f), want (%f to %f)", tt.label, cfg.RotationFrom, cfg.RotationTo, tt.wantRotFrom, tt.wantRotTo)
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
	if len(doc2.Layers) != 60 {
		t.Errorf("complex_motion.svg frame layers count = %d, want 60", len(doc2.Layers))
	}
	if len(doc2.MotionPaths) != 37 {
		t.Fatalf("complex_motion.svg motion paths count = %d, want 37", len(doc2.MotionPaths))
	}
	if doc2.CameraPath == nil || doc2.CameraPath.Config.Type != "camera" || doc2.CameraPath.Config.StartFrame != 1 || doc2.CameraPath.Config.EndFrame != 60 {
		t.Fatalf("complex_motion.svg CameraPath = %+v, want camera f: 1-60", doc2.CameraPath)
	}

	mpMap := make(map[string]MotionPath)
	for _, mp := range doc2.MotionPaths {
		mpMap[mp.ID] = mp
	}

	mpFadeDown := mpMap["twinkle_fade_down"]
	if mpFadeDown.Config.Type != "fade" || mpFadeDown.Config.OpacityFrom != 1.0 || mpFadeDown.Config.OpacityTo != 0.2 {
		t.Errorf("complex_motion.svg twinkle fade down = %+v, want fade 1.0 to 0.2", mpFadeDown.Config)
	}
	mpStrobe1 := mpMap["strobe_1"]
	if mpStrobe1.Config.Type != "show" || mpStrobe1.Config.StartFrame != 1 || mpStrobe1.Config.EndFrame != 6 {
		t.Errorf("complex_motion.svg strobe 1 = %+v, want show 1-6", mpStrobe1.Config)
	}
	mpFadeBlink := mpMap["fb_fade"]
	if mpFadeBlink.Config.Type != "fade" || mpFadeBlink.Config.OpacityFrom != 0.15 || mpFadeBlink.Config.OpacityTo != 1.0 {
		t.Errorf("complex_motion.svg fade blink = %+v, want fade 0.15 to 1.0", mpFadeBlink.Config)
	}
	mpPlanetDist := mpMap["planet_dist"]
	if mpPlanetDist.Config.Type != "dist" || mpPlanetDist.Config.ParallaxFactor != 0.8 {
		t.Errorf("complex_motion.svg planet dist = %+v, want dist factor 0.8", mpPlanetDist.Config)
	}
	mpMoonDepthBack := mpMap["moon_depth_back"]
	if mpMoonDepthBack.Config.Type != "depth" || mpMoonDepthBack.Config.DepthOffset != -1 || mpMoonDepthBack.Config.StartFrame != 1 || mpMoonDepthBack.Config.EndFrame != 30 {
		t.Errorf("complex_motion.svg moon depth back = %+v, want depth z: -1 f: 1-30", mpMoonDepthBack.Config)
	}
	mpMoonDepthFront := mpMap["moon_depth_front"]
	if mpMoonDepthFront.Config.Type != "depth" || mpMoonDepthFront.Config.DepthOffset != 1 || mpMoonDepthFront.Config.StartFrame != 31 || mpMoonDepthFront.Config.EndFrame != 60 {
		t.Errorf("complex_motion.svg moon depth front = %+v, want depth z: +1 f: 31-60", mpMoonDepthFront.Config)
	}
	mpFlameUp := mpMap["flame_p1_up"]
	if mpFlameUp.Config.Type != "scale" || mpFlameUp.Config.ScaleFromX != 0.7 || mpFlameUp.Config.ScaleToX != 1.6 {
		t.Errorf("complex_motion.svg flame up = %+v, want scale from-x: 0.7 to-x: 1.6", mpFlameUp.Config)
	}
	mpScale := mpMap["zoom_scale"]
	if mpScale.Config.Type != "scale" || mpScale.Config.ScaleFromX != 0.6 || mpScale.Config.ScaleToX != 1.4 {
		t.Errorf("complex_motion.svg scale path config = %+v, want scale from 0.6 to 1.4", mpScale.Config)
	}

	mpRollTop := mpMap["roll_top"]
	if mpRollTop.Config.Type != "rot" || mpRollTop.Config.RotationFrom != 0 || mpRollTop.Config.RotationTo != 180 {
		t.Errorf("complex_motion.svg roll top = %+v, want rot from 0 to 180", mpRollTop.Config)
	}
	mpRollHold := mpMap["roll_hold"]
	if mpRollHold.Config.Type != "rot" || mpRollHold.Config.RotationFrom != 180 || mpRollHold.Config.RotationTo != 180 {
		t.Errorf("complex_motion.svg roll hold = %+v, want rot from 180 to 180", mpRollHold.Config)
	}
	mpRollBottom := mpMap["roll_bottom"]
	if mpRollBottom.Config.Type != "rot" || mpRollBottom.Config.RotationFrom != 180 || mpRollBottom.Config.RotationTo != 0 {
		t.Errorf("complex_motion.svg roll bottom = %+v, want rot from 180 to 0", mpRollBottom.Config)
	}

	// Render all 60 frames to verify compositing across overlapping Move, Rot, Scale, Fade, Show/Hide, and Depth
	for i := 0; i < len(doc2.Layers); i++ {
		frameBytes, err := BuildTimelineFrameSVG(doc2, i, doc2.GetDrawingRect())
		if err != nil {
			t.Fatalf("BuildTimelineFrameSVG(complex_motion, frame %d) failed: %v", i, err)
		}
		if len(frameBytes) == 0 {
			t.Fatalf("BuildTimelineFrameSVG(complex_motion, frame %d) returned empty bytes", i)
		}

		frameStr := string(frameBytes)
		idxPlanet := strings.Index(frameStr, `id="group_planet"`)
		idxMoon := strings.Index(frameStr, `id="group_moon"`)
		if idxPlanet == -1 || idxMoon == -1 {
			t.Fatalf("Frame %d missing planet or moon group in serialized frame", i)
		}
		if i < 30 {
			// Frames 1-30 (i=0..29): Moon is behind planet (idxMoon < idxPlanet)
			if idxMoon >= idxPlanet {
				t.Errorf("Frame %d: expected Moon to render behind Planet, got Moon idx %d >= Planet idx %d", i+1, idxMoon, idxPlanet)
			}
		} else {
			// Frames 31-60 (i=30..59): Moon is in front of planet (idxPlanet < idxMoon)
			if idxPlanet >= idxMoon {
				t.Errorf("Frame %d: expected Moon to render in front of Planet, got Planet idx %d >= Moon idx %d", i+1, idxPlanet, idxMoon)
			}
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

func TestParseMotionConfig_FadeAndVisibility(t *testing.T) {
	tests := []struct {
		label             string
		wantOK            bool
		wantType          string
		wantStart         int
		wantEnd           int
		wantAll           bool
		wantEase          string
		wantOpacityFrom   float64
		wantOpacityTo     float64
		wantHasOpacity    bool
		wantVisibility    string
		wantHasVisibility bool
	}{
		{
			label:             "Fade {f: 1-30; ease: in-out; from: 0; to: 100}",
			wantOK:            true,
			wantType:          "fade",
			wantStart:         1,
			wantEnd:           30,
			wantEase:          "in-out",
			wantOpacityFrom:   0.0,
			wantOpacityTo:     1.0,
			wantHasOpacity:    true,
		},
		{
			label:             "Fade {f: 1-15; from: 1.0; to: 0.0}",
			wantOK:            true,
			wantType:          "fade",
			wantStart:         1,
			wantEnd:           15,
			wantEase:          "linear",
			wantOpacityFrom:   1.0,
			wantOpacityTo:     0.0,
			wantHasOpacity:    true,
		},
		{
			label:             "Fade {f: 1-20; opacity: 50}",
			wantOK:            true,
			wantType:          "fade",
			wantStart:         1,
			wantEnd:           20,
			wantEase:          "linear",
			wantOpacityFrom:   0.5,
			wantOpacityTo:     0.5,
			wantHasOpacity:    true,
		},
		{
			label:             "Fade {opacity: 0.5}",
			wantOK:            true,
			wantType:          "fade",
			wantAll:           true,
			wantEase:          "linear",
			wantOpacityFrom:   0.5,
			wantOpacityTo:     0.5,
			wantHasOpacity:    true,
		},
		{
			label:             "Fade {from: 0; to: 80%}",
			wantOK:            true,
			wantType:          "fade",
			wantAll:           true,
			wantEase:          "linear",
			wantOpacityFrom:   0.0,
			wantOpacityTo:     0.8,
			wantHasOpacity:    true,
		},
		{
			label:             "Show {f: 10-50}",
			wantOK:            true,
			wantType:          "show",
			wantStart:         10,
			wantEnd:           50,
			wantEase:          "linear",
			wantVisibility:    "show",
			wantHasVisibility: true,
		},
		{
			label:             "Hide {f: 1-10}",
			wantOK:            true,
			wantType:          "hide",
			wantStart:         1,
			wantEnd:           10,
			wantEase:          "linear",
			wantVisibility:    "hide",
			wantHasVisibility: true,
		},
	}

	for _, tt := range tests {
		cfg, ok := parseMotionConfig(tt.label)
		if ok != tt.wantOK {
			t.Errorf("[%s] ok = %v, want %v", tt.label, ok, tt.wantOK)
			continue
		}
		if !tt.wantOK {
			continue
		}
		if cfg.Type != tt.wantType {
			t.Errorf("[%s] type = %s, want %s", tt.label, cfg.Type, tt.wantType)
		}
		if cfg.StartFrame != tt.wantStart || cfg.EndFrame != tt.wantEnd {
			t.Errorf("[%s] range = %d-%d, want %d-%d", tt.label, cfg.StartFrame, cfg.EndFrame, tt.wantStart, tt.wantEnd)
		}
		if cfg.IsAll != tt.wantAll {
			t.Errorf("[%s] isAll = %v, want %v", tt.label, cfg.IsAll, tt.wantAll)
		}
		if cfg.Ease != tt.wantEase {
			t.Errorf("[%s] ease = %s, want %s", tt.label, cfg.Ease, tt.wantEase)
		}
		if tt.wantHasOpacity {
			if math.Abs(cfg.OpacityFrom-tt.wantOpacityFrom) > 1e-4 {
				t.Errorf("[%s] opacityFrom = %f, want %f", tt.label, cfg.OpacityFrom, tt.wantOpacityFrom)
			}
			if math.Abs(cfg.OpacityTo-tt.wantOpacityTo) > 1e-4 {
				t.Errorf("[%s] opacityTo = %f, want %f", tt.label, cfg.OpacityTo, tt.wantOpacityTo)
			}
		}
		if cfg.HasOpacity != tt.wantHasOpacity {
			t.Errorf("[%s] hasOpacity = %v, want %v", tt.label, cfg.HasOpacity, tt.wantHasOpacity)
		}
		if cfg.VisibilityState != tt.wantVisibility {
			t.Errorf("[%s] visibility = %s, want %s", tt.label, cfg.VisibilityState, tt.wantVisibility)
		}
		if cfg.HasVisibility != tt.wantHasVisibility {
			t.Errorf("[%s] hasVisibility = %v, want %v", tt.label, cfg.HasVisibility, tt.wantHasVisibility)
		}
	}
}

func TestParseMotionConfig_Depth(t *testing.T) {
	tests := []struct {
		label           string
		wantOK          bool
		wantType        string
		wantStart       int
		wantEnd         int
		wantAll         bool
		wantDepthOffset int
		wantHasDepth    bool
	}{
		{
			label:           "Depth {f: 30-60; z: -1}",
			wantOK:          true,
			wantType:        "depth",
			wantStart:       30,
			wantEnd:         60,
			wantAll:         false,
			wantDepthOffset: -1,
			wantHasDepth:    true,
		},
		{
			label:           "Depth {f: 15-30; z: +2}",
			wantOK:          true,
			wantType:        "depth",
			wantStart:       15,
			wantEnd:         30,
			wantAll:         false,
			wantDepthOffset: 2,
			wantHasDepth:    true,
		},
		{
			label:           "Depth {z: 1}",
			wantOK:          true,
			wantType:        "depth",
			wantAll:         true,
			wantDepthOffset: 1,
			wantHasDepth:    true,
		},
		{
			label:           "Depth {f: all; z: -3}",
			wantOK:          true,
			wantType:        "depth",
			wantAll:         true,
			wantDepthOffset: -3,
			wantHasDepth:    true,
		},
		{
			label:           "Depth {f: 1-10; depth: 4}",
			wantOK:          true,
			wantType:        "depth",
			wantStart:       1,
			wantEnd:         10,
			wantAll:         false,
			wantDepthOffset: 4,
			wantHasDepth:    true,
		},
	}

	for _, tt := range tests {
		cfg, ok := parseMotionConfig(tt.label)
		if ok != tt.wantOK {
			t.Errorf("[%s] ok = %v, want %v", tt.label, ok, tt.wantOK)
			continue
		}
		if !tt.wantOK {
			continue
		}
		if cfg.Type != tt.wantType {
			t.Errorf("[%s] type = %s, want %s", tt.label, cfg.Type, tt.wantType)
		}
		if cfg.StartFrame != tt.wantStart || cfg.EndFrame != tt.wantEnd {
			t.Errorf("[%s] range = %d-%d, want %d-%d", tt.label, cfg.StartFrame, cfg.EndFrame, tt.wantStart, tt.wantEnd)
		}
		if cfg.IsAll != tt.wantAll {
			t.Errorf("[%s] isAll = %v, want %v", tt.label, cfg.IsAll, tt.wantAll)
		}
		if cfg.DepthOffset != tt.wantDepthOffset {
			t.Errorf("[%s] depthOffset = %d, want %d", tt.label, cfg.DepthOffset, tt.wantDepthOffset)
		}
		if cfg.HasDepth != tt.wantHasDepth {
			t.Errorf("[%s] hasDepth = %v, want %v", tt.label, cfg.HasDepth, tt.wantHasDepth)
		}
	}
}

func groupHasDisplayNone(svgStr, groupID string) bool {
	idx := strings.Index(svgStr, `id="`+groupID+`"`)
	if idx == -1 {
		return false
	}
	start := strings.LastIndex(svgStr[:idx], "<g")
	if start == -1 {
		return false
	}
	end := strings.Index(svgStr[idx:], ">")
	if end == -1 {
		return false
	}
	tag := svgStr[start : idx+end+1]
	return strings.Contains(tag, `display:none`) || strings.Contains(tag, `display="none"`)
}

func TestFadeAnimation_FrameOpacity(t *testing.T) {
	svgContent := `<svg width="200" height="200" viewBox="0 0 200 200" xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape">
  <g id="boxGroup" inkscape:groupmode="layer" inkscape:label="Box">
    <rect id="rect" x="50" y="50" width="100" height="60" fill="blue"/>
    <path id="fade_ctrl" inkscape:label="Fade {f:1-3 from:0 to:100}" d="M 0,0 L 0,0"/>
  </g>
</svg>`

	doc, err := ParseSVG([]byte(svgContent))
	if err != nil {
		t.Fatalf("ParseSVG failed: %v", err)
	}

	// Frame 0 (t=0.0): opacity = 0.0 -> opacity="0.0000"
	f0, err := BuildTimelineFrameSVG(doc, 0, doc.GetDocumentRect())
	if err != nil {
		t.Fatalf("f0 failed: %v", err)
	}
	f0Str := string(f0)
	if !strings.Contains(f0Str, `opacity="0.0000"`) {
		t.Errorf("f0 expected opacity=\"0.0000\", got:\n%s", f0Str)
	}

	// Frame 1 (t=0.5): opacity = 0.5 -> opacity="0.5000"
	f1, err := BuildTimelineFrameSVG(doc, 1, doc.GetDocumentRect())
	if err != nil {
		t.Fatalf("f1 failed: %v", err)
	}
	f1Str := string(f1)
	if !strings.Contains(f1Str, `opacity="0.5000"`) {
		t.Errorf("f1 expected opacity=\"0.5000\", got:\n%s", f1Str)
	}

	// Frame 2 (t=1.0): opacity = 1.0 -> omitted per PRD 4.1
	f2, err := BuildTimelineFrameSVG(doc, 2, doc.GetDocumentRect())
	if err != nil {
		t.Fatalf("f2 failed: %v", err)
	}
	f2Str := string(f2)
	if strings.Contains(f2Str, `opacity=`) {
		t.Errorf("f2 expected opacity attribute to be omitted for 1.0, got:\n%s", f2Str)
	}
}

func TestFadeAnimation_MultiplicativeFade(t *testing.T) {
	svgContent := `<svg width="200" height="200" viewBox="0 0 200 200" xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape">
  <g id="boxGroup" inkscape:groupmode="layer" inkscape:label="Box">
    <rect id="rect" x="50" y="50" width="100" height="60" fill="blue"/>
    <path id="f1" inkscape:label="Fade {f:1-3 opacity:50}" d="M 0,0 L 0,0"/>
    <path id="f2" inkscape:label="Fade {f:1-3 opacity:50}" d="M 0,0 L 0,0"/>
  </g>
</svg>`

	doc, err := ParseSVG([]byte(svgContent))
	if err != nil {
		t.Fatalf("ParseSVG failed: %v", err)
	}

	// 0.5 * 0.5 = 0.25 -> opacity="0.2500"
	f1, err := BuildTimelineFrameSVG(doc, 1, doc.GetDocumentRect())
	if err != nil {
		t.Fatalf("f1 failed: %v", err)
	}
	f1Str := string(f1)
	if !strings.Contains(f1Str, `opacity="0.2500"`) {
		t.Errorf("f1 expected opacity=\"0.2500\", got:\n%s", f1Str)
	}
}

func TestVisibility_ShowAndHide(t *testing.T) {
	svgContent := `<svg width="200" height="200" viewBox="0 0 200 200" xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape">
  <g id="showBox" inkscape:groupmode="layer" inkscape:label="Show Box">
    <rect id="rect1" x="10" y="10" width="20" height="20" fill="red"/>
    <path id="ctrl_show" inkscape:label="Show {f:2-3}" d="M 0,0 L 0,0"/>
  </g>
  <g id="hideBox" inkscape:groupmode="layer" inkscape:label="Hide Box">
    <rect id="rect2" x="50" y="50" width="20" height="20" fill="green"/>
    <path id="ctrl_hide" inkscape:label="Hide {f:1-2}" d="M 0,0 L 0,0"/>
  </g>
</svg>`

	doc, err := ParseSVG([]byte(svgContent))
	if err != nil {
		t.Fatalf("ParseSVG failed: %v", err)
	}

	// Frame 0 (frame 1):
	// showBox: hidden (Show 2-3) -> style="display:none"
	// hideBox: hidden (Hide 1-2) -> style="display:none"
	f0, err := BuildTimelineFrameSVG(doc, 0, doc.GetDocumentRect())
	if err != nil {
		t.Fatalf("f0 failed: %v", err)
	}
	f0Str := string(f0)
	if !groupHasDisplayNone(f0Str, "showBox") {
		t.Errorf("f0 expected showBox to be hidden with display:none, got:\n%s", f0Str)
	}
	if !groupHasDisplayNone(f0Str, "hideBox") {
		t.Errorf("f0 expected hideBox to be hidden with display:none, got:\n%s", f0Str)
	}

	// Frame 1 (frame 2):
	// showBox: visible (within 2-3) -> visible
	// hideBox: hidden (within 1-2) -> style="display:none"
	f1, err := BuildTimelineFrameSVG(doc, 1, doc.GetDocumentRect())
	if err != nil {
		t.Fatalf("f1 failed: %v", err)
	}
	f1Str := string(f1)
	if groupHasDisplayNone(f1Str, "showBox") {
		t.Errorf("f1 expected showBox to be visible, got:\n%s", f1Str)
	}
	if !groupHasDisplayNone(f1Str, "hideBox") {
		t.Errorf("f1 expected hideBox to be hidden with display:none, got:\n%s", f1Str)
	}

	// Frame 2 (frame 3):
	// showBox: visible (within 2-3) -> visible
	// hideBox: visible (outside 1-2) -> visible
	f2, err := BuildTimelineFrameSVG(doc, 2, doc.GetDocumentRect())
	if err != nil {
		t.Fatalf("f2 failed: %v", err)
	}
	f2Str := string(f2)
	if groupHasDisplayNone(f2Str, "showBox") {
		t.Errorf("f2 expected showBox to be visible, got:\n%s", f2Str)
	}
	if groupHasDisplayNone(f2Str, "hideBox") {
		t.Errorf("f2 expected hideBox to be visible, got:\n%s", f2Str)
	}
}

func TestFadeAndVisibility_MultiMotionCompositing(t *testing.T) {
	svgContent := `<svg width="200" height="200" viewBox="0 0 200 200" xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape">
  <g id="hero" inkscape:groupmode="layer" inkscape:label="Hero">
    <rect id="rect" x="10" y="10" width="30" height="30" fill="gold"/>
    <path id="m1" inkscape:label="Move {f:1-3}" d="M 0,0 L 20,10"/>
    <path id="m2" inkscape:label="Rot {f:1-3 angle:45 pivot:center}" d="M 0,0 L 0,0"/>
    <path id="m3" inkscape:label="Scale {f:1-3 scale:2.0 pivot:center}" d="M 0,0 L 0,0"/>
    <path id="m4" inkscape:label="Fade {f:1-3 from:20 to:80}" d="M 0,0 L 0,0"/>
  </g>
</svg>`

	doc, err := ParseSVG([]byte(svgContent))
	if err != nil {
		t.Fatalf("ParseSVG failed: %v", err)
	}

	// Frame 1 (t=0.5):
	// opacity: 0.2 + 0.5*(0.8 - 0.2) = 0.5000
	// transform contains translate, rotate, and scale!
	f1, err := BuildTimelineFrameSVG(doc, 1, doc.GetDocumentRect())
	if err != nil {
		t.Fatalf("f1 failed: %v", err)
	}
	f1Str := string(f1)
	if !strings.Contains(f1Str, `opacity="0.5000"`) {
		t.Errorf("f1 expected opacity=\"0.5000\", got:\n%s", f1Str)
	}
	if !strings.Contains(f1Str, "translate(") || !strings.Contains(f1Str, "rotate(") || !strings.Contains(f1Str, "scale(") {
		t.Errorf("f1 expected composed translate, rotate, scale transforms, got:\n%s", f1Str)
	}
}

func TestDepth_ReorderingAndRasterOcclusion(t *testing.T) {
	data, err := os.ReadFile("../../testdata/depth_test.svg")
	if err != nil {
		t.Fatalf("Failed to read depth_test.svg: %v", err)
	}

	doc, err := ParseSVG(data)
	if err != nil {
		t.Fatalf("ParseSVG failed: %v", err)
	}

	// Frame 1 (frameIndex 0, outside active range 2-3):
	// Natural order: Red (Z=0) -> Green (Z=1) -> Blue (Z=2)
	f1Bytes, err := BuildTimelineFrameSVG(doc, 0, doc.GetDocumentRect())
	if err != nil {
		t.Fatalf("BuildTimelineFrameSVG frame 1 failed: %v", err)
	}
	f1Str := string(f1Bytes)
	idxRed1 := strings.Index(f1Str, `id="layer_red"`)
	idxGreen1 := strings.Index(f1Str, `id="layer_green"`)
	idxBlue1 := strings.Index(f1Str, `id="layer_blue"`)
	if idxRed1 >= idxGreen1 || idxGreen1 >= idxBlue1 {
		t.Errorf("Frame 1 expected Red < Green < Blue, got Red=%d Green=%d Blue=%d", idxRed1, idxGreen1, idxBlue1)
	}

	// Raster occlusion check on Frame 1:
	// Point (25, 25) is covered by both Red (10..90) and Green (20..80).
	// Because Green is rendered after Red, Green should occlude Red.
	img1, err := RenderSVGToRGBA(f1Bytes, 100, 100)
	if err != nil {
		t.Fatalf("RenderSVGToRGBA frame 1 failed: %v", err)
	}
	c1 := img1.At(25, 25)
	r1, g1, b1, _ := c1.RGBA()
	// 16-bit color: > 0xc000 is > 192 in 8-bit
	if g1 < 0xc000 || r1 > 0x4000 {
		t.Errorf("Frame 1 (25,25) expected Green on top of Red, got RGBA=(%d, %d, %d)", r1>>8, g1>>8, b1>>8)
	}

	// Frame 2 (frameIndex 1, inside active range 2-3 with z: -2):
	// Green is pushed behind Red: Green (Z=-3) -> Red (Z=0) -> Blue (Z=2)
	f2Bytes, err := BuildTimelineFrameSVG(doc, 1, doc.GetDocumentRect())
	if err != nil {
		t.Fatalf("BuildTimelineFrameSVG frame 2 failed: %v", err)
	}
	f2Str := string(f2Bytes)
	idxRed2 := strings.Index(f2Str, `id="layer_red"`)
	idxGreen2 := strings.Index(f2Str, `id="layer_green"`)
	idxBlue2 := strings.Index(f2Str, `id="layer_blue"`)
	if idxGreen2 >= idxRed2 || idxRed2 >= idxBlue2 {
		t.Errorf("Frame 2 expected Green < Red < Blue, got Green=%d Red=%d Blue=%d", idxGreen2, idxRed2, idxBlue2)
	}

	// Raster occlusion check on Frame 2:
	// Because Green is rendered BEFORE Red, Red now occludes Green at (25, 25)!
	img2, err := RenderSVGToRGBA(f2Bytes, 100, 100)
	if err != nil {
		t.Fatalf("RenderSVGToRGBA frame 2 failed: %v", err)
	}
	c2 := img2.At(25, 25)
	r2, g2, b2, _ := c2.RGBA()
	if r2 < 0xc000 || g2 > 0x4000 {
		t.Errorf("Frame 2 (25,25) expected Red on top of Green, got RGBA=(%d, %d, %d)", r2>>8, g2>>8, b2>>8)
	}

	// Frame 4 (frameIndex 3, outside range 2-3):
	// Green returns to natural order: Red -> Green -> Blue
	f4Bytes, err := BuildTimelineFrameSVG(doc, 3, doc.GetDocumentRect())
	if err != nil {
		t.Fatalf("BuildTimelineFrameSVG frame 4 failed: %v", err)
	}
	f4Str := string(f4Bytes)
	idxRed4 := strings.Index(f4Str, `id="layer_red"`)
	idxGreen4 := strings.Index(f4Str, `id="layer_green"`)
	idxBlue4 := strings.Index(f4Str, `id="layer_blue"`)
	if idxRed4 >= idxGreen4 || idxGreen4 >= idxBlue4 {
		t.Errorf("Frame 4 expected Red < Green < Blue, got Red=%d Green=%d Blue=%d", idxRed4, idxGreen4, idxBlue4)
	}

	img4, err := RenderSVGToRGBA(f4Bytes, 100, 100)
	if err != nil {
		t.Fatalf("RenderSVGToRGBA frame 4 failed: %v", err)
	}
	c4 := img4.At(25, 25)
	r4, g4, b4, _ := c4.RGBA()
	if g4 < 0xc000 || r4 > 0x4000 {
		t.Errorf("Frame 4 (25,25) expected Green on top of Red again, got RGBA=(%d, %d, %d)", r4>>8, g4>>8, b4>>8)
	}
}

func TestParseMotionConfig_CameraAndDist(t *testing.T) {
	tests := []struct {
		label      string
		wantType   string
		wantCam    bool
		wantPar    bool
		wantFactor float64
		wantEase   string
		wantStart  int
		wantEnd    int
	}{
		{
			label:     "Camera {f: 1-20; ease: in-out}",
			wantType:  "camera",
			wantCam:   true,
			wantEase:  "in-out",
			wantStart: 1,
			wantEnd:   20,
		},
		{
			label:      "Dist {factor: 0.2}",
			wantType:   "dist",
			wantPar:    true,
			wantFactor: 0.2,
		},
		{
			label:      "Distance {factor: 1.5}",
			wantType:   "dist",
			wantPar:    true,
			wantFactor: 1.5,
		},
		{
			label:      "Dist {depth: 100}",
			wantType:   "dist",
			wantPar:    true,
			wantFactor: 0.5, // 1.0 / (1.0 + 100*0.01) = 0.5
		},
		{
			label:      "Dist {fixed: true}",
			wantType:   "dist",
			wantPar:    true,
			wantFactor: 0.0,
		},
		{
			label:      "Dist {fixed}",
			wantType:   "dist",
			wantPar:    true,
			wantFactor: 0.0,
		},
	}

	for _, tt := range tests {
		cfg, ok := parseMotionConfig(tt.label)
		if !ok {
			t.Errorf("[%s] parseMotionConfig failed", tt.label)
			continue
		}
		if cfg.Type != tt.wantType {
			t.Errorf("[%s] got Type=%s, want %s", tt.label, cfg.Type, tt.wantType)
		}
		if cfg.IsCamera != tt.wantCam {
			t.Errorf("[%s] got IsCamera=%v, want %v", tt.label, cfg.IsCamera, tt.wantCam)
		}
		if cfg.HasParallax != tt.wantPar {
			t.Errorf("[%s] got HasParallax=%v, want %v", tt.label, cfg.HasParallax, tt.wantPar)
		}
		if tt.wantFactor != 0 && math.Abs(cfg.ParallaxFactor-tt.wantFactor) > 1e-4 {
			t.Errorf("[%s] got ParallaxFactor=%f, want %f", tt.label, cfg.ParallaxFactor, tt.wantFactor)
		}
		if tt.wantEase != "" && cfg.Ease != tt.wantEase {
			t.Errorf("[%s] got Ease=%s, want %s", tt.label, cfg.Ease, tt.wantEase)
		}
		if tt.wantStart != 0 && cfg.StartFrame != tt.wantStart {
			t.Errorf("[%s] got StartFrame=%d, want %d", tt.label, cfg.StartFrame, tt.wantStart)
		}
		if tt.wantEnd != 0 && cfg.EndFrame != tt.wantEnd {
			t.Errorf("[%s] got EndFrame=%d, want %d", tt.label, cfg.EndFrame, tt.wantEnd)
		}
	}
}

func TestBuildTimelineFrameSVG_CameraParallax(t *testing.T) {
	data, err := os.ReadFile("../../testdata/parallax_test.svg")
	if err != nil {
		t.Fatalf("Failed to read parallax_test.svg: %v", err)
	}

	doc, err := ParseSVG(data)
	if err != nil {
		t.Fatalf("ParseSVG failed: %v", err)
	}

	if doc.CameraPath == nil {
		t.Fatalf("Expected doc.CameraPath to be populated, got nil")
	}

	// Frame 0 (frame1Idx = 1, t = 0.0):
	// Camera translation is (0, 0), so no parallax offsets
	f0Bytes, err := BuildTimelineFrameSVG(doc, 0, doc.GetDocumentRect())
	if err != nil {
		t.Fatalf("BuildTimelineFrameSVG f0 failed: %v", err)
	}
	f0Str := string(f0Bytes)
	if strings.Contains(f0Str, "cam_track") {
		t.Errorf("Frame 0 expected camera path cam_track to be hidden")
	}

	// Frame 2 (frame1Idx = 3, t = 1.0):
	// Camera has moved +40px horizontally: cx = 40, cy = 0
	// Background (factor 0.1): dx = -40 * 0.1 = -4.0
	// Midground (depth 100 -> factor 0.5): dx = -40 * 0.5 = -20.0
	// Focal Character (default factor 1.0): dx = -40 * 1.0 = -40.0
	// Foreground (factor 1.5): dx = -40 * 1.5 = -60.0
	// HUD (fixed): dx = 0
	f2Bytes, err := BuildTimelineFrameSVG(doc, 2, doc.GetDocumentRect())
	if err != nil {
		t.Fatalf("BuildTimelineFrameSVG f2 failed: %v", err)
	}
	f2Str := string(f2Bytes)

	// Verify Background group translation (-4.0)
	if !strings.Contains(f2Str, "translate(-4.000000, -0.000000)") && !strings.Contains(f2Str, "translate(-4.000000, 0.000000)") {
		t.Errorf("Expected layer_bg to have translate(-4, 0), frame SVG:\n%s", f2Str)
	}

	// Verify Midground group translation (-20.0)
	if !strings.Contains(f2Str, "translate(-20.000000, -0.000000)") && !strings.Contains(f2Str, "translate(-20.000000, 0.000000)") {
		t.Errorf("Expected layer_mid to have translate(-20, 0), frame SVG:\n%s", f2Str)
	}

	// Verify Focal Character translation (-40.0)
	if !strings.Contains(f2Str, "translate(-40.000000, -0.000000)") && !strings.Contains(f2Str, "translate(-40.000000, 0.000000)") {
		t.Errorf("Expected layer_focal to have translate(-40, 0), frame SVG:\n%s", f2Str)
	}

	// Verify Foreground translation (-60.0)
	if !strings.Contains(f2Str, "translate(-60.000000, -0.000000)") && !strings.Contains(f2Str, "translate(-60.000000, 0.000000)") {
		t.Errorf("Expected layer_fg to have translate(-60, 0), frame SVG:\n%s", f2Str)
	}

	// Verify HUD has NO translation
	idxHud := strings.Index(f2Str, `id="layer_hud"`)
	if idxHud != -1 {
		hudSub := f2Str[idxHud : idxHud+strings.Index(f2Str[idxHud:], ">")]
		if strings.Contains(hudSub, "translate") {
			t.Errorf("Expected layer_hud to have NO translate, got: %s", hudSub)
		}
	}

	// Verify nested child inside focal character does NOT double translate
	idxChild := strings.Index(f2Str, `id="focal_child"`)
	if idxChild != -1 {
		childSub := f2Str[idxChild : idxChild+strings.Index(f2Str[idxChild:], ">")]
		if strings.Contains(childSub, "translate(-40") || strings.Contains(childSub, "translate(-80") {
			t.Errorf("Expected focal_child to NOT double translate, got: %s", childSub)
		}
		// But child should still have its local scale transform!
		if !strings.Contains(childSub, "scale(2.000000, 2.000000)") {
			t.Errorf("Expected focal_child to have scale(2, 2), got: %s", childSub)
		}
	}

	// Verify rasterization renders without errors
	img, err := RenderSVGToRGBA(f2Bytes, 200, 200)
	if err != nil {
		t.Fatalf("RenderSVGToRGBA failed on parallax frame: %v", err)
	}
	if img == nil || img.Bounds().Dx() != 200 {
		t.Fatalf("Rendered image bounds unexpected: %v", img.Bounds())
	}
}


