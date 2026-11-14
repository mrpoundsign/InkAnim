# InkAnim

**Multi-Platform Inkscape SVG to Animated GIF Studio**

[![Documentation](https://img.shields.io/badge/Docs-mrpoundsign.com%2FInkAnim-38bdf8?style=for-the-badge)](https://mrpoundsign.com/InkAnim/)
[![GitHub Release](https://img.shields.io/github/v/release/mrpoundsign/InkAnim?style=for-the-badge&color=22c55e)](https://github.com/mrpoundsign/InkAnim/releases/latest)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg?style=for-the-badge)](LICENSE)

InkAnim is a cross-platform native **Desktop Studio** built with **Go** and the **Fyne Toolkit**. It converts multi-frame and IAMS-animated vector artwork designed in **Inkscape** into high-fidelity **animated GIFs** optimized for Twitch emotes, Discord stickers, and the web.

---

## ✨ Features

- **Visual Animation Timeline & Canvas Gizmos**:
  - Edit motion trajectories, rotation/scale pivots, placement handles, and 1:1 Inkscape layer/group/shape tracks interactively inside the studio with full undo/redo history.
- **`.inka` Multi-SVG Project Workflow**:
  - Link one or more external Inkscape SVGs non-destructively into a single `.inka` project file with live file watching and automatic reload as you iterate in Inkscape.
- **Precision Vector Animation Engine (IAMS)**:
  - Supports trajectory paths (`Move`), spin (`Rot`), zoom/pulsing (`Scale`), opacity transitions (`Fade`), z-stacking order (`Depth`), visibility (`Show`/`Hide`), camera/parallax (`Camera`/`Dist`), and gradient color sweeps (`Color`).
  - Choose between native document page bounds or automatic tight drawing bounds.
- **Export Square Mode**:
  - Automatically centers artwork on a 1:1 square canvas with transparent padding for Twitch & Discord emote requirements, with target resolution customizable up to **4096 × 4096 px**.
- **Twitch Chat-Scale Inspector (Live Emulation)**:
  - Real-time emulation dock displaying your animation simultaneously at **112×112**, **56×56**, and **28×28** over both **Twitch Dark (`#18181B`)** and **Twitch Light (`#FFFFFF`)** backgrounds.
- **Color & Alpha Channel Optimization**:
  - Palette quantization (2 to 256 colors) with ordered Bayer 8×8 dithering, clean alpha thresholding, and Twitch 1.0 MB / 60-frame compliance checks.

---

## 🚀 Quick Start & Build Scripts

### Build & Development Commands

**On Windows (PowerShell):**
```powershell
.\build.ps1                # Run lint (go fix -diff + golangci-lint-v2), tests, and build Desktop Studio -> build\bin\inkanim.exe
.\build.ps1 -Target gui    # Build Desktop Studio GUI only (automatically uses Zig CGo) -> build\bin\inkanim.exe
.\build.ps1 -Target web    # Assemble static GitHub Pages site -> build\web\
.\build.ps1 -Target test   # Run all unit tests
.\build.ps1 -Target lint   # Run go fix -diff and golangci-lint-v2
.\build.ps1 -Target check  # Validate GoReleaser configuration
.\build.ps1 -Target clean  # Clean build artifacts
```

**On Linux / macOS (Bash):**
```bash
./build.sh                 # Run lint, tests, and build Desktop Studio -> build/bin/inkanim
./build.sh gui             # Build Desktop Studio GUI into build/bin/inkanim
./build.sh gui-win         # Cross-compile Windows Desktop Studio GUI via MinGW into build/bin/inkanim.exe
./build.sh web             # Assemble static GitHub Pages site into build/web/
./build.sh test            # Run unit tests
./build.sh lint            # Run go fix -diff and golangci-lint-v2
./build.sh clean           # Clean build artifacts
```

### Run Locally from Source

```bash
go run ./cmd/inkanim
```

Drag and drop any Inkscape `.svg` or `.inka` project into the window to inspect tracks, tweak motion actions and canvas gizmos, preview Twitch chat scaling, and export an optimized GIF.

---

## 📦 Release Packaging

Cross-platform Desktop Studio binaries are built automatically in `.github/workflows/release.yml` using `fyne package`:
- Windows (`amd64` `.exe`)
- macOS (`darwin_all` `.app` bundle archive)
- Linux (`amd64` `.tar.xz`)
- Consolidated SHA-256 checksums (`checksums.txt` / `SHA256SUMS.txt`)

Static documentation and sample showcase pages are deployed to GitHub Pages (`https://mrpoundsign.com/InkAnim/`) via `.github/workflows/pages.yml`.

---

## 🧪 Testing

Run all unit tests:
```bash
go test ./internal/... ./pkg/... -v
```

---

## 📄 License
MIT
