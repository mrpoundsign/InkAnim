//go:build !windows || !cgo

package main

import (
	"os"
)

func initDebugLog() {}

func logDebug(format string, args ...any) {}

func closeDebugLog() {}

func writeOutput(data []byte) error {
	_, err := os.Stdout.Write(data)
	return err
}
