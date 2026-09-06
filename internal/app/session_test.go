package app

import (
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
