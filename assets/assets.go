package assets

import (
	_ "embed"

	"fyne.io/fyne/v2"
)

//go:embed icon.png
var iconBytes []byte

// AppIcon is the embedded static resource for the InkAnim application icon.
var AppIcon = fyne.NewStaticResource("icon.png", iconBytes)
