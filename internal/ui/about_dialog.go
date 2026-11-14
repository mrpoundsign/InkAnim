package ui

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/mrpoundsign/appinstaller/pkg/updater"

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

### DejaVu Fonts (Embedded Vector Typography)
- **License**: DejaVu Fonts License (Bitstream Vera & Arev Fonts Copyright)
- **URL**: https://dejavu-fonts.github.io
- **Copyright**: (c) 2003 by Bitstream, Inc. & (c) 2006 by Tavmjong Bah. All rights reserved.

---

### Go Standard Library
- **License**: BSD 3-Clause License
- **URL**: https://golang.org
- **Copyright**: (c) The Go Authors. All rights reserved.
`

// ShowLicensesDialog displays the third-party open source licenses modal.
func ShowLicensesDialog(parent fyne.Window, onClosed ...func()) {
	richText := widget.NewRichTextFromMarkdown(openSourceLicensesMarkdown)
	richText.Wrapping = fyne.TextWrapWord

	scroll := container.NewScroll(richText)
	scroll.SetMinSize(fyne.NewSize(500, 360))

	licensesDlg := dialog.NewCustom("Open Source Licenses", "Close", scroll, parent)
	if len(onClosed) > 0 && onClosed[0] != nil {
		licensesDlg.SetOnClosed(onClosed[0])
	}
	licensesDlg.Resize(fyne.NewSize(540, 420))
	licensesDlg.Show()
}

// ShowAboutDialog presents the application metadata, links, update checker, and licenses modal.
func ShowAboutDialog(parent fyne.Window, currentVersion string, onClosed ...func()) {
	// App Icon
	iconImg := canvas.NewImageFromResource(assets.AppIcon)
	iconImg.SetMinSize(fyne.NewSize(64, 64))
	iconImg.FillMode = canvas.ImageFillContain

	// Title & Tagline (using standard foreground for high-contrast light grey text)
	title := widget.NewLabelWithStyle("InkAnim", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	tagline := widget.NewLabel("Inkscape SVG to Animated GIF Studio")

	cleanVer := strings.TrimPrefix(strings.TrimSpace(currentVersion), "v")
	verStr := "Version: v" + cleanVer
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

	updateNowBtn := widget.NewButton("Update Now...", nil)
	updateNowBtn.Importance = widget.HighImportance
	updateNowBtn.Hide()

	var checkBtn *widget.Button
	checkBtn = widget.NewButton("Check for Updates", func() {
		checkBtn.Disable()
		updateStatus.SetText("Checking for updates...")
		updateNowBtn.Hide()

		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			checker := updater.NewChecker("mrpoundsign", "InkAnim")
			rel, isNewer, err := checker.CheckLatest(ctx, currentVersion)
			fyne.Do(func() {
				if err != nil {
					updateStatus.SetText(fmt.Sprintf("Update check failed: %v", err))
					checkBtn.Enable()
					return
				}

				if isNewer && rel != nil {
					updateStatus.SetText(fmt.Sprintf("New version v%s is available!", rel.Version))
					updateNowBtn.SetText(fmt.Sprintf("Update to v%s...", rel.Version))
					updateNowBtn.OnTapped = func() {
						showSelfUpdateDialog(parent, currentVersion, rel, checker)
					}
					updateNowBtn.Show()
				} else {
					updateStatus.SetText(fmt.Sprintf("InkAnim is up to date (v%s).", updater.NormalizeVersion(currentVersion)))
				}
				checkBtn.Enable()
			})
		}()
	})

	updateSection := container.NewVBox(
		container.NewHBox(checkBtn, updateNowBtn),
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
	if len(onClosed) > 0 && onClosed[0] != nil {
		aboutDialog.SetOnClosed(onClosed[0])
	}
	aboutDialog.Resize(fyne.NewSize(480, 340))
	aboutDialog.Show()
}
