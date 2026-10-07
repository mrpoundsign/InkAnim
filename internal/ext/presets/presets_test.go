package presets

import (
	"strings"
	"testing"

	"inkanim/internal/ext/doctree"
)

func TestPresets_Catalog(t *testing.T) {
	all := AllPresets()
	if len(all) == 0 {
		t.Fatalf("expected non-empty preset catalog")
	}

	for _, p := range all {
		if p.ID == "" || p.Name == "" || p.Directives == "" {
			t.Errorf("preset %+v has missing fields", p)
		}
		label := p.FormatLabel()
		if !strings.HasPrefix(label, p.Name+": ") {
			t.Errorf("expected label to start with '%s: ', got %q", p.Name, label)
		}

		// Verify parsing by doctree
		prefix, dirs, _ := doctree.ParseDirectives(label)
		if prefix != p.Name+":" {
			t.Errorf("expected prefix '%s:', got %q", p.Name, prefix)
		}
		if len(dirs) == 0 {
			t.Errorf("failed to parse preset directives for %q", label)
		}
	}
}

func TestPresets_GenerateAnchorElement(t *testing.T) {
	spin, ok := FindPreset("spin")
	if !ok {
		t.Fatalf("spin preset not found")
	}
	dotElem := spin.GenerateAnchorElement("g1_anchor", 100.5, 200.75, 20)
	if !strings.Contains(dotElem, `id="g1_anchor"`) {
		t.Errorf("expected anchor ID in element, got: %s", dotElem)
	}
	if !strings.Contains(dotElem, `cx="100.50"`) || !strings.Contains(dotElem, `cy="200.75"`) {
		t.Errorf("expected coordinates (100.50, 200.75), got: %s", dotElem)
	}
	if !strings.Contains(dotElem, `inkscape:label="Spin: Rot {f: 1-20; deg: 360}"`) {
		t.Errorf("expected preset label in anchor element, got: %s", dotElem)
	}

	floatPreset, ok := FindPreset("float")
	if !ok {
		t.Fatalf("float preset not found")
	}
	pathElem := floatPreset.GenerateAnchorElement("g1_float", 50, 50, 20)
	if !strings.Contains(pathElem, `<path id="g1_float"`) {
		t.Errorf("expected path element for float preset, got: %s", pathElem)
	}
	if !strings.Contains(pathElem, `d="M 50.00 50.00 L 50.00 35.00"`) {
		t.Errorf("expected vertical bob path, got: %s", pathElem)
	}

	// Test Shake frame adaptation for 20 frames
	shakePreset, ok := FindPreset("shake")
	if !ok {
		t.Fatalf("shake preset not found")
	}
	shakeLabel := shakePreset.FormatLabelForFrames(20)
	if !strings.Contains(shakeLabel, "f: 1-20") {
		t.Errorf("expected shake to adapt to 20 frames, got: %s", shakeLabel)
	}
	if !strings.Contains(shakeLabel, "pingpong") {
		t.Errorf("expected shake to include pingpong, got: %s", shakeLabel)
	}
	if !strings.Contains(shakeLabel, "r: 4") {
		t.Errorf("expected shake to have r: 4 for 20 frames, got: %s", shakeLabel)
	}
}

