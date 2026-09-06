package main

import (
	"fyne.io/fyne/v2/app"

	"inkanim/internal/ui"
)

func main() {
	myApp := app.NewWithID("com.inkanim.studio")
	myApp.Settings().SetTheme(&ui.StudioTheme{})

	win := ui.NewMainWindow(myApp)
	win.ShowAndRun()
}
