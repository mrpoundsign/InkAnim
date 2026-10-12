package ui

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"inkanim/internal/app"
)

// MainWindow builds and manages the primary Fyne application window.
type MainWindow struct {
	window          fyne.Window
	session         *app.Session
	leftPanel       *LeftFramesPanel
	centerPanel     *CenterPreviewPanel
	rightPanel      *RightExportPanel
	sidebarTabs     *container.AppTabs
	fileLabel       *widget.Label
	statusLabel     *widget.Label
	bottomBar       *fyne.Container
	reloadBtn       *widget.Button
	currentPath     string
	currentFilename string
	fileWatcher     *fileWatcher

	// Editor components
	editorState        *EditorState
	tree               *deletableTree
	inspectorCard      *fyne.Container
	undoBtn            *widget.Button
	redoBtn            *widget.Button
	undoItem           *fyne.MenuItem
	redoItem           *fyne.MenuItem
	copyItem           *fyne.MenuItem
	pasteItem          *fyne.MenuItem
	deleteItem         *fyne.MenuItem
	mainMenu           *fyne.MainMenu
	previewTimer       *time.Timer
	previewMu          sync.Mutex
	lastHistoryMsg     string
	refreshTreeFn      func()
	refreshInspectorFn func()
	updateUndoRedo     func()
	doUndo             func()
	doRedo             func()
	doCopy             func()
	doPaste            func()
	doDeleteActiveElement func()

	// Extension mode
	isExtMode bool
	closed    bool
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

	mw.fileWatcher = newFileWatcher(mw)

	mw.fileLabel = widget.NewLabel("No file loaded")
	mw.statusLabel = widget.NewLabel("Ready. Open an Inkscape SVG to begin.")

	mw.reloadBtn = widget.NewButtonWithIcon("Reload", theme.ViewRefreshIcon(), func() {
		mw.Reload()
	})
	mw.reloadBtn.Hide() // Hidden until a file is opened

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

	aboutBtn := widget.NewButtonWithIcon(fmt.Sprintf("About (v%s)", app.Version), theme.InfoIcon(), func() {
		resume := mw.PausePlayback()
		ShowAboutDialog(mw.window, app.Version, resume)
	})

	topToolbar := container.NewBorder(
		nil, nil,
		container.NewHBox(
			mw.reloadBtn,
			mw.fileLabel,
		),
		aboutBtn,
	)

	// Bottom Status Bar
	mw.bottomBar = container.NewHBox(
		mw.statusLabel,
	)

	// Layers & Motion tab
	motionSplit := setupLayersAndMotionTab(mw)

	// Unified tabbed sidebar hosting Layers & Motion, Frames, and Export panels
	mw.sidebarTabs = container.NewAppTabs(
		container.NewTabItemWithIcon("Layers & Motion", theme.VisibilityIcon(), motionSplit),
		container.NewTabItemWithIcon("Frames", theme.ListIcon(), mw.leftPanel.Container()),
		container.NewTabItemWithIcon("Export", theme.DownloadIcon(), mw.rightPanel.Container()),
	)

	mainSplit := container.NewHSplit(mw.sidebarTabs, mw.centerPanel.Container())
	mainSplit.SetOffset(0.55)

	root := container.NewBorder(
		container.NewVBox(topToolbar, widget.NewSeparator()),
		container.NewVBox(widget.NewSeparator(), mw.bottomBar),
		nil,
		nil,
		mainSplit,
	)

	win.SetContent(root)

	// Menus: File & Edit
	openItem := fyne.NewMenuItem("Open SVG...", func() {
		mw.promptOpenFile()
	})
	openItem.Shortcut = &desktop.CustomShortcut{KeyName: fyne.KeyO, Modifier: fyne.KeyModifierControl}

	reloadItem := fyne.NewMenuItem("Reload SVG", func() {
		mw.Reload()
	})
	reloadItem.Shortcut = &desktop.CustomShortcut{KeyName: fyne.KeyR, Modifier: fyne.KeyModifierControl}

	saveItem := fyne.NewMenuItem("Save SVG", func() {
		mw.Save()
	})
	saveItem.Shortcut = &desktop.CustomShortcut{KeyName: fyne.KeyS, Modifier: fyne.KeyModifierControl}

	saveAsItem := fyne.NewMenuItem("Save SVG As...", func() {
		mw.SaveAs()
	})
	saveAsItem.Shortcut = &desktop.CustomShortcut{KeyName: fyne.KeyS, Modifier: fyne.KeyModifierControl | fyne.KeyModifierShift}

	exportItem := fyne.NewMenuItem("Export GIF...", func() {
		if mw.sidebarTabs != nil && len(mw.sidebarTabs.Items) >= 3 {
			mw.sidebarTabs.SelectIndex(2)
		}
		if mw.rightPanel != nil {
			mw.rightPanel.PromptExport()
		}
	})
	exportItem.Shortcut = &desktop.CustomShortcut{KeyName: fyne.KeyE, Modifier: fyne.KeyModifierControl}

	fileMenu := fyne.NewMenu("File",
		openItem,
		reloadItem,
		fyne.NewMenuItemSeparator(),
		saveItem,
		saveAsItem,
		fyne.NewMenuItemSeparator(),
		exportItem,
	)

	editMenu := fyne.NewMenu("Edit",
		mw.undoItem,
		mw.redoItem,
		fyne.NewMenuItemSeparator(),
		mw.copyItem,
		mw.pasteItem,
		fyne.NewMenuItemSeparator(),
		mw.deleteItem,
	)

	mw.mainMenu = fyne.NewMainMenu(fileMenu, editMenu)
	win.SetMainMenu(mw.mainMenu)

	// Keyboard Shortcuts for File Menu
	win.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyO, Modifier: fyne.KeyModifierControl}, func(_ fyne.Shortcut) {
		mw.promptOpenFile()
	})
	win.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyR, Modifier: fyne.KeyModifierControl}, func(_ fyne.Shortcut) {
		mw.Reload()
	})
	win.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyS, Modifier: fyne.KeyModifierControl}, func(_ fyne.Shortcut) {
		mw.Save()
	})
	win.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyS, Modifier: fyne.KeyModifierControl | fyne.KeyModifierShift}, func(_ fyne.Shortcut) {
		mw.SaveAs()
	})
	win.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyE, Modifier: fyne.KeyModifierControl}, func(_ fyne.Shortcut) {
		if mw.sidebarTabs != nil && len(mw.sidebarTabs.Items) >= 3 {
			mw.sidebarTabs.SelectIndex(2)
		}
		if mw.rightPanel != nil {
			mw.rightPanel.PromptExport()
		}
	})

	// Drag & Drop of SVG files onto window
	win.SetOnDropped(func(pos fyne.Position, uris []fyne.URI) {
		for _, u := range uris {
			if u.Extension() == ".svg" {
				reader, err := storage.Reader(u)
				if err == nil {
					data, readErr := io.ReadAll(reader)
					_ = reader.Close()
					if readErr == nil {
						mw.currentPath = u.Path()
						mw.loadData(data, u.Name())
						break
					}
				}
				mw.loadFilePath(u.Path())
				break
			}
		}
	})

	win.SetCloseIntercept(mw.handleCloseRequest)
	win.SetOnClosed(func() {
		mw.previewMu.Lock()
		mw.closed = true
		if mw.previewTimer != nil {
			mw.previewTimer.Stop()
		}
		mw.previewMu.Unlock()

		if mw.centerPanel != nil {
			mw.centerPanel.Pause()
		}
	})

	mw.initPlatform()
	return mw
}

// ShowAndRun displays the window.
func (mw *MainWindow) ShowAndRun() {
	CheckStartupUpdate(mw.window, app.Version)
	mw.window.ShowAndRun()
}

// Save saves the modified SVG back to disk (desktop) or downloads it (WASM).
func (mw *MainWindow) Save() {
	mw.promptSaveSVG()
}

// Close unconditionally cleans up timers, playback, and closes the window.
func (mw *MainWindow) Close() {
	mw.previewMu.Lock()
	if mw.closed {
		mw.previewMu.Unlock()
		return
	}
	mw.closed = true
	if mw.previewTimer != nil {
		mw.previewTimer.Stop()
	}
	mw.previewMu.Unlock()

	if mw.centerPanel != nil {
		mw.centerPanel.Pause()
	}
	mw.window.SetCloseIntercept(nil)
	mw.window.Close()
}

// StopPreviewTimer stops any debounced preview update timer.
func (mw *MainWindow) StopPreviewTimer() {
	mw.previewMu.Lock()
	if mw.previewTimer != nil {
		mw.previewTimer.Stop()
	}
	mw.previewMu.Unlock()
}

// SaveAs prompts for a new file destination to save the SVG.
func (mw *MainWindow) SaveAs() {
	mw.promptSaveSVGAs()
}

// OpenFile loads and displays an SVG file from the specified file path.
func (mw *MainWindow) OpenFile(path string) {
	mw.loadFilePath(path)
}

// Reload reloads the currently opened SVG file from disk.
func (mw *MainWindow) Reload() {
	if mw.currentPath != "" {
		mw.loadFilePath(mw.currentPath)
	}
}

func (mw *MainWindow) loadFilePath(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		dialog.ShowError(err, mw.window)
		mw.statusLabel.SetText("Failed to read SVG file.")
		return
	}
	mw.currentPath = path
	mw.currentFilename = filepath.Base(path)
	mw.fileWatcher.watchFile(path)
	mw.showReloadButton(mw.reloadBtn)
	mw.loadData(data, filepath.Base(path))
}

func (mw *MainWindow) loadData(data []byte, filename string) {
	if mw.centerPanel != nil {
		mw.centerPanel.Pause()
	}
	mw.currentFilename = filename
	mw.statusLabel.SetText("Loading: " + filename)
	mw.centerPanel.BeginLoading("Rasterizing frames…")

	if !mw.isExtMode || mw.editorState == nil {
		var selectedIDs []string
		state, err := NewEditorState(data, mw.currentPath, selectedIDs)
		if err != nil {
			mw.centerPanel.EndLoading()
			dialog.ShowError(err, mw.window)
			mw.statusLabel.SetText("Failed to parse SVG.")
			return
		}
		mw.editorState = state
	}
	state := mw.editorState

	if err := mw.session.LoadSVGData(data, filename); err != nil {
		mw.centerPanel.EndLoading()
		dialog.ShowError(err, mw.window)
		mw.statusLabel.SetText("Failed to load SVG.")
		return
	}

	mw.updateFileLabel()
	mw.updateStatus()

	if mw.tree != nil {
		mw.tree.OpenAllBranches()
		mw.tree.ScrollToTop()
		if state.ActiveNode != nil {
			mw.tree.Select(state.ActiveNode.ID)
		}
	}
	if mw.refreshTreeFn != nil {
		mw.refreshTreeFn()
	}
	if mw.refreshInspectorFn != nil {
		mw.refreshInspectorFn()
	}
	if mw.updateUndoRedo != nil {
		mw.updateUndoRedo()
	}

	mw.leftPanel.Refresh()
	mw.centerPanel.Refresh()
	mw.rightPanel.syncOptions()

	if len(mw.session.RenderedFrames) > 1 {
		mw.centerPanel.Play()
	}
}

// EnableExtensionMode configures the window for Inkscape extension mode with Apply and Cancel buttons.
func (mw *MainWindow) EnableExtensionMode() {
	mw.isExtMode = true
	applyBtn := widget.NewButtonWithIcon("Apply Changes", theme.ConfirmIcon(), func() {
		if mw.centerPanel != nil {
			mw.centerPanel.Pause()
		}
		if mw.editorState != nil && mw.editorState.IsDirty() {
			mw.editorState.Result = mw.editorState.ComputePatchedSVG()
			mw.editorState.Applied = true
		} else if mw.editorState != nil {
			mw.editorState.Applied = false
		}
		mw.window.Close()
	})
	applyBtn.Importance = widget.HighImportance

	cancelBtn := widget.NewButtonWithIcon("Cancel", theme.CancelIcon(), func() {
		mw.handleCloseRequest()
	})

	if mw.bottomBar != nil {
		mw.bottomBar.Add(layout.NewSpacer())
		mw.bottomBar.Add(cancelBtn)
		mw.bottomBar.Add(applyBtn)
		mw.bottomBar.Refresh()
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
