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

	presetSelect  *widget.Select
	squareCheck   *widget.Check
	squareSizeEnt *widget.Entry
	widthEntry    *widget.Entry
	heightEntry   *widget.Entry
	colorsSelect  *widget.Select
	ditherCheck   *widget.Check

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

	p.squareCheck = widget.NewCheck("Export Square (Centers on max side)", func(checked bool) {
		p.session.ExportOptions.ExportSquare = checked
		if checked {
			p.squareSizeEnt.Enable()
			p.widthEntry.Disable()
			p.heightEntry.Disable()
		} else {
			p.squareSizeEnt.Disable()
			p.widthEntry.Enable()
			p.heightEntry.Enable()
		}
		p.syncOptions()
	})
	p.squareCheck.Checked = true

	p.squareSizeEnt = widget.NewEntry()
	p.squareSizeEnt.SetPlaceHolder("Resolution (e.g. 512, max 4096)")
	p.squareSizeEnt.SetText("512")
	p.squareSizeEnt.OnChanged = func(s string) {
		p.syncOptions()
	}

	p.widthEntry = widget.NewEntry()
	p.widthEntry.SetPlaceHolder("Width (px)")
	p.widthEntry.Disable()
	p.widthEntry.OnChanged = func(s string) {
		p.syncOptions()
	}

	p.heightEntry = widget.NewEntry()
	p.heightEntry.SetPlaceHolder("Height (px)")
	p.heightEntry.Disable()
	p.heightEntry.OnChanged = func(s string) {
		p.syncOptions()
	}

	// Preset Dropdown
	p.presetSelect = widget.NewSelect([]string{
		"Twitch Emote (Square, Max 4096)",
		"Discord Emote (128x128)",
		"Custom Dimensions",
	}, func(selected string) {
		switch selected {
		case "Twitch Emote (Square, Max 4096)":
			p.squareCheck.SetChecked(true)
			p.squareSizeEnt.SetText("512")
			p.squareSizeEnt.Enable()
			p.widthEntry.Disable()
			p.heightEntry.Disable()
		case "Discord Emote (128x128)":
			p.squareCheck.SetChecked(true)
			p.squareSizeEnt.SetText("128")
			p.squareSizeEnt.Disable()
			p.widthEntry.Disable()
			p.heightEntry.Disable()
		case "Custom Dimensions":
			p.squareCheck.SetChecked(false)
			p.squareSizeEnt.Disable()
			p.widthEntry.Enable()
			p.heightEntry.Enable()
		}
		p.syncOptions()
	})
	p.presetSelect.Selected = "Twitch Emote (Square, Max 4096)"

	// Colors
	p.colorsSelect = widget.NewSelect([]string{"256", "128", "64", "32"}, func(s string) {
		if c, err := strconv.Atoi(s); err == nil {
			p.session.ExportOptions.NumColors = c
			p.syncOptions()
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
		widget.NewLabel("Preset:"),
		p.presetSelect,
		widget.NewSeparator(),
		p.squareCheck,
		widget.NewLabel("Square Target Size (px):"),
		p.squareSizeEnt,
		widget.NewLabel("Custom Dimensions:"),
		container.NewGridWithColumns(2, p.widthEntry, p.heightEntry),
		widget.NewSeparator(),
		widget.NewLabel("Max Colors:"),
		p.colorsSelect,
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
	if p.squareCheck == nil || p.squareSizeEnt == nil || p.widthEntry == nil || p.heightEntry == nil {
		return
	}

	opts := &p.session.ExportOptions
	opts.ExportSquare = p.squareCheck.Checked

	if sz, err := strconv.Atoi(p.squareSizeEnt.Text); err == nil && sz > 0 {
		if sz > 4096 {
			sz = 4096
		}
		opts.SquareSize = sz
	} else {
		opts.SquareSize = 0
	}

	if w, err := strconv.Atoi(p.widthEntry.Text); err == nil {
		opts.TargetWidth = w
	}
	if h, err := strconv.Atoi(p.heightEntry.Text); err == nil {
		opts.TargetHeight = h
	}

	p.validateTwitch()
	if p.onOptionsChange != nil {
		p.onOptionsChange()
	}
}

func (p *RightExportPanel) validateTwitch() {
	if p.twitchStatusLabel == nil {
		return
	}
	if p.session == nil || p.session.Document == nil || len(p.session.RenderedFrames) == 0 {
		p.twitchStatusLabel.SetText("Twitch Status: No frames loaded.")
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

	// Rough estimation for GIF size: ~1.2 KB per frame at 112x112, scale quadratically
	estBytes := int64(float64(frames*1500) * (float64(w*h) / (112.0 * 112.0 * 8.0)))
	if estBytes < 50000 {
		estBytes = 50000
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
