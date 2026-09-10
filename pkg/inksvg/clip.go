package inksvg

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"image"
	"image/draw"
	"strings"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
)

// extractClipPaths parses <clipPath id="..."> definitions from the SVG document
// and returns their inner XML content keyed by clip ID.
func extractClipPaths(svgData []byte) map[string][]byte {
	dec := xml.NewDecoder(bytes.NewReader(svgData))
	clips := make(map[string][]byte)

	var curID string
	var inClip bool
	var clipBuf bytes.Buffer
	var enc *xml.Encoder

	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}

		switch elem := tok.(type) {
		case xml.StartElement:
			if elem.Name.Local == "clipPath" {
				for _, a := range elem.Attr {
					if a.Name.Local == "id" {
						curID = a.Value
					}
				}
				if curID != "" {
					inClip = true
					clipBuf.Reset()
					enc = xml.NewEncoder(&clipBuf)
					continue
				}
			}
			if inClip {
				// Normalize shapes inside clipPath to have fill="#ffffff" and stroke="none"
				fillIdx := -1
				strokeIdx := -1
				for i, a := range elem.Attr {
					if a.Name.Local == "fill" {
						fillIdx = i
					}
					if a.Name.Local == "stroke" {
						strokeIdx = i
					}
				}
				if fillIdx >= 0 {
					elem.Attr[fillIdx].Value = "#ffffff"
				} else {
					elem.Attr = append(elem.Attr, xml.Attr{Name: xml.Name{Local: "fill"}, Value: "#ffffff"})
				}
				if strokeIdx >= 0 {
					elem.Attr[strokeIdx].Value = "none"
				} else {
					elem.Attr = append(elem.Attr, xml.Attr{Name: xml.Name{Local: "stroke"}, Value: "none"})
				}
				_ = enc.EncodeToken(elem)
			}

		case xml.EndElement:
			if elem.Name.Local == "clipPath" && inClip {
				_ = enc.Flush()
				clips[curID] = clipBuf.Bytes()
				inClip = false
				curID = ""
				continue
			}
			if inClip {
				_ = enc.EncodeToken(elem)
			}

		default:
			if inClip {
				_ = enc.EncodeToken(tok)
			}
		}
	}
	return clips
}

// extractPathClipIDs maps each path index in the document to its active clip-path ID.
func extractPathClipIDs(svgData []byte) []string {
	dec := xml.NewDecoder(bytes.NewReader(svgData))
	clipStack := []string{""}
	var pathClipIDs []string
	var inDefs int

	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}

		switch elem := tok.(type) {
		case xml.StartElement:
			name := elem.Name.Local
			if name == "defs" {
				inDefs++
			}

			curClip := clipStack[len(clipStack)-1]
			for _, a := range elem.Attr {
				if a.Name.Local == "clip-path" {
					curClip = parseClipURL(a.Value)
				}
				if a.Name.Local == "style" {
					if cp := extractCSSProp(a.Value, "clip-path"); cp != "" {
						curClip = parseClipURL(cp)
					}
				}
			}

			if name == "g" {
				clipStack = append(clipStack, curClip)
			}

			if inDefs == 0 && isShapeElement(name) {
				pathClipIDs = append(pathClipIDs, curClip)
			}

		case xml.EndElement:
			name := elem.Name.Local
			if name == "defs" && inDefs > 0 {
				inDefs--
			}
			if name == "g" && len(clipStack) > 1 {
				clipStack = clipStack[:len(clipStack)-1]
			}
		}
	}
	return pathClipIDs
}

func parseClipURL(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "url(")
	v = strings.TrimSuffix(v, ")")
	v = strings.Trim(v, `"'`)
	v = strings.TrimPrefix(v, "#")
	return strings.TrimSpace(v)
}

// renderClipMask renders the inner geometry of a <clipPath> into an RGBA mask image.
func renderClipMask(clipInner []byte, rootSVG []byte, w, h int, transform rasterx.Matrix2D) (*image.RGBA, error) {
	// Extract root SVG attributes (viewBox)
	doc, err := ParseSVG(rootSVG)
	if err != nil {
		return nil, fmt.Errorf("failed to parse root SVG for clip mask: %w", err)
	}
	vbStr := fmt.Sprintf("%f %f %f %f", doc.ViewBoxX, doc.ViewBoxY, doc.ViewBoxW, doc.ViewBoxH)
	if doc.ViewBoxW <= 0 || doc.ViewBoxH <= 0 {
		vbStr = fmt.Sprintf("0 0 %f %f", doc.Width, doc.Height)
	}

	maskSVG := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="%s">
%s
</svg>`, w, h, vbStr, string(clipInner))

	// Preprocess and render mask SVG
	preprocessed, err := PreprocessSVG([]byte(maskSVG))
	if err != nil {
		preprocessed = []byte(maskSVG)
	}

	icon, err := oksvg.ReadIconStream(bytes.NewReader(preprocessed))
	if err != nil {
		return nil, err
	}

	icon.Transform = transform

	maskImg := image.NewRGBA(image.Rect(0, 0, w, h))
	scanner := rasterx.NewScannerGV(w, h, maskImg, maskImg.Bounds())
	raster := rasterx.NewDasher(w, h, scanner)
	icon.Draw(raster, 1.0)

	return maskImg, nil
}

// applyAlphaMask multiplies the RGB and Alpha channels of dst by the alpha channel of mask.
func applyAlphaMask(dst *image.RGBA, mask *image.RGBA) {
	b := dst.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			dOff := dst.PixOffset(x, y)
			mOff := mask.PixOffset(x, y)
			maskA := uint32(mask.Pix[mOff+3])
			if maskA == 0 {
				dst.Pix[dOff+0] = 0
				dst.Pix[dOff+1] = 0
				dst.Pix[dOff+2] = 0
				dst.Pix[dOff+3] = 0
			} else if maskA < 255 {
				dst.Pix[dOff+0] = uint8(uint32(dst.Pix[dOff+0]) * maskA / 255)
				dst.Pix[dOff+1] = uint8(uint32(dst.Pix[dOff+1]) * maskA / 255)
				dst.Pix[dOff+2] = uint8(uint32(dst.Pix[dOff+2]) * maskA / 255)
				dst.Pix[dOff+3] = uint8(uint32(dst.Pix[dOff+3]) * maskA / 255)
			}
		}
	}
}

// compositeLayerWithClip applies the mask corresponding to clipID and blits layerImg onto mainImg.
func compositeLayerWithClip(mainImg, layerImg *image.RGBA, clipID string, clipMasks map[string]*image.RGBA) {
	if mask, ok := clipMasks[clipID]; ok && mask != nil {
		applyAlphaMask(layerImg, mask)
	}
	draw.Draw(mainImg, mainImg.Bounds(), layerImg, image.Point{}, draw.Over)
	// Clear layerImg for reuse
	clear(layerImg.Pix)
}
