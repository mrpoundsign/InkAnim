package ui

import (
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"inkanim/internal/app"
	"inkanim/internal/gif"
)

// RightExportPanel manages dimension inputs, Twitch presets, and single animated GIF export.
type RightExportPanel struct {
	session       *app.Session
	container     *container.Scroll
	parentWindow  fyne.Window

	presetSelect       *widget.Select
	customResContainer *fyne.Container
	squareCheck        *widget.Check
	squareSizeInput    *NumericCommitInput
	widthInput         *NumericCommitInput
	heightInput        *NumericCommitInput
	widthHeightRow     *fyne.Container

	colorsSelect      *widget.Select
	customColorsInput *NumericCommitInput
	customColorsRow   *fyne.Container
	ditherCheck       *widget.Check

	twitchStatusLabel *widget.Label
	exportBtn         *widget.Button

	onOptionsChange func()
	pausePlayback   func() func()
}

// NewRightExportPanel constructs the export settings panel.
func NewRightExportPanel(sess *app.Session, win fyne.Window, onOptionsChange func(), pausePlayback ...func() func()) *RightExportPanel {
	var pauseFn func() func()
	if len(pausePlayback) > 0 {
		pauseFn = pausePlayback[0]
	}
	p := &RightExportPanel{
		session:         sess,
		parentWindow:    win,
		onOptionsChange: onOptionsChange,
		pausePlayback:   pauseFn,
	}

	header := widget.NewLabelWithStyle("Export Settings", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	// Pre-allocate status label first so callbacks are safe
	p.twitchStatusLabel = widget.NewLabel("Twitch Status: Ready")

	p.squareSizeInput = NewNumericCommitInput(512, 16, 4096, "Resolution (px):", func(val int) {
		p.session.ExportOptions.SquareSize = val
		p.validateTwitch()
	})

	p.widthInput = NewNumericCommitInput(512, 16, 4096, "W:", func(val int) {
		p.session.ExportOptions.TargetWidth = val
		p.validateTwitch()
	})

	p.heightInput = NewNumericCommitInput(512, 16, 4096, "H:", func(val int) {
		p.session.ExportOptions.TargetHeight = val
		p.validateTwitch()
	})

	p.widthHeightRow = container.NewGridWithColumns(2,
		p.widthInput.Container,
		p.heightInput.Container,
	)
	p.widthHeightRow.Hide()

	p.squareCheck = widget.NewCheck("Force 1:1 Square (Twitch standard)", func(checked bool) {
		p.session.ExportOptions.ExportSquare = checked
		if checked {
			p.squareSizeInput.Show()
			p.widthHeightRow.Hide()
			p.session.ExportOptions.SquareSize = p.squareSizeInput.Value
		} else {
			p.squareSizeInput.Hide()
			p.widthHeightRow.Show()
			p.session.ExportOptions.TargetWidth = p.widthInput.Value
			p.session.ExportOptions.TargetHeight = p.heightInput.Value
		}
		p.validateTwitch()
		if p.onOptionsChange != nil {
			p.onOptionsChange()
		}
	})
	p.squareCheck.Checked = true

	p.customResContainer = container.NewVBox(
		p.squareCheck,
		p.squareSizeInput.Container,
		p.widthHeightRow,
	)
	p.customResContainer.Hide()

	// Preset Dropdown
	p.presetSelect = widget.NewSelect([]string{
		"Twitch Emote (512x512 Square)",
		"Discord Emote (128x128 Square)",
		"Custom Dimensions",
	}, func(selected string) {
		switch selected {
		case "Twitch Emote (512x512 Square)":
			p.session.ExportOptions.ExportSquare = true
			p.session.ExportOptions.SquareSize = 512
			p.squareCheck.Checked = true
			p.squareSizeInput.SetValue(512)
			p.squareSizeInput.Show()
			p.widthHeightRow.Hide()
			p.customResContainer.Hide()
			p.validateTwitch()
			if p.onOptionsChange != nil {
				p.onOptionsChange()
			}
		case "Discord Emote (128x128 Square)":
			p.session.ExportOptions.ExportSquare = true
			p.session.ExportOptions.SquareSize = 128
			p.squareCheck.Checked = true
			p.squareSizeInput.SetValue(128)
			p.squareSizeInput.Show()
			p.widthHeightRow.Hide()
			p.customResContainer.Hide()
			p.validateTwitch()
			if p.onOptionsChange != nil {
				p.onOptionsChange()
			}
		case "Custom Dimensions":
			p.customResContainer.Show()
			p.session.ExportOptions.ExportSquare = p.squareCheck.Checked
			if p.squareCheck.Checked {
				p.squareSizeInput.Show()
				p.widthHeightRow.Hide()
				p.session.ExportOptions.SquareSize = p.squareSizeInput.Value
			} else {
				p.squareSizeInput.Hide()
				p.widthHeightRow.Show()
				p.session.ExportOptions.TargetWidth = p.widthInput.Value
				p.session.ExportOptions.TargetHeight = p.heightInput.Value
			}
			p.validateTwitch()
			if p.onOptionsChange != nil {
				p.onOptionsChange()
			}
		}
	})
	p.presetSelect.Selected = "Twitch Emote (512x512 Square)"
	p.session.ExportOptions.ExportSquare = true
	p.session.ExportOptions.SquareSize = 512

	// Colors
	p.customColorsInput = NewNumericCommitInput(256, 2, 256, "Colors (2-256):", func(val int) {
		p.session.ExportOptions.NumColors = val
		p.validateTwitch()
	})
	p.customColorsRow = container.NewVBox(p.customColorsInput.Container)
	p.customColorsRow.Hide()

	p.colorsSelect = widget.NewSelect([]string{"256", "128", "64", "32", "Custom"}, func(s string) {
		if s == "Custom" {
			p.customColorsRow.Show()
			p.session.ExportOptions.NumColors = p.customColorsInput.Value
			p.validateTwitch()
		} else if c, err := strconv.Atoi(s); err == nil {
			p.customColorsRow.Hide()
			p.session.ExportOptions.NumColors = c
			p.validateTwitch()
		}
	})
	p.colorsSelect.Selected = "256"

	p.ditherCheck = widget.NewCheck("Dithering (Floyd-Steinberg)", func(b bool) {
		p.session.ExportOptions.Dither = b
	})

	p.exportBtn = widget.NewButton("Export Animated GIF...", func() {
		p.PromptExport()
	})
	p.exportBtn.Importance = widget.HighImportance

	form := container.NewVBox(
		header,
		widget.NewLabel("Export Preset:"),
		p.presetSelect,
		p.customResContainer,
		widget.NewSeparator(),
		widget.NewLabel("Max Colors:"),
		p.colorsSelect,
		p.customColorsRow,
		p.ditherCheck,
		widget.NewSeparator(),
		p.twitchStatusLabel,
		p.exportBtn,
	)

	p.container = container.NewVScroll(form)
	return p
}

// Container returns the UI container for the export panel.
func (p *RightExportPanel) Container() *container.Scroll {
	return p.container
}

func (p *RightExportPanel) syncOptions() {
	p.validateTwitch()
	if p.onOptionsChange != nil {
		p.onOptionsChange()
	}
}

func formatEstimatedSize(b int64) string {
	if b < 1024*1024 {
		kb := float64(b) / 1024.0
		return fmt.Sprintf("~%0.0f KB", kb)
	}
	mb := float64(b) / (1024.0 * 1024.0)
	return fmt.Sprintf("~%0.1f MB", mb)
}

func (p *RightExportPanel) validateTwitch() {
	if p.twitchStatusLabel == nil {
		return
	}
	if p.session == nil || p.session.Document == nil || len(p.session.RenderedFrames) == 0 {
		p.twitchStatusLabel.SetText("Twitch Status: No frames loaded.")
		if p.exportBtn != nil {
			p.exportBtn.SetText("Export Animated GIF...")
		}
		return
	}

	frames := len(p.session.RenderedFrames)
	boundW, boundH := p.session.GetActiveBoundaryDimensions()
	w := int(boundW)
	h := int(boundH)
	if p.session.ExportOptions.ExportSquare {
		maxSide := w
		if h > maxSide {
			maxSide = h
		}
		if p.session.ExportOptions.SquareSize > 0 {
			maxSide = p.session.ExportOptions.SquareSize
		}
		w, h = maxSide, maxSide
	}

	totalDurMs := 0
	for _, f := range p.session.RenderedFrames {
		totalDurMs += f.DurationMs
	}

	// Realistic GIF compression estimation:
	// Active artwork pixels compress to ~0.08 bytes/px with LZW; transparent padding compresses to ~0.005 bytes/px.
	activePixels := boundW * boundH
	totalPixels := float64(w * h)
	if activePixels > totalPixels {
		activePixels = totalPixels
	}
	paddingPixels := totalPixels - activePixels

	perFrameBytes := 600.0 + (activePixels * 0.08) + (paddingPixels * 0.005)
	estBytes := int64(float64(frames) * perFrameBytes)

	// Adjust estimate based on palette size (e.g. 32 colors is ~50% size of 256 colors)
	numColors := p.session.ExportOptions.NumColors
	if numColors > 0 && numColors < 256 {
		colorRatio := float64(numColors) / 256.0
		scaleFactor := 0.4 + 0.6*colorRatio
		estBytes = int64(float64(estBytes) * scaleFactor)
	}
	if estBytes < 1024 {
		estBytes = 1024
	}

	if p.exportBtn != nil {
		p.exportBtn.SetText(fmt.Sprintf("Export Animated GIF (%s)...", formatEstimatedSize(estBytes)))
	}

	res := gif.ValidateTwitchEmote(frames, totalDurMs, w, h, estBytes)
	if res.IsValid && len(res.Warnings) == 0 {
		p.twitchStatusLabel.SetText(fmt.Sprintf("Twitch: %dx%d - %d frames - %0.1fs", w, h, frames, float64(totalDurMs)/1000.0))
	} else {
		var parts []string
		parts = append(parts, res.Errors...)
		parts = append(parts, res.Warnings...)
		p.twitchStatusLabel.SetText("Twitch: " + strings.Join(parts, "; "))
	}
}
