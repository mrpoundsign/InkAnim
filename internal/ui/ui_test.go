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
	defer waitForPreview(mw) // settle async preview builds before the next test
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

func TestGPUResidentPreviewFrameStack(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	mw := NewMainWindow(app)
	defer waitForPreview(mw)

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
	defer waitForPreview(mw)

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
	defer waitForPreview(mw)

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
	defer waitForPreview(mw)

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
	defer waitForPreview(mw)

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

