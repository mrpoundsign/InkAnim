package main

import (
	"fmt"
	"os"

	"fyne.io/fyne/v2/app"

	"inkanim/assets"
	"inkanim/internal/ext/inkscapeargs"
	"inkanim/internal/ui"
)

func main() {
	initDebugLog()
	code := runMain()
	closeDebugLog()
	if code != 0 {
		os.Exit(code)
	}
}

func runMain() int {
	logDebug("=== inkanim-ext started (PID %d) ===", os.Getpid())
	logDebug("os.Args: %v", os.Args)

	if err := run(); err != nil {
		logDebug("run() failed: %v", err)
		fmt.Fprintf(os.Stderr, "inkanim-ext: %v\n", err)
		return 1
	}
	logDebug("inkanim-ext completed successfully")
	return 0
}

func run() error {
	req, err := inkscapeargs.Parse(os.Args[1:])
	if err != nil {
		return fmt.Errorf("parsing arguments: %w", err)
	}
	logDebug("Parsed request: Mode=%s, IDs=%v, InputPath=%s", req.Mode, req.IDs, req.InputPath)

	if req.Mode != "editor" {
		return fmt.Errorf("unsupported mode %q: only editor mode is supported", req.Mode)
	}

	data, err := os.ReadFile(req.InputPath)
	if err != nil {
		return fmt.Errorf("reading input SVG file %q: %w", req.InputPath, err)
	}
	logDebug("Read %d bytes from %s", len(data), req.InputPath)

	editorState, err := NewEditorState(data, req.InputPath, req.IDs)
	if err != nil {
		return fmt.Errorf("parsing SVG document structure: %w", err)
	}

	a := app.NewWithID("com.mrpoundsign.inkanim.ext")
	a.SetIcon(assets.AppIcon)
	a.Settings().SetTheme(&ui.StudioTheme{})

	ShowEditorWindow(a, editorState)
	a.Run()

	logDebug("a.Run exited. applied=%v, resultLen=%d", editorState.Applied, len(editorState.Result))

	if editorState.Applied {
		if err := writeOutput(editorState.Result); err != nil {
			return fmt.Errorf("writing SVG to stdout: %w", err)
		}
		logDebug("writeOutput completed successfully")
	}

	return nil
}
