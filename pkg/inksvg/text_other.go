//go:build !windows

package inksvg

func scanPlatformFontRegistry(fm *FontManager) {
	// No-op on non-Windows platforms
}
