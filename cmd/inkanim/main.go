package main

import (
	"os"

	"fyne.io/fyne/v2/app"

	"inkanim/assets"
	"inkanim/internal/ui"
)

func main() {
	myApp := app.NewWithID("com.inkanim.studio")
	myApp.SetIcon(assets.AppIcon)
	myApp.Settings().SetTheme(&ui.StudioTheme{})

	win := ui.NewMainWindow(myApp)

	if len(os.Args) > 1 && os.Args[1] != "" {
		win.OpenFile(os.Args[1])
	}

	win.ShowAndRun()
}
