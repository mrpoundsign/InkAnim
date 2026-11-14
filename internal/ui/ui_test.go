package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	appkg "inkanim/internal/app"
	svg "inkanim/pkg/inksvg"
)

// uiQueue receives async preview-build callbacks in tests. The Fyne test driver
// runs fyne.Do inline on the calling goroutine, which would let build goroutines
// mutate widgets concurrently with the test goroutine; draining this queue from
// the test goroutine restores production's single-UI-thread semantics.
var uiQueue = make(chan func(), 4096)

func TestMain(m *testing.M) {
	dispatchUI = func(f func()) { uiQueue <- f }
	os.Exit(m.Run())
}

// waitForPreview runs queued UI callbacks on the calling (test) goroutine until
// no preview build is in flight and nothing is left to apply.
func waitForPreview(mw *MainWindow) {
	mw.flushPreviewUpdate()
	p := mw.centerPanel
	for {
		select {
		case f := <-uiQueue:
			f()
			continue
		default:
		}
		// A build enqueues its result before decrementing the counter, so once it
		// reads zero every result is already queued; applying one may start another.
		if p.rebuildsInFlight.Load() == 0 && len(uiQueue) == 0 {
			return
		}
		select {
		case f := <-uiQueue:
			f()
		case <-time.After(time.Millisecond):
		}
	}
}

// loadAndWait loads a file and waits for its async preview build to be applied.
func loadAndWait(mw *MainWindow, path string) {
	mw.loadFilePath(path)
	waitForPreview(mw)
}

func TestAsyncPreviewBuildAndInspectorFollowsPlayback(t *testing.T) {
	testApp := test.NewApp()
	defer testApp.Quit()

	mw := NewMainWindow(testApp)
	defer func() {
		waitForPreview(mw)
		mw.centerPanel.Pause()
	}() // settle async preview builds before the next test
	testSVGPath, err := filepath.Abs("../../testdata/pendulum.svg")
	if err != nil {
		t.Fatalf("failed to resolve test SVG path: %v", err)
	}

	loadAndWait(mw, testSVGPath)
	mw.centerPanel.Pause()
	waitForPreview(mw)

	p := mw.centerPanel
	p.mu.Lock()
	if p.rebuilding {
		t.Errorf("expected no build in flight after waiting")
	}
	if p.progressOverlay.Visible() {
		t.Errorf("expected progress overlay hidden once the preview cache is applied")
	}
	if len(p.cachedFrames) != len(mw.session.RenderedFrames) {
		t.Fatalf("expected %d cached frames, got %d", len(mw.session.RenderedFrames), len(p.cachedFrames))
	}
	for i, c := range p.cachedFrames {
		if c.withGuides == nil || c.withoutGuides == nil || c.twitch112 == nil || c.twitch56 == nil || c.twitch28 == nil {
			t.Fatalf("frame %d: expected all preview images precomputed", i)
		}
		if b := c.twitch28.Bounds(); b.Dx() != 28 || b.Dy() != 28 {
			t.Errorf("frame %d: expected 28x28 thumbnail, got %v", i, b)
		}
	}

	// Inspector must update on every frame step, including while playing (regression).
	p.isPlaying = true
	for step := range 3 {
		p.currentIdx = step + 1
		p.renderCurrentFrameLocked()
		if p.twitch112Dark.Image != p.cachedFrames[p.currentIdx].twitch112 {
			t.Errorf("step %d: inspector 112px dark not updated during playback", step)
		}
		if p.twitch28Light.Image != p.cachedFrames[p.currentIdx].twitch28 {
			t.Errorf("step %d: inspector 28px light not updated during playback", step)
		}
	}
	p.isPlaying = false
	p.mu.Unlock()
}

func TestMainWindowInitAndLoad(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	mw := NewMainWindow(app)
	defer func() {
		waitForPreview(mw)
		mw.centerPanel.Pause()
	}() // settle async preview builds before the next test
	if mw == nil {
		t.Fatalf("expected NewMainWindow to succeed")
	}

	if mw.rightPanel.exportBtn.Text != "Export Animated GIF..." {
		t.Errorf("expected initial button text 'Export Animated GIF...', got '%s'", mw.rightPanel.exportBtn.Text)
	}

	// Verify loading demo SVG does not panic
	testSVGPath, err := filepath.Abs("../../testdata/pendulum.svg")
	if err != nil {
		t.Fatalf("failed to resolve test SVG path: %v", err)
	}

	loadAndWait(mw, testSVGPath)
	mw.centerPanel.Pause()

	if mw.rightPanel.exportBtn.Text != "Export Animated GIF..." {
		t.Errorf("expected exportBtn text to be 'Export Animated GIF...', got '%s'", mw.rightPanel.exportBtn.Text)
	}

	if len(mw.session.Frames) != 10 {
		t.Errorf("expected 10 frames, got %d", len(mw.session.Frames))
	}
	if len(mw.session.RenderedFrames) != 10 {
		t.Errorf("expected 10 rendered frames, got %d", len(mw.session.RenderedFrames))
	}

	// Test loading second SVG with different frame count
	secondSVGPath, err := filepath.Abs("../../testdata/color_test.svg")
	if err != nil {
		t.Fatalf("failed to resolve color_test SVG path: %v", err)
	}

	loadAndWait(mw, secondSVGPath)
	mw.centerPanel.Pause()
	if len(mw.session.RenderedFrames) != 30 {
		t.Errorf("expected 30 frames, got %d", len(mw.session.RenderedFrames))
	}

	// Switch back and forth between SVGs with different frame counts
	for range 5 {
		loadAndWait(mw, testSVGPath)
		mw.centerPanel.Pause()
		mw.centerPanel.StepFrame(2)
		loadAndWait(mw, secondSVGPath)
		mw.centerPanel.Pause()
		mw.centerPanel.StepFrame(1)
	}
	mw.centerPanel.Pause()

	// Test changing FPS and duration in centerPanel
	loadAndWait(mw, testSVGPath)
	mw.centerPanel.Pause()
	mw.centerPanel.fpsSelect.SetSelected("20 FPS (50ms)")
	waitForPreview(mw)
	if mw.session.ExportOptions.DefaultDurationMs != 50 {
		t.Errorf("expected global duration 50ms, got %d", mw.session.ExportOptions.DefaultDurationMs)
	}
	if len(mw.session.RenderedFrames) != 20 {
		t.Errorf("expected 20 interpolated frames at 1.0s @ 20 FPS, got %d", len(mw.session.RenderedFrames))
	}
	if mw.session.RenderedFrames[0].DurationMs != 50 {
		t.Errorf("expected frame 0 duration 50ms, got %d", mw.session.RenderedFrames[0].DurationMs)
	}

	// Test changing duration to 1.5s at 20 FPS -> 30 frames
	mw.centerPanel.durationInput.Entry.SetText("1.5")
	mw.centerPanel.durationInput.Entry.OnSubmitted("1.5")
	waitForPreview(mw)
	if len(mw.session.RenderedFrames) != 30 {
		t.Errorf("expected 30 interpolated frames at 1.5s @ 20 FPS, got %d", len(mw.session.RenderedFrames))
	}
	if !strings.Contains(mw.centerPanel.totalFramesLabel.Text, "Total: 30 frames") {
		t.Errorf("expected totalFramesLabel to contain 'Total: 30 frames', got %q", mw.centerPanel.totalFramesLabel.Text)
	}
}

func TestAnimationPlayback(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	mw := NewMainWindow(app)
	defer func() {
		waitForPreview(mw)
		mw.centerPanel.Pause()
	}() // settle async preview builds before the next test
	testSVGPath, err := filepath.Abs("../../testdata/pendulum.svg")
	if err != nil {
		t.Fatalf("failed to resolve test SVG path: %v", err)
	}

	loadAndWait(mw, testSVGPath)

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
	defer func() {
		waitForPreview(mw)
		mw.centerPanel.Pause()
	}() // settle async preview builds before the next test
	testSVGPath, err := filepath.Abs("../../testdata/pendulum.svg")
	if err != nil {
		t.Fatalf("failed to resolve test SVG path: %v", err)
	}

	loadAndWait(mw, testSVGPath)
	mw.centerPanel.Pause()
	frames := mw.session.RenderedFrames
	if len(frames) != 10 {
		t.Fatalf("expected 10 frames, got %d", len(frames))
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
	if diff01 == 0 {
		t.Errorf("expected differences between frame 0 and frame 1")
	}

	diff12 := 0
	for y := 0; y < f1.Bounds().Dy(); y++ {
		for x := 0; x < f1.Bounds().Dx(); x++ {
			if f1.RGBAAt(x, y) != f2.RGBAAt(x, y) {
				diff12++
			}
		}
	}
	t.Logf("Differences between frame 1 and frame 2: %d pixels", diff12)
	if diff12 == 0 {
		t.Errorf("expected differences between frame 1 and frame 2")
	}
}

func TestNoLoopPlaybackAndButtonStates(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	mw := NewMainWindow(app)
	defer func() {
		waitForPreview(mw)
		mw.centerPanel.Pause()
	}() // settle async preview builds before the next test
	testSVGPath, err := filepath.Abs("../../testdata/pendulum.svg")
	if err != nil {
		t.Fatalf("failed to resolve test SVG path: %v", err)
	}

	loadAndWait(mw, testSVGPath)
	mw.centerPanel.Pause()
	waitForPreview(mw)

	// Initial stopped state: text is Play and Importance is DangerImportance (Red)
	txt, imp := mw.centerPanel.PlayPauseButtonState()
	if txt != "Play" {
		t.Errorf("expected button text 'Play', got %s", txt)
	}
	if imp != widget.DangerImportance {
		t.Errorf("expected DangerImportance (Red), got %v", imp)
	}

	// Disable loop
	mw.centerPanel.loopCheck.SetChecked(false)

	// Start play from frame 0 (10 frames at default 100ms = 1,000ms total)
	mw.centerPanel.StepFrame(0)
	mw.centerPanel.Play()

	// While playing: text is Pause and Importance is SuccessImportance (Green)
	txt, imp = mw.centerPanel.PlayPauseButtonState()
	if txt != "Pause" {
		t.Errorf("expected button text 'Pause' while playing, got %s", txt)
	}
	if imp != widget.SuccessImportance {
		t.Errorf("expected SuccessImportance (Green) while playing, got %v", imp)
	}

	// Wait for playback to complete (10 frames * 100ms = 1,000ms + buffer)
	time.Sleep(1200 * time.Millisecond)

	mw.centerPanel.mu.Lock()
	playing := mw.centerPanel.isPlaying
	finalIdx := mw.centerPanel.currentIdx
	mw.centerPanel.mu.Unlock()

	if playing {
		t.Errorf("expected playback to stop when reaching the end without loop")
	}
	if finalIdx != 9 {
		t.Errorf("expected to stop on last frame (index 9), got %d", finalIdx)
	}

	// Button should automatically be back to Play and DangerImportance
	txt, imp = mw.centerPanel.PlayPauseButtonState()
	if txt != "Play" {
		t.Errorf("expected button text 'Play' after stopping at end, got %s", txt)
	}
	if imp != widget.DangerImportance {
		t.Errorf("expected DangerImportance (Red) after stopping at end, got %v", imp)
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

func TestPingPongPlaybackAndExportUI(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	mw := NewMainWindow(app)
	defer func() {
		waitForPreview(mw)
		mw.centerPanel.Pause()
	}() // settle async preview builds before the next test
	testSVGPath, err := filepath.Abs("../../testdata/pendulum.svg")
	if err != nil {
		t.Fatalf("failed to resolve test SVG path: %v", err)
	}

	loadAndWait(mw, testSVGPath)
	mw.centerPanel.Pause()

	// 1. pendulum.svg has 10 frames -> Ping-Pong should be enabled
	if mw.centerPanel.pingPongCheck.Disabled() {
		t.Errorf("expected pingPongCheck to be enabled for 10 frames")
	}

	// 2. Enable Ping-Pong
	mw.centerPanel.pingPongCheck.SetChecked(true)
	if !mw.session.ExportOptions.PingPong {
		t.Errorf("expected session ExportOptions.PingPong to be true")
	}

	// 3. Verify Twitch status and export button preserve the 10-frame (1.0s) total cycle duration
	if !strings.Contains(mw.rightPanel.twitchStatusLabel.Text, "10 frames - 1.0s") {
		t.Errorf("expected twitchStatusLabel to preserve 10 bounced frames (1.0s), got '%s'", mw.rightPanel.twitchStatusLabel.Text)
	}

	// 4. Test bounce playback sequence
	mw.centerPanel.mu.Lock()
	mw.centerPanel.currentIdx = 0
	mw.centerPanel.pingPongDir = 1
	mw.centerPanel.speedFactor = 2.0 // 2x speed for fast test
	mw.centerPanel.mu.Unlock()

	mw.centerPanel.Play()
	time.Sleep(200 * time.Millisecond)
	mw.centerPanel.Pause()

	// 5. Edge case: load 1-frame document -> Ping-Pong must be disabled
	staticPath, err := filepath.Abs("../../testdata/Alert Icon.svg")
	if err != nil {
		t.Fatalf("failed to resolve Alert Icon SVG path: %v", err)
	}
	loadAndWait(mw, staticPath)
	mw.centerPanel.Pause()

	if len(mw.session.RenderedFrames) != 1 {
		t.Fatalf("expected 1 frame for static SVG, got %d", len(mw.session.RenderedFrames))
	}
	if !mw.centerPanel.pingPongCheck.Disabled() {
		t.Errorf("expected pingPongCheck to be disabled for < 3 frames")
	}
}

func TestCropBoundaryUIAndGuides(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	mw := NewMainWindow(app)
	defer func() {
		waitForPreview(mw)
		mw.centerPanel.Pause()
	}() // settle async preview builds before the next test
	if mw == nil {
		t.Fatalf("expected NewMainWindow to succeed")
	}

	multiPagePath, err := filepath.Abs("../../testdata/multipage_walk.svg")
	if err != nil {
		t.Fatalf("failed to resolve multipage SVG path: %v", err)
	}

	loadAndWait(mw, multiPagePath)
	mw.centerPanel.Pause()

	// Initially in Page boundary mode
	if mw.session.CropBoundaryMode != "page" {
		t.Errorf("expected page crop boundary, got %s", mw.session.CropBoundaryMode)
	}
	if mw.rightPanel.cropBoundaryRadio.Selected != "Page" {
		t.Errorf("expected cropBoundaryRadio selected 'Page', got %s", mw.rightPanel.cropBoundaryRadio.Selected)
	}
	if mw.rightPanel.cropPageSelect.Disabled() {
		t.Errorf("expected cropPageSelect to be enabled in Page mode")
	}

	// Switch to Drawing boundary via radio group
	mw.rightPanel.cropBoundaryRadio.SetSelected("Drawing")
	if mw.session.CropBoundaryMode != "drawing" {
		t.Errorf("expected drawing crop boundary, got %s", mw.session.CropBoundaryMode)
	}
	if !mw.rightPanel.cropPageSelect.Disabled() {
		t.Errorf("expected cropPageSelect to be disabled in Drawing mode")
	}

	// Switch back to Page boundary
	mw.rightPanel.cropBoundaryRadio.SetSelected("Page")

	// Select Document (1st option) in cropPageSelect
	mw.rightPanel.cropPageSelect.SetSelected("Document (560x256)")
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
	defer func() {
		waitForPreview(mw)
		mw.centerPanel.Pause()
	}() // settle async preview builds before the next test
	testSVGPath, err := filepath.Abs("../../testdata/pendulum.svg")
	if err != nil {
		t.Fatalf("failed to resolve test SVG path: %v", err)
	}

	loadAndWait(mw, testSVGPath)

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

func TestSpeedPresets(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	sess := appkg.NewSession()
	panel := NewCenterPreviewPanel(sess)

	// Default: 10 FPS (100ms)
	if panel.fpsSelect.Selected != "10 FPS (100ms)" {
		t.Errorf("expected default speed '10 FPS (100ms)', got %q", panel.fpsSelect.Selected)
	}
	if sess.ExportOptions.DefaultDurationMs != 100 {
		t.Errorf("expected default duration 100ms, got %d", sess.ExportOptions.DefaultDurationMs)
	}

	// Switch to 20 FPS (50ms)
	panel.fpsSelect.SetSelected("20 FPS (50ms)")
	if sess.ExportOptions.DefaultDurationMs != 50 {
		t.Errorf("expected duration 50ms, got %d", sess.ExportOptions.DefaultDurationMs)
	}

	// Switch to 60 FPS (16ms)
	panel.fpsSelect.SetSelected("60 FPS (16ms)")
	if sess.ExportOptions.DefaultDurationMs != 17 && sess.ExportOptions.DefaultDurationMs != 16 {
		t.Errorf("expected duration 16 or 17ms, got %d", sess.ExportOptions.DefaultDurationMs)
	}
}

func TestSidebarTabs(t *testing.T) {
	testApp := test.NewApp()
	defer testApp.Quit()

	mw := NewMainWindow(testApp)
	defer func() {
		waitForPreview(mw)
		mw.centerPanel.Pause()
	}() // settle async preview builds before the next test
	if mw.sidebarTabs == nil {
		t.Fatal("expected sidebarTabs to be non-nil")
	}

	if len(mw.sidebarTabs.Items) != 2 {
		t.Fatalf("expected 2 sidebar tabs, got %d", len(mw.sidebarTabs.Items))
	}

	if mw.sidebarTabs.Items[0].Text != "Layers & Motion" {
		t.Errorf("expected first tab to be 'Layers & Motion', got %q", mw.sidebarTabs.Items[0].Text)
	}
	if mw.sidebarTabs.Items[1].Text != "Export" {
		t.Errorf("expected second tab to be 'Export', got %q", mw.sidebarTabs.Items[1].Text)
	}

	// Switch tabs
	mw.sidebarTabs.SelectIndex(1)
	if mw.sidebarTabs.SelectedIndex() != 1 {
		t.Errorf("expected selected tab index 1, got %d", mw.sidebarTabs.SelectedIndex())
	}
	mw.sidebarTabs.SelectIndex(0)
	if mw.sidebarTabs.SelectedIndex() != 0 {
		t.Errorf("expected selected tab index 0, got %d", mw.sidebarTabs.SelectedIndex())
	}
}

func TestScaleInspectorToggle(t *testing.T) {
	testApp := test.NewApp()
	defer testApp.Quit()

	mw := NewMainWindow(testApp)
	defer func() {
		waitForPreview(mw)
		mw.centerPanel.Pause()
	}() // settle async preview builds before the next test
	if mw.centerPanel.inspectorCheck == nil {
		t.Fatal("expected inspectorCheck to be non-nil")
	}

	// Initially visible
	if !mw.centerPanel.inspectorCheck.Checked {
		t.Errorf("expected inspectorCheck to be checked by default")
	}
	if !mw.centerPanel.twitchEmulationBox.Visible() {
		t.Errorf("expected twitchEmulationBox to be visible by default")
	}

	// Resize window to layout objects
	mw.window.Resize(fyne.NewSize(1200, 750))
	mw.window.Content().Resize(fyne.NewSize(1200, 750))
	wWithInspector := mw.centerPanel.mainCanvasImage.Size().Width

	// Hide inspector
	mw.centerPanel.inspectorCheck.SetChecked(false)
	if mw.centerPanel.twitchEmulationBox.Visible() {
		t.Errorf("expected twitchEmulationBox to be hidden after unchecking")
	}

	wWithoutInspector := mw.centerPanel.mainCanvasImage.Size().Width
	t.Logf("Main canvas width: with inspector=%.0f, without inspector=%.0f (+%.0f px)", wWithInspector, wWithoutInspector, wWithoutInspector-wWithInspector)
	if wWithoutInspector <= wWithInspector {
		t.Errorf("expected main canvas to expand width when inspector hidden: with=%f, without=%f", wWithInspector, wWithoutInspector)
	}

	// Show inspector via helper
	mw.centerPanel.SetInspectorVisible(true)
	if !mw.centerPanel.inspectorCheck.Checked {
		t.Errorf("expected inspectorCheck to be checked after SetInspectorVisible(true)")
	}
	if !mw.centerPanel.twitchEmulationBox.Visible() {
		t.Errorf("expected twitchEmulationBox to be visible after SetInspectorVisible(true)")
	}
	if mw.centerPanel.mainCanvasImage.Size().Width != wWithInspector {
		t.Errorf("expected main canvas to restore width when inspector shown: got %f, want %f", mw.centerPanel.mainCanvasImage.Size().Width, wWithInspector)
	}
}

func TestWysiwygPreview(t *testing.T) {
	testApp := test.NewApp()
	defer testApp.Quit()

	mw := NewMainWindow(testApp)
	defer func() {
		waitForPreview(mw)
		mw.centerPanel.Pause()
	}() // settle async preview builds before the next test
	testSVGPath, err := filepath.Abs("../../testdata/bouncing_walker.svg")
	if err != nil {
		t.Fatalf("failed to resolve test SVG path: %v", err)
	}

	loadAndWait(mw, testSVGPath)
	mw.centerPanel.Pause()

	if mw.centerPanel.wysiwygCheck == nil {
		t.Fatal("expected wysiwygCheck to be non-nil")
	}

	// Initially checked
	if !mw.centerPanel.wysiwygCheck.Checked {
		t.Errorf("expected wysiwygCheck to be checked by default")
	}
	if !mw.centerPanel.wysiwygColors {
		t.Errorf("expected wysiwygColors to be true by default")
	}

	// Capture frame with WYSIWYG
	imgWysiwyg := mw.centerPanel.mainCanvasImage.Image

	// Toggle WYSIWYG off
	mw.centerPanel.wysiwygCheck.SetChecked(false)
	waitForPreview(mw)
	if mw.centerPanel.wysiwygColors {
		t.Errorf("expected wysiwygColors to be false after unchecking")
	}
	imgRaw := mw.centerPanel.mainCanvasImage.Image

	if imgWysiwyg == nil || imgRaw == nil {
		t.Fatal("expected non-nil images for preview")
	}

	// Change palette colors to 32 and re-enable WYSIWYG
	mw.rightPanel.colorsSelect.SetSelected("32")
	waitForPreview(mw)
	if mw.session.ExportOptions.NumColors != 32 {
		t.Errorf("expected 32 colors in session options, got %d", mw.session.ExportOptions.NumColors)
	}

	mw.centerPanel.SetWysiwyg(true)
	waitForPreview(mw)
	if !mw.centerPanel.wysiwygCheck.Checked || !mw.centerPanel.wysiwygColors {
		t.Errorf("expected wysiwyg enabled after SetWysiwyg(true)")
	}

	// Toggle Dithering
	mw.rightPanel.ditherCheck.SetChecked(true)
	waitForPreview(mw)
	if !mw.session.ExportOptions.Dither {
		t.Errorf("expected session Dither to be true")
	}
}

func TestPreviewBackgroundSwitcher(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	mw := NewMainWindow(app)
	defer func() {
		waitForPreview(mw)
		mw.centerPanel.Pause()
	}() // settle async preview builds before the next test
	if mw.centerPanel.bgSelect == nil {
		t.Fatal("expected bgSelect to be non-nil")
	}

	// 1. Initial default state: Checkerboard
	if mw.centerPanel.CurrentBackground() != BgCheckerboard {
		t.Errorf("expected initial background %v, got %v", BgCheckerboard, mw.centerPanel.CurrentBackground())
	}
	if mw.centerPanel.bgSelect.Selected != string(BgCheckerboard) {
		t.Errorf("expected bgSelect to display %s, got %s", BgCheckerboard, mw.centerPanel.bgSelect.Selected)
	}
	if len(mw.centerPanel.bgContainer.Objects) == 0 {
		t.Fatal("expected bgContainer to have background objects")
	}

	// 2. Test cycling through each background option via dropdown
	for _, bgStr := range PreviewBackgrounds {
		bg := PreviewBackground(bgStr)
		mw.centerPanel.bgSelect.SetSelected(bgStr)
		if mw.centerPanel.CurrentBackground() != bg {
			t.Errorf("expected current background %v after selecting %s, got %v", bg, bgStr, mw.centerPanel.CurrentBackground())
		}
		if len(mw.centerPanel.bgContainer.Objects) == 0 {
			t.Errorf("expected bgContainer to not be empty for %s", bgStr)
		}
	}

	// 3. Test programmatic SetBackground helper
	mw.centerPanel.SetBackground(BgTwitchDark)
	if mw.centerPanel.CurrentBackground() != BgTwitchDark {
		t.Errorf("expected current background %v after SetBackground, got %v", BgTwitchDark, mw.centerPanel.CurrentBackground())
	}
	if mw.centerPanel.bgSelect.Selected != string(BgTwitchDark) {
		t.Errorf("expected bgSelect to reflect %s after SetBackground, got %s", BgTwitchDark, mw.centerPanel.bgSelect.Selected)
	}

	// Set invalid background falls back to Checkerboard
	mw.centerPanel.SetBackground("Unknown Option")
	if mw.centerPanel.CurrentBackground() != BgCheckerboard {
		t.Errorf("expected fallback to %v on invalid background, got %v", BgCheckerboard, mw.centerPanel.CurrentBackground())
	}
}

func TestGPUResidentPreviewFrameStack(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	mw := NewMainWindow(app)
	defer func() {
		waitForPreview(mw)
		mw.centerPanel.Pause()
	}()

	testSVGPath, err := filepath.Abs("../../testdata/pendulum.svg")
	if err != nil {
		t.Fatalf("failed to resolve test SVG path: %v", err)
	}

	loadAndWait(mw, testSVGPath)
	mw.centerPanel.Pause()
	waitForPreview(mw)

	p := mw.centerPanel
	p.mu.Lock()
	defer p.mu.Unlock()

	totalFrames := len(p.cachedFrames)
	if totalFrames < 2 {
		t.Fatalf("expected at least 2 frames for pendulum.svg, got %d", totalFrames)
	}

	if len(p.mainCanvasFrames) != totalFrames {
		t.Fatalf("expected %d GPU-resident frame images, got %d", totalFrames, len(p.mainCanvasFrames))
	}

	// 1. Initial state: frame 0 visible, all others hidden
	if !p.mainCanvasFrames[0].Visible() {
		t.Errorf("expected frame 0 to be visible initially")
	}
	for i := 1; i < totalFrames; i++ {
		if p.mainCanvasFrames[i].Visible() {
			t.Errorf("frame %d: expected to be hidden initially", i)
		}
	}
	if p.mainCanvasImage != p.mainCanvasFrames[0] {
		t.Errorf("expected mainCanvasImage to point to frame 0")
	}

	// 2. Step forward to frame 1: frame 0 must hide, frame 1 must show
	p.currentIdx = 1
	p.renderCurrentFrameLocked()

	if p.mainCanvasFrames[0].Visible() {
		t.Errorf("expected frame 0 to be hidden after stepping to frame 1")
	}
	if !p.mainCanvasFrames[1].Visible() {
		t.Errorf("expected frame 1 to be visible after stepping to frame 1")
	}
	if p.mainCanvasImage != p.mainCanvasFrames[1] {
		t.Errorf("expected mainCanvasImage to point to frame 1")
	}

	// 3. Step across all frames in a complete loop
	for step := 2; step < totalFrames; step++ {
		p.currentIdx = step
		p.renderCurrentFrameLocked()
		if !p.mainCanvasFrames[step].Visible() {
			t.Errorf("step %d: expected frame %d to be visible", step, step)
		}
		if p.mainCanvasFrames[step-1].Visible() {
			t.Errorf("step %d: expected previous frame %d to be hidden", step, step-1)
		}
	}

	// 4. Wrap around from last frame to frame 0
	p.currentIdx = 0
	p.renderCurrentFrameLocked()
	if !p.mainCanvasFrames[0].Visible() {
		t.Errorf("expected frame 0 visible after wrap-around")
	}
	if p.mainCanvasFrames[totalFrames-1].Visible() {
		t.Errorf("expected last frame hidden after wrap-around")
	}

	// 5. Toggle crop guides updates underlying image pointers
	p.showCropGuides = false
	if len(p.mainCanvasFrames) == len(p.cachedFrames) {
		for i, cf := range p.cachedFrames {
			p.mainCanvasFrames[i].Image = cf.withoutGuides
			p.mainCanvasFrames[i].Refresh()
		}
	}
	p.renderCurrentFrameLocked()
	if p.mainCanvasFrames[0].Image != p.cachedFrames[0].withoutGuides {
		t.Errorf("expected frame 0 image without guides after toggling")
	}
}

func BenchmarkRenderPlaybackTick(b *testing.B) {
	app := test.NewApp()
	defer app.Quit()

	mw := NewMainWindow(app)
	defer func() {
		waitForPreview(mw)
		mw.centerPanel.Pause()
	}()

	testSVGPath, err := filepath.Abs("../../testdata/pendulum.svg")
	if err != nil {
		b.Fatalf("failed to resolve test SVG path: %v", err)
	}

	loadAndWait(mw, testSVGPath)
	mw.centerPanel.Pause()
	waitForPreview(mw)

	p := mw.centerPanel
	p.mu.Lock()
	defer p.mu.Unlock()

	totalFrames := len(p.cachedFrames)
	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		p.currentIdx = (p.currentIdx + 1) % totalFrames
		p.renderCurrentFrameLocked()
	}
}

func TestDroppedFramesWarning(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	mw := NewMainWindow(app)
	defer func() {
		waitForPreview(mw)
		mw.centerPanel.Pause()
	}()

	testSVGPath, err := filepath.Abs("../../testdata/pendulum.svg")
	if err != nil {
		t.Fatalf("failed to resolve test SVG path: %v", err)
	}

	loadAndWait(mw, testSVGPath)
	mw.centerPanel.Pause()
	waitForPreview(mw)

	p := mw.centerPanel

	// 1. Initial state while paused: warning is not visible
	text, visible := p.FPSWarning()
	if visible {
		t.Errorf("expected warning to be hidden initially, got visible with text %q", text)
	}

	p.mu.Lock()
	totalFrames := len(p.cachedFrames)

	// 2. Normal step within cycle (0 to 1): 0 dropped frames, warning still hidden
	p.recordTimelineStepLocked(0, 1, 1, 1, 0, totalFrames)
	p.mu.Unlock()

	waitForPreview(mw)
	text, visible = p.FPSWarning()
	if visible {
		t.Errorf("expected 0 dropped frames for single step, got visible: %q", text)
	}

	// 3. Jump forward within cycle (1 to 4): skips 2 frames (2 and 3),
	// but warning should remain hidden until the cycle completes!
	p.mu.Lock()
	p.recordTimelineStepLocked(1, 1, 4, 1, 0, totalFrames)
	p.mu.Unlock()

	waitForPreview(mw)
	text, visible = p.FPSWarning()
	if visible {
		t.Errorf("expected warning to remain hidden mid-cycle before cycle completion, got %q", text)
	}

	// 4. Cycle completes (loop wrap from 4 to 0, cycleDiff = 1):
	// Skips remaining frames in cycle and updates warning!
	p.mu.Lock()
	p.recordTimelineStepLocked(4, 1, 0, 1, 1, totalFrames)
	p.mu.Unlock()

	waitForPreview(mw)
	text, visible = p.FPSWarning()
	if !visible {
		t.Fatalf("expected dropped frames warning to be visible after cycle completed")
	}
	if !strings.Contains(text, "frames dropped") {
		t.Errorf("expected text to mention 'frames dropped', got %q", text)
	}

	// 5. Verify Pause does NOT clear the warning (warning persists so user can inspect it)
	p.Pause()
	waitForPreview(mw)
	textAfterPause, visibleAfterPause := p.FPSWarning()
	if !visibleAfterPause || textAfterPause != text {
		t.Errorf("expected warning to persist after pause, got visible=%v text=%q", visibleAfterPause, textAfterPause)
	}

	// 6. Non-looping finalizeRun: test accurate calculation and display
	p.mu.Lock()
	p.finalizeRunLocked(1, 1, 4, 1, totalFrames)
	p.mu.Unlock()

	waitForPreview(mw)
	textRun, visibleRun := p.FPSWarning()
	if !visibleRun || !strings.Contains(textRun, "2 frames dropped") {
		t.Errorf("expected '2 frames dropped' after finalizeRun, got visible=%v text=%q", visibleRun, textRun)
	}

	// 7. Verify resetFPSMonitor clears it
	p.mu.Lock()
	p.resetFPSMonitorLocked()
	p.mu.Unlock()
	waitForPreview(mw)
	_, visibleReset := p.FPSWarning()
	if visibleReset {
		t.Errorf("expected resetFPSMonitorLocked to clear warning")
	}
}

func TestTimelinePlaybackAndFrameSkipping(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	mw := NewMainWindow(app)
	defer func() {
		waitForPreview(mw)
		mw.centerPanel.Pause()
	}()

	testSVGPath, err := filepath.Abs("../../testdata/pendulum.svg")
	if err != nil {
		t.Fatalf("failed to resolve test SVG path: %v", err)
	}

	loadAndWait(mw, testSVGPath)
	mw.centerPanel.Pause()
	waitForPreview(mw)

	p := mw.centerPanel
	p.mu.Lock()
	defer p.mu.Unlock()

	totalFrames := len(p.session.RenderedFrames)
	if totalFrames < 2 {
		t.Fatalf("expected at least 2 frames, got %d", totalFrames)
	}

	// 1. Verify timeline durations in linear mode
	p.pingPong = false
	fwdTotal, cycleTotal := p.timelineDurationsLocked()
	if fwdTotal != cycleTotal {
		t.Errorf("expected fwdTotal == cycleTotal in linear mode, got %v vs %v", fwdTotal, cycleTotal)
	}
	if cycleTotal <= 0 {
		t.Fatalf("expected positive cycleTotal, got %v", cycleTotal)
	}

	// 2. Test timeline offset lookup for frame 0
	off0 := p.timelineOffsetOfFrameLocked(0, 1)
	if off0 != 0 {
		t.Errorf("expected frame 0 offset 0, got %v", off0)
	}

	// 3. Test frame skipping: if elapsed time jumps forward by 2.5 frames of duration
	frameDur := time.Duration(max(p.session.RenderedFrames[0].DurationMs, 10)) * time.Millisecond
	jumpOffset := frameDur*2 + frameDur/2
	idx, dir, remaining := p.frameAtTimelineOffsetLocked(jumpOffset, fwdTotal, cycleTotal)
	if idx != 2 {
		t.Errorf("expected jumped frame index 2, got %d", idx)
	}
	if dir != 1 {
		t.Errorf("expected direction 1, got %d", dir)
	}
	if remaining != frameDur/2 {
		t.Errorf("expected remaining %v, got %v", frameDur/2, remaining)
	}

	// 4. Test Ping-Pong timeline cycle
	if totalFrames >= 3 {
		p.pingPong = true
		fwdPP, cyclePP := p.timelineDurationsLocked()
		expectedCycle := fwdPP
		for i := totalFrames - 2; i >= 1; i-- {
			expectedCycle += time.Duration(max(p.session.RenderedFrames[i].DurationMs, 10)) * time.Millisecond
		}
		if cyclePP != expectedCycle {
			t.Errorf("expected ping pong cycle %v, got %v", expectedCycle, cyclePP)
		}

		// Reverse leg lookup: just past fwdPP
		revIdx, revDir, _ := p.frameAtTimelineOffsetLocked(fwdPP+10*time.Millisecond, fwdPP, cyclePP)
		if revDir != -1 {
			t.Errorf("expected reverse direction -1, got %d", revDir)
		}
		if revIdx != totalFrames-2 {
			t.Errorf("expected first reverse frame %d, got %d", totalFrames-2, revIdx)
		}
	}
}

func TestStudioThemeAndUIHelpers(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	st := &StudioTheme{}
	colorNames := []fyne.ThemeColorName{
		theme.ColorNameBackground,
		theme.ColorNameInputBackground,
		theme.ColorNameButton,
		theme.ColorNamePrimary,
		theme.ColorNameHeaderBackground,
		theme.ColorNameSelection,
		theme.ColorNameHover,
		theme.ColorNameForeground,
		theme.ColorNameDisabled,
		theme.ColorNameHyperlink,
		theme.ColorNameOverlayBackground,
		theme.ColorNamePlaceHolder,
		theme.ColorNameScrollBar,
		theme.ColorNameError,
		theme.ColorNameWarning,
		theme.ColorNameSuccess,
		"custom_fallback_color",
	}
	for _, cn := range colorNames {
		if c := st.Color(cn, theme.VariantDark); c == nil {
			t.Errorf("expected non-nil color for %s", cn)
		}
	}
	if f := st.Font(fyne.TextStyle{}); f == nil {
		t.Errorf("expected non-nil font")
	}
	if ic := st.Icon(theme.IconNameConfirm); ic == nil {
		t.Errorf("expected non-nil icon")
	}
	if sz := st.Size(theme.SizeNameSeparatorThickness); sz != 3.0 {
		t.Errorf("expected separator thickness 3.0, got %v", sz)
	}
	if sz := st.Size(theme.SizeNameText); sz <= 0 {
		t.Errorf("expected positive text size, got %v", sz)
	}

	// Test background image generators
	if cb := createCheckerboardImage(32, 32); cb == nil || cb.Bounds().Dx() != 32 {
		t.Errorf("expected 32x32 checkerboard image")
	}
	if vg := createVibrantGradientImage(32, 32); vg == nil || vg.Bounds().Dx() != 32 {
		t.Errorf("expected 32x32 vibrant gradient image")
	}

	// Test commitEntry FocusLost
	committed := false
	ce := newCommitEntry(func() {
		committed = true
	})
	ce.SetText("120")
	ce.FocusLost()
	if !committed {
		t.Errorf("expected commitEntry.FocusLost() to trigger onCommit")
	}
}

func TestMainWindowSaveReloadAndCloseLifecycle(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	mw := NewMainWindow(app)
	defer func() {
		waitForPreview(mw)
		mw.centerPanel.Pause()
	}()

	// Test RightExportPanel helpers and empty-frame PromptExport dialog
	mw.rightPanel.SetOnExportSuccess(func(path string) {})
	mw.rightPanel.Refresh()
	mw.rightPanel.PromptExport()

	// Test CenterPreviewPanel SetPingPong and BeginLoading/EndLoading
	mw.centerPanel.SetPingPong(true)
	mw.centerPanel.BeginLoading("Testing...")
	mw.centerPanel.EndLoading()

	// Copy pendulum.svg to a temp file so we can test Save(), Reload(), MarkClean(), and handleCloseRequest()
	srcPath, err := filepath.Abs("../../testdata/pendulum.svg")
	if err != nil {
		t.Fatalf("failed to resolve pendulum.svg: %v", err)
	}
	rawBytes, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("failed to read pendulum.svg: %v", err)
	}
	tmpSVG := filepath.Join(t.TempDir(), "pendulum_copy.svg")
	if err := os.WriteFile(tmpSVG, rawBytes, 0600); err != nil {
		t.Fatalf("failed to write temp SVG: %v", err)
	}

	loadAndWait(mw, tmpSVG)
	mw.StopPreviewTimer()

	// Mutate state and test Save() & MarkClean()
	if mw.editorState == nil {
		t.Fatalf("expected non-nil editorState")
	}
	mw.editorState.RecordChange("Test mutation")
	mw.Save()
	if mw.editorState.IsDirty() {
		t.Errorf("expected editorState to be clean after Save()")
	}

	// Test Reload()
	mw.Reload()
	waitForPreview(mw)

	// Test handleCloseRequest when clean
	mw.handleCloseRequest()
}

func TestSidecarControlFileUI(t *testing.T) {
	testApp := test.NewApp()
	defer testApp.Quit()

	srcPath, err := filepath.Abs("../../testdata/pendulum.svg")
	if err != nil {
		t.Fatalf("failed to resolve pendulum.svg: %v", err)
	}
	rawBytes, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("failed to read pendulum.svg: %v", err)
	}
	tmpDir := t.TempDir()
	tmpSVG := filepath.Join(tmpDir, "emote.svg")
	if err := os.WriteFile(tmpSVG, rawBytes, 0600); err != nil {
		t.Fatalf("failed to write temp SVG: %v", err)
	}

	// 1. Open SVG, change timing/export settings, and save
	mw1 := NewMainWindow(testApp)
	loadAndWait(mw1, tmpSVG)
	mw1.centerPanel.Pause()

	mw1.centerPanel.fpsSelect.SetSelected("20 FPS (50ms)")
	waitForPreview(mw1)
	mw1.centerPanel.durationInput.Entry.SetText("1.5")
	mw1.centerPanel.durationInput.Entry.OnSubmitted("1.5")
	waitForPreview(mw1)
	mw1.centerPanel.pingPongCheck.SetChecked(true)
	mw1.rightPanel.presetSelect.SetSelected("Discord Emote (128x128 Square)")
	mw1.rightPanel.colorsSelect.SetSelected("64")
	mw1.rightPanel.ditherCheck.SetChecked(true)
	waitForPreview(mw1)

	mw1.Save()
	mw1.Close()

	inkaPath := filepath.Join(tmpDir, "emote.inka")
	if _, statErr := os.Stat(inkaPath); statErr != nil {
		t.Fatalf("expected sidecar %s to be created on Save(): %v", inkaPath, statErr)
	}

	// 2. Open the .inka file directly in a new window and verify all UI controls restore
	mw2 := NewMainWindow(testApp)
	defer func() {
		waitForPreview(mw2)
		mw2.centerPanel.Pause()
	}()
	loadAndWait(mw2, inkaPath)
	mw2.centerPanel.Pause()

	if mw2.currentPath != tmpSVG {
		t.Errorf("expected currentPath to resolve to %q, got %q", tmpSVG, mw2.currentPath)
	}
	if len(mw2.session.RenderedFrames)*2-2 != 30 {
		t.Errorf("expected 30 restored bounced frames (1.5s @ 20 FPS), got %d (rendered=%d)", len(mw2.session.RenderedFrames)*2-2, len(mw2.session.RenderedFrames))
	}
	if mw2.centerPanel.fpsSelect.Selected != "20 FPS (50ms)" {
		t.Errorf("expected fpsSelect '20 FPS (50ms)', got %q", mw2.centerPanel.fpsSelect.Selected)
	}
	if mw2.centerPanel.durationInput.Entry.Text != "1.5" {
		t.Errorf("expected durationInput '1.5', got %q", mw2.centerPanel.durationInput.Entry.Text)
	}
	if !mw2.centerPanel.pingPongCheck.Checked {
		t.Errorf("expected pingPongCheck to be restored as checked")
	}
	if mw2.rightPanel.presetSelect.Selected != "Discord Emote (128x128 Square)" {
		t.Errorf("expected presetSelect 'Discord Emote (128x128 Square)', got %q", mw2.rightPanel.presetSelect.Selected)
	}
	if mw2.rightPanel.colorsSelect.Selected != "64" {
		t.Errorf("expected colorsSelect '64', got %q", mw2.rightPanel.colorsSelect.Selected)
	}
	if !mw2.rightPanel.ditherCheck.Checked {
		t.Errorf("expected ditherCheck to be restored as checked")
	}
}

func TestProjectWorkflowAndLinkedSVGTree(t *testing.T) {
	testApp := test.NewApp()
	defer testApp.Quit()

	mw := NewMainWindow(testApp)
	defer func() {
		waitForPreview(mw)
		mw.centerPanel.Pause()
	}()

	// 1. Verify blank Untitled Project startup state and project-centric File menu
	if mw.fileLabel.Text != "Untitled Project" {
		t.Errorf("expected initial fileLabel 'Untitled Project', got %q", mw.fileLabel.Text)
	}
	if !strings.Contains(mw.statusLabel.Text, "New project ready") {
		t.Errorf("expected initial statusLabel to mention 'New project ready', got %q", mw.statusLabel.Text)
	}

	var fileMenu *fyne.Menu
	for _, m := range mw.mainMenu.Items {
		if m.Label == "File" {
			fileMenu = m
			break
		}
	}
	if fileMenu == nil {
		t.Fatalf("File menu not found")
	}
	var menuLabels []string
	for _, item := range fileMenu.Items {
		if !item.IsSeparator {
			menuLabels = append(menuLabels, item.Label)
		}
	}
	expectedLabels := []string{
		"New Project",
		"Open Project...",
		"Save Project",
		"Save Project As...",
		"Add / Import SVG...",
		"Reload Linked SVGs",
		"Export GIF...",
	}
	if len(menuLabels) != len(expectedLabels) {
		t.Fatalf("expected File menu items %v, got %v", expectedLabels, menuLabels)
	}
	for i, want := range expectedLabels {
		if menuLabels[i] != want {
			t.Errorf("File menu item %d: got %q, want %q", i, menuLabels[i], want)
		}
	}

	// 2. Create two SVGs in an external directory and link them into the Untitled Project
	extDir := t.TempDir()
	heroSVG := `<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape" viewBox="0 0 128 128" width="128" height="128">
  <g id="layer_hero" inkscape:groupmode="layer" inkscape:label="Hero">
    <g id="g_arm" inkscape:label="LeftArm">
      <rect id="arm_rect" x="32" y="32" width="32" height="16" fill="#38bdf8"/>
      <path id="arm_rot" inkscape:label="Rot {f: 1-10; deg: 45}" d="M 32,32 L 64,32"/>
    </g>
  </g>
</svg>`
	propsSVG := `<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape" viewBox="0 0 128 128" width="128" height="128">
  <g id="layer_props" inkscape:groupmode="layer" inkscape:label="Props">
    <g id="g_star" inkscape:label="Star">
      <circle id="star_circle" cx="96" cy="32" r="12" fill="#facc15"/>
    </g>
  </g>
</svg>`
	heroPath := filepath.Join(extDir, "Hero.svg")
	propsPath := filepath.Join(extDir, "Props.svg")
	if err := os.WriteFile(heroPath, []byte(heroSVG), 0600); err != nil {
		t.Fatalf("write Hero.svg: %v", err)
	}
	if err := os.WriteFile(propsPath, []byte(propsSVG), 0600); err != nil {
		t.Fatalf("write Props.svg: %v", err)
	}

	hasIAMS, err := mw.AddSVGFileWithOptions(heroPath, false)
	if err != nil {
		t.Fatalf("AddSVGFileWithOptions(Hero.svg) failed: %v", err)
	}
	waitForPreview(mw)
	mw.centerPanel.Pause()
	if !hasIAMS {
		t.Errorf("expected Hero.svg to report hasIAMS == true")
	}

	// Strip legacy IAMS from Hero.svg in-project and verify animations remain driven by .inka Objects
	if err := mw.StripIAMSFromLinkedSVG("src_1"); err != nil {
		t.Fatalf("StripIAMSFromLinkedSVG(src_1) failed: %v", err)
	}
	waitForPreview(mw)
	mw.centerPanel.Pause()

	// Add second linked SVG (Props.svg)
	if _, err := mw.AddSVGFileWithOptions(propsPath, false); err != nil {
		t.Fatalf("AddSVGFileWithOptions(Props.svg) failed: %v", err)
	}
	waitForPreview(mw)
	mw.centerPanel.Pause()

	// 3. Verify tree shows Linked File roots -> Layers -> Nested Groups -> Drawable Leaf Shapes,
	// while stripping/hiding legacy IAMS motion helper paths.
	roots := mw.tree.ChildUIDs("")
	if len(roots) != 2 {
		t.Fatalf("expected 2 linked SVG root groups in tree, got %v", roots)
	}
	for _, parentUID := range []string{"", "src_1", "src_2", "src_1__layer_hero", "src_1__g_arm", "src_2__layer_props", "src_2__g_star"} {
		for _, child := range mw.tree.ChildUIDs(parentUID) {
			if child == "src_1__arm_rot" {
				t.Errorf("expected motion helper path %q to be hidden from tree under %q", child, parentUID)
			}
		}
	}
	if children := mw.tree.ChildUIDs("src_1__g_arm"); len(children) != 1 || children[0] != "src_1__arm_rect" {
		t.Errorf("expected drawable leaf shape src_1__arm_rect under src_1__g_arm, got %v", children)
	}
	if children := mw.tree.ChildUIDs("src_2__g_star"); len(children) != 1 || children[0] != "src_2__star_circle" {
		t.Errorf("expected drawable leaf shape src_2__star_circle under src_2__g_star, got %v", children)
	}

	// Select nested group src_2__g_star and verify Stage Properties card is rendered in Inspector
	mw.tree.Select("src_2__g_star")
	if mw.editorState.ActiveNode == nil || mw.editorState.ActiveNode.ID != "src_2__g_star" {
		t.Fatalf("expected active node src_2__g_star, got %+v", mw.editorState.ActiveNode)
	}

	// 4. Save project to a new directory and verify both linked SVGs are co-located alongside the .inka file
	projDir := t.TempDir()
	inkaPath := filepath.Join(projDir, "my_scene.inka")
	if err := mw.saveProjectToPath(inkaPath); err != nil {
		t.Fatalf("saveProjectToPath failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(projDir, "Hero.svg")); err != nil {
		t.Errorf("expected Hero.svg to be co-located in project dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(projDir, "Props.svg")); err != nil {
		t.Errorf("expected Props.svg to be co-located in project dir: %v", err)
	}

	// 5. Call NewProject() and verify reset to blank state, then re-open my_scene.inka
	mw.NewProject()
	if mw.session.Document != nil || len(mw.session.LinkedSources) != 0 {
		t.Errorf("expected NewProject() to clear session Document and LinkedSources")
	}
	if len(mw.centerPanel.cachedFrames) != 0 || len(mw.centerPanel.mainCanvasFrames) != 0 {
		t.Errorf("expected NewProject() to clear centerPanel cachedFrames and mainCanvasFrames")
	}
	if mw.centerPanel.mainCanvasImage == nil || mw.centerPanel.mainCanvasImage.Image == nil || mw.centerPanel.mainCanvasImage.Image.Bounds().Dx() != 1 {
		t.Errorf("expected NewProject() to reset mainCanvasImage to blank 1x1 image")
	}
	if mw.fileLabel.Text != "Untitled Project" {
		t.Errorf("expected fileLabel 'Untitled Project' after NewProject(), got %q", mw.fileLabel.Text)
	}

	loadAndWait(mw, inkaPath)
	mw.centerPanel.Pause()
	if len(mw.session.LinkedSources) != 2 {
		t.Errorf("expected 2 linked sources restored from my_scene.inka, got %d", len(mw.session.LinkedSources))
	}
	if len(mw.session.RenderedFrames) < 2 {
		t.Errorf("expected animated frames restored from my_scene.inka, got %d", len(mw.session.RenderedFrames))
	}
}

func TestTimelinePanelRulerScrubAndActionBars(t *testing.T) {
	testApp := test.NewApp()
	defer testApp.Quit()

	mw := NewMainWindow(testApp)
	defer func() {
		waitForPreview(mw)
		mw.centerPanel.Pause()
	}()

	if mw.timelinePanel == nil {
		t.Fatalf("expected mw.timelinePanel to be initialized")
	}

	testSVGPath, err := filepath.Abs("../../testdata/pendulum.svg")
	if err != nil {
		t.Fatalf("failed to resolve test SVG path: %v", err)
	}
	loadAndWait(mw, testSVGPath)
	mw.centerPanel.Pause()
	waitForPreview(mw)

	tp := mw.timelinePanel

	// 1. Verify hover timestamp & frame readout on ruler
	tp.UpdateHover(200, 400)
	if !strings.Contains(tp.HoverLabel.Text, "0.50s") || !strings.Contains(tp.HoverLabel.Text, "/ 10") {
		t.Errorf("expected HoverLabel to show frame & 0.50s timestamp, got %q", tp.HoverLabel.Text)
	}
	rulerImg := tp.ruler.draw(400, 30)
	if rulerImg.Bounds().Dx() != 400 || rulerImg.Bounds().Dy() != 30 {
		t.Errorf("unexpected ruler image bounds: %v", rulerImg.Bounds())
	}
	tp.ClearHover()
	if tp.HoverLabel.Text != "" {
		t.Errorf("expected HoverLabel cleared on ClearHover(), got %q", tp.HoverLabel.Text)
	}

	// 2. Verify scrubbing via ScrubAtX updates playhead frame & timestamp readout
	tp.ScrubAtX(280, 400) // 70% of 1.0s (10 frames) -> frame index 7 (Frame 08 / 10 • 0.70s)
	if mw.centerPanel.CurrentFrameIndex() != 7 {
		t.Errorf("expected ScrubAtX(280, 400) to jump to frame index 7, got %d", mw.centerPanel.CurrentFrameIndex())
	}
	if !strings.Contains(tp.PlayheadLabel.Text, "0.70s") {
		t.Errorf("expected PlayheadLabel to contain '0.70s', got %q", tp.PlayheadLabel.Text)
	}
	if !strings.Contains(mw.centerPanel.frameLabel.Text, "0.70s") {
		t.Errorf("expected centerPanel.frameLabel to contain '0.70s', got %q", mw.centerPanel.frameLabel.Text)
	}

	// 3. Select a group node, add a .inka time-based action, and test dragging/resizing its action bar
	tracks := tp.CollectTracks()
	if len(tracks) == 0 {
		t.Fatalf("expected non-empty timeline tracks after loading pendulum.svg")
	}
	targetNodeID := tracks[0].NodeID
	mw.tree.Select(targetNodeID)
	tp.AddActionToActiveNode("rot")
	waitForPreview(mw)

	if len(tp.lanes) == 0 {
		t.Fatalf("expected timeline track lanes to be built")
	}
	lane := tp.lanes[0]
	lane.Resize(fyne.NewSize(400, timelineTrackRowHeight))
	if len(lane.track.Config.Actions) == 0 {
		t.Fatalf("expected action to be added to track %q", lane.track.NodeID)
	}

	// Resize right edge of the action from 1.0s (x=400) to 0.6s (x=240)
	lane.Dragged(&fyne.DragEvent{
		PointEvent: fyne.PointEvent{Position: fyne.NewPos(240, 16)},
		Dragged:    fyne.Delta{DX: -158, DY: 0}, // start at x=398 (right edge grip)
	})
	lane.DragEnd()
	waitForPreview(mw)

	_, updatedCfg, ok := mw.resolveNodeObjectConfig(mw.editorState.NodeMap[targetNodeID])
	if !ok || len(updatedCfg.Actions) == 0 {
		t.Fatalf("expected updated ObjectConfig with actions in session.Objects")
	}
	if updatedCfg.Actions[len(updatedCfg.Actions)-1].EndSec != 0.6 {
		t.Errorf("expected resized action EndSec == 0.6, got %g", updatedCfg.Actions[len(updatedCfg.Actions)-1].EndSec)
	}

	// Drag body of the action (0.0s..0.6s) right by +0.2s (+80px out of 400px) -> (0.2s..0.8s)
	lane = tp.lanes[0]
	lane.Resize(fyne.NewSize(400, timelineTrackRowHeight))
	lane.Dragged(&fyne.DragEvent{
		PointEvent: fyne.PointEvent{Position: fyne.NewPos(200, 16)},
		Dragged:    fyne.Delta{DX: 80, DY: 0}, // start at x=120 (middle of 0..240 bar)
	})
	lane.DragEnd()
	waitForPreview(mw)

	_, movedCfg, _ := mw.resolveNodeObjectConfig(mw.editorState.NodeMap[targetNodeID])
	lastAct := movedCfg.Actions[len(movedCfg.Actions)-1]
	if lastAct.StartSec != 0.2 || lastAct.EndSec != 0.8 {
		t.Errorf("expected moved action range 0.2s..0.8s, got %gs..%gs", lastAct.StartSec, lastAct.EndSec)
	}

	// Render track lane image to ensure no panics
	laneImg := lane.draw(400, 34)
	if laneImg.Bounds().Dx() != 400 || laneImg.Bounds().Dy() != 34 {
		t.Errorf("unexpected lane image bounds: %v", laneImg.Bounds())
	}

	// 4. Add a second action ("scale") onto the same Stage Object and verify vertical sub-row stacking + idempotent migration
	tp.AddActionToActiveNode("scale")
	waitForPreview(mw)
	lane = tp.lanes[0]
	if len(lane.track.Config.Actions) < 2 {
		t.Fatalf("expected at least 2 actions on track %q, got %d", lane.track.NodeID, len(lane.track.Config.Actions))
	}
	minSz := lane.MinSize()
	if minSz.Height <= timelineTrackRowHeight {
		t.Errorf("expected multi-action track MinSize().Height > %g, got %g", timelineTrackRowHeight, minSz.Height)
	}
	lane.Resize(fyne.NewSize(400, minSz.Height))
	hitTop, _ := lane.hitTestAction(160, 8, 400, minSz.Height)
	hitBottom, _ := lane.hitTestAction(160, minSz.Height-8, 400, minSz.Height)
	if hitTop == hitBottom || hitTop < 0 || hitBottom < 0 {
		t.Errorf("expected distinct vertical hit-test indices for stacked actions, got top=%d bottom=%d", hitTop, hitBottom)
	}

	// Verify MigrateDocumentIAMSToControlFile is idempotent and does not duplicate actions
	countBefore := len(lane.track.Config.Actions)
	tmpCtrl := &appkg.ControlFile{Objects: mw.session.Objects}
	added := appkg.MigrateDocumentIAMSToControlFile(mw.session.Document, "", tp.TotalDurationSec(), tmpCtrl)
	if added != 0 {
		t.Errorf("expected 0 duplicate actions migrated on repeat call, got %d", added)
	}
	_, afterCfg, _ := mw.resolveNodeObjectConfig(mw.editorState.NodeMap[targetNodeID])
	if len(afterCfg.Actions) != countBefore {
		t.Errorf("expected action count to remain %d after repeat migration, got %d", countBefore, len(afterCfg.Actions))
	}

	// 5. Verify collapsible parent track rows, layer/group renaming, preset actions, and double-click modals
	tracksInitial := tp.CollectTracks()
	var parentID string
	for _, tr := range tracksInitial {
		if tr.HasChildren {
			parentID = tr.NodeID
			break
		}
	}
	if parentID != "" {
		tp.collapsedNodes[parentID] = false
		tp.Refresh()
		tracksExpanded := tp.CollectTracks()
		tp.collapsedNodes[parentID] = true
		tp.Refresh()
		tracksCollapsed := tp.CollectTracks()
		if len(tracksCollapsed) >= len(tracksExpanded) {
			t.Errorf("expected collapsing parent %q to hide child tracks: expanded=%d, collapsed=%d", parentID, len(tracksExpanded), len(tracksCollapsed))
		}
		tp.collapsedNodes[parentID] = false
		tp.Refresh()
	}

	// Test RenameTrackNode on a non-linked-SVG layer/group track
	var renameTargetID string
	for _, tr := range tp.CollectTracks() {
		if n := mw.editorState.NodeMap[tr.NodeID]; n != nil && !n.IsLinkedFile {
			renameTargetID = tr.NodeID
			break
		}
	}
	if renameTargetID != "" {
		if err := mw.RenameTrackNode(renameTargetID, "HeroPendulum"); err != nil {
			t.Fatalf("RenameTrackNode(%q) failed: %v", renameTargetID, err)
		}
		if mw.editorState.NodeMap[renameTargetID].DisplayTitle() != "HeroPendulum" {
			t.Errorf("expected renamed node DisplayTitle 'HeroPendulum', got %q", mw.editorState.NodeMap[renameTargetID].DisplayTitle())
		}
		waitForPreview(mw)
	}

	// Test adding a Preset action ("Preset: Pulse") and opening Track/Action modals via DoubleTapped
	tp.SelectTrack(targetNodeID)
	tp.AddActionToActiveNode("Preset: Pulse")
	waitForPreview(mw)
	if len(tp.lanes) > 0 {
		tp.lanes[0].DoubleTapped(&fyne.PointEvent{Position: fyne.NewPos(160, 10)})
	}
	tp.ShowTrackPropertiesModal(targetNodeID)
	tp.ShowActionEditModal(targetNodeID, 0)
}

func TestTimelinePingPongDurationReorderAndUndoRedo(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	mw := NewMainWindow(app)
	defer func() {
		waitForPreview(mw)
		mw.centerPanel.Pause()
	}()

	// 1. Verify default 512x512 preview scaling before and after NewProject()
	if mw.session.ExportOptions.SquareSize != 512 {
		t.Errorf("expected initial SquareSize == 512, got %d", mw.session.ExportOptions.SquareSize)
	}
	mw.NewProject()
	if mw.session.ExportOptions.SquareSize != 512 {
		t.Errorf("expected SquareSize == 512 after NewProject(), got %d", mw.session.ExportOptions.SquareSize)
	}

	testSVGPath, err := filepath.Abs("../../testdata/pendulum.svg")
	if err != nil {
		t.Fatalf("failed to resolve test SVG path: %v", err)
	}
	loadAndWait(mw, testSVGPath)
	mw.centerPanel.Pause()
	tp := mw.timelinePanel

	// 2. Verify Ping-Pong keeps the same total duration and target cycle frame count
	durBeforeMs := mw.session.TotalDurationMs
	cycleBefore := tp.FrameCount()
	mw.centerPanel.pingPongCheck.SetChecked(true)
	waitForPreview(mw)
	if mw.session.TotalDurationMs != durBeforeMs {
		t.Errorf("expected Ping-Pong to preserve TotalDurationMs (%d), got %d", durBeforeMs, mw.session.TotalDurationMs)
	}
	if tp.FrameCount() != cycleBefore {
		t.Errorf("expected Ping-Pong to preserve total cycle frames (%d), got %d", cycleBefore, tp.FrameCount())
	}
	mw.centerPanel.pingPongCheck.SetChecked(false)
	waitForPreview(mw)

	// 3. Test Studio Undo / Redo across timeline action addition
	tracks := tp.CollectTracks()
	if len(tracks) == 0 {
		t.Fatalf("expected non-empty tracks")
	}
	targetID := tracks[0].NodeID
	tp.SelectTrack(targetID)
	_, cfgInit, _ := mw.resolveNodeObjectConfig(mw.editorState.NodeMap[targetID])
	initActs := len(cfgInit.Actions)

	tp.AddActionToActiveNode("fade")
	waitForPreview(mw)
	_, cfgAfterAdd, _ := mw.resolveNodeObjectConfig(mw.editorState.NodeMap[targetID])
	if len(cfgAfterAdd.Actions) != initActs+1 {
		t.Fatalf("expected %d actions after AddActionToActiveNode, got %d", initActs+1, len(cfgAfterAdd.Actions))
	}

	// Undo action addition
	if !mw.editorState.CanUndo() {
		t.Fatal("expected CanUndo() == true after adding timeline action")
	}
	_, _ = mw.editorState.Undo()
	mw.rebuildAfterHistoryChange()
	waitForPreview(mw)
	_, cfgAfterUndo, _ := mw.resolveNodeObjectConfig(mw.editorState.NodeMap[targetID])
	if len(cfgAfterUndo.Actions) != initActs {
		t.Errorf("expected Undo to restore %d actions, got %d", initActs, len(cfgAfterUndo.Actions))
	}

	// Redo action addition
	_, _ = mw.editorState.Redo()
	mw.rebuildAfterHistoryChange()
	waitForPreview(mw)
	_, cfgAfterRedo, _ := mw.resolveNodeObjectConfig(mw.editorState.NodeMap[targetID])
	if len(cfgAfterRedo.Actions) != initActs+1 {
		t.Errorf("expected Redo to restore %d actions, got %d", initActs+1, len(cfgAfterRedo.Actions))
	}

	// 4. Test MoveAction reordering within a track
	if len(cfgAfterRedo.Actions) >= 2 {
		firstID := cfgAfterRedo.Actions[0].ID
		secondID := cfgAfterRedo.Actions[1].ID
		if !tp.MoveAction(targetID, 0, 1) {
			t.Errorf("expected MoveAction(0, +1) to succeed")
		}
		waitForPreview(mw)
		_, cfgReordered, _ := mw.resolveNodeObjectConfig(mw.editorState.NodeMap[targetID])
		if cfgReordered.Actions[0].ID != secondID || cfgReordered.Actions[1].ID != firstID {
			t.Errorf("expected actions to swap order: got [%s, %s], want [%s, %s]",
				cfgReordered.Actions[0].ID, cfgReordered.Actions[1].ID, secondID, firstID)
		}
	}

	// 5. Test ApplyDurationChange modes (Scale, PadEnd, PadStart)
	baseObjects := cloneObjectsMap(mw.session.Objects)
	oldSec := tp.TotalDurationSec()
	newSec := oldSec * 2.0

	mw.ApplyDurationChangeWithObjects(oldSec, newSec, baseObjects, DurationAdjustPadEnd, false)
	_, cfgPadEnd, _ := mw.resolveNodeObjectConfig(mw.editorState.NodeMap[targetID])
	if len(cfgPadEnd.Actions) > 0 && cfgPadEnd.Actions[0].EndSec != oldSec {
		t.Errorf("expected DurationAdjustPadEnd to keep EndSec=%g, got %g", oldSec, cfgPadEnd.Actions[0].EndSec)
	}

	mw.ApplyDurationChangeWithObjects(oldSec, newSec, baseObjects, DurationAdjustPadStart, false)
	_, cfgPadStart, _ := mw.resolveNodeObjectConfig(mw.editorState.NodeMap[targetID])
	if len(cfgPadStart.Actions) > 0 && cfgPadStart.Actions[0].StartSec != (newSec-oldSec) {
		t.Errorf("expected DurationAdjustPadStart to shift StartSec by %g, got %g", newSec-oldSec, cfgPadStart.Actions[0].StartSec)
	}

	mw.ApplyDurationChangeWithObjects(oldSec, newSec, baseObjects, DurationAdjustScale, true)
	_, cfgScaled, _ := mw.resolveNodeObjectConfig(mw.editorState.NodeMap[targetID])
	if len(cfgScaled.Actions) > 0 && cfgScaled.Actions[0].EndSec != newSec {
		t.Errorf("expected DurationAdjustScale to scale EndSec to %g, got %g", newSec, cfgScaled.Actions[0].EndSec)
	}

	// 6. Test Track Reordering (MoveTrackNode) when a second SVG is linked
	secondSVGPath, err := filepath.Abs("../../testdata/bouncing_walker.svg")
	if err == nil {
		_ = mw.AddSVGFile(secondSVGPath)
		waitForPreview(mw)
		mw.centerPanel.Pause()
		rootsBefore := mw.editorState.Roots
		if len(rootsBefore) >= 2 {
			firstRootID := rootsBefore[0].ID
			secondRootID := rootsBefore[1].ID
			canUp, canDown := mw.CanMoveTrackNode(firstRootID)
			if canUp || !canDown {
				t.Errorf("expected first root CanMoveTrackNode == (false, true), got (%v, %v)", canUp, canDown)
			}
			if err := mw.MoveTrackNode(firstRootID, 1); err != nil {
				t.Fatalf("MoveTrackNode(%q, +1) failed: %v", firstRootID, err)
			}
			waitForPreview(mw)
			if mw.editorState.Roots[0].ID != secondRootID || mw.editorState.Roots[1].ID != firstRootID {
				t.Errorf("expected roots to swap order after MoveTrackNode")
			}
		}
	}

	// 7. Verify importing a 30-frame (3.0s) SVG with stripIAMS=true preserves 3.0s duration and keeps right-edge grips reachable
	mw.NewProject()
	svg30 := []byte(`<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape" viewBox="0 0 512 512">
  <g id="layer1" inkscape:groupmode="layer" inkscape:label="Layer 1">
    <path id="p1" d="M 10 10 L 100 10" inkscape:label="Move {f: all; r: 5}"/>
    <circle id="c1" cx="50" cy="50" r="2" inkscape:label="Shake: Rot {f: 1-30; deg: 8; pingpong; r: 6}"/>
  </g>
</svg>`)
	if _, err := mw.AddSVGDataWithOptions(svg30, "GloriousTest.svg", true); err != nil {
		t.Fatalf("AddSVGDataWithOptions(stripIAMS=true) failed: %v", err)
	}
	waitForPreview(mw)
	mw.centerPanel.Pause()
	if tp.TotalDurationSec() != 3.0 {
		t.Errorf("expected TotalDurationSec() == 3.0 after importing 30-frame SVG with stripIAMS=true, got %g", tp.TotalDurationSec())
	}
	tp.UpdateHover(400, 800)
	if tp.Container().MinSize().Width > 800 {
		t.Errorf("expected compact timeline MinSize().Width (<= 800) during hover, got %g", tp.Container().MinSize().Width)
	}
	for _, lane := range tp.lanes {
		if len(lane.track.Config.Actions) > 0 {
			idx, mode := lane.hitTestAction(798, 10, 800, 32)
			if idx < 0 || mode != dragActionResizeEnd {
				t.Errorf("expected right edge hitTestAction to return dragActionResizeEnd, got idx=%d mode=%v", idx, mode)
			}
		}
	}

	// 8. Verify deleting an action removes it from Document.MotionPaths (and Paths guide overlay)
	// even when legacy IAMS was NOT stripped from the source SVG XML (stripIAMS=false).
	mw.NewProject()
	if _, err := mw.AddSVGDataWithOptions(svg30, "GloriousUnstripped.svg", false); err != nil {
		t.Fatalf("AddSVGDataWithOptions(stripIAMS=false) failed: %v", err)
	}
	waitForPreview(mw)
	mw.centerPanel.Pause()
	initialMPCount := len(mw.session.Document.MotionPaths)
	if initialMPCount != 2 {
		t.Fatalf("expected 2 initial MotionPaths, got %d", initialMPCount)
	}
	var targetActID string
	for _, cfg := range mw.session.Objects {
		for _, act := range cfg.Actions {
			if act.Type == "move" && act.PathD != "" {
				targetActID = act.ID
				break
			}
		}
		if targetActID != "" {
			break
		}
	}
	if targetActID == "" {
		t.Fatalf("expected to find a move action to delete")
	}
	mw.selectedActionID = targetActID
	if !tp.DeleteSelectedAction() {
		t.Fatalf("expected DeleteSelectedAction to return true")
	}
	patched := mw.editorState.ComputePatchedSVG()
	if err := mw.session.LoadSVGData(patched, mw.currentFilename); err != nil {
		t.Fatalf("LoadSVGData after DeleteSelectedAction failed: %v", err)
	}
	if len(mw.session.Document.MotionPaths) != initialMPCount-1 {
		t.Errorf("expected %d MotionPaths after deleting action, got %d", initialMPCount-1, len(mw.session.Document.MotionPaths))
	}
	for _, mp := range mw.session.Document.MotionPaths {
		if mp.ID == targetActID {
			t.Errorf("expected deleted action %q to be absent from Document.MotionPaths", targetActID)
		}
	}
}

func TestCanvasGizmoOverlayPathAndStagePlacement(t *testing.T) {
	fyneApp := test.NewApp()
	defer fyneApp.Quit()

	mw := NewMainWindow(fyneApp)
	defer func() {
		waitForPreview(mw)
		mw.centerPanel.Pause()
	}()

	svgPath, err := filepath.Abs("../../testdata/pendulum.svg")
	if err != nil {
		t.Fatalf("failed to resolve pendulum.svg: %v", err)
	}
	if err := mw.AddSVGFile(svgPath); err != nil {
		t.Fatalf("AddSVGFile failed: %v", err)
	}
	waitForPreview(mw)
	mw.centerPanel.Pause()

	// 1. Verify ParseGizmoPath and FormatGizmoPath round-trip for lines and cubic curves
	segs := ParseGizmoPath("M 10 20 L 110 20 C 120 40 150 60 200 80")
	if len(segs) != 2 {
		t.Fatalf("expected 2 gizmo segments, got %d", len(segs))
	}
	if segs[0].IsCurve || segs[0].StartX != 10 || segs[0].EndX != 110 {
		t.Errorf("unexpected first segment: %+v", segs[0])
	}
	if !segs[1].IsCurve || segs[1].Ctrl1X != 120 || segs[1].EndY != 80 {
		t.Errorf("unexpected second segment: %+v", segs[1])
	}
	formatted := FormatGizmoPath(segs)
	if !strings.Contains(formatted, "M 10.0 20.0 L 110.0 20.0 C 120.0 40.0 150.0 60.0 200.0 80.0") {
		t.Errorf("unexpected formatted path: %s", formatted)
	}

	// 2. Select a layer/group track and add a Move action
	tracks := mw.timelinePanel.CollectTracks()
	var targetNodeID string
	for _, tr := range tracks {
		if n := mw.editorState.NodeMap[tr.NodeID]; n != nil && !n.IsLinkedFile {
			targetNodeID = tr.NodeID
			break
		}
	}
	if targetNodeID == "" {
		t.Fatalf("expected a non-linked track node")
	}
	mw.timelinePanel.SelectTrack(targetNodeID)
	mw.timelinePanel.AddActionToActiveNode("Move")

	gizmo := mw.centerPanel.gizmoOverlay
	if gizmo == nil {
		t.Fatalf("expected CenterPreviewPanel to have a gizmoOverlay")
	}
	gizmo.Resize(fyne.NewSize(400, 400))

	svgToView, _, ok := gizmo.viewportTransform(400, 400)
	if !ok {
		t.Fatalf("expected viewportTransform to succeed")
	}

	// 3. Drag the Move action's End anchor (anchor index 1) via the gizmo
	_, objKey, objCfg, _, moveIdx, moveSegs := gizmo.activeSelection()
	if moveIdx < 0 || len(moveSegs) == 0 {
		t.Fatalf("expected active Move action with segments")
	}
	origPathD := objCfg.Actions[moveIdx].PathD
	ex, ey := svgToView(moveSegs[0].EndX, moveSegs[0].EndY)
	gizmo.Dragged(&fyne.DragEvent{
		PointEvent: fyne.PointEvent{Position: fyne.NewPos(float32(ex)+40, float32(ey)+30)},
		Dragged:    fyne.NewDelta(40, 30),
	})
	gizmo.DragEnd()

	updatedCfg := mw.session.Objects[objKey]
	if updatedCfg.Actions[moveIdx].PathD == origPathD {
		t.Errorf("expected Move action PathD to change after dragging end anchor, still %q", updatedCfg.Actions[moveIdx].PathD)
	}

	// 4. Drag the first control handle (Ctrl1) to bend the line into a cubic Bézier curve (C)
	_, _, _, _, _, moveSegs = gizmo.activeSelection()
	c1x, c1y := svgToView(moveSegs[0].Ctrl1X, moveSegs[0].Ctrl1Y)
	gizmo.Dragged(&fyne.DragEvent{
		PointEvent: fyne.PointEvent{Position: fyne.NewPos(float32(c1x), float32(c1y)-35)},
		Dragged:    fyne.NewDelta(0, -35),
	})
	gizmo.DragEnd()

	curvedCfg := mw.session.Objects[objKey]
	if !strings.Contains(curvedCfg.Actions[moveIdx].PathD, " C ") {
		t.Errorf("expected dragging Ctrl1 handle to promote path to cubic Bézier (' C '), got %q", curvedCfg.Actions[moveIdx].PathD)
	}

	// 5. Switch to Track selection mode (clear selectedActionID) and drag the Pivot crosshair to set a custom pivot
	mw.selectedActionID = ""
	mw.timelinePanel.SelectTrack(targetNodeID)
	_, pivX, pivY, hasBounds := gizmo.resolveObjectBoundsAndPivot(targetNodeID, curvedCfg)
	if hasBounds {
		px, py := svgToView(pivX, pivY)
		gizmo.Dragged(&fyne.DragEvent{
			PointEvent: fyne.PointEvent{Position: fyne.NewPos(float32(px)+25, float32(py)+25)},
			Dragged:    fyne.NewDelta(25, 25),
		})
		gizmo.DragEnd()

		pivCfg := mw.session.Objects[objKey]
		if pivCfg.PivotType != "custom" {
			t.Errorf("expected PivotType=='custom' after dragging pivot crosshair, got %q", pivCfg.PivotType)
		}
	}
}

func TestMultiTrackMoveGizmoAnimatesElements(t *testing.T) {
	fyneApp := test.NewApp()
	defer fyneApp.Quit()

	mw := NewMainWindow(fyneApp)
	defer func() {
		waitForPreview(mw)
		mw.centerPanel.Pause()
	}()

	svgData := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape" width="200" height="200" viewBox="0 0 200 200">
  <g id="layer1" inkscape:groupmode="layer" inkscape:label="Red Box">
    <rect id="rect1" x="10" y="20" width="30" height="30" fill="#ff0000" />
  </g>
  <g id="layer2" inkscape:groupmode="layer" inkscape:label="Blue Box">
    <rect id="rect2" x="10" y="120" width="30" height="30" fill="#0000ff" />
  </g>
</svg>`)
	tmpDir := t.TempDir()
	svgPath := filepath.Join(tmpDir, "two_layers.svg")
	if err := os.WriteFile(svgPath, svgData, 0644); err != nil {
		t.Fatalf("failed to write test SVG: %v", err)
	}
	if err := mw.AddSVGFile(svgPath); err != nil {
		t.Fatalf("AddSVGFile failed: %v", err)
	}
	waitForPreview(mw)
	mw.centerPanel.Pause()

	tracks := mw.timelinePanel.CollectTracks()
	var layerNodeIDs []string
	for _, tr := range tracks {
		if n := mw.editorState.NodeMap[tr.NodeID]; n != nil && !n.IsLinkedFile {
			layerNodeIDs = append(layerNodeIDs, tr.NodeID)
		}
	}
	if len(layerNodeIDs) < 2 {
		t.Fatalf("expected at least 2 layer tracks, got %v", layerNodeIDs)
	}

	gizmo := mw.centerPanel.gizmoOverlay
	gizmo.Resize(fyne.NewSize(400, 400))
	svgToView, _, ok := gizmo.viewportTransform(400, 400)
	if !ok {
		t.Fatalf("viewportTransform failed")
	}

	// Add Move actions to both layers and drag their end anchors far to the right
	seenActionIDs := make(map[string]bool)
	for _, nID := range layerNodeIDs[:2] {
		mw.timelinePanel.SelectTrack(nID)
		mw.timelinePanel.AddActionToActiveNode("Move")
		if mw.selectedActionID == "" {
			t.Fatalf("expected selectedActionID after adding Move to %s", nID)
		}
		if seenActionIDs[mw.selectedActionID] {
			t.Fatalf("duplicate action ID generated across tracks: %s", mw.selectedActionID)
		}
		seenActionIDs[mw.selectedActionID] = true

		// Even if user clicks the track header (clearing selectedActionID), the Move gizmo must stay active
		mw.selectedActionID = ""
		mw.timelinePanel.SelectTrack(nID)

		selNodeID, _, _, _, moveIdx, moveSegs := gizmo.activeSelection()
		if selNodeID != nID || moveIdx < 0 || len(moveSegs) == 0 {
			t.Fatalf("expected active Move gizmo for %s, got node=%s moveIdx=%d", nID, selNodeID, moveIdx)
		}

		ex, ey := svgToView(moveSegs[0].EndX, moveSegs[0].EndY)
		gizmo.Dragged(&fyne.DragEvent{
			PointEvent: fyne.PointEvent{Position: fyne.NewPos(float32(ex)+80, float32(ey))},
			Dragged:    fyne.NewDelta(80, 0),
		})
		gizmo.DragEnd()
	}

	waitForPreview(mw)

	if len(mw.session.Document.MotionPaths) < 2 {
		t.Fatalf("expected at least 2 MotionPaths on Document, got %d", len(mw.session.Document.MotionPaths))
	}
	frames := mw.centerPanel.mainCanvasFrames
	if len(frames) < 2 {
		t.Fatalf("expected animated preview frames, got %d", len(frames))
	}

	// Compare first frame and last frame to verify elements actually moved across the canvas
	first := frames[0].Image
	last := frames[len(frames)-1].Image
	bounds := first.Bounds()
	diffPixels := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r1, g1, b1, a1 := first.At(x, y).RGBA()
			r2, g2, b2, a2 := last.At(x, y).RGBA()
			if r1 != r2 || g1 != g2 || b1 != b2 || a1 != a2 {
				diffPixels++
			}
		}
	}
	if diffPixels == 0 {
		t.Errorf("expected rendered pixels to move between frame 0 and frame %d after dragging Move gizmos, but frames are identical", len(frames)-1)
	}
}

func TestEmptyLayersFilteredAndLeafShapesInTransformedGroupAnimate(t *testing.T) {
	fyneApp := test.NewApp()
	defer fyneApp.Quit()

	mw := NewMainWindow(fyneApp)
	defer func() {
		waitForPreview(mw)
		mw.centerPanel.Pause()
	}()

	// SVG modeled after MossNerd.svg:
	// - Empty self-closing <g inkscape:groupmode="layer" /> shells ("Glasses", "Hair Top")
	// - Non-empty layer ("Ear") with a translated parent group g36 containing:
	//   * Unlabeled leaf <path id="path7"> (actual glasses)
	//   * Labeled leaf <path id="path8" inkscape:label="Shirt" style="...;paint-order:stroke fill markers">
	//   * Nested group <g id="g35" inkscape:label="Right Eye">
	svgData := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape" width="256" height="256" viewBox="0 0 256 256">
  <g inkscape:groupmode="layer" id="layer6" inkscape:label="Glasses" style="display:inline" />
  <g inkscape:groupmode="layer" id="layer2" inkscape:label="Hair Top" style="display:inline" />
  <g inkscape:groupmode="layer" id="layer4" inkscape:label="Ear" style="display:inline">
    <g id="g36" transform="translate(37.66581,-7.1259644)">
      <path id="path7" d="M 40 40 L 90 40 L 90 60 L 40 60 Z" style="fill:#00ff00;stroke:#000000;stroke-width:2" />
      <path id="path8" inkscape:label="Shirt" d="M 40 110 L 120 110 L 120 170 L 40 170 Z" style="fill:#38bdf8;stroke:#000000;stroke-width:6;paint-order:stroke fill markers" />
      <g id="g35" inkscape:label="Right Eye">
        <circle id="c_eye" cx="65" cy="50" r="8" fill="#ff0055" />
      </g>
    </g>
  </g>
</svg>`)

	tmpDir := t.TempDir()
	svgPath := filepath.Join(tmpDir, "moss_nerd_like.svg")
	if err := os.WriteFile(svgPath, svgData, 0644); err != nil {
		t.Fatalf("failed to write test SVG: %v", err)
	}
	if err := mw.AddSVGFile(svgPath); err != nil {
		t.Fatalf("AddSVGFile failed: %v", err)
	}
	waitForPreview(mw)
	mw.centerPanel.Pause()

	// 1. Empty layers must return an empty Rect{} from GetElementRect (not fall back to full DrawingRect)
	emptyRect := mw.session.Document.GetElementRect("layer6")
	if emptyRect.Width > 0 || emptyRect.Height > 0 {
		t.Errorf("expected empty layer6 GetElementRect to have 0 size, got %+v", emptyRect)
	}

	// 2. CollectTracks must match Inkscape 1:1: show empty layers marked "(empty)" in visual z-order,
	// with Layer: Ear starting collapsed (not exploding grandchildren).
	tracks := mw.timelinePanel.CollectTracks()
	trackByID := make(map[string]TimelineTrackModel, len(tracks))
	gotIDs := make([]string, 0, len(tracks))
	for _, tr := range tracks {
		trackByID[tr.NodeID] = tr
		gotIDs = append(gotIDs, tr.NodeID)
	}
	wantInitialOrder := []string{"src_1", "layer4", "layer2", "layer6"}
	if strings.Join(gotIDs, ",") != strings.Join(wantInitialOrder, ",") {
		t.Fatalf("expected initial tracks %v, got %v", wantInitialOrder, gotIDs)
	}
	if !trackByID["layer6"].IsEmpty || trackByID["layer6"].Title != "Layer: Glasses (empty)" || trackByID["layer6"].HasChildren {
		t.Errorf("unexpected empty layer6 model: %+v", trackByID["layer6"])
	}
	if !trackByID["layer2"].IsEmpty || trackByID["layer2"].Title != "Layer: Hair Top (empty)" || trackByID["layer2"].HasChildren {
		t.Errorf("unexpected empty layer2 model: %+v", trackByID["layer2"])
	}
	if trackByID["layer4"].IsEmpty || !trackByID["layer4"].IsCollapsed || !trackByID["layer4"].HasChildren {
		t.Errorf("expected layer4 ('Ear') to start collapsed with children, got %+v", trackByID["layer4"])
	}

	// Verify tapping a rendered track header row button triggers selection cleanly
	var tappedLayer6Btn *timelineTrackHeaderBtn
	var findTrackBtn func(co fyne.CanvasObject, label string) *timelineTrackHeaderBtn
	findTrackBtn = func(co fyne.CanvasObject, label string) *timelineTrackHeaderBtn {
		if b, ok := co.(*timelineTrackHeaderBtn); ok && b.text == label {
			return b
		}
		if c, ok := co.(*fyne.Container); ok {
			for _, child := range c.Objects {
				if found := findTrackBtn(child, label); found != nil {
					return found
				}
			}
		}
		return nil
	}
	for _, row := range mw.timelinePanel.tracksVBox.Objects {
		if b := findTrackBtn(row, "Layer: Glasses (empty)"); b != nil {
			tappedLayer6Btn = b
			break
		}
	}
	if tappedLayer6Btn == nil {
		t.Fatalf("expected to find rendered timelineTrackHeaderBtn for empty layer6")
	}
	test.Tap(tappedLayer6Btn)
	if mw.editorState.ActiveNode == nil || mw.editorState.ActiveNode.ID != "layer6" {
		t.Fatalf("expected tapping header button to select layer6, got %+v", mw.editorState.ActiveNode)
	}
	if mw.timelinePanel.AddActionBtn.Disabled() {
		t.Errorf("expected AddActionBtn to be enabled when empty layer6 is selected")
	}
	mw.timelinePanel.AddActionToActiveNode("Move")
	if _, cfg, hasCfg := mw.resolveNodeObjectConfig(mw.editorState.NodeMap["layer6"]); !hasCfg || len(cfg.Actions) != 1 {
		t.Errorf("expected AddActionToActiveNode on empty layer6 to add 1 action, got hasCfg=%v len=%d", hasCfg, len(cfg.Actions))
	}
	mw.doUndo()
	waitForPreview(mw)
	if _, cfg, hasCfg := mw.resolveNodeObjectConfig(mw.editorState.NodeMap["layer6"]); hasCfg && len(cfg.Actions) != 0 {
		t.Errorf("expected Undo to revert empty layer6 action, got len=%d", len(cfg.Actions))
	}

	// 3. Expanding ONLY layer4 must reveal g36 collapsed (no grandchild explosion yet)
	mw.timelinePanel.collapsedNodes["layer4"] = false
	mw.timelinePanel.Refresh()
	tracksAfterEar := mw.timelinePanel.CollectTracks()
	afterEarByID := make(map[string]TimelineTrackModel)
	for _, tr := range tracksAfterEar {
		afterEarByID[tr.NodeID] = tr
	}
	if tr, ok := afterEarByID["g36"]; !ok || !tr.IsCollapsed || !tr.HasChildren {
		t.Fatalf("expected expanding Ear to reveal g36 collapsed with children, got ok=%v tr=%+v", ok, tr)
	}
	if _, exploded := afterEarByID["path7"]; exploded {
		t.Errorf("expected g36 children (path7) to stay hidden until g36 is expanded")
	}

	// 4. Expanding g36 reveals path7, path8, and g35 (collapsed so c_eye stays hidden)
	mw.timelinePanel.collapsedNodes["g36"] = false
	mw.timelinePanel.Refresh()
	tracksAfterG36 := mw.timelinePanel.CollectTracks()
	afterG36ByID := make(map[string]TimelineTrackModel)
	for _, tr := range tracksAfterG36 {
		afterG36ByID[tr.NodeID] = tr
	}
	for _, wantID := range []string{"layer4", "g36", "path7", "path8", "g35"} {
		if _, exists := afterG36ByID[wantID]; !exists {
			t.Errorf("expected drawable element %q to be present after expanding g36, got tracks=%+v", wantID, tracksAfterG36)
		}
	}
	if tr, ok := afterG36ByID["g35"]; !ok || !tr.IsCollapsed {
		t.Errorf("expected nested group g35 to remain collapsed, got ok=%v tr=%+v", ok, tr)
	}
	if _, exploded := afterG36ByID["c_eye"]; exploded {
		t.Errorf("expected c_eye inside collapsed g35 to stay hidden")
	}

	// 5. Animate leaf shape path7 (glasses) and paint-order leaf shape path8 (Shirt) inside translated group g36
	for _, nID := range []string{"path7", "path8"} {
		mw.timelinePanel.SelectTrack(nID)
		mw.timelinePanel.AddActionToActiveNode("Move")
	}
	waitForPreview(mw)

	frame0SVG, err := svg.BuildTimelineFrameSVGAtTime(mw.session.Document, 0.0, mw.timelinePanel.TotalDurationSec(), svg.Rect{})
	if err != nil {
		t.Fatalf("BuildTimelineFrameSVGAtTime(0) failed: %v", err)
	}
	frameEndSVG, err := svg.BuildTimelineFrameSVGAtTime(mw.session.Document, mw.timelinePanel.TotalDurationSec(), mw.timelinePanel.TotalDurationSec(), svg.Rect{})
	if err != nil {
		t.Fatalf("BuildTimelineFrameSVGAtTime(end) failed: %v", err)
	}
	if string(frame0SVG) == string(frameEndSVG) {
		t.Fatalf("expected frame SVG at t=end to differ from t=0 when leaf shapes are animated")
	}
	if !strings.Contains(string(frameEndSVG), `id="path8_stroke"`) || !strings.Contains(string(frameEndSVG), `translate(32.000000, 0.000000)`) {
		t.Errorf("expected both path7 and paint-order desugared path8_stroke/path8 to receive translate(32, 0) at end frame, got:\n%s", string(frameEndSVG))
	}

	frames := mw.centerPanel.mainCanvasFrames
	if len(frames) < 2 {
		t.Fatalf("expected animated preview frames, got %d", len(frames))
	}
	first := frames[0].Image
	last := frames[len(frames)-1].Image
	bounds := first.Bounds()
	diffPixels := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r1, g1, b1, a1 := first.At(x, y).RGBA()
			r2, g2, b2, a2 := last.At(x, y).RGBA()
			if r1 != r2 || g1 != g2 || b1 != b2 || a1 != a2 {
				diffPixels++
			}
		}
	}
	if diffPixels == 0 {
		t.Errorf("expected rendered pixels to move between frame 0 and frame %d for leaf shapes in transformed group", len(frames)-1)
	}
}

func TestActionEditModalLiveUpdateAndDeleteLayerWithChildren(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()

	mw := NewMainWindow(a)
	defer func() {
		mw.Close()
		mw.centerPanel.Pause()
	}()

	svgData := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape" width="256" height="256" viewBox="0 0 256 256">
  <g inkscape:groupmode="layer" id="layer4" inkscape:label="Ear">
    <g id="g36">
      <path id="path7" d="M 40 40 L 90 40 L 90 60 L 40 60 Z" style="fill:#00ff00" />
    </g>
  </g>
  <g inkscape:groupmode="layer" id="layer2" inkscape:label="KeepLayer">
    <rect id="keepRect" x="10" y="10" width="20" height="20" fill="#ff0000" />
  </g>
</svg>`)

	tmpDir := t.TempDir()
	svgPath := filepath.Join(tmpDir, "edit_and_delete.svg")
	if err := os.WriteFile(svgPath, svgData, 0644); err != nil {
		t.Fatalf("failed to write test SVG: %v", err)
	}
	if err := mw.AddSVGFile(svgPath); err != nil {
		t.Fatalf("AddSVGFile failed: %v", err)
	}
	waitForPreview(mw)
	mw.centerPanel.Pause()

	// Add a Rot action to path7
	mw.timelinePanel.SelectTrack("path7")
	mw.timelinePanel.AddActionToActiveNode("Rot")
	waitForPreview(mw)

	openModalAndFindGridControls := func() (*widget.Select, *widget.Entry, *widget.Button, *widget.Button) {
		mw.timelinePanel.ShowActionEditModal("path7", 0)
		top := mw.window.Canvas().Overlays().Top()
		if top == nil {
			t.Fatalf("expected action edit modal overlay to be open")
		}
		var easeSelect *widget.Select
		var degEntry *widget.Entry
		var saveBtn, cancelBtn *widget.Button

		// Walk containers/widgets/popups looking for labels paired with their adjacent grid controls and action buttons
		var walk func(co fyne.CanvasObject)
		walk = func(co fyne.CanvasObject) {
			if co == nil {
				return
			}
			if btn, ok := co.(*widget.Button); ok {
				switch btn.Text {
				case "Save Action":
					saveBtn = btn
				case "Cancel":
					cancelBtn = btn
				}
				return
			}
			if pop, ok := co.(*widget.PopUp); ok {
				walk(pop.Content)
				return
			}
			if c, ok := co.(*fyne.Container); ok {
				for i, child := range c.Objects {
					if lbl, isLbl := child.(*widget.Label); isLbl && i+1 < len(c.Objects) {
						switch lbl.Text {
						case "Ease:":
							if sel, okSel := c.Objects[i+1].(*widget.Select); okSel {
								easeSelect = sel
							}
						case "Degrees:":
							if ent, okEnt := c.Objects[i+1].(*widget.Entry); okEnt {
								degEntry = ent
							}
						}
					}
					walk(child)
				}
				return
			}
			if w, ok := co.(fyne.Widget); ok {
				for _, obj := range test.WidgetRenderer(w).Objects() {
					walk(obj)
				}
			}
		}
		walk(top)
		if easeSelect == nil || degEntry == nil || saveBtn == nil || cancelBtn == nil {
			t.Fatalf("missing modal widgets: ease=%v deg=%v save=%v cancel=%v", easeSelect != nil, degEntry != nil, saveBtn != nil, cancelBtn != nil)
		}
		return easeSelect, degEntry, saveBtn, cancelBtn
	}

	// 1. Live editing updates session.Objects + Document.MotionPaths BEFORE Save, and Cancel reverts!
	easeSel, degEnt, _, cancelBtn := openModalAndFindGridControls()
	degEnt.SetText("180")
	easeSel.SetSelected("linear")

	_, cfgLive, _ := mw.resolveNodeObjectConfig(mw.editorState.NodeMap["path7"])
	if len(cfgLive.Actions) != 1 || cfgLive.Actions[0].RotationAngle != 180 || cfgLive.Actions[0].Ease != "linear" {
		t.Fatalf("expected immediate live update before Save (180/linear), got %+v", cfgLive.Actions)
	}
	if len(mw.session.Document.MotionPaths) == 0 || mw.session.Document.MotionPaths[0].Config.RotationAngle != 180 {
		t.Fatalf("expected Document.MotionPaths to update live to 180, got %+v", mw.session.Document.MotionPaths)
	}

	test.Tap(cancelBtn)
	waitForPreview(mw)
	_, cfgCanceled, _ := mw.resolveNodeObjectConfig(mw.editorState.NodeMap["path7"])
	if cfgCanceled.Actions[0].RotationAngle != 45 || cfgCanceled.Actions[0].Ease != "in-out" {
		t.Fatalf("expected Cancel to revert to 45/in-out, got %+v", cfgCanceled.Actions[0])
	}

	// 2. Re-open, edit live to 120 deg, and click Save Action
	_, degEnt2, saveBtn2, _ := openModalAndFindGridControls()
	degEnt2.SetText("120")
	test.Tap(saveBtn2)
	waitForPreview(mw)
	_, cfgSaved, _ := mw.resolveNodeObjectConfig(mw.editorState.NodeMap["path7"])
	if cfgSaved.Actions[0].RotationAngle != 120 {
		t.Fatalf("expected Save Action to persist 120 deg, got %+v", cfgSaved.Actions[0])
	}

	// 3. Delete non-empty Layer "layer4" (contains g36 -> path7 + Rot action)
	if err := mw.DeleteTrackNode("layer4"); err != nil {
		t.Fatalf("DeleteTrackNode(layer4) failed: %v", err)
	}
	waitForPreview(mw)

	if mw.editorState.NodeMap["layer4"] != nil || mw.editorState.NodeMap["g36"] != nil || mw.editorState.NodeMap["path7"] != nil {
		t.Fatalf("expected layer4 and descendants removed from NodeMap after deletion")
	}
	if strings.Contains(string(mw.rawSourceBytes["src_1"]), `id="layer4"`) || strings.Contains(string(mw.rawSourceBytes["src_1"]), `id="path7"`) {
		t.Errorf("expected layer4 subtree removed from rawSourceBytes, got:\n%s", string(mw.rawSourceBytes["src_1"]))
	}
	for k, cfg := range mw.session.Objects {
		if k == "layer4" || k == "g36" || k == "path7" || cfg.FallbackXMLID == "path7" {
			t.Errorf("expected deleted descendant ObjectConfig %q (fallback %q) removed", k, cfg.FallbackXMLID)
		}
	}

	// 4. Undo must restore layer4, descendants, rawSourceBytes, AND path7's 120-deg Rot action; Redo removes them again
	mw.doUndo()
	waitForPreview(mw)
	if mw.editorState.NodeMap["layer4"] == nil || mw.editorState.NodeMap["path7"] == nil {
		t.Fatalf("expected Undo to restore layer4 and path7")
	}
	if !strings.Contains(string(mw.rawSourceBytes["src_1"]), `id="layer4"`) {
		t.Errorf("expected Undo to restore layer4 in rawSourceBytes")
	}
	if _, cfgRestored, hasRestored := mw.resolveNodeObjectConfig(mw.editorState.NodeMap["path7"]); !hasRestored || len(cfgRestored.Actions) != 1 || cfgRestored.Actions[0].RotationAngle != 120 {
		t.Errorf("expected Undo to restore path7 Rot(120) action, got has=%v cfg=%+v", hasRestored, cfgRestored)
	}

	mw.doRedo()
	waitForPreview(mw)
	if mw.editorState.NodeMap["layer4"] != nil || mw.editorState.NodeMap["path7"] != nil {
		t.Fatalf("expected Redo to remove layer4 and path7 again")
	}
	if strings.Contains(string(mw.rawSourceBytes["src_1"]), `id="layer4"`) {
		t.Errorf("expected Redo to remove layer4 from rawSourceBytes again")
	}
}






