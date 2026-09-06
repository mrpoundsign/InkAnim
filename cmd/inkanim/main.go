package main

import (
	"fyne.io/fyne/v2/app"

	"inkanim/assets"
	"inkanim/internal/ui"
)

func main() {
	myApp := app.NewWithID("com.inkanim.studio")
	myApp.SetIcon(assets.AppIcon)
	myApp.Settings().SetTheme(&ui.StudioTheme{})

	win := ui.NewMainWindow(myApp)
	win.ShowAndRun()
}
