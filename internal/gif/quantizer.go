package gif

import (
	"image"
	"image/color"
	"image/draw"
	"math"
)

// RGBAColorKey represents an RGB triplet for palette generation.
type rgbKey struct {
	r, g, b uint8
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

	// Collect color frequencies from visible pixels
	colorCounts := make(map[rgbKey]int)
	for _, frame := range frames {
		bounds := frame.Bounds()
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				c := frame.RGBAAt(x, y)
				if c.A >= alphaThreshold {
					// Quantize slightly to 5-bit per channel to cluster close colors
					key := rgbKey{
						r: c.R & 0xF8,
						g: c.G & 0xF8,
						b: c.B & 0xF8,
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
		var list []colorFreq
		for k, count := range colorCounts {
			list = append(list, colorFreq{key: k, count: count})
		}
		// Sort by frequency descending
		for i := 0; i < len(list)-1; i++ {
			for j := i + 1; j < len(list); j++ {
				if list[j].count > list[i].count {
					list[i], list[j] = list[j], list[i]
				}
			}
			if i >= availableSlots {
				break
			}
		}

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

// QuantizeFrame converts an image.RGBA to image.Paletted using the given palette and alpha threshold.
func QuantizeFrame(src *image.RGBA, palette color.Palette, alphaThreshold uint8, dither bool) *image.Paletted {
	bounds := src.Bounds()
	paletted := image.NewPaletted(bounds, palette)

	if dither {
		draw.FloydSteinberg.Draw(paletted, bounds, src, bounds.Min)
		// Post-process to re-enforce transparency
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				origA := src.RGBAAt(x, y).A
				if origA < alphaThreshold {
					paletted.SetColorIndex(x, y, 0)
				}
			}
		}
	} else {
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				c := src.RGBAAt(x, y)
				if c.A < alphaThreshold {
					paletted.SetColorIndex(x, y, 0) // transparent index
				} else {
					bestIdx := findClosestPaletteIndex(c, palette)
					paletted.SetColorIndex(x, y, uint8(bestIdx))
				}
			}
		}
	}

	return paletted
}

func findClosestPaletteIndex(c color.RGBA, palette color.Palette) int {
	bestIdx := 1
	bestDist := math.MaxFloat64

	r := float64(c.R)
	g := float64(c.G)
	b := float64(c.B)

	// Skip index 0 because it's transparent
	for i := 1; i < len(palette); i++ {
		pc, ok := palette[i].(color.RGBA)
		if !ok {
			r1, g1, b1, _ := palette[i].RGBA()
			pc = color.RGBA{R: uint8(r1 >> 8), G: uint8(g1 >> 8), B: uint8(b1 >> 8), A: 255}
		}
		dr := r - float64(pc.R)
		dg := g - float64(pc.G)
		db := b - float64(pc.B)

		// Weighted Euclidean distance (human perception)
		dist := dr*dr*0.299 + dg*dg*0.587 + db*db*0.114
		if dist < bestDist {
			bestDist = dist
			bestIdx = i
		}
	}

	return bestIdx
}
