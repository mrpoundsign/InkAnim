package ui

import (
	"path/filepath"
	"testing"
	"time"

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
	mw.centerPanel.Pause()

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
	mw.centerPanel.Pause()
	mw.leftPanel.modeRadio.SetSelected("Pages")
	if len(mw.session.RenderedFrames) != 2 {
		t.Errorf("expected 2 page frames, got %d", len(mw.session.RenderedFrames))
	}

	// Switch back and forth between SVGs with different frame counts
	for i := 0; i < 5; i++ {
		mw.loadFilePath(testSVGPath)
		mw.centerPanel.Pause()
		mw.centerPanel.StepFrame(2)
		mw.loadFilePath(multiPagePath)
		mw.centerPanel.Pause()
		mw.centerPanel.StepFrame(1)
	}
	mw.centerPanel.Pause()

	// Test changing global speed
	mw.loadFilePath(testSVGPath)
	mw.centerPanel.Pause()
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
	mw.centerPanel.Pause()
	if len(mw.leftPanel.listContainer.Objects) != 3 {
		t.Errorf("expected exactly 3 frame objects in list, got %d", len(mw.leftPanel.listContainer.Objects))
	}
	mw.loadFilePath(multiPagePath)
	mw.centerPanel.Pause()
	if len(mw.leftPanel.listContainer.Objects) != 2 {
		t.Errorf("expected exactly 2 page objects in list, got %d", len(mw.leftPanel.listContainer.Objects))
	}
	mw.loadFilePath(testSVGPath)
	mw.centerPanel.Pause()
	if len(mw.leftPanel.listContainer.Objects) != 3 {
		t.Errorf("expected exactly 3 frame objects in list after switching back, got %d", len(mw.leftPanel.listContainer.Objects))
	}
}

func TestAnimationPlayback(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	mw := NewMainWindow(app)
	testSVGPath, err := filepath.Abs("../../testdata/character_walk.svg")
	if err != nil {
		t.Fatalf("failed to resolve test SVG path: %v", err)
	}

	mw.loadFilePath(testSVGPath)

	t.Logf("Initial isPlaying: %v", mw.centerPanel.isPlaying)
	mw.centerPanel.TogglePlay()
	t.Logf("After TogglePlay isPlaying: %v", mw.centerPanel.isPlaying)
	t.Logf("Initial currentIdx: %d", mw.centerPanel.currentIdx)

	time.Sleep(300 * time.Millisecond)

	mw.centerPanel.mu.Lock()
	idxAfter := mw.centerPanel.currentIdx
	playingAfter := mw.centerPanel.isPlaying
	mw.centerPanel.mu.Unlock()

	t.Logf("After 300ms: idx=%d, isPlaying=%v", idxAfter, playingAfter)
	mw.centerPanel.Pause()
}

func TestCheckFrameDifferences(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	mw := NewMainWindow(app)
	testSVGPath, err := filepath.Abs("../../testdata/character_walk.svg")
	if err != nil {
		t.Fatalf("failed to resolve test SVG path: %v", err)
	}

	mw.loadFilePath(testSVGPath)
	frames := mw.session.RenderedFrames
	if len(frames) != 3 {
		t.Fatalf("expected 3 frames, got %d", len(frames))
	}

	// Compare pixels of frame 0 and frame 1
	f0 := frames[0].Image
	f1 := frames[1].Image
	f2 := frames[2].Image

	diff01 := 0
	for y := 0; y < f0.Bounds().Dy(); y++ {
		for x := 0; x < f0.Bounds().Dx(); x++ {
			if f0.RGBAAt(x, y) != f1.RGBAAt(x, y) {
				diff01++
			}
		}
	}
	t.Logf("Differences between frame 0 and frame 1: %d pixels", diff01)

	diff12 := 0
	for y := 0; y < f1.Bounds().Dy(); y++ {
		for x := 0; x < f1.Bounds().Dx(); x++ {
			if f1.RGBAAt(x, y) != f2.RGBAAt(x, y) {
				diff12++
			}
		}
	}
	t.Logf("Differences between frame 1 and frame 2: %d pixels", diff12)
}


