//go:build profile

// Package prof provides lightweight phase timers for performance investigation.
//
// Timers are compiled in only with the "profile" build tag:
//
//	GOOS=js GOARCH=wasm go build -tags "migrated_fynedo profile" ./cmd/inkanim
//	go run -tags profile ./cmd/inkanim
//
// Without the tag every function is a no-op.
package prof

import (
	"fmt"
	"log"
	"time"
)

// Enabled reports whether profiling instrumentation is compiled in.
const Enabled = true

// Logf logs a "[Profiler]"-prefixed message.
func Logf(format string, args ...any) {
	log.Printf("[Profiler] "+format, args...)
}

// Since logs the elapsed time since start for the named phase.
func Since(start time.Time, format string, args ...any) {
	log.Printf("[Profiler] %s: %v", fmt.Sprintf(format, args...), time.Since(start))
}
