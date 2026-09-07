//go:build !js

package prof

import (
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"sync"
	"syscall"
)

// SetupProfiler configures CPU profiling, memory profiling, and/or a live HTTP pprof server.
// It returns a cleanup function that flushes and closes active profiles.
// If interrupted by SIGINT or SIGTERM, cleanup is automatically invoked before exiting.
func SetupProfiler(cpuProfile, memProfile, pprofAddr string) (func(), error) {
	if cpuProfile == "" && memProfile == "" && pprofAddr == "" {
		return func() {}, nil
	}

	var cpuFile *os.File
	if cpuProfile != "" {
		f, err := os.Create(cpuProfile)
		if err != nil {
			return nil, fmt.Errorf("failed to create cpu profile: %w", err)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("failed to start cpu profile: %w", err)
		}
		cpuFile = f
		log.Printf("[pprof] CPU profiling enabled -> %s\n", cpuProfile)
	}

	var server *http.Server
	if pprofAddr != "" {
		server = &http.Server{Addr: pprofAddr}
		go func() {
			log.Printf("[pprof] Live HTTP profiler listening at http://%s/debug/pprof/\n", pprofAddr)
			if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Printf("[pprof] HTTP server error: %v\n", err)
			}
		}()
	}

	var once sync.Once
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	cleanup := func() {
		once.Do(func() {
			signal.Stop(sigChan)

			if cpuFile != nil {
				pprof.StopCPUProfile()
				_ = cpuFile.Close()
				log.Printf("[pprof] CPU profile written -> %s\n", cpuProfile)
			}

			if memProfile != "" {
				mf, err := os.Create(memProfile)
				if err != nil {
					log.Printf("[pprof] Failed to create mem profile %s: %v\n", memProfile, err)
				} else {
					runtime.GC()
					if err := pprof.WriteHeapProfile(mf); err != nil {
						log.Printf("[pprof] Failed to write mem profile: %v\n", err)
					}
					_ = mf.Close()
					log.Printf("[pprof] Memory profile written -> %s\n", memProfile)
				}
			}

			if server != nil {
				_ = server.Close()
			}
		})
	}

	go func() {
		if _, ok := <-sigChan; ok {
			log.Println("[pprof] Caught termination signal, finalizing profiles...")
			cleanup()
			os.Exit(0)
		}
	}()

	return cleanup, nil
}
