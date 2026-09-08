package ui

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"inkanim/internal/app"
)

// MainWindow builds and manages the primary Fyne application window.
type MainWindow struct {
	window       fyne.Window
	session      *app.Session
	leftPanel   *LeftFramesPanel
	centerPanel *CenterPreviewPanel
	rightPanel  *RightExportPanel
	sidebarTabs *container.AppTabs
	fileLabel   *widget.Label
	statusLabel *widget.Label
}

// NewMainWindow creates and initializes the studio interface.
func NewMainWindow(appInstance fyne.App) *MainWindow {
	win := appInstance.NewWindow("InkAnim — Inkscape SVG to Animated GIF Studio")
	win.Resize(fyne.NewSize(1200, 750))

	sess := app.NewSession()
	mw := &MainWindow{
		window:  win,
		session: sess,
	}

	mw.fileLabel = widget.NewLabel("No file loaded")
	mw.statusLabel = widget.NewLabel("Ready. Open an Inkscape SVG to begin.")

	// Construct panels with coordinated refresh hooks
	mw.leftPanel = NewLeftFramesPanel(sess, func() {
		if mw.centerPanel != nil {
			mw.centerPanel.Refresh()
		}
		if mw.rightPanel != nil {
			mw.rightPanel.validateTwitch()
		}
	})

	mw.centerPanel = NewCenterPreviewPanel(sess)
	mw.centerPanel.SetOnPingPongChange(func() {
		if mw.rightPanel != nil {
			mw.rightPanel.validateTwitch()
		}
	})

	mw.rightPanel = NewRightExportPanel(sess, win, func() {
		if mw.centerPanel != nil {
			mw.centerPanel.Refresh()
		}
	}, mw.PausePlayback)

	// Top Bar
	openBtn := widget.NewButton("Open SVG...", func() {
		mw.promptOpenFile()
	})
	openBtn.Importance = widget.MediumImportance

	aboutBtn := widget.NewButtonWithIcon(fmt.Sprintf("About (v%s)", app.Version), theme.InfoIcon(), func() {
		resume := mw.PausePlayback()
		ShowAboutDialog(mw.window, app.Version, resume)
	})

	topToolbar := container.NewBorder(
		nil, nil,
		container.NewHBox(
			openBtn,
			widget.NewSeparator(),
			mw.fileLabel,
		),
		aboutBtn,
	)

	// Bottom Status Bar
	bottomBar := container.NewBorder(
		nil, nil,
		mw.statusLabel,
		nil,
	)

	// Unified right-side tabbed sidebar hosting Frames & Export panels
	mw.sidebarTabs = container.NewAppTabs(
		container.NewTabItemWithIcon("Frames", theme.ListIcon(), mw.leftPanel.Container()),
		container.NewTabItemWithIcon("Export", theme.DownloadIcon(), mw.rightPanel.Container()),
	)

	rightWrapper := container.New(&fixedWidthLayout{width: 320}, mw.sidebarTabs)
	rightSection := container.NewBorder(nil, nil, widget.NewSeparator(), nil, rightWrapper)

	body := container.NewBorder(
		nil,
		nil,
		nil,
		rightSection,
		mw.centerPanel.Container(),
	)

	root := container.NewBorder(
		container.NewVBox(topToolbar, widget.NewSeparator()),
		container.NewVBox(widget.NewSeparator(), bottomBar),
		nil,
		nil,
		body,
	)

	win.SetContent(root)

	// Support Drag & Drop of SVG files onto window
	win.SetOnDropped(func(pos fyne.Position, uris []fyne.URI) {
		for _, u := range uris {
			if u.Extension() == ".svg" {
				reader, err := storage.Reader(u)
				if err == nil {
					data, readErr := io.ReadAll(reader)
					_ = reader.Close()
					if readErr == nil {
						mw.loadData(data, u.Name())
						break
					}
				}
				mw.loadFilePath(u.Path())
				break
			}
		}
	})

	mw.initPlatform()
	return mw
}

// ShowAndRun displays the window.
func (mw *MainWindow) ShowAndRun() {
	mw.window.ShowAndRun()
}

// OpenFile loads and displays an SVG file from the specified file path.
func (mw *MainWindow) OpenFile(path string) {
	mw.loadFilePath(path)
}

func (mw *MainWindow) loadFilePath(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		dialog.ShowError(err, mw.window)
		mw.statusLabel.SetText("Failed to read SVG file.")
		return
	}
	mw.loadData(data, filepath.Base(path))
}

func (mw *MainWindow) loadData(data []byte, filename string) {
	if mw.centerPanel != nil {
		mw.centerPanel.Pause()
	}
	mw.statusLabel.SetText("Loading: " + filename)
	if err := mw.session.LoadSVGData(data, filename); err != nil {
		dialog.ShowError(err, mw.window)
		mw.statusLabel.SetText("Failed to load SVG.")
		return
	}

	mw.fileLabel.SetText(fmt.Sprintf("%s (%0.0fx%0.0f)", filename, mw.session.Document.Width, mw.session.Document.Height))
	mw.statusLabel.SetText(fmt.Sprintf("Loaded %d layers, %d pages. Ready to preview and export.", len(mw.session.Layers), len(mw.session.Pages)))

	mw.leftPanel.Refresh()
	mw.centerPanel.Refresh()
	mw.rightPanel.syncOptions()

	if len(mw.session.RenderedFrames) > 1 {
		mw.centerPanel.Play()
	}
}

// PausePlayback pauses active preview playback and returns a closure that resumes playback if it was previously playing.
func (mw *MainWindow) PausePlayback() func() {
	if mw.centerPanel == nil || !mw.centerPanel.IsPlaying() {
		return func() {}
	}
	mw.centerPanel.Pause()
	return func() {
		if mw.centerPanel != nil {
			mw.centerPanel.Play()
		}
	}
}

// fixedWidthLayout locks a container to a fixed horizontal width while letting height stretch.
type fixedWidthLayout struct {
	width float32
}

func (l *fixedWidthLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	h := float32(0)
	for _, o := range objects {
		if o.Visible() {
			ms := o.MinSize()
			if ms.Height > h {
				h = ms.Height
			}
		}
	}
	return fyne.NewSize(l.width, h)
}

func (l *fixedWidthLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objects {
		if o.Visible() {
			o.Resize(size)
			o.Move(fyne.NewPos(0, 0))
		}
	}
}
