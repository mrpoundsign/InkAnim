package main

import (
	"fyne.io/fyne/v2"

	"inkanim/internal/ui"
)

type EditorState = ui.EditorState

func NewEditorState(data []byte, inputPath string, selectedIDs []string) (*EditorState, error) {
	return ui.NewEditorState(data, inputPath, selectedIDs)
}

func ShowEditorWindow(a fyne.App, state *EditorState) fyne.Window {
	return ui.ShowEditorWindow(a, state)
}
