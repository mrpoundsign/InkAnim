package ui

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"github.com/ncruces/zenity"

	"inkanim/internal/app"
)

func (mw *MainWindow) promptSaveSVG() {
	mw.promptSaveSVGWithCallback(nil)
}

func (mw *MainWindow) promptSaveSVGWithCallback(onSaved func()) {
	targetInka := mw.projectPath
	if targetInka == "" && mw.currentPath != "" {
		targetInka = app.SidecarPathForSVG(mw.currentPath)
	}
	if targetInka != "" {
		if err := mw.saveProjectToPath(targetInka); err != nil {
			dialog.ShowError(err, mw.window)
			return
		}
		if onSaved != nil {
			onSaved()
		}
		return
	}
	mw.promptSaveSVGAsWithCallback(onSaved)
}

func (mw *MainWindow) promptSaveSVGAs() {
	mw.promptSaveSVGAsWithCallback(nil)
}

func (mw *MainWindow) promptSaveSVGAsWithCallback(onSaved func()) {
	go func() {
		defaultName := "project.inka"
		if mw.projectPath != "" {
			defaultName = filepath.Base(mw.projectPath)
		} else if mw.currentPath != "" {
			ext := filepath.Ext(mw.currentPath)
			base := filepath.Base(mw.currentPath)
			defaultName = strings.TrimSuffix(base, ext) + app.ControlFileExt
		}
		filename, err := zenity.SelectFileSave(
			zenity.Title("Save InkAnim Project"),
			zenity.Filename(defaultName),
			zenity.FileFilter{
				Name:     "InkAnim Project (*.inka)",
				Patterns: []string{"*.inka"},
			},
			zenity.ConfirmOverwrite(),
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
		if filename == "" {
			return
		}
		fyne.Do(func() {
			if err := mw.saveProjectToPath(filename); err != nil {
				dialog.ShowError(err, mw.window)
				return
			}
			if onSaved != nil {
				onSaved()
			}
		})
	}()
}

func (mw *MainWindow) promptOpenFile() {
	go func() {
		filename, err := zenity.SelectFile(
			zenity.Title("Open InkAnim Project"),
			zenity.FileFilter{
				Name:     "InkAnim Project (*.inka)",
				Patterns: []string{"*.inka"},
			},
			zenity.FileFilter{
				Name:     "All Supported Files (*.inka, *.svg)",
				Patterns: []string{"*.inka", "*.svg"},
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

func (mw *MainWindow) promptAddSVG() {
	go func() {
		filename, err := zenity.SelectFile(
			zenity.Title("Add / Import SVG to Project"),
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
				mw.addSVGInteractive(filename)
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
			p.validateTwitch()
			p.exportBtn.Enable()

			if expErr != nil {
				d := dialog.NewError(expErr, p.parentWindow)
				d.SetOnClosed(resume)
				d.Show()
			} else {
				if p.onExportSuccess != nil {
					p.onExportSuccess(filename)
				}
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
