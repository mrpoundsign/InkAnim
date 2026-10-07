package doctree

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"

	"inkanim/pkg/inksvg"
)

// Directive represents a single IAMS motion directive parsed from an element's label.
type Directive struct {
	Type   string // e.g. "Move", "Rot", "Scale", "Fade", "Show", "Hide", "Depth", "Dist", "Color"
	Params string // e.g. "f: 1-20; ease: in-out"
	Raw    string // e.g. "Move {f: 1-20; ease: in-out}"
}

// DocNode represents an element in the SVG document hierarchy.
type DocNode struct {
	ID          string
	Tag         string
	Label       string // inkscape:label if set
	IsLayer     bool   // true if inkscape:groupmode="layer"
	IsGroup     bool   // true if tag == "g"
	Directives  []Directive
	LabelPrefix string // text before any motion directive (e.g. "Fade In: ")
	LabelSuffix string // text after all motion directives (e.g. " · Preset")
	ParentID    string
	Parent      *DocNode
	Children    []*DocNode
}

// DisplayTitle returns a friendly label for the UI tree node.
func (n *DocNode) DisplayTitle() string {
	if n.LabelPrefix != "" {
		return n.LabelPrefix
	}
	if n.ID != "" {
		return n.ID
	}
	if n.Label != "" {
		return n.Label
	}
	return n.Tag
}

// HasDirectives returns true if the node has at least one motion directive.
func (n *DocNode) HasDirectives() bool {
	return len(n.Directives) > 0
}

// FormatLabel constructs the full inkscape:label string from prefix, directives, and suffix.
func (n *DocNode) FormatLabel() string {
	var parts []string
	if n.LabelPrefix != "" {
		parts = append(parts, n.LabelPrefix)
	}
	for _, d := range n.Directives {
		parts = append(parts, d.Raw)
	}
	if n.LabelSuffix != "" {
		parts = append(parts, n.LabelSuffix)
	}
	return strings.Join(parts, " ")
}

var directiveScannerRe = regexp.MustCompile(`(?i)\b(move|motion|movement|rot|scale|scal|fade|show|hide|depth|camera|distance|dist|color)\s*\{([^}]*)\}`)

// ParseDirectives extracts all motion directives from an inkscape:label.
func ParseDirectives(label string) (prefix string, directives []Directive, suffix string) {
	label = inksvg.MigrateLabel(label)
	matches := directiveScannerRe.FindAllStringSubmatchIndex(label, -1)
	if len(matches) == 0 {
		return strings.TrimSpace(label), nil, ""
	}

	firstStart := matches[0][0]
	lastEnd := matches[len(matches)-1][1]

	prefix = strings.TrimSpace(label[:firstStart])
	suffix = strings.TrimSpace(label[lastEnd:])

	for _, loc := range matches {
		typeStart, typeEnd := loc[2], loc[3]
		paramsStart, paramsEnd := loc[4], loc[5]

		rawDType := label[typeStart:typeEnd]
		var dType string
		switch strings.ToLower(rawDType) {
		case "motion", "movement":
			dType = "Move"
		case "scal":
			dType = "Scale"
		case "distance":
			dType = "Dist"
		default:
			if len(rawDType) > 0 {
				dType = strings.ToUpper(rawDType[:1]) + strings.ToLower(rawDType[1:])
			}
		}

		params := strings.TrimSpace(label[paramsStart:paramsEnd])
		raw := fmt.Sprintf("%s {%s}", dType, params)

		directives = append(directives, Directive{
			Type:   dType,
			Params: params,
			Raw:    raw,
		})
	}

	return prefix, directives, suffix
}

var ignorableTags = map[string]bool{
	"defs":           true,
	"namedview":      true,
	"metadata":       true,
	"marker":         true,
	"clipPath":       true,
	"mask":           true,
	"linearGradient": true,
	"radialGradient": true,
	"pattern":        true,
	"filter":         true,
	"style":          true,
	"script":         true,
}

// ParseTree parses the SVG XML into a hierarchy of DocNodes rooted at top-level groups/elements.
func ParseTree(svgData []byte) ([]*DocNode, map[string]*DocNode, error) {
	decoder := xml.NewDecoder(bytes.NewReader(svgData))
	nodeMap := make(map[string]*DocNode)
	var roots []*DocNode

	type stackItem struct {
		tag  string
		node *DocNode
		skip bool
	}

	var stack []stackItem

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, err
		}

		switch tok := token.(type) {
		case xml.StartElement:
			localTag := tok.Name.Local
			inSkippedParent := len(stack) > 0 && stack[len(stack)-1].skip

			if inSkippedParent || ignorableTags[localTag] {
				stack = append(stack, stackItem{tag: localTag, skip: true})
				continue
			}

			// Don't create a DocNode for the root <svg> element itself, but process its children
			if localTag == "svg" {
				stack = append(stack, stackItem{tag: localTag, skip: false})
				continue
			}

			var id, label, groupMode string
			for _, attr := range tok.Attr {
				switch attr.Name.Local {
				case "id":
					id = attr.Value
				case "label":
					label = attr.Value
				case "groupmode":
					groupMode = attr.Value
				}
			}

			if id == "" {
				// Generate a synthetic or fallback identifier for grouping
				id = localTag
			}

			prefix, directives, suffix := ParseDirectives(label)
			if len(directives) == 0 && prefix == "" {
				prefix = id
			}

			node := &DocNode{
				ID:          id,
				Tag:         localTag,
				Label:       label,
				IsLayer:     groupMode == "layer",
				IsGroup:     localTag == "g",
				Directives:  directives,
				LabelPrefix: prefix,
				LabelSuffix: suffix,
			}

			nodeMap[id] = node

			// Attach to closest parent DocNode in stack
			var parent *DocNode
			for _, s := range slices.Backward(stack) {
				if s.node != nil {
					parent = s.node
					break
				}
			}

			if parent != nil {
				node.Parent = parent
				node.ParentID = parent.ID
				parent.Children = append(parent.Children, node)
			} else {
				roots = append(roots, node)
			}

			stack = append(stack, stackItem{tag: localTag, node: node, skip: false})

		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}

	// Reverse tree hierarchy so display order matches Inkscape's Layers & Objects panel
	// (visual z-order: top-most layer/object at the top, bottom-most layer at the bottom).
	var reverseOrder func(nodes []*DocNode)
	reverseOrder = func(nodes []*DocNode) {
		slices.Reverse(nodes)
		for _, n := range nodes {
			if len(n.Children) > 0 {
				reverseOrder(n.Children)
			}
		}
	}
	reverseOrder(roots)

	return roots, nodeMap, nil
}
