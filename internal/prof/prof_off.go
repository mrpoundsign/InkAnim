//go:build !profile

// Package prof provides lightweight phase timers for performance investigation.
// Build with the "profile" tag to enable them; otherwise every function is a no-op.
package prof

import "time"

// Enabled reports whether profiling instrumentation is compiled in.
const Enabled = false

// Logf is a no-op without the "profile" build tag.
func Logf(string, ...any) {}

// Since is a no-op without the "profile" build tag.
func Since(time.Time, string, ...any) {}
