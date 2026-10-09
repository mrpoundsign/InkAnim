package gif

import (
	"cmp"
	"image"
	"image/color"
	"math"
	"slices"
)

// rgbKey represents an RGB triplet for palette generation.
type rgbKey struct {
	r, g, b uint8
}

type paletteEntry struct {
	r, g, b int32
	idx     uint8
}

type colorCacheEntry struct {
	tag uint32
	idx uint8
}

// GeneratePalette creates an optimized color.Palette with up to maxColors entries.
// Entry 0 is ALWAYS reserved as transparent color.RGBA{0, 0, 0, 0}.
func GeneratePalette(frames []*image.RGBA, maxColors int, alphaThreshold uint8) color.Palette {
	if maxColors > 256 {
		maxColors = 256
	}
	if maxColors < 2 {
		maxColors = 2
	}

	// Always reserve index 0 for transparency
	palette := color.Palette{color.RGBA{R: 0, G: 0, B: 0, A: 0}}

	// Collect color frequencies from visible pixels using direct byte slice scanning
	colorCounts := make(map[rgbKey]int)
	for _, frame := range frames {
		bounds := frame.Bounds()
		w := bounds.Dx()
		h := bounds.Dy()
		minX := bounds.Min.X - frame.Rect.Min.X
		minY := bounds.Min.Y - frame.Rect.Min.Y

		for y := range h {
			rowStart := (minY+y)*frame.Stride + minX*4
			rowEnd := rowStart + w*4
			row := frame.Pix[rowStart:rowEnd]

			for x := 0; x < len(row); x += 4 {
				if row[x+3] >= alphaThreshold {
					// Quantize slightly to 5-bit per channel to cluster close colors
					key := rgbKey{
						r: row[x] & 0xF8,
						g: row[x+1] & 0xF8,
						b: row[x+2] & 0xF8,
					}
					colorCounts[key]++
				}
			}
		}
	}

	availableSlots := maxColors - 1
	if len(colorCounts) <= availableSlots {
		for k := range colorCounts {
			palette = append(palette, color.RGBA{R: k.r, G: k.g, B: k.b, A: 255})
		}
	} else {
		// Take top availableSlots most frequent colors
		type colorFreq struct {
			key   rgbKey
			count int
		}
		list := make([]colorFreq, 0, len(colorCounts))
		for k, count := range colorCounts {
			list = append(list, colorFreq{key: k, count: count})
		}

		// Sort by frequency descending using pdqsort (O(N log N))
		slices.SortFunc(list, func(a, b colorFreq) int {
			return cmp.Compare(b.count, a.count)
		})

		for i := 0; i < availableSlots && i < len(list); i++ {
			k := list[i].key
			palette = append(palette, color.RGBA{R: k.r, G: k.g, B: k.b, A: 255})
		}
	}

	// If palette has only 1 color (e.g. all pixels are transparent), add a default color
	if len(palette) < 2 {
		palette = append(palette, color.RGBA{R: 255, G: 255, B: 255, A: 255})
	}

	return palette
}

func unpackPalette(palette color.Palette) []paletteEntry {
	if len(palette) <= 1 {
		return nil
	}
	entries := make([]paletteEntry, 0, len(palette)-1)
	for i := 1; i < len(palette); i++ {
		if c, ok := palette[i].(color.RGBA); ok {
			entries = append(entries, paletteEntry{r: int32(c.R), g: int32(c.G), b: int32(c.B), idx: uint8(i)})
		} else {
			r, g, b, _ := palette[i].RGBA()
			entries = append(entries, paletteEntry{r: int32(r >> 8), g: int32(g >> 8), b: int32(b >> 8), idx: uint8(i)})
		}
	}
	return entries
}

// QuantizeFrame converts an image.RGBA to image.Paletted using the given palette and alpha threshold.
func QuantizeFrame(src *image.RGBA, palette color.Palette, alphaThreshold uint8, dither bool) *image.Paletted {
	bounds := src.Bounds()
	paletted := image.NewPaletted(bounds, palette)

	w := bounds.Dx()
	h := bounds.Dy()
	srcMinX := bounds.Min.X - src.Rect.Min.X
	srcMinY := bounds.Min.Y - src.Rect.Min.Y
	dstMinX := bounds.Min.X - paletted.Rect.Min.X
	dstMinY := bounds.Min.Y - paletted.Rect.Min.Y

	entries := unpackPalette(palette)
	if len(entries) == 0 {
		return paletted
	}

	singleColor := len(entries) == 1
	singleIdx := entries[0].idx

	// 4096-entry direct-mapped color cache (32 KB, fits entirely in CPU L1 data cache)
	var cache [4096]colorCacheEntry

	// Bayer 8x8 matrix for ordered dithering
	var bayer8x8 = [64]int32{
		 0, 32,  8, 40,  2, 34, 10, 42,
		48, 16, 56, 24, 50, 18, 58, 26,
		12, 44,  4, 36, 14, 46,  6, 38,
		60, 28, 52, 20, 62, 30, 54, 22,
		 3, 35, 11, 43,  1, 33,  9, 41,
		51, 19, 59, 27, 49, 17, 57, 25,
		15, 47,  7, 39, 13, 45,  5, 37,
		63, 31, 55, 23, 61, 29, 53, 21,
	}

	for y := range h {
		srcRow := src.Pix[(srcMinY+y)*src.Stride+srcMinX*4 : (srcMinY+y)*src.Stride+(srcMinX+w)*4]
		dstRow := paletted.Pix[(dstMinY+y)*paletted.Stride+dstMinX : (dstMinY+y)*paletted.Stride+dstMinX+w]

		bayerRowIdx := (y & 7) * 8

		for x := range w {
			p := x * 4
			if srcRow[p+3] < alphaThreshold {
				dstRow[x] = 0 // transparent index
				continue
			}

			if singleColor {
				dstRow[x] = singleIdx
				continue
			}

			r := int32(srcRow[p])
			g := int32(srcRow[p+1])
			b := int32(srcRow[p+2])

			if dither {
				// Offset RGB by Bayer threshold mapped to roughly [-16, +15] to spread the quantization error
				threshold := (bayer8x8[bayerRowIdx+(x&7)] - 32) / 2
				r += threshold
				g += threshold
				b += threshold

				if r < 0 { r = 0 } else if r > 255 { r = 255 }
				if g < 0 { g = 0 } else if g > 255 { g = 255 }
				if b < 0 { b = 0 } else if b > 255 { b = 255 }
			}

			rgb := uint32(r)<<16 | uint32(g)<<8 | uint32(b)
			tag := rgb | 0x01000000
			hash := (rgb * 0x9E3779B9) >> 20

			if cache[hash].tag == tag {
				dstRow[x] = cache[hash].idx
				continue
			}

			// Cache miss: search pre-unpacked palette entries with integer squared Euclidean distance
			bestIdx := entries[0].idx
			bestDist := int32(math.MaxInt32)

			for i := range entries {
				dr := r - entries[i].r
				dg := g - entries[i].g
				db := b - entries[i].b
				dist := dr*dr*299 + dg*dg*587 + db*db*114
				if dist < bestDist {
					bestDist = dist
					bestIdx = entries[i].idx
					if dist == 0 {
						break
					}
				}
			}

			cache[hash] = colorCacheEntry{tag: tag, idx: bestIdx}
			dstRow[x] = bestIdx
		}
	}

	return paletted
}

// PalettedToRGBA converts an image.Paletted back to image.RGBA for display and scaling.
func PalettedToRGBA(p *image.Paletted) *image.RGBA {
	if p == nil {
		return nil
	}
	b := p.Bounds()
	rgba := image.NewRGBA(b)

	pal := make([]color.RGBA, len(p.Palette))
	for i, c := range p.Palette {
		if rgbaCol, ok := c.(color.RGBA); ok {
			pal[i] = rgbaCol
		} else {
			r, g, b, a := c.RGBA()
			pal[i] = color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}
		}
	}

	w := b.Dx()
	h := b.Dy()
	minX := b.Min.X - p.Rect.Min.X
	minY := b.Min.Y - p.Rect.Min.Y
	dstMinX := b.Min.X - rgba.Rect.Min.X
	dstMinY := b.Min.Y - rgba.Rect.Min.Y

	for y := range h {
		srcRow := p.Pix[(minY+y)*p.Stride+minX : (minY+y)*p.Stride+minX+w]
		dstRow := rgba.Pix[(dstMinY+y)*rgba.Stride+dstMinX*4 : (dstMinY+y)*rgba.Stride+(dstMinX+w)*4]
		for x := range w {
			c := pal[srcRow[x]]
			dstRow[x*4] = c.R
			dstRow[x*4+1] = c.G
			dstRow[x*4+2] = c.B
			dstRow[x*4+3] = c.A
		}
	}

	return rgba
}
