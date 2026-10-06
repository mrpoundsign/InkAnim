# InkAnim

**Multi-Platform Inkscape SVG to Animated GIF Studio**

[![Web Studio](https://img.shields.io/badge/Web_Studio-WebAssembly-9146ff?style=for-the-badge&logo=webassembly&logoColor=white)](https://mrpoundsign.github.io/InkAnim/)
[![GitHub Release](https://img.shields.io/github/v/release/mrpoundsign/InkAnim?style=for-the-badge&color=22c55e)](https://github.com/mrpoundsign/InkAnim/releases/latest)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg?style=for-the-badge)](LICENSE)

InkAnim is a cross-platform desktop studio, WebAssembly web app, and command-line tool built with **Go** and the **Fyne Toolkit**. It converts multi-frame vector artwork designed in **Inkscape** into single, high-fidelity **animated GIFs** optimized for Twitch emotes and Discord stickers.

---

## 🌐 Live WebAssembly Studio

Use InkAnim right now in your web browser with zero installation:  
👉 **[Launch InkAnim Web Studio (In-Browser)](https://mrpoundsign.github.io/InkAnim/)**

- **100% Client-Side Privacy**: Vector parsing and GIF quantization execute entirely inside your browser sandbox via WebAssembly. Your artwork never leaves your machine.
- **Full In-Browser Features**: Load Inkscape SVGs, configure Twitch emote specs, inspect real-time chat scaling, and download exported GIFs directly.

---

## ✨ Features

- **Precision Vector Animation Engine**:
  - **IAMS Object Motion Syntax**: Animate vector elements directly using declarative tags on object labels or descriptions: trajectory paths (`Move`), spin (`Rot`), zoom/pulsing (`Scale`), opacity transitions (`Fade`), z-stacking order (`Depth`), and gradient color sweeps (`Color`).
  - **Artboard & Multi-Page Framing**: Choose between native document page bounds, multi-page bounds, or automatic tight drawing bounds.
- **Export Square Mode**:
  - Automatically takes the wider/longer dimension of the SVG ($S = \max(\text{Width}, \text{Height})$) as the export resolution and centers the artwork with transparent padding.
  - Target resolution customizable up to the maximum Twitch limit of **4096 x 4096 px**.
- **Twitch Chat-Scale Inspector (Live Emulation)**:
  - Real-time emulation dock displaying the animation running simultaneously at **112x112**, **56x56**, and **28x28** over both **Twitch Dark (#18181B)** and **Twitch Light (#FFFFFF)** backgrounds.
  - Artists can verify line weights and small-scale readability without needing to export multiple test files.
- **Color & Alpha Channel Optimization**:
  - Palette quantization with clean alpha thresholding and background disposal to eliminate dark/light halo fringes.
- **Twitch Emote Compliance Engine**:
  - Live checks for square aspect ratio (1:1), maximum 60 frames, and recommended animation duration.
- **Zero-CGo Cross-Compilable CLI**:
  - In addition to the desktop GUI, a headless CLI tool (`inkanim-cli`) compiles with `CGO_ENABLED=0` to any operating system (Windows, macOS, Linux) without requiring C compilers or Docker.

---

## 🚀 Quick Start & Build Scripts

### Convenient Build Scripts
You can build, test, and package everything with a single command:

**On Windows (PowerShell):**
```powershell
.\build.ps1                # Run tests and build both GUI (inkanim.exe) and CLI (inkanim-cli.exe)
.\build.ps1 -Target gui    # Build Desktop GUI only (automatically uses Zig CGo)
.\build.ps1 -Target cli    # Build CLI only (Pure-Go, Zero CGo)
.\build.ps1 -Target wasm   # Package WebAssembly studio & landing page into build/gh-pages
.\build.ps1 -Target serve  # Run local preview server at http://localhost:8080
.\build.ps1 -Target test   # Run unit tests
.\build.ps1 -Target cross  # Cross-compile CLI for Windows, Linux, and macOS into dist/
.\build.ps1 -Target check  # Validate GoReleaser configuration
.\build.ps1 -Target clean  # Remove built binaries and dist/
```

**On Linux / macOS (Bash):**
```bash
./build.sh                 # Run tests and build both GUI and CLI
./build.sh cli             # Build CLI only
./build.sh gui             # Build GUI only
./build.sh wasm            # Package WebAssembly studio & landing page into build/gh-pages
./build.sh serve           # Run local preview server at http://localhost:8080
./build.sh cross           # Cross-compile CLI for all targets
./build.sh test            # Run unit tests
./build.sh lint            # Run golangci-lint
```

---

### 1. Pure-Go CLI (`inkanim-cli`)

Compile instantly on any platform:
```bash
go build ./cmd/inkanim-cli
```

Convert an SVG with **IAMS Motion Syntax** to a 512x512 square animated GIF:
```bash
inkanim-cli -i star_swirl.svg -o emote.gif -square -size 512 -fps 15
```

Convert with dithering and ping-pong loop:
```bash
inkanim-cli -i pendulum.svg -o anim.gif -square -fps 12 -dither -pingpong
```

CLI options:
```text
  -i string
        Input Inkscape SVG file path (required)
  -o string
        Output animated GIF file path (default: input with .gif extension)
  -square
        Export Square: center graphic on max(width, height) with transparent padding (default: true)
  -size int
        Target square size (e.g. 512, max 4096). 0 uses max(width, height)
  -width int
        Custom target width (if not using square mode)
  -height int
        Custom target height (if not using square mode)
  -fps int
        Frames per second (default: 10)
  -colors int
        Max palette colors: 2-256 (default: 256)
  -dither
        Apply Floyd-Steinberg dithering (default: false)
  -check-twitch
        Validate output against Twitch animated emote specifications (default: true)
```

---

### 2. Desktop GUI (`inkanim`)

Run locally with Fyne:
```bash
go run ./cmd/inkanim
```

Drag and drop any Inkscape SVG into the window to immediately preview the animation timeline, inspect Twitch chat scaling, and export.

---

## 📦 Cross-Compilation

InkAnim uses a two-pronged cross-platform architecture:

### Multi-Platform Desktop GUI (`fyne-cross`)
Cross-compile the desktop application with native windowing for Windows, macOS, and Linux using [`fyne-cross`](https://github.com/fyne-io/fyne-cross):

```bash
# Install fyne-cross
go install github.com/fyne-io/fyne-cross@latest

# Cross-compile for Windows (.exe)
fyne-cross windows -arch=amd64,arm64 ./cmd/inkanim

# Cross-compile for macOS (.app / .dmg)
fyne-cross darwin -arch=amd64,arm64 ./cmd/inkanim

# Cross-compile for Linux (.deb / .tar.xz)
fyne-cross linux -arch=amd64,arm64 ./cmd/inkanim
```

Automated builds are also configured in `.github/workflows/release.yml`.

### Pure-Go Headless CLI (Instant Cross-Compilation)
Because the core vector rasterizer, SVG parser, and GIF quantizer are written in pure Go, the CLI cross-compiles without CGo:
```bash
GOOS=linux   GOARCH=amd64 CGO_ENABLED=0 go build -o inkanim-linux   ./cmd/inkanim-cli
GOOS=darwin  GOARCH=arm64 CGO_ENABLED=0 go build -o inkanim-mac-m1 ./cmd/inkanim-cli
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o inkanim-win.exe ./cmd/inkanim-cli
```

---

## 🧪 Testing

Run all unit tests:
```bash
go test ./pkg/inksvg ./internal/... -v
```

---

## 📄 License
MIT
