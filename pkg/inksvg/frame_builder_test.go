package inksvg

import (
	"bytes"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBuildTimelineFrameSVG_ValidXML verifies that all generated frame SVGs
// pass strict XML decoding without duplicate attributes or namespace mangling.
func TestBuildTimelineFrameSVG_ValidXML(t *testing.T) {
	testFiles := []string{
		"../../testdata/complex_motion.svg",
		"../../testdata/bouncing_walker.svg",
		"../../testdata/multipage_walk.svg",
	}

	for _, relPath := range testFiles {
		t.Run(filepath.Base(relPath), func(t *testing.T) {
			data, err := os.ReadFile(relPath)
			if err != nil {
				t.Fatalf("failed to read test file %s: %v", relPath, err)
			}

			doc, err := ParseSVG(data)
			if err != nil {
				t.Fatalf("ParseSVG failed for %s: %v", relPath, err)
			}

			rect := doc.GetDocumentRect()
			frameCount := len(doc.Frames)
			if frameCount == 0 {
				frameCount = 1
			}

			for i := 0; i < frameCount; i++ {
				frameBytes, err := BuildTimelineFrameSVG(doc, i, rect)
				if err != nil {
					t.Fatalf("BuildTimelineFrameSVG frame %d failed: %v", i, err)
				}

				// 1. Strict XML decoder token stream check
				dec := xml.NewDecoder(bytes.NewReader(frameBytes))
				for {
					tok, err := dec.Token()
					if err != nil {
						if err.Error() == "EOF" {
							break
						}
						t.Fatalf("frame %d failed strict XML decoding: %v\nSVG snippet:\n%s",
							i, err, string(frameBytes[:min(len(frameBytes), 500)]))
					}
					// Ensure no mangled namespaces on any start element
					if se, ok := tok.(xml.StartElement); ok {
						for _, attr := range se.Attr {
							if strings.Contains(attr.Name.Local, "_xmlns") || strings.Contains(attr.Name.Space, "_xmlns") {
								t.Fatalf("frame %d contains mangled _xmlns attribute: %+v", i, attr)
							}
						}
					}
				}

				// 2. Ensure canonical xmlns is present exactly once on root
				s := string(frameBytes)
				if !strings.Contains(s, `xmlns="http://www.w3.org/2000/svg"`) {
					t.Fatalf("frame %d missing canonical xmlns declaration", i)
				}
				if strings.Count(s, `xmlns="http://www.w3.org/2000/svg"`) > 1 {
					t.Fatalf("frame %d contains duplicate xmlns declarations", i)
				}
			}
		})
	}
}

// TestBuildPageFrameSVG_ValidXML verifies that BuildPageFrameSVG emits strictly valid XML.
func TestBuildPageFrameSVG_ValidXML(t *testing.T) {
	data, err := os.ReadFile("../../testdata/multipage_walk.svg")
	if err != nil {
		t.Fatalf("failed to read multipage_walk.svg: %v", err)
	}

	doc, err := ParseSVG(data)
	if err != nil {
		t.Fatalf("ParseSVG failed: %v", err)
	}

	if len(doc.Pages) == 0 {
		t.Skip("no pages found in test document")
	}

	for i, page := range doc.Pages {
		pageBytes, err := BuildPageFrameSVG(doc, page, Rect{X: page.X, Y: page.Y, Width: page.Width, Height: page.Height})
		if err != nil {
			t.Fatalf("BuildPageFrameSVG page %d failed: %v", i, err)
		}

		dec := xml.NewDecoder(bytes.NewReader(pageBytes))
		for {
			_, err := dec.Token()
			if err != nil {
				if err.Error() == "EOF" {
					break
				}
				t.Fatalf("page %d failed strict XML decoding: %v", i, err)
			}
		}
	}
}
