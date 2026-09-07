package parallel

import (
	"errors"
	"sync/atomic"
	"testing"
)

func TestRun_EmptyAndSingle(t *testing.T) {
	// Count <= 0
	if err := Run(0, func(i int) error {
		t.Fatal("should not be called for count 0")
		return nil
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Count == 1
	var called bool
	if err := Run(1, func(i int) error {
		if i != 0 {
			t.Fatalf("expected index 0, got %d", i)
		}
		called = true
		return nil
	}); err != nil || !called {
		t.Fatalf("expected single call without error, called=%v, err=%v", called, err)
	}
}

func TestRun_Multiple(t *testing.T) {
	const n = 100
	visited := make([]int32, n)

	err := Run(n, func(i int) error {
		atomic.AddInt32(&visited[i], 1)
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for i := range n {
		if visited[i] != 1 {
			t.Errorf("expected visited[%d] == 1, got %d", i, visited[i])
		}
	}
}

func TestRun_ErrorPropagation(t *testing.T) {
	sentinel := errors.New("expected failure")

	err := Run(50, func(i int) error {
		if i == 25 {
			return sentinel
		}
		return nil
	})

	if !errors.Is(err, sentinel) {
		t.Fatalf("expected %v, got %v", sentinel, err)
	}
}
