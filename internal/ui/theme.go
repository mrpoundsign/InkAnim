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
		return color.RGBA{R: 169, G: 112, B: 255, A: 255} // Vibrant Twitch purple #A970FF
	case theme.ColorNameHeaderBackground:
		return color.RGBA{R: 32, G: 32, B: 38, A: 255} // Elevated header/tab bar
	case theme.ColorNameSelection:
		return color.RGBA{R: 145, G: 71, B: 255, A: 80}
	case theme.ColorNameHover:
		return color.RGBA{R: 255, G: 255, B: 255, A: 25}
	case theme.ColorNameForeground:
		return color.RGBA{R: 244, G: 244, B: 245, A: 255} // Zinc-100
	case theme.ColorNameDisabled:
		return color.RGBA{R: 180, G: 180, B: 188, A: 255} // Zinc-300 (readable light gray)
	case theme.ColorNameHyperlink:
		return color.RGBA{R: 147, G: 197, B: 253, A: 255} // Sky-300 (crisp light blue)
	case theme.ColorNameOverlayBackground:
		return color.RGBA{R: 32, G: 32, B: 38, A: 255} // Modal/dialog background
	case theme.ColorNamePlaceHolder:
		return color.RGBA{R: 161, G: 161, B: 170, A: 255}
	case theme.ColorNameScrollBar:
		return color.RGBA{R: 82, G: 82, B: 91, A: 180}
	case theme.ColorNameError:
		return color.RGBA{R: 239, G: 68, B: 68, A: 255} // Red #EF4444
	case theme.ColorNameSuccess:
		return color.RGBA{R: 34, G: 197, B: 94, A: 255} // Green #22C55E
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
	switch name {
	case theme.SizeNameSeparatorThickness:
		return 3.0 // Bold, high-visibility 3px active tab indicator line
	default:
		return theme.DefaultTheme().Size(name)
	}
}
