package svg

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// BuildLayerFrameSVG generates an SVG where only the target layer and any pinned layers are visible.
func BuildLayerFrameSVG(doc *SVGDocument, targetLayerID string, pinnedLayerIDs map[string]bool) ([]byte, error) {
	decoder := xml.NewDecoder(bytes.NewReader(doc.RawContent))
	var buf bytes.Buffer
	encoder := xml.NewEncoder(&buf)
	skipDepth := 0

	for {
		token, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("xml transform error: %w", err)
		}

		switch elem := token.(type) {
		case xml.StartElement:
			if skipDepth > 0 {
				skipDepth++
				continue
			}

			if elem.Name.Local == "g" {
				var isLayer bool
				var layerID string

				for _, attr := range elem.Attr {
					if attr.Name.Local == "groupmode" && attr.Value == "layer" {
						isLayer = true
					}
					if attr.Name.Local == "id" {
						layerID = attr.Value
					}
				}

				if isLayer {
					shouldShow := (layerID == targetLayerID) || (pinnedLayerIDs != nil && pinnedLayerIDs[layerID])
					if !shouldShow {
						// Skip this layer subtree entirely so oksvg doesn't render hidden elements
						skipDepth = 1
						continue
					}
				}
			}

			if err := encoder.EncodeToken(elem); err != nil {
				return nil, err
			}

		case xml.EndElement:
			if skipDepth > 0 {
				skipDepth--
				continue
			}
			if err := encoder.EncodeToken(elem); err != nil {
				return nil, err
			}

		default:
			if skipDepth > 0 {
				continue
			}
			if err := encoder.EncodeToken(token); err != nil {
				return nil, err
			}
		}
	}

	if err := encoder.Flush(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// BuildPageFrameSVG generates an SVG where the viewBox and dimensions correspond to the given page.
func BuildPageFrameSVG(doc *SVGDocument, page Page) ([]byte, error) {
	decoder := xml.NewDecoder(bytes.NewReader(doc.RawContent))
	var buf bytes.Buffer
	encoder := xml.NewEncoder(&buf)
	var processedRoot bool

	newViewBox := fmt.Sprintf("%f %f %f %f", page.X, page.Y, page.Width, page.Height)
	newW := fmt.Sprintf("%f", page.Width)
	newH := fmt.Sprintf("%f", page.Height)

	for {
		token, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("xml transform error: %w", err)
		}

		switch elem := token.(type) {
		case xml.StartElement:
			if elem.Name.Local == "svg" && !processedRoot {
				processedRoot = true
				var vbFound, wFound, hFound bool
				for i := range elem.Attr {
					switch elem.Attr[i].Name.Local {
					case "viewBox":
						elem.Attr[i].Value = newViewBox
						vbFound = true
					case "width":
						elem.Attr[i].Value = newW
						wFound = true
					case "height":
						elem.Attr[i].Value = newH
						hFound = true
					}
				}
				if !vbFound {
					elem.Attr = append(elem.Attr, xml.Attr{Name: xml.Name{Local: "viewBox"}, Value: newViewBox})
				}
				if !wFound {
					elem.Attr = append(elem.Attr, xml.Attr{Name: xml.Name{Local: "width"}, Value: newW})
				}
				if !hFound {
					elem.Attr = append(elem.Attr, xml.Attr{Name: xml.Name{Local: "height"}, Value: newH})
				}
			}
			if err := encoder.EncodeToken(elem); err != nil {
				return nil, err
			}
		default:
			if err := encoder.EncodeToken(token); err != nil {
				return nil, err
			}
		}
	}

	if err := encoder.Flush(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// removeStyleProp removes a CSS property from a style string, e.g. "display:none;opacity:1" -> "opacity:1"
func removeStyleProp(style, prop string) string {
	parts := strings.Split(style, ";")
	var kept []string
	prefix := prop + ":"
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" || strings.HasPrefix(trimmed, prefix) {
			continue
		}
		kept = append(kept, trimmed)
	}
	return strings.Join(kept, ";")
}
