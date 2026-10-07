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
		return color.RGBA{R: 10, G: 10, B: 12, A: 255} // Obsidian Slate #0A0A0C
	case theme.ColorNameInputBackground:
		return color.RGBA{R: 30, G: 41, B: 59, A: 255} // Elevated Slate-800 #1E293B
	case theme.ColorNameButton:
		return color.RGBA{R: 30, G: 41, B: 59, A: 255} // Elevated Slate-800 #1E293B (visible against Obsidian background)
	case theme.ColorNamePrimary:
		return color.RGBA{R: 56, G: 189, B: 248, A: 255} // Electric Sky Blue #38BDF8
	case theme.ColorNameHeaderBackground:
		return color.RGBA{R: 15, G: 17, B: 21, A: 255} // Elevated studio header #0F1115
	case theme.ColorNameSelection:
		return color.RGBA{R: 56, G: 189, B: 248, A: 64} // Sky blue selection tint
	case theme.ColorNameHover:
		return color.RGBA{R: 255, G: 255, B: 255, A: 25}
	case theme.ColorNameForeground:
		return color.RGBA{R: 255, G: 255, B: 255, A: 255} // Chrome White #FFFFFF
	case theme.ColorNameDisabled:
		return color.RGBA{R: 148, G: 163, B: 184, A: 255} // Slate-400 #94A3B8
	case theme.ColorNameHyperlink:
		return color.RGBA{R: 56, G: 189, B: 248, A: 255} // Electric Sky Blue #38BDF8
	case theme.ColorNameOverlayBackground:
		return color.RGBA{R: 20, G: 20, B: 22, A: 255} // Modal/dialog background #141416
	case theme.ColorNamePlaceHolder:
		return color.RGBA{R: 100, G: 116, B: 139, A: 255} // Slate-500 #64748B
	case theme.ColorNameScrollBar:
		return color.RGBA{R: 71, G: 85, B: 105, A: 180} // Slate-600 #475569
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
	case theme.SizeNameText:
		return theme.DefaultTheme().Size(name) * 0.75
	case theme.SizeNameHeadingText:
		return theme.DefaultTheme().Size(name) * 0.75
	case theme.SizeNameSubHeadingText:
		return theme.DefaultTheme().Size(name) * 0.75
	case theme.SizeNameCaptionText:
		return theme.DefaultTheme().Size(name) * 0.75
	case theme.SizeNameInlineIcon:
		return theme.DefaultTheme().Size(name) * 0.75
	default:
		return theme.DefaultTheme().Size(name)
	}
}
