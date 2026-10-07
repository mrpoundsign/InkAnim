package svgpatch

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"strings"
)

const InkscapeNS = "http://www.inkscape.org/namespaces/inkscape"

// escapeAttrValue escapes special XML characters for an attribute value.
func escapeAttrValue(s string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(s))
	// xml.EscapeText handles &, <, >, \r, \t, etc. We must also escape double-quotes for attributes.
	return strings.ReplaceAll(buf.String(), `"`, "&quot;")
}

// detectNamespacePrefix scans the document for an xmlns:prefix="namespaceURI" definition.
// If not found, it returns the fallback.
func detectNamespacePrefix(data []byte, space, fallback string) string {
	if space == "" {
		return ""
	}
	re := regexp.MustCompile(`xmlns:([a-zA-Z0-9_\.\-]+)\s*=\s*["']` + regexp.QuoteMeta(space) + `["']`)
	match := re.FindSubmatch(data)
	if len(match) > 1 {
		return string(match[1])
	}
	return fallback
}

// GetAttr returns the value of an attribute by local name on an element identified by ID.
func GetAttr(data []byte, elementID, attrLocal string) (string, bool) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := dec.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", false
		}

		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}

		var hasID bool
		for _, attr := range se.Attr {
			if attr.Name.Local == "id" && attr.Value == elementID {
				hasID = true
				break
			}
		}

		if hasID {
			for _, attr := range se.Attr {
				if attr.Name.Local == attrLocal {
					return attr.Value, true
				}
			}
			return "", false
		}
	}
	return "", false
}

// SetAttr sets or updates an attribute on an element identified by ID without re-serializing
// untouched portions of the document.
func SetAttr(data []byte, elementID, attrSpace, attrLocal, value string) ([]byte, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	var prevOffset int64

	for {
		tok, err := dec.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return data, fmt.Errorf("xml decoding error: %w", err)
		}

		currOffset := dec.InputOffset()

		se, ok := tok.(xml.StartElement)
		if ok {
			var hasID bool
			for _, attr := range se.Attr {
				if attr.Name.Local == "id" && attr.Value == elementID {
					hasID = true
					break
				}
			}

			if hasID {
				// Locate tag start '<' in data[prevOffset:currOffset]
				segment := data[prevOffset:currOffset]
				relStart := bytes.IndexByte(segment, '<')
				if relStart == -1 {
					return data, fmt.Errorf("failed to locate start of tag for element %q", elementID)
				}
				tagStart := int(prevOffset) + relStart
				tagEnd := int(currOffset)
				tagBytes := data[tagStart:tagEnd]

				escapedVal := escapeAttrValue(value)

				// Look for existing attribute matching attrLocal
				attrRe := regexp.MustCompile(`(?s)([\s/])((?:[a-zA-Z0-9_\.\-]+:)?` + regexp.QuoteMeta(attrLocal) + `)\s*=\s*(?:"([^"]*)"|'([^']*)')`)
				matchLoc := attrRe.FindSubmatchIndex(tagBytes)

				if matchLoc != nil {
					// Existing attribute found.
					// If group 3 matched (double quote), matchLoc[6] and matchLoc[7] are its bounds.
					// If group 4 matched (single quote), matchLoc[8] and matchLoc[9] are its bounds.
					var valStart, valEnd int
					if matchLoc[6] != -1 {
						valStart = tagStart + matchLoc[6]
						valEnd = tagStart + matchLoc[7]
					} else {
						valStart = tagStart + matchLoc[8]
						valEnd = tagStart + matchLoc[9]
					}

					res := make([]byte, 0, len(data)-valEnd+valStart+len(escapedVal))
					res = append(res, data[:valStart]...)
					res = append(res, []byte(escapedVal)...)
					res = append(res, data[valEnd:]...)
					return res, nil
				}

				// Attribute does not exist: insert it.
				prefix := ""
				if attrSpace != "" {
					prefix = detectNamespacePrefix(data, attrSpace, "inkscape")
				}
				var attrName string
				if prefix != "" {
					attrName = prefix + ":" + attrLocal
				} else {
					attrName = attrLocal
				}
				attrToInsert := fmt.Sprintf(` %s="%s"`, attrName, escapedVal)

				// Determine insertion point before closing '>' or '/>'
				insertPos := tagEnd - 1
				if bytes.HasSuffix(bytes.TrimSpace(tagBytes), []byte("/>")) {
					// Find the '/'
					slashIdx := bytes.LastIndexByte(tagBytes, '/')
					if slashIdx != -1 {
						insertPos = tagStart + slashIdx
					}
				}

				res := make([]byte, 0, len(data)+len(attrToInsert))
				res = append(res, data[:insertPos]...)
				res = append(res, []byte(attrToInsert)...)
				res = append(res, data[insertPos:]...)
				return res, nil
			}
		}

		prevOffset = currOffset
	}

	return data, fmt.Errorf("element %q not found", elementID)
}

// InsertChild inserts childXML into the parent element identified by parentID
// immediately before the parent's closing tag without re-serializing untouched portions of the document.
func InsertChild(data []byte, parentID string, childXML string) ([]byte, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	var prevOffset int64
	var targetDepth int
	var insideTarget bool
	var parentTag string

	for {
		tok, err := dec.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return data, fmt.Errorf("xml decoding error: %w", err)
		}

		currOffset := dec.InputOffset()

		switch elem := tok.(type) {
		case xml.StartElement:
			if !insideTarget {
				for _, attr := range elem.Attr {
					if attr.Name.Local == "id" && attr.Value == parentID {
						insideTarget = true
						targetDepth = 1
						parentTag = elem.Name.Local

						// Check if the element is self-closing (<tag ... />)
						segment := data[prevOffset:currOffset]
						relStart := bytes.IndexByte(segment, '<')
						if relStart != -1 {
							tagBytes := segment[relStart:]
							if bytes.HasSuffix(bytes.TrimSpace(tagBytes), []byte("/>")) {
								slashIdx := bytes.LastIndexByte(tagBytes, '/')
								replaceStart := int(prevOffset) + relStart + slashIdx
								replaceEnd := int(currOffset)
								trimmedChild := strings.TrimSpace(childXML)
								replacement := fmt.Sprintf(">\n    %s\n  </%s>", trimmedChild, parentTag)

								res := make([]byte, 0, len(data)+len(replacement)-(replaceEnd-replaceStart))
								res = append(res, data[:replaceStart]...)
								res = append(res, []byte(replacement)...)
								res = append(res, data[replaceEnd:]...)
								return res, nil
							}
						}
						break
					}
				}
			} else {
				targetDepth++
			}

		case xml.EndElement:
			if insideTarget {
				targetDepth--
				if targetDepth == 0 {
					// We reached the closing tag of parentID, e.g. </g>
					segment := data[prevOffset:currOffset]
					relClose := bytes.LastIndex(segment, []byte("</"))
					if relClose == -1 {
						return data, fmt.Errorf("failed to locate closing tag for parent %q", parentID)
					}
					closeTagStart := int(prevOffset) + relClose

					// Detect existing indent of closing tag
					lineStart := bytes.LastIndexByte(data[:closeTagStart], '\n')
					indent := "  "
					if lineStart != -1 {
						indent = string(data[lineStart+1 : closeTagStart])
					}

					trimmedChild := strings.TrimSpace(childXML)
					var toInsert string
					// If preceding characters before closeTagStart are whitespace after a newline,
					// replace or insert cleanly:
					if lineStart != -1 && strings.TrimSpace(string(data[lineStart+1:closeTagStart])) == "" {
						toInsert = fmt.Sprintf("  %s\n%s", trimmedChild, indent)
					} else {
						toInsert = fmt.Sprintf("\n  %s\n%s", trimmedChild, indent)
					}

					res := make([]byte, 0, len(data)+len(toInsert))
					res = append(res, data[:closeTagStart]...)
					res = append(res, []byte(toInsert)...)
					res = append(res, data[closeTagStart:]...)
					return res, nil
				}
			}
		}

		prevOffset = currOffset
	}

	return data, fmt.Errorf("parent element %q not found", parentID)
}

