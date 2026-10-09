//go:build !wasm && !js

package gif

import (
	"image"
	"image/color"

	"inkanim/internal/parallel"
)

// QuantizeFrames concurrently quantizes a batch of RGBA images.
// On Native platforms, this uses parallel.Run across CPU cores.
func QuantizeFrames(frames []*image.RGBA, palette color.Palette, alphaThreshold uint8, dither bool) ([]*image.Paletted, error) {
	res := make([]*image.Paletted, len(frames))
	err := parallel.Run(len(frames), func(i int) error {
		if frames[i] != nil {
			res[i] = QuantizeFrame(frames[i], palette, alphaThreshold, dither)
		}
		return nil
	})
	return res, err
}
