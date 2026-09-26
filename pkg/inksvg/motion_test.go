package inksvg

import (
	"math"
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

func TestParseMotionConfig_SpaceSeparatedAndReverse(t *testing.T) {
	tests := []struct {
		label      string
		wantOK     bool
		wantStart  int
		wantEnd    int
		wantAll    bool
		wantEase   string
		wantRev    bool
	}{
		{
			label:     "Motion {f:1-15}",
			wantOK:    true,
			wantStart: 1,
			wantEnd:   15,
			wantEase:  "linear",
			wantRev:   false,
		},
		{
			label:     "Motion {f:1-15 rev}",
			wantOK:    true,
			wantStart: 1,
			wantEnd:   15,
			wantEase:  "linear",
			wantRev:   true,
		},
		{
			label:     "Motion {f:1-15 ease:in-out rev}",
			wantOK:    true,
			wantStart: 1,
			wantEnd:   15,
			wantEase:  "in-out",
			wantRev:   true,
		},
		{
			label:     "Motion {f:all reverse}",
			wantOK:    true,
			wantAll:   true,
			wantEase:  "linear",
			wantRev:   true,
		},
		{
			label:     "Motion {f:1-10; ease:out; rev}", // legacy semicolon separation
			wantOK:    true,
			wantStart: 1,
			wantEnd:   10,
			wantEase:  "out",
			wantRev:   true,
		},
		{
			label:     "Regular Label",
			wantOK:    false,
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
	}
}

func TestBuildTimelineFrameSVG_Reverse(t *testing.T) {
	svgContent := `<svg width="100" height="100" viewBox="0 0 100 100" xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape">
  <g id="starGroup" inkscape:groupmode="layer" inkscape:label="Star">
    <rect id="star" x="0" y="0" width="10" height="10"/>
    <path id="motionPath" inkscape:label="Motion {f:1-3 rev}" d="M 0,0 L 100,50"/>
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
