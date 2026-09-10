package inksvg

import (
	"math"
	"testing"
)

func TestEvaluatePathAt_Lines(t *testing.T) {
	// A simple path of 3 connected lines: (0,0) -> (100,0) -> (100,100) -> (0,100)
	// Total length: 300
	pathData := "M 0,0 L 100,0 L 100,100 L 0,100"

	// At t=0, we should be at (0,0)
	// At t=1/3, we should be at (100,0)
	// At t=2/3, we should be at (100,100)
	// At t=1, we should be at (0,100)

	tests := []struct {
		t       float64
		wantX   float64
		wantY   float64
		tol     float64
	}{
		{0.0, 0, 0, 0.001},
		{1.0 / 3.0, 100, 0, 0.001},
		{0.5, 100, 50, 0.001},
		{2.0 / 3.0, 100, 100, 0.001},
		{1.0, 0, 100, 0.001},
	}

	for _, tt := range tests {
		gotX, gotY, err := EvaluatePathAt(pathData, tt.t)
		if err != nil {
			t.Fatalf("EvaluatePathAt(%f) returned error: %v", tt.t, err)
		}
		if math.Abs(gotX-tt.wantX) > tt.tol || math.Abs(gotY-tt.wantY) > tt.tol {
			t.Errorf("EvaluatePathAt(%f) = (%f, %f), want (%f, %f)", tt.t, gotX, gotY, tt.wantX, tt.wantY)
		}
	}
}

func TestEvaluatePathAt_RelativeCommands(t *testing.T) {
	pathData := "m 10,10 l 90,0 l 0,100"

	gotX, gotY, err := EvaluatePathAt(pathData, 1.0)
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	wantX, wantY := 90.0, 100.0
	if math.Abs(gotX-wantX) > 0.001 || math.Abs(gotY-wantY) > 0.001 {
		t.Errorf("EvaluatePathAt(1.0) = (%f, %f), want (%f, %f)", gotX, gotY, wantX, wantY)
	}
}
