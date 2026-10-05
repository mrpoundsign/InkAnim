//go:build wasm || js

package ui

import (
	"errors"
	"fmt"
	"image"
	"syscall/js"

	"inkanim/pkg/inksvg"
)

func initNativeRasterizer() {
	inksvg.CustomBatchRasterizer = rasterizeBatchWASM
	inksvg.CustomRasterizer = rasterizeSingleWASM
}

func rasterizeSingleWASM(svgData []byte, targetW, targetH int) (*image.RGBA, error) {
	frames, err := rasterizeBatchWASM([][]byte{svgData}, targetW, targetH)
	if err != nil {
		return nil, err
	}
	if len(frames) == 0 || frames[0] == nil {
		return nil, errors.New("empty rasterization result from browser")
	}
	return frames[0], nil
}

type batchRasterResult struct {
	frames []*image.RGBA
	err    error
}

func rasterizeBatchWASM(svgList [][]byte, targetW, targetH int) ([]*image.RGBA, error) {
	if len(svgList) == 0 {
		return nil, nil
	}

	fn := js.Global().Get("inkanimRasterizeBatchNative")
	if fn.IsUndefined() || fn.IsNull() {
		return nil, errors.New("browser native rasterizer not available")
	}

	jsList := js.Global().Get("Array").New(len(svgList))
	for i, svg := range svgList {
		jsList.SetIndex(i, string(svg))
	}

	done := make(chan batchRasterResult, 1)

	var onResolve js.Func
	var onReject js.Func

	onResolve = js.FuncOf(func(this js.Value, args []js.Value) any {
		defer onResolve.Release()
		defer onReject.Release()

		if len(args) == 0 {
			done <- batchRasterResult{err: errors.New("browser rasterizer returned empty arguments")}
			return nil
		}

		jsFrames := args[0]
		count := jsFrames.Length()
		frames := make([]*image.RGBA, count)
		expectedBytes := targetW * targetH * 4

		for i := 0; i < count; i++ {
			u8 := jsFrames.Index(i)
			img := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
			if !u8.IsNull() && !u8.IsUndefined() && u8.Length() == expectedBytes {
				js.CopyBytesToGo(img.Pix, u8)
			}
			frames[i] = img
		}

		done <- batchRasterResult{frames: frames}
		return nil
	})

	onReject = js.FuncOf(func(this js.Value, args []js.Value) any {
		defer onResolve.Release()
		defer onReject.Release()

		var errMsg string
		if len(args) > 0 {
			errMsg = args[0].String()
		}
		done <- batchRasterResult{err: fmt.Errorf("browser rasterization failed: %s", errMsg)}
		return nil
	})

	promise := fn.Invoke(jsList, targetW, targetH)
	promise.Call("then", onResolve, onReject)

	res := <-done
	return res.frames, res.err
}
