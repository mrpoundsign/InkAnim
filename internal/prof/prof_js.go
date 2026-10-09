//go:build profile && js

package prof

import "syscall/js"

// Expose the flag to index.html so its JS-side timers log only in profile builds.
func init() {
	js.Global().Set("inkanimProfile", true)
}
