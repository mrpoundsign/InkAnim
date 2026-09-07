package ui

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	xdraw "golang.org/x/image/draw"

	"inkanim/internal/app"
	"inkanim/internal/gif"
)

type cachedPreviewFrame struct {
	withGuides    *image.RGBA
	withoutGuides *image.RGBA
	twitch112     *image.RGBA
	twitch56      *image.RGBA
	twitch28      *image.RGBA
}

// CenterPreviewPanel manages the live animation player and Twitch chat-scale emulation preview.
type CenterPreviewPanel struct {
	session   *app.Session
	container *fyne.Container

	mainCanvasImage *canvas.Image
	frameLabel      *widget.Label
	playPauseBtn    *widget.Button
	loopCheck       *widget.Check
	cropGuidesCheck *widget.Check

	// Twitch Scale Emulation previews
	twitch112Dark  *canvas.Image
	twitch56Dark   *canvas.Image
	twitch28Dark   *canvas.Image
	twitch112Light *canvas.Image
	twitch56Light  *canvas.Image
	twitch28Light  *canvas.Image

	isPlaying      bool
	loop           bool
	showCropGuides bool
	currentIdx     int
	speedFactor    float64
	cachedFrames   []cachedPreviewFrame

	mu      sync.Mutex
	timer   *time.Timer
	animGen int
}

// NewCenterPreviewPanel constructs the animation preview and Twitch inspector dock.
func NewCenterPreviewPanel(sess *app.Session) *CenterPreviewPanel {
	p := &CenterPreviewPanel{
		session:        sess,
		loop:           true,
		showCropGuides: true,
		speedFactor:    1.0,
	}

	// Main Canvas Image
	blank := image.NewRGBA(image.Rect(0, 0, 300, 300))
	p.mainCanvasImage = canvas.NewImageFromImage(blank)
	p.mainCanvasImage.FillMode = canvas.ImageFillContain
	p.mainCanvasImage.ScaleMode = canvas.ImageScaleFastest
	p.mainCanvasImage.SetMinSize(fyne.NewSize(150, 150))

	p.frameLabel = widget.NewLabel("Frame: 0 / 0")

	p.playPauseBtn = widget.NewButton("Play", func() {
		p.TogglePlay()
	})
	p.playPauseBtn.Importance = widget.DangerImportance

	prevBtn := widget.NewButton("◀ Step", func() {
		p.StepFrame(-1)
	})
	nextBtn := widget.NewButton("Step ▶", func() {
		p.StepFrame(1)
	})

	p.loopCheck = widget.NewCheck("Loop", func(checked bool) {
		p.loop = checked
	})
	p.loopCheck.Checked = true

	p.cropGuidesCheck = widget.NewCheck("Crop Guides", func(checked bool) {
		p.mu.Lock()
		p.showCropGuides = checked
		p.renderCurrentFrameLocked()
		p.mu.Unlock()
	})
	p.cropGuidesCheck.Checked = true

	speedSelect := widget.NewSelect([]string{"0.25x", "0.5x", "1x", "1.5x", "2x"}, func(s string) {
		p.mu.Lock()
		defer p.mu.Unlock()
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
	})
	speedSelect.SetSelected("1x")

	playbackControls := container.NewHBox(
		prevBtn,
		p.playPauseBtn,
		nextBtn,
		p.loopCheck,
		p.cropGuidesCheck,
		speedSelect,
		p.frameLabel,
	)


	// Twitch Scale Emulation Dock
	p.twitch112Dark = p.newScaledImage(112)
	p.twitch56Dark = p.newScaledImage(56)
	p.twitch28Dark = p.newScaledImage(28)

	p.twitch112Light = p.newScaledImage(112)
	p.twitch56Light = p.newScaledImage(56)
	p.twitch28Light = p.newScaledImage(28)

	darkLabel := widget.NewLabelWithStyle("Twitch Dark (#18181B):", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	darkRow := container.NewHBox(
		darkLabel,
		p.wrapWithBackground(p.twitch112Dark, 112, color.RGBA{R: 24, G: 24, B: 27, A: 255}),
		p.wrapWithBackground(p.twitch56Dark, 56, color.RGBA{R: 24, G: 24, B: 27, A: 255}),
		p.wrapWithBackground(p.twitch28Dark, 28, color.RGBA{R: 24, G: 24, B: 27, A: 255}),
	)

	lightLabel := widget.NewLabelWithStyle("Twitch Light (#FFFFFF):", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	lightRow := container.NewHBox(
		lightLabel,
		p.wrapWithBackground(p.twitch112Light, 112, color.RGBA{R: 255, G: 255, B: 255, A: 255}),
		p.wrapWithBackground(p.twitch56Light, 56, color.RGBA{R: 255, G: 255, B: 255, A: 255}),
		p.wrapWithBackground(p.twitch28Light, 28, color.RGBA{R: 255, G: 255, B: 255, A: 255}),
	)

	twitchHeader := widget.NewLabelWithStyle("Twitch Chat-Scale Inspector (112px, 56px, 28px)", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	twitchEmulationBox := container.NewVBox(
		widget.NewSeparator(),
		twitchHeader,
		darkRow,
		lightRow,
	)

	mainStage := container.NewBorder(
		nil,
		playbackControls,
		nil,
		nil,
		p.mainCanvasImage,
	)

	p.container = container.NewBorder(
		nil,
		twitchEmulationBox,
		nil,
		nil,
		mainStage,
	)

	return p
}

func (p *CenterPreviewPanel) newScaledImage(size float32) *canvas.Image {
	blank := image.NewRGBA(image.Rect(0, 0, int(size), int(size)))
	img := canvas.NewImageFromImage(blank)
	img.FillMode = canvas.ImageFillContain
	img.ScaleMode = canvas.ImageScaleFastest
	img.SetMinSize(fyne.NewSize(size, size))
	return img
}

func (p *CenterPreviewPanel) wrapWithBackground(img *canvas.Image, size float32, bg color.Color) fyne.CanvasObject {
	rect := canvas.NewRectangle(bg)
	rect.SetMinSize(fyne.NewSize(size+8, size+8))
	return container.NewStack(rect, container.NewCenter(img))
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
		p.cachedFrames = nil
		p.frameLabel.SetText("Frame: 0 / 0")
		p.pauseLocked()
		return
	}

	p.rebuildCachedFramesLocked()

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

func (p *CenterPreviewPanel) pauseLocked() {
	p.animGen++
	if !p.isPlaying {
		return
	}
	p.isPlaying = false
	btn := p.playPauseBtn
	if btn != nil {
		fyne.Do(func() {
			btn.SetText("Play")
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

	// If loop is disabled or already on/past the last frame, restart from beginning
	if !p.loop || p.currentIdx >= len(frames)-1 {
		p.currentIdx = 0
	}

	// Stop any existing timer first
	if p.timer != nil {
		p.timer.Stop()
		p.timer = nil
	}

	p.animGen++
	currentGen := p.animGen
	p.isPlaying = true
	btn := p.playPauseBtn
	if btn != nil {
		fyne.Do(func() {
			btn.SetText("Pause")
			btn.Importance = widget.SuccessImportance
			btn.Refresh()
		})
	}

	durMs := p.session.RenderedFrames[p.currentIdx].DurationMs
	if durMs <= 0 {
		durMs = 100
	}
	speed := p.speedFactor
	if speed <= 0 {
		speed = 1.0
	}
	tickDelay := time.Duration(float64(durMs)/speed) * time.Millisecond
	if tickDelay < 10*time.Millisecond {
		tickDelay = 10 * time.Millisecond
	}

	var scheduleNextFrame func()
	scheduleNextFrame = func() {
		p.mu.Lock()
		if !p.isPlaying || p.animGen != currentGen || len(p.session.RenderedFrames) <= 1 {
			p.mu.Unlock()
			return
		}

		totalFrames := len(p.session.RenderedFrames)
		nextIdx := p.currentIdx + 1
		if nextIdx >= totalFrames {
			if !p.loop {
				p.currentIdx = totalFrames - 1
				p.pauseLocked()
				p.renderCurrentFrameLocked()
				p.mu.Unlock()
				return
			}
			nextIdx = 0
		}

		p.currentIdx = nextIdx
		p.renderCurrentFrameLocked()

		dur := p.session.RenderedFrames[p.currentIdx].DurationMs
		if dur <= 0 {
			dur = 100
		}
		s := p.speedFactor
		if s <= 0 {
			s = 1.0
		}
		delay := time.Duration(float64(dur)/s) * time.Millisecond
		if delay < 10*time.Millisecond {
			delay = 10 * time.Millisecond
		}

		p.timer = time.AfterFunc(delay, func() {
			fyne.Do(scheduleNextFrame)
		})
		p.mu.Unlock()
	}

	p.timer = time.AfterFunc(tickDelay, func() {
		fyne.Do(scheduleNextFrame)
	})
}

func (p *CenterPreviewPanel) rebuildCachedFramesLocked() {
	frames := p.session.RenderedFrames
	if len(frames) == 0 {
		p.cachedFrames = nil
		return
	}

	p.cachedFrames = make([]cachedPreviewFrame, len(frames))
	previewRect := p.session.GetPreviewBoundaryRect()

	for i, curr := range frames {
		if curr.Image == nil {
			continue
		}

		activeRect := p.session.GetActiveBoundaryRect(i)
		scaleX := float64(curr.Image.Bounds().Dx()) / previewRect.Width
		scaleY := float64(curr.Image.Bounds().Dy()) / previewRect.Height
		cropX0 := int(math.Round((activeRect.X - previewRect.X) * scaleX))
		cropY0 := int(math.Round((activeRect.Y - previewRect.Y) * scaleY))
		cropW := int(math.Round(activeRect.Width * scaleX))
		cropH := int(math.Round(activeRect.Height * scaleY))
		cropRect := image.Rect(cropX0, cropY0, cropX0+cropW, cropY0+cropH)

		// Crop for Twitch preview
		croppedForTwitch := cropImage(curr.Image, cropRect)

		displayImg := curr.Image
		var contentRect image.Rectangle
		if p.session.ExportOptions.ExportSquare {
			displayImg = gif.MakeSquare(curr.Image, 0)
			origB := curr.Image.Bounds()
			sqB := displayImg.Bounds()
			offsetX := (sqB.Dx() - origB.Dx()) / 2
			offsetY := (sqB.Dy() - origB.Dy()) / 2
			contentRect = cropRect.Add(image.Pt(offsetX, offsetY))
		} else {
			contentRect = cropRect
		}

		withGuides := drawCropGuides(displayImg, contentRect)
		withoutGuides := displayImg

		var twitchDisplayImg *image.RGBA
		if p.session.ExportOptions.ExportSquare {
			twitchDisplayImg = gif.MakeSquare(croppedForTwitch, 0)
		} else {
			twitchDisplayImg = croppedForTwitch
		}

		// Pre-scale Twitch thumbnails once to exact target dimensions
		twitch112 := scaleRGBA(twitchDisplayImg, 112, 112)
		twitch56 := scaleRGBA(twitchDisplayImg, 56, 56)
		twitch28 := scaleRGBA(twitchDisplayImg, 28, 28)

		p.cachedFrames[i] = cachedPreviewFrame{
			withGuides:    withGuides,
			withoutGuides: withoutGuides,
			twitch112:     twitch112,
			twitch56:      twitch56,
			twitch28:      twitch28,
		}
	}
}

func (p *CenterPreviewPanel) renderCurrentFrameLocked() {
	frames := p.session.RenderedFrames
	if len(frames) == 0 || p.currentIdx >= len(frames) {
		return
	}

	if len(p.cachedFrames) != len(frames) {
		p.rebuildCachedFramesLocked()
	}
	if len(p.cachedFrames) <= p.currentIdx {
		return
	}

	curr := frames[p.currentIdx]
	p.frameLabel.SetText(fmt.Sprintf("Frame %d of %d - %s - %dms", p.currentIdx+1, len(frames), curr.Label, curr.DurationMs))

	cached := p.cachedFrames[p.currentIdx]

	if p.showCropGuides {
		p.mainCanvasImage.Image = cached.withGuides
	} else {
		p.mainCanvasImage.Image = cached.withoutGuides
	}
	p.mainCanvasImage.Refresh()

	// Update Twitch scale emulation images (matching exact widget sizes, zero GL painter scaling)
	p.twitch112Dark.Image = cached.twitch112
	p.twitch112Dark.Refresh()
	p.twitch56Dark.Image = cached.twitch56
	p.twitch56Dark.Refresh()
	p.twitch28Dark.Image = cached.twitch28
	p.twitch28Dark.Refresh()

	p.twitch112Light.Image = cached.twitch112
	p.twitch112Light.Refresh()
	p.twitch56Light.Image = cached.twitch56
	p.twitch56Light.Refresh()
	p.twitch28Light.Image = cached.twitch28
	p.twitch28Light.Refresh()
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
	cornerLen := 14
	if w/4 < cornerLen {
		cornerLen = w / 4
	}
	if h/4 < cornerLen {
		cornerLen = h / 4
	}
	if cornerLen < 4 {
		cornerLen = 4
	}

	// Top-Left corner
	for i := 0; i < cornerLen; i++ {
		blendPixel(x0+i, y0, cornerCol)
		blendPixel(x0+i, y0+1, cornerCol)
		blendPixel(x0, y0+i, cornerCol)
		blendPixel(x0+1, y0+i, cornerCol)
	}
	// Top-Right corner
	for i := 0; i < cornerLen; i++ {
		blendPixel(x1-i, y0, cornerCol)
		blendPixel(x1-i, y0+1, cornerCol)
		blendPixel(x1, y0+i, cornerCol)
		blendPixel(x1-1, y0+i, cornerCol)
	}
	// Bottom-Left corner
	for i := 0; i < cornerLen; i++ {
		blendPixel(x0+i, y1, cornerCol)
		blendPixel(x0+i, y1-1, cornerCol)
		blendPixel(x0, y1-i, cornerCol)
		blendPixel(x0+1, y1-i, cornerCol)
	}
	// Bottom-Right corner
	for i := 0; i < cornerLen; i++ {
		blendPixel(x1-i, y1, cornerCol)
		blendPixel(x1-i, y1-1, cornerCol)
		blendPixel(x1, y1-i, cornerCol)
		blendPixel(x1-1, y1-i, cornerCol)
	}

	return dst
}

