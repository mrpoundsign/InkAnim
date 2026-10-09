//go:build wasm || js

package worker

import (
	"image"
	"image/color"
	"syscall/js"

	"inkanim/internal/gif"
)

func Run() {
	js.Global().Set("inkanimWorkerProcess", js.FuncOf(func(this js.Value, args []js.Value) any {
		rgbaArray := args[0]
		w := args[1].Int()
		h := args[2].Int()
		jsPalette := args[3]
		alphaThresh := uint8(args[4].Int())
		dither := args[5].Bool()

		rgbaBytes := make([]byte, rgbaArray.Length())
		js.CopyBytesToGo(rgbaBytes, rgbaArray)
		
		img := &image.RGBA{
			Pix:    rgbaBytes,
			Stride: 4 * w,
			Rect:   image.Rect(0, 0, w, h),
		}

		palLen := jsPalette.Length()
		palette := make(color.Palette, palLen)
		for i := 0; i < palLen; i++ {
			c := jsPalette.Index(i)
			palette[i] = color.RGBA{
				R: uint8(c.Index(0).Int()),
				G: uint8(c.Index(1).Int()),
				B: uint8(c.Index(2).Int()),
				A: 255, // Palette is fully opaque in our quantizer
			}
		}

		paletted := gif.QuantizeFrame(img, palette, alphaThresh, dither)

		outArray := js.Global().Get("Uint8Array").New(len(paletted.Pix))
		js.CopyBytesToJS(outArray, paletted.Pix)
		return outArray
	}))

	// Signal to JS that we are bound and ready
	js.Global().Call("postMessage", "READY")
	
	// Block forever to keep the WebWorker alive
	select {}
}
