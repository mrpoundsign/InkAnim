package presets

import "fmt"

// Preset defines a motion preset.
type Preset struct {
	ID          string // unique preset identifier, e.g. "spin"
	Name        string // display name, e.g. "Spin"
	Description string // user description
	Type        string // primary motion type: "Rot", "Scale", "Move", "Fade", "Show"
	Directives  string // raw directives string without prefix, e.g. "Rot {f: 1-20; angle: 360}"
	NeedsAnchor bool   // true if applying to a group creates an anchor object
	PathType    string // "circle" for anchor dot, "path" for linear motion path, or ""
}

// AllPresets returns the available motion preset library.
func AllPresets() []Preset {
	return []Preset{
		{
			ID:          "spin",
			Name:        "Spin",
			Description: "Continuous 360° rotation around center",
			Type:        "Rot",
			Directives:  "Rot {f: 1-20; deg: 360}",
			NeedsAnchor: true,
			PathType:    "circle",
		},
		{
			ID:          "pulse",
			Name:        "Pulse",
			Description: "Smooth expansion and contraction (pingpong)",
			Type:        "Scale",
			Directives:  "Scale {f: 1-20; from: 1.0; to: 1.25; pingpong}",
			NeedsAnchor: true,
			PathType:    "circle",
		},
		{
			ID:          "float",
			Name:        "Float",
			Description: "Gentle up/down bobbing motion (pingpong)",
			Type:        "Move",
			Directives:  "Move {f: 1-20; ease: in-out; pingpong}",
			NeedsAnchor: true,
			PathType:    "path",
		},
		{
			ID:          "shake",
			Name:        "Shake",
			Description: "Rapid subtle vibration and wobble",
			Type:        "Rot",
			Directives:  "Rot {f: 1-10; deg: 8; pingpong}",
			NeedsAnchor: true,
			PathType:    "circle",
		},
		{
			ID:          "fade_in",
			Name:        "Fade In",
			Description: "Gradual opacity increase from 0% to 100%",
			Type:        "Fade",
			Directives:  "Fade {f: 1-20; from: 0; to: 1}",
			NeedsAnchor: true,
			PathType:    "circle",
		},
		{
			ID:          "fade_out",
			Name:        "Fade Out",
			Description: "Gradual opacity decrease from 100% to 0%",
			Type:        "Fade",
			Directives:  "Fade {f: 1-20; from: 1; to: 0}",
			NeedsAnchor: true,
			PathType:    "circle",
		},
		{
			ID:          "blink",
			Name:        "Blink",
			Description: "Alternating show/hide flashing",
			Type:        "Show",
			Directives:  "Show {f: 1-10} Hide {f: 11-20}",
			NeedsAnchor: true,
			PathType:    "circle",
		},
	}
}

// FindPreset returns the preset matching the given ID or name.
func FindPreset(idOrName string) (Preset, bool) {
	for _, p := range AllPresets() {
		if p.ID == idOrName || p.Name == idOrName {
			return p, true
		}
	}
	return Preset{}, false
}

// DirectivesForFrames returns the motion directives customized for the given document frame count.
func (p Preset) DirectivesForFrames(totalFrames int) string {
	if totalFrames <= 0 {
		totalFrames = 20
	}
	switch p.ID {
	case "spin":
		return fmt.Sprintf("Rot {f: 1-%d; deg: 360}", totalFrames)
	case "pulse":
		return fmt.Sprintf("Scale {f: 1-%d; from: 1.0; to: 1.25; pingpong}", totalFrames)
	case "float":
		return fmt.Sprintf("Move {f: 1-%d; ease: in-out; pingpong}", totalFrames)
	case "shake":
		r := max(totalFrames/5, 2)
		return fmt.Sprintf("Rot {f: 1-%d; deg: 8; pingpong; r: %d}", totalFrames, r)
	case "fade_in":
		return fmt.Sprintf("Fade {f: 1-%d; from: 0; to: 1}", totalFrames)
	case "fade_out":
		return fmt.Sprintf("Fade {f: 1-%d; from: 1; to: 0}", totalFrames)
	case "blink":
		half := max(totalFrames/2, 1)
		return fmt.Sprintf("Show {f: 1-%d} Hide {f: %d-%d}", half, half+1, totalFrames)
	default:
		return p.Directives
	}
}

// FormatLabelForFrames returns the prefixed label for a preset customized for totalFrames.
func (p Preset) FormatLabelForFrames(totalFrames int) string {
	return fmt.Sprintf("%s: %s", p.Name, p.DirectivesForFrames(totalFrames))
}

// FormatLabel returns the prefixed label for a preset with default 20 frames.
func (p Preset) FormatLabel() string {
	return p.FormatLabelForFrames(20)
}

// GenerateAnchorElement creates an XML anchor element string for the preset centered at (cx, cy).
func (p Preset) GenerateAnchorElement(anchorID string, cx, cy float64, totalFrames int) string {
	label := p.FormatLabelForFrames(totalFrames)
	if p.PathType == "path" {
		d := fmt.Sprintf("M %.2f %.2f L %.2f %.2f", cx, cy, cx, cy-15)
		return fmt.Sprintf(`<path id="%s" d="%s" style="fill:none;stroke:#38bdf8;stroke-width:1;stroke-dasharray:2,2" inkscape:label="%s" />`,
			anchorID, d, label)
	}
	return fmt.Sprintf(`<circle id="%s" cx="%.2f" cy="%.2f" r="1.5" style="fill:#38bdf8;stroke:none" inkscape:label="%s" />`,
		anchorID, cx, cy, label)
}
