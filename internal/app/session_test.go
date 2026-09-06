package app

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestSessionGlobalAndOverrideDurations(t *testing.T) {
	testSVGPath, err := filepath.Abs("../../testdata/character_walk.svg")
	if err != nil {
		t.Fatalf("failed to resolve test SVG path: %v", err)
	}

	sess := NewSession()
	if err := sess.LoadSVG(testSVGPath); err != nil {
		t.Fatalf("failed to load SVG: %v", err)
	}

	if len(sess.RenderedFrames) != 3 {
		t.Fatalf("expected 3 rendered frames, got %d", len(sess.RenderedFrames))
	}

	// Default global duration is 100ms
	for i, f := range sess.RenderedFrames {
		if f.DurationMs != 100 {
			t.Errorf("frame %d expected default 100ms, got %d", i, f.DurationMs)
		}
	}

	// Change global duration to 50ms
	sess.SetGlobalDuration(50)
	for i, f := range sess.RenderedFrames {
		if f.DurationMs != 50 {
			t.Errorf("frame %d expected updated 50ms, got %d", i, f.DurationMs)
		}
	}

	// Set override on frame 1 (index 1) to 300ms
	sess.SetFrameOverride(1, true, 300)
	if sess.RenderedFrames[0].DurationMs != 50 {
		t.Errorf("frame 0 should remain 50ms, got %d", sess.RenderedFrames[0].DurationMs)
	}
	if sess.RenderedFrames[1].DurationMs != 300 {
		t.Errorf("frame 1 should be overridden to 300ms, got %d", sess.RenderedFrames[1].DurationMs)
	}
	if sess.RenderedFrames[2].DurationMs != 50 {
		t.Errorf("frame 2 should remain 50ms, got %d", sess.RenderedFrames[2].DurationMs)
	}

	// Change global duration to 75ms: frame 0 and 2 should update, frame 1 should keep 300ms
	sess.SetGlobalDuration(75)
	if sess.RenderedFrames[0].DurationMs != 75 {
		t.Errorf("frame 0 should update to 75ms, got %d", sess.RenderedFrames[0].DurationMs)
	}
	if sess.RenderedFrames[1].DurationMs != 300 {
		t.Errorf("frame 1 override should stay 300ms, got %d", sess.RenderedFrames[1].DurationMs)
	}
	if sess.RenderedFrames[2].DurationMs != 75 {
		t.Errorf("frame 2 should update to 75ms, got %d", sess.RenderedFrames[2].DurationMs)
	}

	// Unset override on frame 1: should immediately revert to global (75ms)
	sess.SetFrameOverride(1, false, 0)
	if sess.RenderedFrames[1].DurationMs != 75 {
		t.Errorf("frame 1 should revert to 75ms, got %d", sess.RenderedFrames[1].DurationMs)
	}
}

func TestSessionToggleLayerPinnedAndActive(t *testing.T) {
	testSVGPath, err := filepath.Abs("../../testdata/character_walk.svg")
	if err != nil {
		t.Fatalf("failed to resolve test SVG path: %v", err)
	}

	// Test pinning frame 1 (index 1)
	sess := NewSession()
	if err := sess.LoadSVG(testSVGPath); err != nil {
		t.Fatalf("failed to load SVG: %v", err)
	}

	t.Logf("Initial frames count: %d", len(sess.RenderedFrames))

	// Pin frame 1
	if err := sess.ToggleLayerPinned(1); err != nil {
		t.Fatalf("ToggleLayerPinned(1) failed: %v", err)
	}
	t.Logf("Frames count after pinning frame 1: %d", len(sess.RenderedFrames))
	if len(sess.RenderedFrames) != 2 {
		t.Errorf("expected 2 frames after pinning frame 1, got %d", len(sess.RenderedFrames))
	}

	// Pin frame 2
	sess2 := NewSession()
	sess2.LoadSVG(testSVGPath)
	if err := sess2.ToggleLayerPinned(2); err != nil {
		t.Fatalf("ToggleLayerPinned(2) failed: %v", err)
	}
	t.Logf("Frames count after pinning frame 2: %d", len(sess2.RenderedFrames))
	if len(sess2.RenderedFrames) != 2 {
		t.Errorf("expected 2 frames after pinning frame 2, got %d", len(sess2.RenderedFrames))
	}

	// Disable frame 1
	sess3 := NewSession()
	sess3.LoadSVG(testSVGPath)
	if err := sess3.ToggleLayerActive(1); err != nil {
		t.Fatalf("ToggleLayerActive(1) failed: %v", err)
	}
	t.Logf("Frames count after disabling frame 1: %d", len(sess3.RenderedFrames))
	if len(sess3.RenderedFrames) != 2 {
		t.Errorf("expected 2 frames after disabling frame 1, got %d", len(sess3.RenderedFrames))
	}
}

func TestSessionLoadSVGDataAndExportWriter(t *testing.T) {
	testSVGPath, err := filepath.Abs("../../testdata/character_walk.svg")
	if err != nil {
		t.Fatalf("failed to resolve test SVG path: %v", err)
	}

	data, err := os.ReadFile(testSVGPath)
	if err != nil {
		t.Fatalf("failed to read test SVG: %v", err)
	}

	sess := NewSession()
	if err := sess.LoadSVGData(data, "test.svg"); err != nil {
		t.Fatalf("LoadSVGData failed: %v", err)
	}

	if len(sess.Layers) != 3 {
		t.Fatalf("expected 3 layers, got %d", len(sess.Layers))
	}

	// Test ExportGIFWriter
	var buf bytes.Buffer
	written, err := sess.ExportGIFWriter(&buf)
	if err != nil {
		t.Fatalf("ExportGIFWriter failed: %v", err)
	}
	if written <= 0 || buf.Len() == 0 {
		t.Errorf("expected non-empty output buffer, got %d bytes", written)
	}
}

