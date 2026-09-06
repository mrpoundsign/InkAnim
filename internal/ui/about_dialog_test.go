package ui

import (
	"testing"

	"fyne.io/fyne/v2/test"
)

func TestShowAboutDialog(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	win := app.NewWindow("Test Window")
	defer win.Close()

	// Ensure ShowAboutDialog initializes all widgets without panics
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("ShowAboutDialog panicked: %v", r)
		}
	}()

	ShowAboutDialog(win, "v0.1.2")
	ShowLicensesDialog(win)
}
