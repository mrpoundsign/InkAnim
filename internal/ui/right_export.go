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
	"inkanim/pkg/inksvg"
)

// RightExportPanel manages dimension inputs, Twitch presets, and single animated GIF export.
type RightExportPanel struct {
	session      *app.Session
	container    *container.Scroll
	parentWindow fyne.Window

	cropBoundaryRadio *widget.RadioGroup
	cropPageSelect    *widget.Select
	isUpdating        bool

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
	onExportSuccess func(outputPath string)
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
	p.twitchStatusLabel.Wrapping = fyne.TextWrapWord

	p.cropBoundaryRadio = widget.NewRadioGroup([]string{"Drawing", "Page"}, func(selected string) {
		if p.isUpdating || p.session == nil {
			return
		}
		if selected == "Page" {
			_ = p.session.SetCropBoundary(inksvg.BoundaryPage, p.session.CropPageIndex)
		} else {
			_ = p.session.SetCropBoundary(inksvg.BoundaryDrawing, p.session.CropPageIndex)
		}
		p.Refresh()
		if p.onOptionsChange != nil {
			p.onOptionsChange()
		}
	})
	p.cropBoundaryRadio.Selected = "Page"

	p.cropPageSelect = widget.NewSelect([]string{"(No document loaded)"}, func(selected string) {
		if p.isUpdating || p.session == nil || p.session.Document == nil {
			return
		}
		pageIdx := 0
		docRect := p.session.Document.GetDocumentRect()
		docLabel := fmt.Sprintf("Document (%0.0fx%0.0f)", docRect.Width, docRect.Height)
		if selected == docLabel {
			pageIdx = 0
		} else {
			for i, pg := range p.session.Pages {
				optLabel := fmt.Sprintf("%d. %s (%0.0fx%0.0f)", i+1, pg.Label, pg.Width, pg.Height)
				if optLabel == selected {
					pageIdx = i + 1
					break
				}
			}
		}
		_ = p.session.SetCropBoundary(p.session.CropBoundaryMode, pageIdx)
		p.Refresh()
		if p.onOptionsChange != nil {
			p.onOptionsChange()
		}
	})

	p.squareSizeInput = NewNumericCommitInput(512, 16, 4096, "Resolution (px):", func(val int) {
		p.session.ExportOptions.SquareSize = val
		p.validateTwitch()
		if p.onOptionsChange != nil {
			p.onOptionsChange()
		}
	})

	p.widthInput = NewNumericCommitInput(512, 16, 4096, "W:", func(val int) {
		p.session.ExportOptions.TargetWidth = val
		p.validateTwitch()
		if p.onOptionsChange != nil {
			p.onOptionsChange()
		}
	})

	p.heightInput = NewNumericCommitInput(512, 16, 4096, "H:", func(val int) {
		p.session.ExportOptions.TargetHeight = val
		p.validateTwitch()
		if p.onOptionsChange != nil {
			p.onOptionsChange()
		}
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
		if p.onOptionsChange != nil {
			p.onOptionsChange()
		}
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
		if p.onOptionsChange != nil {
			p.onOptionsChange()
		}
	})
	p.colorsSelect.Selected = "256"

	p.ditherCheck = widget.NewCheck("Dithering (Bayer 8x8)", func(b bool) {
		p.session.ExportOptions.Dither = b
		p.validateTwitch()
		if p.onOptionsChange != nil {
			p.onOptionsChange()
		}
	})

	p.exportBtn = widget.NewButton("Export Animated GIF...", func() {
		p.PromptExport()
	})
	p.exportBtn.Importance = widget.HighImportance

	form := container.NewVBox(
		header,
		widget.NewLabel("Crop Boundary:"),
		p.cropBoundaryRadio,
		p.cropPageSelect,
		widget.NewSeparator(),
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
	p.Refresh()
	return p
}

// Container returns the UI container for the export panel.
func (p *RightExportPanel) Container() *container.Scroll {
	return p.container
}

// Refresh re-validates export specifications and updates status.
func (p *RightExportPanel) Refresh() {
	if !p.isUpdating {
		p.isUpdating = true
		p.syncCropControls()
		p.isUpdating = false
	}
	p.validateTwitch()
}

func (p *RightExportPanel) syncCropControls() {
	if p.cropBoundaryRadio == nil || p.cropPageSelect == nil {
		return
	}
	if p.session == nil || p.session.Document == nil {
		p.cropPageSelect.Options = []string{"(No document loaded)"}
		p.cropPageSelect.SetSelected("(No document loaded)")
		p.cropPageSelect.Disable()
		return
	}
	if p.session.CropBoundaryMode == inksvg.BoundaryPage {
		if p.cropBoundaryRadio.Selected != "Page" {
			p.cropBoundaryRadio.SetSelected("Page")
		}
	} else {
		if p.cropBoundaryRadio.Selected != "Drawing" {
			p.cropBoundaryRadio.SetSelected("Drawing")
		}
	}
	docRect := p.session.Document.GetDocumentRect()
	var pageOptions []string
	pageOptions = append(pageOptions, fmt.Sprintf("Document (%0.0fx%0.0f)", docRect.Width, docRect.Height))
	for i, pg := range p.session.Pages {
		pageOptions = append(pageOptions, fmt.Sprintf("%d. %s (%0.0fx%0.0f)", i+1, pg.Label, pg.Width, pg.Height))
	}
	p.cropPageSelect.Options = pageOptions
	selectedIdx := p.session.CropPageIndex
	if selectedIdx < 0 || selectedIdx >= len(pageOptions) {
		selectedIdx = 0
	}
	p.cropPageSelect.SetSelected(pageOptions[selectedIdx])
	if p.session.CropBoundaryMode == inksvg.BoundaryPage {
		p.cropPageSelect.Enable()
	} else {
		p.cropPageSelect.Disable()
	}
}

// SetOnExportSuccess registers a callback invoked when an animated GIF is successfully exported.
func (p *RightExportPanel) SetOnExportSuccess(fn func(outputPath string)) {
	p.onExportSuccess = fn
}

// PopulateControlFile writes the export panel's active preset and settings into ctrl.
func (p *RightExportPanel) PopulateControlFile(ctrl *app.ControlFile) {
	if p == nil || ctrl == nil {
		return
	}
	if p.presetSelect != nil {
		ctrl.Export.Preset = p.presetSelect.Selected
	}
}

// ApplyControlFileUI synchronizes the export panel's UI widgets with a loaded ControlFile.
func (p *RightExportPanel) ApplyControlFileUI(ctrl *app.ControlFile) {
	if p == nil || ctrl == nil {
		return
	}
	if p.squareSizeInput != nil && ctrl.Export.SquareSize >= 16 {
		p.squareSizeInput.SetValue(ctrl.Export.SquareSize)
	}
	if p.widthInput != nil && ctrl.Export.TargetWidth >= 16 {
		p.widthInput.SetValue(ctrl.Export.TargetWidth)
	}
	if p.heightInput != nil && ctrl.Export.TargetHeight >= 16 {
		p.heightInput.SetValue(ctrl.Export.TargetHeight)
	}
	if p.squareCheck != nil {
		p.squareCheck.SetChecked(ctrl.Export.ExportSquare)
	}
	if p.presetSelect != nil {
		preset := ctrl.Export.Preset
		if preset == "" {
			switch {
			case ctrl.Export.ExportSquare && ctrl.Export.SquareSize == 512:
				preset = "Twitch Emote (512x512 Square)"
			case ctrl.Export.ExportSquare && ctrl.Export.SquareSize == 128:
				preset = "Discord Emote (128x128 Square)"
			default:
				preset = "Custom Dimensions"
			}
		}
		p.presetSelect.SetSelected(preset)
	}
	if p.colorsSelect != nil && ctrl.Export.NumColors >= 2 {
		switch ctrl.Export.NumColors {
		case 256, 128, 64, 32:
			p.colorsSelect.SetSelected(strconv.Itoa(ctrl.Export.NumColors))
		default:
			if p.customColorsInput != nil {
				p.customColorsInput.SetValue(ctrl.Export.NumColors)
			}
			p.colorsSelect.SetSelected("Custom")
		}
	}
	if p.ditherCheck != nil {
		p.ditherCheck.SetChecked(ctrl.Export.Dither)
	}
	p.Refresh()
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
	dims := p.session.ResolveExportDimensions()
	w, h := dims.OutWidth, dims.OutHeight

	totalDurMs := 0
	if p.session.ExportOptions.PingPong && frames >= 3 {
		for _, f := range p.session.RenderedFrames {
			totalDurMs += f.DurationMs
		}
		for i := frames - 2; i >= 1; i-- {
			totalDurMs += p.session.RenderedFrames[i].DurationMs
		}
		frames = frames*2 - 2
	} else {
		for _, f := range p.session.RenderedFrames {
			totalDurMs += f.DurationMs
		}
	}

	if p.exportBtn != nil {
		p.exportBtn.SetText("Export Animated GIF...")
	}

	res := gif.ValidateTwitchEmote(frames, totalDurMs, w, h)
	if res.IsValid && len(res.Warnings) == 0 {
		p.twitchStatusLabel.SetText(fmt.Sprintf("Twitch: %dx%d - %d frames - %0.1fs", w, h, frames, float64(totalDurMs)/1000.0))
	} else {
		parts := make([]string, 0, len(res.Errors)+len(res.Warnings))
		parts = append(parts, res.Errors...)
		parts = append(parts, res.Warnings...)
		p.twitchStatusLabel.SetText("Twitch: " + strings.Join(parts, "; "))
	}
}
