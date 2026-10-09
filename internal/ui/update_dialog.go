package ui

import (
	"fmt"
	"net/url"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"inkanim/internal/app"
)

// CheckStartupUpdate verifies if a new version is available and prompts the user.
func CheckStartupUpdate(parent fyne.Window, currentVersion string) {
	prefs := fyne.CurrentApp().Preferences()

	snoozeUntil := prefs.String("UpdateSnoozeUntil")
	if snoozeUntil != "" {
		if t, err := time.Parse(time.RFC3339, snoozeUntil); err == nil {
			if time.Now().Before(t) {
				return
			}
		}
	}

	skippedVersion := prefs.String("SkippedUpdateVersion")

	app.CheckForUpdateAsync(currentVersion, nil, func(res *app.UpdateResult, err error) {
		if err != nil || !res.IsOutdated {
			return
		}
		if res.LatestVersion == skippedVersion {
			return
		}

		fyne.Do(func() {
			var dlg dialog.Dialog

			msg := fmt.Sprintf("A new version of InkAnim is available!\n\nCurrent: %s\nLatest: %s", res.CurrentVersion, res.LatestVersion)
			label := widget.NewLabel(msg)

			downloadBtn := widget.NewButton("Download Now", func() {
				if u, err := url.Parse(res.ReleaseURL); err == nil {
					_ = fyne.CurrentApp().OpenURL(u)
				}
				dlg.Hide()
			})
			downloadBtn.Importance = widget.HighImportance

			snoozeBtn := widget.NewButton("Remind Me Later", func() {
				fyne.CurrentApp().Preferences().SetString("UpdateSnoozeUntil", time.Now().Add(24*time.Hour).Format(time.RFC3339))
				dlg.Hide()
			})

			skipBtn := widget.NewButton("Skip This Version", func() {
				fyne.CurrentApp().Preferences().SetString("SkippedUpdateVersion", res.LatestVersion)
				dlg.Hide()
			})

			buttons := container.NewHBox(skipBtn, snoozeBtn, downloadBtn)
			content := container.NewVBox(label, widget.NewSeparator(), buttons)

			dlg = dialog.NewCustom("Update Available", "Close", content, parent)
			dlg.Show()
		})
	})
}
