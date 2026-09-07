package svg

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"maps"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"unicode"

	"fyne.io/fyne/v2/theme"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// FontManager caches parsed SFNT fonts to avoid disk and parse overhead.
type FontManager struct {
	mu           sync.RWMutex
	cache        map[string]*sfnt.Font
	systemDirs   []string
	fileScanOnce sync.Once
	fileMap      map[string]string // normalized family/filename -> file path
}

var globalFontManager = newFontManager()

func newFontManager() *FontManager {
	fm := &FontManager{
		cache:   make(map[string]*sfnt.Font),
		fileMap: make(map[string]string),
	}
	fm.initSystemDirs()
	return fm
}

func (fm *FontManager) initSystemDirs() {
	switch runtime.GOOS {
	case "windows":
		windir := os.Getenv("WINDIR")
		if windir == "" {
			windir = "C:\\Windows"
		}
		fm.systemDirs = append(fm.systemDirs, filepath.Join(windir, "Fonts"))
		if localApp := os.Getenv("LOCALAPPDATA"); localApp != "" {
			fm.systemDirs = append(fm.systemDirs, filepath.Join(localApp, "Microsoft", "Windows", "Fonts"))
		}
	case "linux":
		fm.systemDirs = append(fm.systemDirs,
			"/usr/share/fonts",
			"/usr/local/share/fonts",
			filepath.Join(os.Getenv("HOME"), ".fonts"),
			filepath.Join(os.Getenv("HOME"), ".local", "share", "fonts"),
		)
	}
}

func normalizeFontName(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, "_", "")
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "'", "")
	s = strings.ReplaceAll(s, "\"", "")
	return s
}

// scanSystemFonts maps normalized font file names and base names to full paths.
func (fm *FontManager) scanSystemFonts() {
	for _, dir := range fm.systemDirs {
		_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info == nil || info.IsDir() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			if ext == ".ttf" || ext == ".otf" || ext == ".ttc" {
				base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
				norm := normalizeFontName(base)
				if _, exists := fm.fileMap[norm]; !exists {
					fm.fileMap[norm] = path
				}
			}
			return nil
		})
	}
}

// ResolveFont returns an sfnt.Font matching the requested font family and weight/style,
// falling back to Fyne's embedded cross-platform fonts if unavailable.
func (fm *FontManager) ResolveFont(family string, bold, italic bool) *sfnt.Font {
	key := fmt.Sprintf("%s:%t:%t", strings.ToLower(family), bold, italic)

	fm.mu.RLock()
	if f, ok := fm.cache[key]; ok {
		fm.mu.RUnlock()
		return f
	}
	fm.mu.RUnlock()

	fm.mu.Lock()
	defer fm.mu.Unlock()

	// Double check under lock
	if f, ok := fm.cache[key]; ok {
		return f
	}

	// Try finding system font
	f := fm.findSystemFont(family, bold, italic)
	if f == nil {
		// Fallback to embedded Fyne cross-platform fonts
		f = fm.loadEmbeddedFont(bold)
	}

	if f != nil {
		fm.cache[key] = f
	}
	return f
}

func (fm *FontManager) loadEmbeddedFont(bold bool) *sfnt.Font {
	var data []byte
	if bold {
		data = theme.DefaultTextBoldFont().Content()
	} else {
		data = theme.DefaultTextFont().Content()
	}
	if len(data) == 0 {
		return nil
	}
	font, err := sfnt.Parse(data)
	if err != nil {
		return nil
	}
	return font
}

func (fm *FontManager) findSystemFont(family string, bold, italic bool) *sfnt.Font {
	fm.fileScanOnce.Do(func() {
		fm.scanSystemFonts()
	})

	normFam := normalizeFontName(family)
	if normFam == "" {
		return nil
	}

	// Try candidate names
	var candidates []string
	switch {
	case bold && italic:
		candidates = append(candidates, normFam+"bolditalic", normFam+"bi", normFam+"z")
	case bold:
		candidates = append(candidates, normFam+"bold", normFam+"bd", normFam+"b")
	case italic:
		candidates = append(candidates, normFam+"italic", normFam+"i")
	}

	// Special aliases for standard fonts
	switch normFam {
	case "segoeuivariable":
		if bold {
			candidates = append([]string{"segoeuib", "seguivarbd", "seguivarbold"}, candidates...)
		} else {
			candidates = append([]string{"seguivar", "segoeui"}, candidates...)
		}
	case "segoeui":
		if bold {
			candidates = append([]string{"segoeuib"}, candidates...)
		}
	case "arial":
		if bold {
			candidates = append([]string{"arialbd"}, candidates...)
		}
	}

	candidates = append(candidates, normFam)

	for _, cand := range candidates {
		if path, ok := fm.fileMap[cand]; ok {
			if font := fm.loadFontFromFile(path); font != nil {
				return font
			}
		}
	}

	// Prefix search in fileMap
	for normKey, path := range fm.fileMap {
		if strings.HasPrefix(normKey, normFam) {
			if bold && (strings.Contains(normKey, "bold") || strings.Contains(normKey, "bd") || strings.HasSuffix(normKey, "b")) {
				if font := fm.loadFontFromFile(path); font != nil {
					return font
				}
			}
			if !bold && !strings.Contains(normKey, "bold") && !strings.Contains(normKey, "bd") {
				if font := fm.loadFontFromFile(path); font != nil {
					return font
				}
			}
		}
	}

	return nil
}

func (fm *FontManager) loadFontFromFile(path string) *sfnt.Font {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return nil
	}
	// Try parsing as single font
	font, err := sfnt.Parse(data)
	if err == nil {
		return font
	}
	// Try parsing collection (.ttc)
	col, err := sfnt.ParseCollection(data)
	if err == nil && col.NumFonts() > 0 {
		font, err = col.Font(0)
		if err == nil {
			return font
		}
	}
	return nil
}

// TextStyle holds extracted SVG font and text styling attributes.
type TextStyle struct {
	Family        string
	Size          float64
	Bold          bool
	Italic        bool
	LetterSpacing float64
	TextAnchor    string // "start", "middle", "end"
	Fill          string
	Stroke        string
	StrokeWidth   string
	PaintOrder    string
	ExtraStyles   map[string]string
}

// ParseTextStyle extracts text and styling properties from SVG style strings and element attributes.
func ParseTextStyle(styleStr string, attrs []xml.Attr) TextStyle {
	ts := TextStyle{
		Size:        16, // SVG standard default font-size
		TextAnchor:  "start",
		ExtraStyles: make(map[string]string),
	}

	props := make(map[string]string)
	for part := range strings.SplitSeq(styleStr, ";") {
		kv := strings.SplitN(strings.TrimSpace(part), ":", 2)
		if len(kv) == 2 {
			props[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		}
	}

	// Only read presentation attributes from XML attrs (never structural/transform attrs like transform, x, y, id)
	for _, attr := range attrs {
		switch attr.Name.Local {
		case "font-family", "font-size", "font-weight", "font-style", "letter-spacing",
			"text-anchor", "fill", "stroke", "stroke-width", "paint-order",
			"stroke-linecap", "stroke-linejoin", "stroke-dasharray", "stroke-dashoffset",
			"stroke-miterlimit", "stroke-opacity", "fill-opacity", "fill-rule", "opacity", "display", "visibility":
			if _, exists := props[attr.Name.Local]; !exists {
				props[attr.Name.Local] = attr.Value
			}
		}
	}

	for k, v := range props {
		switch k {
		case "font-family":
			ts.Family = strings.Trim(v, "'\"")
			if idx := strings.IndexByte(ts.Family, ','); idx != -1 {
				ts.Family = strings.TrimSpace(ts.Family[:idx])
			}
		case "font-size":
			ts.Size = parseDimension(v)
			if ts.Size <= 0 {
				ts.Size = 16
			}
		case "font-weight":
			w := strings.ToLower(v)
			if w == "bold" || w == "bolder" || w == "700" || w == "800" || w == "900" {
				ts.Bold = true
			}
		case "font-style":
			s := strings.ToLower(v)
			if s == "italic" || s == "oblique" {
				ts.Italic = true
			}
		case "letter-spacing":
			ts.LetterSpacing = parseDimension(v)
		case "text-anchor":
			ts.TextAnchor = strings.ToLower(v)
		case "fill":
			ts.Fill = v
		case "stroke":
			ts.Stroke = v
		case "stroke-width":
			ts.StrokeWidth = v
		case "paint-order":
			ts.PaintOrder = v
		case "stroke-linecap", "stroke-linejoin", "stroke-dasharray", "stroke-dashoffset",
			"stroke-miterlimit", "stroke-opacity", "fill-opacity", "fill-rule", "opacity", "display", "visibility":
			ts.ExtraStyles[k] = v
		}
	}

	return ts
}

// BuildStyleString recreates a sanitized CSS style attribute string for generated path elements.
func (ts TextStyle) BuildStyleString() string {
	var parts []string
	if ts.Fill != "" {
		parts = append(parts, "fill:"+ts.Fill)
	}
	if ts.Stroke != "" {
		parts = append(parts, "stroke:"+ts.Stroke)
	}
	if ts.StrokeWidth != "" {
		parts = append(parts, "stroke-width:"+ts.StrokeWidth)
	}
	if ts.PaintOrder != "" {
		parts = append(parts, "paint-order:"+ts.PaintOrder)
	}
	for k, v := range ts.ExtraStyles {
		parts = append(parts, k+":"+v)
	}
	return strings.Join(parts, ";")
}

// TextSpan represents an in-memory run of text to convert to path vectors.
type TextSpan struct {
	ID     string
	Text   string
	X, Y   float64
	HasX   bool
	HasY   bool
	Styles TextStyle
}

// ConvertTextToPaths transforms SVG <text> and <tspan> elements into <path> vectors in memory.
func ConvertTextToPaths(data []byte) ([]byte, error) {
	if !bytes.Contains(data, []byte("<text")) {
		return data, nil
	}

	decoder := xml.NewDecoder(bytes.NewReader(data))
	var buf bytes.Buffer
	encoder := xml.NewEncoder(&buf)
	transformStack := []Matrix2D{IdentityMatrix()}

	for {
		token, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("xml decode error: %w", err)
		}

		switch elem := token.(type) {
		case xml.StartElement:
			if elem.Name.Local == "text" {
				if err := ProcessTextElementToPaths(elem, decoder, encoder, &transformStack); err != nil {
					return nil, err
				}
				continue
			}
			if elem.Name.Local == "g" {
				curM := IdentityMatrix()
				for _, attr := range elem.Attr {
					if attr.Name.Local == "transform" {
						curM = parseTransform(attr.Value)
						break
					}
				}
				top := transformStack[len(transformStack)-1]
				transformStack = append(transformStack, top.Multiply(curM))
			}
			if err := encoder.EncodeToken(elem); err != nil {
				return nil, err
			}
		case xml.EndElement:
			if elem.Name.Local == "g" && len(transformStack) > 1 {
				transformStack = transformStack[:len(transformStack)-1]
			}
			if err := encoder.EncodeToken(elem); err != nil {
				return nil, err
			}
		default:
			if err := encoder.EncodeToken(elem); err != nil {
				return nil, err
			}
		}
	}

	if err := encoder.Flush(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// ProcessTextElementToPaths parses a <text> element and all its children, replacing them with <g> and <path> elements
// directly into the PreprocessSVG streaming encoder, with group transform scaling and paint-order desugaring applied.
func ProcessTextElementToPaths(textStart xml.StartElement, decoder *xml.Decoder, encoder *xml.Encoder, transformStack *[]Matrix2D) error {
	var styleStr string
	var textID string
	var transformStr string
	var rootX, rootY float64
	var hasRootX, hasRootY bool

	for _, attr := range textStart.Attr {
		switch attr.Name.Local {
		case "id":
			textID = attr.Value
		case "style":
			styleStr = attr.Value
		case "transform":
			transformStr = attr.Value
		case "x":
			rootX = parseDimension(attr.Value)
			hasRootX = true
		case "y":
			rootY = parseDimension(attr.Value)
			hasRootY = true
		}
	}

	rootStyle := ParseTextStyle(styleStr, textStart.Attr)

	// Collect inner spans and direct text
	var spans []TextSpan
	var currentSpan *TextSpan

	depth := 1
	for depth > 0 {
		tok, err := decoder.Token()
		if err != nil {
			return err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "tspan" {
				depth++
				var spanStyleStr, spanID string
				var spanX, spanY float64
				var hasSpanX, hasSpanY bool

				for _, attr := range t.Attr {
					switch attr.Name.Local {
					case "id":
						spanID = attr.Value
					case "style":
						spanStyleStr = attr.Value
					case "x":
						spanX = parseDimension(attr.Value)
						hasSpanX = true
					case "y":
						spanY = parseDimension(attr.Value)
						hasSpanY = true
					}
				}

				// Merge parent rootStyle with tspan overrides
				childStyle := rootStyle
				overrides := ParseTextStyle(spanStyleStr, t.Attr)
				mergeTextStyle(&childStyle, overrides)

				s := TextSpan{
					ID:     spanID,
					X:      spanX,
					Y:      spanY,
					HasX:   hasSpanX,
					HasY:   hasSpanY,
					Styles: childStyle,
				}
				currentSpan = &s
			} else {
				depth++
			}

		case xml.EndElement:
			depth--
			if t.Name.Local == "tspan" && currentSpan != nil {
				if strings.TrimSpace(currentSpan.Text) != "" {
					spans = append(spans, *currentSpan)
				}
				currentSpan = nil
			}

		case xml.CharData:
			textStr := string(t)
			if currentSpan != nil {
				currentSpan.Text += textStr
			} else if strings.TrimSpace(textStr) != "" {
				spans = append(spans, TextSpan{
					ID:     textID + "_span",
					Text:   textStr,
					X:      rootX,
					Y:      rootY,
					HasX:   hasRootX,
					HasY:   hasRootY,
					Styles: rootStyle,
				})
			}
		}
	}

	// Generate path vectors for each span
	type pathWithStyle struct {
		elem       xml.StartElement
		paintOrder string
	}
	var generatedPaths []pathWithStyle

	for i, span := range spans {
		curX := span.X
		if !span.HasX && hasRootX {
			curX = rootX
		}
		curY := span.Y
		if !span.HasY && hasRootY {
			curY = rootY
		}

		font := globalFontManager.ResolveFont(span.Styles.Family, span.Styles.Bold, span.Styles.Italic)
		if font == nil {
			continue
		}

		d := GenerateGlyphPathD(font, span.Text, curX, curY, span.Styles.Size, span.Styles.LetterSpacing, span.Styles.TextAnchor)
		if d == "" {
			continue
		}

		pathID := span.ID
		if pathID == "" {
			pathID = fmt.Sprintf("%s_path_%d", textID, i)
		}

		pathElem := xml.StartElement{
			Name: xml.Name{Local: "path"},
			Attr: []xml.Attr{
				{Name: xml.Name{Local: "id"}, Value: pathID},
				{Name: xml.Name{Local: "d"}, Value: d},
				{Name: xml.Name{Local: "style"}, Value: span.Styles.BuildStyleString()},
			},
		}
		generatedPaths = append(generatedPaths, pathWithStyle{
			elem:       pathElem,
			paintOrder: span.Styles.PaintOrder,
		})
	}

	if len(generatedPaths) == 0 {
		return nil
	}

	// Track group transform if transform is present
	var poppedTransform bool
	if transformStr != "" && transformStack != nil {
		curM := parseTransform(transformStr)
		top := (*transformStack)[len(*transformStack)-1]
		*transformStack = append(*transformStack, top.Multiply(curM))
		poppedTransform = true
	}

	// Wrap in <g id="..." transform="..."> to preserve coordinates, transforms, and structure
	groupElem := xml.StartElement{
		Name: xml.Name{Local: "g"},
		Attr: []xml.Attr{
			{Name: xml.Name{Local: "id"}, Value: textID},
		},
	}
	if transformStr != "" {
		groupElem.Attr = append(groupElem.Attr, xml.Attr{
			Name:  xml.Name{Local: "transform"},
			Value: transformStr,
		})
	}

	if err := encoder.EncodeToken(groupElem); err != nil {
		return err
	}

	// Emit paths, applying ancestor stroke-width scaling and paint-order desugaring
	for _, p := range generatedPaths {
		pathElem := p.elem

		// Stroke width scaling from transformStack
		if transformStack != nil && len(*transformStack) > 0 {
			ancestorScale := (*transformStack)[len(*transformStack)-1].ScaleFactor()
			if ancestorScale > 0 && math.Abs(ancestorScale-1.0) > 0.001 {
				for attrIdx, attr := range pathElem.Attr {
					if attr.Name.Local == "style" {
						swVal := extractCSSProp(attr.Value, "stroke-width")
						if swVal != "" {
							if swNum, unit := parseStrokeWidth(swVal); swNum > 0 {
								newSW := fmt.Sprintf("%.4f%s", swNum*ancestorScale, unit)
								pathElem.Attr[attrIdx].Value = setStyleProp(attr.Value, "stroke-width", newSW)
							}
						}
						break
					}
				}
			}
		}

		// Desugar paint-order if needed
		styleAttrIdx := -1
		for attrIdx, attr := range pathElem.Attr {
			if attr.Name.Local == "style" {
				styleAttrIdx = attrIdx
				break
			}
		}

		if needsPaintOrderDesugar(p.paintOrder, pathElem.Attr, styleAttrIdx) {
			strokeElem, fillElem := desugarPaintOrder(&pathElem, styleAttrIdx)
			if err := encoder.EncodeToken(strokeElem); err != nil {
				return err
			}
			if err := encoder.EncodeToken(xml.EndElement{Name: pathElem.Name}); err != nil {
				return err
			}
			if err := encoder.EncodeToken(fillElem); err != nil {
				return err
			}
			if err := encoder.EncodeToken(xml.EndElement{Name: pathElem.Name}); err != nil {
				return err
			}
		} else {
			if err := encoder.EncodeToken(pathElem); err != nil {
				return err
			}
			if err := encoder.EncodeToken(xml.EndElement{Name: pathElem.Name}); err != nil {
				return err
			}
		}
	}

	if poppedTransform && transformStack != nil && len(*transformStack) > 1 {
		*transformStack = (*transformStack)[:len(*transformStack)-1]
	}

	return encoder.EncodeToken(xml.EndElement{Name: groupElem.Name})
}

func mergeTextStyle(base *TextStyle, overrides TextStyle) {
	if overrides.Family != "" {
		base.Family = overrides.Family
	}
	if overrides.Size > 0 && overrides.Size != 16 {
		base.Size = overrides.Size
	}
	if overrides.Bold {
		base.Bold = true
	}
	if overrides.Italic {
		base.Italic = true
	}
	if overrides.LetterSpacing != 0 {
		base.LetterSpacing = overrides.LetterSpacing
	}
	if overrides.TextAnchor != "start" && overrides.TextAnchor != "" {
		base.TextAnchor = overrides.TextAnchor
	}
	if overrides.Fill != "" {
		base.Fill = overrides.Fill
	}
	if overrides.Stroke != "" {
		base.Stroke = overrides.Stroke
	}
	if overrides.StrokeWidth != "" {
		base.StrokeWidth = overrides.StrokeWidth
	}
	if overrides.PaintOrder != "" {
		base.PaintOrder = overrides.PaintOrder
	}
	maps.Copy(base.ExtraStyles, overrides.ExtraStyles)
}

// GenerateGlyphPathD converts a text string into an SVG path d string using sfnt vector glyph contours.
func GenerateGlyphPathD(f *sfnt.Font, text string, startX, baselineY, fontSize, letterSpacing float64, anchor string) string {
	if len(text) == 0 || fontSize <= 0 {
		return ""
	}

	var b sfnt.Buffer
	ppem := fixed.Int26_6(math.Round(fontSize * 64.0))

	// Collect glyph indices
	runes := []rune(text)
	indices := make([]sfnt.GlyphIndex, len(runes))
	for i, r := range runes {
		idx, err := f.GlyphIndex(&b, r)
		if err != nil {
			idx = 0
		}
		indices[i] = idx
	}

	// Compute advances between characters
	advances := make([]float64, len(runes))
	var totalAdvance float64
	for i := range runes {
		adv, err := f.GlyphAdvance(&b, indices[i], ppem, 0)
		advPx := float64(adv) / 64.0
		if err != nil {
			advPx = fontSize * 0.5
		}

		if i < len(runes)-1 {
			kern, err := f.Kern(&b, indices[i], indices[i+1], ppem, 0)
			if err == nil {
				advPx += float64(kern) / 64.0
			}
			advPx += letterSpacing
		}
		advances[i] = advPx
		totalAdvance += advPx
	}

	// Adjust for text-anchor
	anchorOffset := 0.0
	switch anchor {
	case "middle":
		anchorOffset = -totalAdvance / 2.0
	case "end":
		anchorOffset = -totalAdvance
	}

	var d strings.Builder
	cursorX := startX + anchorOffset

	for i, idx := range indices {
		if idx == 0 && unicode.IsSpace(runes[i]) {
			cursorX += advances[i]
			continue
		}

		segs, err := f.LoadGlyph(&b, idx, ppem, nil)
		if err == nil && len(segs) > 0 {
			subPathOpen := false
			for _, seg := range segs {
				switch seg.Op {
				case sfnt.SegmentOpMoveTo:
					if subPathOpen {
						d.WriteString(" Z")
					}
					p0X := cursorX + float64(seg.Args[0].X)/64.0
					p0Y := baselineY + float64(seg.Args[0].Y)/64.0
					fmt.Fprintf(&d, " M %.2f %.2f", p0X, p0Y)
					subPathOpen = true

				case sfnt.SegmentOpLineTo:
					p0X := cursorX + float64(seg.Args[0].X)/64.0
					p0Y := baselineY + float64(seg.Args[0].Y)/64.0
					fmt.Fprintf(&d, " L %.2f %.2f", p0X, p0Y)

				case sfnt.SegmentOpQuadTo:
					cx := cursorX + float64(seg.Args[0].X)/64.0
					cy := baselineY + float64(seg.Args[0].Y)/64.0
					ex := cursorX + float64(seg.Args[1].X)/64.0
					ey := baselineY + float64(seg.Args[1].Y)/64.0
					fmt.Fprintf(&d, " Q %.2f %.2f %.2f %.2f", cx, cy, ex, ey)

				case sfnt.SegmentOpCubeTo:
					c1x := cursorX + float64(seg.Args[0].X)/64.0
					c1y := baselineY + float64(seg.Args[0].Y)/64.0
					c2x := cursorX + float64(seg.Args[1].X)/64.0
					c2y := baselineY + float64(seg.Args[1].Y)/64.0
					ex := cursorX + float64(seg.Args[2].X)/64.0
					ey := baselineY + float64(seg.Args[2].Y)/64.0
					fmt.Fprintf(&d, " C %.2f %.2f %.2f %.2f %.2f %.2f", c1x, c1y, c2x, c2y, ex, ey)
				}
			}
			if subPathOpen {
				d.WriteString(" Z")
			}
		}

		cursorX += advances[i]
	}

	return strings.TrimSpace(d.String())
}
