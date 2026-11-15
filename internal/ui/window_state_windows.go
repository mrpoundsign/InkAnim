//go:build windows

package ui

import (
	"syscall"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver"
)

var (
	modUser32      = syscall.NewLazyDLL("user32.dll")
	procIsZoomed   = modUser32.NewProc("IsZoomed")
	procShowWindow = modUser32.NewProc("ShowWindow")
)

const swMaximize = 3

func isWindowMaximized(win fyne.Window) bool {
	nw, ok := win.(driver.NativeWindow)
	if !ok {
		return false
	}
	var zoomed bool
	nw.RunNative(func(ctx any) {
		if wc, ok := ctx.(driver.WindowsWindowContext); ok && wc.HWND != 0 {
			ret, _, _ := procIsZoomed.Call(wc.HWND)
			zoomed = ret != 0
		}
	})
	return zoomed
}

func maximizeWindow(win fyne.Window) {
	nw, ok := win.(driver.NativeWindow)
	if !ok {
		return
	}
	nw.RunNative(func(ctx any) {
		if wc, ok := ctx.(driver.WindowsWindowContext); ok && wc.HWND != 0 {
			_, _, _ = procShowWindow.Call(wc.HWND, swMaximize)
		}
	})
}
