package app

import (
	"fmt"
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

	doc, err := svg.ParseSVG(data)
	if err != nil {
		return fmt.Errorf("failed to parse SVG: %w", err)
	}

	s.FilePath = filePath
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

// SetFrameDuration sets the duration (in milliseconds) for a specific frame.
func (s *Session) SetFrameDuration(index int, ms int) {
	if s.CurrentMode == svg.ModeLayers {
		if index >= 0 && index < len(s.Layers) {
			s.Layers[index].DurationMs = ms
		}
	} else {
		if index >= 0 && index < len(s.Pages) {
			s.Pages[index].DurationMs = ms
		}
	}
}

// RerenderAllFrames rasterizes all active frames for the current mode.
func (s *Session) RerenderAllFrames() error {
	if s.Document == nil {
		s.RenderedFrames = nil
		return nil
	}

	var frames []svg.RenderedFrame

	if s.CurrentMode == svg.ModeLayers {
		for i, layer := range s.Layers {
			if !layer.IsActive || layer.IsPinned {
				continue
			}

			frameSVG, err := svg.BuildLayerFrameSVG(s.Document, layer.ID, s.PinnedLayers)
			if err != nil {
				return fmt.Errorf("failed to build frame for layer %s: %w", layer.Label, err)
			}

			img, err := svg.RenderSVGToRGBA(frameSVG, int(s.Document.Width), int(s.Document.Height))
			if err != nil {
				return fmt.Errorf("failed to render layer %s: %w", layer.Label, err)
			}

			dur := layer.DurationMs
			if dur <= 0 {
				dur = s.ExportOptions.DefaultDurationMs
			}

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

			frameSVG, err := svg.BuildPageFrameSVG(s.Document, page)
			if err != nil {
				return fmt.Errorf("failed to build frame for page %s: %w", page.Label, err)
			}

			img, err := svg.RenderSVGToRGBA(frameSVG, int(page.Width), int(page.Height))
			if err != nil {
				return fmt.Errorf("failed to render page %s: %w", page.Label, err)
			}

			dur := page.DurationMs
			if dur <= 0 {
				dur = s.ExportOptions.DefaultDurationMs
			}

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

// ExportGIF exports the active frames to a single animated GIF at destinationPath.
func (s *Session) ExportGIF(destinationPath string) (int64, error) {
	if len(s.RenderedFrames) == 0 {
		return 0, fmt.Errorf("no frames available to export")
	}

	frameInputs := make([]gif.FrameInput, len(s.RenderedFrames))
	for i, rf := range s.RenderedFrames {
		frameInputs[i] = gif.FrameInput{
			Index:      rf.Index,
			Label:      rf.Label,
			Image:      rf.Image,
			DurationMs: rf.DurationMs,
		}
	}

	return gif.WriteGIFToFile(destinationPath, frameInputs, s.ExportOptions)
}
