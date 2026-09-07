//go:build !js

package prof

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSetupProfiler(t *testing.T) {
	tempDir := t.TempDir()
	cpuPath := filepath.Join(tempDir, "test_cpu.pprof")
	memPath := filepath.Join(tempDir, "test_mem.pprof")

	cleanup, err := SetupProfiler(cpuPath, memPath, "")
	if err != nil {
		t.Fatalf("SetupProfiler failed: %v", err)
	}

	// Do some small CPU & memory work
	var sink []byte
	start := time.Now()
	for time.Since(start) < 20*time.Millisecond {
		sink = append(sink, make([]byte, 1024)...)
	}
	_ = sink

	cleanup()

	// Verify CPU profile was created and has non-zero size
	cpuFi, err := os.Stat(cpuPath)
	if err != nil {
		t.Fatalf("cpu profile was not created: %v", err)
	}
	if cpuFi.Size() == 0 {
		t.Errorf("cpu profile is empty")
	}

	// Verify Mem profile was created and has non-zero size
	memFi, err := os.Stat(memPath)
	if err != nil {
		t.Fatalf("mem profile was not created: %v", err)
	}
	if memFi.Size() == 0 {
		t.Errorf("mem profile is empty")
	}
}

func TestSetupProfiler_NoArgs(t *testing.T) {
	cleanup, err := SetupProfiler("", "", "")
	if err != nil {
		t.Fatalf("SetupProfiler with empty args failed: %v", err)
	}
	if cleanup == nil {
		t.Fatal("cleanup func should not be nil")
	}
	cleanup()
}
