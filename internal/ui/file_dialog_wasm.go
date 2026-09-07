//go:build wasm || js

package ui

import (
	"bytes"
	"fmt"
	"syscall/js"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
)

func (mw *MainWindow) initPlatform() {
	doc := js.Global().Get("document")
	if doc.IsUndefined() || doc.IsNull() {
		return
	}

	var onDragOver js.Func
	onDragOver = js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) > 0 {
			args[0].Call("preventDefault")
		}
		return nil
	})
	doc.Call("addEventListener", "dragover", onDragOver)

	var onDrop js.Func
	onDrop = js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) == 0 {
			return nil
		}
		e := args[0]
		e.Call("preventDefault")
		dt := e.Get("dataTransfer")
		if dt.IsUndefined() || dt.IsNull() {
			return nil
		}
		files := dt.Get("files")
		if files.Length() == 0 {
			return nil
		}
		file := files.Index(0)
		name := file.Get("name").String()

		go func() {
			reader := js.Global().Get("FileReader").New()
			done := make(chan []byte, 1)

			var onLoad js.Func
			onLoad = js.FuncOf(func(this js.Value, args []js.Value) any {
				defer onLoad.Release()
				result := reader.Get("result")
				uint8Array := js.Global().Get("Uint8Array").New(result)
				length := uint8Array.Length()
				data := make([]byte, length)
				js.CopyBytesToGo(data, uint8Array)
				done <- data
				return nil
			})

			var onError js.Func
			onError = js.FuncOf(func(this js.Value, args []js.Value) any {
				defer onError.Release()
				done <- nil
				return nil
			})

			reader.Set("onload", onLoad)
			reader.Set("onerror", onError)
			reader.Call("readAsArrayBuffer", file)

			data := <-done
			if data != nil {
				fyne.Do(func() {
					mw.loadData(data, name)
				})
			}
		}()
		return nil
	})
	doc.Call("addEventListener", "drop", onDrop)

	var loadBytes js.Func
	loadBytes = js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) < 2 {
			return nil
		}
		name := args[0].String()
		uint8Array := args[1]
		length := uint8Array.Length()
		data := make([]byte, length)
		js.CopyBytesToGo(data, uint8Array)
		fyne.Do(func() {
			mw.loadData(data, name)
		})
		return nil
	})
	js.Global().Set("inkanimLoadBytes", loadBytes)
}

func (mw *MainWindow) promptOpenFile() {
	doc := js.Global().Get("document")
	body := doc.Get("body")
	input := doc.Call("createElement", "input")
	input.Set("type", "file")
	input.Set("accept", ".svg")
	input.Get("style").Set("display", "none")
	body.Call("appendChild", input)

	var onChange js.Func
	onChange = js.FuncOf(func(this js.Value, args []js.Value) any {
		go func() {
			defer onChange.Release()
			files := input.Get("files")
			if files.Length() == 0 {
				body.Call("removeChild", input)
				return
			}

			file := files.Index(0)
			name := file.Get("name").String()

			reader := js.Global().Get("FileReader").New()
			done := make(chan []byte, 1)

			var onLoad js.Func
			onLoad = js.FuncOf(func(this js.Value, args []js.Value) any {
				defer onLoad.Release()
				result := reader.Get("result")
				uint8Array := js.Global().Get("Uint8Array").New(result)
				length := uint8Array.Length()
				data := make([]byte, length)
				js.CopyBytesToGo(data, uint8Array)
				done <- data
				return nil
			})

			var onError js.Func
			onError = js.FuncOf(func(this js.Value, args []js.Value) any {
				defer onError.Release()
				done <- nil
				return nil
			})

			reader.Set("onload", onLoad)
			reader.Set("onerror", onError)
			reader.Call("readAsArrayBuffer", file)

			data := <-done
			body.Call("removeChild", input)

			if data != nil {
				fyne.Do(func() {
					mw.loadData(data, name)
				})
			}
		}()
		return nil
	})

	input.Call("addEventListener", "change", onChange)
	input.Call("click")
}

// PromptExport exports the animation as an animated GIF and triggers a browser download.
func (p *RightExportPanel) PromptExport() {
	var resume func()
	if p.pausePlayback != nil {
		resume = p.pausePlayback()
	} else {
		resume = func() {}
	}

	if len(p.session.RenderedFrames) == 0 {
		d := dialog.NewInformation("No Frames", "Please load an SVG with animation frames before exporting.", p.parentWindow)
		d.SetOnClosed(resume)
		d.Show()
		return
	}

	p.exportBtn.SetText("Exporting...")
	p.exportBtn.Disable()

	go func() {
		var buf bytes.Buffer
		sizeBytes, expErr := p.session.ExportGIFWriter(&buf)

		fyne.Do(func() {
			p.validateTwitch()
			p.exportBtn.Enable()

			if expErr != nil {
				d := dialog.NewError(expErr, p.parentWindow)
				d.SetOnClosed(resume)
				d.Show()
				return
			}

			data := buf.Bytes()
			uint8Array := js.Global().Get("Uint8Array").New(len(data))
			js.CopyBytesToJS(uint8Array, data)

			blobParts := js.Global().Get("Array").New(1)
			blobParts.SetIndex(0, uint8Array)

			blobOptions := js.Global().Get("Object").New()
			blobOptions.Set("type", "image/gif")

			blob := js.Global().Get("Blob").New(blobParts, blobOptions)
			url := js.Global().Get("URL").Call("createObjectURL", blob)

			doc := js.Global().Get("document")
			body := doc.Get("body")
			a := doc.Call("createElement", "a")
			a.Set("href", url)
			a.Set("download", "emote.gif")
			a.Get("style").Set("display", "none")
			body.Call("appendChild", a)
			a.Call("click")
			body.Call("removeChild", a)
			js.Global().Get("URL").Call("revokeObjectURL", url)

			d := dialog.NewInformation("Export Succeeded",
				fmt.Sprintf("Successfully exported animated GIF:\nemote.gif\n\nFile Size: %0.2f KB", float64(sizeBytes)/1024.0),
				p.parentWindow)
			d.SetOnClosed(resume)
			d.Show()
		})
	}()
}
