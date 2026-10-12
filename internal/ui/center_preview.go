package ui

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	xdraw "golang.org/x/image/draw"

	"inkanim/internal/app"
	"inkanim/internal/prof"
)

type cachedPreviewFrame struct {
	withGuides    *image.RGBA
	withoutGuides *image.RGBA
	twitch112     *image.RGBA
	twitch56      *image.RGBA
	twitch28      *image.RGBA
}

// dispatchUI hands async preview-build callbacks to the UI thread. Tests swap it
// for a queue drained on the test goroutine, because the Fyne test driver runs
// fyne.Do inline on the calling goroutine instead of the main thread.
var dispatchUI = fyne.Do

// PreviewBackground defines the background style rendered under the main preview canvas.
type PreviewBackground string

const (
	BgCheckerboard    PreviewBackground = "Checkerboard"
	BgTwitchDark      PreviewBackground = "Twitch Dark"
	BgTwitchLight     PreviewBackground = "Twitch Light"
	BgDiscordDark     PreviewBackground = "Discord Dark"
	BgVibrantGradient PreviewBackground = "Vibrant Gradient"
)

// PreviewBackgrounds lists the selectable preview backgrounds matching testdata/preview.html.
var PreviewBackgrounds = []string{
	string(BgCheckerboard),
	string(BgTwitchDark),
	string(BgTwitchLight),
	string(BgDiscordDark),
	string(BgVibrantGradient),
}

// CenterPreviewPanel manages the live animation player and Twitch chat-scale emulation preview.
type CenterPreviewPanel struct {
	session   *app.Session
	container *fyne.Container

	mainCanvasImage    *canvas.Image
	mainCanvasStack    *fyne.Container
	mainCanvasFrames   []*canvas.Image
	activeFrameIdx     int
	bgContainer        *fyne.Container
	stageStack         *fyne.Container
	currentBg          PreviewBackground
	bgSelect           *widget.Select
	frameLabel         *widget.Label
	playPauseBtn       *widget.Button
	loopCheck          *widget.Check
	pingPongCheck      *widget.Check
	cropGuidesCheck    *widget.Check
	motionLinesCheck   *widget.Check
	wysiwygCheck       *widget.Check
	inspectorCheck     *widget.Check
	twitchEmulationBox *fyne.Container

	// Twitch Scale Emulation previews
	twitch112Dark        *canvas.Image
	twitch56Dark         *canvas.Image
	twitch28Dark         *canvas.Image
	twitch112Light       *canvas.Image
	twitch56Light        *canvas.Image
	twitch28Light        *canvas.Image
	twitch112DarkStack   *fyne.Container
	twitch56DarkStack    *fyne.Container
	twitch28DarkStack    *fyne.Container
	twitch112LightStack  *fyne.Container
	twitch56LightStack   *fyne.Container
	twitch28LightStack   *fyne.Container
	twitch112DarkFrames  []*canvas.Image
	twitch56DarkFrames   []*canvas.Image
	twitch28DarkFrames   []*canvas.Image
	twitch112LightFrames []*canvas.Image
	twitch56LightFrames  []*canvas.Image
	twitch28LightFrames  []*canvas.Image

	isPlaying      bool
	loop           bool
	pingPong       bool
	pingPongDir    int
	showCropGuides bool
	wysiwygColors  bool
	currentIdx     int
	speedFactor    float64
	cachedFrames   []cachedPreviewFrame

	buttonText       string
	buttonImportance widget.Importance

	onPingPongChange func()

	// Async preview build state (guarded by mu)
	rebuildGen       int          // bumped per requested build; stale results are dropped
	rebuilding       bool         // a build for rebuildGen is in flight
	loadStart        time.Time    // set by BeginLoading; used for the profile-only first-paint timer
	rebuildsInFlight atomic.Int32 // builds whose result has not been dispatched yet (lets tests wait)

	// Loading / progress overlay shown over the preview while building
	progressOverlay *fyne.Container
	progressLabel   *widget.Label
	progressBar     *widget.ProgressBar
	progressBarInf  *widget.ProgressBarInfinite

	mu              sync.Mutex
	timer           *time.Timer
	animGen         int
	playStartTime             time.Time
	playStartOffset           time.Duration
	playLastCycle             int
	droppedFramesCurrentCycle int
	droppedFramesLastCycle    int
	fpsWarningText            string
}

func createCheckerboardImage(w, h int) image.Image {
	if w <= 0 || h <= 0 {
		return image.NewRGBA(image.Rect(0, 0, 1, 1))
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	c1 := [4]byte{31, 31, 35, 255} // #1f1f23
	c2 := [4]byte{42, 42, 48, 255} // #2a2a30
	const squareSize = 20

	pix := img.Pix
	stride := img.Stride
	for y := range h {
		rowEven := (y/squareSize)%2 == 0
		rowOffset := y * stride
		for x := range w {
			colEven := (x/squareSize)%2 == 0
			c := &c1
			if rowEven != colEven {
				c = &c2
			}
			offset := rowOffset + x*4
			pix[offset] = c[0]
			pix[offset+1] = c[1]
			pix[offset+2] = c[2]
			pix[offset+3] = c[3]
		}
	}
	return img
}

func createVibrantGradientImage(w, h int) image.Image {
	if w <= 0 || h <= 0 {
		return image.NewRGBA(image.Rect(0, 0, 1, 1))
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	// Color stops matching testdata/preview.html:
	// 0.0: #ff007f (255, 0, 127)
	// 0.5: #7928ca (121, 40, 202)
	// 1.0: #0070f3 (0, 112, 243)
	c0 := [3]float64{255, 0, 127}
	c1 := [3]float64{121, 40, 202}
	c2 := [3]float64{0, 112, 243}

	pix := img.Pix
	stride := img.Stride
	invW := 1.0 / float64(max(w-1, 1))
	invH := 1.0 / float64(max(h-1, 1))

	for y := range h {
		normY := float64(y) * invH
		rowOffset := y * stride
		for x := range w {
			normX := float64(x) * invW
			t := (normX + normY) * 0.5
			if t < 0 {
				t = 0
			} else if t > 1 {
				t = 1
			}

			var r, g, b byte
			if t <= 0.5 {
				factor := t * 2.0
				r = byte(c0[0] + (c1[0]-c0[0])*factor + 0.5)
				g = byte(c0[1] + (c1[1]-c0[1])*factor + 0.5)
				b = byte(c0[2] + (c1[2]-c0[2])*factor + 0.5)
			} else {
				factor := (t - 0.5) * 2.0
				r = byte(c1[0] + (c2[0]-c1[0])*factor + 0.5)
				g = byte(c1[1] + (c2[1]-c1[1])*factor + 0.5)
				b = byte(c1[2] + (c2[2]-c1[2])*factor + 0.5)
			}

			offset := rowOffset + x*4
			pix[offset] = r
			pix[offset+1] = g
			pix[offset+2] = b
			pix[offset+3] = 255
		}
	}
	return img
}

func (p *CenterPreviewPanel) makeBackgroundObject(bg PreviewBackground) fyne.CanvasObject {
	switch bg {
	case BgTwitchDark:
		return canvas.NewRectangle(color.RGBA{R: 24, G: 24, B: 27, A: 255})
	case BgTwitchLight:
		return canvas.NewRectangle(color.RGBA{R: 255, G: 255, B: 255, A: 255})
	case BgDiscordDark:
		return canvas.NewRectangle(color.RGBA{R: 49, G: 51, B: 56, A: 255})
	case BgVibrantGradient:
		return canvas.NewRaster(createVibrantGradientImage)
	case BgCheckerboard:
		fallthrough
	default:
		return canvas.NewRaster(createCheckerboardImage)
	}
}

func normalizeBackground(bg PreviewBackground) PreviewBackground {
	switch bg {
	case BgTwitchDark, BgTwitchLight, BgDiscordDark, BgVibrantGradient:
		return bg
	case BgCheckerboard:
		return BgCheckerboard
	default:
		return BgCheckerboard
	}
}

// SetBackground changes the main canvas background dynamically.
func (p *CenterPreviewPanel) SetBackground(bg PreviewBackground) {
	bg = normalizeBackground(bg)
	fyne.Do(func() {
		p.mu.Lock()
		p.currentBg = bg
		if p.bgContainer != nil {
			obj := p.makeBackgroundObject(bg)
			p.bgContainer.Objects = []fyne.CanvasObject{obj}
		}
		p.mu.Unlock()

		if p.bgContainer != nil {
			p.bgContainer.Refresh()
		}

		if p.bgSelect != nil && p.bgSelect.Selected != string(bg) {
			p.bgSelect.SetSelected(string(bg))
		}
	})
}

// CurrentBackground returns the currently active preview background mode.
func (p *CenterPreviewPanel) CurrentBackground() PreviewBackground {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.currentBg
}

// NewCenterPreviewPanel constructs the animation preview and Twitch inspector dock.
func NewCenterPreviewPanel(sess *app.Session) *CenterPreviewPanel {
	p := &CenterPreviewPanel{
		session:          sess,
		loop:             true,
		showCropGuides:   true,
		wysiwygColors:    true,
		speedFactor:      1.0,
		currentBg:        BgCheckerboard,
		buttonText:       "Play",
		buttonImportance: widget.DangerImportance,
	}

	p.bgContainer = container.NewStack(p.makeBackgroundObject(BgCheckerboard))

	// Main Canvas Image & GPU-resident frame stack
	blank := image.NewRGBA(image.Rect(0, 0, 300, 300))
	p.mainCanvasImage = canvas.NewImageFromImage(blank)
	p.mainCanvasImage.FillMode = canvas.ImageFillContain
	p.mainCanvasImage.ScaleMode = canvas.ImageScaleFastest
	p.mainCanvasImage.SetMinSize(fyne.NewSize(150, 150))
	p.mainCanvasStack = container.NewStack(p.mainCanvasImage)
	p.activeFrameIdx = -1

	// Loading / progress overlay
	p.progressLabel = widget.NewLabelWithStyle("", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	p.progressBar = widget.NewProgressBar()
	p.progressBarInf = widget.NewProgressBarInfinite()
	p.progressBarInf.Stop()
	p.progressBarInf.Hide()
	progressCardBg := canvas.NewRectangle(color.RGBA{R: 15, G: 17, B: 21, A: 235})
	progressCardBg.StrokeColor = color.RGBA{R: 56, G: 189, B: 248, A: 160} // Electric Sky Blue (#38BDF8)
	progressCardBg.StrokeWidth = 1
	progressCardBg.CornerRadius = 8
	progressCard := container.NewStack(
		progressCardBg,
		container.NewPadded(container.NewVBox(
			p.progressLabel,
			container.NewStack(p.progressBar, p.progressBarInf),
		)),
	)
	p.progressOverlay = container.NewCenter(container.New(&fixedWidthLayout{width: 280}, progressCard))
	p.progressOverlay.Hide()

	p.stageStack = container.NewStack(
		p.bgContainer,
		p.mainCanvasStack,
		p.progressOverlay,
	)

	p.frameLabel = widget.NewLabelWithStyle("Frame: 0 / 0", fyne.TextAlignCenter, fyne.TextStyle{})

	p.playPauseBtn = widget.NewButtonWithIcon("Play", theme.MediaPlayIcon(), func() {
		p.TogglePlay()
	})
	p.playPauseBtn.Importance = widget.DangerImportance

	prevBtn := widget.NewButtonWithIcon("Step Back", theme.MediaSkipPreviousIcon(), func() {
		p.StepFrame(-1)
	})
	nextBtn := widget.NewButtonWithIcon("Step Forward", theme.MediaSkipNextIcon(), func() {
		p.StepFrame(1)
	})

	p.loopCheck = widget.NewCheck("Loop", func(checked bool) {
		p.loop = checked
	})
	p.loopCheck.Checked = true

	p.pingPongCheck = widget.NewCheck("Ping-Pong", func(checked bool) {
		p.mu.Lock()
		p.pingPong = checked
		if p.session != nil {
			p.session.ExportOptions.PingPong = checked
		}
		p.mu.Unlock()
		if p.onPingPongChange != nil {
			p.onPingPongChange()
		}
	})

	p.cropGuidesCheck = widget.NewCheck("Crop Guides", func(checked bool) {
		p.mu.Lock()
		p.showCropGuides = checked
		if len(p.mainCanvasFrames) == len(p.cachedFrames) {
			for i, cf := range p.cachedFrames {
				if checked {
					p.mainCanvasFrames[i].Image = cf.withGuides
				} else {
					p.mainCanvasFrames[i].Image = cf.withoutGuides
				}
				p.mainCanvasFrames[i].Refresh()
			}
		}
		p.renderCurrentFrameLocked()
		p.mu.Unlock()
	})
	p.cropGuidesCheck.Checked = true

	p.motionLinesCheck = widget.NewCheck("Motion Lines", func(checked bool) {
		p.mu.Lock()
		if p.session != nil {
			_ = p.session.SetShowMotionLines(checked)
		}
		p.requestRebuildLocked()
		p.renderCurrentFrameLocked()
		p.mu.Unlock()
	})
	p.motionLinesCheck.Checked = false

	speedSelect := widget.NewSelect([]string{"0.25x", "0.5x", "1x", "1.5x", "2x"}, func(s string) {
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.isPlaying && !p.playStartTime.IsZero() {
			realElapsed := time.Since(p.playStartTime)
			p.playStartOffset += time.Duration(float64(realElapsed) * p.speedFactor)
			p.playStartTime = time.Now()
		}
		switch s {
		case "0.25x":
			p.speedFactor = 0.25
		case "0.5x":
			p.speedFactor = 0.5
		case "1x":
			p.speedFactor = 1.0
		case "1.5x":
			p.speedFactor = 1.5
		case "2x":
			p.speedFactor = 2.0
		default:
			p.speedFactor = 1.0
		}
		p.resetFPSMonitorLocked()
	})
	speedSelect.SetSelected("1x")

	p.bgSelect = widget.NewSelect(PreviewBackgrounds, func(s string) {
		p.SetBackground(PreviewBackground(s))
	})
	p.bgSelect.SetSelected(string(BgCheckerboard))

	p.inspectorCheck = widget.NewCheck("Scale Inspector", func(checked bool) {
		p.mu.Lock()
		if checked {
			p.twitchEmulationBox.Show()
			p.renderCurrentFrameLocked()
		} else {
			p.twitchEmulationBox.Hide()
		}
		p.mu.Unlock()
		if p.container != nil {
			p.container.Refresh()
		}
	})
	p.inspectorCheck.Checked = true

	p.wysiwygCheck = widget.NewCheck("WYSIWYG", func(checked bool) {
		p.mu.Lock()
		p.wysiwygColors = checked
		p.requestRebuildLocked()
		p.renderCurrentFrameLocked()
		p.mu.Unlock()
	})
	p.wysiwygCheck.Checked = true

	playbackRow := container.NewHBox(
		prevBtn,
		p.playPauseBtn,
		nextBtn,
		speedSelect,
		p.loopCheck,
		p.pingPongCheck,
	)

	viewOptionsRow := container.NewHBox(
		widget.NewLabel("BG:"),
		p.bgSelect,
		p.cropGuidesCheck,
		p.motionLinesCheck,
		p.wysiwygCheck,
		p.inspectorCheck,
	)

	playbackControls := container.NewVBox(
		container.NewCenter(playbackRow),
		container.NewCenter(viewOptionsRow),
		p.frameLabel,
	)

	// Twitch Scale Emulation Dock: 3 sizes (112px, 56px, 28px) on Dark & Light backgrounds
	p.twitch112DarkStack, p.twitch112Dark = p.newScaledStack(112)
	p.twitch56DarkStack, p.twitch56Dark = p.newScaledStack(56)
	p.twitch28DarkStack, p.twitch28Dark = p.newScaledStack(28)

	p.twitch112LightStack, p.twitch112Light = p.newScaledStack(112)
	p.twitch56LightStack, p.twitch56Light = p.newScaledStack(56)
	p.twitch28LightStack, p.twitch28Light = p.newScaledStack(28)

	darkLabel := widget.NewLabelWithStyle("Twitch Dark (#18181B)", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	darkSubSwatches := container.NewHBox(
		p.wrapWithBackground(p.twitch56DarkStack, 56, color.RGBA{R: 24, G: 24, B: 27, A: 255}),
		p.wrapWithBackground(p.twitch28DarkStack, 28, color.RGBA{R: 24, G: 24, B: 27, A: 255}),
	)
	darkCardContent := container.NewVBox(
		darkLabel,
		p.wrapWithBackground(p.twitch112DarkStack, 112, color.RGBA{R: 24, G: 24, B: 27, A: 255}),
		container.NewCenter(darkSubSwatches),
	)
	darkCardBg := canvas.NewRectangle(color.RGBA{R: 15, G: 17, B: 21, A: 255})
	darkCardBg.StrokeColor = color.RGBA{R: 47, G: 55, B: 69, A: 200}
	darkCardBg.StrokeWidth = 1
	darkCardBg.CornerRadius = 8
	darkCard := container.NewStack(
		darkCardBg,
		container.NewPadded(darkCardContent),
	)

	lightLabel := widget.NewLabelWithStyle("Twitch Light (#FFFFFF)", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	lightSubSwatches := container.NewHBox(
		p.wrapWithBackground(p.twitch56LightStack, 56, color.RGBA{R: 255, G: 255, B: 255, A: 255}),
		p.wrapWithBackground(p.twitch28LightStack, 28, color.RGBA{R: 255, G: 255, B: 255, A: 255}),
	)
	lightCardContent := container.NewVBox(
		lightLabel,
		p.wrapWithBackground(p.twitch112LightStack, 112, color.RGBA{R: 255, G: 255, B: 255, A: 255}),
		container.NewCenter(lightSubSwatches),
	)
	lightCardBg := canvas.NewRectangle(color.RGBA{R: 15, G: 17, B: 21, A: 255})
	lightCardBg.StrokeColor = color.RGBA{R: 47, G: 55, B: 69, A: 200}
	lightCardBg.StrokeWidth = 1
	lightCardBg.CornerRadius = 8
	lightCard := container.NewStack(
		lightCardBg,
		container.NewPadded(lightCardContent),
	)

	inspectorHeader := widget.NewLabelWithStyle("Chat Scale", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	inspectorContent := container.NewVBox(
		inspectorHeader,
		darkCard,
		lightCard,
	)

	p.twitchEmulationBox = container.NewBorder(
		nil, nil,
		widget.NewSeparator(),
		nil,
		container.NewVScroll(container.NewPadded(inspectorContent)),
	)

	stageWithInspector := container.NewBorder(
		nil,
		nil,
		nil,
		p.twitchEmulationBox,
		p.stageStack,
	)

	p.container = container.NewBorder(
		nil,
		playbackControls,
		nil,
		nil,
		stageWithInspector,
	)

	return p
}

// SetInspectorVisible toggles visibility of the bottom chat-scale inspector dock.
func (p *CenterPreviewPanel) SetInspectorVisible(visible bool) {
	p.inspectorCheck.SetChecked(visible)
}

// SetWysiwyg toggles whether preview frames display color quantization and dithering.
func (p *CenterPreviewPanel) SetWysiwyg(enabled bool) {
	p.wysiwygCheck.SetChecked(enabled)
}

// SetPingPong toggles the ping-pong playback mode.
func (p *CenterPreviewPanel) SetPingPong(enabled bool) {
	p.pingPongCheck.SetChecked(enabled)
}

// SetOnPingPongChange registers a callback when ping-pong mode changes.
func (p *CenterPreviewPanel) SetOnPingPongChange(fn func()) {
	p.onPingPongChange = fn
}

type fixedSquareLayout struct {
	size float32
}

func (l *fixedSquareLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objects {
		o.Resize(fyne.NewSize(l.size, l.size))
		o.Move(fyne.NewPos((size.Width-l.size)/2, (size.Height-l.size)/2))
	}
}

func (l *fixedSquareLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(l.size, l.size)
}

func (p *CenterPreviewPanel) newScaledStack(size float32) (*fyne.Container, *canvas.Image) {
	img := p.newScaledImage(size)
	st := container.NewStack(img)
	return st, img
}

func (p *CenterPreviewPanel) newScaledImage(size float32) *canvas.Image {
	blank := image.NewRGBA(image.Rect(0, 0, int(size), int(size)))
	img := canvas.NewImageFromImage(blank)
	img.FillMode = canvas.ImageFillContain
	img.ScaleMode = canvas.ImageScaleFastest
	img.SetMinSize(fyne.NewSize(size, size))
	return img
}

func (p *CenterPreviewPanel) wrapWithBackground(obj fyne.CanvasObject, size float32, bg color.Color) fyne.CanvasObject {
	boxSize := size + 8
	rect := canvas.NewRectangle(bg)
	rect.SetMinSize(fyne.NewSize(boxSize, boxSize))
	rect.CornerRadius = 4
	rect.StrokeColor = color.RGBA{R: 71, G: 85, B: 105, A: 180} // Slate border (#475569)
	rect.StrokeWidth = 1
	sq := container.NewStack(rect, container.New(&fixedSquareLayout{size: size}, obj))
	return container.NewCenter(sq)
}

// Container returns the UI container for the preview.
func (p *CenterPreviewPanel) Container() *fyne.Container {
	return p.container
}

// Refresh updates the preview with current session frames.
func (p *CenterPreviewPanel) Refresh() {
	p.mu.Lock()
	defer p.mu.Unlock()

	frames := p.session.RenderedFrames
	if len(frames) == 0 {
		p.rebuildGen++ // drop any in-flight build
		p.rebuilding = false
		p.hideProgressLocked()
		p.cachedFrames = nil
		p.frameLabel.SetText("Frame: 0 / 0")
		p.resetFPSMonitorLocked()
		p.pauseLocked()
		return
	}

	// Build the preview cache in the background. Until it lands, playback keeps
	// showing the previous cache if it still matches, otherwise playback waits.
	p.requestRebuildLocked()

	totalFrames := len(frames)
	if p.pingPongCheck != nil {
		if totalFrames >= 3 {
			p.pingPongCheck.Enable()
		} else {
			p.pingPongCheck.Disable()
		}
	}

	if p.currentIdx >= len(frames) {
		p.currentIdx = 0
	}

	p.renderCurrentFrameLocked()

	if p.isPlaying && len(frames) > 1 {
		p.playLocked()
	} else {
		p.pauseLocked()
	}
}

// StepFrame steps forward or backward by delta.
func (p *CenterPreviewPanel) StepFrame(delta int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	frames := p.session.RenderedFrames
	if len(frames) == 0 {
		return
	}

	if p.currentIdx < 0 || p.currentIdx >= len(frames) {
		p.currentIdx = 0
	}
	p.currentIdx = (p.currentIdx + delta%len(frames) + len(frames)) % len(frames)
	p.playStartOffset = p.timelineOffsetOfFrameLocked(p.currentIdx, p.pingPongDir)
	p.playStartTime = time.Now()
	p.renderCurrentFrameLocked()
}

// TogglePlay starts or pauses the animated playback.
func (p *CenterPreviewPanel) TogglePlay() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.isPlaying {
		p.pauseLocked()
	} else {
		p.playLocked()
	}
}

// Play starts the animated playback if there are multiple frames.
func (p *CenterPreviewPanel) Play() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.playLocked()
}

// Pause stops the animated playback.
func (p *CenterPreviewPanel) Pause() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pauseLocked()
}

// IsPlaying returns whether the animation is currently playing.
func (p *CenterPreviewPanel) IsPlaying() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.isPlaying
}

// PlayPauseButtonState returns the current play/pause button text and importance under lock.
func (p *CenterPreviewPanel) PlayPauseButtonState() (string, widget.Importance) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.buttonText, p.buttonImportance
}

// FPSWarning returns the current text of the low FPS warning and whether it is active under lock.
func (p *CenterPreviewPanel) FPSWarning() (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.fpsWarningText, p.fpsWarningText != ""
}

func (p *CenterPreviewPanel) timelineDurationsLocked() (forwardTotal, cycleTotal time.Duration) {
	frames := p.session.RenderedFrames
	if len(frames) == 0 {
		return 0, 0
	}
	for _, f := range frames {
		dur := time.Duration(max(f.DurationMs, 10)) * time.Millisecond
		forwardTotal += dur
	}
	if p.pingPong && len(frames) >= 3 {
		reverseTotal := time.Duration(0)
		for i := len(frames) - 2; i >= 1; i-- {
			dur := time.Duration(max(frames[i].DurationMs, 10)) * time.Millisecond
			reverseTotal += dur
		}
		cycleTotal = forwardTotal + reverseTotal
	} else {
		cycleTotal = forwardTotal
	}
	return forwardTotal, cycleTotal
}

func (p *CenterPreviewPanel) timelineOffsetOfFrameLocked(frameIdx, pingPongDir int) time.Duration {
	frames := p.session.RenderedFrames
	if len(frames) == 0 || frameIdx < 0 || frameIdx >= len(frames) {
		return 0
	}
	offset := time.Duration(0)
	if p.pingPong && len(frames) >= 3 && pingPongDir < 0 {
		for _, f := range frames {
			offset += time.Duration(max(f.DurationMs, 10)) * time.Millisecond
		}
		for i := len(frames) - 2; i > frameIdx; i-- {
			offset += time.Duration(max(frames[i].DurationMs, 10)) * time.Millisecond
		}
		return offset
	}
	for i := range frameIdx {
		offset += time.Duration(max(frames[i].DurationMs, 10)) * time.Millisecond
	}
	return offset
}

func (p *CenterPreviewPanel) frameAtTimelineOffsetLocked(offset, forwardTotal, cycleTotal time.Duration) (int, int, time.Duration) {
	frames := p.session.RenderedFrames
	totalFrames := len(frames)
	if totalFrames == 0 {
		return 0, 1, 10 * time.Millisecond
	}
	if totalFrames == 1 {
		return 0, 1, 10 * time.Millisecond
	}

	if p.pingPong && totalFrames >= 3 {
		if offset >= forwardTotal {
			revOffset := offset - forwardTotal
			accum := time.Duration(0)
			for i := totalFrames - 2; i >= 1; i-- {
				dur := time.Duration(max(frames[i].DurationMs, 10)) * time.Millisecond
				if revOffset < accum+dur {
					return i, -1, (accum + dur) - revOffset
				}
				accum += dur
			}
			return 1, -1, 10 * time.Millisecond
		}
	}

	accum := time.Duration(0)
	for i := range totalFrames {
		dur := time.Duration(max(frames[i].DurationMs, 10)) * time.Millisecond
		if offset < accum+dur {
			return i, 1, (accum + dur) - offset
		}
		accum += dur
	}
	return totalFrames - 1, 1, 10 * time.Millisecond
}

func timelineStepIndex(frameIdx, dir, totalFrames int, pingPong bool) int {
	if pingPong && totalFrames >= 3 {
		if dir < 0 {
			return (totalFrames - 1) + (totalFrames - 1 - frameIdx)
		}
		return frameIdx
	}
	return frameIdx
}

func timelineStepsPerCycle(totalFrames int, pingPong bool) int {
	if pingPong && totalFrames >= 3 {
		return (totalFrames - 1) * 2
	}
	return totalFrames
}

func (p *CenterPreviewPanel) recordTimelineStepLocked(prevIdx, prevDir, nextIdx, nextDir, cycleDiff, totalFrames int) {
	stepsPerCycle := timelineStepsPerCycle(totalFrames, p.pingPong)
	if stepsPerCycle <= 0 {
		return
	}
	prevStep := timelineStepIndex(prevIdx, prevDir, totalFrames, p.pingPong)
	currStep := timelineStepIndex(nextIdx, nextDir, totalFrames, p.pingPong)

	if cycleDiff == 0 {
		stepsAdvanced := currStep - prevStep
		if stepsAdvanced > 1 {
			p.droppedFramesCurrentCycle += (stepsAdvanced - 1)
		}
		return
	}

	// cycleDiff > 0: One or more cycles completed!
	oldStepsRemaining := (stepsPerCycle - 1) - prevStep
	if oldStepsRemaining > 0 {
		p.droppedFramesCurrentCycle += oldStepsRemaining
	}

	if cycleDiff > 1 {
		p.droppedFramesLastCycle = stepsPerCycle
	} else {
		p.droppedFramesLastCycle = p.droppedFramesCurrentCycle
	}
	p.fpsWarningText = formatDroppedFramesWarning(p.droppedFramesLastCycle)
	p.droppedFramesCurrentCycle = currStep
}

func formatDroppedFramesWarning(dropped int) string {
	switch {
	case dropped == 1:
		return "  ⚠️ 1 frame dropped"
	case dropped > 1:
		return fmt.Sprintf("  ⚠️ %d frames dropped", dropped)
	default:
		return ""
	}
}

func (p *CenterPreviewPanel) finalizeRunLocked(prevIdx, prevDir, finalIdx, finalDir, totalFrames int) {
	stepsPerCycle := timelineStepsPerCycle(totalFrames, p.pingPong)
	if stepsPerCycle <= 0 {
		return
	}
	prevStep := timelineStepIndex(prevIdx, prevDir, totalFrames, p.pingPong)
	finalStep := timelineStepIndex(finalIdx, finalDir, totalFrames, p.pingPong)
	if p.pingPong && totalFrames >= 3 && finalIdx == 0 && finalDir < 0 {
		finalStep = stepsPerCycle
	}
	if finalStep > prevStep+1 {
		p.droppedFramesCurrentCycle += (finalStep - prevStep - 1)
	}
	p.droppedFramesLastCycle = p.droppedFramesCurrentCycle
	p.fpsWarningText = formatDroppedFramesWarning(p.droppedFramesLastCycle)
	p.droppedFramesCurrentCycle = 0
}

func (p *CenterPreviewPanel) resetFPSMonitorLocked() {
	p.droppedFramesCurrentCycle = 0
	p.droppedFramesLastCycle = 0
	if p.fpsWarningText != "" {
		p.fpsWarningText = ""
		p.renderCurrentFrameLocked()
	}
}

func (p *CenterPreviewPanel) pauseLocked() {
	p.animGen++
	p.playStartTime = time.Time{}
	if !p.isPlaying {
		return
	}
	p.isPlaying = false
	p.buttonText = "Play"
	p.buttonImportance = widget.DangerImportance
	btn := p.playPauseBtn
	if btn != nil {
		fyne.Do(func() {
			btn.SetText("Play")
			btn.SetIcon(theme.MediaPlayIcon())
			btn.Importance = widget.DangerImportance
			btn.Refresh()
		})
	}
	if p.timer != nil {
		p.timer.Stop()
		p.timer = nil
	}
}

func (p *CenterPreviewPanel) playLocked() {
	frames := p.session.RenderedFrames
	if len(frames) <= 1 {
		return
	}

	if p.pingPongDir == 0 {
		p.pingPongDir = 1
	}

	forwardTotal, cycleTotal := p.timelineDurationsLocked()
	if cycleTotal <= 0 {
		return
	}

	// Determine starting timeline offset
	switch {
	case !p.loop:
		switch {
		case p.pingPong && len(frames) >= 3:
			if p.currentIdx == 0 && p.pingPongDir < 0 {
				p.pingPongDir = 1
				p.currentIdx = 0
				p.playStartOffset = 0
			} else {
				p.playStartOffset = p.timelineOffsetOfFrameLocked(p.currentIdx, p.pingPongDir)
			}
		case p.currentIdx >= len(frames)-1:
			p.currentIdx = 0
			p.playStartOffset = 0
		default:
			p.playStartOffset = p.timelineOffsetOfFrameLocked(p.currentIdx, 1)
		}
	case p.currentIdx >= len(frames):
		p.currentIdx = 0
		p.playStartOffset = 0
	default:
		p.playStartOffset = p.timelineOffsetOfFrameLocked(p.currentIdx, p.pingPongDir)
	}

	// Stop any existing timer first
	if p.timer != nil {
		p.timer.Stop()
		p.timer = nil
	}

	p.animGen++
	currentGen := p.animGen
	p.isPlaying = true
	p.buttonText = "Pause"
	p.buttonImportance = widget.SuccessImportance
	btn := p.playPauseBtn
	if btn != nil {
		fyne.Do(func() {
			btn.SetText("Pause")
			btn.SetIcon(theme.MediaPauseIcon())
			btn.Importance = widget.SuccessImportance
			btn.Refresh()
		})
	}

	p.playStartTime = time.Now()
	p.playLastCycle = 0
	if p.playStartOffset == 0 {
		p.droppedFramesCurrentCycle = 0
	}

	speed := p.speedFactor
	if speed <= 0 {
		speed = 1.0
	}

	_, _, remaining := p.frameAtTimelineOffsetLocked(p.playStartOffset, forwardTotal, cycleTotal)
	tickDelay := max(time.Duration(float64(remaining)/speed), 5*time.Millisecond)

	var scheduleNextFrame func()
	scheduleNextFrame = func() {
		p.mu.Lock()
		if !p.isPlaying || p.animGen != currentGen || len(p.session.RenderedFrames) <= 1 {
			p.mu.Unlock()
			return
		}

		totalFrames := len(p.session.RenderedFrames)
		forwardTotal, cycleTotal := p.timelineDurationsLocked()
		if cycleTotal <= 0 {
			p.mu.Unlock()
			return
		}

		s := p.speedFactor
		if s <= 0 {
			s = 1.0
		}

		realElapsed := time.Since(p.playStartTime)
		timelineElapsed := p.playStartOffset + time.Duration(float64(realElapsed)*s)

		prevIdx := p.currentIdx
		prevDir := p.pingPongDir

		if !p.loop {
			if timelineElapsed >= cycleTotal {
				if p.pingPong && totalFrames >= 3 {
					p.finalizeRunLocked(prevIdx, prevDir, 0, -1, totalFrames)
					p.currentIdx = 0
					p.pingPongDir = 1
				} else {
					p.finalizeRunLocked(prevIdx, prevDir, totalFrames-1, 1, totalFrames)
					p.currentIdx = totalFrames - 1
					p.pingPongDir = 1
				}
				p.pauseLocked()
				p.renderCurrentFrameLocked()
				p.mu.Unlock()
				return
			}
		}

		cycleNum := int(timelineElapsed / cycleTotal)
		cycleDiff := cycleNum - p.playLastCycle
		p.playLastCycle = cycleNum
		cycleOffset := timelineElapsed % cycleTotal

		nextIdx, dir, remaining := p.frameAtTimelineOffsetLocked(cycleOffset, forwardTotal, cycleTotal)
		p.recordTimelineStepLocked(prevIdx, prevDir, nextIdx, dir, cycleDiff, totalFrames)
		p.currentIdx = nextIdx
		p.pingPongDir = dir

		p.renderCurrentFrameLocked()

		delay := max(time.Duration(float64(remaining)/s), 5*time.Millisecond)
		p.timer = time.AfterFunc(delay, func() {
			fyne.Do(scheduleNextFrame)
		})
		p.mu.Unlock()
	}

	// Playback state is set (button shows Pause), but the first tick waits until
	// the preview cache matches the frames; applyRebuildResult starts it.
	if len(p.cachedFrames) != len(frames) {
		return
	}

	p.timer = time.AfterFunc(tickDelay, func() {
		fyne.Do(scheduleNextFrame)
	})
}

// requestRebuildLocked starts a background build of the preview cache from a
// snapshot of the current session. Results from superseded builds are dropped.
func (p *CenterPreviewPanel) requestRebuildLocked() {
	p.rebuildGen++
	gen := p.rebuildGen
	p.rebuilding = true
	snap := p.snapshotLocked()
	p.showProgressLocked("Preparing previews…", 0, 0)

	p.rebuildsInFlight.Add(1)
	go func() {
		defer p.rebuildsInFlight.Add(-1)
		frames := buildPreviewCache(snap, func(stage string, done, total int) {
			dispatchUI(func() {
				p.mu.Lock()
				defer p.mu.Unlock()
				if gen == p.rebuildGen {
					p.showProgressLocked(stage, done, total)
				}
			})
		})
		dispatchUI(func() { p.applyRebuildResult(gen, frames) })
	}()
}

// applyRebuildResult swaps in a finished preview cache if it is still current.
func (p *CenterPreviewPanel) applyRebuildResult(gen int, frames []cachedPreviewFrame) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if gen != p.rebuildGen {
		return // superseded by a newer request
	}
	if len(frames) != len(p.session.RenderedFrames) {
		p.requestRebuildLocked() // session frames changed under us; build again
		return
	}

	p.cachedFrames = frames
	p.rebuilding = false
	if p.currentIdx >= len(frames) {
		p.currentIdx = 0
	}

	// Clean up any existing frame textures before installing the new frame stack
	if len(p.mainCanvasFrames) > 0 {
		for _, img := range p.mainCanvasFrames {
			img.Refresh()
		}
	}

	// Prepare GPU-resident preview frame stack
	p.mainCanvasFrames = make([]*canvas.Image, len(frames))
	stackObjs := make([]fyne.CanvasObject, len(frames))
	for i, cf := range frames {
		var imgData *image.RGBA
		if p.showCropGuides {
			imgData = cf.withGuides
		} else {
			imgData = cf.withoutGuides
		}
		img := canvas.NewImageFromImage(imgData)
		img.FillMode = canvas.ImageFillContain
		img.ScaleMode = canvas.ImageScaleFastest
		img.SetMinSize(fyne.NewSize(150, 150))
		if i == p.currentIdx {
			img.Show()
			p.mainCanvasImage = img
		} else {
			img.Hide()
		}
		p.mainCanvasFrames[i] = img
		stackObjs[i] = img
	}
	p.activeFrameIdx = p.currentIdx
	if len(stackObjs) > 0 {
		p.mainCanvasStack.Objects = stackObjs
	} else {
		p.mainCanvasStack.Objects = []fyne.CanvasObject{p.mainCanvasImage}
	}
	p.mainCanvasStack.Refresh()

	// Prepare GPU-resident Twitch thumbnail stacks
	makeThumbFrames := func(size float32, pick func(cf *cachedPreviewFrame) *image.RGBA) ([]*canvas.Image, []fyne.CanvasObject, *canvas.Image) {
		imgs := make([]*canvas.Image, len(frames))
		objs := make([]fyne.CanvasObject, len(frames))
		var active *canvas.Image
		for i, cf := range frames {
			img := canvas.NewImageFromImage(pick(&cf))
			img.FillMode = canvas.ImageFillContain
			img.ScaleMode = canvas.ImageScaleFastest
			img.SetMinSize(fyne.NewSize(size, size))
			if i == p.currentIdx {
				img.Show()
				active = img
			} else {
				img.Hide()
			}
			imgs[i] = img
			objs[i] = img
		}
		return imgs, objs, active
	}

	var stack112D, stack56D, stack28D, stack112L, stack56L, stack28L []fyne.CanvasObject
	p.twitch112DarkFrames, stack112D, p.twitch112Dark = makeThumbFrames(112, func(cf *cachedPreviewFrame) *image.RGBA { return cf.twitch112 })
	p.twitch56DarkFrames, stack56D, p.twitch56Dark = makeThumbFrames(56, func(cf *cachedPreviewFrame) *image.RGBA { return cf.twitch56 })
	p.twitch28DarkFrames, stack28D, p.twitch28Dark = makeThumbFrames(28, func(cf *cachedPreviewFrame) *image.RGBA { return cf.twitch28 })

	p.twitch112LightFrames, stack112L, p.twitch112Light = makeThumbFrames(112, func(cf *cachedPreviewFrame) *image.RGBA { return cf.twitch112 })
	p.twitch56LightFrames, stack56L, p.twitch56Light = makeThumbFrames(56, func(cf *cachedPreviewFrame) *image.RGBA { return cf.twitch56 })
	p.twitch28LightFrames, stack28L, p.twitch28Light = makeThumbFrames(28, func(cf *cachedPreviewFrame) *image.RGBA { return cf.twitch28 })

	if p.twitch112DarkStack != nil {
		p.twitch112DarkStack.Objects = stack112D
		p.twitch56DarkStack.Objects = stack56D
		p.twitch28DarkStack.Objects = stack28D
		p.twitch112LightStack.Objects = stack112L
		p.twitch56LightStack.Objects = stack56L
		p.twitch28LightStack.Objects = stack28L

		p.twitch112DarkStack.Refresh()
		p.twitch56DarkStack.Refresh()
		p.twitch28DarkStack.Refresh()
		p.twitch112LightStack.Refresh()
		p.twitch56LightStack.Refresh()
		p.twitch28LightStack.Refresh()
	}

	p.hideProgressLocked()
	p.renderCurrentFrameLocked()
	if !p.loadStart.IsZero() {
		prof.Since(p.loadStart, "TOTAL load -> first paint")
		p.loadStart = time.Time{}
	}
	if p.isPlaying && p.timer == nil {
		p.playLocked() // start playback that was requested while building
	}
}

// BeginLoading clears the preview and shows the loading overlay while a new
// document is rasterized. The overlay stays up until its preview cache lands.
func (p *CenterPreviewPanel) BeginLoading(stage string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.rebuildGen++ // drop any in-flight build for the previous document
	p.rebuilding = false
	p.cachedFrames = nil
	if len(p.mainCanvasFrames) > 0 {
		for _, img := range p.mainCanvasFrames {
			img.Refresh() // explicitly free GPU textures on previous session frames
		}
		p.mainCanvasFrames = nil
	}
	clearThumbFrames := func(frames *[]*canvas.Image, stack *fyne.Container, placeholder *canvas.Image) {
		if len(*frames) > 0 {
			for _, img := range *frames {
				img.Refresh()
			}
			*frames = nil
		}
		if stack != nil && placeholder != nil {
			stack.Objects = []fyne.CanvasObject{placeholder}
			placeholder.Show()
		}
	}
	clearThumbFrames(&p.twitch112DarkFrames, p.twitch112DarkStack, p.twitch112Dark)
	clearThumbFrames(&p.twitch56DarkFrames, p.twitch56DarkStack, p.twitch56Dark)
	clearThumbFrames(&p.twitch28DarkFrames, p.twitch28DarkStack, p.twitch28Dark)
	clearThumbFrames(&p.twitch112LightFrames, p.twitch112LightStack, p.twitch112Light)
	clearThumbFrames(&p.twitch56LightFrames, p.twitch56LightStack, p.twitch56Light)
	clearThumbFrames(&p.twitch28LightFrames, p.twitch28LightStack, p.twitch28Light)

	p.activeFrameIdx = -1
	p.currentIdx = 0
	p.pingPongDir = 1
	p.playStartTime = time.Time{}
	p.playStartOffset = 0
	p.playLastCycle = 0
	p.resetFPSMonitorLocked()
	p.loadStart = time.Now()
	p.mainCanvasImage.Image = image.NewRGBA(image.Rect(0, 0, 1, 1))
	p.mainCanvasImage.Show()
	p.mainCanvasStack.Objects = []fyne.CanvasObject{p.mainCanvasImage}
	p.mainCanvasImage.Refresh()
	p.showProgressLocked(stage, 0, 0)
}

// EndLoading hides the loading overlay (e.g. when loading failed).
func (p *CenterPreviewPanel) EndLoading() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.loadStart = time.Time{}
	if !p.rebuilding {
		p.hideProgressLocked()
	}
}

// showProgressLocked updates the overlay. total == 0 shows an indeterminate bar.
func (p *CenterPreviewPanel) showProgressLocked(stage string, done, total int) {
	if p.progressOverlay == nil {
		return
	}
	if total > 0 {
		p.progressLabel.SetText(fmt.Sprintf("%s %d/%d", stage, done, total))
		p.progressBarInf.Stop()
		p.progressBarInf.Hide()
		p.progressBar.Max = float64(total)
		p.progressBar.SetValue(float64(done))
		p.progressBar.Show()
	} else {
		p.progressLabel.SetText(stage)
		p.progressBar.Hide()
		p.progressBarInf.Show()
		if !p.progressBarInf.Running() {
			p.progressBarInf.Start()
		}
	}
	p.progressOverlay.Show()
}

func (p *CenterPreviewPanel) hideProgressLocked() {
	if p.progressOverlay == nil {
		return
	}
	p.progressBarInf.Stop()
	p.progressOverlay.Hide()
}

func (p *CenterPreviewPanel) renderCurrentFrameLocked() {
	frames := p.session.RenderedFrames
	if len(frames) == 0 || p.currentIdx >= len(frames) {
		return
	}

	if len(p.cachedFrames) != len(frames) {
		if !p.rebuilding {
			p.requestRebuildLocked()
		}
		return // drawn once the cache lands
	}

	curr := frames[p.currentIdx]

	displayIdx := p.currentIdx
	displayTotal := len(frames)
	if p.pingPong && len(frames) >= 3 {
		displayTotal = len(frames)*2 - 2
		if p.pingPongDir < 0 {
			displayIdx = displayTotal - p.currentIdx
		}
	}

	labelPart := ""
	if curr.Label != "" &&
		!strings.EqualFold(curr.Label, fmt.Sprintf("Frame %d", displayIdx+1)) &&
		!strings.EqualFold(curr.Label, fmt.Sprintf("Frame %d", p.currentIdx+1)) &&
		!strings.EqualFold(curr.Label, fmt.Sprintf("frame_%02d", p.currentIdx)) &&
		!strings.EqualFold(curr.Label, fmt.Sprintf("frame_%d", p.currentIdx)) &&
		!strings.EqualFold(curr.Label, fmt.Sprintf("frame_%02d", p.currentIdx+1)) &&
		!strings.EqualFold(curr.Label, fmt.Sprintf("frame_%d", p.currentIdx+1)) {
		labelPart = fmt.Sprintf(" (%s)", curr.Label)
	}

	p.frameLabel.SetText(fmt.Sprintf("Frame %d of %d%s - %dms%s", displayIdx+1, displayTotal, labelPart, curr.DurationMs, p.fpsWarningText))

	cached := p.cachedFrames[p.currentIdx]

	oldIdx := p.activeFrameIdx
	newIdx := p.currentIdx

	if len(p.mainCanvasFrames) == len(frames) && newIdx < len(p.mainCanvasFrames) {
		if oldIdx >= 0 && oldIdx < len(p.mainCanvasFrames) && oldIdx != newIdx {
			p.mainCanvasFrames[oldIdx].Hide()
		}
		p.mainCanvasFrames[newIdx].Show()
		p.mainCanvasImage = p.mainCanvasFrames[newIdx]
	} else {
		if p.showCropGuides {
			p.mainCanvasImage.Image = cached.withGuides
		} else {
			p.mainCanvasImage.Image = cached.withoutGuides
		}
		p.mainCanvasImage.Refresh()
	}

	// Update Twitch scale emulation images (GPU-resident frame stacks; zero re-upload).
	// Skipped while the inspector is hidden; its toggle re-renders when shown.
	if p.inspectorCheck != nil && p.inspectorCheck.Checked {
		updateThumbSlot := func(slotFrames []*canvas.Image, active **canvas.Image, fallback *image.RGBA) {
			if len(slotFrames) == len(frames) && newIdx < len(slotFrames) {
				if oldIdx >= 0 && oldIdx < len(slotFrames) && oldIdx != newIdx {
					slotFrames[oldIdx].Hide()
				}
				slotFrames[newIdx].Show()
				*active = slotFrames[newIdx]
			} else if *active != nil && fallback != nil {
				(*active).Image = fallback
				(*active).Refresh()
			}
		}

		updateThumbSlot(p.twitch112DarkFrames, &p.twitch112Dark, cached.twitch112)
		updateThumbSlot(p.twitch56DarkFrames, &p.twitch56Dark, cached.twitch56)
		updateThumbSlot(p.twitch28DarkFrames, &p.twitch28Dark, cached.twitch28)
		updateThumbSlot(p.twitch112LightFrames, &p.twitch112Light, cached.twitch112)
		updateThumbSlot(p.twitch56LightFrames, &p.twitch56Light, cached.twitch56)
		updateThumbSlot(p.twitch28LightFrames, &p.twitch28Light, cached.twitch28)
	}

	p.activeFrameIdx = newIdx
}

func scaleRGBA(src *image.RGBA, w, h int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	xdraw.BiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), xdraw.Src, nil)
	return dst
}

func cropImage(src *image.RGBA, r image.Rectangle) *image.RGBA {
	intersect := r.Intersect(src.Bounds())
	if intersect.Empty() || intersect.Dx() <= 0 || intersect.Dy() <= 0 {
		return src
	}
	dst := image.NewRGBA(image.Rect(0, 0, intersect.Dx(), intersect.Dy()))
	draw.Draw(dst, dst.Bounds(), src, intersect.Min, draw.Src)
	return dst
}

// drawCropGuides overlays subtle crop boundary lines, outer margin dimming, and corner L-brackets.
func drawCropGuides(src *image.RGBA, contentRect image.Rectangle) *image.RGBA {
	dst := image.NewRGBA(src.Bounds())
	copy(dst.Pix, src.Pix)

	b := dst.Bounds()
	x0 := contentRect.Min.X
	y0 := contentRect.Min.Y
	x1 := contentRect.Max.X - 1
	y1 := contentRect.Max.Y - 1

	// Subtle darkening outside the crop rectangle (if crop is smaller than canvas)
	if x0 > b.Min.X || y0 > b.Min.Y || x1 < b.Max.X-1 || y1 < b.Max.Y-1 {
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				if x < x0 || x > x1 || y < y0 || y > y1 {
					offset := (y-b.Min.Y)*dst.Stride + (x-b.Min.X)*4
					dst.Pix[offset] = uint8(float64(dst.Pix[offset]) * 0.6)
					dst.Pix[offset+1] = uint8(float64(dst.Pix[offset+1]) * 0.6)
					dst.Pix[offset+2] = uint8(float64(dst.Pix[offset+2]) * 0.6)
				}
			}
		}
	}
	if x0 < b.Min.X {
		x0 = b.Min.X
	}
	if y0 < b.Min.Y {
		y0 = b.Min.Y
	}
	if x1 >= b.Max.X {
		x1 = b.Max.X - 1
	}
	if y1 >= b.Max.Y {
		y1 = b.Max.Y - 1
	}
	if x1 <= x0 || y1 <= y0 {
		return dst
	}

	guideCol := color.RGBA{R: 145, G: 70, B: 255, A: 220}   // Twitch purple #9146FF
	cornerCol := color.RGBA{R: 191, G: 148, B: 255, A: 255} // Bright accent #BF94FF
	shadowCol := color.RGBA{R: 0, G: 0, B: 0, A: 140}

	blendPixel := func(x, y int, c color.RGBA) {
		if x < b.Min.X || x >= b.Max.X || y < b.Min.Y || y >= b.Max.Y {
			return
		}
		offset := (y-b.Min.Y)*dst.Stride + (x-b.Min.X)*4
		sr, sg, sb, sa := uint32(c.R), uint32(c.G), uint32(c.B), uint32(c.A)
		dr, dg, db, da := uint32(dst.Pix[offset]), uint32(dst.Pix[offset+1]), uint32(dst.Pix[offset+2]), uint32(dst.Pix[offset+3])
		a := sa
		invA := 255 - a
		dst.Pix[offset] = uint8((sr*a + dr*invA) / 255)
		dst.Pix[offset+1] = uint8((sg*a + dg*invA) / 255)
		dst.Pix[offset+2] = uint8((sb*a + db*invA) / 255)
		if da < a {
			dst.Pix[offset+3] = uint8(a)
		}
	}

	// 1. Subtle dashed perimeter lines (dash 4px, space 4px)
	for x := x0; x <= x1; x++ {
		if (x/4)%2 == 0 {
			blendPixel(x, y0, guideCol)
			blendPixel(x, y1, guideCol)
		} else {
			blendPixel(x, y0, shadowCol)
			blendPixel(x, y1, shadowCol)
		}
	}
	for y := y0; y <= y1; y++ {
		if (y/4)%2 == 0 {
			blendPixel(x0, y, guideCol)
			blendPixel(x1, y, guideCol)
		} else {
			blendPixel(x0, y, shadowCol)
			blendPixel(x1, y, shadowCol)
		}
	}

	// 2. Solid corner brackets (length = min(14, min(w, h)/4))
	w := x1 - x0
	h := y1 - y0
	cornerLen := max(min(h/4, min(w/4, 14)), 4)

	// Top-Left corner
	for i := range cornerLen {
		blendPixel(x0+i, y0, cornerCol)
		blendPixel(x0+i, y0+1, cornerCol)
		blendPixel(x0, y0+i, cornerCol)
		blendPixel(x0+1, y0+i, cornerCol)
	}
	// Top-Right corner
	for i := range cornerLen {
		blendPixel(x1-i, y0, cornerCol)
		blendPixel(x1-i, y0+1, cornerCol)
		blendPixel(x1, y0+i, cornerCol)
		blendPixel(x1-1, y0+i, cornerCol)
	}
	// Bottom-Left corner
	for i := range cornerLen {
		blendPixel(x0+i, y1, cornerCol)
		blendPixel(x0+i, y1-1, cornerCol)
		blendPixel(x0, y1-i, cornerCol)
		blendPixel(x0+1, y1-i, cornerCol)
	}
	// Bottom-Right corner
	for i := range cornerLen {
		blendPixel(x1-i, y1, cornerCol)
		blendPixel(x1-i, y1-1, cornerCol)
		blendPixel(x1, y1-i, cornerCol)
		blendPixel(x1-1, y1-i, cornerCol)
	}

	return dst
}
