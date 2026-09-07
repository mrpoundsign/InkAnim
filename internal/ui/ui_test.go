package ui

import (
	"path/filepath"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
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
	mw.centerPanel.Pause()
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

func TestNoLoopPlaybackAndButtonStates(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	mw := NewMainWindow(app)
	testSVGPath, err := filepath.Abs("../../testdata/character_walk.svg")
	if err != nil {
		t.Fatalf("failed to resolve test SVG path: %v", err)
	}

	mw.loadFilePath(testSVGPath)
	mw.centerPanel.Pause()

	// Initial stopped state: text is Play and Importance is DangerImportance (Red)
	if mw.centerPanel.playPauseBtn.Text != "Play" {
		t.Errorf("expected button text 'Play', got %s", mw.centerPanel.playPauseBtn.Text)
	}
	if mw.centerPanel.playPauseBtn.Importance != widget.DangerImportance {
		t.Errorf("expected DangerImportance (Red), got %v", mw.centerPanel.playPauseBtn.Importance)
	}

	// Disable loop
	mw.centerPanel.loopCheck.SetChecked(false)

	// Start play from frame 0
	mw.centerPanel.StepFrame(0)
	mw.centerPanel.Play()

	// While playing: text is Pause and Importance is SuccessImportance (Green)
	if mw.centerPanel.playPauseBtn.Text != "Pause" {
		t.Errorf("expected button text 'Pause' while playing, got %s", mw.centerPanel.playPauseBtn.Text)
	}
	if mw.centerPanel.playPauseBtn.Importance != widget.SuccessImportance {
		t.Errorf("expected SuccessImportance (Green) while playing, got %v", mw.centerPanel.playPauseBtn.Importance)
	}

	// Wait for playback to complete (100ms * 3 frames = 300ms + buffer)
	time.Sleep(450 * time.Millisecond)

	mw.centerPanel.mu.Lock()
	playing := mw.centerPanel.isPlaying
	finalIdx := mw.centerPanel.currentIdx
	mw.centerPanel.mu.Unlock()

	if playing {
		t.Errorf("expected playback to stop when reaching the end without loop")
	}
	if finalIdx != 2 {
		t.Errorf("expected to stop on last frame (index 2), got %d", finalIdx)
	}

	// Button should automatically be back to Play and DangerImportance
	if mw.centerPanel.playPauseBtn.Text != "Play" {
		t.Errorf("expected button text 'Play' after stopping at end, got %s", mw.centerPanel.playPauseBtn.Text)
	}
	if mw.centerPanel.playPauseBtn.Importance != widget.DangerImportance {
		t.Errorf("expected DangerImportance (Red) after stopping at end, got %v", mw.centerPanel.playPauseBtn.Importance)
	}

	// Pressing Play again should restart from frame 0
	mw.centerPanel.Play()
	mw.centerPanel.mu.Lock()
	newIdx := mw.centerPanel.currentIdx
	mw.centerPanel.mu.Unlock()

	if newIdx != 0 {
		t.Errorf("expected restart from frame 0, got %d", newIdx)
	}
	mw.centerPanel.Pause()
}

func TestCropBoundaryUIAndGuides(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	mw := NewMainWindow(app)
	if mw == nil {
		t.Fatalf("expected NewMainWindow to succeed")
	}

	multiPagePath, err := filepath.Abs("../../testdata/multipage_walk.svg")
	if err != nil {
		t.Fatalf("failed to resolve multipage SVG path: %v", err)
	}

	mw.loadFilePath(multiPagePath)
	mw.centerPanel.Pause()

	// Initially in Page mode with BoundaryPage
	if mw.session.CropBoundaryMode != "page" {
		t.Errorf("expected page crop boundary, got %s", mw.session.CropBoundaryMode)
	}
	if mw.leftPanel.cropBoundaryRadio.Selected != "Page" {
		t.Errorf("expected cropBoundaryRadio selected 'Page', got %s", mw.leftPanel.cropBoundaryRadio.Selected)
	}

	// Switch to Drawing boundary via radio group
	mw.leftPanel.cropBoundaryRadio.SetSelected("Drawing")
	if mw.session.CropBoundaryMode != "drawing" {
		t.Errorf("expected drawing crop boundary, got %s", mw.session.CropBoundaryMode)
	}
	if !mw.leftPanel.cropPageSelect.Disabled() {
		t.Errorf("expected cropPageSelect to be disabled in Drawing mode")
	}

	// Switch back to Page boundary
	mw.leftPanel.cropBoundaryRadio.SetSelected("Page")
	if mw.session.CropBoundaryMode != "page" {
		t.Errorf("expected page crop boundary, got %s", mw.session.CropBoundaryMode)
	}
	if mw.leftPanel.cropPageSelect.Disabled() {
		t.Errorf("expected cropPageSelect to be enabled in Page mode")
	}

	// Select Document (1st option) in cropPageSelect
	mw.leftPanel.cropPageSelect.SetSelected("Document (560x256)")
	w, h := mw.session.GetActiveBoundaryDimensions()
	if w != 560 || h != 256 {
		t.Errorf("expected 560x256 document boundary, got %fx%f", w, h)
	}

	// Toggle Crop Guides checkbox in center panel
	if !mw.centerPanel.showCropGuides {
		t.Errorf("expected showCropGuides to be true by default")
	}
	mw.centerPanel.cropGuidesCheck.SetChecked(false)
	if mw.centerPanel.showCropGuides {
		t.Errorf("expected showCropGuides to be false after toggle")
	}
	mw.centerPanel.cropGuidesCheck.SetChecked(true)
	if !mw.centerPanel.showCropGuides {
		t.Errorf("expected showCropGuides to be true after toggle")
	}
}

func TestPausePlaybackModal(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	mw := NewMainWindow(app)
	testSVGPath, err := filepath.Abs("../../testdata/character_walk.svg")
	if err != nil {
		t.Fatalf("failed to resolve test SVG path: %v", err)
	}

	mw.loadFilePath(testSVGPath)

	// Ensure playing
	mw.centerPanel.Play()
	if !mw.centerPanel.IsPlaying() {
		t.Fatalf("expected animation to be playing")
	}

	// Calling PausePlayback while playing should pause and return a resume closure
	resume := mw.PausePlayback()
	if mw.centerPanel.IsPlaying() {
		t.Errorf("expected animation to be paused by PausePlayback")
	}

	// Calling resume() should restore playing
	resume()
	if !mw.centerPanel.IsPlaying() {
		t.Errorf("expected animation to resume playing after resume()")
	}

	// Pause manually
	mw.centerPanel.Pause()
	if mw.centerPanel.IsPlaying() {
		t.Fatalf("expected animation to be paused")
	}

	// Calling PausePlayback when already paused should return a no-op closure
	noOpResume := mw.PausePlayback()
	if mw.centerPanel.IsPlaying() {
		t.Errorf("expected animation to remain paused")
	}
	noOpResume()
	if mw.centerPanel.IsPlaying() {
		t.Errorf("expected animation to remain paused after no-op resume")
	}
}

