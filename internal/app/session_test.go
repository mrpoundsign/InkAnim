package app

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestSessionGlobalAndOverrideDurations(t *testing.T) {
	testSVGPath, err := filepath.Abs("../../testdata/character_walk.svg")
	if err != nil {
		t.Fatalf("failed to resolve test SVG path: %v", err)
	}

	sess := NewSession()
	if err := sess.LoadSVG(testSVGPath); err != nil {
		t.Fatalf("failed to load SVG: %v", err)
	}

	if len(sess.RenderedFrames) != 3 {
		t.Fatalf("expected 3 rendered frames, got %d", len(sess.RenderedFrames))
	}

	// Default global duration is 100ms
	for i, f := range sess.RenderedFrames {
		if f.DurationMs != 100 {
			t.Errorf("frame %d expected default 100ms, got %d", i, f.DurationMs)
		}
	}

	// Change global duration to 50ms
	sess.SetGlobalDuration(50)
	for i, f := range sess.RenderedFrames {
		if f.DurationMs != 50 {
			t.Errorf("frame %d expected updated 50ms, got %d", i, f.DurationMs)
		}
	}

	// Set override on frame 1 (index 1) to 300ms
	sess.SetFrameOverride(1, true, 300)
	if sess.RenderedFrames[0].DurationMs != 50 {
		t.Errorf("frame 0 should remain 50ms, got %d", sess.RenderedFrames[0].DurationMs)
	}
	if sess.RenderedFrames[1].DurationMs != 300 {
		t.Errorf("frame 1 should be overridden to 300ms, got %d", sess.RenderedFrames[1].DurationMs)
	}
	if sess.RenderedFrames[2].DurationMs != 50 {
		t.Errorf("frame 2 should remain 50ms, got %d", sess.RenderedFrames[2].DurationMs)
	}

	// Change global duration to 75ms: frame 0 and 2 should update, frame 1 should keep 300ms
	sess.SetGlobalDuration(75)
	if sess.RenderedFrames[0].DurationMs != 75 {
		t.Errorf("frame 0 should update to 75ms, got %d", sess.RenderedFrames[0].DurationMs)
	}
	if sess.RenderedFrames[1].DurationMs != 300 {
		t.Errorf("frame 1 override should stay 300ms, got %d", sess.RenderedFrames[1].DurationMs)
	}
	if sess.RenderedFrames[2].DurationMs != 75 {
		t.Errorf("frame 2 should update to 75ms, got %d", sess.RenderedFrames[2].DurationMs)
	}

	// Unset override on frame 1: should immediately revert to global (75ms)
	sess.SetFrameOverride(1, false, 0)
	if sess.RenderedFrames[1].DurationMs != 75 {
		t.Errorf("frame 1 should revert to 75ms, got %d", sess.RenderedFrames[1].DurationMs)
	}
}

func TestSessionToggleLayerPinnedAndActive(t *testing.T) {
	testSVGPath, err := filepath.Abs("../../testdata/character_walk.svg")
	if err != nil {
		t.Fatalf("failed to resolve test SVG path: %v", err)
	}

	// Test pinning frame 1 (index 1)
	sess := NewSession()
	if err := sess.LoadSVG(testSVGPath); err != nil {
		t.Fatalf("failed to load SVG: %v", err)
	}

	t.Logf("Initial frames count: %d", len(sess.RenderedFrames))

	// Pin frame 1
	if err := sess.ToggleLayerPinned(1); err != nil {
		t.Fatalf("ToggleLayerPinned(1) failed: %v", err)
	}
	t.Logf("Frames count after pinning frame 1: %d", len(sess.RenderedFrames))
	if len(sess.RenderedFrames) != 2 {
		t.Errorf("expected 2 frames after pinning frame 1, got %d", len(sess.RenderedFrames))
	}

	// Pin frame 2
	sess2 := NewSession()
	if err := sess2.LoadSVG(testSVGPath); err != nil {
		t.Fatalf("LoadSVG failed: %v", err)
	}
	if err := sess2.ToggleLayerPinned(2); err != nil {
		t.Fatalf("ToggleLayerPinned(2) failed: %v", err)
	}
	t.Logf("Frames count after pinning frame 2: %d", len(sess2.RenderedFrames))
	if len(sess2.RenderedFrames) != 2 {
		t.Errorf("expected 2 frames after pinning frame 2, got %d", len(sess2.RenderedFrames))
	}

	// Disable frame 1
	sess3 := NewSession()
	if err := sess3.LoadSVG(testSVGPath); err != nil {
		t.Fatalf("LoadSVG failed: %v", err)
	}
	if err := sess3.ToggleLayerActive(1); err != nil {
		t.Fatalf("ToggleLayerActive(1) failed: %v", err)
	}
	t.Logf("Frames count after disabling frame 1: %d", len(sess3.RenderedFrames))
	if len(sess3.RenderedFrames) != 2 {
		t.Errorf("expected 2 frames after disabling frame 1, got %d", len(sess3.RenderedFrames))
	}
}

func TestSessionLoadSVGDataAndExportWriter(t *testing.T) {
	testSVGPath, err := filepath.Abs("../../testdata/character_walk.svg")
	if err != nil {
		t.Fatalf("failed to resolve test SVG path: %v", err)
	}

	data, err := os.ReadFile(testSVGPath)
	if err != nil {
		t.Fatalf("failed to read test SVG: %v", err)
	}

	sess := NewSession()
	if err := sess.LoadSVGData(data, "test.svg"); err != nil {
		t.Fatalf("LoadSVGData failed: %v", err)
	}

	if len(sess.Layers) != 3 {
		t.Fatalf("expected 3 layers, got %d", len(sess.Layers))
	}

	// Test ExportGIFWriter
	var buf bytes.Buffer
	written, err := sess.ExportGIFWriter(&buf)
	if err != nil {
		t.Fatalf("ExportGIFWriter failed: %v", err)
	}
	if written <= 0 || buf.Len() == 0 {
		t.Errorf("expected non-empty output buffer, got %d bytes", written)
	}
}

func TestRenderExportFramesVectorResolution(t *testing.T) {
	testSVGPath, err := filepath.Abs("../../testdata/character_walk.svg")
	if err != nil {
		t.Fatalf("failed to resolve test SVG path: %v", err)
	}

	sess := NewSession()
	if err := sess.LoadSVG(testSVGPath); err != nil {
		t.Fatalf("failed to load SVG: %v", err)
	}

	// 1. Test square export at 512x512
	sess.ExportOptions.ExportSquare = true
	sess.ExportOptions.SquareSize = 512

	frames512, err := sess.RenderExportFrames()
	if err != nil {
		t.Fatalf("RenderExportFrames(512) failed: %v", err)
	}
	if len(frames512) != 3 {
		t.Fatalf("expected 3 frames, got %d", len(frames512))
	}
	for i, f := range frames512 {
		b := f.Image.Bounds()
		if b.Dx() != 512 || b.Dy() != 512 {
			t.Errorf("frame %d expected 512x512, got %dx%d", i, b.Dx(), b.Dy())
		}
	}

	// 2. Test square export at 1024x1024
	sess.ExportOptions.SquareSize = 1024
	frames1024, err := sess.RenderExportFrames()
	if err != nil {
		t.Fatalf("RenderExportFrames(1024) failed: %v", err)
	}
	for i, f := range frames1024 {
		b := f.Image.Bounds()
		if b.Dx() != 1024 || b.Dy() != 1024 {
			t.Errorf("frame %d expected 1024x1024, got %dx%d", i, b.Dx(), b.Dy())
		}
	}

	// 3. Test non-square custom resolution
	sess.ExportOptions.ExportSquare = false
	sess.ExportOptions.TargetWidth = 384
	sess.ExportOptions.TargetHeight = 192
	framesCustom, err := sess.RenderExportFrames()
	if err != nil {
		t.Fatalf("RenderExportFrames(non-square) failed: %v", err)
	}
	for i, f := range framesCustom {
		b := f.Image.Bounds()
		if b.Dx() != 384 || b.Dy() != 192 {
			t.Errorf("frame %d expected 384x192, got %dx%d", i, b.Dx(), b.Dy())
		}
	}

	// 4. Test file export
	tmpDir := t.TempDir()
	outGIF := filepath.Join(tmpDir, "exported_512.gif")
	sess.ExportOptions.ExportSquare = true
	sess.ExportOptions.SquareSize = 512
	sizeBytes, err := sess.ExportGIF(outGIF)
	if err != nil {
		t.Fatalf("ExportGIF failed: %v", err)
	}
	if sizeBytes <= 0 {
		t.Errorf("expected positive exported file size, got %d", sizeBytes)
	}
	fi, err := os.Stat(outGIF)
	if err != nil || fi.Size() == 0 {
		t.Fatalf("exported file does not exist or is empty: %v", err)
	}
}

func TestSessionCropBoundaryModes(t *testing.T) {
	testSVGPath, err := filepath.Abs("../../testdata/multipage_walk.svg")
	if err != nil {
		t.Fatalf("failed to resolve test SVG path: %v", err)
	}

	sess := NewSession()
	if err := sess.LoadSVG(testSVGPath); err != nil {
		t.Fatalf("LoadSVG failed: %v", err)
	}

	// multipage_walk.svg has 2 pages (each 256x256), doc is 560x256
	// Default mode for multipage_walk is ModePages
	if sess.CurrentMode != "pages" {
		t.Errorf("expected default mode pages, got %s", sess.CurrentMode)
	}
	if sess.CropBoundaryMode != "page" {
		t.Errorf("expected default boundary mode page, got %s", sess.CropBoundaryMode)
	}

	// Active boundary dimensions should be 256x256
	w, h := sess.GetActiveBoundaryDimensions()
	if w != 256 || h != 256 {
		t.Errorf("expected 256x256 boundary, got %fx%f", w, h)
	}

	// Switch to Document boundary in Page mode
	if err := sess.SetCropBoundary("document", 0); err != nil {
		t.Fatalf("SetCropBoundary document failed: %v", err)
	}
	docW, docH := sess.GetActiveBoundaryDimensions()
	if docW != 560 || docH != 256 {
		t.Errorf("expected 560x256 document boundary, got %fx%f", docW, docH)
	}

	// Switch to Layers mode (character_walk.svg)
	charSVGPath, err := filepath.Abs("../../testdata/character_walk.svg")
	if err != nil {
		t.Fatalf("failed to resolve character_walk path: %v", err)
	}
	sessLayers := NewSession()
	if err := sessLayers.LoadSVG(charSVGPath); err != nil {
		t.Fatalf("LoadSVG character_walk failed: %v", err)
	}
	if sessLayers.CurrentMode != "layers" {
		t.Errorf("expected layers mode, got %s", sessLayers.CurrentMode)
	}
	if sessLayers.CropBoundaryMode != "document" {
		t.Errorf("expected document boundary for layers, got %s", sessLayers.CropBoundaryMode)
	}
	cW, cH := sessLayers.GetActiveBoundaryDimensions()
	if cW != 256 || cH != 256 {
		t.Errorf("expected 256x256 for character_walk, got %fx%f", cW, cH)
	}
}



