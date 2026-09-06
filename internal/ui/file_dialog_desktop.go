//go:build !wasm && !js

package ui

import (
	"fmt"
	"io"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
)

func (mw *MainWindow) initPlatform() {}

func (mw *MainWindow) promptOpenFile() {
	fileDialog := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil || reader == nil {
			return
		}
		defer reader.Close()

		data, readErr := io.ReadAll(reader)
		if readErr != nil {
			dialog.ShowError(readErr, mw.window)
			return
		}
		name := reader.URI().Name()
		if name == "" {
			name = filepath.Base(reader.URI().Path())
		}
		mw.loadData(data, name)
	}, mw.window)

	fileDialog.SetFilter(storage.NewExtensionFileFilter([]string{".svg"}))
	fileDialog.Show()
}

// PromptExport opens a file save dialog and exports the animation.
func (p *RightExportPanel) PromptExport() {
	if len(p.session.RenderedFrames) == 0 {
		dialog.ShowInformation("No Frames", "Please load an SVG with animation frames before exporting.", p.parentWindow)
		return
	}

	saveDialog := dialog.NewFileSave(func(writer fyne.URIWriteCloser, err error) {
		if err != nil || writer == nil {
			return
		}

		p.progressBar.Show()
		p.exportBtn.Disable()

		go func(w fyne.URIWriteCloser, destURI fyne.URI) {
			defer w.Close()
			sizeBytes, expErr := p.session.ExportGIFWriter(w)

			fyne.Do(func() {
				p.progressBar.Hide()
				p.exportBtn.Enable()

				if expErr != nil {
					dialog.ShowError(expErr, p.parentWindow)
				} else {
					dialog.ShowInformation("Export Succeeded",
						fmt.Sprintf("Successfully exported single animated GIF:\n%s\n\nFile Size: %0.2f KB",
							destURI.Name(), float64(sizeBytes)/1024.0),
						p.parentWindow)
				}
			})
		}(writer, writer.URI())
	}, p.parentWindow)

	saveDialog.SetFilter(nil)
	saveDialog.SetFileName("emote.gif")
	saveDialog.Show()
}
