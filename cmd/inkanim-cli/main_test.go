package main

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
)

func TestCLIRun(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	// 1. Test -v (version flag)
	flag.CommandLine = flag.NewFlagSet("inkanim-cli", flag.ContinueOnError)
	os.Args = []string{"inkanim-cli", "-v"}
	if err := run(); err != nil {
		t.Fatalf("expected -v to succeed, got %v", err)
	}

	// 2. Test missing -i flag
	flag.CommandLine = flag.NewFlagSet("inkanim-cli", flag.ContinueOnError)
	os.Args = []string{"inkanim-cli"}
	if err := run(); err == nil {
		t.Fatalf("expected error when -i is missing")
	}

	// 3. Test full SVG -> GIF CLI export with Twitch validation and pingpong
	svgPath, err := filepath.Abs("../../testdata/pendulum.svg")
	if err != nil {
		t.Fatalf("failed to resolve pendulum.svg: %v", err)
	}
	outGIF := filepath.Join(t.TempDir(), "cli_out.gif")

	flag.CommandLine = flag.NewFlagSet("inkanim-cli", flag.ContinueOnError)
	os.Args = []string{"inkanim-cli", "-i", svgPath, "-o", outGIF, "-size", "64", "-fps", "10", "-pingpong", "-check-twitch"}
	if err := run(); err != nil {
		t.Fatalf("CLI export failed: %v", err)
	}

	fi, err := os.Stat(outGIF)
	if err != nil || fi.Size() == 0 {
		t.Fatalf("expected non-empty output GIF at %s, err=%v", outGIF, err)
	}
}
