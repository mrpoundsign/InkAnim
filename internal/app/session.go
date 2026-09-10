package app

import (
	"errors"
	"fmt"
	"image"
	"image/draw"
	"io"
	"math"
	"os"
	"sync"

	"inkanim/internal/gif"
	"inkanim/internal/parallel"
	"inkanim/pkg/inksvg"
)

// Session manages the loaded SVG, frame pipeline, preview buffers, and export settings.
type Session struct {
	FilePath         string
	Document         *inksvg.SVGDocument
	CurrentMode      inksvg.FrameMode
	CropBoundaryMode inksvg.BoundaryMode
	CropPageIndex    int
	Layers           []inksvg.Layer
	Pages            []inksvg.Page
	ExportOptions    gif.ExportOptions
	RenderedFrames   []inksvg.RenderedFrame
	PinnedLayers     map[string]bool

	frameCache map[string][]inksvg.RenderedFrame
	cacheMu    sync.RWMutex
	renderGen  int
}

// NewSession creates an empty session with default options.
func NewSession() *Session {
	return &Session{
		CurrentMode:      inksvg.ModeLayers,
		CropBoundaryMode: inksvg.BoundaryDrawing,
		CropPageIndex:    0,
		ExportOptions:    gif.DefaultOptions(),
		PinnedLayers:     make(map[string]bool),
		frameCache:       make(map[string][]inksvg.RenderedFrame),
	}
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

func (s *Session) invalidateCache() {
	s.InvalidateCache()
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
	doc, err := inksvg.ParseSVG(data)
	if err != nil {
		return fmt.Errorf("failed to parse SVG: %w", err)
	}

	s.FilePath = filename
	s.Document = doc
	s.Layers = make([]inksvg.Layer, len(doc.Layers))
	copy(s.Layers, doc.Layers)

	s.Pages = make([]inksvg.Page, len(doc.Pages))
	copy(s.Pages, doc.Pages)

	s.CurrentMode = inksvg.ModeLayers
	s.CropBoundaryMode = inksvg.BoundaryDrawing
	s.CropPageIndex = 0
	s.PinnedLayers = make(map[string]bool)

	s.invalidateCache()

	if err := s.RerenderAllFrames(); err != nil {
		return err
	}

	s.PrerenderPages()
	return nil
}

// SetMode sets the animation mode (always inksvg.ModeLayers; pages serve as artboard crop boundaries).
func (s *Session) SetMode(mode inksvg.FrameMode) error {
	s.CurrentMode = inksvg.ModeLayers
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


// ToggleLayerActive toggles whether a layer is included as a frame.
func (s *Session) ToggleLayerActive(index int) error {
	if index < 0 || index >= len(s.Layers) {
		return fmt.Errorf("invalid layer index %d", index)
	}
	s.Layers[index].IsActive = !s.Layers[index].IsActive
	s.invalidateCache()
	if err := s.RerenderAllFrames(); err != nil {
		return err
	}
	s.PrerenderPages()
	return nil
}

// ToggleLayerPinned toggles whether a layer is pinned as a background across all frames.
func (s *Session) ToggleLayerPinned(index int) error {
	if index < 0 || index >= len(s.Layers) {
		return fmt.Errorf("invalid layer index %d", index)
	}
	s.Layers[index].IsPinned = !s.Layers[index].IsPinned
	if s.Layers[index].IsPinned {
		s.PinnedLayers[s.Layers[index].ID] = true
	} else {
		delete(s.PinnedLayers, s.Layers[index].ID)
	}
	s.invalidateCache()
	if err := s.RerenderAllFrames(); err != nil {
		return err
	}
	s.PrerenderPages()
	return nil
}

// MoveLayer moves a layer up or down in the animation order.
func (s *Session) MoveLayer(fromIndex, toIndex int) error {
	if fromIndex < 0 || fromIndex >= len(s.Layers) || toIndex < 0 || toIndex >= len(s.Layers) {
		return fmt.Errorf("invalid move indices: %d -> %d", fromIndex, toIndex)
	}
	s.Layers[fromIndex], s.Layers[toIndex] = s.Layers[toIndex], s.Layers[fromIndex]
	s.invalidateCache()
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

// SetFrameOverride enables or disables a custom duration override for a layer.
func (s *Session) SetFrameOverride(index int, hasOverride bool, ms int) {
	if index >= 0 && index < len(s.Layers) {
		s.Layers[index].HasOverride = hasOverride
		if hasOverride && ms > 0 {
			s.Layers[index].OverrideMs = ms
			s.Layers[index].DurationMs = ms
		}
	}
	s.UpdateFrameDurations()
}

// SetFrameDuration sets the override duration (in milliseconds) for a specific frame.
func (s *Session) SetFrameDuration(index int, ms int) {
	s.SetFrameOverride(index, true, ms)
}

// UpdateFrameDurations synchronizes the effective duration on all RenderedFrames and cached frames without re-rasterizing.
func (s *Session) UpdateFrameDurations() {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	updateList := func(frames []inksvg.RenderedFrame) {
		frameIdx := 0
		for _, layer := range s.Layers {
			if !layer.IsActive || layer.IsPinned {
				continue
			}
			if frameIdx < len(frames) {
				eff := layer.EffectiveDuration(s.ExportOptions.DefaultDurationMs)
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

// renderFramesForBoundary rasterizes the active layers into preview frames at the specified boundary rectangle.
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

	type layerJob struct {
		frameIdx int
		layerIdx int
		layer    inksvg.Layer
	}
	var jobs []layerJob

	if s.Document.DefaultMode == inksvg.ModeTimeline {
		for i, layer := range s.Layers {
			if !layer.IsActive {
				continue
			}
			jobs = append(jobs, layerJob{
				frameIdx: len(jobs),
				layerIdx: i,
				layer:    layer,
			})
		}
	} else {
		for i, layer := range s.Layers {
			if !layer.IsActive || layer.IsPinned {
				continue
			}
			jobs = append(jobs, layerJob{
				frameIdx: len(jobs),
				layerIdx: i,
				layer:    layer,
			})
		}
	}

	frames := make([]inksvg.RenderedFrame, len(jobs))
	err := parallel.Run(len(jobs), func(idx int) error {
		j := jobs[idx]
		
		var frameSVG []byte
		var err error
		if s.Document.DefaultMode == inksvg.ModeTimeline {
			frameSVG, err = inksvg.BuildTimelineFrameSVG(s.Document, j.layerIdx, boundaryRect)
		} else {
			frameSVG, err = inksvg.BuildLayerFrameSVG(s.Document, j.layer.ID, s.PinnedLayers, boundaryRect)
		}
		
		if err != nil {
			return fmt.Errorf("failed to build frame for layer %s: %w", j.layer.Label, err)
		}

		img, err := inksvg.RenderSVGToRGBA(frameSVG, renderW, renderH)
		if err != nil {
			return fmt.Errorf("failed to render layer %s: %w", j.layer.Label, err)
		}

		dur := j.layer.EffectiveDuration(s.ExportOptions.DefaultDurationMs)
		frames[j.frameIdx] = inksvg.RenderedFrame{
			Index:      j.layerIdx,
			Label:      j.layer.Label,
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

// RenderExportFrames rasterizes the active animation frames directly from vector SVG data
// at the target export resolution (up to 4096px), eliminating bitmap scaling blur.
func (s *Session) RenderExportFrames() ([]gif.FrameInput, error) {
	if s.Document == nil {
		return nil, errors.New("no SVG document loaded")
	}

	boundW, boundH := s.GetActiveBoundaryDimensions()
	if boundW <= 0 {
		boundW = 512
	}
	if boundH <= 0 {
		boundH = 512
	}
	boundaryRect := s.GetActiveBoundaryRect(0)

	// Calculate export dimensions
	var exportSquare bool
	var targetSquare int
	var fitW, fitH int

	if s.ExportOptions.ExportSquare {
		exportSquare = true
		targetSquare = s.ExportOptions.SquareSize
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
	}

	if fitW < 1 {
		fitW = 1
	}
	if fitH < 1 {
		fitH = 1
	}

	type layerExportJob struct {
		frameIdx int
		layerIdx int
		layer    inksvg.Layer
	}
	var jobs []layerExportJob

	if s.Document.DefaultMode == inksvg.ModeTimeline {
		for i, layer := range s.Layers {
			if !layer.IsActive {
				continue
			}
			jobs = append(jobs, layerExportJob{
				frameIdx: len(jobs),
				layerIdx: i,
				layer:    layer,
			})
		}
	} else {
		for i, layer := range s.Layers {
			if !layer.IsActive || layer.IsPinned {
				continue
			}
			jobs = append(jobs, layerExportJob{
				frameIdx: len(jobs),
				layerIdx: i,
				layer:    layer,
			})
		}
	}

	frameInputs := make([]gif.FrameInput, len(jobs))
	err := parallel.Run(len(jobs), func(idx int) error {
		j := jobs[idx]
		
		var frameSVG []byte
		var err error
		if s.Document.DefaultMode == inksvg.ModeTimeline {
			frameSVG, err = inksvg.BuildTimelineFrameSVG(s.Document, j.layerIdx, boundaryRect)
		} else {
			frameSVG, err = inksvg.BuildLayerFrameSVG(s.Document, j.layer.ID, s.PinnedLayers, boundaryRect)
		}
		
		if err != nil {
			return fmt.Errorf("failed to build frame for layer %s: %w", j.layer.Label, err)
		}

		rawImg, err := inksvg.RenderSVGToRGBA(frameSVG, fitW, fitH)
		if err != nil {
			return fmt.Errorf("failed to rasterize layer %s at %dx%d: %w", j.layer.Label, fitW, fitH, err)
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

		dur := j.layer.EffectiveDuration(s.ExportOptions.DefaultDurationMs)
		frameInputs[j.frameIdx] = gif.FrameInput{
			Index:      j.layerIdx,
			Label:      j.layer.Label,
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
