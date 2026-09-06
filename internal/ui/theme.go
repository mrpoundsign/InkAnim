package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// StudioTheme is a sleek dark theme designed for pixel & vector emote artists.
type StudioTheme struct{}

var _ fyne.Theme = (*StudioTheme)(nil)

func (m *StudioTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameBackground:
		return color.RGBA{R: 24, G: 24, B: 27, A: 255} // Twitch dark base #18181B
	case theme.ColorNameInputBackground:
		return color.RGBA{R: 39, G: 39, B: 42, A: 255} // Zinc-800
	case theme.ColorNameButton:
		return color.RGBA{R: 49, G: 49, B: 54, A: 255}
	case theme.ColorNamePrimary:
		return color.RGBA{R: 145, G: 71, B: 255, A: 255} // Twitch brand purple #9146FF
	case theme.ColorNameForeground:
		return color.RGBA{R: 244, G: 244, B: 245, A: 255} // Zinc-100
	case theme.ColorNamePlaceHolder:
		return color.RGBA{R: 161, G: 161, B: 170, A: 255}
	case theme.ColorNameScrollBar:
		return color.RGBA{R: 82, G: 82, B: 91, A: 180}
	default:
		return theme.DefaultTheme().Color(name, theme.VariantDark)
	}
}

func (m *StudioTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}

func (m *StudioTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

func (m *StudioTheme) Size(name fyne.ThemeSizeName) float32 {
	return theme.DefaultTheme().Size(name)
}
