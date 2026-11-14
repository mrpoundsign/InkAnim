package ui

import (
	"context"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"github.com/mrpoundsign/appinstaller/pkg/updater"
	"github.com/mrpoundsign/appinstaller/pkg/updaterui"
)

func showSelfUpdateDialog(parent fyne.Window, currentVersion string, rel *updater.ReleaseInfo, checker *updater.Checker) {
	updaterui.ShowUpdateDialog(updaterui.DialogConfig{
		Parent:         parent,
		AppName:        "InkAnim",
		CurrentVersion: currentVersion,
		Release:        rel,
		Checker:        checker,
		OnSkipVersion: func(ver string) {
			if a := fyne.CurrentApp(); a != nil {
				a.Preferences().SetString("SkippedUpdateVersion", ver)
			}
		},
		OnQuitApp: func() {
			if a := fyne.CurrentApp(); a != nil {
				a.Quit()
			}
		},
	})
}

// CheckStartupUpdate verifies if a new version is available and prompts the user with SHA-256 verified self-update.
func CheckStartupUpdate(parent fyne.Window, currentVersion string) {
	if strings.EqualFold(strings.TrimSpace(currentVersion), "dev") {
		return
	}
	a := fyne.CurrentApp()
	if a == nil {
		return
	}
	prefs := a.Preferences()

	snoozeUntil := prefs.String("UpdateSnoozeUntil")
	if snoozeUntil != "" {
		if t, err := time.Parse(time.RFC3339, snoozeUntil); err == nil {
			if time.Now().Before(t) {
				return
			}
		}
	}

	skippedVersion := prefs.String("SkippedUpdateVersion")

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		checker := updater.NewChecker("mrpoundsign", "InkAnim")
		rel, isNewer, err := checker.CheckLatest(ctx, currentVersion)
		if err != nil || !isNewer || rel == nil {
			return
		}
		if skippedVersion != "" && updater.NormalizeVersion(rel.TagName) == updater.NormalizeVersion(skippedVersion) {
			return
		}

		fyne.Do(func() {
			showSelfUpdateDialog(parent, currentVersion, rel, checker)
		})
	}()
}
