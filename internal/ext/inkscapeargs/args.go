package inkscapeargs

import (
	"errors"
	"strings"
)

// Request represents the parsed invocation parameters from Inkscape.
type Request struct {
	Mode      string
	IDs       []string
	Params    map[string]string
	InputPath string
}

// Parse extracts typed Request parameters from Inkscape CLI arguments.
// It handles repeated --id flags, --mode, key=value options, and the input SVG file path.
func Parse(args []string) (Request, error) {
	req := Request{
		Mode:   "editor",
		Params: make(map[string]string),
	}

	var positional []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "--") {
			opt := arg[2:]
			key, val, hasEq := strings.Cut(opt, "=")
			if !hasEq {
				// Inkscape might pass flags as "--name value"
				if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
					val = args[i+1]
					i++
				} else {
					val = "true"
				}
			}
			switch key {
			case "id":
				req.IDs = append(req.IDs, val)
			case "mode":
				req.Mode = val
			default:
				req.Params[key] = val
			}
		} else {
			positional = append(positional, arg)
		}
	}

	if len(positional) == 0 {
		return req, errors.New("missing input SVG path")
	}

	// The last non-flag argument is the input file path
	req.InputPath = positional[len(positional)-1]
	return req, nil
}
