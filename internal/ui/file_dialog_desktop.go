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
	if len(p.session.RenderedFrames) == 0 {
		dialog.ShowInformation("No Frames", "Please load an SVG with animation frames before exporting.", p.parentWindow)
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
				return
			}
			fyne.Do(func() {
				dialog.ShowError(err, p.parentWindow)
			})
			return
		}

		if filename == "" {
			return
		}

		fyne.Do(func() {
			p.progressBar.Show()
			p.exportBtn.Disable()
		})

		sizeBytes, expErr := p.session.ExportGIF(filename)

		fyne.Do(func() {
			p.progressBar.Hide()
			p.exportBtn.Enable()

			if expErr != nil {
				dialog.ShowError(expErr, p.parentWindow)
			} else {
				dialog.ShowInformation("Export Succeeded",
					fmt.Sprintf("Successfully exported single animated GIF:\n%s\n\nFile Size: %0.2f KB",
						filepath.Base(filename), float64(sizeBytes)/1024.0),
					p.parentWindow)
			}
		})
	}()
}
