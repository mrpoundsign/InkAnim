package app

import (
	"errors"
	"fmt"
	"image"
	"image/draw"
	"io"
	"log"
	"math"
	"os"
	"sync"
	"time"

	"inkanim/internal/gif"
	"inkanim/internal/parallel"
	"inkanim/internal/prof"
	"inkanim/pkg/inksvg"
)

// Session manages the loaded SVG, frame pipeline, preview buffers, and export settings.
type Session struct {
	FilePath          string
	Document          *inksvg.SVGDocument
	CurrentMode       inksvg.FrameMode
	CropBoundaryMode  inksvg.BoundaryMode
	CropPageIndex     int
	FPS               int
	TotalDurationMs   int
	HasManualDuration bool
	Frames            []inksvg.Frame
	Pages             []inksvg.Page
	ExportOptions     gif.ExportOptions
	RenderedFrames    []inksvg.RenderedFrame
	ShowMotionLines   bool

	frameCache map[string][]inksvg.RenderedFrame
	cacheMu    sync.RWMutex
	renderGen  int
}

// NewSession creates an empty session with default options.
func NewSession() *Session {
	return &Session{
		CurrentMode:      inksvg.ModeTimeline,
		CropBoundaryMode: inksvg.BoundaryPage,
		CropPageIndex:    0,
		FPS:              10,
		TotalDurationMs:  1000,
		ExportOptions:    gif.DefaultOptions(),
		frameCache:       make(map[string][]inksvg.RenderedFrame),
	}
}

// ResetManualDuration clears the manual duration override so the next loaded SVG
// derives its initial duration from its keyframe count and the active FPS.
func (s *Session) ResetManualDuration() {
	s.HasManualDuration = false
}

// KeyframeCount returns the number of keyframes defined in the SVG document (at least 1).
func (s *Session) KeyframeCount() int {
	if s.Document == nil {
		return 1
	}
	return s.Document.FrameCount()
}

// IsAnimated reports whether the loaded SVG has any motion paths or multiple keyframes.
func (s *Session) IsAnimated() bool {
	if s.Document == nil {
		return false
	}
	return len(s.Document.MotionPaths) > 0 || s.Document.FrameCount() > 1
}

// TargetFrameCount returns the number of interpolated output frames for the current
// TotalDurationMs and FPS (or 1 if the SVG is static).
func (s *Session) TargetFrameCount() int {
	if s.Document == nil || !s.IsAnimated() {
		return 1
	}
	fps := s.FPS
	if fps <= 0 {
		fps = 10
	}
	durMs := s.TotalDurationMs
	if durMs <= 0 {
		durMs = int(math.Round(float64(s.KeyframeCount()) * 1000.0 / float64(fps)))
	}
	count := max(int(math.Round(float64(durMs)*float64(fps)/1000.0)), 2)
	return count
}

func (s *Session) rebuildTimelineFrames() {
	count := s.TargetFrameCount()
	dur := s.ExportOptions.DefaultDurationMs
	if dur <= 0 {
		dur = 100
	}
	s.Frames = make([]inksvg.Frame, count)
	for i := range count {
		s.Frames[i] = inksvg.Frame{
			ID:         fmt.Sprintf("frame_%d", i+1),
			Label:      fmt.Sprintf("Frame %d", i+1),
			DurationMs: dur,
			IsActive:   true,
		}
	}
}

// SetFPS updates the target frame rate (1..60 FPS), updates per-frame delay,
// and re-renders the interpolated timeline frames.
func (s *Session) SetFPS(fps int) error {
	if fps < 1 {
		fps = 1
	}
	if fps > 60 {
		fps = 60
	}
	s.FPS = fps
	delayMs := max(int(math.Round(1000.0/float64(fps))), 10)
	s.ExportOptions.DefaultDurationMs = delayMs
	if s.Document == nil {
		return nil
	}
	s.rebuildTimelineFrames()
	s.InvalidateCache()
	return s.RerenderAllFrames()
}

// SetTotalDurationMs updates the total animation duration in milliseconds (100..60000ms),
// marks duration as manually set, and re-renders the interpolated timeline frames.
func (s *Session) SetTotalDurationMs(ms int) error {
	if ms < 100 {
		ms = 100
	}
	if ms > 60000 {
		ms = 60000
	}
	s.TotalDurationMs = ms
	s.HasManualDuration = true
	if s.Document == nil {
		return nil
	}
	s.rebuildTimelineFrames()
	s.InvalidateCache()
	return s.RerenderAllFrames()
}

// SetTotalDurationSec updates the total animation duration in seconds (0.1..60.0s).
func (s *Session) SetTotalDurationSec(sec float64) error {
	return s.SetTotalDurationMs(int(math.Round(sec * 1000.0)))
}

func (s *Session) activeCacheKey() string {
	return s.boundaryCacheKey(s.CropBoundaryMode, s.CropPageIndex)
}

func (s *Session) boundaryCacheKey(mode inksvg.BoundaryMode, pIdx int) string {
	if mode == inksvg.BoundaryDrawing {
		return "drawing"
	}
	return fmt.Sprintf("page:%d", pIdx)
}

// InvalidateCache clears all cached frames and advances the render generation.
func (s *Session) InvalidateCache() {
	s.cacheMu.Lock()
	s.renderGen++
	s.frameCache = make(map[string][]inksvg.RenderedFrame)
	s.cacheMu.Unlock()
}

// IsPageCached reports whether the specified boundary has already been pre-rendered into the frame cache.
func (s *Session) IsPageCached(mode inksvg.BoundaryMode, pageIndex int) bool {
	s.cacheMu.RLock()
	defer s.cacheMu.RUnlock()
	key := s.boundaryCacheKey(mode, pageIndex)
	_, ok := s.frameCache[key]
	return ok
}

// LoadSVG loads and parses an SVG file from disk.
func (s *Session) LoadSVG(filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read SVG file: %w", err)
	}
	return s.LoadSVGData(data, filePath)
}

// LoadSVGData parses an SVG from in-memory byte slice with a filename.
func (s *Session) LoadSVGData(data []byte, filename string) error {
	t0 := time.Now()
	doc, err := inksvg.ParseSVG(data)
	if err != nil {
		return fmt.Errorf("failed to parse SVG: %w", err)
	}
	prof.Since(t0, "L1 ParseSVG (%s)", filename)

	s.FilePath = filename
	doc.ShowMotionLines = s.ShowMotionLines
	s.Document = doc
	if s.FPS <= 0 {
		s.FPS = 10
	}
	if !s.HasManualDuration || s.TotalDurationMs <= 0 {
		s.TotalDurationMs = max(int(math.Round(float64(doc.FrameCount())*1000.0/float64(s.FPS))), 100)
	}
	s.rebuildTimelineFrames()

	s.Pages = make([]inksvg.Page, len(doc.Pages))
	copy(s.Pages, doc.Pages)

	s.CurrentMode = inksvg.ModeTimeline
	s.CropBoundaryMode = inksvg.BoundaryPage
	s.CropPageIndex = 0

	s.InvalidateCache()

	tRerender := time.Now()
	if err := s.RerenderAllFrames(); err != nil {
		return err
	}
	prof.Since(tRerender, "Active boundary RerenderAllFrames total")

	return nil
}

// SetShowMotionLines toggles visibility of dashed motion lines in preview frames.
func (s *Session) SetShowMotionLines(show bool) error {
	s.ShowMotionLines = show
	if s.Document != nil {
		s.Document.ShowMotionLines = show
	}
	s.InvalidateCache()
	return s.RerenderAllFrames()
}

// SetCropBoundary sets the boundary mode ("drawing" or "page") and target page index, then rerenders.
// pageIndex 0 represents "Document" (root viewBox), and pageIndex 1..N represent Pages[0..N-1].
func (s *Session) SetCropBoundary(mode inksvg.BoundaryMode, pageIndex int) error {
	s.CropBoundaryMode = mode
	if pageIndex < 0 {
		pageIndex = 0
	}
	if s.Document != nil && pageIndex > len(s.Pages) {
		pageIndex = 0
	}
	s.CropPageIndex = pageIndex
	return s.RerenderAllFrames()
}

func (s *Session) resolvePageCropRect(pIdx int) inksvg.Rect {
	if s.Document == nil {
		return inksvg.Rect{X: 0, Y: 0, Width: 512, Height: 512}
	}
	// pIdx == 0 corresponds to "Document" (root viewBox)
	if pIdx <= 0 || len(s.Pages) == 0 {
		return s.Document.GetDocumentRect()
	}
	pageIdx := pIdx - 1
	if pageIdx >= 0 && pageIdx < len(s.Pages) {
		rect, ok := s.Document.GetPageRect(pageIdx)
		if ok && rect.Width > 0 && rect.Height > 0 {
			return rect
		}
	}
	return s.Document.GetDocumentRect()
}

// GetActiveBoundaryDimensions returns the width and height of the active crop boundary.
func (s *Session) GetActiveBoundaryDimensions() (float64, float64) {
	if s.Document == nil {
		return 512, 512
	}

	if s.CropBoundaryMode == inksvg.BoundaryDrawing {
		rect := s.Document.GetDrawingRect()
		return rect.Width, rect.Height
	}

	rect := s.resolvePageCropRect(s.CropPageIndex)
	return rect.Width, rect.Height
}

// GetActiveBoundaryRect returns the bounding rectangle for the active crop boundary.
func (s *Session) GetActiveBoundaryRect(frameIndex int) inksvg.Rect {
	if s.Document == nil {
		return inksvg.Rect{X: 0, Y: 0, Width: 512, Height: 512}
	}

	if s.CropBoundaryMode == inksvg.BoundaryDrawing {
		return s.Document.GetDrawingRect()
	}

	return s.resolvePageCropRect(s.CropPageIndex)
}

// GetPreviewBoundaryRect returns the base canvas boundary for interactive preview rendering.
// It returns the active crop boundary rectangle, ensuring the selected scene or artboard is
// rendered at maximum resolution.
func (s *Session) GetPreviewBoundaryRect() inksvg.Rect {
	if s.Document == nil {
		return inksvg.Rect{X: 0, Y: 0, Width: 512, Height: 512}
	}
	return s.GetActiveBoundaryRect(0)
}

// ToggleFrameActive toggles whether a frame is included in the animation.
func (s *Session) ToggleFrameActive(index int) error {
	if index < 0 || index >= len(s.Frames) {
		return fmt.Errorf("invalid frame index %d", index)
	}
	s.Frames[index].IsActive = !s.Frames[index].IsActive
	s.InvalidateCache()
	if err := s.RerenderAllFrames(); err != nil {
		return err
	}
	s.PrerenderPages()
	return nil
}

// MoveFrame moves a frame up or down in the animation order.
func (s *Session) MoveFrame(fromIndex, toIndex int) error {
	if fromIndex < 0 || fromIndex >= len(s.Frames) || toIndex < 0 || toIndex >= len(s.Frames) {
		return fmt.Errorf("invalid move indices: %d -> %d", fromIndex, toIndex)
	}
	s.Frames[fromIndex], s.Frames[toIndex] = s.Frames[toIndex], s.Frames[fromIndex]
	s.InvalidateCache()
	if err := s.RerenderAllFrames(); err != nil {
		return err
	}
	s.PrerenderPages()
	return nil
}

// SetGlobalDuration updates the fallback/global frame duration and updates all non-overridden frames.
func (s *Session) SetGlobalDuration(ms int) {
	if ms <= 0 {
		ms = 100
	}
	s.ExportOptions.DefaultDurationMs = ms
	s.UpdateFrameDurations()
}

// SetFrameOverride enables or disables a custom duration override for a frame.
func (s *Session) SetFrameOverride(index int, hasOverride bool, ms int) {
	if index >= 0 && index < len(s.Frames) {
		s.Frames[index].HasOverride = hasOverride
		if hasOverride && ms > 0 {
			s.Frames[index].OverrideMs = ms
			s.Frames[index].DurationMs = ms
		}
	}
	s.UpdateFrameDurations()
}

// UpdateFrameDurations synchronizes the effective duration on all RenderedFrames and cached frames without re-rasterizing.
func (s *Session) UpdateFrameDurations() {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	updateList := func(frames []inksvg.RenderedFrame) {
		frameIdx := 0
		for _, frame := range s.Frames {
			if !frame.IsActive {
				continue
			}
			if frameIdx < len(frames) {
				eff := frame.EffectiveDuration(s.ExportOptions.DefaultDurationMs)
				frames[frameIdx].DurationMs = eff
				frameIdx++
			}
		}
	}

	updateList(s.RenderedFrames)
	for _, cached := range s.frameCache {
		updateList(cached)
	}
}

// renderFramesForBoundary rasterizes the active frames into preview frames at the specified boundary rectangle.
func (s *Session) renderFramesForBoundary(boundaryRect inksvg.Rect) ([]inksvg.RenderedFrame, error) {
	if s.Document == nil {
		return nil, nil
	}

	boundW := boundaryRect.Width
	boundH := boundaryRect.Height
	if boundW <= 0 {
		boundW = 512
	}
	if boundH <= 0 {
		boundH = 512
	}
	maxDim := boundW
	if boundH > maxDim {
		maxDim = boundH
	}
	const maxPreviewDim = 512.0
	previewScale := maxPreviewDim / maxDim
	renderW := int(math.Round(boundW * previewScale))
	renderH := int(math.Round(boundH * previewScale))

	type frameJob struct {
		frameIdx    int
		docFrameIdx int
		frame       inksvg.Frame
	}
	var jobs []frameJob

	for i, frame := range s.Frames {
		if !frame.IsActive {
			continue
		}
		jobs = append(jobs, frameJob{
			frameIdx:    len(jobs),
			docFrameIdx: i,
			frame:       frame,
		})
	}

	totalOutFrames := len(s.Frames)
	keyframeCount := s.KeyframeCount()

	if inksvg.CustomBatchRasterizer != nil && len(jobs) > 0 {
		tBuild := time.Now()
		svgList := make([][]byte, len(jobs))
		var buildErr error
		for i, j := range jobs {
			var frameSVG []byte
			kfPos := inksvg.KeyframePosition(j.docFrameIdx, totalOutFrames, keyframeCount)
			frameSVG, buildErr = inksvg.BuildTimelineFrameSVGAt(s.Document, kfPos, boundaryRect)
			if buildErr != nil {
				return nil, fmt.Errorf("failed to build frame %d: %w", j.frameIdx+1, buildErr)
			}
			svgList[i] = frameSVG
		}
		prof.Since(tBuild, "L2 BuildTimelineFrameSVG (%d frames)", len(jobs))

		tRaster := time.Now()
		images, err := inksvg.CustomBatchRasterizer(svgList, renderW, renderH)
		prof.Since(tRaster, "L4 CustomBatchRasterizer call (%d frames)", len(jobs))
		if err == nil && len(images) == len(jobs) {
			frames := make([]inksvg.RenderedFrame, len(jobs))
			for i, j := range jobs {
				dur := j.frame.EffectiveDuration(s.ExportOptions.DefaultDurationMs)
				frames[j.frameIdx] = inksvg.RenderedFrame{
					Index:      j.docFrameIdx,
					Label:      j.frame.Label,
					Image:      images[i],
					DurationMs: dur,
				}
			}
			return frames, nil
		}
		if err != nil {
			log.Printf("CustomBatchRasterizer failed: %v (falling back to per-frame rasterization)", err)
		}
	}

	frames := make([]inksvg.RenderedFrame, len(jobs))
	err := parallel.Run(len(jobs), func(idx int) error {
		j := jobs[idx]

		kfPos := inksvg.KeyframePosition(j.docFrameIdx, totalOutFrames, keyframeCount)
		frameSVG, err := inksvg.BuildTimelineFrameSVGAt(s.Document, kfPos, boundaryRect)
		if err != nil {
			return fmt.Errorf("failed to build frame %d: %w", j.frameIdx+1, err)
		}

		img, err := inksvg.RenderSVGToRGBA(frameSVG, renderW, renderH)
		if err != nil {
			return fmt.Errorf("failed to render frame %s: %w", j.frame.Label, err)
		}

		dur := j.frame.EffectiveDuration(s.ExportOptions.DefaultDurationMs)
		frames[j.frameIdx] = inksvg.RenderedFrame{
			Index:      j.docFrameIdx,
			Label:      j.frame.Label,
			Image:      img,
			DurationMs: dur,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return frames, nil
}

// RerenderAllFrames rasterizes all active frames for the current mode into preview buffers.
// For small source SVGs (<512px), preview rasterization scales up to 512px so that live playback is sharp.
func (s *Session) RerenderAllFrames() error {
	if s.Document == nil {
		s.RenderedFrames = nil
		return nil
	}

	key := s.activeCacheKey()
	s.cacheMu.RLock()
	if cached, ok := s.frameCache[key]; ok && len(cached) > 0 {
		frames := make([]inksvg.RenderedFrame, len(cached))
		copy(frames, cached)
		s.cacheMu.RUnlock()
		s.RenderedFrames = frames
		s.UpdateFrameDurations()
		return nil
	}
	s.cacheMu.RUnlock()

	boundaryRect := s.GetActiveBoundaryRect(0)
	frames, err := s.renderFramesForBoundary(boundaryRect)
	if err != nil {
		return err
	}

	s.cacheMu.Lock()
	if s.frameCache == nil {
		s.frameCache = make(map[string][]inksvg.RenderedFrame)
	}
	s.frameCache[key] = frames
	s.cacheMu.Unlock()

	s.RenderedFrames = frames
	return nil
}

// PrerenderPagesSync rasterizes and caches frames for drawing and all pages synchronously.
func (s *Session) PrerenderPagesSync() {
	if s.Document == nil {
		return
	}
	tPrerender := time.Now()
	prof.Logf("A1 PrerenderPagesSync started")
	defer prof.Since(tPrerender, "A1 PrerenderPagesSync finished")

	s.cacheMu.RLock()
	currentGen := s.renderGen
	_, hasDrawing := s.frameCache["drawing"]
	s.cacheMu.RUnlock()

	if !hasDrawing {
		drawingRect := s.Document.GetDrawingRect()
		frames, err := s.renderFramesForBoundary(drawingRect)
		if err == nil {
			s.cacheMu.Lock()
			if s.renderGen == currentGen {
				if s.frameCache == nil {
					s.frameCache = make(map[string][]inksvg.RenderedFrame)
				}
				s.frameCache["drawing"] = frames
			}
			s.cacheMu.Unlock()
		}
	}

	for pIdx := 0; pIdx <= len(s.Pages); pIdx++ {
		s.cacheMu.RLock()
		if s.renderGen != currentGen {
			s.cacheMu.RUnlock()
			return
		}
		key := fmt.Sprintf("page:%d", pIdx)
		_, cached := s.frameCache[key]
		s.cacheMu.RUnlock()
		if cached {
			continue
		}

		pageRect := s.resolvePageCropRect(pIdx)
		frames, err := s.renderFramesForBoundary(pageRect)
		if err != nil {
			continue
		}

		s.cacheMu.Lock()
		if s.renderGen == currentGen {
			if s.frameCache == nil {
				s.frameCache = make(map[string][]inksvg.RenderedFrame)
			}
			s.frameCache[key] = frames
		}
		s.cacheMu.Unlock()
	}
}

// PrerenderPages rasterizes and caches frames for all pages in the background.
func (s *Session) PrerenderPages() {
	go s.PrerenderPagesSync()
}

// ExportDimensions holds the rasterization dimensions (FitWidth, FitHeight)
// and the canvas/output dimensions (OutWidth, OutHeight).
type ExportDimensions struct {
	FitWidth  int
	FitHeight int
	OutWidth  int
	OutHeight int
	IsSquare  bool
}

// ResolveExportDimensions computes the SVG rasterization dimensions and the final
// export canvas dimensions based on the active boundary mode and ExportOptions.
func (s *Session) ResolveExportDimensions() ExportDimensions {
	boundW, boundH := s.GetActiveBoundaryDimensions()
	if boundW <= 0 {
		boundW = 512
	}
	if boundH <= 0 {
		boundH = 512
	}

	var isSquare bool
	var outW, outH int
	var fitW, fitH int

	if s.ExportOptions.ExportSquare {
		isSquare = true
		targetSquare := s.ExportOptions.SquareSize
		if targetSquare <= 0 {
			maxSide := boundW
			if boundH > maxSide {
				maxSide = boundH
			}
			switch {
			case maxSide < 512:
				targetSquare = 512
			case maxSide > 4096:
				targetSquare = 4096
			default:
				targetSquare = int(math.Round(maxSide))
			}
		}
		if targetSquare > 4096 {
			targetSquare = 4096
		}

		maxSide := boundW
		if boundH > maxSide {
			maxSide = boundH
		}
		scale := float64(targetSquare) / maxSide
		fitW = int(math.Round(boundW * scale))
		fitH = int(math.Round(boundH * scale))
		outW, outH = targetSquare, targetSquare
	} else {
		if s.ExportOptions.TargetWidth > 0 && s.ExportOptions.TargetHeight > 0 {
			fitW = s.ExportOptions.TargetWidth
			fitH = s.ExportOptions.TargetHeight
			if fitW > 4096 {
				fitW = 4096
			}
			if fitH > 4096 {
				fitH = 4096
			}
		} else {
			maxSide := boundW
			if boundH > maxSide {
				maxSide = boundH
			}
			switch {
			case maxSide < 512:
				scale := 512.0 / maxSide
				fitW = int(math.Round(boundW * scale))
				fitH = int(math.Round(boundH * scale))
			case maxSide > 4096:
				scale := 4096.0 / maxSide
				fitW = int(math.Round(boundW * scale))
				fitH = int(math.Round(boundH * scale))
			default:
				fitW = int(math.Round(boundW))
				fitH = int(math.Round(boundH))
			}
		}
		outW, outH = fitW, fitH
	}

	if fitW < 1 {
		fitW = 1
	}
	if fitH < 1 {
		fitH = 1
	}
	if outW < 1 {
		outW = 1
	}
	if outH < 1 {
		outH = 1
	}

	return ExportDimensions{
		FitWidth:  fitW,
		FitHeight: fitH,
		OutWidth:  outW,
		OutHeight: outH,
		IsSquare:  isSquare,
	}
}

// RenderExportFrames rasterizes the active animation frames directly from vector SVG data
// at the target export resolution (up to 4096px), eliminating bitmap scaling blur.
func (s *Session) RenderExportFrames() ([]gif.FrameInput, error) {
	if s.Document == nil {
		return nil, errors.New("no SVG document loaded")
	}

	exportDoc := s.Document.CloneWithoutMotionLines()

	boundaryRect := s.GetActiveBoundaryRect(0)
	dims := s.ResolveExportDimensions()
	fitW, fitH := dims.FitWidth, dims.FitHeight
	exportSquare := dims.IsSquare
	targetSquare := dims.OutWidth

	type frameExportJob struct {
		frameIdx    int
		docFrameIdx int
		frame       inksvg.Frame
	}
	var jobs []frameExportJob

	for i, frame := range s.Frames {
		if !frame.IsActive {
			continue
		}
		jobs = append(jobs, frameExportJob{
			frameIdx:    len(jobs),
			docFrameIdx: i,
			frame:       frame,
		})
	}

	totalOutFrames := len(s.Frames)
	keyframeCount := s.KeyframeCount()

	frameInputs := make([]gif.FrameInput, len(jobs))
	err := parallel.Run(len(jobs), func(idx int) error {
		j := jobs[idx]

		kfPos := inksvg.KeyframePosition(j.docFrameIdx, totalOutFrames, keyframeCount)
		frameSVG, err := inksvg.BuildTimelineFrameSVGAt(exportDoc, kfPos, boundaryRect)
		if err != nil {
			return fmt.Errorf("failed to build frame %d: %w", j.frameIdx+1, err)
		}

		rawImg, err := inksvg.RenderSVGToRGBA(frameSVG, fitW, fitH)
		if err != nil {
			return fmt.Errorf("failed to rasterize frame %s at %dx%d: %w", j.frame.Label, fitW, fitH, err)
		}

		var finalImg *image.RGBA
		if exportSquare {
			dst := image.NewRGBA(image.Rect(0, 0, targetSquare, targetSquare))
			offsetX := (targetSquare - fitW) / 2
			offsetY := (targetSquare - fitH) / 2
			dstRect := image.Rect(offsetX, offsetY, offsetX+fitW, offsetY+fitH)
			draw.Draw(dst, dstRect, rawImg, rawImg.Bounds().Min, draw.Src)
			finalImg = dst
		} else {
			finalImg = rawImg
		}

		dur := j.frame.EffectiveDuration(s.ExportOptions.DefaultDurationMs)
		frameInputs[j.frameIdx] = gif.FrameInput{
			Index:      j.docFrameIdx,
			Label:      j.frame.Label,
			Image:      finalImg,
			DurationMs: dur,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return frameInputs, nil
}

// ExportGIFWriter exports the active frames to an io.Writer with direct high-resolution vector rasterization.
func (s *Session) ExportGIFWriter(w io.Writer) (int64, error) {
	frameInputs, err := s.RenderExportFrames()
	if err != nil {
		return 0, err
	}
	if len(frameInputs) == 0 {
		return 0, errors.New("no frames available to export")
	}

	// Since frames are already vector-rasterized and squared at final target dimensions,
	// pass options with ExportSquare disabled to avoid redundant bitmap re-scaling.
	exportOpts := s.ExportOptions
	exportOpts.ExportSquare = false
	if len(frameInputs) > 0 && frameInputs[0].Image != nil {
		b := frameInputs[0].Image.Bounds()
		exportOpts.TargetWidth = b.Dx()
		exportOpts.TargetHeight = b.Dy()
	}

	return gif.WriteGIFToWriter(w, frameInputs, exportOpts)
}

// ExportGIF exports the active frames to a single animated GIF at destinationPath with direct high-resolution vector rasterization.
func (s *Session) ExportGIF(destinationPath string) (int64, error) {
	frameInputs, err := s.RenderExportFrames()
	if err != nil {
		return 0, err
	}
	if len(frameInputs) == 0 {
		return 0, errors.New("no frames available to export")
	}

	exportOpts := s.ExportOptions
	exportOpts.ExportSquare = false
	if len(frameInputs) > 0 && frameInputs[0].Image != nil {
		b := frameInputs[0].Image.Bounds()
		exportOpts.TargetWidth = b.Dx()
		exportOpts.TargetHeight = b.Dy()
	}

	return gif.WriteGIFToFile(destinationPath, frameInputs, exportOpts)
}
