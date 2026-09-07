package main

import (
	"flag"
	"log"

	"fyne.io/fyne/v2/app"

	"inkanim/assets"
	"inkanim/internal/prof"
	"inkanim/internal/ui"
)

func main() {
	cpuprofile := flag.String("cpuprofile", "", "Write cpu profile to file")
	memprofile := flag.String("memprofile", "", "Write memory profile to file")
	pprofAddr := flag.String("pprof", "", "Serve live pprof HTTP endpoint at address (e.g. :6060)")
	flag.Parse()

	cleanup, err := prof.SetupProfiler(*cpuprofile, *memprofile, *pprofAddr)
	if err != nil {
		log.Printf("[pprof] Profiler error: %v", err)
	} else if cleanup != nil {
		defer cleanup()
	}

	myApp := app.NewWithID("com.inkanim.studio")
	myApp.SetIcon(assets.AppIcon)
	myApp.Settings().SetTheme(&ui.StudioTheme{})

	win := ui.NewMainWindow(myApp)

	if flag.NArg() > 0 && flag.Arg(0) != "" {
		win.OpenFile(flag.Arg(0))
	}

	win.ShowAndRun()
}

