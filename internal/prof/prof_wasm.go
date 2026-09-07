//go:build js && wasm

package prof

// SetupProfiler is a no-op implementation for WebAssembly environments.
func SetupProfiler(cpuProfile, memProfile, pprofAddr string) (func(), error) {
	return func() {}, nil
}
