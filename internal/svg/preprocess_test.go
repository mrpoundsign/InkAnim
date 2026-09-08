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
