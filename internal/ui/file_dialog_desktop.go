//go:build !wasm && !js

package ui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"github.com/ncruces/zenity"
)

func (mw *MainWindow) initPlatform() {}

func (mw *MainWindow) promptSaveSVG() {
	mw.promptSaveSVGWithCallback(nil)
}

func (mw *MainWindow) promptSaveSVGWithCallback(onSaved func()) {
	if mw.editorState == nil {
		return
	}
	if mw.currentPath != "" {
		data := mw.editorState.ComputePatchedSVG()
		if err := os.WriteFile(mw.currentPath, data, 0644); err != nil {
			dialog.ShowError(err, mw.window)
			return
		}
		mw.editorState.MarkClean()
		mw.saveSidecarControlFile()
		mw.updateStatus()
		mw.updateFileLabel()
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
	if mw.editorState == nil {
		return
	}
	go func() {
		defaultName := mw.currentFilename
		if defaultName == "" {
			defaultName = "animated.svg"
		}
		filename, err := zenity.SelectFileSave(
			zenity.Title("Save Inkscape SVG"),
			zenity.Filename(defaultName),
			zenity.FileFilter{
				Name:     "Inkscape SVG (*.svg)",
				Patterns: []string{"*.svg"},
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
		data := mw.editorState.ComputePatchedSVG()
		if err := os.WriteFile(filename, data, 0644); err != nil {
			fyne.Do(func() {
				dialog.ShowError(err, mw.window)
			})
			return
		}
		fyne.Do(func() {
			mw.currentPath = filename
			mw.currentFilename = filepath.Base(filename)
			mw.editorState.InputPath = filename
			mw.editorState.MarkClean()
			mw.saveSidecarControlFile()
			mw.fileWatcher.watchFile(filename)
			mw.showReloadButton(mw.reloadBtn)
			mw.updateStatus()
			mw.updateFileLabel()
			if onSaved != nil {
				onSaved()
			}
		})
	}()
}

func (mw *MainWindow) promptOpenFile() {
	go func() {
		filename, err := zenity.SelectFile(
			zenity.Title("Open Inkscape SVG or InkAnim Project"),
			zenity.FileFilter{
				Name:     "InkAnim & Inkscape Files (*.svg, *.inka)",
				Patterns: []string{"*.svg", "*.inka"},
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
