package ui

import (
	"image"
	"math"
	"time"

	"inkanim/internal/gif"
	"inkanim/internal/prof"
	"inkanim/pkg/inksvg"
)

// previewSnapshot captures everything buildPreviewCache needs, so the build can
// run off the UI thread without touching panel or session state.
type previewSnapshot struct {
	images         []*image.RGBA // one per rendered frame; nil entries are skipped
	activeRects    []inksvg.Rect // GetActiveBoundaryRect(i) per frame
	previewRect    inksvg.Rect
	exportSquare   bool
	wysiwyg        bool
	numColors      int
	alphaThreshold uint8
	dither         bool
}

// progressFunc reports build progress. total == 0 means the stage is indeterminate.
type progressFunc func(stage string, done, total int)

// yieldInterval bounds how long the build runs between yields to the browser.
const yieldInterval = 16 * time.Millisecond

// yielder lets a long CPU-bound build hand control back to the host event loop.
// On js/wasm this is what lets progress repaint and input stay responsive.
type yielder struct{ last time.Time }

func (y *yielder) maybe() {
	if time.Since(y.last) >= yieldInterval {
		y.force()
	}
}

func (y *yielder) force() {
	yieldToHost()
	y.last = time.Now()
}

func (p *CenterPreviewPanel) snapshotLocked() previewSnapshot {
	frames := p.session.RenderedFrames
	s := previewSnapshot{
		images:         make([]*image.RGBA, len(frames)),
		activeRects:    make([]inksvg.Rect, len(frames)),
		previewRect:    p.session.GetPreviewBoundaryRect(),
		exportSquare:   p.session.ExportOptions.ExportSquare,
		wysiwyg:        p.wysiwygColors,
		numColors:      p.session.ExportOptions.NumColors,
		alphaThreshold: p.session.ExportOptions.AlphaThreshold,
		dither:         p.session.ExportOptions.Dither,
	}
	for i, f := range frames {
		s.images[i] = f.Image
		s.activeRects[i] = p.session.GetActiveBoundaryRect(i)
	}
	return s
}

// buildPreviewCache produces the display frames (with/without crop guides) and the
// Twitch scale-inspector thumbnails for every frame. It is pure: it only reads the
// snapshot, so it is safe to run on a background goroutine.
func buildPreviewCache(s previewSnapshot, progress progressFunc) []cachedPreviewFrame {
	tTotal := time.Now()
	n := len(s.images)
	out := make([]cachedPreviewFrame, n)
	if n == 0 {
		return out
	}
	y := &yielder{last: time.Now()}

	displayImgs := make([]*image.RGBA, n)
	for i, img := range s.images {
		if img == nil {
			continue
		}
		if s.exportSquare {
			displayImgs[i] = gif.MakeSquare(img, 0)
			progress("Preparing frames", i+1, n)
			y.maybe()
		} else {
			displayImgs[i] = img
		}
	}

	// Apply WYSIWYG quantization and dithering if enabled
	if s.wysiwyg {
		numColors := s.numColors
		if numColors <= 0 || numColors > 256 {
			numColors = 256
		}
		alphaThreshold := s.alphaThreshold
		if alphaThreshold == 0 {
			alphaThreshold = 128
		}

		progress("Building palette…", 0, 0)
		y.force()
		tPal := time.Now()
		palette := gif.GeneratePalette(displayImgs, numColors, alphaThreshold)
		prof.Since(tPal, "L6a GeneratePalette (%d frames)", n)

		progress("Quantizing colors…", 0, 0)
		y.force()
		tQuant := time.Now()
		palettedList, _ := gif.QuantizeFrames(displayImgs, palette, alphaThreshold, s.dither)
		prof.Since(tQuant, "L6b QuantizeFrames (%d frames, dither=%v)", n, s.dither)

		tPalToRGBA := time.Now()
		for i, pal := range palettedList {
			if pal != nil {
				displayImgs[i] = gif.PalettedToRGBA(pal)
			}
			progress("Applying palette", i+1, len(palettedList))
			y.maybe()
		}
		prof.Since(tPalToRGBA, "L6c PalettedToRGBA (%d frames)", len(palettedList))
	}

	tPreviews := time.Now()
	for i, img := range s.images {
		if img == nil {
			continue
		}

		activeRect := s.activeRects[i]
		scaleX := float64(img.Bounds().Dx()) / s.previewRect.Width
		scaleY := float64(img.Bounds().Dy()) / s.previewRect.Height
		cropX0 := int(math.Round((activeRect.X - s.previewRect.X) * scaleX))
		cropY0 := int(math.Round((activeRect.Y - s.previewRect.Y) * scaleY))
		cropW := int(math.Round(activeRect.Width * scaleX))
		cropH := int(math.Round(activeRect.Height * scaleY))
		cropRect := image.Rect(cropX0, cropY0, cropX0+cropW, cropY0+cropH)

		displayImg := displayImgs[i]
		var contentRect image.Rectangle
		if s.exportSquare {
			origB := img.Bounds()
			sqB := displayImg.Bounds()
			offsetX := (sqB.Dx() - origB.Dx()) / 2
			offsetY := (sqB.Dy() - origB.Dy()) / 2
			contentRect = cropRect.Add(image.Pt(offsetX, offsetY))
		} else {
			contentRect = cropRect
		}

		croppedForTwitch := cropImage(displayImg, contentRect)

		var twitchDisplayImg *image.RGBA
		if s.exportSquare {
			twitchDisplayImg = gif.MakeSquare(croppedForTwitch, 0)
		} else {
			twitchDisplayImg = croppedForTwitch
		}

		// Pre-scale Twitch thumbnails once to exact target dimensions
		out[i] = cachedPreviewFrame{
			withGuides:    drawCropGuides(displayImg, contentRect),
			withoutGuides: displayImg,
			twitch112:     scaleRGBA(twitchDisplayImg, 112, 112),
			twitch56:      scaleRGBA(twitchDisplayImg, 56, 56),
			twitch28:      scaleRGBA(twitchDisplayImg, 28, 28),
		}
		progress("Building previews", i+1, n)
		y.maybe()
	}
	prof.Since(tPreviews, "L6d crop, guides & Twitch thumbnails (%d frames)", n)
	prof.Since(tTotal, "L6 buildPreviewCache total (%d frames)", n)
	return out
}
