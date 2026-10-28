package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"inkanim/pkg/inksvg"
)

const (
	// ControlFileExt is the file extension for InkAnim sidecar control files.
	ControlFileExt = ".inka"
	// ControlFileVersion is the current schema version for .inka files.
	ControlFileVersion = 1
)

// ControlFile represents the persisted project settings stored in a companion .inka file.
type ControlFile struct {
	Version int                `json:"version"`
	SVGFile string             `json:"svg_file,omitempty"`
	Timing  ControlFileTiming  `json:"timing"`
	Crop    ControlFileCrop    `json:"crop"`
	Export  ControlFileExport  `json:"export"`
	Preview ControlFilePreview `json:"preview"`
}

// ControlFileTiming holds timeline duration, frame rate, and playback mode settings.
type ControlFileTiming struct {
	DurationSec float64 `json:"duration_sec"`
	FPS         int     `json:"fps"`
	PingPong    bool    `json:"ping_pong"`
	Loop        bool    `json:"loop"`
}

// ControlFileCrop holds crop boundary mode and target page selection.
type ControlFileCrop struct {
	BoundaryMode string `json:"boundary_mode"`
	PageIndex    int    `json:"page_index"`
}

// ControlFileExport holds GIF export preset, resolution, and palette settings.
type ControlFileExport struct {
	Preset       string `json:"preset,omitempty"`
	ExportSquare bool   `json:"export_square"`
	SquareSize   int    `json:"square_size"`
	TargetWidth  int    `json:"target_width"`
	TargetHeight int    `json:"target_height"`
	NumColors    int    `json:"num_colors"`
	Dither       bool   `json:"dither"`
}

// ControlFilePreview holds viewport toggle and background preferences.
type ControlFilePreview struct {
	Background string `json:"background,omitempty"`
	ShowGuides bool   `json:"show_guides"`
	ShowPaths  bool   `json:"show_paths"`
	ShowScales bool   `json:"show_scales"`
}

// SidecarPathForSVG returns the companion .inka path for a given .svg or .inka path.
func SidecarPathForSVG(path string) string {
	if path == "" {
		return ""
	}
	ext := filepath.Ext(path)
	if strings.EqualFold(ext, ".svg") || strings.EqualFold(ext, ControlFileExt) {
		return path[:len(path)-len(ext)] + ControlFileExt
	}
	return path + ControlFileExt
}

// ParseControlFile parses a JSON .inka control file from raw bytes.
func ParseControlFile(data []byte) (*ControlFile, error) {
	var ctrl ControlFile
	if err := json.Unmarshal(data, &ctrl); err != nil {
		return nil, fmt.Errorf("invalid .inka control file: %w", err)
	}
	if ctrl.Version <= 0 {
		ctrl.Version = ControlFileVersion
	}
	return &ctrl, nil
}

// LoadControlFile reads and parses a .inka control file from disk.
func LoadControlFile(path string) (*ControlFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseControlFile(data)
}

// SaveControlFile serializes and writes a .inka control file to disk.
func SaveControlFile(path string, ctrl *ControlFile) error {
	if path == "" || ctrl == nil {
		return errors.New("control file path and data are required")
	}
	if ctrl.Version <= 0 {
		ctrl.Version = ControlFileVersion
	}
	data, err := json.MarshalIndent(ctrl, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling .inka control file: %w", err)
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0644)
}

// ResolveSVGAndControlFile accepts either an .svg path or a .inka path:
//   - If inputPath is a .inka file, it parses the .inka file and resolves the companion .svg path.
//   - If inputPath is an .svg file, it checks for a sibling .inka file and parses it if present.
func ResolveSVGAndControlFile(inputPath string) (string, *ControlFile, error) {
	if inputPath == "" {
		return "", nil, errors.New("empty file path")
	}
	ext := filepath.Ext(inputPath)
	if strings.EqualFold(ext, ControlFileExt) {
		ctrl, err := LoadControlFile(inputPath)
		if err != nil {
			return "", nil, err
		}
		dir := filepath.Dir(inputPath)
		var candidates []string
		if ctrl.SVGFile != "" {
			if filepath.IsAbs(ctrl.SVGFile) {
				candidates = append(candidates, ctrl.SVGFile)
			} else {
				candidates = append(candidates, filepath.Join(dir, ctrl.SVGFile))
			}
		}
		fallbackSVG := inputPath[:len(inputPath)-len(ext)] + ".svg"
		candidates = append(candidates, fallbackSVG)

		for _, cand := range candidates {
			if info, statErr := os.Stat(cand); statErr == nil && !info.IsDir() {
				return cand, ctrl, nil
			}
		}
		return "", ctrl, fmt.Errorf("companion SVG file not found for %s", filepath.Base(inputPath))
	}

	sidecar := SidecarPathForSVG(inputPath)
	if sidecar != "" {
		if info, statErr := os.Stat(sidecar); statErr == nil && !info.IsDir() {
			if ctrl, loadErr := LoadControlFile(sidecar); loadErr == nil {
				return inputPath, ctrl, nil
			}
		}
	}
	return inputPath, nil, nil
}

// BuildControlFile constructs a ControlFile snapshot from the current session state.
func (s *Session) BuildControlFile(svgPath string) *ControlFile {
	fps := s.FPS
	if fps <= 0 {
		fps = 10
	}
	durSec := float64(s.TotalDurationMs) / 1000.0
	if durSec <= 0 {
		durSec = 1.0
	}
	durSec = math.Round(durSec*100) / 100

	boundary := string(s.CropBoundaryMode)
	if boundary == "" {
		boundary = string(inksvg.BoundaryPage)
	}

	svgBase := ""
	if svgPath != "" {
		svgBase = filepath.Base(svgPath)
	}

	return &ControlFile{
		Version: ControlFileVersion,
		SVGFile: svgBase,
		Timing: ControlFileTiming{
			DurationSec: durSec,
			FPS:         fps,
			PingPong:    s.ExportOptions.PingPong,
			Loop:        true,
		},
		Crop: ControlFileCrop{
			BoundaryMode: boundary,
			PageIndex:    s.CropPageIndex,
		},
		Export: ControlFileExport{
			ExportSquare: s.ExportOptions.ExportSquare,
			SquareSize:   s.ExportOptions.SquareSize,
			TargetWidth:  s.ExportOptions.TargetWidth,
			TargetHeight: s.ExportOptions.TargetHeight,
			NumColors:    s.ExportOptions.NumColors,
			Dither:       s.ExportOptions.Dither,
		},
		Preview: ControlFilePreview{
			ShowGuides: true,
			ShowPaths:  s.ShowMotionLines,
			ShowScales: true,
		},
	}
}

// ApplyControlFile applies the settings from a parsed ControlFile to the session.
// When called before LoadSVGData, LoadSVGData will use these settings during initial rendering.
func (s *Session) ApplyControlFile(ctrl *ControlFile) {
	if s == nil || ctrl == nil {
		return
	}

	if ctrl.Timing.FPS >= 1 && ctrl.Timing.FPS <= 60 {
		s.FPS = ctrl.Timing.FPS
		s.ExportOptions.DefaultDurationMs = max(int(math.Round(1000.0/float64(s.FPS))), 10)
	}
	if ctrl.Timing.DurationSec > 0 {
		s.TotalDurationMs = max(int(math.Round(ctrl.Timing.DurationSec*1000.0)), 100)
		s.HasManualDuration = true
	}
	s.ExportOptions.PingPong = ctrl.Timing.PingPong

	switch inksvg.BoundaryMode(strings.ToLower(ctrl.Crop.BoundaryMode)) {
	case inksvg.BoundaryDrawing:
		s.CropBoundaryMode = inksvg.BoundaryDrawing
	case inksvg.BoundaryPage:
		s.CropBoundaryMode = inksvg.BoundaryPage
	}
	if ctrl.Crop.PageIndex >= 0 {
		s.CropPageIndex = ctrl.Crop.PageIndex
	}

	s.ExportOptions.ExportSquare = ctrl.Export.ExportSquare
	if ctrl.Export.SquareSize >= 16 && ctrl.Export.SquareSize <= 4096 {
		s.ExportOptions.SquareSize = ctrl.Export.SquareSize
	}
	if ctrl.Export.TargetWidth >= 16 && ctrl.Export.TargetWidth <= 4096 {
		s.ExportOptions.TargetWidth = ctrl.Export.TargetWidth
	}
	if ctrl.Export.TargetHeight >= 16 && ctrl.Export.TargetHeight <= 4096 {
		s.ExportOptions.TargetHeight = ctrl.Export.TargetHeight
	}
	if ctrl.Export.NumColors >= 2 && ctrl.Export.NumColors <= 256 {
		s.ExportOptions.NumColors = ctrl.Export.NumColors
	}
	s.ExportOptions.Dither = ctrl.Export.Dither

	s.ShowMotionLines = ctrl.Preview.ShowPaths
	if s.Document != nil {
		s.Document.ShowMotionLines = s.ShowMotionLines
	}
}
