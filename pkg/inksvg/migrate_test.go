package inksvg

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestMigrateLabel(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{
			input: "Movement {f: 1-20}",
			want:  "Move {f: 1-20}",
		},
		{
			input: "movement{f:all}",
			want:  "Move{f:all}",
		},
		{
			input: "Move {f:1-2}",
			want:  "Move {f:1-2}",
		},
		{
			input: "Movements",
			want:  "Movements",
		},
	}

	for _, tt := range tests {
		got := migrateLabel(tt.input)
		if got != tt.want {
			t.Errorf("migrateLabel(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestDetectMigrations(t *testing.T) {
	movementHits := DetectMigrations("Movement {f: 1-20; ease: in-out}")
	if len(movementHits) != 1 {
		t.Fatalf("expected 1 hit for Movement label, got %d", len(movementHits))
	}
	if movementHits[0].RuleID != "movement-to-move" {
		t.Errorf("expected RuleID 'movement-to-move', got %q", movementHits[0].RuleID)
	}
	if movementHits[0].Before != "Movement {f: 1-20; ease: in-out}" {
		t.Errorf("unexpected Before: %q", movementHits[0].Before)
	}
	if movementHits[0].After != "Move {f: 1-20; ease: in-out}" {
		t.Errorf("unexpected After: %q", movementHits[0].After)
	}

	moveHits := DetectMigrations("Move {f: 1-20; ease: in-out}")
	if len(moveHits) != 0 {
		t.Errorf("expected 0 hits for current Move label, got %d", len(moveHits))
	}
}

func TestMigration_MovementLegacyFixture(t *testing.T) {
	legacyPath := filepath.Join("..", "..", "testdata", "migrations", "movement_legacy.svg")
	legacyBytes, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatalf("failed to read legacy fixture: %v", err)
	}
	legacyHash := sha256.Sum256(legacyBytes)

	currentPath := filepath.Join("..", "..", "testdata", "spline_test.svg")
	currentBytes, err := os.ReadFile(currentPath)
	if err != nil {
		t.Fatalf("failed to read current spline_test.svg: %v", err)
	}

	legacyDoc, err := ParseSVG(legacyBytes)
	if err != nil {
		t.Fatalf("failed to parse legacy fixture: %v", err)
	}

	currentDoc, err := ParseSVG(currentBytes)
	if err != nil {
		t.Fatalf("failed to parse current spline_test: %v", err)
	}

	if len(legacyDoc.Migrations) != 2 {
		t.Fatalf("expected 2 migrations in legacyDoc, got %d", len(legacyDoc.Migrations))
	}
	for i, m := range legacyDoc.Migrations {
		if m.RuleID != "movement-to-move" {
			t.Errorf("migration[%d] RuleID = %q, want 'movement-to-move'", i, m.RuleID)
		}
	}

	if len(legacyDoc.Layers) != 20 || len(currentDoc.Layers) != 20 {
		t.Fatalf("layer count mismatch: legacy=%d, current=%d, want 20", len(legacyDoc.Layers), len(currentDoc.Layers))
	}

	if len(legacyDoc.MotionPaths) != len(currentDoc.MotionPaths) {
		t.Fatalf("motion paths count mismatch: legacy=%d, current=%d", len(legacyDoc.MotionPaths), len(currentDoc.MotionPaths))
	}

	for i := range legacyDoc.MotionPaths {
		lPath := legacyDoc.MotionPaths[i]
		cPath := currentDoc.MotionPaths[i]
		if lPath.Config != cPath.Config {
			t.Errorf("motion path %d config mismatch:\nlegacy: %+v\ncurrent: %+v", i, lPath.Config, cPath.Config)
		}
	}

	labelRe := regexp.MustCompile(`inkscape:label="[^"]*"`)
	for i := 0; i < len(legacyDoc.Layers); i++ {
		legacyFrameSVG, err := BuildTimelineFrameSVG(legacyDoc, i, legacyDoc.GetDocumentRect())
		if err != nil {
			t.Fatalf("BuildTimelineFrameSVG failed on legacy doc frame %d: %v", i, err)
		}
		currentFrameSVG, err := BuildTimelineFrameSVG(currentDoc, i, currentDoc.GetDocumentRect())
		if err != nil {
			t.Fatalf("BuildTimelineFrameSVG failed on current doc frame %d: %v", i, err)
		}

		normLegacy := labelRe.ReplaceAllString(string(legacyFrameSVG), `inkscape:label=""`)
		normCurrent := labelRe.ReplaceAllString(string(currentFrameSVG), `inkscape:label=""`)

		if normLegacy != normCurrent {
			t.Errorf("frame %d normalized SVG mismatch between legacy and current", i)
		}
	}

	afterBytes, err := os.ReadFile(legacyPath)
	if err != nil {
		t.Fatalf("failed to re-read legacy fixture: %v", err)
	}
	afterHash := sha256.Sum256(afterBytes)
	if legacyHash != afterHash {
		t.Errorf("legacy fixture file was modified on disk! Hash mismatch.")
	}
}
