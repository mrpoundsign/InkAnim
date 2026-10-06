package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"inkanim/internal/ext/inkscapeargs"
	"inkanim/internal/ext/svgpatch"
	"inkanim/internal/ui"
	"inkanim/pkg/inksvg"
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
		return fmt.Errorf("unsupported mode %q: only editor mode is supported in this spike", req.Mode)
	}

	data, err := os.ReadFile(req.InputPath)
	if err != nil {
		return fmt.Errorf("reading input SVG file %q: %w", req.InputPath, err)
	}
	logDebug("Read %d bytes from %s", len(data), req.InputPath)

	doc, _ := inksvg.ParseSVG(data)
	var layerCount int
	var migrationCount int
	if doc != nil {
		layerCount = len(doc.Layers)
		migrationCount = len(doc.Migrations)
	}

	a := app.NewWithID("com.mrpoundsign.inkanim.ext")
	a.Settings().SetTheme(&ui.StudioTheme{})

	w := a.NewWindow("InkAnim Motion Editor (Spike)")
	w.Resize(fyne.NewSize(500, 240))

	result := data
	applied := false

	applyBtn := widget.NewButton("Apply", func() {
		logDebug("Apply button clicked. IDs: %v", req.IDs)
		if len(req.IDs) > 0 {
			targetID := req.IDs[0]
			currentLabel, hasLabel := svgpatch.GetAttr(data, targetID, "label")
			logDebug("targetID=%s, hasLabel=%v, currentLabel=%q", targetID, hasLabel, currentLabel)
			newLabel := currentLabel + " · Spike"
			patched, patchErr := svgpatch.SetAttr(data, targetID, svgpatch.InkscapeNS, "label", newLabel)
			if patchErr == nil {
				result = patched
				applied = true
				logDebug("SetAttr succeeded. Patched length: %d", len(patched))
			} else {
				logDebug("SetAttr error: %v", patchErr)
				fmt.Fprintf(os.Stderr, "Failed to patch element %q: %v\n", targetID, patchErr)
			}
		} else {
			applied = true
			logDebug("No IDs selected, applying original data")
		}
		w.Close()
		a.Quit()
	})
	applyBtn.Importance = widget.HighImportance

	cancelBtn := widget.NewButton("Cancel", func() {
		logDebug("Cancel button clicked")
		result = data
		w.Close()
		a.Quit()
	})

	w.SetOnClosed(func() {
		logDebug("w.SetOnClosed fired. applied=%v", applied)
		if !applied {
			result = data
		}
	})

	idsText := "none"
	if len(req.IDs) > 0 {
		idsText = strings.Join(req.IDs, ", ")
	}

	filePathLabel := widget.NewLabel("File: " + req.InputPath)
	filePathLabel.Truncation = fyne.TextTruncateClip

	infoForm := widget.NewForm(
		widget.NewFormItem("Selected IDs", widget.NewLabel(idsText)),
		widget.NewFormItem("Frames", widget.NewLabel(strconv.Itoa(layerCount))),
		widget.NewFormItem("Migrations", widget.NewLabel(strconv.Itoa(migrationCount))),
	)

	btnRow := container.NewHBox(layout.NewSpacer(), cancelBtn, applyBtn)
	content := container.NewVBox(
		filePathLabel,
		widget.NewSeparator(),
		infoForm,
		widget.NewSeparator(),
		btnRow,
	)

	w.SetContent(container.NewPadded(content))
	w.Show()
	a.Run()

	logDebug("a.Run exited. applied=%v, resultLen=%d", applied, len(result))

	if applied {
		if err := writeOutput(result); err != nil {
			return fmt.Errorf("writing SVG to stdout: %w", err)
		}
		logDebug("writeOutput completed successfully")
	}

	return nil
}
