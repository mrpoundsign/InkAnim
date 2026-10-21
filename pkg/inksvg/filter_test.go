package inksvg

import (
	"image"
	"image/color"
	"testing"
)

func TestCompositeOver(t *testing.T) {
	dst := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for y := range 10 {
		for x := range 10 {
			dst.SetRGBA(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255}) // Red opaque
		}
	}

	src := image.NewRGBA(image.Rect(0, 0, 5, 5))
	for y := range 5 {
		for x := range 5 {
			src.SetRGBA(x, y, color.RGBA{R: 0, G: 255, B: 0, A: 255}) // Green opaque
		}
	}

	// Composite at offset (2, 2)
	compositeOver(dst, src, 2, 2)

	// Outside overlay should remain red
	outside := dst.RGBAAt(0, 0)
	if outside.R != 255 || outside.G != 0 || outside.B != 0 {
		t.Errorf("expected outside pixel (0,0) to be red, got %+v", outside)
	}

	// Inside overlay should be green
	inside := dst.RGBAAt(2, 2)
	if inside.R != 0 || inside.G != 255 || inside.B != 0 {
		t.Errorf("expected inside pixel (2,2) to be green, got %+v", inside)
	}

	// Test boundary clipping with offset completely off-screen
	compositeOver(dst, src, 100, 100)
	compositeOver(dst, src, -100, -100)
}

func BenchmarkCompositeOver(b *testing.B) {
	w, h := 512, 512
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	src := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(src.Pix); i += 4 {
		src.Pix[i] = 50
		src.Pix[i+1] = 100
		src.Pix[i+2] = 150
		src.Pix[i+3] = 128
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		compositeOver(dst, src, 10, 10)
	}
}
