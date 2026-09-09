package svg

import (
	"strings"
	"testing"
)

func TestPreprocess_DisplayNonePruning(t *testing.T) {
	rawSVG := `<svg width="100" height="100">
  <rect id="visible" x="0" y="0" width="100" height="100" fill="blue" />
  <circle id="hidden-attr" cx="50" cy="50" r="20" fill="red" display="none" />
  <rect id="hidden-style" x="10" y="10" width="20" height="20" fill="green" style="display:none" />
  <g id="hidden-group" style="display: none">
    <path id="child-in-hidden-group" d="M 0 0 L 10 10" stroke="yellow" />
  </g>
</svg>`

	preprocessed, err := PreprocessSVG([]byte(rawSVG))
	if err != nil {
		t.Fatalf("PreprocessSVG failed: %v", err)
	}

	res := string(preprocessed)
	if !strings.Contains(res, `id="visible"`) {
		t.Errorf("expected visible element to remain in SVG")
	}
	if strings.Contains(res, "hidden-attr") || strings.Contains(res, "hidden-style") ||
		strings.Contains(res, "hidden-group") || strings.Contains(res, "child-in-hidden-group") {
		t.Errorf("expected all display=none elements and children to be pruned, got:\n%s", res)
	}
}

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

func TestDesugarPathArcs(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "single arc untouched",
			input:    "M 10 20 a 5 5 0 0 1 10 10 Z",
			expected: "M 10 20 a 5 5 0 0 1 10 10 Z",
		},
		{
			name:     "no arcs fast path",
			input:    "M 0 0 L 10 10 C 20 20 30 30 40 40 Z",
			expected: "M 0 0 L 10 10 C 20 20 30 30 40 40 Z",
		},
		{
			name:     "implicit repeated relative arcs",
			input:    "M 10 50 a 20 20 0 0 1 40 0 20 20 0 0 1 40 0 Z",
			expected: "M 10 50 a 20 20 0 0 1 40 0 a 20 20 0 0 1 40 0 Z",
		},
		{
			name:     "implicit repeated absolute arcs",
			input:    "M 10 50 A 20 20 0 0 1 50 50 20 20 0 0 1 90 50",
			expected: "M 10 50 A 20 20 0 0 1 50 50 A 20 20 0 0 1 90 50",
		},
		{
			name:     "concatenated flags in repeated arcs",
			input:    "M 10 50 a 20 20 0 01 40 0 20 20 0 01 40 0",
			expected: "M 10 50 a 20 20 0 0 1 40 0 a 20 20 0 0 1 40 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := DesugarPathArcs(tt.input)
			// Normalize whitespace runs for comparison
			normAct := strings.Join(strings.Fields(actual), " ")
			normExp := strings.Join(strings.Fields(tt.expected), " ")
			if normAct != normExp {
				t.Errorf("DesugarPathArcs(%q) =\n  %q\nexpected:\n  %q", tt.input, normAct, normExp)
			}
		})
	}
}

func TestPreprocess_DefaultStrokeLinejoin(t *testing.T) {
	rawSVG := `<svg width="100" height="100">
  <path id="unspecified" d="M 10 10 L 50 10" stroke="red" stroke-width="2" />
  <rect id="specified-attr" x="0" y="0" width="10" height="10" stroke="blue" stroke-linejoin="round" />
  <path id="specified-style" d="M 0 0 L 10 10" style="stroke:green;stroke-linejoin:bevel" />
  <circle id="no-stroke" cx="10" cy="10" r="5" fill="yellow" />
</svg>`

	preprocessed, err := PreprocessSVG([]byte(rawSVG))
	if err != nil {
		t.Fatalf("PreprocessSVG failed: %v", err)
	}

	res := string(preprocessed)
	if !strings.Contains(res, `id="unspecified"`) || !strings.Contains(res, `stroke-linejoin="miter"`) {
		t.Errorf("expected stroke-linejoin=\"miter\" on unspecified stroke shape, got:\n%s", res)
	}
	if !strings.Contains(res, `stroke-linejoin="round"`) {
		t.Errorf("expected specified stroke-linejoin=\"round\" to be preserved")
	}
	if !strings.Contains(res, `stroke-linejoin:bevel`) {
		t.Errorf("expected specified stroke-linejoin:bevel to be preserved")
	}
}



