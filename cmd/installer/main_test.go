package main

import (
	"context"
	"testing"
)

func TestBuildAppSpec(t *testing.T) {
	spec := buildAppSpec()
	if spec.AppName != "InkAnim" {
		t.Fatalf("expected AppName InkAnim, got %q", spec.AppName)
	}
	if spec.AppID != "com.inkanim.studio" {
		t.Fatalf("expected AppID com.inkanim.studio, got %q", spec.AppID)
	}
	if spec.BinaryName != "inkanim.exe" {
		t.Fatalf("expected BinaryName inkanim.exe, got %q", spec.BinaryName)
	}
	if spec.PayloadSubdir != "payload" {
		t.Fatalf("expected PayloadSubdir payload, got %q", spec.PayloadSubdir)
	}
	if len(spec.OptionalComponents) != 1 || spec.OptionalComponents[0].ID != "starter_samples" {
		t.Fatalf("expected starter_samples optional component, got %+v", spec.OptionalComponents)
	}

	origOverride := starterSampleDirOverride
	starterSampleDirOverride = t.TempDir()
	defer func() { starterSampleDirOverride = origOverride }()

	err := spec.OptionalComponents[0].Run(context.Background(), t.TempDir(), func(_, _ int64, _ string) {})
	if err != nil {
		t.Fatalf("starter_samples component failed: %v", err)
	}
}
