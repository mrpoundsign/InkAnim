//go:build !wasm && !js

package worker

func Run() {
	// No-op on native platforms
}
