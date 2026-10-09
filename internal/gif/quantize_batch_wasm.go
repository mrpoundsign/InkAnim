//go:build wasm || js

package gif

import (
	"fmt"
	"image"
	"image/color"
	"syscall/js"
)

// QuantizeFrames concurrently quantizes a batch of RGBA images.
// On WASM, it offloads to a JS Web Worker pool for parallel execution.
func QuantizeFrames(frames []*image.RGBA, palette color.Palette, alphaThreshold uint8, dither bool) ([]*image.Paletted, error) {
	fn := js.Global().Get("inkanimQuantizeBatch")
	if fn.IsUndefined() || fn.IsNull() {
		// Fallback to sequential if JS orchestrator is not present
		res := make([]*image.Paletted, len(frames))
		for i, f := range frames {
			if f != nil {
				res[i] = QuantizeFrame(f, palette, alphaThreshold, dither)
			}
		}
		return res, nil
	}

	jsFrames := js.Global().Get("Array").New(len(frames))
	var w, h int
	for i, f := range frames {
		if f != nil {
			if w == 0 {
				w = f.Rect.Dx()
				h = f.Rect.Dy()
			}
			uint8Array := js.Global().Get("Uint8Array").New(len(f.Pix))
			js.CopyBytesToJS(uint8Array, f.Pix)
			jsFrames.SetIndex(i, uint8Array)
		} else {
			jsFrames.SetIndex(i, js.Null())
		}
	}

	// Pack the palette
	palLen := len(palette)
	jsPalette := js.Global().Get("Array").New(palLen)
	for i, c := range palette {
		r, g, b, _ := c.RGBA()
		colorArr := js.Global().Get("Array").New(3)
		colorArr.SetIndex(0, r>>8)
		colorArr.SetIndex(1, g>>8)
		colorArr.SetIndex(2, b>>8)
		jsPalette.SetIndex(i, colorArr)
	}

	done := make(chan struct {
		results []js.Value
		err     error
	})

	var onResolve, onReject js.Func
	onResolve = js.FuncOf(func(this js.Value, args []js.Value) any {
		defer onResolve.Release()
		defer onReject.Release()
		jsResults := args[0]
		count := jsResults.Length()
		results := make([]js.Value, count)
		for i := 0; i < count; i++ {
			results[i] = jsResults.Index(i)
		}
		done <- struct {
			results []js.Value
			err     error
		}{results, nil}
		return nil
	})

	onReject = js.FuncOf(func(this js.Value, args []js.Value) any {
		defer onResolve.Release()
		defer onReject.Release()
		errMsg := "unknown error"
		if len(args) > 0 {
			errMsg = args[0].String()
		}
		done <- struct {
			results []js.Value
			err     error
		}{nil, fmt.Errorf("worker error: %s", errMsg)}
		return nil
	})

	promise := fn.Invoke(jsFrames, w, h, jsPalette, alphaThreshold, dither)
	promise.Call("then", onResolve, onReject)

	res := <-done
	if res.err != nil {
		return nil, res.err
	}

	paletted := make([]*image.Paletted, len(frames))
	for i, jsRes := range res.results {
		if !jsRes.IsNull() && !jsRes.IsUndefined() {
			buf := make([]byte, jsRes.Length())
			js.CopyBytesToGo(buf, jsRes)
			paletted[i] = &image.Paletted{
				Pix:     buf,
				Stride:  w,
				Rect:    image.Rect(0, 0, w, h),
				Palette: palette,
			}
		}
	}

	return paletted, nil
}
