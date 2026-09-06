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
	FilePath       string
	Document       *svg.SVGDocument
	CurrentMode    svg.FrameMode
	Layers         []svg.Layer
	Pages          []svg.Page
	ExportOptions  gif.ExportOptions
	RenderedFrames []svg.RenderedFrame
	PinnedLayers   map[string]bool
}

// NewSession creates an empty session with default options.
func NewSession() *Session {
	return &Session{
		CurrentMode:   svg.ModeLayers,
		ExportOptions: gif.DefaultOptions(),
		PinnedLayers:  make(map[string]bool),
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
	s.PinnedLayers = make(map[string]bool)

	return s.RerenderAllFrames()
}

// SetMode switches between ModeLayers and ModePages and rerenders.
func (s *Session) SetMode(mode svg.FrameMode) error {
	s.CurrentMode = mode
	return s.RerenderAllFrames()
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
		docW := s.Document.Width
		docH := s.Document.Height
		if docW <= 0 {
			docW = 512
		}
		if docH <= 0 {
			docH = 512
		}
		maxDim := docW
		if docH > maxDim {
			maxDim = docH
		}
		// Display preview only needs to be large enough to render crisply on screen (512px max dimension).
		// Full resolution (up to 4096px) is rasterized separately on export via RenderExportFrames().
		const maxPreviewDim = 512.0
		previewScale := maxPreviewDim / maxDim
		renderW := int(math.Round(docW * previewScale))
		renderH := int(math.Round(docH * previewScale))

		for i, layer := range s.Layers {
			if !layer.IsActive || layer.IsPinned {
				continue
			}

			frameSVG, err := svg.BuildLayerFrameSVG(s.Document, layer.ID, s.PinnedLayers)
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

			pageW := page.Width
			pageH := page.Height
			if pageW <= 0 {
				pageW = 512
			}
			if pageH <= 0 {
				pageH = 512
			}
			maxDim := pageW
			if pageH > maxDim {
				maxDim = pageH
			}
			// Display preview only needs to be large enough to render crisply on screen (512px max dimension).
			// Full resolution (up to 4096px) is rasterized separately on export via RenderExportFrames().
			const maxPreviewDim = 512.0
			previewScale := maxPreviewDim / maxDim
			renderW := int(math.Round(pageW * previewScale))
			renderH := int(math.Round(pageH * previewScale))

			frameSVG, err := svg.BuildPageFrameSVG(s.Document, page)
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
		docW := s.Document.Width
		docH := s.Document.Height
		if docW <= 0 {
			docW = 512
		}
		if docH <= 0 {
			docH = 512
		}

		// Calculate export dimensions
		var exportSquare bool
		var targetSquare int
		var fitW, fitH int

		if s.ExportOptions.ExportSquare {
			exportSquare = true
			targetSquare = s.ExportOptions.SquareSize
			if targetSquare <= 0 {
				maxSide := docW
				if docH > maxSide {
					maxSide = docH
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

			maxSide := docW
			if docH > maxSide {
				maxSide = docH
			}
			scale := float64(targetSquare) / maxSide
			fitW = int(math.Round(docW * scale))
			fitH = int(math.Round(docH * scale))
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
				maxSide := docW
				if docH > maxSide {
					maxSide = docH
				}
				if maxSide < 512 {
					scale := 512.0 / maxSide
					fitW = int(math.Round(docW * scale))
					fitH = int(math.Round(docH * scale))
				} else if maxSide > 4096 {
					scale := 4096.0 / maxSide
					fitW = int(math.Round(docW * scale))
					fitH = int(math.Round(docH * scale))
				} else {
					fitW = int(math.Round(docW))
					fitH = int(math.Round(docH))
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

			frameSVG, err := svg.BuildLayerFrameSVG(s.Document, layer.ID, s.PinnedLayers)
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

			pageW := page.Width
			pageH := page.Height
			if pageW <= 0 {
				pageW = 512
			}
			if pageH <= 0 {
				pageH = 512
			}

			var exportSquare bool
			var targetSquare int
			var fitW, fitH int

			if s.ExportOptions.ExportSquare {
				exportSquare = true
				targetSquare = s.ExportOptions.SquareSize
				if targetSquare <= 0 {
					maxSide := pageW
					if pageH > maxSide {
						maxSide = pageH
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

				maxSide := pageW
				if pageH > maxSide {
					maxSide = pageH
				}
				scale := float64(targetSquare) / maxSide
				fitW = int(math.Round(pageW * scale))
				fitH = int(math.Round(pageH * scale))
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
					maxSide := pageW
					if pageH > maxSide {
						maxSide = pageH
					}
					if maxSide < 512 {
						scale := 512.0 / maxSide
						fitW = int(math.Round(pageW * scale))
						fitH = int(math.Round(pageH * scale))
					} else if maxSide > 4096 {
						scale := 4096.0 / maxSide
						fitW = int(math.Round(pageW * scale))
						fitH = int(math.Round(pageH * scale))
					} else {
						fitW = int(math.Round(pageW))
						fitH = int(math.Round(pageH))
					}
				}
			}

			if fitW < 1 {
				fitW = 1
			}
			if fitH < 1 {
				fitH = 1
			}

			frameSVG, err := svg.BuildPageFrameSVG(s.Document, page)
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
