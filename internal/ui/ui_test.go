package ui

import (
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2/test"
)

func TestMainWindowInitAndLoad(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	mw := NewMainWindow(app)
	if mw == nil {
		t.Fatalf("expected NewMainWindow to succeed")
	}

	// Verify loading demo SVG does not panic
	testSVGPath, err := filepath.Abs("../../testdata/character_walk.svg")
	if err != nil {
		t.Fatalf("failed to resolve test SVG path: %v", err)
	}

	mw.loadFilePath(testSVGPath)

	if len(mw.session.Layers) != 3 {
		t.Errorf("expected 3 layers, got %d", len(mw.session.Layers))
	}
	if len(mw.session.RenderedFrames) != 3 {
		t.Errorf("expected 3 rendered layer frames, got %d", len(mw.session.RenderedFrames))
	}

	// Test loading multipage SVG
	multiPagePath, err := filepath.Abs("../../testdata/multipage_walk.svg")
	if err != nil {
		t.Fatalf("failed to resolve multipage SVG path: %v", err)
	}

	mw.loadFilePath(multiPagePath)
	mw.leftPanel.modeRadio.SetSelected("Pages")
	if len(mw.session.RenderedFrames) != 2 {
		t.Errorf("expected 2 page frames, got %d", len(mw.session.RenderedFrames))
	}

	// Switch back and forth between SVGs with different frame counts while playing
	for i := 0; i < 5; i++ {
		mw.loadFilePath(testSVGPath)
		mw.centerPanel.TogglePlay()
		mw.centerPanel.StepFrame(2)
		mw.loadFilePath(multiPagePath)
		mw.centerPanel.StepFrame(1)
	}
	mw.centerPanel.Pause()

	// Test changing global speed
	mw.loadFilePath(testSVGPath)
	mw.leftPanel.speedPresetSelect.SetSelected("20 FPS (50ms)")
	if mw.session.ExportOptions.DefaultDurationMs != 50 {
		t.Errorf("expected global duration 50ms, got %d", mw.session.ExportOptions.DefaultDurationMs)
	}
	if mw.session.RenderedFrames[0].DurationMs != 50 {
		t.Errorf("expected frame 0 duration 50ms, got %d", mw.session.RenderedFrames[0].DurationMs)
	}

	// Test per-frame override
	mw.session.SetFrameOverride(0, true, 200)
	if mw.session.RenderedFrames[0].DurationMs != 200 {
		t.Errorf("expected frame 0 overridden to 200ms, got %d", mw.session.RenderedFrames[0].DurationMs)
	}
	if mw.session.RenderedFrames[1].DurationMs != 50 {
		t.Errorf("expected frame 1 to remain 50ms, got %d", mw.session.RenderedFrames[1].DurationMs)
	}

	// Verify that switching between SVGs does not double the list items
	mw.loadFilePath(testSVGPath)
	if len(mw.leftPanel.listContainer.Objects) != 3 {
		t.Errorf("expected exactly 3 frame objects in list, got %d", len(mw.leftPanel.listContainer.Objects))
	}
	mw.loadFilePath(multiPagePath)
	if len(mw.leftPanel.listContainer.Objects) != 2 {
		t.Errorf("expected exactly 2 page objects in list, got %d", len(mw.leftPanel.listContainer.Objects))
	}
	mw.loadFilePath(testSVGPath)
	if len(mw.leftPanel.listContainer.Objects) != 3 {
		t.Errorf("expected exactly 3 frame objects in list after switching back, got %d", len(mw.leftPanel.listContainer.Objects))
	}
}
