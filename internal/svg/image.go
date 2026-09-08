package svg

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"strconv"
	"strings"
	"unicode"

	"github.com/srwiley/rasterx"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

// decodeDataURI decodes a base64 Data URI (e.g. data:image/png;base64,...) into an image.Image.
func decodeDataURI(uri string) (image.Image, error) {
	uri = strings.TrimSpace(uri)
	if !strings.HasPrefix(uri, "data:") {
		return nil, errors.New("not a data URI")
	}
	commaIdx := strings.IndexByte(uri, ',')
	if commaIdx == -1 {
		return nil, errors.New("invalid data URI: missing comma")
	}
	header := uri[:commaIdx]
	payload := uri[commaIdx+1:]

	if !strings.Contains(header, ";base64") {
		return nil, errors.New("unsupported encoding: expected base64")
	}

	// SVG base64 strings frequently contain formatting newlines and whitespace
	payload = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, payload)

	data, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return nil, fmt.Errorf("base64 decode error: %w", err)
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("image decode error: %w", err)
	}
	return img, nil
}

// extractEmbeddedImages parses an SVG document and extracts embedded <image> elements with their
// user-space bounds, transforms, opacity, and path z-index.
func extractEmbeddedImages(data []byte) []EmbeddedImage {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	transformStack := []Matrix2D{IdentityMatrix()}
	var images []EmbeddedImage
	var pathCount int
	var inDefs int

	for {
		token, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			break
		}

		switch tok := token.(type) {
		case xml.StartElement:
			name := tok.Name.Local

			if name == "defs" {
				inDefs++
			}

			if name == "g" {
				curMatrix := transformStack[len(transformStack)-1]
				for _, attr := range tok.Attr {
					if attr.Name.Local == "transform" {
						parsed := parseTransform(attr.Value)
						curMatrix = curMatrix.Multiply(parsed)
					}
				}
				transformStack = append(transformStack, curMatrix)
			}

			if inDefs == 0 && isShapeElement(name) {
				pathCount++
			}

			if inDefs == 0 && name == "image" {
				var href, xStr, yStr, wStr, hStr, transformStr string
				opacity := 1.0

				for _, a := range tok.Attr {
					switch a.Name.Local {
					case "href":
						href = a.Value
					case "x":
						xStr = a.Value
					case "y":
						yStr = a.Value
					case "width":
						wStr = a.Value
					case "height":
						hStr = a.Value
					case "transform":
						transformStr = a.Value
					case "opacity":
						if v, err := strconv.ParseFloat(a.Value, 64); err == nil {
							opacity = v
						}
					case "style":
						if opStr := extractCSSProp(a.Value, "opacity"); opStr != "" {
							if v, err := strconv.ParseFloat(opStr, 64); err == nil {
								opacity = v
							}
						}
					}
				}

				if strings.HasPrefix(href, "data:") {
					decoded, err := decodeDataURI(href)
					if err == nil {
						curM := transformStack[len(transformStack)-1]
						if transformStr != "" {
							curM = curM.Multiply(parseTransform(transformStr))
						}

						wVal := parseDimension(wStr)
						hVal := parseDimension(hStr)
						if wVal <= 0 && decoded.Bounds().Dx() > 0 {
							wVal = float64(decoded.Bounds().Dx())
						}
						if hVal <= 0 && decoded.Bounds().Dy() > 0 {
							hVal = float64(decoded.Bounds().Dy())
						}

						images = append(images, EmbeddedImage{
							Data:      decoded,
							X:         parseDimension(xStr),
							Y:         parseDimension(yStr),
							Width:     wVal,
							Height:    hVal,
							Transform: curM,
							Opacity:   opacity,
							PathIndex: pathCount,
						})
					}
				}
			}

		case xml.EndElement:
			if tok.Name.Local == "defs" && inDefs > 0 {
				inDefs--
			}
			if tok.Name.Local == "g" && len(transformStack) > 1 {
				transformStack = transformStack[:len(transformStack)-1]
			}
		}
	}

	return images
}

// drawEmbeddedImage renders an embedded raster image onto the destination canvas.
func drawEmbeddedImage(dest *image.RGBA, emb EmbeddedImage, viewTransform rasterx.Matrix2D) {
	if emb.Data == nil || emb.Width <= 0 || emb.Height <= 0 || emb.Opacity <= 0 {
		return
	}

	scaleW := viewTransform.A
	scaleH := viewTransform.D
	tx := viewTransform.E
	ty := viewTransform.F

	// Map user-space image bounds through emb.Transform and viewTransform
	m := emb.Transform
	x0 := m.A*emb.X + m.C*emb.Y + m.E
	y0 := m.B*emb.X + m.D*emb.Y + m.F
	x1 := m.A*(emb.X+emb.Width) + m.C*(emb.Y+emb.Height) + m.E
	y1 := m.B*(emb.X+emb.Width) + m.D*(emb.Y+emb.Height) + m.F

	cx0 := x0*scaleW + tx
	cy0 := y0*scaleH + ty
	cx1 := x1*scaleW + tx
	cy1 := y1*scaleH + ty

	minX := int(math.Round(math.Min(cx0, cx1)))
	minY := int(math.Round(math.Min(cy0, cy1)))
	maxX := int(math.Round(math.Max(cx0, cx1)))
	maxY := int(math.Round(math.Max(cy0, cy1)))

	dstRect := image.Rect(minX, minY, maxX, maxY)
	if dstRect.Empty() {
		return
	}

	if emb.Opacity >= 0.999 {
		draw.BiLinear.Scale(dest, dstRect, emb.Data, emb.Data.Bounds(), draw.Over, nil)
		return
	}

	// For partial opacity, scale into a buffer and apply alpha factor
	scaled := image.NewRGBA(image.Rect(0, 0, dstRect.Dx(), dstRect.Dy()))
	draw.BiLinear.Scale(scaled, scaled.Bounds(), emb.Data, emb.Data.Bounds(), draw.Src, nil)
	for i := 3; i < len(scaled.Pix); i += 4 {
		scaled.Pix[i] = uint8(float64(scaled.Pix[i]) * emb.Opacity)
	}
	draw.Draw(dest, dstRect, scaled, image.Point{}, draw.Over)
}
