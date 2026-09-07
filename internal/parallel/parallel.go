package parallel

import (
	"runtime"
	"sync"
)

// Run executes fn(i) for each index i in [0, count) across up to runtime.GOMAXPROCS(0)
// worker goroutines. If count <= 1, it executes fn directly on the caller goroutine.
// If any invocation returns a non-nil error, task execution is aborted early and
// the first encountered error is returned.
func Run(count int, fn func(i int) error) error {
	if count <= 0 {
		return nil
	}
	if count == 1 {
		return fn(0)
	}

	workers := runtime.GOMAXPROCS(0)
	if workers > count {
		workers = count
	}
	if workers < 1 {
		workers = 1
	}

	tasks := make(chan int, count)
	for i := 0; i < count; i++ {
		tasks <- i
	}
	close(tasks)

	var wg sync.WaitGroup
	var errMu sync.Mutex
	var firstErr error

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range tasks {
				errMu.Lock()
				hasErr := firstErr != nil
				errMu.Unlock()
				if hasErr {
					return
				}

				if err := fn(idx); err != nil {
					errMu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					errMu.Unlock()
					return
				}
			}
		}()
	}

	wg.Wait()
	return firstErr
}
