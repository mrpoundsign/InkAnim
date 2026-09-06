package ui

import (
	"fmt"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"inkanim/internal/app"
	"inkanim/internal/svg"
)

// LeftFramesPanel builds the frame management sidebar.
type LeftFramesPanel struct {
	session           *app.Session
	container         *fyne.Container
	listContainer     *fyne.Container
	modeRadio         *widget.RadioGroup
	speedPresetSelect *widget.Select
	globalMsEntry     *widget.Entry
	onFramesChange    func()
	isUpdating        bool
}

// NewLeftFramesPanel creates the left sidebar widget.
func NewLeftFramesPanel(sess *app.Session, onFramesChange func()) *LeftFramesPanel {
	p := &LeftFramesPanel{
		session:        sess,
		listContainer:  container.NewVBox(),
		onFramesChange: onFramesChange,
	}

	p.modeRadio = widget.NewRadioGroup([]string{"Layers", "Pages"}, func(selected string) {
		if p.isUpdating {
			return
		}
		if selected == "Pages" {
			_ = p.session.SetMode(svg.ModePages)
		} else {
			_ = p.session.SetMode(svg.ModeLayers)
		}
		p.Refresh()
		if p.onFramesChange != nil {
			p.onFramesChange()
		}
	})
	p.modeRadio.Selected = "Layers"

	header := widget.NewLabelWithStyle("Animation Frames", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	modeLabel := widget.NewLabel("Frame Source:")

	// Global Speed Master Bar
	speedLabel := widget.NewLabelWithStyle("Global Speed:", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	p.globalMsEntry = widget.NewEntry()
	p.globalMsEntry.SetText(fmt.Sprintf("%d", sess.ExportOptions.DefaultDurationMs))

	p.speedPresetSelect = widget.NewSelect([]string{
		"10 FPS (100ms)",
		"12 FPS (83ms)",
		"15 FPS (66ms)",
		"20 FPS (50ms)",
		"24 FPS (41ms)",
		"30 FPS (33ms)",
		"Custom",
	}, func(selected string) {
		if p.isUpdating {
			return
		}
		ms := 0
		switch selected {
		case "10 FPS (100ms)":
			ms = 100
		case "12 FPS (83ms)":
			ms = 83
		case "15 FPS (66ms)":
			ms = 66
		case "20 FPS (50ms)":
			ms = 50
		case "24 FPS (41ms)":
			ms = 41
		case "30 FPS (33ms)":
			ms = 33
		}
		if ms > 0 {
			p.isUpdating = true
			p.globalMsEntry.SetText(fmt.Sprintf("%d", ms))
			p.session.SetGlobalDuration(ms)
			p.isUpdating = false
			p.Refresh()
			if p.onFramesChange != nil {
				p.onFramesChange()
			}
		}
	})
	p.speedPresetSelect.SetSelected("10 FPS (100ms)")

	p.globalMsEntry.OnChanged = func(val string) {
		if p.isUpdating {
			return
		}
		if ms, err := strconv.Atoi(val); err == nil && ms > 0 {
			p.isUpdating = true
			p.speedPresetSelect.SetSelected("Custom")
			p.session.SetGlobalDuration(ms)
			p.isUpdating = false
			p.Refresh()
			if p.onFramesChange != nil {
				p.onFramesChange()
			}
		}
	}

	globalSpeedRow := container.NewBorder(
		nil, nil,
		widget.NewLabel("ms:"), nil,
		p.globalMsEntry,
	)

	topControls := container.NewVBox(
		header,
		modeLabel,
		p.modeRadio,
		widget.NewSeparator(),
		speedLabel,
		p.speedPresetSelect,
		globalSpeedRow,
		widget.NewSeparator(),
	)

	scroll := container.NewVScroll(p.listContainer)
	p.container = container.NewBorder(topControls, nil, nil, nil, scroll)

	p.Refresh()
	return p
}

// Container returns the root container for the left panel.
func (p *LeftFramesPanel) Container() *fyne.Container {
	return p.container
}

// Refresh updates the frame list based on the session state.
func (p *LeftFramesPanel) Refresh() {
	if p.isUpdating {
		return
	}
	p.isUpdating = true
	defer func() {
		p.isUpdating = false
	}()

	p.listContainer.Objects = nil

	if p.session.Document == nil {
		p.listContainer.Add(widget.NewLabel("No SVG loaded.\nUse 'Open SVG' above."))
		p.listContainer.Refresh()
		return
	}

	if p.session.CurrentMode == svg.ModePages {
		if p.modeRadio.Selected != "Pages" {
			p.modeRadio.SetSelected("Pages")
		}
		if len(p.session.Pages) == 0 {
			p.listContainer.Add(widget.NewLabel("No Inkscape pages found.\nTry Layers mode."))
			p.listContainer.Refresh()
			return
		}

		for i, page := range p.session.Pages {
			idx := i
			activeCheck := widget.NewCheck(fmt.Sprintf("%d. %s", idx+1, page.Label), func(checked bool) {
				p.session.Pages[idx].IsActive = checked
				_ = p.session.RerenderAllFrames()
				if p.onFramesChange != nil {
					p.onFramesChange()
				}
			})
			activeCheck.Checked = page.IsActive

			durEntry := widget.NewEntry()
			durVal := p.session.ExportOptions.DefaultDurationMs
			if page.HasOverride && page.OverrideMs > 0 {
				durVal = page.OverrideMs
			}
			durEntry.SetText(fmt.Sprintf("%d", durVal))
			if !page.HasOverride {
				durEntry.Disable()
			}

			durEntry.OnChanged = func(val string) {
				if ms, err := strconv.Atoi(val); err == nil && ms > 0 {
					p.session.SetFrameOverride(idx, true, ms)
					if p.onFramesChange != nil {
						p.onFramesChange()
					}
				}
			}

			overrideCheck := widget.NewCheck("Override", func(checked bool) {
				if checked {
					durEntry.Enable()
					ms := p.session.ExportOptions.DefaultDurationMs
					if parsed, err := strconv.Atoi(durEntry.Text); err == nil && parsed > 0 {
						ms = parsed
					}
					p.session.SetFrameOverride(idx, true, ms)
				} else {
					durEntry.Disable()
					durEntry.SetText(fmt.Sprintf("%d", p.session.ExportOptions.DefaultDurationMs))
					p.session.SetFrameOverride(idx, false, 0)
				}
				if p.onFramesChange != nil {
					p.onFramesChange()
				}
			})
			overrideCheck.Checked = page.HasOverride

			bottomRow := container.NewHBox(
				overrideCheck,
				widget.NewLabel("ms:"),
				container.NewGridWrap(fyne.NewSize(50, 30), durEntry),
			)
			card := container.NewVBox(
				activeCheck,
				bottomRow,
				widget.NewSeparator(),
			)
			p.listContainer.Add(card)
		}
	} else {
		if p.modeRadio.Selected != "Layers" {
			p.modeRadio.SetSelected("Layers")
		}
		if len(p.session.Layers) == 0 {
			p.listContainer.Add(widget.NewLabel("No Inkscape layers found."))
			p.listContainer.Refresh()
			return
		}

		for i, layer := range p.session.Layers {
			idx := i
			activeCheck := widget.NewCheck(fmt.Sprintf("%d. %s", idx+1, layer.Label), func(checked bool) {
				_ = p.session.ToggleLayerActive(idx)
				p.Refresh()
				if p.onFramesChange != nil {
					p.onFramesChange()
				}
			})
			activeCheck.Checked = layer.IsActive

			pinCheck := widget.NewCheck("Pin BG", func(checked bool) {
				_ = p.session.ToggleLayerPinned(idx)
				p.Refresh()
				if p.onFramesChange != nil {
					p.onFramesChange()
				}
			})
			pinCheck.Checked = layer.IsPinned

			durEntry := widget.NewEntry()
			durVal := p.session.ExportOptions.DefaultDurationMs
			if layer.HasOverride && layer.OverrideMs > 0 {
				durVal = layer.OverrideMs
			}
			durEntry.SetText(fmt.Sprintf("%d", durVal))
			if !layer.HasOverride {
				durEntry.Disable()
			}

			durEntry.OnChanged = func(val string) {
				if ms, err := strconv.Atoi(val); err == nil && ms > 0 {
					p.session.SetFrameOverride(idx, true, ms)
					if p.onFramesChange != nil {
						p.onFramesChange()
					}
				}
			}

			overrideCheck := widget.NewCheck("Override", func(checked bool) {
				if checked {
					durEntry.Enable()
					ms := p.session.ExportOptions.DefaultDurationMs
					if parsed, err := strconv.Atoi(durEntry.Text); err == nil && parsed > 0 {
						ms = parsed
					}
					p.session.SetFrameOverride(idx, true, ms)
				} else {
					durEntry.Disable()
					durEntry.SetText(fmt.Sprintf("%d", p.session.ExportOptions.DefaultDurationMs))
					p.session.SetFrameOverride(idx, false, 0)
				}
				if p.onFramesChange != nil {
					p.onFramesChange()
				}
			})
			overrideCheck.Checked = layer.HasOverride

			upBtn := widget.NewButton("▲", func() {
				if idx > 0 {
					_ = p.session.MoveLayer(idx, idx-1)
					p.Refresh()
					if p.onFramesChange != nil {
						p.onFramesChange()
					}
				}
			})
			downBtn := widget.NewButton("▼", func() {
				if idx < len(p.session.Layers)-1 {
					_ = p.session.MoveLayer(idx, idx+1)
					p.Refresh()
					if p.onFramesChange != nil {
						p.onFramesChange()
					}
				}
			})

			topRow := container.NewHBox(
				activeCheck,
				pinCheck,
			)
			bottomRow := container.NewHBox(
				container.NewGridWrap(fyne.NewSize(28, 28), upBtn),
				container.NewGridWrap(fyne.NewSize(28, 28), downBtn),
				overrideCheck,
				widget.NewLabel("ms:"),
				container.NewGridWrap(fyne.NewSize(50, 30), durEntry),
			)
			card := container.NewVBox(
				topRow,
				bottomRow,
				widget.NewSeparator(),
			)
			p.listContainer.Add(card)
		}
	}

	p.listContainer.Refresh()
}
