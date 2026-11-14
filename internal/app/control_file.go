package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"inkanim/pkg/inksvg"
)

const (
	// ControlFileExt is the file extension for InkAnim sidecar control files.
	ControlFileExt = ".inka"
	// ControlFileVersion is the current schema version for .inka files.
	ControlFileVersion = 1
)

// LinkedSVG represents a co-located SVG file linked into the .inka project.
type LinkedSVG struct {
	ID   string `json:"id"`
	File string `json:"file"`
}

// ObjectConfig holds stage placement, display timing, pivot, and timeline actions for a group/layer.
type ObjectConfig struct {
	SourceID         string         `json:"source_id,omitempty"`
	LabelPath        string         `json:"label_path,omitempty"`
	FallbackXMLID    string         `json:"fallback_xml_id,omitempty"`
	HasDisplayWindow bool           `json:"has_display_window,omitempty"`
	StartTimeSec     float64        `json:"start_time_sec"`
	EndTimeSec       float64        `json:"end_time_sec,omitempty"`
	OverrideStartPos bool           `json:"override_start_pos,omitempty"`
	StartX           float64        `json:"start_x,omitempty"`
	StartY           float64        `json:"start_y,omitempty"`
	BaseScaleX       float64        `json:"base_scale_x,omitempty"`
	BaseScaleY       float64        `json:"base_scale_y,omitempty"`
	BaseRotation     float64        `json:"base_rotation,omitempty"`
	ZOffset          int            `json:"z_offset,omitempty"`
	PivotType        string         `json:"pivot_type,omitempty"`
	PivotEdgeAngle   float64        `json:"pivot_edge_angle,omitempty"`
	PivotNodeID      string         `json:"pivot_node_id,omitempty"`
	PivotX           float64        `json:"pivot_x,omitempty"`
	PivotY           float64        `json:"pivot_y,omitempty"`
	Actions          []ActionConfig `json:"actions,omitempty"`
}

// ActionConfig holds a single timeline action (Move, Rot, Scale, Fade, Color, Depth, Camera, etc.) in seconds.
type ActionConfig struct {
	ID               string      `json:"id"`
	Type             string      `json:"type"`
	StartSec         float64     `json:"start_sec"`
	EndSec           float64     `json:"end_sec"`
	IsAll            bool        `json:"is_all,omitempty"`
	Ease             string      `json:"ease,omitempty"`
	Repeat           int         `json:"repeat,omitempty"`
	PingPong         bool        `json:"ping_pong,omitempty"`
	Reverse          bool        `json:"reverse,omitempty"`
	OrientPath       bool        `json:"orient_path,omitempty"`
	PathD            string      `json:"path_d,omitempty"`
	PivotType        string      `json:"pivot_type,omitempty"`
	PivotEdgeAngle   float64     `json:"pivot_edge_angle,omitempty"`
	PivotNodeID      string      `json:"pivot_node_id,omitempty"`
	PivotX           float64     `json:"pivot_x,omitempty"`
	PivotY           float64     `json:"pivot_y,omitempty"`
	RotationAngle    float64     `json:"rotation_angle,omitempty"`
	RotationFrom     float64     `json:"rotation_from,omitempty"`
	RotationTo       float64     `json:"rotation_to,omitempty"`
	HasRotationRange bool        `json:"has_rotation_range,omitempty"`
	RotationDir      string      `json:"rotation_dir,omitempty"`
	ScaleFromX       float64     `json:"scale_from_x,omitempty"`
	ScaleFromY       float64     `json:"scale_from_y,omitempty"`
	ScaleToX         float64     `json:"scale_to_x,omitempty"`
	ScaleToY         float64     `json:"scale_to_y,omitempty"`
	OpacityFrom      float64     `json:"opacity_from,omitempty"`
	OpacityTo        float64     `json:"opacity_to,omitempty"`
	HasOpacity       bool        `json:"has_opacity,omitempty"`
	VisibilityState  string      `json:"visibility_state,omitempty"`
	DepthOffset      int         `json:"depth_offset,omitempty"`
	ParallaxFactor   float64     `json:"parallax_factor,omitempty"`
	HasParallax      bool        `json:"has_parallax,omitempty"`
	ColorTarget      string      `json:"color_target,omitempty"`
	ColorAngle       float64     `json:"color_angle,omitempty"`
	HasColorAngle    bool        `json:"has_color_angle,omitempty"`
	FillURL          string      `json:"fill_url,omitempty"`
	Bounds           inksvg.Rect `json:"bounds"`
}

// ControlFile represents the persisted project settings and timeline actions stored in a companion .inka file.
type ControlFile struct {
	Version int                     `json:"version"`
	SVGFile string                  `json:"svg_file,omitempty"`
	Sources []LinkedSVG             `json:"sources,omitempty"`
	Timing  ControlFileTiming       `json:"timing"`
	Crop    ControlFileCrop         `json:"crop"`
	Export  ControlFileExport       `json:"export"`
	Preview ControlFilePreview      `json:"preview"`
	Objects map[string]ObjectConfig `json:"objects,omitempty"`
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
	if len(ctrl.Sources) == 0 && ctrl.SVGFile != "" {
		ctrl.Sources = []LinkedSVG{
			{
				ID:   "src_1",
				File: filepath.Base(ctrl.SVGFile),
			},
		}
	}
	if ctrl.SVGFile == "" && len(ctrl.Sources) > 0 {
		ctrl.SVGFile = ctrl.Sources[0].File
	}
	if ctrl.Objects == nil {
		ctrl.Objects = make(map[string]ObjectConfig)
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
	if len(ctrl.Sources) == 0 && ctrl.SVGFile != "" {
		ctrl.Sources = []LinkedSVG{
			{
				ID:   "src_1",
				File: filepath.Base(ctrl.SVGFile),
			},
		}
	}
	if ctrl.SVGFile == "" && len(ctrl.Sources) > 0 {
		ctrl.SVGFile = ctrl.Sources[0].File
	}
	data, err := json.MarshalIndent(ctrl, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling .inka control file: %w", err)
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0644)
}

// EnsureCoLocatedSVG ensures that externalSVGPath exists inside projectDir by copying it
// if it resides in a different directory. It returns the local filename (base name).
func EnsureCoLocatedSVG(projectDir, externalSVGPath string) (string, error) {
	if externalSVGPath == "" {
		return "", errors.New("empty SVG path")
	}
	baseName := filepath.Base(externalSVGPath)
	if projectDir == "" {
		return baseName, nil
	}
	absSrc, err := filepath.Abs(externalSVGPath)
	if err != nil {
		return "", err
	}
	targetPath := filepath.Join(projectDir, baseName)
	absDst, err := filepath.Abs(targetPath)
	if err != nil {
		return "", err
	}
	if strings.EqualFold(filepath.Clean(absSrc), filepath.Clean(absDst)) {
		return baseName, nil
	}
	srcBytes, err := os.ReadFile(absSrc)
	if err != nil {
		return "", fmt.Errorf("reading source SVG %s: %w", externalSVGPath, err)
	}
	if dstBytes, readErr := os.ReadFile(absDst); readErr == nil && bytes.Equal(srcBytes, dstBytes) {
		return baseName, nil
	}
	if err := os.WriteFile(absDst, srcBytes, 0644); err != nil {
		return "", fmt.Errorf("copying SVG to project directory %s: %w", absDst, err)
	}
	return baseName, nil
}

// AddLinkedSVG co-locates svgPath in projectDir (if projectDir is non-empty) and registers it
// in ctrl.Sources, returning its LinkedSVG entry.
func (ctrl *ControlFile) AddLinkedSVG(projectDir, svgPath string) (LinkedSVG, error) {
	if ctrl == nil {
		return LinkedSVG{}, errors.New("nil ControlFile")
	}
	localFile, err := EnsureCoLocatedSVG(projectDir, svgPath)
	if err != nil {
		return LinkedSVG{}, err
	}
	for _, existing := range ctrl.Sources {
		if strings.EqualFold(existing.File, localFile) {
			return existing, nil
		}
	}
	nextIdx := len(ctrl.Sources) + 1
	id := fmt.Sprintf("src_%d", nextIdx)
	usedIDs := make(map[string]bool, len(ctrl.Sources))
	for _, s := range ctrl.Sources {
		usedIDs[s.ID] = true
	}
	for usedIDs[id] {
		nextIdx++
		id = fmt.Sprintf("src_%d", nextIdx)
	}
	entry := LinkedSVG{
		ID:   id,
		File: localFile,
	}
	ctrl.Sources = append(ctrl.Sources, entry)
	if ctrl.SVGFile == "" {
		ctrl.SVGFile = localFile
	}
	return entry, nil
}

// ResolveSVGAndControlFile accepts either an .svg path or a .inka path:
//   - If inputPath is a .inka file, it parses the .inka file and resolves the primary companion .svg path.
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
		for _, src := range ctrl.Sources {
			if src.File != "" {
				if filepath.IsAbs(src.File) {
					candidates = append(candidates, src.File)
				} else {
					candidates = append(candidates, filepath.Join(dir, src.File))
				}
			}
		}
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
				if len(ctrl.Sources) == 0 {
					ctrl.Sources = []LinkedSVG{{ID: "src_1", File: filepath.Base(cand)}}
				}
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

// LoadLinkedSources reads all linked SVG files referenced in ctrl.Sources from projectDir.
func LoadLinkedSources(projectDir string, ctrl *ControlFile, fallbackSVGPath string) ([]inksvg.LinkedSource, error) {
	if ctrl == nil || len(ctrl.Sources) == 0 {
		if fallbackSVGPath == "" {
			return nil, errors.New("no linked SVG sources specified")
		}
		data, err := os.ReadFile(fallbackSVGPath)
		if err != nil {
			return nil, err
		}
		return []inksvg.LinkedSource{
			{
				ID:     "src_1",
				Name:   filepath.Base(fallbackSVGPath),
				RawSVG: data,
			},
		}, nil
	}

	var sources []inksvg.LinkedSource
	for i, s := range ctrl.Sources {
		if s.File == "" {
			continue
		}
		srcID := s.ID
		if srcID == "" {
			srcID = fmt.Sprintf("src_%d", i+1)
		}
		svgPath := s.File
		if !filepath.IsAbs(svgPath) && projectDir != "" {
			svgPath = filepath.Join(projectDir, s.File)
		}
		data, err := os.ReadFile(svgPath)
		if err != nil {
			return nil, fmt.Errorf("reading linked SVG %q: %w", s.File, err)
		}
		sources = append(sources, inksvg.LinkedSource{
			ID:     srcID,
			Name:   filepath.Base(s.File),
			RawSVG: data,
		})
	}
	if len(sources) == 0 {
		return nil, errors.New("no valid linked SVG sources found")
	}
	return sources, nil
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

	sources := slices.Clone(s.LinkedSources)
	if len(sources) == 0 && svgBase != "" {
		sources = []LinkedSVG{
			{
				ID:   "src_1",
				File: svgBase,
			},
		}
	}
	if svgBase == "" && len(sources) > 0 {
		svgBase = sources[0].File
	}

	var objects map[string]ObjectConfig
	if len(s.Objects) > 0 {
		objects = make(map[string]ObjectConfig, len(s.Objects))
		for k, v := range s.Objects {
			cloned := v
			cloned.Actions = slices.Clone(v.Actions)
			objects[k] = cloned
		}
	}

	return &ControlFile{
		Version: ControlFileVersion,
		SVGFile: svgBase,
		Sources: sources,
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
		Objects: objects,
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

	if len(ctrl.Sources) > 0 {
		s.LinkedSources = slices.Clone(ctrl.Sources)
	}
	if len(ctrl.Objects) > 0 {
		s.Objects = make(map[string]ObjectConfig, len(ctrl.Objects))
		for k, v := range ctrl.Objects {
			cloned := v
			cloned.Actions = slices.Clone(v.Actions)
			s.Objects[k] = cloned
		}
	}

	s.ShowMotionLines = ctrl.Preview.ShowPaths
	if s.Document != nil {
		s.Document.ShowMotionLines = s.ShowMotionLines
		ApplyObjectsToDocument(s.Document, s.Objects, float64(s.TotalDurationMs)/1000.0)
	}
}

// ApplyObjectsToDocument binds .inka ObjectConfig stage properties and time-based actions
// onto a parsed SVGDocument by matching group label paths and fallback XML IDs.
func ApplyObjectsToDocument(doc *inksvg.SVGDocument, objects map[string]ObjectConfig, totalDurationSec float64) {
	if doc == nil {
		return
	}
	if totalDurationSec <= 0 {
		totalDurationSec = 1.0
	}
	doc.TotalDurationSec = totalDurationSec
	if doc.StageObjects == nil {
		doc.StageObjects = make(map[string]inksvg.StageObject)
	}
	if len(objects) == 0 {
		return
	}

	_, lookup := inksvg.ExtractGroupHierarchy(doc.RawContent)

	// When .inka objects are active, studio ObjectConfigs are authoritative for actions:
	// rebuild doc.MotionPaths and doc.CameraPath exclusively from objects so deleting or
	// reordering an action in the timeline takes effect even when legacy IAMS wasn't stripped from the SVG XML.
	doc.MotionPaths = nil
	doc.CameraPath = nil

	var hasAnimatedStageObj bool
	for _, key := range slices.Sorted(maps.Keys(objects)) {
		obj := objects[key]
		resolvedID := resolveObjectXMLID(key, obj, lookup)
		if resolvedID == "" {
			continue
		}

		hasWin := obj.HasDisplayWindow || obj.StartTimeSec > 0 || obj.EndTimeSec > 0
		if hasWin {
			hasAnimatedStageObj = true
		}

		doc.StageObjects[resolvedID] = inksvg.StageObject{
			GroupID:          resolvedID,
			HasDisplayWindow: hasWin,
			StartTimeSec:     obj.StartTimeSec,
			EndTimeSec:       obj.EndTimeSec,
			OverrideStartPos: obj.OverrideStartPos,
			StartX:           obj.StartX,
			StartY:           obj.StartY,
			BaseScaleX:       obj.BaseScaleX,
			BaseScaleY:       obj.BaseScaleY,
			BaseRotation:     obj.BaseRotation,
			ZOffset:          obj.ZOffset,
			PivotType:        obj.PivotType,
			PivotEdgeAngle:   obj.PivotEdgeAngle,
			PivotNodeID:      obj.PivotNodeID,
			PivotX:           obj.PivotX,
			PivotY:           obj.PivotY,
		}

		for i, act := range obj.Actions {
			actID := act.ID
			if actID == "" {
				actID = fmt.Sprintf("%s_act_%d", resolvedID, i+1)
			}
			mp := actionConfigToMotionPath(act, actID, resolvedID)
			if mp.Config.IsCamera {
				doc.CameraPath = &mp
			} else {
				doc.MotionPaths = append(doc.MotionPaths, mp)
			}
		}
	}

	if len(doc.Frames) <= 1 && (len(doc.MotionPaths) > 0 || hasAnimatedStageObj || doc.CameraPath != nil) {
		const defaultFrames = 15
		doc.Frames = make([]inksvg.Frame, defaultFrames)
		for i := range defaultFrames {
			doc.Frames[i] = inksvg.Frame{
				ID:         fmt.Sprintf("timeline_frame_%d", i+1),
				Label:      fmt.Sprintf("Frame %d", i+1),
				Index:      i,
				Visible:    true,
				IsActive:   true,
				DurationMs: 100,
			}
		}
	}
}

func resolveObjectXMLID(key string, obj ObjectConfig, lookup map[string]string) string {
	var candidates []string
	if obj.SourceID != "" && obj.FallbackXMLID != "" {
		candidates = append(candidates, obj.SourceID+"__"+obj.FallbackXMLID)
	}
	if obj.FallbackXMLID != "" {
		candidates = append(candidates, obj.FallbackXMLID)
	}
	candidates = append(candidates, key)
	if obj.SourceID != "" && obj.LabelPath != "" {
		candidates = append(candidates, obj.SourceID+"/"+obj.LabelPath)
	}
	if obj.LabelPath != "" {
		candidates = append(candidates, obj.LabelPath)
	}
	if obj.SourceID != "" && obj.LabelPath == "" {
		candidates = append(candidates, obj.SourceID)
	}

	for _, cand := range candidates {
		if cand == "" {
			continue
		}
		if xmlID, ok := lookup[cand]; ok && xmlID != "" {
			return xmlID
		}
	}
	return obj.FallbackXMLID
}

func actionConfigToMotionPath(act ActionConfig, actID, groupID string) inksvg.MotionPath {
	cfg := inksvg.DefaultMotionConfig(strings.ToLower(act.Type))
	cfg.HasTimeRange = true
	cfg.StartSec = act.StartSec
	cfg.EndSec = act.EndSec
	cfg.IsAll = act.IsAll
	if act.Ease != "" {
		cfg.Ease = act.Ease
	}
	if act.Repeat > 0 {
		cfg.Repeat = act.Repeat
		cfg.ColorRepeat = act.Repeat
	}
	cfg.IsPingPong = act.PingPong
	cfg.Reverse = act.Reverse
	cfg.OrientPath = act.OrientPath
	if act.PivotType != "" {
		cfg.PivotType = act.PivotType
	}
	cfg.PivotEdgeAngle = act.PivotEdgeAngle
	cfg.PivotNodeID = act.PivotNodeID
	cfg.PivotX = act.PivotX
	cfg.PivotY = act.PivotY
	cfg.RotationAngle = act.RotationAngle
	cfg.RotationFrom = act.RotationFrom
	cfg.RotationTo = act.RotationTo
	cfg.HasRotationRange = act.HasRotationRange
	if act.RotationDir != "" {
		cfg.RotationDir = act.RotationDir
	}
	if act.ScaleFromX != 0 {
		cfg.ScaleFromX = act.ScaleFromX
	}
	if act.ScaleFromY != 0 {
		cfg.ScaleFromY = act.ScaleFromY
	}
	if act.ScaleToX != 0 {
		cfg.ScaleToX = act.ScaleToX
	}
	if act.ScaleToY != 0 {
		cfg.ScaleToY = act.ScaleToY
	}
	if act.HasOpacity || act.Type == "fade" {
		cfg.HasOpacity = true
		cfg.OpacityFrom = act.OpacityFrom
		cfg.OpacityTo = act.OpacityTo
	}
	if act.VisibilityState != "" {
		cfg.VisibilityState = act.VisibilityState
		cfg.HasVisibility = true
	}
	if act.Type == "depth" || act.DepthOffset != 0 {
		cfg.DepthOffset = act.DepthOffset
		cfg.HasDepth = true
	}
	if act.HasParallax || act.Type == "dist" || act.Type == "distance" {
		cfg.HasParallax = true
		cfg.ParallaxFactor = act.ParallaxFactor
	}
	if act.Type == "color" {
		cfg.IsColor = true
		if act.ColorTarget != "" {
			cfg.ColorTarget = act.ColorTarget
		}
		cfg.ColorAngle = act.ColorAngle
		cfg.HasColorAngle = act.HasColorAngle
	}

	return inksvg.MotionPath{
		ID:       actID,
		GroupID:  groupID,
		PathData: act.PathD,
		Config:   cfg,
		FillURL:  act.FillURL,
		Bounds:   act.Bounds,
	}
}

// MigrateDocumentIAMSToControlFile extracts legacy IAMS directives and helper paths from doc
// and converts them into .inka ObjectConfig and seconds-based ActionConfig entries on ctrl.
func MigrateDocumentIAMSToControlFile(doc *inksvg.SVGDocument, sourceID string, totalDurationSec float64, ctrl *ControlFile) int {
	if doc == nil || ctrl == nil {
		return 0
	}
	if totalDurationSec <= 0 {
		totalDurationSec = ctrl.Timing.DurationSec
	}
	if totalDurationSec <= 0 {
		totalDurationSec = 1.0
	}
	if ctrl.Objects == nil {
		ctrl.Objects = make(map[string]ObjectConfig)
	}

	groups, _ := inksvg.ExtractGroupHierarchy(doc.RawContent)
	metaByXMLID := make(map[string]inksvg.GroupMetadata, len(groups))
	for _, g := range groups {
		if g.XMLID != "" {
			metaByXMLID[g.XMLID] = g
		}
	}

	maxF := doc.FrameCount()
	if maxF <= 0 {
		maxF = 15
	}

	frameToSec := func(f int) float64 {
		if maxF <= 1 || f <= 1 {
			return 0.0
		}
		if f >= maxF {
			return totalDurationSec
		}
		sec := (float64(f-1) / float64(maxF-1)) * totalDurationSec
		return math.Round(sec*1000) / 1000
	}

	migratedCount := 0
	for i, mp := range doc.MotionPaths {
		groupID := mp.GroupID
		if groupID == "" {
			continue
		}
		meta, hasMeta := metaByXMLID[groupID]
		labelPath := meta.LabelPath
		if !hasMeta || labelPath == "" {
			labelPath = groupID
		}
		srcID := sourceID
		if hasMeta && meta.SourceID != "" {
			srcID = meta.SourceID
		}

		objKey := labelPath
		if srcID != "" && labelPath != srcID {
			objKey = srcID + "/" + labelPath
		}

		obj, exists := ctrl.Objects[objKey]
		if !exists {
			for k, existingObj := range ctrl.Objects {
				if k == groupID || existingObj.FallbackXMLID == groupID {
					objKey = k
					obj = existingObj
					exists = true
					break
				}
			}
		}
		if !exists {
			obj = ObjectConfig{
				SourceID:      srcID,
				LabelPath:     labelPath,
				FallbackXMLID: groupID,
			}
		}

		actID := mp.ID
		if actID == "" {
			actID = fmt.Sprintf("%s_act_%d", groupID, i+1)
		}

		// Ensure globally unique action ID across all objects in ctrl before checking idempotency.
		usedOnOther := func(id string) bool {
			for otherKey, otherObj := range ctrl.Objects {
				if otherKey == objKey {
					continue
				}
				if slices.ContainsFunc(otherObj.Actions, func(a ActionConfig) bool { return a.ID == id }) {
					return true
				}
			}
			return false
		}
		if usedOnOther(actID) {
			actID = fmt.Sprintf("%s_%s", groupID, actID)
		}

		// Idempotency guard: if this action ID was already migrated onto obj, skip it.
		if slices.ContainsFunc(obj.Actions, func(existing ActionConfig) bool {
			return existing.ID == actID
		}) {
			continue
		}

		startSec := 0.0
		endSec := totalDurationSec
		if mp.Config.HasTimeRange && mp.Config.EndSec > 0 {
			startSec = mp.Config.StartSec
			endSec = mp.Config.EndSec
		} else if !mp.Config.IsAll && mp.Config.EndFrame > 0 {
			startSec = frameToSec(mp.Config.StartFrame)
			endSec = frameToSec(mp.Config.EndFrame)
		}

		act := ActionConfig{
			ID:               actID,
			Type:             mp.Config.Type,
			StartSec:         startSec,
			EndSec:           endSec,
			IsAll:            mp.Config.IsAll,
			Ease:             mp.Config.Ease,
			Repeat:           mp.Config.Repeat,
			PingPong:         mp.Config.IsPingPong,
			Reverse:          mp.Config.Reverse,
			OrientPath:       mp.Config.OrientPath,
			PathD:            mp.PathData,
			PivotType:        mp.Config.PivotType,
			PivotEdgeAngle:   mp.Config.PivotEdgeAngle,
			PivotNodeID:      mp.Config.PivotNodeID,
			PivotX:           mp.Config.PivotX,
			PivotY:           mp.Config.PivotY,
			RotationAngle:    mp.Config.RotationAngle,
			RotationFrom:     mp.Config.RotationFrom,
			RotationTo:       mp.Config.RotationTo,
			HasRotationRange: mp.Config.HasRotationRange,
			RotationDir:      mp.Config.RotationDir,
			ScaleFromX:       mp.Config.ScaleFromX,
			ScaleFromY:       mp.Config.ScaleFromY,
			ScaleToX:         mp.Config.ScaleToX,
			ScaleToY:         mp.Config.ScaleToY,
			OpacityFrom:      mp.Config.OpacityFrom,
			OpacityTo:        mp.Config.OpacityTo,
			HasOpacity:       mp.Config.HasOpacity,
			VisibilityState:  mp.Config.VisibilityState,
			DepthOffset:      mp.Config.DepthOffset,
			ParallaxFactor:   mp.Config.ParallaxFactor,
			HasParallax:      mp.Config.HasParallax,
			ColorTarget:      mp.Config.ColorTarget,
			ColorAngle:       mp.Config.ColorAngle,
			HasColorAngle:    mp.Config.HasColorAngle,
			FillURL:          mp.FillURL,
			Bounds:           mp.Bounds,
		}
		obj.Actions = append(obj.Actions, act)
		ctrl.Objects[objKey] = obj
		migratedCount++
	}

	return migratedCount
}
