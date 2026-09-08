package svg

import (
	"strings"
	"testing"
)

func TestExpandUseElements_BasicAndChained(t *testing.T) {
	rawSVG := `<svg width="100" height="100">
  <defs>
    <circle id="dot" cx="10" cy="10" r="5" fill="red" />
    <g id="two_dots">
      <use href="#dot" x="5" y="5" />
      <use href="#dot" x="20" y="20" />
    </g>
  </defs>
  <use href="#two_dots" transform="scale(2)" />
</svg>`

	preprocessed, err := PreprocessSVG([]byte(rawSVG))
	if err != nil {
		t.Fatalf("PreprocessSVG failed: %v", err)
	}

	result := string(preprocessed)

	// No <use> elements should remain in the rendered tree
	if strings.Contains(result, "<use") {
		t.Errorf("expected all <use> tags to be expanded, found in result:\n%s", result)
	}

	// Should contain the cloned circles
	if !strings.Contains(result, `cx="10"`) || !strings.Contains(result, `fill="red"`) {
		t.Errorf("expected cloned circle attributes in result:\n%s", result)
	}

	// Should contain matrix transformation from scale(2)
	if !strings.Contains(result, `matrix(2.000000 0.000000 0.000000 2.000000`) {
		t.Errorf("expected scale(2) matrix in result:\n%s", result)
	}
}

func TestExpandUseElements_MissingReference(t *testing.T) {
	rawSVG := `<svg width="100" height="100">
  <use href="#nonexistent" x="10" y="10" />
</svg>`

	preprocessed, err := PreprocessSVG([]byte(rawSVG))
	if err != nil {
		t.Fatalf("PreprocessSVG failed: %v", err)
	}

	result := string(preprocessed)
	// Missing references should safely fall through without crashing
	if !strings.Contains(result, "nonexistent") {
		t.Errorf("expected non-existent reference to be preserved gracefully:\n%s", result)
	}
}

func TestGradientStopInliningAndTransform(t *testing.T) {
	rawSVG := `<svg width="128" height="128" viewBox="0 0 128 128" xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink">
  <defs>
    <linearGradient id="palette">
      <stop offset="0%" stop-color="#3b82f6" />
      <stop offset="100%" stop-color="#ef4444" />
    </linearGradient>
    <linearGradient id="linGrad" xlink:href="#palette" x1="16" y1="36" x2="112" y2="36" gradientUnits="userSpaceOnUse" gradientTransform="rotate(15 64 36)" />
    <radialGradient id="radGrad" href="#palette" cx="64" cy="92" r="28" gradientUnits="userSpaceOnUse" />
  </defs>
  <rect x="16" y="16" width="96" height="40" fill="url(#linGrad)" />
  <circle cx="64" cy="92" r="28" fill="url(#radGrad)" />
</svg>`

	preprocessed, err := PreprocessSVG([]byte(rawSVG))
	if err != nil {
		t.Fatalf("PreprocessSVG failed: %v", err)
	}

	result := string(preprocessed)

	// linGrad should have stops inlined and href stripped
	if !strings.Contains(result, `id="linGrad"`) {
		t.Errorf("expected linGrad in result:\n%s", result)
	}
	if strings.Contains(result, `href="#palette"`) {
		t.Errorf("expected href to be stripped from inlined gradients:\n%s", result)
	}
	// Verify gradientTransform was normalized to matrix(...)
	if !strings.Contains(result, `gradientTransform="matrix(`) {
		t.Errorf("expected normalized matrix(...) gradientTransform in result:\n%s", result)
	}
	// Verify stop elements are present inside linGrad
	linGradPos := strings.Index(result, `id="linGrad"`)
	if linGradPos == -1 {
		t.Fatalf("linGrad not found in result")
	}
	subAfterLinGrad := result[linGradPos:]
	endLinGradPos := strings.Index(subAfterLinGrad, `</linearGradient>`)
	if endLinGradPos == -1 {
		t.Fatalf("end of linGrad not found in result")
	}
	linGradBody := subAfterLinGrad[:endLinGradPos]
	if !strings.Contains(linGradBody, `<stop`) || !strings.Contains(linGradBody, `#3b82f6`) {
		t.Errorf("expected inlined stops inside linGrad, got:\n%s", linGradBody)
	}
}

