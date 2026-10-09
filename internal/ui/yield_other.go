//go:build !(js && wasm)

package ui

// yieldToHost is a no-op on native targets: the preview build runs on its own
// OS thread and never blocks the UI.
func yieldToHost() {}
