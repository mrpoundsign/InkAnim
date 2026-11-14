package ui

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"inkanim/internal/app"
	"inkanim/pkg/inksvg"
)

// MainWindow builds and manages the primary Fyne application window.
type MainWindow struct {
	window             fyne.Window
	session            *app.Session
	centerPanel        *CenterPreviewPanel
	rightPanel         *RightExportPanel
	timelinePanel      *TimelinePanel
	fileLabel          *widget.Label
	statusLabel        *widget.Label
	bottomBar          *fyne.Container
	reloadBtn          *widget.Button
	projectPath        string
	projectDirty       bool
	pendingSourcePaths map[string]string
	rawSourceBytes     map[string][]byte
	currentPath        string
	currentFilename    string
	fileWatcher        *fileWatcher

	// Editor components
	editorState           *EditorState
	selectedActionID      string
	stripIAMSOnImport     bool
	topSplit              *container.Split
	undoItem              *fyne.MenuItem
	redoItem              *fyne.MenuItem
	deleteItem            *fyne.MenuItem
	mainMenu              *fyne.MainMenu
	previewTimer          *time.Timer
	previewGen            int
	previewMu             sync.Mutex
	lastHistoryMsg        string
	updateUndoRedo        func()
	doUndo                func()
	doRedo                func()
	doDeleteActiveElement func()

	closed bool
}

// NewMainWindow creates and initializes the studio interface.
func NewMainWindow(appInstance fyne.App) *MainWindow {
	win := appInstance.NewWindow("InkAnim — Inkscape SVG to Animated GIF Studio")
	win.Resize(fyne.NewSize(1200, 750))

	sess := app.NewSession()
	mw := &MainWindow{
		window:             win,
		session:            sess,
		currentFilename:    "Untitled Project",
		pendingSourcePaths: make(map[string]string),
		rawSourceBytes:     make(map[string][]byte),
	}

	mw.fileWatcher = newFileWatcher(mw)

	mw.fileLabel = widget.NewLabel("Untitled Project")
	mw.statusLabel = widget.NewLabel("New project ready. Add an SVG or open a .inka project.")

	mw.reloadBtn = widget.NewButtonWithIcon("Reload", theme.ViewRefreshIcon(), func() {
		mw.Reload()
	})
	mw.reloadBtn.Hide() // Hidden until a file is opened

	// Construct panels with coordinated refresh hooks
	mw.centerPanel = NewCenterPreviewPanel(sess)
	mw.centerPanel.SetGizmoOverlay(NewCanvasGizmoOverlay(mw))

	mw.timelinePanel = NewTimelinePanel(mw)
	setupStudioHistoryAndShortcuts(mw)

	mw.centerPanel.SetOnPingPongChange(func() {
		if mw.rightPanel != nil {
			mw.rightPanel.validateTwitch()
		}
		if mw.timelinePanel != nil {
			mw.timelinePanel.Refresh()
		}
		mw.RecordStudioChange("Toggle Ping-Pong")
	})
	mw.centerPanel.SetOnTimingChange(func() {
		if mw.rightPanel != nil {
			mw.rightPanel.validateTwitch()
		}
		if mw.timelinePanel != nil {
			mw.timelinePanel.Refresh()
		}
		mw.RecordStudioChange("Change timing settings")
	})
	mw.centerPanel.SetOnDurationChangeRequest(func(oldSec, newSec float64) bool {
		origObjects := cloneObjectsMap(mw.session.Objects)
		hasItems := mw.HasTimelineActionsOrWindows()
		mw.ApplyDurationChangeWithObjects(oldSec, newSec, origObjects, DurationAdjustScale, true)
		if hasItems && mw.window != nil {
			mw.ShowDurationChangeModal(oldSec, newSec, origObjects)
		}
		return true
	})
	mw.centerPanel.SetOnFrameChange(func(frameIdx, totalFrames int, timeSec float64, frameDurMs int) {
		if mw.timelinePanel != nil {
			mw.timelinePanel.UpdatePlayhead(frameIdx, totalFrames, timeSec, frameDurMs)
		}
	})

	mw.rightPanel = NewRightExportPanel(sess, win, func() {
		if mw.centerPanel != nil {
			mw.centerPanel.Refresh()
		}
	}, mw.PausePlayback)
	mw.rightPanel.SetOnExportSuccess(func(_ string) {
		mw.saveSidecarControlFile()
	})

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

	mw.topSplit = container.NewHSplit(mw.centerPanel.Container(), mw.rightPanel.Container())
	mw.topSplit.SetOffset(0.68)

	workspaceSplit := container.NewVSplit(mw.topSplit, mw.timelinePanel.Container())
	workspaceSplit.SetOffset(0.68)

	root := container.NewBorder(
		container.NewVBox(container.New(&nonExpandingWidthLayout{}, topToolbar), widget.NewSeparator()),
		container.NewVBox(widget.NewSeparator(), container.New(&nonExpandingWidthLayout{}, mw.bottomBar)),
		nil,
		nil,
		workspaceSplit,
	)

	win.SetContent(root)

	// Menus: File (Project-First) & Edit
	newProjectItem := fyne.NewMenuItem("New Project", func() {
		mw.NewProject()
	})
	newProjectItem.Shortcut = &desktop.CustomShortcut{KeyName: fyne.KeyN, Modifier: fyne.KeyModifierControl}

	openItem := fyne.NewMenuItem("Open Project...", func() {
		mw.promptOpenFile()
	})
	openItem.Shortcut = &desktop.CustomShortcut{KeyName: fyne.KeyO, Modifier: fyne.KeyModifierControl}

	saveItem := fyne.NewMenuItem("Save Project", func() {
		mw.Save()
	})
	saveItem.Shortcut = &desktop.CustomShortcut{KeyName: fyne.KeyS, Modifier: fyne.KeyModifierControl}

	saveAsItem := fyne.NewMenuItem("Save Project As...", func() {
		mw.SaveAs()
	})
	saveAsItem.Shortcut = &desktop.CustomShortcut{KeyName: fyne.KeyS, Modifier: fyne.KeyModifierControl | fyne.KeyModifierShift}

	addSVGItem := fyne.NewMenuItem("Add / Import SVG...", func() {
		mw.promptAddSVG()
	})
	addSVGItem.Shortcut = &desktop.CustomShortcut{KeyName: fyne.KeyI, Modifier: fyne.KeyModifierControl}

	reloadItem := fyne.NewMenuItem("Reload Linked SVGs", func() {
		mw.Reload()
	})
	reloadItem.Shortcut = &desktop.CustomShortcut{KeyName: fyne.KeyR, Modifier: fyne.KeyModifierControl}

	exportItem := fyne.NewMenuItem("Export GIF...", func() {
		if mw.rightPanel != nil {
			mw.rightPanel.PromptExport()
		}
	})
	exportItem.Shortcut = &desktop.CustomShortcut{KeyName: fyne.KeyE, Modifier: fyne.KeyModifierControl}

	fileMenu := fyne.NewMenu("File",
		newProjectItem,
		openItem,
		saveItem,
		saveAsItem,
		fyne.NewMenuItemSeparator(),
		addSVGItem,
		reloadItem,
		fyne.NewMenuItemSeparator(),
		exportItem,
	)

	editMenu := fyne.NewMenu("Edit",
		mw.undoItem,
		mw.redoItem,
		fyne.NewMenuItemSeparator(),
		mw.deleteItem,
	)

	mw.mainMenu = fyne.NewMainMenu(fileMenu, editMenu)
	win.SetMainMenu(mw.mainMenu)

	// Keyboard Shortcuts for File Menu
	win.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyN, Modifier: fyne.KeyModifierControl}, func(_ fyne.Shortcut) {
		mw.NewProject()
	})
	win.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyO, Modifier: fyne.KeyModifierControl}, func(_ fyne.Shortcut) {
		mw.promptOpenFile()
	})
	win.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyI, Modifier: fyne.KeyModifierControl}, func(_ fyne.Shortcut) {
		mw.promptAddSVG()
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
		if mw.rightPanel != nil {
			mw.rightPanel.PromptExport()
		}
	})

	// Drag & Drop: .inka opens the project; .svg links/adds SVG(s) into the current project
	win.SetOnDropped(func(_ fyne.Position, uris []fyne.URI) {
		for _, u := range uris {
			ext := strings.ToLower(u.Extension())
			if ext == app.ControlFileExt && u.Path() != "" {
				mw.loadFilePath(u.Path())
				return
			}
		}
		for _, u := range uris {
			ext := strings.ToLower(u.Extension())
			if ext == ".svg" {
				if u.Path() != "" {
					mw.addSVGInteractive(u.Path())
					continue
				}
				reader, err := storage.Reader(u)
				if err == nil {
					data, readErr := io.ReadAll(reader)
					_ = reader.Close()
					if readErr == nil {
						mw.addSVGDataInteractive(data, u.Name())
					}
				}
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
		if mw.fileWatcher != nil {
			mw.fileWatcher.close()
		}
	})

	return mw
}

// ShowAndRun displays the window.
func (mw *MainWindow) ShowAndRun() {
	CheckStartupUpdate(mw.window, app.Version)
	mw.window.ShowAndRun()
}

// Save saves the current project or modified SVG back to disk.
func (mw *MainWindow) Save() {
	mw.promptSaveSVG()
}

// SaveWithCallback saves the modified SVG and invokes onSaved once the save completes.
func (mw *MainWindow) SaveWithCallback(onSaved func()) {
	mw.promptSaveSVGWithCallback(onSaved)
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

	if mw.fileWatcher != nil {
		mw.fileWatcher.close()
	}
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

// SaveAs prompts for a new file destination to save the project.
func (mw *MainWindow) SaveAs() {
	mw.promptSaveSVGAs()
}

// NewProject resets the studio to a blank Untitled Project.
func (mw *MainWindow) NewProject() {
	if mw.centerPanel != nil {
		mw.centerPanel.Pause()
	}
	if mw.fileWatcher != nil {
		mw.fileWatcher.watchFiles(nil)
	}
	mw.projectPath = ""
	mw.projectDirty = false
	mw.currentPath = ""
	mw.currentFilename = "Untitled Project"
	mw.pendingSourcePaths = make(map[string]string)
	mw.rawSourceBytes = make(map[string][]byte)
	mw.editorState = nil
	mw.selectedActionID = ""
	if mw.session != nil {
		mw.session.Reset()
	}
	if mw.reloadBtn != nil {
		mw.reloadBtn.Hide()
	}
	mw.updateFileLabel()
	mw.updateStatus()
	if mw.updateUndoRedo != nil {
		mw.updateUndoRedo()
	}
	if mw.centerPanel != nil {
		mw.centerPanel.Refresh()
	}
	if mw.rightPanel != nil {
		mw.rightPanel.Refresh()
	}
	if mw.timelinePanel != nil {
		mw.timelinePanel.Refresh()
	}
}

// OpenProject opens a .inka project file from the specified file path.
func (mw *MainWindow) OpenProject(path string) {
	mw.loadFilePath(path)
}

// OpenFile loads and displays an SVG or .inka project file from the specified file path.
func (mw *MainWindow) OpenFile(path string) {
	mw.loadFilePath(path)
}

// AddSVGFile links an SVG file into the active project, migrating any legacy IAMS directives into .inka Objects.
func (mw *MainWindow) AddSVGFile(svgPath string) error {
	_, err := mw.AddSVGFileWithOptions(svgPath, false)
	return err
}

// AddSVGFileWithOptions links an SVG file into the active project, migrating any legacy IAMS directives
// into .inka Objects and optionally stripping legacy IAMS labels and helper paths from the SVG.
func (mw *MainWindow) AddSVGFileWithOptions(svgPath string, stripIAMS bool) (bool, error) {
	rawBytes, err := os.ReadFile(svgPath)
	if err != nil {
		return false, err
	}
	return mw.addSVGSourceInternal(rawBytes, filepath.Base(svgPath), svgPath, stripIAMS)
}

// AddSVGData links in-memory SVG bytes into the active project.
func (mw *MainWindow) AddSVGData(data []byte, filename string) error {
	_, err := mw.AddSVGDataWithOptions(data, filename, false)
	return err
}

// AddSVGDataWithOptions links in-memory SVG bytes into the active project, migrating any legacy IAMS directives
// into .inka Objects and optionally stripping legacy IAMS labels and helper paths from the SVG.
func (mw *MainWindow) AddSVGDataWithOptions(data []byte, filename string, stripIAMS bool) (bool, error) {
	return mw.addSVGSourceInternal(data, filename, "", stripIAMS)
}

func (mw *MainWindow) addSVGSourceInternal(rawBytes []byte, filename, diskPath string, stripIAMS bool) (bool, error) {
	hadEditorState := mw.editorState != nil
	if mw.session == nil {
		mw.session = app.NewSession()
	}
	if mw.pendingSourcePaths == nil {
		mw.pendingSourcePaths = make(map[string]string)
	}
	if mw.rawSourceBytes == nil {
		mw.rawSourceBytes = make(map[string][]byte)
	}
	if mw.session.Objects == nil {
		mw.session.Objects = make(map[string]app.ObjectConfig)
	}

	baseName := filepath.Base(filename)
	if baseName == "" || baseName == "." {
		baseName = "artwork.svg"
	}

	var srcID string
	for i, existing := range mw.session.LinkedSources {
		if strings.EqualFold(existing.File, baseName) {
			srcID = existing.ID
			if srcID == "" {
				srcID = fmt.Sprintf("src_%d", i+1)
				mw.session.LinkedSources[i].ID = srcID
			}
			break
		}
	}
	if srcID == "" {
		srcID = fmt.Sprintf("src_%d", len(mw.session.LinkedSources)+1)
		mw.session.LinkedSources = append(mw.session.LinkedSources, app.LinkedSVG{
			ID:   srcID,
			File: baseName,
		})
	}

	if len(mw.session.LinkedSources) == 1 && !hadEditorState {
		mw.session.ResetManualDuration()
	}

	var hasIAMS bool
	if parsedDoc, pErr := inksvg.ParseSVG(rawBytes); pErr == nil && parsedDoc != nil {
		hasIAMS = len(parsedDoc.MotionPaths) > 0 || parsedDoc.CameraPath != nil
		if hasIAMS {
			durSec := float64(max(mw.session.TotalDurationMs, 1000)) / 1000.0
			if len(mw.session.LinkedSources) == 1 && !mw.session.HasManualDuration && parsedDoc.FrameCount() > 1 {
				fps := max(mw.session.FPS, 10)
				durSec = float64(parsedDoc.FrameCount()) / float64(fps)
			}
			tmpCtrl := &app.ControlFile{Objects: mw.session.Objects}
			app.MigrateDocumentIAMSToControlFile(parsedDoc, srcID, durSec, tmpCtrl)
			mw.session.Objects = tmpCtrl.Objects
			if stripIAMS {
				if cleaned, sErr := inksvg.StripIAMSFromSVG(rawBytes); sErr == nil {
					rawBytes = cleaned
				}
			}
		}
	}

	if mw.projectPath != "" {
		projectDir := filepath.Dir(mw.projectPath)
		if diskPath != "" && !stripIAMS {
			_, _ = app.EnsureCoLocatedSVG(projectDir, diskPath)
		} else {
			_ = os.WriteFile(filepath.Join(projectDir, baseName), rawBytes, 0644)
		}
	}

	if diskPath != "" {
		mw.pendingSourcePaths[srcID] = diskPath
		if mw.currentPath == "" {
			mw.currentPath = diskPath
		}
	}
	mw.rawSourceBytes[srcID] = bytes.Clone(rawBytes)
	mw.projectDirty = true

	composed, err := mw.composeProjectData()
	if err != nil {
		return hasIAMS, err
	}
	mw.fileWatcher.watchFiles(mw.linkedDiskPaths())
	if len(mw.linkedDiskPaths()) > 0 && mw.reloadBtn != nil {
		mw.reloadBtn.Show()
	}
	preserveDur := len(mw.session.LinkedSources) > 1 || mw.session.HasManualDuration
	mw.loadDataWithOptions(composed, mw.projectDisplayName(), nil, preserveDur)
	if hadEditorState {
		mw.RecordStudioChange("Add SVG: " + baseName)
	}
	return hasIAMS, nil
}

func (mw *MainWindow) addSVGInteractive(svgPath string) {
	_, err := mw.AddSVGFileWithOptions(svgPath, mw.stripIAMSOnImport)
	if err != nil {
		dialog.ShowError(err, mw.window)
		mw.statusLabel.SetText("Failed to add SVG file.")
		return
	}
}

func (mw *MainWindow) addSVGDataInteractive(data []byte, filename string) {
	_, err := mw.AddSVGDataWithOptions(data, filename, mw.stripIAMSOnImport)
	if err != nil {
		dialog.ShowError(err, mw.window)
		mw.statusLabel.SetText("Failed to add SVG data.")
		return
	}
}

// StripIAMSFromLinkedSVG migrates any legacy IAMS directives from the specified linked SVG into .inka Objects
// and strips the IAMS labels and helper paths from the SVG data.
func (mw *MainWindow) StripIAMSFromLinkedSVG(sourceID string) error {
	if mw.session == nil {
		return errors.New("no active session")
	}
	var src *app.LinkedSVG
	for i := range mw.session.LinkedSources {
		if mw.session.LinkedSources[i].ID == sourceID {
			src = &mw.session.LinkedSources[i]
			break
		}
	}
	if src == nil {
		return fmt.Errorf("linked SVG %q not found", sourceID)
	}

	rawBytes := mw.rawSourceBytes[sourceID]
	if len(rawBytes) == 0 {
		diskPath := mw.resolveSourceDiskPath(*src)
		if diskPath == "" {
			return fmt.Errorf("source file for %q not found", src.File)
		}
		var err error
		rawBytes, err = os.ReadFile(diskPath)
		if err != nil {
			return err
		}
	}

	if parsedDoc, pErr := inksvg.ParseSVG(rawBytes); pErr == nil && parsedDoc != nil {
		if mw.session.Objects == nil {
			mw.session.Objects = make(map[string]app.ObjectConfig)
		}
		durSec := float64(max(mw.session.TotalDurationMs, 1000)) / 1000.0
		tmpCtrl := &app.ControlFile{Objects: mw.session.Objects}
		app.MigrateDocumentIAMSToControlFile(parsedDoc, sourceID, durSec, tmpCtrl)
		mw.session.Objects = tmpCtrl.Objects
	}

	cleaned, err := inksvg.StripIAMSFromSVG(rawBytes)
	if err != nil {
		return err
	}
	if mw.rawSourceBytes == nil {
		mw.rawSourceBytes = make(map[string][]byte)
	}
	mw.rawSourceBytes[sourceID] = cleaned

	if mw.projectPath != "" {
		coLocated := filepath.Join(filepath.Dir(mw.projectPath), src.File)
		_ = os.WriteFile(coLocated, cleaned, 0644)
	} else if pending := mw.pendingSourcePaths[sourceID]; pending != "" {
		_ = os.WriteFile(pending, cleaned, 0644)
	}

	mw.projectDirty = true
	composed, err := mw.composeProjectData()
	if err != nil {
		return err
	}
	mw.loadDataWithOptions(composed, mw.projectDisplayName(), nil, true)
	mw.RecordStudioChange("Strip IAMS from " + src.File)
	mw.statusLabel.SetText("Stripped legacy IAMS directives from " + src.File)
	return nil
}

// RemoveLinkedSVG unlinks an SVG source from the active project.
func (mw *MainWindow) RemoveLinkedSVG(sourceID string) error {
	if mw.session == nil {
		return nil
	}
	idx := -1
	for i, s := range mw.session.LinkedSources {
		if s.ID == sourceID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("linked SVG %q not found", sourceID)
	}
	removedName := mw.session.LinkedSources[idx].File
	mw.session.LinkedSources = append(mw.session.LinkedSources[:idx], mw.session.LinkedSources[idx+1:]...)
	delete(mw.pendingSourcePaths, sourceID)
	delete(mw.rawSourceBytes, sourceID)
	for k, obj := range mw.session.Objects {
		if obj.SourceID == sourceID || strings.HasPrefix(k, sourceID+"/") {
			delete(mw.session.Objects, k)
		}
	}
	mw.projectDirty = true

	if len(mw.session.LinkedSources) == 0 {
		mw.editorState = nil
		mw.selectedActionID = ""
		mw.session.Objects = make(map[string]app.ObjectConfig)
		mw.session.ResetManualDuration()
		mw.session.TotalDurationMs = 1000
		mw.session.Document = nil
		mw.session.Frames = nil
		mw.session.RenderedFrames = nil
		mw.session.InvalidateCache()
		mw.updateFileLabel()
		mw.updateStatus()
		if mw.updateUndoRedo != nil {
			mw.updateUndoRedo()
		}
		if mw.centerPanel != nil {
			mw.centerPanel.Refresh()
		}
		if mw.rightPanel != nil {
			mw.rightPanel.Refresh()
		}
		if mw.timelinePanel != nil {
			mw.timelinePanel.Refresh()
		}
		return nil
	}

	composed, err := mw.composeProjectData()
	if err != nil {
		return err
	}
	mw.fileWatcher.watchFiles(mw.linkedDiskPaths())
	mw.loadDataWithOptions(composed, mw.projectDisplayName(), nil, true)
	mw.RecordStudioChange("Unlink SVG: " + removedName)
	return nil
}

func (mw *MainWindow) resolveSourceDiskPath(src app.LinkedSVG) string {
	if mw.projectPath != "" && src.File != "" {
		cand := filepath.Join(filepath.Dir(mw.projectPath), src.File)
		if info, err := os.Stat(cand); err == nil && !info.IsDir() {
			return cand
		}
	}
	if p := mw.pendingSourcePaths[src.ID]; p != "" {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	if mw.currentPath != "" && strings.EqualFold(filepath.Base(mw.currentPath), src.File) {
		if info, err := os.Stat(mw.currentPath); err == nil && !info.IsDir() {
			return mw.currentPath
		}
	}
	return ""
}

func (mw *MainWindow) linkedDiskPaths() []string {
	if mw.session == nil {
		return nil
	}
	var paths []string
	for _, src := range mw.session.LinkedSources {
		if p := mw.resolveSourceDiskPath(src); p != "" {
			paths = append(paths, p)
		}
	}
	if len(paths) == 0 && mw.currentPath != "" {
		paths = append(paths, mw.currentPath)
	}
	return paths
}

func (mw *MainWindow) composeProjectData() ([]byte, error) {
	if mw.session == nil || len(mw.session.LinkedSources) == 0 {
		return nil, errors.New("no linked SVGs in project")
	}
	linked := make([]inksvg.LinkedSource, 0, len(mw.session.LinkedSources))
	for i, src := range mw.session.LinkedSources {
		srcID := src.ID
		if srcID == "" {
			srcID = fmt.Sprintf("src_%d", i+1)
		}
		raw := mw.rawSourceBytes[srcID]
		if len(raw) == 0 {
			diskPath := mw.resolveSourceDiskPath(src)
			if diskPath == "" {
				return nil, fmt.Errorf("linked SVG %q not found", src.File)
			}
			var err error
			raw, err = os.ReadFile(diskPath)
			if err != nil {
				return nil, err
			}
			if mw.rawSourceBytes != nil {
				mw.rawSourceBytes[srcID] = bytes.Clone(raw)
			}
		}
		linked = append(linked, inksvg.LinkedSource{
			ID:     srcID,
			Name:   src.File,
			RawSVG: raw,
		})
	}
	return inksvg.ComposeLinkedSVGs(linked)
}

func (mw *MainWindow) projectDisplayName() string {
	if mw.projectPath != "" {
		return filepath.Base(mw.projectPath)
	}
	if mw.currentPath != "" {
		return filepath.Base(mw.currentPath)
	}
	if mw.session != nil && len(mw.session.LinkedSources) > 0 {
		return fmt.Sprintf("Untitled Project (%d SVG)", len(mw.session.LinkedSources))
	}
	return "Untitled Project"
}

func readOrComposeProjectSVG(projectDir, svgPath string, ctrl *app.ControlFile) ([]byte, error) {
	if ctrl != nil && len(ctrl.Sources) > 0 {
		if projectDir == "" || projectDir == "." {
			projectDir = filepath.Dir(svgPath)
		}
		linked, err := app.LoadLinkedSources(projectDir, ctrl, svgPath)
		if err != nil {
			return nil, err
		}
		return inksvg.ComposeLinkedSVGs(linked)
	}
	raw, err := os.ReadFile(svgPath)
	if err != nil {
		return nil, err
	}
	return inksvg.ComposeLinkedSVGs([]inksvg.LinkedSource{
		{
			ID:     "src_1",
			Name:   filepath.Base(svgPath),
			RawSVG: raw,
		},
	})
}

// Reload reloads all linked SVG files from disk while preserving active project settings.
func (mw *MainWindow) Reload() {
	if mw.currentPath == "" && (mw.session == nil || len(mw.session.LinkedSources) == 0) {
		return
	}
	for k := range mw.rawSourceBytes {
		delete(mw.rawSourceBytes, k)
	}
	var data []byte
	var err error
	if mw.session != nil && len(mw.session.LinkedSources) > 0 {
		data, err = mw.composeProjectData()
	} else {
		data, err = readOrComposeProjectSVG(filepath.Dir(mw.currentPath), mw.currentPath, nil)
	}
	if err != nil {
		dialog.ShowError(err, mw.window)
		mw.statusLabel.SetText("Failed to read SVG file.")
		return
	}
	mw.loadDataWithOptions(data, mw.projectDisplayName(), nil, true)
}

func (mw *MainWindow) buildControlFile() *app.ControlFile {
	if mw.session == nil {
		return nil
	}
	ctrl := mw.session.BuildControlFile(mw.currentPath)
	if mw.centerPanel != nil {
		mw.centerPanel.PopulateControlFile(ctrl)
	}
	if mw.rightPanel != nil {
		mw.rightPanel.PopulateControlFile(ctrl)
	}
	if mw.session.Document != nil && len(ctrl.Objects) == 0 {
		durSec := float64(max(mw.session.TotalDurationMs, 1000)) / 1000.0
		app.MigrateDocumentIAMSToControlFile(mw.session.Document, "", durSec, ctrl)
	}
	return ctrl
}

func (mw *MainWindow) saveProjectToPath(inkaPath string) error {
	if inkaPath == "" {
		return errors.New("empty project path")
	}
	if !strings.EqualFold(filepath.Ext(inkaPath), app.ControlFileExt) {
		inkaPath += app.ControlFileExt
	}
	projectDir := filepath.Dir(inkaPath)
	if mw.session != nil {
		for _, src := range mw.session.LinkedSources {
			if src.File == "" {
				continue
			}
			destPath := filepath.Join(projectDir, src.File)
			if diskPath := mw.pendingSourcePaths[src.ID]; diskPath != "" {
				_, _ = app.EnsureCoLocatedSVG(projectDir, diskPath)
			} else if raw := mw.rawSourceBytes[src.ID]; len(raw) > 0 {
				if _, statErr := os.Stat(destPath); statErr != nil {
					_ = os.WriteFile(destPath, raw, 0644)
				}
			}
		}
	}

	ctrl := mw.buildControlFile()
	if err := app.SaveControlFile(inkaPath, ctrl); err != nil {
		return err
	}

	mw.projectPath = inkaPath
	mw.projectDirty = false
	if mw.editorState != nil {
		mw.editorState.MarkClean()
	}
	mw.fileWatcher.watchFiles(mw.linkedDiskPaths())
	if mw.reloadBtn != nil {
		mw.reloadBtn.Show()
	}
	mw.updateStatus()
	mw.updateFileLabel()
	return nil
}

func (mw *MainWindow) saveSidecarControlFile() {
	if mw.session == nil || mw.session.Document == nil {
		return
	}
	targetInka := mw.projectPath
	if targetInka == "" && mw.currentPath != "" {
		targetInka = app.SidecarPathForSVG(mw.currentPath)
	}
	if targetInka == "" {
		return
	}
	_ = mw.saveProjectToPath(targetInka)
}

func (mw *MainWindow) loadFilePath(path string) {
	svgPath, ctrl, err := app.ResolveSVGAndControlFile(path)
	if err != nil {
		dialog.ShowError(err, mw.window)
		mw.statusLabel.SetText("Failed to read SVG or project file.")
		return
	}
	if mw.pendingSourcePaths == nil {
		mw.pendingSourcePaths = make(map[string]string)
	} else {
		clear(mw.pendingSourcePaths)
	}
	if mw.rawSourceBytes == nil {
		mw.rawSourceBytes = make(map[string][]byte)
	} else {
		clear(mw.rawSourceBytes)
	}
	mw.projectDirty = false
	mw.selectedActionID = ""
	mw.session.Objects = make(map[string]app.ObjectConfig)
	if strings.EqualFold(filepath.Ext(path), app.ControlFileExt) {
		mw.projectPath = path
	} else {
		mw.projectPath = app.SidecarPathForSVG(svgPath)
	}

	if ctrl != nil && len(ctrl.Sources) > 0 {
		mw.session.LinkedSources = slices.Clone(ctrl.Sources)
	} else if svgPath != "" {
		mw.session.LinkedSources = []app.LinkedSVG{{ID: "src_1", File: filepath.Base(svgPath)}}
		mw.pendingSourcePaths["src_1"] = svgPath
	}

	data, err := readOrComposeProjectSVG(filepath.Dir(path), svgPath, ctrl)
	if err != nil {
		dialog.ShowError(err, mw.window)
		mw.statusLabel.SetText("Failed to read SVG file.")
		return
	}
	mw.currentPath = svgPath
	mw.currentFilename = filepath.Base(svgPath)
	mw.fileWatcher.watchFiles(mw.linkedDiskPaths())
	if mw.reloadBtn != nil {
		mw.reloadBtn.Show()
	}
	mw.loadDataWithOptions(data, filepath.Base(svgPath), ctrl, false)
}

func (mw *MainWindow) loadData(data []byte, filename string) {
	if mw.session != nil {
		mw.session.Objects = make(map[string]app.ObjectConfig)
	}
	mw.selectedActionID = ""
	mw.loadDataWithOptions(data, filename, nil, false)
}

func (mw *MainWindow) loadDataWithOptions(data []byte, filename string, ctrl *app.ControlFile, preserveSettings bool) {
	if mw.centerPanel != nil {
		mw.centerPanel.Pause()
	}
	mw.currentFilename = filename
	mw.statusLabel.SetText("Loading: " + filename)
	mw.centerPanel.BeginLoading("Rasterizing frames…")

	if len(data) > 0 && !bytes.Contains(data, []byte("data-inka-source=")) {
		srcName := filename
		if srcName == "" {
			srcName = "artwork.svg"
		}
		if composed, compErr := inksvg.ComposeLinkedSVGs([]inksvg.LinkedSource{
			{ID: "src_1", Name: srcName, RawSVG: data},
		}); compErr == nil {
			data = composed
		}
	}

	if mw.editorState == nil || !preserveSettings {
		var selectedIDs []string
		state, err := NewEditorState(data, mw.currentPath, selectedIDs)
		if err != nil {
			mw.centerPanel.EndLoading()
			dialog.ShowError(err, mw.window)
			mw.statusLabel.SetText("Failed to parse SVG.")
			return
		}
		mw.editorState = state
	} else {
		activeID := ""
		if mw.editorState.ActiveNode != nil {
			activeID = mw.editorState.ActiveNode.ID
		}
		_ = mw.editorState.RestoreSnapshot(data, activeID)
	}
	state := mw.editorState

	switch {
	case ctrl != nil:
		mw.session.Objects = make(map[string]app.ObjectConfig)
		mw.session.ApplyControlFile(ctrl)
	case preserveSettings:
		mw.session.HasManualDuration = true
	default:
		mw.session.ResetManualDuration()
	}
	if err := mw.session.LoadSVGData(data, filename); err != nil {
		mw.centerPanel.EndLoading()
		dialog.ShowError(err, mw.window)
		mw.statusLabel.SetText("Failed to load SVG.")
		return
	}
	if mw.session.Document != nil && !preserveSettings && len(mw.session.Objects) == 0 {
		mw.session.Objects = make(map[string]app.ObjectConfig)
		durSec := float64(max(mw.session.TotalDurationMs, 1000)) / 1000.0
		tmpCtrl := &app.ControlFile{Objects: mw.session.Objects}
		app.MigrateDocumentIAMSToControlFile(mw.session.Document, "", durSec, tmpCtrl)
		mw.session.Objects = tmpCtrl.Objects
	}
	mw.bindEditorStateStudioHooks(state, !preserveSettings || !state.HasStudioSnapshot)

	mw.updateFileLabel()
	mw.updateStatus()
	if mw.updateUndoRedo != nil {
		mw.updateUndoRedo()
	}

	if ctrl != nil {
		mw.centerPanel.ApplyControlFileUI(ctrl)
		mw.rightPanel.ApplyControlFileUI(ctrl)
	}
	mw.centerPanel.Refresh()
	mw.rightPanel.Refresh()
	if mw.timelinePanel != nil {
		mw.timelinePanel.Refresh()
	}

	if len(mw.session.RenderedFrames) > 1 {
		mw.centerPanel.Play()
	}
}

func (mw *MainWindow) captureStudioSnapshot() StudioSnapshot {
	snap := StudioSnapshot{
		SelectedActionID: mw.selectedActionID,
	}
	if mw.session != nil {
		snap.Objects = cloneObjectsMap(mw.session.Objects)
		snap.LinkedSources = slices.Clone(mw.session.LinkedSources)
		snap.TotalDurationMs = mw.session.TotalDurationMs
		snap.FPS = mw.session.FPS
		snap.PingPong = mw.session.ExportOptions.PingPong
	}
	if mw.rawSourceBytes != nil {
		snap.RawSourceBytes = make(map[string][]byte, len(mw.rawSourceBytes))
		for k, v := range mw.rawSourceBytes {
			snap.RawSourceBytes[k] = bytes.Clone(v)
		}
	}
	return snap
}

func (mw *MainWindow) applyStudioSnapshot(snap StudioSnapshot) {
	if mw.session == nil {
		return
	}
	cloned := snap.Clone()
	mw.session.Objects = cloned.Objects
	if mw.session.Objects == nil {
		mw.session.Objects = make(map[string]app.ObjectConfig)
	}
	mw.session.LinkedSources = cloned.LinkedSources
	mw.rawSourceBytes = cloned.RawSourceBytes
	if mw.rawSourceBytes == nil {
		mw.rawSourceBytes = make(map[string][]byte)
	}
	mw.selectedActionID = cloned.SelectedActionID
	if cloned.TotalDurationMs > 0 {
		mw.session.TotalDurationMs = cloned.TotalDurationMs
		mw.session.HasManualDuration = true
	}
	if cloned.FPS > 0 {
		mw.session.FPS = cloned.FPS
	}
	mw.session.ExportOptions.PingPong = cloned.PingPong
	if mw.centerPanel != nil {
		mw.centerPanel.isUpdatingTiming = true
		mw.centerPanel.pingPongCheck.SetChecked(cloned.PingPong)
		mw.centerPanel.isUpdatingTiming = false
	}
	if mw.session.Document != nil {
		durSec := float64(max(mw.session.TotalDurationMs, 100)) / 1000.0
		mw.session.Document.TotalDurationSec = durSec
		app.ApplyObjectsToDocument(mw.session.Document, mw.session.Objects, durSec)
	}
	mw.projectDirty = true
}

func (mw *MainWindow) bindEditorStateStudioHooks(state *EditorState, resetBaseline bool) {
	if state == nil {
		return
	}
	state.CaptureStudio = mw.captureStudioSnapshot
	state.ApplyStudio = mw.applyStudioSnapshot
	if resetBaseline || !state.HasStudioSnapshot {
		state.OriginalStudio = mw.captureStudioSnapshot().Clone()
		state.HasStudioSnapshot = true
	}
}

// RecordStudioChange records a studio-level change (.inka objects, actions, timing, or track order) in the undo/redo history.
func (mw *MainWindow) RecordStudioChange(description string) {
	if mw.editorState == nil {
		return
	}
	mw.bindEditorStateStudioHooks(mw.editorState, false)
	mw.editorState.RecordChange(description)
	mw.projectDirty = true
	if mw.updateUndoRedo != nil {
		mw.updateUndoRedo()
	}
	mw.updateStatus()
	mw.updateFileLabel()
}

// DurationAdjustMode specifies how existing timeline actions and display windows adjust when project duration changes.
type DurationAdjustMode int

const (
	// DurationAdjustScale scales existing action and display window times proportionally to the new duration.
	DurationAdjustScale DurationAdjustMode = iota
	// DurationAdjustPadEnd keeps existing start/end times and pads or clamps at the end of the timeline.
	DurationAdjustPadEnd
	// DurationAdjustPadStart shifts existing start/end times by the duration delta (adding/removing time at the beginning).
	DurationAdjustPadStart
)

// HasTimelineActionsOrWindows reports whether the active project has any timeline actions or custom display windows.
func (mw *MainWindow) HasTimelineActionsOrWindows() bool {
	if mw.session == nil {
		return false
	}
	for _, cfg := range mw.session.Objects {
		if len(cfg.Actions) > 0 || cfg.HasDisplayWindow {
			return true
		}
	}
	return false
}

func roundMillis(v float64) float64 {
	return math.Round(v*1000.0) / 1000.0
}

func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func adjustObjectsForDuration(objects map[string]app.ObjectConfig, oldSec, newSec float64, mode DurationAdjustMode) map[string]app.ObjectConfig {
	out := cloneObjectsMap(objects)
	if len(out) == 0 || oldSec <= 0 || newSec <= 0 {
		return out
	}
	switch mode {
	case DurationAdjustScale:
		ratio := newSec / oldSec
		for id, cfg := range out {
			if cfg.HasDisplayWindow {
				cfg.StartTimeSec = roundMillis(clampFloat(cfg.StartTimeSec*ratio, 0, newSec))
				cfg.EndTimeSec = roundMillis(clampFloat(cfg.EndTimeSec*ratio, cfg.StartTimeSec, newSec))
			}
			for i := range cfg.Actions {
				ns := roundMillis(clampFloat(cfg.Actions[i].StartSec*ratio, 0, max(0, newSec-0.05)))
				ne := roundMillis(clampFloat(cfg.Actions[i].EndSec*ratio, ns+0.05, newSec))
				cfg.Actions[i].StartSec = ns
				cfg.Actions[i].EndSec = ne
			}
			out[id] = cfg
		}
	case DurationAdjustPadEnd:
		for id, cfg := range out {
			if cfg.HasDisplayWindow {
				cfg.StartTimeSec = roundMillis(clampFloat(cfg.StartTimeSec, 0, newSec))
				cfg.EndTimeSec = roundMillis(clampFloat(cfg.EndTimeSec, cfg.StartTimeSec, newSec))
			}
			for i := range cfg.Actions {
				ns := roundMillis(clampFloat(cfg.Actions[i].StartSec, 0, max(0, newSec-0.05)))
				ne := roundMillis(clampFloat(cfg.Actions[i].EndSec, ns+0.05, newSec))
				cfg.Actions[i].StartSec = ns
				cfg.Actions[i].EndSec = ne
			}
			out[id] = cfg
		}
	case DurationAdjustPadStart:
		delta := newSec - oldSec
		for id, cfg := range out {
			if cfg.HasDisplayWindow {
				cfg.StartTimeSec = roundMillis(clampFloat(cfg.StartTimeSec+delta, 0, newSec))
				cfg.EndTimeSec = roundMillis(clampFloat(cfg.EndTimeSec+delta, cfg.StartTimeSec, newSec))
			}
			for i := range cfg.Actions {
				dur := max(0.05, cfg.Actions[i].EndSec-cfg.Actions[i].StartSec)
				ns := roundMillis(clampFloat(cfg.Actions[i].StartSec+delta, 0, max(0, newSec-0.05)))
				ne := roundMillis(clampFloat(ns+dur, ns+0.05, newSec))
				cfg.Actions[i].StartSec = ns
				cfg.Actions[i].EndSec = ne
			}
			out[id] = cfg
		}
	}
	return out
}

// ApplyDurationChange adjusts existing timeline items according to mode and updates the total project duration.
func (mw *MainWindow) ApplyDurationChange(newSec float64, mode DurationAdjustMode) {
	if mw.session == nil {
		return
	}
	oldSec := float64(max(mw.session.TotalDurationMs, 100)) / 1000.0
	mw.ApplyDurationChangeWithObjects(oldSec, newSec, mw.session.Objects, mode, true)
}

// ApplyDurationChangeWithObjects applies a duration change from a baseline objects snapshot.
func (mw *MainWindow) ApplyDurationChangeWithObjects(oldSec, newSec float64, baseObjects map[string]app.ObjectConfig, mode DurationAdjustMode, recordHistory bool) {
	if mw.session == nil {
		return
	}
	mw.session.Objects = adjustObjectsForDuration(baseObjects, oldSec, newSec, mode)
	_ = mw.session.SetTotalDurationSec(newSec)
	mw.projectDirty = true
	if recordHistory {
		mw.RecordStudioChange(fmt.Sprintf("Change duration to %.2fs", newSec))
	} else if mw.editorState != nil && mw.editorState.HistoryIndex >= 0 && mw.editorState.HistoryIndex < len(mw.editorState.History) {
		mw.editorState.History[mw.editorState.HistoryIndex].Studio = mw.captureStudioSnapshot().Clone()
		mw.editorState.History[mw.editorState.HistoryIndex].HasStudio = true
	}
	if mw.centerPanel != nil {
		mw.centerPanel.Refresh()
	}
	if mw.rightPanel != nil {
		mw.rightPanel.validateTwitch()
	}
	if mw.timelinePanel != nil {
		mw.timelinePanel.Refresh()
	}
}

// ShowDurationChangeModal prompts the user how existing timeline elements should adjust when total duration changes.
func (mw *MainWindow) ShowDurationChangeModal(oldSec, newSec float64, origObjects map[string]app.ObjectConfig) {
	if mw.window == nil {
		return
	}
	const (
		optScale    = "Scale existing timeline elements proportionally"
		optPadEnd   = "Add / trim empty time at the end (keep current start/stop times)"
		optPadStart = "Add / shift empty time at the beginning"
	)
	selectedMode := DurationAdjustScale
	radio := widget.NewRadioGroup([]string{optScale, optPadEnd, optPadStart}, func(val string) {
		switch val {
		case optPadEnd:
			selectedMode = DurationAdjustPadEnd
		case optPadStart:
			selectedMode = DurationAdjustPadStart
		default:
			selectedMode = DurationAdjustScale
		}
		mw.ApplyDurationChangeWithObjects(oldSec, newSec, origObjects, selectedMode, false)
	})
	radio.SetSelected(optScale)

	info := widget.NewLabel(fmt.Sprintf("Timeline duration changed from %.2fs to %.2fs.\nHow should existing timeline actions and visibility windows be adjusted?", oldSec, newSec))
	info.Wrapping = fyne.TextWrapWord

	content := container.NewVBox(
		info,
		widget.NewSeparator(),
		radio,
	)

	d := dialog.NewCustomConfirm(
		"Adjust Timeline Duration",
		"Apply",
		"Cancel",
		container.New(&fixedWidthLayout{width: 440}, content),
		func(apply bool) {
			if !apply {
				if mw.editorState != nil && mw.editorState.CanUndo() {
					_, _ = mw.editorState.Undo()
					mw.rebuildAfterHistoryChange()
				} else {
					mw.ApplyDurationChangeWithObjects(newSec, oldSec, origObjects, DurationAdjustPadEnd, false)
				}
			}
		},
		mw.window,
	)
	d.Show()
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
