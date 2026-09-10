//go:build wasm || js

package ui

import "fyne.io/fyne/v2/widget"

type fileWatcher struct {
	mw *MainWindow
}

func newFileWatcher(mw *MainWindow) *fileWatcher {
	return &fileWatcher{mw: mw}
}

func (fw *fileWatcher) watchFile(path string) {
	// No-op for WASM
}

func (mw *MainWindow) showReloadButton(btn *widget.Button) {
	// Reload button is not supported in WASM due to sandbox restrictions
	btn.Hide()
}
