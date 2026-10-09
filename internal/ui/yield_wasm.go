//go:build js && wasm

package ui

import "time"

// yieldToHost blocks briefly so the browser event loop can run (paint, input,
// queued fyne.Do callbacks). runtime.Gosched() is not enough on js/wasm: it
// switches goroutines but never returns control to the event loop.
func yieldToHost() {
	time.Sleep(time.Millisecond)
}
