package main

import (
	"context"
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/mrpoundsign/appinstaller/pkg/installer"
	"github.com/mrpoundsign/appinstaller/pkg/installerui"

	"inkanim/assets"
	"inkanim/internal/app"
)

//go:embed payload/*
var payloadFS embed.FS

// AppVersion is the installer version (injected via -ldflags on release builds).
var AppVersion = app.Version

// starterSampleDirOverride lets unit tests redirect sample installation into os.TempDir().
var starterSampleDirOverride string

func main() {
	os.Exit(installerui.RunMain(buildAppSpec(), assets.AppIcon))
}

func buildAppSpec() installer.AppSpec {
	return installer.AppSpec{
		AppName:       "InkAnim",
		AppID:         "com.inkanim.studio",
		Description:   "Inkscape SVG to Animated GIF Studio",
		Publisher:     "mrpoundsign",
		Version:       AppVersion,
		RepoOwner:     "mrpoundsign",
		RepoName:      "InkAnim",
		BinaryName:    "inkanim.exe",
		PayloadFS:     payloadFS,
		PayloadSubdir: "payload",
		DataDirsToPurge: func() []string {
			var dirs []string
			if cfgDir, err := os.UserConfigDir(); err == nil && cfgDir != "" {
				dirs = append(dirs, filepath.Join(cfgDir, "fyne", "com.inkanim.studio"))
			}
			if localApp := os.Getenv("LOCALAPPDATA"); localApp != "" {
				dirs = append(dirs, filepath.Join(localApp, "com.inkanim.studio"))
			} else if cacheDir, err := os.UserCacheDir(); err == nil && cacheDir != "" {
				dirs = append(dirs, filepath.Join(cacheDir, "fyne", "com.inkanim.studio"))
			}
			return dirs
		},
		OptionalComponents: []installer.ComponentSpec{
			{
				ID:          "starter_samples",
				Title:       "Copy Starter Sample SVG to Documents\\InkAnim",
				Description: "Adds pendulum.svg to your Documents folder so you can try InkAnim right away",
				DefaultOn:   true,
				Run: func(_ context.Context, _ string, progress func(cur, total int64, status string)) error {
					progress(0, 1, "Copying starter sample SVG...")
					data, err := fs.ReadFile(payloadFS, "payload/pendulum.svg")
					if err != nil {
						// Sample file is optional during minimal/placeholder builds.
						progress(1, 1, "No bundled sample SVG present; skipping.")
						return nil
					}
					targetDir := starterSampleDirOverride
					if targetDir == "" {
						if testing.Testing() {
							targetDir = filepath.Join(os.TempDir(), "inkanim-test-samples")
						} else {
							home, homeErr := os.UserHomeDir()
							if homeErr != nil {
								return homeErr
							}
							targetDir = filepath.Join(home, "Documents", "InkAnim")
						}
					}
					if err := os.MkdirAll(targetDir, 0755); err != nil {
						return err
					}
					dst := filepath.Join(targetDir, "pendulum.svg")
					if err := os.WriteFile(dst, data, 0644); err != nil {
						return err
					}
					progress(1, 1, "Copied starter sample to "+dst)
					return nil
				},
			},
		},
	}
}
