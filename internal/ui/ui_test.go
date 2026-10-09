package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	appkg "inkanim/internal/app"
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
	defer waitForPreview(mw) // settle async preview builds before the next test
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
	defer waitForPreview(mw) // settle async preview builds before the next test
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

	// Test changing global speed
	loadAndWait(mw, testSVGPath)
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
	loadAndWait(mw, testSVGPath)
	mw.centerPanel.Pause()
	if len(mw.leftPanel.listContainer.Objects) != 10 {
		t.Errorf("expected exactly 10 frame objects in list, got %d", len(mw.leftPanel.listContainer.Objects))
	}
	loadAndWait(mw, secondSVGPath)
	mw.centerPanel.Pause()
	if len(mw.leftPanel.listContainer.Objects) != 30 {
		t.Errorf("expected exactly 30 frame objects in list, got %d", len(mw.leftPanel.listContainer.Objects))
	}
	loadAndWait(mw, testSVGPath)
	mw.centerPanel.Pause()
	if len(mw.leftPanel.listContainer.Objects) != 10 {
		t.Errorf("expected exactly 10 frame objects in list after switching back, got %d", len(mw.leftPanel.listContainer.Objects))
	}
}

func TestAnimationPlayback(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	mw := NewMainWindow(app)
	defer waitForPreview(mw) // settle async preview builds before the next test
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
	defer waitForPreview(mw) // settle async preview builds before the next test
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
	defer waitForPreview(mw) // settle async preview builds before the next test
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
	defer waitForPreview(mw) // settle async preview builds before the next test
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

	// 3. Verify Twitch status and export button reflect 18 frames (10*2 - 2)
	if !strings.Contains(mw.rightPanel.twitchStatusLabel.Text, "18 frames") {
		t.Errorf("expected twitchStatusLabel to reflect 18 bounced frames, got '%s'", mw.rightPanel.twitchStatusLabel.Text)
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
	defer waitForPreview(mw) // settle async preview builds before the next test
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
	if mw.leftPanel.cropBoundaryRadio.Selected != "Page" {
		t.Errorf("expected cropBoundaryRadio selected 'Page', got %s", mw.leftPanel.cropBoundaryRadio.Selected)
	}
	if mw.leftPanel.cropPageSelect.Disabled() {
		t.Errorf("expected cropPageSelect to be enabled in Page mode")
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
	defer waitForPreview(mw) // settle async preview builds before the next test
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

func TestSidebarTabs(t *testing.T) {
	testApp := test.NewApp()
	defer testApp.Quit()

	mw := NewMainWindow(testApp)
	defer waitForPreview(mw) // settle async preview builds before the next test
	if mw.sidebarTabs == nil {
		t.Fatal("expected sidebarTabs to be non-nil")
	}

	if len(mw.sidebarTabs.Items) != 2 {
		t.Fatalf("expected 2 sidebar tabs, got %d", len(mw.sidebarTabs.Items))
	}

	if mw.sidebarTabs.Items[0].Text != "Frames" {
		t.Errorf("expected first tab to be 'Frames', got %q", mw.sidebarTabs.Items[0].Text)
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
	defer waitForPreview(mw) // settle async preview builds before the next test
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
	defer waitForPreview(mw) // settle async preview builds before the next test
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
	defer waitForPreview(mw) // settle async preview builds before the next test
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

