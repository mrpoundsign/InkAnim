package svg

import (
	"testing"
)

func TestDecodeDataURI_ValidPNG(t *testing.T) {
	// Minimal 1x1 transparent PNG
	dataURI := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
	img, err := decodeDataURI(dataURI)
	if err != nil {
		t.Fatalf("failed to decode valid data URI: %v", err)
	}
	if img.Bounds().Dx() != 1 || img.Bounds().Dy() != 1 {
		t.Errorf("expected 1x1 image, got %dx%d", img.Bounds().Dx(), img.Bounds().Dy())
	}
}

func TestDecodeDataURI_Invalid(t *testing.T) {
	tests := []struct {
		name string
		uri  string
	}{
		{"not data uri", "https://example.com/image.png"},
		{"missing comma", "data:image/png;base64"},
		{"not base64", "data:image/png,notbase64"},
		{"corrupt base64", "data:image/png;base64,!!!invalid!!!"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := decodeDataURI(tc.uri)
			if err == nil {
				t.Errorf("expected error for %s, got nil", tc.name)
			}
		})
	}
}

func TestExtractEmbeddedImages_ZOrderAndTransforms(t *testing.T) {
	svgXML := `<svg width="100" height="100">
  <rect width="50" height="50" fill="red" />
  <g transform="translate(10, 20)">
    <image x="5" y="5" width="20" height="20" opacity="0.8"
           href="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==" />
  </g>
  <circle cx="25" cy="25" r="10" fill="blue" />
</svg>`

	images := extractEmbeddedImages([]byte(svgXML))
	if len(images) != 1 {
		t.Fatalf("expected 1 embedded image, got %d", len(images))
	}

	emb := images[0]
	if emb.PathIndex != 1 {
		t.Errorf("expected PathIndex 1 (after rect, before circle), got %d", emb.PathIndex)
	}
	if emb.Opacity != 0.8 {
		t.Errorf("expected opacity 0.8, got %f", emb.Opacity)
	}
	if emb.Transform.E != 10 || emb.Transform.F != 20 {
		t.Errorf("expected transform translate(10, 20), got (E=%f, F=%f)", emb.Transform.E, emb.Transform.F)
	}
}
