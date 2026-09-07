package gif

import (
	"image"
	"image/color"
	"testing"
)

func TestMakeSquare(t *testing.T) {
	// Create rectangular 200x100 image
	src := image.NewRGBA(image.Rect(0, 0, 200, 100))
	for y := range 100 {
		for x := range 200 {
			src.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
		}
	}

	// 1. Automatic square (targetSize = 0 => max(200, 100) = 200)
	sq := MakeSquare(src, 0)
	if sq.Bounds().Dx() != 200 || sq.Bounds().Dy() != 200 {
		t.Fatalf("expected 200x200, got %dx%d", sq.Bounds().Dx(), sq.Bounds().Dy())
	}

	// Verify top and bottom padding are transparent (y=10, x=100 should be transparent)
	topPad := sq.RGBAAt(100, 10)
	if topPad.A != 0 {
		t.Errorf("expected transparent top padding, got %+v", topPad)
	}

	// Verify center pixel is red
	center := sq.RGBAAt(100, 100)
	if center.R != 255 || center.A != 255 {
		t.Errorf("expected red center, got %+v", center)
	}

	// 2. Target square size 512
	sq512 := MakeSquare(src, 512)
	if sq512.Bounds().Dx() != 512 || sq512.Bounds().Dy() != 512 {
		t.Fatalf("expected 512x512, got %dx%d", sq512.Bounds().Dx(), sq512.Bounds().Dy())
	}
}

func TestEncodeAnimatedGIF(t *testing.T) {
	frames := make([]FrameInput, 3)
	for i := range 3 {
		img := image.NewRGBA(image.Rect(0, 0, 64, 32))
		// Color each frame differently
		var c color.RGBA
		switch i {
		case 0:
			c = color.RGBA{R: 255, G: 0, B: 0, A: 255}
		case 1:
			c = color.RGBA{R: 0, G: 255, B: 0, A: 255}
		case 2:
			c = color.RGBA{R: 0, G: 0, B: 255, A: 255}
		}
		for y := 8; y < 24; y++ {
			for x := 16; x < 48; x++ {
				img.Set(x, y, c)
			}
		}
		frames[i] = FrameInput{
			Index:      i,
			Label:      "Frame",
			Image:      img,
			DurationMs: 100,
		}
	}

	opts := DefaultOptions()
	opts.ExportSquare = true

	anim, err := EncodeAnimatedGIF(frames, opts)
	if err != nil {
		t.Fatalf("EncodeAnimatedGIF failed: %v", err)
	}

	if len(anim.Image) != 3 {
		t.Errorf("expected 3 paletted frames, got %d", len(anim.Image))
	}

	// ExportSquare should make each frame 64x64 (max of 64, 32)
	if anim.Image[0].Bounds().Dx() != 64 || anim.Image[0].Bounds().Dy() != 64 {
		t.Errorf("expected 64x64 square bounds, got %dx%d", anim.Image[0].Bounds().Dx(), anim.Image[0].Bounds().Dy())
	}

	// Delay units: 100ms / 10 = 10 units
	if anim.Delay[0] != 10 {
		t.Errorf("expected delay 10, got %d", anim.Delay[0])
	}
}

func TestValidateTwitchEmote(t *testing.T) {
	// Valid square emote
	res := ValidateTwitchEmote(10, 1000, 112, 112, 200000)
	if !res.IsValid || len(res.Errors) > 0 {
		t.Errorf("expected valid emote, got: %+v", res)
	}

	// Non-square emote
	resNonSquare := ValidateTwitchEmote(10, 1000, 120, 112, 200000)
	if resNonSquare.IsValid {
		t.Errorf("expected invalid for non-square")
	}

	// Over 1MB
	resOversized := ValidateTwitchEmote(10, 1000, 512, 512, 1200000)
	if resOversized.IsValid {
		t.Errorf("expected invalid for oversized file")
	}
}

func BenchmarkEncodeAnimatedGIF(b *testing.B) {
	const numFrames = 12
	frames := make([]FrameInput, numFrames)
	for i := range numFrames {
		img := image.NewRGBA(image.Rect(0, 0, 128, 128))
		for y := range 128 {
			for x := range 128 {
				img.Set(x, y, color.RGBA{
					R: uint8((x + i*10) % 256),
					G: uint8((y + i*15) % 256),
					B: uint8((x + y) % 256),
					A: 255,
				})
			}
		}
		frames[i] = FrameInput{
			Index:      i,
			Label:      "frame",
			Image:      img,
			DurationMs: 100,
		}
	}

	opts := ExportOptions{
		NumColors:         256,
		Dither:            true,
		DefaultDurationMs: 100,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		anim, err := EncodeAnimatedGIF(frames, opts)
		if err != nil || anim == nil {
			b.Fatalf("EncodeAnimatedGIF failed: %v", err)
		}
	}
}
