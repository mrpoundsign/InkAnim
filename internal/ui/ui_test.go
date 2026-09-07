package ui

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	appkg "inkanim/internal/app"
)

func TestMainWindowInitAndLoad(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	mw := NewMainWindow(app)
	if mw == nil {
		t.Fatalf("expected NewMainWindow to succeed")
	}

	if mw.rightPanel.exportBtn.Text != "Export Animated GIF..." {
		t.Errorf("expected initial button text 'Export Animated GIF...', got '%s'", mw.rightPanel.exportBtn.Text)
	}

	// Verify loading demo SVG does not panic
	testSVGPath, err := filepath.Abs("../../testdata/character_walk.svg")
	if err != nil {
		t.Fatalf("failed to resolve test SVG path: %v", err)
	}

	mw.loadFilePath(testSVGPath)
	mw.centerPanel.Pause()

	if !strings.Contains(mw.rightPanel.exportBtn.Text, "(~") {
		t.Errorf("expected exportBtn text to contain estimated size, got '%s'", mw.rightPanel.exportBtn.Text)
	}

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

func TestNumericCommitInput(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	appliedVal := 0
	input := NewNumericCommitInput(100, 16, 4096, "Test:", func(val int) {
		appliedVal = val
	})

	if input.Value != 100 {
		t.Fatalf("expected initial value 100, got %d", input.Value)
	}

	// 1. Test digit filtering
	input.Entry.SetText("abc256!#")
	if input.Entry.Text != "256" {
		t.Errorf("expected filtered text '256', got %q", input.Entry.Text)
	}

	// 2. Test button commit
	test.Tap(input.Button)
	if appliedVal != 256 || input.Value != 256 {
		t.Errorf("expected applied value 256, got %d (input.Value=%d)", appliedVal, input.Value)
	}

	// 3. Test Enter key (OnSubmitted) and min clamping
	input.Entry.SetText("5")
	input.Entry.OnSubmitted("5")
	if appliedVal != 16 || input.Value != 16 {
		t.Errorf("expected min-clamped value 16, got %d (input.Value=%d)", appliedVal, input.Value)
	}

	// 4. Test max clamping
	input.Entry.SetText("99999")
	input.Entry.OnSubmitted("99999")
	if appliedVal != 4096 || input.Value != 4096 {
		t.Errorf("expected max-clamped value 4096, got %d (input.Value=%d)", appliedVal, input.Value)
	}

	// 5. Test SetValue does not call OnApply
	appliedVal = -1
	input.SetValue(512)
	if input.Value != 512 || input.Entry.Text != "512" {
		t.Errorf("expected SetValue to update value and text to 512, got %d / %q", input.Value, input.Entry.Text)
	}
	if appliedVal != -1 {
		t.Errorf("expected SetValue not to invoke OnApply callback")
	}

	// 6. Test Show / Hide
	input.Hide()
	if input.Visible() {
		t.Errorf("expected input to be hidden")
	}
	input.Show()
	if !input.Visible() {
		t.Errorf("expected input to be visible")
	}
}

func TestExportPresetsAndCustomInputs(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	sess := appkg.NewSession()
	win := app.NewWindow("Test")
	defer win.Close()

	changedCount := 0
	panel := NewRightExportPanel(sess, win, func() {
		changedCount++
	})

	// Default state: Twitch Emote (512x512)
	if panel.presetSelect.Selected != "Twitch Emote (512x512 Square)" {
		t.Errorf("expected default preset 'Twitch Emote (512x512 Square)', got %q", panel.presetSelect.Selected)
	}
	if sess.ExportOptions.SquareSize != 512 || !sess.ExportOptions.ExportSquare {
		t.Errorf("expected ExportSquare=true, SquareSize=512, got %v, %d", sess.ExportOptions.ExportSquare, sess.ExportOptions.SquareSize)
	}
	if panel.customResContainer.Visible() {
		t.Errorf("expected custom resolution container to be hidden by default")
	}

	// Switch to Discord Emote
	panel.presetSelect.SetSelected("Discord Emote (128x128 Square)")
	if sess.ExportOptions.SquareSize != 128 || !sess.ExportOptions.ExportSquare {
		t.Errorf("expected Discord square size 128, got %d", sess.ExportOptions.SquareSize)
	}
	if panel.customResContainer.Visible() {
		t.Errorf("expected custom resolution container to remain hidden for Discord preset")
	}

	// Switch to Custom Dimensions
	panel.presetSelect.SetSelected("Custom Dimensions")
	if !panel.customResContainer.Visible() {
		t.Errorf("expected custom resolution container to be visible for Custom Dimensions")
	}

	// Apply custom square resolution
	panel.squareSizeInput.Entry.SetText("300")
	test.Tap(panel.squareSizeInput.Button)
	if sess.ExportOptions.SquareSize != 300 {
		t.Errorf("expected custom resolution 300, got %d", sess.ExportOptions.SquareSize)
	}

	// Test Palette Custom option
	if panel.customColorsRow.Visible() {
		t.Errorf("expected custom colors row to be hidden by default")
	}
	panel.colorsSelect.SetSelected("Custom")
	if !panel.customColorsRow.Visible() {
		t.Errorf("expected custom colors row to be visible when 'Custom' selected")
	}
	panel.customColorsInput.Entry.SetText("48")
	test.Tap(panel.customColorsInput.Button)
	if sess.ExportOptions.NumColors != 48 {
		t.Errorf("expected 48 custom colors, got %d", sess.ExportOptions.NumColors)
	}

	// Switch back to fixed palette preset
	panel.colorsSelect.SetSelected("64")
	if panel.customColorsRow.Visible() {
		t.Errorf("expected custom colors row to be hidden when '64' selected")
	}
	if sess.ExportOptions.NumColors != 64 {
		t.Errorf("expected 64 colors, got %d", sess.ExportOptions.NumColors)
	}
}

func TestSpeedPresetAndCustomInput(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	sess := appkg.NewSession()
	panel := NewLeftFramesPanel(sess, nil)

	// Default: 10 FPS (100ms) and custom input hidden
	if panel.speedPresetSelect.Selected != "10 FPS (100ms)" {
		t.Errorf("expected default speed '10 FPS (100ms)', got %q", panel.speedPresetSelect.Selected)
	}
	if panel.globalMsInput.Visible() {
		t.Errorf("expected custom ms input to be hidden by default")
	}
	if sess.ExportOptions.DefaultDurationMs != 100 {
		t.Errorf("expected default duration 100ms, got %d", sess.ExportOptions.DefaultDurationMs)
	}

	// Switch to 20 FPS (50ms)
	panel.speedPresetSelect.SetSelected("20 FPS (50ms)")
	if panel.globalMsInput.Visible() {
		t.Errorf("expected custom ms input to remain hidden for 20 FPS")
	}
	if sess.ExportOptions.DefaultDurationMs != 50 {
		t.Errorf("expected duration 50ms, got %d", sess.ExportOptions.DefaultDurationMs)
	}

	// Switch to Custom
	panel.speedPresetSelect.SetSelected("Custom")
	if !panel.globalMsInput.Visible() {
		t.Errorf("expected custom ms input to be visible when 'Custom' selected")
	}

	// Apply custom duration
	panel.globalMsInput.Entry.SetText("75")
	test.Tap(panel.globalMsInput.Button)
	if sess.ExportOptions.DefaultDurationMs != 75 {
		t.Errorf("expected custom duration 75ms, got %d", sess.ExportOptions.DefaultDurationMs)
	}
}


