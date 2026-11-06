package inksvg

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// LinkedSource represents a linked SVG document to be composed into a stage scene.
type LinkedSource struct {
	ID     string // Unique source identifier (e.g. "src_1")
	Name   string // Display name or filename (e.g. "Character.svg")
	RawSVG []byte // Raw SVG file bytes
}

// GroupMetadata describes a <g> layer or group discovered in an SVG document.
type GroupMetadata struct {
	XMLID      string // Actual XML id attribute in the SVG (or composite SVG)
	SourceID   string // Source ID if inside a multi-source composite SVG (e.g. "src_1")
	CleanLabel string // Group label with any IAMS directives stripped
	RawLabel   string // Original inkscape:label attribute
	LabelPath  string // Slash-separated clean label hierarchy (e.g. "Hero/LeftArm")
	ObjectKey  string // Canonical .inka object key (e.g. "src_1/Hero/LeftArm" or "Hero/LeftArm")
	IsLayer    bool   // True if inkscape:groupmode="layer"
	ParentID   string // Parent <g> XML ID
}

var (
	urlRefRe       = regexp.MustCompile(`url\(\s*['"]?#([^)'"\s]+)['"]?\s*\)`)
	directiveCutRe = regexp.MustCompile(`(?i)\b(move|rot|scale|scal|fade|show|hide|depth|camera|distance|dist|color)\s*\{[^}]*\}`)
)

// CleanGroupLabel strips IAMS motion directives and separator pipes from an inkscape:label,
// returning the human-readable group or layer name.
func CleanGroupLabel(label string) string {
	if label == "" {
		return ""
	}
	migrated := migrateLabel(label)
	cleaned := directiveCutRe.ReplaceAllString(migrated, "")
	parts := strings.Split(cleaned, "|")
	var kept []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		trimmed = strings.Trim(trimmed, "-;")
		trimmed = strings.TrimSpace(trimmed)
		if trimmed != "" {
			kept = append(kept, trimmed)
		}
	}
	return strings.Join(kept, " / ")
}

// StripIAMSFromSVG removes IAMS motion directives from <g> labels and removes helper
// motion path shapes (shapes inside groups whose label defines IAMS motion directives)
// from the SVG byte stream, leaving pure vector artwork with clean group/layer labels.
func StripIAMSFromSVG(svgData []byte) ([]byte, error) {
	if len(svgData) == 0 {
		return svgData, nil
	}

	decoder := xml.NewDecoder(bytes.NewReader(svgData))
	var buf bytes.Buffer
	encoder := xml.NewEncoder(&buf)

	var gDepth int
	for {
		tok, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("xml decode error in StripIAMSFromSVG: %w", err)
		}

		switch elem := tok.(type) {
		case xml.StartElement:
			name := elem.Name.Local
			if name == "g" {
				gDepth++
				for i, attr := range elem.Attr {
					if attr.Name.Local == "label" {
						if len(parseMotionConfigs(attr.Value)) > 0 {
							clean := CleanGroupLabel(attr.Value)
							elem.Attr[i].Value = clean
						}
					}
				}
				if err := encoder.EncodeToken(elem); err != nil {
					return nil, err
				}
				continue
			}

			if gDepth > 0 && isMotionShapeElement(name) {
				var label string
				for _, attr := range elem.Attr {
					if attr.Name.Local == "label" {
						label = attr.Value
						break
					}
				}
				if len(parseMotionConfigs(label)) > 0 {
					if err := decoder.Skip(); err != nil {
						return nil, err
					}
					continue
				}
			}

			if err := encoder.EncodeToken(elem); err != nil {
				return nil, err
			}

		case xml.EndElement:
			if elem.Name.Local == "g" && gDepth > 0 {
				gDepth--
			}
			if err := encoder.EncodeToken(elem); err != nil {
				return nil, err
			}

		default:
			if err := encoder.EncodeToken(tok); err != nil {
				return nil, err
			}
		}
	}

	if err := encoder.Flush(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func isMotionShapeElement(localName string) bool {
	switch localName {
	case "path", "circle", "rect", "ellipse", "line", "polygon", "polyline":
		return true
	default:
		return false
	}
}

// ExtractGroupHierarchy scans an SVG document and returns metadata for all <g> groups and layers,
// along with a lookup table mapping object keys ("src_1/Hero/LeftArm", "Hero/LeftArm", or XML ID)
// to the target <g>'s XML ID in the document.
func ExtractGroupHierarchy(svgData []byte) ([]GroupMetadata, map[string]string) {
	var groups []GroupMetadata
	lookup := make(map[string]string)
	if len(svgData) == 0 {
		return groups, lookup
	}

	type stackEntry struct {
		xmlID      string
		cleanLabel string
		isSource   bool
		sourceID   string
	}

	decoder := xml.NewDecoder(bytes.NewReader(svgData))
	var stack []stackEntry

	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}

		switch elem := tok.(type) {
		case xml.StartElement:
			if elem.Name.Local != "g" {
				continue
			}
			var id, rawLabel, groupMode, sourceIDAttr string
			for _, attr := range elem.Attr {
				switch attr.Name.Local {
				case "id":
					id = attr.Value
				case "label":
					rawLabel = attr.Value
				case "groupmode":
					groupMode = attr.Value
				case "data-inka-source":
					sourceIDAttr = attr.Value
				}
			}

			cleanLabel := CleanGroupLabel(rawLabel)
			if cleanLabel == "" {
				cleanLabel = id
			}

			isSourceRoot := sourceIDAttr != ""
			activeSourceID := sourceIDAttr
			if activeSourceID == "" && len(stack) > 0 {
				activeSourceID = stack[len(stack)-1].sourceID
			}

			var parentID string
			if len(stack) > 0 {
				parentID = stack[len(stack)-1].xmlID
			}

			stack = append(stack, stackEntry{
				xmlID:      id,
				cleanLabel: cleanLabel,
				isSource:   isSourceRoot,
				sourceID:   activeSourceID,
			})

			var pathParts []string
			for _, s := range stack {
				if s.isSource {
					continue
				}
				if s.cleanLabel != "" {
					pathParts = append(pathParts, s.cleanLabel)
				}
			}
			labelPath := strings.Join(pathParts, "/")
			objectKey := labelPath
			if activeSourceID != "" {
				if labelPath != "" {
					objectKey = activeSourceID + "/" + labelPath
				} else {
					objectKey = activeSourceID
				}
			}

			meta := GroupMetadata{
				XMLID:      id,
				SourceID:   activeSourceID,
				CleanLabel: cleanLabel,
				RawLabel:   rawLabel,
				LabelPath:  labelPath,
				ObjectKey:  objectKey,
				IsLayer:    groupMode == "layer",
				ParentID:   parentID,
			}
			groups = append(groups, meta)

			if id != "" {
				lookup[id] = id
				if objectKey != "" {
					if _, exists := lookup[objectKey]; !exists {
						lookup[objectKey] = id
					}
				}
				if labelPath != "" {
					if _, exists := lookup[labelPath]; !exists {
						lookup[labelPath] = id
					}
				}
				if cleanLabel != "" {
					if _, exists := lookup[cleanLabel]; !exists {
						lookup[cleanLabel] = id
					}
					if activeSourceID != "" {
						shortKey := activeSourceID + "/" + cleanLabel
						if _, exists := lookup[shortKey]; !exists {
							lookup[shortKey] = id
						}
					}
				}
			}

		case xml.EndElement:
			if elem.Name.Local == "g" && len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}

	return groups, lookup
}

// ComposeLinkedSVGs merges one or more linked SVG sources into a single composite stage SVG.
// Each source's visual elements are wrapped in a top-level parent <g id="<source.ID>" inkscape:label="<source.Name>" data-inka-source="<source.ID">
// so the file itself acts as a manipulable parent group. Internal XML IDs and url(#...) / href="#..." references
// in secondary sources are namespace-prefixed with "<source.ID>__" to prevent ID and gradient collisions.
func ComposeLinkedSVGs(sources []LinkedSource) ([]byte, error) {
	if len(sources) == 0 {
		return nil, errors.New("no linked SVG sources provided")
	}

	// Preprocess all sources first
	preprocessed := make([][]byte, len(sources))
	for i, src := range sources {
		if len(src.RawSVG) == 0 {
			return nil, fmt.Errorf("linked SVG source %q is empty", src.Name)
		}
		p, err := PreprocessSVG(src.RawSVG)
		if err != nil {
			p = src.RawSVG
		}
		preprocessed[i] = p
	}

	// Parse first source to extract root <svg> attributes and namedview/pages
	firstTpl, err := parseTimelineTemplate(preprocessed[0])
	if err != nil {
		return nil, fmt.Errorf("failed to parse primary linked SVG %q: %w", sources[0].Name, err)
	}

	var buf bytes.Buffer
	enc := xml.NewEncoder(&buf)

	rootStart := xml.StartElement{
		Name: xml.Name{Local: "svg"},
		Attr: []xml.Attr{
			{Name: xml.Name{Local: "xmlns"}, Value: "http://www.w3.org/2000/svg"},
			{Name: xml.Name{Local: "xmlns:xlink"}, Value: "http://www.w3.org/1999/xlink"},
			{Name: xml.Name{Local: "xmlns:inkscape"}, Value: "http://www.inkscape.org/namespaces/inkscape"},
			{Name: xml.Name{Local: "xmlns:sodipodi"}, Value: "http://sodipodi.sourceforge.net/DTD/sodipodi-0.dtd"},
		},
	}
	for _, a := range firstTpl.rootElem.Attr {
		if a.Name.Local == "xmlns" || a.Name.Space == "xmlns" || strings.HasPrefix(a.Name.Local, "xmlns:") {
			continue
		}
		rootStart.Attr = append(rootStart.Attr, xml.Attr{Name: xml.Name{Local: a.Name.Local}, Value: a.Value})
	}

	if err := enc.EncodeToken(rootStart); err != nil {
		return nil, err
	}

	// Collect all defs/namedview/style nodes across sources first, then wrap each source's visual nodes in a parent <g>
	type extractedSource struct {
		src        LinkedSource
		prefix     string
		defsNodes  [][]xml.Token
		bodyTokens []xml.Token
	}

	extracted := make([]extractedSource, len(sources))
	for i, src := range sources {
		srcID := src.ID
		if srcID == "" {
			srcID = fmt.Sprintf("src_%d", i+1)
		}
		name := src.Name
		if name == "" {
			name = srcID
		}
		prefix := ""
		if len(sources) > 1 {
			prefix = srcID + "__"
		}

		defs, body, extractErr := splitAndPrefixSourceTokens(preprocessed[i], prefix, i == 0)
		if extractErr != nil {
			return nil, fmt.Errorf("failed to process linked SVG %q: %w", name, extractErr)
		}
		extracted[i] = extractedSource{
			src:        LinkedSource{ID: srcID, Name: name, RawSVG: src.RawSVG},
			prefix:     prefix,
			defsNodes:  defs,
			bodyTokens: body,
		}
	}

	for _, ex := range extracted {
		for _, defTokens := range ex.defsNodes {
			for _, tok := range defTokens {
				if err := enc.EncodeToken(tok); err != nil {
					return nil, err
				}
			}
		}
	}

	for _, ex := range extracted {
		wrapperStart := xml.StartElement{
			Name: xml.Name{Local: "g"},
			Attr: []xml.Attr{
				{Name: xml.Name{Local: "id"}, Value: ex.src.ID},
				{Name: xml.Name{Local: "inkscape:label"}, Value: ex.src.Name},
				{Name: xml.Name{Local: "data-inka-source"}, Value: ex.src.ID},
			},
		}
		if err := enc.EncodeToken(wrapperStart); err != nil {
			return nil, err
		}
		for _, tok := range ex.bodyTokens {
			if err := enc.EncodeToken(tok); err != nil {
				return nil, err
			}
		}
		if err := enc.EncodeToken(wrapperStart.End()); err != nil {
			return nil, err
		}
	}

	if err := enc.EncodeToken(rootStart.End()); err != nil {
		return nil, err
	}
	if err := enc.Flush(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func splitAndPrefixSourceTokens(svgData []byte, idPrefix string, keepNamedView bool) ([][]xml.Token, []xml.Token, error) {
	tpl, err := parseTimelineTemplate(svgData)
	if err != nil {
		return nil, nil, err
	}

	var defsNodes [][]xml.Token
	var bodyTokens []xml.Token

	for _, node := range tpl.nodes {
		if len(node.tokens) == 0 {
			continue
		}
		startElem, ok := node.tokens[0].(xml.StartElement)
		if !ok {
			continue
		}
		isNamedView := startElem.Name.Local == "namedview" || startElem.Name.Space == "http://sodipodi.sourceforge.net/DTD/sodipodi-0.dtd"
		if isNamedView && !keepNamedView {
			continue
		}

		rewritten := rewriteTokensWithPrefix(node.tokens, idPrefix)
		if node.isDefs {
			defsNodes = append(defsNodes, rewritten)
		} else {
			bodyTokens = append(bodyTokens, rewritten...)
		}
	}

	return defsNodes, bodyTokens, nil
}

func rewriteTokensWithPrefix(tokens []xml.Token, prefix string) []xml.Token {
	out := make([]xml.Token, 0, len(tokens))
	for _, tok := range tokens {
		switch elem := tok.(type) {
		case xml.StartElement:
			normElem := normalizeStartElementForComposite(elem, prefix)
			out = append(out, normElem)
		case xml.EndElement:
			out = append(out, normalizeEndElementForComposite(elem))
		default:
			out = append(out, xml.CopyToken(tok))
		}
	}
	return out
}

func normalizeStartElementForComposite(elem xml.StartElement, prefix string) xml.StartElement {
	name := elem.Name.Local
	if strings.Contains(elem.Name.Space, "inkscape") && !strings.HasPrefix(name, "inkscape:") {
		name = "inkscape:" + name
	} else if strings.Contains(elem.Name.Space, "sodipodi") && !strings.HasPrefix(name, "sodipodi:") {
		name = "sodipodi:" + name
	}

	res := xml.StartElement{
		Name: xml.Name{Local: name},
	}

	for _, a := range elem.Attr {
		if a.Name.Local == "xmlns" || a.Name.Space == "xmlns" || a.Name.Space == "_xmlns" || strings.HasPrefix(a.Name.Local, "xmlns:") {
			continue
		}
		attrName := a.Name.Local
		switch {
		case strings.Contains(a.Name.Space, "inkscape") && !strings.HasPrefix(attrName, "inkscape:"):
			attrName = "inkscape:" + attrName
		case strings.Contains(a.Name.Space, "sodipodi") && !strings.HasPrefix(attrName, "sodipodi:"):
			attrName = "sodipodi:" + attrName
		case strings.Contains(a.Name.Space, "xlink") && !strings.HasPrefix(attrName, "xlink:"):
			attrName = "xlink:" + attrName
		}

		val := a.Value
		if prefix != "" {
			switch attrName {
			case "id":
				if val != "" {
					val = prefix + val
				}
			case "href", "xlink:href":
				if strings.HasPrefix(val, "#") && len(val) > 1 {
					val = "#" + prefix + val[1:]
				}
			case "fill", "stroke", "clip-path", "filter", "mask", "style":
				val = urlRefRe.ReplaceAllString(val, "url(#"+prefix+"$1)")
			case "inkscape:label", "label":
				val = pivotRefReInSVG.ReplaceAllString(val, "pivot:#"+prefix+"$1")
			}
		}

		res.Attr = append(res.Attr, xml.Attr{
			Name:  xml.Name{Local: attrName},
			Value: val,
		})
	}

	return res
}

var pivotRefReInSVG = regexp.MustCompile(`(?i)\bpivot:\s*#([a-zA-Z0-9_\-\.:]+)`)

func normalizeEndElementForComposite(elem xml.EndElement) xml.EndElement {
	name := elem.Name.Local
	if strings.Contains(elem.Name.Space, "inkscape") && !strings.HasPrefix(name, "inkscape:") {
		name = "inkscape:" + name
	} else if strings.Contains(elem.Name.Space, "sodipodi") && !strings.HasPrefix(name, "sodipodi:") {
		name = "sodipodi:" + name
	}
	return xml.EndElement{Name: xml.Name{Local: name}}
}
