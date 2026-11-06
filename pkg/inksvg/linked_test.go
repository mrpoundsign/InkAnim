package inksvg

import (
	"strings"
	"testing"
)

func TestCleanGroupLabelAndStripIAMS(t *testing.T) {
	rawLabel := "Hero | Move{f:1-8; ease:in-out} | Rot{f:1-8; deg:45}"
	got := CleanGroupLabel(rawLabel)
	if got != "Hero" {
		t.Errorf("CleanGroupLabel(%q) = %q, want %q", rawLabel, got, "Hero")
	}

	svgWithIAMS := `<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape" viewBox="0 0 256 256">
  <g id="layer1" inkscape:groupmode="layer" inkscape:label="MainLayer">
    <g id="hero_g" inkscape:label="Hero | Rot{f:1-10; deg:30}">
      <rect id="body_rect" x="50" y="50" width="80" height="80" fill="#38bdf8"/>
      <path id="helper_move" inkscape:label="Move{f:1-10; ease:in-out}" d="M 50 50 L 150 50" fill="none" stroke="#ff0000"/>
    </g>
  </g>
</svg>`

	stripped, err := StripIAMSFromSVG([]byte(svgWithIAMS))
	if err != nil {
		t.Fatalf("StripIAMSFromSVG failed: %v", err)
	}
	s := string(stripped)
	if strings.Contains(s, "Rot{") || strings.Contains(s, "Move{") {
		t.Errorf("expected IAMS directives to be stripped from SVG, got:\n%s", s)
	}
	if strings.Contains(s, "helper_move") {
		t.Errorf("expected helper motion path #helper_move to be removed, got:\n%s", s)
	}
	if !strings.Contains(s, "body_rect") {
		t.Errorf("expected visual artwork #body_rect to be preserved, got:\n%s", s)
	}

	groups, lookup := ExtractGroupHierarchy(stripped)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups after stripping, got %d", len(groups))
	}
	if lookup["MainLayer/Hero"] != "hero_g" {
		t.Errorf("expected MainLayer/Hero -> hero_g, got %q", lookup["MainLayer/Hero"])
	}
}

func TestComposeLinkedSVGsAndStageObjects(t *testing.T) {
	svg1 := `<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape" width="256" height="256" viewBox="0 0 256 256">
  <defs>
    <linearGradient id="grad1"><stop offset="0%" stop-color="#ff0000"/><stop offset="100%" stop-color="#00ff00"/></linearGradient>
  </defs>
  <g id="layer1" inkscape:groupmode="layer" inkscape:label="Character">
    <g id="arm" inkscape:label="LeftArm">
      <rect id="r1" x="20" y="20" width="40" height="40" fill="url(#grad1)"/>
    </g>
  </g>
</svg>`

	svg2 := `<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape" width="128" height="128" viewBox="0 0 128 128">
  <defs>
    <linearGradient id="grad1"><stop offset="0%" stop-color="#0000ff"/><stop offset="100%" stop-color="#ffff00"/></linearGradient>
  </defs>
  <g id="layer1" inkscape:groupmode="layer" inkscape:label="Props">
    <g id="badge" inkscape:label="Badge">
      <rect id="r2" x="300" y="300" width="30" height="30" fill="url(#grad1)"/>
    </g>
  </g>
</svg>`

	composed, err := ComposeLinkedSVGs([]LinkedSource{
		{ID: "src_1", Name: "Character.svg", RawSVG: []byte(svg1)},
		{ID: "src_2", Name: "Props.svg", RawSVG: []byte(svg2)},
	})
	if err != nil {
		t.Fatalf("ComposeLinkedSVGs failed: %v", err)
	}

	_, lookup := ExtractGroupHierarchy(composed)
	if lookup["src_1/Character/LeftArm"] != "src_1__arm" {
		t.Errorf("expected src_1/Character/LeftArm -> src_1__arm, got %q", lookup["src_1/Character/LeftArm"])
	}
	if lookup["src_2/Props/Badge"] != "src_2__badge" {
		t.Errorf("expected src_2/Props/Badge -> src_2__badge, got %q", lookup["src_2/Props/Badge"])
	}

	doc, err := ParseSVG(composed)
	if err != nil {
		t.Fatalf("ParseSVG(composed) failed: %v", err)
	}
	doc.TotalDurationSec = 2.0

	// Configure Badge to appear at 0.5s, override its offscreen (300,300) start pos to (100,100),
	// and move along an in-app spline from 0.5s to 1.5s
	badgeID := lookup["src_2/Props/Badge"]
	doc.StageObjects[badgeID] = StageObject{
		GroupID:          badgeID,
		HasDisplayWindow: true,
		StartTimeSec:     0.5,
		EndTimeSec:       1.8,
		OverrideStartPos: true,
		StartX:           100,
		StartY:           100,
		PivotType:        "custom",
		PivotX:           115,
		PivotY:           115,
	}
	doc.MotionPaths = append(doc.MotionPaths, MotionPath{
		ID:       "badge_move",
		GroupID:  badgeID,
		PathData: "M 100 100 L 160 100",
		Config: MotionConfig{
			Type:         "move",
			HasTimeRange: true,
			StartSec:     0.5,
			EndSec:       1.5,
			Ease:         "linear",
			Repeat:       1,
		},
	})

	// Frame at t = 0.2s (before StartTimeSec=0.5s): Badge should be hidden
	frameEarly, err := BuildTimelineFrameSVGAtTime(doc, 0.2, 2.0, Rect{})
	if err != nil {
		t.Fatalf("BuildTimelineFrameSVGAtTime(0.2s) failed: %v", err)
	}
	if !strings.Contains(string(frameEarly), `id="src_2__badge" style="display:none"`) {
		t.Errorf("expected src_2__badge to be hidden at t=0.2s, got:\n%s", string(frameEarly))
	}

	// Frame at t = 1.0s (halfway through 0.5s..1.5s move):
	// OverrideStartPos shifts (300,300) -> (100,100) => dx=-200, dy=-200
	// Move halfway along M 100 100 L 160 100 adds dx=+30 => total translate(-170, -200)
	frameMid, err := BuildTimelineFrameSVGAtTime(doc, 1.0, 2.0, Rect{})
	if err != nil {
		t.Fatalf("BuildTimelineFrameSVGAtTime(1.0s) failed: %v", err)
	}
	midStr := string(frameMid)
	if strings.Contains(midStr, `id="src_2__badge" style="display:none"`) {
		t.Errorf("expected src_2__badge to be visible at t=1.0s")
	}
	if !strings.Contains(midStr, "translate(-170.000000, -200.000000)") {
		t.Errorf("expected translate(-170.000000, -200.000000) on src_2__badge at t=1.0s, got:\n%s", midStr)
	}
}
