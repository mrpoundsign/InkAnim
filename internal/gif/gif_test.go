package gif

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"io"
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

func TestWriteGIFToWriter(t *testing.T) {
	frames := make([]FrameInput, 2)
	for i := range 2 {
		img := image.NewRGBA(image.Rect(0, 0, 32, 32))
		frames[i] = FrameInput{
			Index:      i,
			Label:      "Frame",
			Image:      img,
			DurationMs: 100,
		}
	}

	opts := DefaultOptions()
	var buf bytes.Buffer
	n, err := WriteGIFToWriter(&buf, frames, opts)
	if err != nil {
		t.Fatalf("WriteGIFToWriter failed: %v", err)
	}
	if n != int64(buf.Len()) {
		t.Errorf("expected %d bytes, got %d", buf.Len(), n)
	}
	if n == 0 {
		t.Errorf("expected non-zero bytes written")
	}

	decoded, err := gif.DecodeAll(&buf)
	if err != nil {
		t.Fatalf("failed to decode generated GIF: %v", err)
	}
	if len(decoded.Image) != 2 {
		t.Errorf("expected 2 frames, got %d", len(decoded.Image))
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

func TestPalettedToRGBA(t *testing.T) {
	pal := color.Palette{
		color.RGBA{R: 0, G: 0, B: 0, A: 0},       // 0: transparent
		color.RGBA{R: 255, G: 0, B: 0, A: 255},   // 1: red
		color.RGBA{R: 0, G: 255, B: 0, A: 255},   // 2: green
		color.RGBA{R: 0, G: 0, B: 255, A: 255},   // 3: blue
	}

	p := image.NewPaletted(image.Rect(0, 0, 2, 2), pal)
	p.SetColorIndex(0, 0, 0)
	p.SetColorIndex(1, 0, 1)
	p.SetColorIndex(0, 1, 2)
	p.SetColorIndex(1, 1, 3)

	rgba := PalettedToRGBA(p)
	if rgba == nil {
		t.Fatal("expected non-nil RGBA")
	}
	if rgba.Bounds() != p.Bounds() {
		t.Errorf("expected bounds %v, got %v", p.Bounds(), rgba.Bounds())
	}

	c00 := rgba.RGBAAt(0, 0)
	if c00 != (color.RGBA{R: 0, G: 0, B: 0, A: 0}) {
		t.Errorf("expected transparent at (0,0), got %+v", c00)
	}
	c10 := rgba.RGBAAt(1, 0)
	if c10 != (color.RGBA{R: 255, G: 0, B: 0, A: 255}) {
		t.Errorf("expected red at (1,0), got %+v", c10)
	}
	c01 := rgba.RGBAAt(0, 1)
	if c01 != (color.RGBA{R: 0, G: 255, B: 0, A: 255}) {
		t.Errorf("expected green at (0,1), got %+v", c01)
	}
	c11 := rgba.RGBAAt(1, 1)
	if c11 != (color.RGBA{R: 0, G: 0, B: 255, A: 255}) {
		t.Errorf("expected blue at (1,1), got %+v", c11)
	}

	// Nil safety
	if PalettedToRGBA(nil) != nil {
		t.Errorf("expected nil for nil paletted")
	}
}

func BenchmarkGeneratePalette(b *testing.B) {
	const numFrames = 6
	frames := make([]*image.RGBA, numFrames)
	for i := range numFrames {
		img := image.NewRGBA(image.Rect(0, 0, 256, 256))
		for y := range 256 {
			for x := range 256 {
				img.Set(x, y, color.RGBA{
					R: uint8((x + i*10) % 256),
					G: uint8((y + i*15) % 256),
					B: uint8((x + y) % 256),
					A: 255,
				})
			}
		}
		frames[i] = img
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		pal := GeneratePalette(frames, 256, 128)
		if len(pal) == 0 {
			b.Fatal("empty palette")
		}
	}
}

func BenchmarkQuantizeFrame_NoDither(b *testing.B) {
	img := image.NewRGBA(image.Rect(0, 0, 256, 256))
	for y := range 256 {
		for x := range 256 {
			// Emulate artwork with repeating color regions (4x4 blocks of color)
			img.Set(x, y, color.RGBA{
				R: uint8(((x / 16) * 32) % 256),
				G: uint8(((y / 16) * 32) % 256),
				B: uint8((x + y) % 256),
				A: 255,
			})
		}
	}
	pal := GeneratePalette([]*image.RGBA{img}, 256, 128)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		paletted := QuantizeFrame(img, pal, 128, false)
		if paletted == nil {
			b.Fatal("nil paletted")
		}
	}
}

func BenchmarkQuantizeFrame_Dither(b *testing.B) {
	img := image.NewRGBA(image.Rect(0, 0, 256, 256))
	for y := range 256 {
		for x := range 256 {
			img.Set(x, y, color.RGBA{
				R: uint8(((x / 16) * 32) % 256),
				G: uint8(((y / 16) * 32) % 256),
				B: uint8((x + y) % 256),
				A: 255,
			})
		}
	}
	pal := GeneratePalette([]*image.RGBA{img}, 256, 128)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		paletted := QuantizeFrame(img, pal, 128, true)
		if paletted == nil {
			b.Fatal("nil paletted")
		}
	}
}

func BenchmarkEncodeAnimatedGIF_NoDither(b *testing.B) {
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
		Dither:            false,
		DefaultDurationMs: 100,
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		anim, err := EncodeAnimatedGIF(frames, opts)
		if err != nil || anim == nil {
			b.Fatalf("EncodeAnimatedGIF failed: %v", err)
		}
	}
}

func BenchmarkEncodeAnimatedGIF_Dither(b *testing.B) {
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
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		anim, err := EncodeAnimatedGIF(frames, opts)
		if err != nil || anim == nil {
			b.Fatalf("EncodeAnimatedGIF failed: %v", err)
		}
	}
}

func BenchmarkWriteGIFToWriter(b *testing.B) {
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
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		n, err := WriteGIFToWriter(io.Discard, frames, opts)
		if err != nil || n <= 0 {
			b.Fatalf("WriteGIFToWriter failed: %v", err)
		}
	}
}
