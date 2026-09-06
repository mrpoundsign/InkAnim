package ui

import (
	"fmt"
	"image"
	"image/color"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"golang.org/x/image/draw"

	"inkanim/internal/app"
	"inkanim/internal/gif"
)

// CenterPreviewPanel manages the live animation player and Twitch chat-scale emulation preview.
type CenterPreviewPanel struct {
	session   *app.Session
	container *fyne.Container

	mainCanvasImage *canvas.Image
	frameLabel      *widget.Label
	playPauseBtn    *widget.Button
	loopCheck       *widget.Check

	// Twitch Scale Emulation previews
	twitch112Dark  *canvas.Image
	twitch56Dark   *canvas.Image
	twitch28Dark   *canvas.Image
	twitch112Light *canvas.Image
	twitch56Light  *canvas.Image
	twitch28Light  *canvas.Image

	isPlaying   bool
	loop        bool
	currentIdx  int
	speedFactor float64

	mu      sync.Mutex
	stop    chan struct{}
	animGen int
}

// NewCenterPreviewPanel constructs the animation preview and Twitch inspector dock.
func NewCenterPreviewPanel(sess *app.Session) *CenterPreviewPanel {
	p := &CenterPreviewPanel{
		session:     sess,
		loop:        true,
		speedFactor: 1.0,
	}

	// Main Canvas Image
	blank := image.NewRGBA(image.Rect(0, 0, 300, 300))
	p.mainCanvasImage = canvas.NewImageFromImage(blank)
	p.mainCanvasImage.FillMode = canvas.ImageFillContain
	p.mainCanvasImage.SetMinSize(fyne.NewSize(150, 150))

	p.frameLabel = widget.NewLabel("Frame: 0 / 0")

	p.playPauseBtn = widget.NewButton("▶ Play", func() {
		p.TogglePlay()
	})

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
		p.frameLabel.SetText("Frame: 0 / 0")
		p.pauseLocked()
		return
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

// Pause stops the animated playback.
func (p *CenterPreviewPanel) Pause() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pauseLocked()
}

func (p *CenterPreviewPanel) pauseLocked() {
	p.animGen++
	if !p.isPlaying {
		return
	}
	p.isPlaying = false
	p.animGen++
	p.playPauseBtn.SetText("▶ Play")
	if p.stop != nil {
		close(p.stop)
		p.stop = nil
	}
}

func (p *CenterPreviewPanel) playLocked() {
	frames := p.session.RenderedFrames
	if len(frames) <= 1 {
		return
	}

	// Stop any existing animation loop first
	if p.stop != nil {
		close(p.stop)
		p.stop = nil
	}

	p.animGen++
	currentGen := p.animGen
	p.isPlaying = true
	p.playPauseBtn.SetText("⏸ Pause")
	p.stop = make(chan struct{})

	go func(stopChan chan struct{}, gen int) {
		for {
			p.mu.Lock()
			totalFrames := len(p.session.RenderedFrames)
			if !p.isPlaying || p.animGen != gen || totalFrames == 0 {
				p.mu.Unlock()
				return
			}

			// Ensure currentIdx is always within bounds if a new SVG was loaded
			if p.currentIdx >= totalFrames {
				p.currentIdx = 0
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

			// Advance to next frame
			nextIdx := (p.currentIdx + 1) % totalFrames
			if !p.loop && nextIdx == 0 {
				// Reached end of animation without loop
				p.pauseLocked()
				p.mu.Unlock()
				return
			}

			p.currentIdx = nextIdx
			p.mu.Unlock()

			// Use fyne.Do to dispatch repaint on the main UI thread (required for OpenGL/GLFW wake-up)
			fyne.Do(func() {
				p.mu.Lock()
				defer p.mu.Unlock()
				if p.isPlaying && p.animGen == gen {
					p.renderCurrentFrameLocked()
				}
			})

			select {
			case <-stopChan:
				return
			case <-time.After(tickDelay):
			}
		}
	}(p.stop, currentGen)
}

func (p *CenterPreviewPanel) renderCurrentFrameLocked() {
	frames := p.session.RenderedFrames
	if len(frames) == 0 || p.currentIdx >= len(frames) {
		return
	}

	curr := frames[p.currentIdx]
	p.frameLabel.SetText(fmt.Sprintf("Frame %d of %d - %s - %dms", p.currentIdx+1, len(frames), curr.Label, curr.DurationMs))

	// If square mode is enabled, square-center the frame for display
	displayImg := curr.Image
	if p.session.ExportOptions.ExportSquare {
		displayImg = gif.MakeSquare(curr.Image, 0)
	}

	p.mainCanvasImage.Image = displayImg
	p.mainCanvasImage.Refresh()

	// Update Twitch scale emulation images
	scaled112 := scaleImage(displayImg, 112, 112)
	scaled56 := scaleImage(displayImg, 56, 56)
	scaled28 := scaleImage(displayImg, 28, 28)

	p.twitch112Dark.Image = scaled112
	p.twitch112Dark.Refresh()
	p.twitch56Dark.Image = scaled56
	p.twitch56Dark.Refresh()
	p.twitch28Dark.Image = scaled28
	p.twitch28Dark.Refresh()

	p.twitch112Light.Image = scaled112
	p.twitch112Light.Refresh()
	p.twitch56Light.Image = scaled56
	p.twitch56Light.Refresh()
	p.twitch28Light.Image = scaled28
	p.twitch28Light.Refresh()
}

func scaleImage(src *image.RGBA, w, h int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.BiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)
	return dst
}
