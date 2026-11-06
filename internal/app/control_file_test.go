package app

import (
	"os"
	"path/filepath"
	"testing"

	"inkanim/pkg/inksvg"
)

func TestControlFileRoundTripAndResolve(t *testing.T) {
	tmpDir := t.TempDir()

	srcSVG, err := filepath.Abs("../../testdata/pendulum.svg")
	if err != nil {
		t.Fatalf("failed to resolve test SVG path: %v", err)
	}
	svgBytes, err := os.ReadFile(srcSVG)
	if err != nil {
		t.Fatalf("failed to read test SVG: %v", err)
	}

	svgPath := filepath.Join(tmpDir, "emote.svg")
	if err := os.WriteFile(svgPath, svgBytes, 0644); err != nil {
		t.Fatalf("failed to write temp SVG: %v", err)
	}

	// 1. Resolve when no .inka sidecar exists yet
	resolvedSVG, ctrl, err := ResolveSVGAndControlFile(svgPath)
	if err != nil {
		t.Fatalf("ResolveSVGAndControlFile failed: %v", err)
	}
	if resolvedSVG != svgPath {
		t.Errorf("expected resolved SVG %q, got %q", svgPath, resolvedSVG)
	}
	if ctrl != nil {
		t.Errorf("expected nil ControlFile when no .inka exists, got %+v", ctrl)
	}

	// 2. Build and save a .inka control file
	sess := NewSession()
	if err := sess.LoadSVG(svgPath); err != nil {
		t.Fatalf("LoadSVG failed: %v", err)
	}
	_ = sess.SetFPS(20)
	_ = sess.SetTotalDurationSec(2.5)
	_ = sess.SetCropBoundary(inksvg.BoundaryDrawing, 0)
	sess.ExportOptions.PingPong = true
	sess.ExportOptions.ExportSquare = true
	sess.ExportOptions.SquareSize = 128
	sess.ExportOptions.NumColors = 64
	sess.ExportOptions.Dither = true
	sess.ShowMotionLines = true

	built := sess.BuildControlFile(svgPath)
	built.Export.Preset = "Discord Emote (128x128 Square)"
	built.Preview.Background = "Twitch Dark"
	built.Preview.ShowGuides = false
	built.Preview.ShowScales = false

	inkaPath := SidecarPathForSVG(svgPath)
	if filepath.Base(inkaPath) != "emote.inka" {
		t.Fatalf("expected emote.inka, got %s", inkaPath)
	}
	if err := SaveControlFile(inkaPath, built); err != nil {
		t.Fatalf("SaveControlFile failed: %v", err)
	}

	// 3. Resolve from .svg path (auto-detects sibling .inka)
	resolvedFromSVG, ctrlFromSVG, err := ResolveSVGAndControlFile(svgPath)
	if err != nil {
		t.Fatalf("ResolveSVGAndControlFile(svgPath) failed: %v", err)
	}
	if resolvedFromSVG != svgPath || ctrlFromSVG == nil {
		t.Fatalf("expected non-nil ControlFile from sibling .inka")
	}

	// 4. Resolve directly from .inka path
	resolvedFromInka, ctrlFromInka, err := ResolveSVGAndControlFile(inkaPath)
	if err != nil {
		t.Fatalf("ResolveSVGAndControlFile(inkaPath) failed: %v", err)
	}
	if resolvedFromInka != svgPath || ctrlFromInka == nil {
		t.Fatalf("expected resolved SVG %q from .inka, got %q", svgPath, resolvedFromInka)
	}

	// 5. Apply control file to a fresh session before LoadSVGData
	sess2 := NewSession()
	sess2.ApplyControlFile(ctrlFromInka)
	if err := sess2.LoadSVGData(svgBytes, svgPath); err != nil {
		t.Fatalf("sess2.LoadSVGData failed: %v", err)
	}
	if sess2.FPS != 20 {
		t.Errorf("expected FPS 20, got %d", sess2.FPS)
	}
	if sess2.TotalDurationMs != 2500 {
		t.Errorf("expected TotalDurationMs 2500, got %d", sess2.TotalDurationMs)
	}
	if len(sess2.RenderedFrames) != 50 {
		t.Errorf("expected 50 interpolated frames (2.5s @ 20 FPS), got %d", len(sess2.RenderedFrames))
	}
	if sess2.CropBoundaryMode != inksvg.BoundaryDrawing {
		t.Errorf("expected CropBoundaryMode drawing, got %s", sess2.CropBoundaryMode)
	}
	if !sess2.ExportOptions.PingPong || sess2.ExportOptions.SquareSize != 128 || sess2.ExportOptions.NumColors != 64 || !sess2.ExportOptions.Dither {
		t.Errorf("unexpected ExportOptions after ApplyControlFile: %+v", sess2.ExportOptions)
	}
	if !sess2.ShowMotionLines {
		t.Errorf("expected ShowMotionLines=true after ApplyControlFile")
	}
}

func TestControlFileMultiLinkedSourcesAndIAMSMigration(t *testing.T) {
	projectDir := t.TempDir()
	externalDir := t.TempDir()

	svgHero := `<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape" width="256" height="256" viewBox="0 0 256 256">
  <g id="layer1" inkscape:groupmode="layer" inkscape:label="Scene">
    <g id="hero_g" inkscape:label="Hero">
      <rect id="hero_rect" x="40" y="40" width="60" height="60" fill="#38bdf8"/>
      <path id="hero_move" inkscape:label="Move{f:1-5; ease:linear}" d="M 40 40 L 140 40" fill="none"/>
    </g>
  </g>
</svg>`

	svgProps := `<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape" width="256" height="256" viewBox="0 0 256 256">
  <g id="layer_props" inkscape:groupmode="layer" inkscape:label="Props">
    <g id="star_g" inkscape:label="Star">
      <circle id="star_c" cx="200" cy="200" r="20" fill="#facc15"/>
    </g>
  </g>
</svg>`

	heroPath := filepath.Join(projectDir, "Hero.svg")
	if err := os.WriteFile(heroPath, []byte(svgHero), 0644); err != nil {
		t.Fatalf("write Hero.svg: %v", err)
	}

	extPropsPath := filepath.Join(externalDir, "Props.svg")
	if err := os.WriteFile(extPropsPath, []byte(svgProps), 0644); err != nil {
		t.Fatalf("write external Props.svg: %v", err)
	}

	// Parse Hero.svg and migrate its legacy IAMS into a ControlFile, then strip IAMS from Hero.svg on disk
	heroDoc, err := inksvg.ParseSVG([]byte(svgHero))
	if err != nil {
		t.Fatalf("ParseSVG(Hero.svg) failed: %v", err)
	}

	ctrl := &ControlFile{
		Version: ControlFileVersion,
		Timing: ControlFileTiming{
			DurationSec: 1.0,
			FPS:         10,
			Loop:        true,
		},
	}
	if _, err := ctrl.AddLinkedSVG(projectDir, heroPath); err != nil {
		t.Fatalf("AddLinkedSVG(Hero.svg) failed: %v", err)
	}
	// Link external Props.svg -> should copy Props.svg into projectDir automatically
	linkedProps, err := ctrl.AddLinkedSVG(projectDir, extPropsPath)
	if err != nil {
		t.Fatalf("AddLinkedSVG(Props.svg) failed: %v", err)
	}
	if linkedProps.ID != "src_2" || linkedProps.File != "Props.svg" {
		t.Errorf("unexpected linkedProps: %+v", linkedProps)
	}
	if _, statErr := os.Stat(filepath.Join(projectDir, "Props.svg")); statErr != nil {
		t.Fatalf("expected Props.svg to be copied into projectDir: %v", statErr)
	}

	migrated := MigrateDocumentIAMSToControlFile(heroDoc, "src_1", 1.0, ctrl)
	if migrated != 1 {
		t.Fatalf("expected 1 migrated IAMS action, got %d", migrated)
	}

	cleanedHero, err := inksvg.StripIAMSFromSVG([]byte(svgHero))
	if err != nil {
		t.Fatalf("StripIAMSFromSVG failed: %v", err)
	}
	if err := os.WriteFile(heroPath, cleanedHero, 0644); err != nil {
		t.Fatalf("write cleaned Hero.svg: %v", err)
	}

	// Also add a .inka stage object config for src_2/Props/Star
	ctrl.Objects["src_2/Props/Star"] = ObjectConfig{
		SourceID:         "src_2",
		LabelPath:        "Props/Star",
		FallbackXMLID:    "star_g",
		HasDisplayWindow: true,
		StartTimeSec:     0.2,
		EndTimeSec:       1.0,
		OverrideStartPos: true,
		StartX:           100,
		StartY:           100,
	}

	inkaPath := filepath.Join(projectDir, "scene.inka")
	if err := SaveControlFile(inkaPath, ctrl); err != nil {
		t.Fatalf("SaveControlFile(scene.inka) failed: %v", err)
	}

	// Load scene.inka into a fresh Session: both stripped Hero.svg and co-located Props.svg
	// should compose and animate via .inka Objects & Actions
	sess := NewSession()
	if err := sess.LoadSVG(inkaPath); err != nil {
		t.Fatalf("sess.LoadSVG(scene.inka) failed: %v", err)
	}
	if len(sess.LinkedSources) != 2 {
		t.Errorf("expected 2 LinkedSources in session, got %d", len(sess.LinkedSources))
	}
	if len(sess.RenderedFrames) != 10 {
		t.Errorf("expected 10 rendered frames (1.0s @ 10 FPS), got %d", len(sess.RenderedFrames))
	}
}
