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
}
