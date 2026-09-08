package app

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"inkanim/internal/svg"
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
	// Default mode is always ModeLayers, CropBoundaryMode is BoundaryDrawing
	if sess.CurrentMode != svg.ModeLayers {
		t.Errorf("expected default mode layers, got %s", sess.CurrentMode)
	}
	if sess.CropBoundaryMode != svg.BoundaryDrawing {
		t.Errorf("expected default boundary mode drawing, got %s", sess.CropBoundaryMode)
	}

	// Switch to Page boundary mode for Page 1 (pageIndex 1 = Page 1)
	if err := sess.SetCropBoundary(svg.BoundaryPage, 1); err != nil {
		t.Fatalf("SetCropBoundary page 1 failed: %v", err)
	}
	// Active boundary dimensions should be 256x256
	w, h := sess.GetActiveBoundaryDimensions()
	if w != 256 || h != 256 {
		t.Errorf("expected 256x256 boundary, got %fx%f", w, h)
	}

	// Switch to Document boundary in Page mode (pageIndex 0 = Document)
	if err := sess.SetCropBoundary(svg.BoundaryPage, 0); err != nil {
		t.Fatalf("SetCropBoundary page/document failed: %v", err)
	}
	docW, docH := sess.GetActiveBoundaryDimensions()
	if docW != 560 || docH != 256 {
		t.Errorf("expected 560x256 document boundary, got %fx%f", docW, docH)
	}

	// Switch to Drawing boundary mode
	if err := sess.SetCropBoundary(svg.BoundaryDrawing, 0); err != nil {
		t.Fatalf("SetCropBoundary drawing failed: %v", err)
	}
	drawW, drawH := sess.GetActiveBoundaryDimensions()
	if drawW <= 0 || drawH <= 0 {
		t.Errorf("expected positive dimensions for drawing boundary, got %fx%f", drawW, drawH)
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
	if sessLayers.CurrentMode != svg.ModeLayers {
		t.Errorf("expected layers mode, got %s", sessLayers.CurrentMode)
	}
	if sessLayers.CropBoundaryMode != svg.BoundaryDrawing {
		t.Errorf("expected drawing boundary for layers, got %s", sessLayers.CropBoundaryMode)
	}

	// Test bouncing_walker.svg with Drawing vs Page (Document vs Focus Page)
	bouncePath, err := filepath.Abs("../../testdata/bouncing_walker.svg")
	if err != nil {
		t.Fatalf("failed to resolve bouncing_walker path: %v", err)
	}
	sessBounce := NewSession()
	if err := sessBounce.LoadSVG(bouncePath); err != nil {
		t.Fatalf("LoadSVG bouncing_walker failed: %v", err)
	}
	// Default mode for bouncing_walker is ModeLayers, CropBoundaryMode is BoundaryDrawing (unclipped, width >= 350)
	bDrawW, _ := sessBounce.GetActiveBoundaryDimensions()
	if bDrawW < 350 {
		t.Errorf("expected drawing width >= 350 in bouncing_walker, got %f", bDrawW)
	}

	// Select Page mode with Document (index 0)
	if err := sessBounce.SetCropBoundary(svg.BoundaryPage, 0); err != nil {
		t.Fatalf("SetCropBoundary Page Document failed: %v", err)
	}
	bDocW, bDocH := sessBounce.GetActiveBoundaryDimensions()
	if bDocW != 256 || bDocH != 256 {
		t.Errorf("expected 256x256 for Document page option, got %fx%f", bDocW, bDocH)
	}

	// Select Page mode with Page 2 (index 2: Focus 160x160)
	if err := sessBounce.SetCropBoundary(svg.BoundaryPage, 2); err != nil {
		t.Fatalf("SetCropBoundary Page 2 failed: %v", err)
	}
	bP2W, bP2H := sessBounce.GetActiveBoundaryDimensions()
	if bP2W != 160 || bP2H != 160 {
		t.Errorf("expected 160x160 for Page 2 option, got %fx%f", bP2W, bP2H)
	}
}

func TestHydrateSessionLoadAndPreviewBounds(t *testing.T) {
	hydratePath, err := filepath.Abs("../../testdata/hydrate.svg")
	if err != nil {
		t.Fatalf("failed to resolve hydrate.svg path: %v", err)
	}

	sess := NewSession()
	if err := sess.LoadSVG(hydratePath); err != nil {
		t.Fatalf("LoadSVG hydrate.svg failed: %v", err)
	}

	// 1. Initial load for layered SVG is in Drawing mode
	if sess.CropBoundaryMode != svg.BoundaryDrawing {
		t.Errorf("expected BoundaryDrawing on load, got %s", sess.CropBoundaryMode)
	}

	// 2. Active boundary in Drawing mode includes 15px stroke width (maxY >= 270.0)
	drawingBound := sess.GetActiveBoundaryRect(0)
	maxY := drawingBound.Y + drawingBound.Height
	if maxY < 270.0 {
		t.Errorf("expected drawing maxY to reach >= 270.0 including stroke, got %f", maxY)
	}

	// 3. In Page mode with Document (index 0), active boundary is Document (210x297)
	if err := sess.SetCropBoundary(svg.BoundaryPage, 0); err != nil {
		t.Fatalf("SetCropBoundary Page Document failed: %v", err)
	}
	activeRect := sess.GetActiveBoundaryRect(0)
	if activeRect.Width != 210 || activeRect.Height != 297 {
		t.Errorf("expected 210x297 active boundary rect in Document mode, got %+v", activeRect)
	}
}

func BenchmarkRerenderAllFrames(b *testing.B) {
	testSVGPath, err := filepath.Abs("../../testdata/bouncing_walker.svg")
	if err != nil {
		b.Fatalf("failed to resolve test SVG path: %v", err)
	}

	sess := NewSession()
	if err := sess.LoadSVG(testSVGPath); err != nil {
		b.Fatalf("failed to load SVG: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := sess.RerenderAllFrames(); err != nil {
			b.Fatalf("RerenderAllFrames failed: %v", err)
		}
	}
}

func BenchmarkRenderExportFrames(b *testing.B) {
	testSVGPath, err := filepath.Abs("../../testdata/bouncing_walker.svg")
	if err != nil {
		b.Fatalf("failed to resolve test SVG path: %v", err)
	}

	sess := NewSession()
	if err := sess.LoadSVG(testSVGPath); err != nil {
		b.Fatalf("failed to load SVG: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		frames, err := sess.RenderExportFrames()
		if err != nil || len(frames) == 0 {
			b.Fatalf("RenderExportFrames failed: %v", err)
		}
	}
}

func TestSessionLoadSVGWithoutLayers(t *testing.T) {
	fixturePath, err := filepath.Abs("../../testdata/fixtures/namedview_pagecolor.svg")
	if err != nil {
		t.Fatalf("failed to resolve fixture path: %v", err)
	}

	sess := NewSession()
	if err := sess.LoadSVG(fixturePath); err != nil {
		t.Fatalf("failed to load fixture SVG: %v", err)
	}

	if len(sess.Layers) != 1 {
		t.Fatalf("expected 1 fallback layer, got %d", len(sess.Layers))
	}
	if len(sess.RenderedFrames) != 1 {
		t.Fatalf("expected 1 rendered frame, got %d", len(sess.RenderedFrames))
	}
	if sess.RenderedFrames[0].Image == nil {
		t.Fatalf("expected non-nil rendered frame image")
	}
}


