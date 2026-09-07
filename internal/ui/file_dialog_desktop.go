//go:build !wasm && !js

package ui

import (
	"errors"
	"fmt"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"github.com/ncruces/zenity"
)

func (mw *MainWindow) initPlatform() {}

func (mw *MainWindow) promptOpenFile() {
	go func() {
		filename, err := zenity.SelectFile(
			zenity.Title("Open Inkscape SVG"),
			zenity.FileFilter{
				Name:     "Inkscape SVG (*.svg)",
				Patterns: []string{"*.svg"},
			},
		)
		if err != nil {
			if errors.Is(err, zenity.ErrCanceled) {
				return
			}
			fyne.Do(func() {
				dialog.ShowError(err, mw.window)
			})
			return
		}

		if filename != "" {
			fyne.Do(func() {
				mw.loadFilePath(filename)
			})
		}
	}()
}

// PromptExport opens a native OS file save dialog and exports the animation.
func (p *RightExportPanel) PromptExport() {
	var resume func()
	if p.pausePlayback != nil {
		resume = p.pausePlayback()
	} else {
		resume = func() {}
	}

	if len(p.session.RenderedFrames) == 0 {
		d := dialog.NewInformation("No Frames", "Please load an SVG with animation frames before exporting.", p.parentWindow)
		d.SetOnClosed(resume)
		d.Show()
		return
	}

	go func() {
		filename, err := zenity.SelectFileSave(
			zenity.Title("Export Animated GIF"),
			zenity.Filename("emote.gif"),
			zenity.FileFilter{
				Name:     "GIF Animation (*.gif)",
				Patterns: []string{"*.gif"},
			},
			zenity.ConfirmOverwrite(),
		)
		if err != nil {
			if errors.Is(err, zenity.ErrCanceled) {
				fyne.Do(resume)
				return
			}
			fyne.Do(func() {
				d := dialog.NewError(err, p.parentWindow)
				d.SetOnClosed(resume)
				d.Show()
			})
			return
		}

		if filename == "" {
			fyne.Do(resume)
			return
		}

		fyne.Do(func() {
			p.exportBtn.SetText("Exporting...")
			p.exportBtn.Disable()
		})

		sizeBytes, expErr := p.session.ExportGIF(filename)

		fyne.Do(func() {
			p.exportBtn.SetText("Export Animated GIF...")
			p.exportBtn.Enable()

			if expErr != nil {
				d := dialog.NewError(expErr, p.parentWindow)
				d.SetOnClosed(resume)
				d.Show()
			} else {
				d := dialog.NewInformation("Export Succeeded",
					fmt.Sprintf("Successfully exported single animated GIF:\n%s\n\nFile Size: %0.2f KB",
						filepath.Base(filename), float64(sizeBytes)/1024.0),
					p.parentWindow)
				d.SetOnClosed(resume)
				d.Show()
			}
		})
	}()
}
