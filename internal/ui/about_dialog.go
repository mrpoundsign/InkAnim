package ui

import (
	"fmt"
	"net/url"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"inkanim/assets"
	"inkanim/internal/app"
)

const openSourceLicensesMarkdown = `## Open Source Licenses

InkAnim incorporates the following open-source libraries and components:

---

### Fyne Toolkit (v2.8.1)
- **License**: BSD 3-Clause License
- **URL**: https://fyne.io
- **Copyright**: (c) 2018-present Fyne.io developers. All rights reserved.

---

### oksvg & rasterx (SVG Parsing & Rasterization)
- **License**: BSD 2-Clause / BSD 3-Clause License
- **URL**: https://github.com/srwiley/oksvg & https://github.com/srwiley/rasterx
- **Copyright**: (c) 2017 Stephen Wiley. All rights reserved.

---

### golang.org/x/image (Draw, GIF, Color Quantization)
- **License**: BSD 3-Clause License
- **URL**: https://golang.org/x/image
- **Copyright**: (c) The Go Authors. All rights reserved.

---

### zenity (Native Desktop File Dialogs)
- **License**: MIT License
- **URL**: https://github.com/ncruces/zenity
- **Copyright**: (c) 2020 ncruces. All rights reserved.

---

### Go Standard Library
- **License**: BSD 3-Clause License
- **URL**: https://golang.org
- **Copyright**: (c) The Go Authors. All rights reserved.
`

// ShowLicensesDialog displays the third-party open source licenses modal.
func ShowLicensesDialog(parent fyne.Window) {
	richText := widget.NewRichTextFromMarkdown(openSourceLicensesMarkdown)
	richText.Wrapping = fyne.TextWrapWord

	scroll := container.NewScroll(richText)
	scroll.SetMinSize(fyne.NewSize(500, 360))

	licensesDlg := dialog.NewCustom("Open Source Licenses", "Close", scroll, parent)
	licensesDlg.Resize(fyne.NewSize(540, 420))
	licensesDlg.Show()
}

// ShowAboutDialog presents the application metadata, links, update checker, and licenses modal.
func ShowAboutDialog(parent fyne.Window, currentVersion string) {
	// App Icon
	iconImg := canvas.NewImageFromResource(assets.AppIcon)
	iconImg.SetMinSize(fyne.NewSize(64, 64))
	iconImg.FillMode = canvas.ImageFillContain

	// Title & Tagline (using standard foreground for high-contrast light grey text)
	title := widget.NewLabelWithStyle("InkAnim", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	tagline := widget.NewLabel("Inkscape SVG to Animated GIF Studio")

	cleanVer := strings.TrimPrefix(strings.TrimSpace(currentVersion), "v")
	verStr := fmt.Sprintf("Version: v%s", cleanVer)
	if app.Commit != "" && app.Commit != "none" {
		commitShort := app.Commit
		if len(commitShort) > 7 {
			commitShort = commitShort[:7]
		}
		verStr += fmt.Sprintf(" (%s)", commitShort)
	}
	versionLabel := widget.NewLabel(verStr)

	headerInfo := container.NewVBox(
		title,
		tagline,
		versionLabel,
	)
	header := container.NewHBox(
		iconImg,
		widget.NewSeparator(),
		headerInfo,
	)

	// Update Checker Section
	updateStatus := widget.NewLabel("")
	updateStatus.Wrapping = fyne.TextWrapWord

	viewReleaseBtn := widget.NewButton("View Release on GitHub", nil)
	viewReleaseBtn.Importance = widget.HighImportance
	viewReleaseBtn.Hide()

	var checkBtn *widget.Button
	checkBtn = widget.NewButton("Check for Updates", func() {
		checkBtn.Disable()
		updateStatus.SetText("Checking for updates...")
		viewReleaseBtn.Hide()

		app.CheckForUpdateAsync(currentVersion, nil, func(res *app.UpdateResult, err error) {
			fyne.Do(func() {
				if err != nil {
					updateStatus.SetText(fmt.Sprintf("Update check failed: %v", err))
					checkBtn.Enable()
					return
				}

				if res.IsOutdated {
					updateStatus.SetText(fmt.Sprintf("New version %s is available!", res.LatestVersion))
					releaseURL := res.ReleaseURL
					viewReleaseBtn.OnTapped = func() {
						if parsed, err := url.Parse(releaseURL); err == nil {
							_ = fyne.CurrentApp().OpenURL(parsed)
						}
					}
					viewReleaseBtn.Show()
				} else {
					updateStatus.SetText(fmt.Sprintf("InkAnim is up to date (%s).", res.LatestVersion))
				}
				checkBtn.Enable()
			})
		})
	})

	updateSection := container.NewVBox(
		container.NewHBox(checkBtn, viewReleaseBtn),
		updateStatus,
	)

	// Links Section
	repoURL, _ := url.Parse("https://github.com/mrpoundsign/InkAnim")
	issuesURL, _ := url.Parse("https://github.com/mrpoundsign/InkAnim/issues")

	repoLink := widget.NewHyperlink("GitHub Repository", repoURL)
	issuesLink := widget.NewHyperlink("Report an Issue / Feedback", issuesURL)
	linksRow := container.NewHBox(repoLink, widget.NewSeparator(), issuesLink)

	// Open Source Licenses Button & Credits
	licensesBtn := widget.NewButton("Open Source Licenses...", func() {
		ShowLicensesDialog(parent)
	})

	licenseLabel := widget.NewLabel("MIT License • Created by mrpoundsign")

	footerRow := container.NewBorder(nil, nil, licenseLabel, licensesBtn)

	content := container.NewVBox(
		header,
		widget.NewSeparator(),
		updateSection,
		widget.NewSeparator(),
		linksRow,
		footerRow,
	)

	aboutDialog := dialog.NewCustom("About InkAnim", "Close", content, parent)
	aboutDialog.Resize(fyne.NewSize(480, 340))
	aboutDialog.Show()
}
