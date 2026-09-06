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
	session        *app.Session
	container      *fyne.Container
	listContainer  *fyne.Container
	modeRadio      *widget.RadioGroup
	onFramesChange func()
}

// NewLeftFramesPanel creates the left sidebar widget.
func NewLeftFramesPanel(sess *app.Session, onFramesChange func()) *LeftFramesPanel {
	p := &LeftFramesPanel{
		session:        sess,
		listContainer:  container.NewVBox(),
		onFramesChange: onFramesChange,
	}

	p.modeRadio = widget.NewRadioGroup([]string{"Layers", "Pages"}, func(selected string) {
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

	topControls := container.NewVBox(
		header,
		modeLabel,
		p.modeRadio,
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
	p.listContainer.Objects = nil

	if p.session.Document == nil {
		p.listContainer.Add(widget.NewLabel("No SVG loaded.\nUse 'Open SVG' above."))
		p.listContainer.Refresh()
		return
	}

	if p.session.CurrentMode == svg.ModePages {
		p.modeRadio.SetSelected("Pages")
		if len(p.session.Pages) == 0 {
			p.listContainer.Add(widget.NewLabel("No Inkscape pages found.\nTry Layers mode."))
			p.listContainer.Refresh()
			return
		}

		for i, page := range p.session.Pages {
			idx := i
			activeCheck := widget.NewCheck("", func(checked bool) {
				p.session.Pages[idx].IsActive = checked
				_ = p.session.RerenderAllFrames()
				if p.onFramesChange != nil {
					p.onFramesChange()
				}
			})
			activeCheck.Checked = page.IsActive

			nameLabel := widget.NewLabel(fmt.Sprintf("%d. %s", idx+1, page.Label))

			durEntry := widget.NewEntry()
			durEntry.SetText(fmt.Sprintf("%d", page.DurationMs))
			durEntry.OnChanged = func(val string) {
				if ms, err := strconv.Atoi(val); err == nil && ms > 0 {
					p.session.SetFrameDuration(idx, ms)
					if p.onFramesChange != nil {
						p.onFramesChange()
					}
				}
			}

			row := container.NewHBox(
				activeCheck,
				nameLabel,
				widget.NewLabel("ms:"),
				container.NewGridWrap(fyne.NewSize(50, 32), durEntry),
			)
			p.listContainer.Add(row)
		}
	} else {
		p.modeRadio.SetSelected("Layers")
		if len(p.session.Layers) == 0 {
			p.listContainer.Add(widget.NewLabel("No Inkscape layers found."))
			p.listContainer.Refresh()
			return
		}

		for i, layer := range p.session.Layers {
			idx := i
			activeCheck := widget.NewCheck("", func(checked bool) {
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

			nameLabel := widget.NewLabel(fmt.Sprintf("%d. %s", idx+1, layer.Label))

			durEntry := widget.NewEntry()
			durEntry.SetText(fmt.Sprintf("%d", layer.DurationMs))
			durEntry.OnChanged = func(val string) {
				if ms, err := strconv.Atoi(val); err == nil && ms > 0 {
					p.session.SetFrameDuration(idx, ms)
					if p.onFramesChange != nil {
						p.onFramesChange()
					}
				}
			}

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

			row := container.NewHBox(
				activeCheck,
				nameLabel,
				pinCheck,
				widget.NewLabel("ms:"),
				container.NewGridWrap(fyne.NewSize(45, 32), durEntry),
				container.NewGridWrap(fyne.NewSize(30, 32), upBtn),
				container.NewGridWrap(fyne.NewSize(30, 32), downBtn),
			)
			p.listContainer.Add(row)
		}
	}

	p.listContainer.Refresh()
}
