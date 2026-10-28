package app

import (
	"os"
	"path/filepath"
	"testing"

	"inkanim/pkg/inksvg"
)

func TestControlFileRoundTripAndResolve(t *testing.T) {
	tmpDir := t.TempDir()

	srcSVG, err := filepath.Abs("../../testdata/pendulum.svg")
	if err != nil {
		t.Fatalf("failed to resolve test SVG path: %v", err)
	}
	svgBytes, err := os.ReadFile(srcSVG)
	if err != nil {
		t.Fatalf("failed to read test SVG: %v", err)
	}

	svgPath := filepath.Join(tmpDir, "emote.svg")
	if err := os.WriteFile(svgPath, svgBytes, 0644); err != nil {
		t.Fatalf("failed to write temp SVG: %v", err)
	}

	// 1. Resolve when no .inka sidecar exists yet
	resolvedSVG, ctrl, err := ResolveSVGAndControlFile(svgPath)
	if err != nil {
		t.Fatalf("ResolveSVGAndControlFile failed: %v", err)
	}
	if resolvedSVG != svgPath {
		t.Errorf("expected resolved SVG %q, got %q", svgPath, resolvedSVG)
	}
	if ctrl != nil {
		t.Errorf("expected nil ControlFile when no .inka exists, got %+v", ctrl)
	}

	// 2. Build and save a .inka control file
	sess := NewSession()
	if err := sess.LoadSVG(svgPath); err != nil {
		t.Fatalf("LoadSVG failed: %v", err)
	}
	_ = sess.SetFPS(20)
	_ = sess.SetTotalDurationSec(2.5)
	_ = sess.SetCropBoundary(inksvg.BoundaryDrawing, 0)
	sess.ExportOptions.PingPong = true
	sess.ExportOptions.ExportSquare = true
	sess.ExportOptions.SquareSize = 128
	sess.ExportOptions.NumColors = 64
	sess.ExportOptions.Dither = true
	sess.ShowMotionLines = true

	built := sess.BuildControlFile(svgPath)
	built.Export.Preset = "Discord Emote (128x128 Square)"
	built.Preview.Background = "Twitch Dark"
	built.Preview.ShowGuides = false
	built.Preview.ShowScales = false

	inkaPath := SidecarPathForSVG(svgPath)
	if filepath.Base(inkaPath) != "emote.inka" {
		t.Fatalf("expected emote.inka, got %s", inkaPath)
	}
	if err := SaveControlFile(inkaPath, built); err != nil {
		t.Fatalf("SaveControlFile failed: %v", err)
	}

	// 3. Resolve from .svg path (auto-detects sibling .inka)
	resolvedFromSVG, ctrlFromSVG, err := ResolveSVGAndControlFile(svgPath)
	if err != nil {
		t.Fatalf("ResolveSVGAndControlFile(svgPath) failed: %v", err)
	}
	if resolvedFromSVG != svgPath || ctrlFromSVG == nil {
		t.Fatalf("expected non-nil ControlFile from sibling .inka")
	}

	// 4. Resolve directly from .inka path
	resolvedFromInka, ctrlFromInka, err := ResolveSVGAndControlFile(inkaPath)
	if err != nil {
		t.Fatalf("ResolveSVGAndControlFile(inkaPath) failed: %v", err)
	}
	if resolvedFromInka != svgPath || ctrlFromInka == nil {
		t.Fatalf("expected resolved SVG %q from .inka, got %q", svgPath, resolvedFromInka)
	}

	// 5. Apply control file to a fresh session before LoadSVGData
	sess2 := NewSession()
	sess2.ApplyControlFile(ctrlFromInka)
	if err := sess2.LoadSVGData(svgBytes, svgPath); err != nil {
		t.Fatalf("sess2.LoadSVGData failed: %v", err)
	}
	if sess2.FPS != 20 {
		t.Errorf("expected FPS 20, got %d", sess2.FPS)
	}
	if sess2.TotalDurationMs != 2500 {
		t.Errorf("expected TotalDurationMs 2500, got %d", sess2.TotalDurationMs)
	}
	if len(sess2.RenderedFrames) != 50 {
		t.Errorf("expected 50 interpolated frames (2.5s @ 20 FPS), got %d", len(sess2.RenderedFrames))
	}
	if sess2.CropBoundaryMode != inksvg.BoundaryDrawing {
		t.Errorf("expected CropBoundaryMode drawing, got %s", sess2.CropBoundaryMode)
	}
	if !sess2.ExportOptions.PingPong || sess2.ExportOptions.SquareSize != 128 || sess2.ExportOptions.NumColors != 64 || !sess2.ExportOptions.Dither {
		t.Errorf("unexpected ExportOptions after ApplyControlFile: %+v", sess2.ExportOptions)
	}
	if !sess2.ShowMotionLines {
		t.Errorf("expected ShowMotionLines=true after ApplyControlFile")
	}
}
