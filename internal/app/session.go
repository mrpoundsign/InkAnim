package app

import (
	"fmt"
	"image"
	"image/draw"
	"io"
	"math"
	"os"

	"inkanim/internal/gif"
	"inkanim/internal/svg"
)

// Session manages the loaded SVG, frame pipeline, preview buffers, and export settings.
type Session struct {
	FilePath         string
	Document         *svg.SVGDocument
	CurrentMode      svg.FrameMode
	CropBoundaryMode svg.BoundaryMode
	CropPageIndex    int
	Layers           []svg.Layer
	Pages            []svg.Page
	ExportOptions    gif.ExportOptions
	RenderedFrames   []svg.RenderedFrame
	PinnedLayers     map[string]bool
}

// NewSession creates an empty session with default options.
func NewSession() *Session {
	return &Session{
		CurrentMode:      svg.ModeLayers,
		CropBoundaryMode: svg.BoundaryDocument,
		CropPageIndex:    0,
		ExportOptions:    gif.DefaultOptions(),
		PinnedLayers:     make(map[string]bool),
	}
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
	doc, err := svg.ParseSVG(data)
	if err != nil {
		return fmt.Errorf("failed to parse SVG: %w", err)
	}

	s.FilePath = filename
	s.Document = doc
	s.Layers = make([]svg.Layer, len(doc.Layers))
	copy(s.Layers, doc.Layers)

	s.Pages = make([]svg.Page, len(doc.Pages))
	copy(s.Pages, doc.Pages)

	s.CurrentMode = doc.DefaultMode
	if doc.DefaultMode == svg.ModePages {
		s.CropBoundaryMode = svg.BoundaryPage
	} else {
		s.CropBoundaryMode = svg.BoundaryDocument
	}
	s.CropPageIndex = 0
	s.PinnedLayers = make(map[string]bool)

	return s.RerenderAllFrames()
}

// SetMode switches between ModeLayers and ModePages and rerenders.
func (s *Session) SetMode(mode svg.FrameMode) error {
	s.CurrentMode = mode
	if mode == svg.ModePages {
		s.CropBoundaryMode = svg.BoundaryPage
	} else {
		s.CropBoundaryMode = svg.BoundaryDocument
	}
	return s.RerenderAllFrames()
}

// SetCropBoundary sets the boundary mode ("document" or "page") and target page index, then rerenders.
func (s *Session) SetCropBoundary(mode svg.BoundaryMode, pageIndex int) error {
	s.CropBoundaryMode = mode
	if pageIndex < 0 {
		pageIndex = 0
	}
	if s.Document != nil && len(s.Pages) > 0 && pageIndex >= len(s.Pages) {
		pageIndex = 0
	}
	s.CropPageIndex = pageIndex
	return s.RerenderAllFrames()
}

// GetActiveBoundaryDimensions returns the width and height of the active crop boundary.
func (s *Session) GetActiveBoundaryDimensions() (float64, float64) {
	if s.Document == nil {
		return 512, 512
	}

	if s.CurrentMode == svg.ModeLayers {
		if s.CropBoundaryMode == svg.BoundaryPage && len(s.Pages) > 0 {
			pIdx := s.CropPageIndex
			if pIdx < 0 || pIdx >= len(s.Pages) {
				pIdx = 0
			}
			rect, ok := s.Document.GetPageRect(pIdx)
			if ok && rect.Width > 0 && rect.Height > 0 {
				return rect.Width, rect.Height
			}
		}
		docRect := s.Document.GetDocumentRect()
		return docRect.Width, docRect.Height
	}

	// ModePages
	if s.CropBoundaryMode == svg.BoundaryDocument {
		docRect := s.Document.GetDocumentRect()
		return docRect.Width, docRect.Height
	}

	if len(s.Pages) > 0 {
		pIdx := s.CropPageIndex
		if pIdx >= 0 && pIdx < len(s.Pages) {
			rect, ok := s.Document.GetPageRect(pIdx)
			if ok && rect.Width > 0 && rect.Height > 0 {
				return rect.Width, rect.Height
			}
		}
		rect, ok := s.Document.GetPageRect(0)
		if ok && rect.Width > 0 && rect.Height > 0 {
			return rect.Width, rect.Height
		}
	}
	docRect := s.Document.GetDocumentRect()
	return docRect.Width, docRect.Height
}

// GetActiveBoundaryRect returns the bounding rectangle for the given frame index or active crop boundary.
func (s *Session) GetActiveBoundaryRect(frameIndex int) svg.Rect {
	if s.Document == nil {
		return svg.Rect{X: 0, Y: 0, Width: 512, Height: 512}
	}

	if s.CurrentMode == svg.ModeLayers {
		if s.CropBoundaryMode == svg.BoundaryPage && len(s.Pages) > 0 {
			pIdx := s.CropPageIndex
			if pIdx < 0 || pIdx >= len(s.Pages) {
				pIdx = 0
			}
			rect, ok := s.Document.GetPageRect(pIdx)
			if ok {
				return rect
			}
		}
		return s.Document.GetDocumentRect()
	}

	// ModePages
	if s.CropBoundaryMode == svg.BoundaryDocument {
		return s.Document.GetDocumentRect()
	}

	// In ModePages with BoundaryPage:
	// Each page frame clips to its own page artboard coordinates
	if frameIndex >= 0 && frameIndex < len(s.Pages) {
		rect, ok := s.Document.GetPageRect(frameIndex)
		if ok {
			return rect
		}
	}
	return s.Document.GetDocumentRect()
}


// ToggleLayerActive toggles whether a layer is included as a frame.
func (s *Session) ToggleLayerActive(index int) error {
	if index < 0 || index >= len(s.Layers) {
		return fmt.Errorf("invalid layer index %d", index)
	}
	s.Layers[index].IsActive = !s.Layers[index].IsActive
	return s.RerenderAllFrames()
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
	return s.RerenderAllFrames()
}

// MoveLayer moves a layer up or down in the animation order.
func (s *Session) MoveLayer(fromIndex, toIndex int) error {
	if fromIndex < 0 || fromIndex >= len(s.Layers) || toIndex < 0 || toIndex >= len(s.Layers) {
		return fmt.Errorf("invalid move indices: %d -> %d", fromIndex, toIndex)
	}
	s.Layers[fromIndex], s.Layers[toIndex] = s.Layers[toIndex], s.Layers[fromIndex]
	return s.RerenderAllFrames()
}

// SetGlobalDuration updates the fallback/global frame duration and updates all non-overridden frames.
func (s *Session) SetGlobalDuration(ms int) {
	if ms <= 0 {
		ms = 100
	}
	s.ExportOptions.DefaultDurationMs = ms
	s.UpdateFrameDurations()
}

// SetFrameOverride enables or disables a custom duration override for a layer/page.
func (s *Session) SetFrameOverride(index int, hasOverride bool, ms int) {
	if s.CurrentMode == svg.ModeLayers {
		if index >= 0 && index < len(s.Layers) {
			s.Layers[index].HasOverride = hasOverride
			if hasOverride && ms > 0 {
				s.Layers[index].OverrideMs = ms
				s.Layers[index].DurationMs = ms
			}
		}
	} else {
		if index >= 0 && index < len(s.Pages) {
			s.Pages[index].HasOverride = hasOverride
			if hasOverride && ms > 0 {
				s.Pages[index].OverrideMs = ms
				s.Pages[index].DurationMs = ms
			}
		}
	}
	s.UpdateFrameDurations()
}

// SetFrameDuration sets the override duration (in milliseconds) for a specific frame.
func (s *Session) SetFrameDuration(index int, ms int) {
	s.SetFrameOverride(index, true, ms)
}

// UpdateFrameDurations synchronizes the effective duration on all RenderedFrames without re-rasterizing.
func (s *Session) UpdateFrameDurations() {
	if s.CurrentMode == svg.ModeLayers {
		frameIdx := 0
		for _, layer := range s.Layers {
			if !layer.IsActive || layer.IsPinned {
				continue
			}
			if frameIdx < len(s.RenderedFrames) {
				eff := layer.EffectiveDuration(s.ExportOptions.DefaultDurationMs)
				s.RenderedFrames[frameIdx].DurationMs = eff
				frameIdx++
			}
		}
	} else {
		frameIdx := 0
		for _, page := range s.Pages {
			if !page.IsActive {
				continue
			}
			if frameIdx < len(s.RenderedFrames) {
				eff := page.EffectiveDuration(s.ExportOptions.DefaultDurationMs)
				s.RenderedFrames[frameIdx].DurationMs = eff
				frameIdx++
			}
		}
	}
}

// RerenderAllFrames rasterizes all active frames for the current mode into preview buffers.
// For small source SVGs (<512px), preview rasterization scales up to 512px so that live playback is sharp.
func (s *Session) RerenderAllFrames() error {
	if s.Document == nil {
		s.RenderedFrames = nil
		return nil
	}

	var frames []svg.RenderedFrame

	if s.CurrentMode == svg.ModeLayers {
		boundW, boundH := s.GetActiveBoundaryDimensions()
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
		// Display preview only needs to be large enough to render crisply on screen (512px max dimension).
		// Full resolution (up to 4096px) is rasterized separately on export via RenderExportFrames().
		const maxPreviewDim = 512.0
		previewScale := maxPreviewDim / maxDim
		renderW := int(math.Round(boundW * previewScale))
		renderH := int(math.Round(boundH * previewScale))
		boundaryRect := s.GetActiveBoundaryRect(0)

		for i, layer := range s.Layers {
			if !layer.IsActive || layer.IsPinned {
				continue
			}

			frameSVG, err := svg.BuildLayerFrameSVG(s.Document, layer.ID, s.PinnedLayers, boundaryRect)
			if err != nil {
				return fmt.Errorf("failed to build frame for layer %s: %w", layer.Label, err)
			}

			img, err := svg.RenderSVGToRGBA(frameSVG, renderW, renderH)
			if err != nil {
				return fmt.Errorf("failed to render layer %s: %w", layer.Label, err)
			}

			dur := layer.EffectiveDuration(s.ExportOptions.DefaultDurationMs)

			frames = append(frames, svg.RenderedFrame{
				Index:      i,
				Label:      layer.Label,
				Image:      img,
				DurationMs: dur,
			})
		}
	} else {
		for i, page := range s.Pages {
			if !page.IsActive {
				continue
			}

			boundaryRect := s.GetActiveBoundaryRect(i)
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
			// Display preview only needs to be large enough to render crisply on screen (512px max dimension).
			// Full resolution (up to 4096px) is rasterized separately on export via RenderExportFrames().
			const maxPreviewDim = 512.0
			previewScale := maxPreviewDim / maxDim
			renderW := int(math.Round(boundW * previewScale))
			renderH := int(math.Round(boundH * previewScale))

			frameSVG, err := svg.BuildPageFrameSVG(s.Document, page, boundaryRect)
			if err != nil {
				return fmt.Errorf("failed to build frame for page %s: %w", page.Label, err)
			}

			img, err := svg.RenderSVGToRGBA(frameSVG, renderW, renderH)
			if err != nil {
				return fmt.Errorf("failed to render page %s: %w", page.Label, err)
			}

			dur := page.EffectiveDuration(s.ExportOptions.DefaultDurationMs)

			frames = append(frames, svg.RenderedFrame{
				Index:      i,
				Label:      page.Label,
				Image:      img,
				DurationMs: dur,
			})
		}
	}

	s.RenderedFrames = frames
	return nil
}

// RenderExportFrames rasterizes the active animation frames directly from vector SVG data
// at the target export resolution (up to 4096px), eliminating bitmap scaling blur.
func (s *Session) RenderExportFrames() ([]gif.FrameInput, error) {
	if s.Document == nil {
		return nil, fmt.Errorf("no SVG document loaded")
	}

	var frameInputs []gif.FrameInput

	if s.CurrentMode == svg.ModeLayers {
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
				if maxSide < 512 {
					targetSquare = 512
				} else if maxSide > 4096 {
					targetSquare = 4096
				} else {
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
				if maxSide < 512 {
					scale := 512.0 / maxSide
					fitW = int(math.Round(boundW * scale))
					fitH = int(math.Round(boundH * scale))
				} else if maxSide > 4096 {
					scale := 4096.0 / maxSide
					fitW = int(math.Round(boundW * scale))
					fitH = int(math.Round(boundH * scale))
				} else {
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

		for i, layer := range s.Layers {
			if !layer.IsActive || layer.IsPinned {
				continue
			}

			frameSVG, err := svg.BuildLayerFrameSVG(s.Document, layer.ID, s.PinnedLayers, boundaryRect)
			if err != nil {
				return nil, fmt.Errorf("failed to build frame for layer %s: %w", layer.Label, err)
			}

			rawImg, err := svg.RenderSVGToRGBA(frameSVG, fitW, fitH)
			if err != nil {
				return nil, fmt.Errorf("failed to rasterize layer %s at %dx%d: %w", layer.Label, fitW, fitH, err)
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

			dur := layer.EffectiveDuration(s.ExportOptions.DefaultDurationMs)
			frameInputs = append(frameInputs, gif.FrameInput{
				Index:      i,
				Label:      layer.Label,
				Image:      finalImg,
				DurationMs: dur,
			})
		}
	} else {
		for i, page := range s.Pages {
			if !page.IsActive {
				continue
			}

			boundaryRect := s.GetActiveBoundaryRect(i)
			boundW := boundaryRect.Width
			boundH := boundaryRect.Height
			if boundW <= 0 {
				boundW = 512
			}
			if boundH <= 0 {
				boundH = 512
			}

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
					if maxSide < 512 {
						targetSquare = 512
					} else if maxSide > 4096 {
						targetSquare = 4096
					} else {
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
					if maxSide < 512 {
						scale := 512.0 / maxSide
						fitW = int(math.Round(boundW * scale))
						fitH = int(math.Round(boundH * scale))
					} else if maxSide > 4096 {
						scale := 4096.0 / maxSide
						fitW = int(math.Round(boundW * scale))
						fitH = int(math.Round(boundH * scale))
					} else {
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

			frameSVG, err := svg.BuildPageFrameSVG(s.Document, page, boundaryRect)
			if err != nil {
				return nil, fmt.Errorf("failed to build frame for page %s: %w", page.Label, err)
			}

			rawImg, err := svg.RenderSVGToRGBA(frameSVG, fitW, fitH)
			if err != nil {
				return nil, fmt.Errorf("failed to rasterize page %s at %dx%d: %w", page.Label, fitW, fitH, err)
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

			dur := page.EffectiveDuration(s.ExportOptions.DefaultDurationMs)
			frameInputs = append(frameInputs, gif.FrameInput{
				Index:      i,
				Label:      page.Label,
				Image:      finalImg,
				DurationMs: dur,
			})
		}
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
		return 0, fmt.Errorf("no frames available to export")
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
		return 0, fmt.Errorf("no frames available to export")
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
