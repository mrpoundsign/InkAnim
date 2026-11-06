//go:build !wasm && !js

package ui

import (
	"log"

	"fyne.io/fyne/v2/widget"
	"github.com/fsnotify/fsnotify"
)

type fileWatcher struct {
	watcher *fsnotify.Watcher
	mw      *MainWindow
}

func newFileWatcher(mw *MainWindow) *fileWatcher {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		log.Println("Error creating fsnotify watcher:", err)
		return &fileWatcher{mw: mw}
	}
	fw := &fileWatcher{watcher: w, mw: mw}
	go fw.watch()
	return fw
}

func (fw *fileWatcher) watch() {
	if fw.watcher == nil {
		return
	}
	for {
		select {
		case event, ok := <-fw.watcher.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Write) {
				// File was saved, trigger reload
				dispatchUI(func() {
					fw.mw.Reload()
				})
			}
		case err, ok := <-fw.watcher.Errors:
			if !ok {
				return
			}
			log.Println("fsnotify error:", err)
		}
	}
}

func (fw *fileWatcher) watchFile(path string) {
	if path == "" {
		fw.watchFiles(nil)
		return
	}
	fw.watchFiles([]string{path})
}

func (fw *fileWatcher) watchFiles(paths []string) {
	if fw.watcher == nil {
		return
	}
	for _, w := range fw.watcher.WatchList() {
		_ = fw.watcher.Remove(w)
	}
	for _, p := range paths {
		if p == "" {
			continue
		}
		if err := fw.watcher.Add(p); err != nil {
			log.Println("Error adding file to watcher:", err)
		}
	}
}

func (fw *fileWatcher) close() {
	if fw != nil && fw.watcher != nil {
		_ = fw.watcher.Close()
	}
}

func (mw *MainWindow) showReloadButton(btn *widget.Button) {
	btn.Show()
}
