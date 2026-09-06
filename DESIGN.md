# InkAnim — Design Document
**Inkscape SVG to Animated GIF Tool (Go & Fyne)**

---

## 1. Executive Summary

**InkAnim** is a cross-platform desktop tool written in **Go** using the **Fyne Toolkit**. It allows designers and digital artists to turn multi-frame Inkscape vector artwork into a single, high-quality **animated GIF**. The tool natively understands Inkscape's SVG structure—specifically supporting animation frames defined either across **layers** or across **pages** (Inkscape 1.2+ multi-page feature).

Exporting is strictly focused on producing a **single animated GIF file** (up to 4096x4096px), with a dedicated **"Export Square"** mode that centers non-square artwork within a square canvas based on the source's wider dimension. To help Twitch emote artists verify that their animation scales cleanly down to chat sizes without needing to export multiple files, the app features a real-time **Twitch Multi-Scale Inspector** (emulating 112px, 56px, and 28px chat rendering side-by-side).

---

## 2. Key Requirements & Features

1. **Multi-Platform GUI**:
   - Modern, lightweight desktop interface built with **Fyne v2**.
   - Dark/Light mode theme with emote-testing backgrounds (Dark Mode, Light Mode, Checkerboard).
2. **Animation Frame Sources**:
   - **Layers Mode**: Treats Inkscape layers (`inkscape:groupmode="layer"`) as discrete animation frames. Supports solitary frames, cumulative frames, or pinned background/foreground layers.
   - **Pages Mode**: Treats Inkscape 1.2+ multi-page documents as frames by isolating each `<inkscape:page>` viewBox.
3. **Timeline & Playback**:
   - Live interactive animation player (Play, Pause, Step, Loop).
   - Per-frame delay adjustments (milliseconds) or global FPS control.
   - Frame reordering (Move up/down, enable/disable frame).
4. **Export & Sizing Presets**:
   - **Twitch Emote Auto-Resize**: Exports a single square animated GIF (up to 4096x4096px, default 512x512 or 1024x1024) meeting Twitch's <1MB requirement.
   - **"Export Square" Mode**: Automatically takes the wider/longer dimension of the source SVG ($S = \max(\text{Width}, \text{Height})$) as the square output resolution, centering the artwork with transparent padding.
   - **Custom Sizing**: Custom width & height, scale multipliers (0.5x, 1x, 2x, 4x), and max cap up to 4096x4096px.
5. **Twitch Emote Scale Inspector (Live Preview Emulation)**:
   - Artists do not need to export 3 files, but need to see how Twitch downscales their emote.
   - The UI includes a live multi-scale preview bar showing the animation simultaneously running at **112x112**, **56x56**, and **28x28** over both Twitch Dark (`#18181B`) and Twitch Light (`#FFFFFF`) backgrounds.
6. **Color & Alpha Optimization**:
   - High-fidelity palette quantization with clean transparency matte control (eliminating dark/white fringe halos).

---

## 3. Architecture & Data Flow

```
+---------------------------------------------------------------------------------+
|                               InkAnim Application                               |
|                                                                                 |
|   +-----------------------+     +-------------------------------------------+   |
|   |   SVG Parser Engine   |     |              Fyne GUI Player              |   |
|   | - Layer Extractor     | --> | - Frame Reorderer & Timeline              |   |
|   | - Page Extractor      |     | - Main Canvas Preview                     |   |
|   +-----------------------+     | - Twitch Inspector (112px, 56px, 28px)    |   |
|               |                 +-------------------------------------------+   |
|               v                                       |                         |
|   +-----------------------+                           v                         |
|   | Vector Rasterizer     | --------------> +-------------------------------+   |
|   | (Pure Go oksvg/rasterx|                 | Quantizer & GIF Encoder       |   |
|   +-----------------------+                 | (image/gif + Clean Alpha Matte|   |
|                                             +-------------------------------+   |
|                                                               |                 |
|                                                               v                 |
|                                             +-------------------------------+   |
|                                             | Export: Square / Custom       |   |
|                                             | (Max: 4096 x 4096)            |   |
|                                             +-------------------------------+   |
+---------------------------------------------------------------------------------+
```

### 3.1 Frame Extraction Mechanics

#### A. Layers Mode
Inkscape labels layers with specific XML attributes:
```xml
<g inkscape:groupmode="layer" id="layer_run_1" inkscape:label="Frame 1" style="display:inline">
  <!-- Frame 1 vector graphics -->
</g>
<g inkscape:groupmode="layer" id="layer_run_2" inkscape:label="Frame 2" style="display:none">
  <!-- Frame 2 vector graphics -->
</g>
```
- **Parsing**: The parser scans for all `<g>` tags possessing `inkscape:groupmode="layer"`.
- **Frame Construction**: For frame $i$, the engine clones the SVG DOM, sets layer $i$ to `display:inline`, and hides all other animation layers (unless designated as persistent background or foreground).

#### B. Pages Mode
Inkscape 1.2 introduced multiple pages:
```xml
<sodipodi:namedview ...>
  <inkscape:page x="0" y="0" width="100" height="100" id="page1" inkscape:label="Frame 1" />
  <inkscape:page x="120" y="0" width="100" height="100" id="page2" inkscape:label="Frame 2" />
</sodipodi:namedview>
```
- **Parsing**: The parser reads `<inkscape:page>` elements to extract individual bounding boxes $(x, y, w, h)$.
- **Frame Construction**: For page $j$, the SVG's root `viewBox` is dynamically rewritten to `x y width height`, effectively isolating that page's artboard for rasterization.

---

## 4. UI Layout Specifications

The Fyne application adopts a standard 3-column studio workstation layout:

```
+---------------------------------------------------------------------------------------------------------+
|  InkAnim - Inkscape SVG to Animated GIF                                                    [_] [O] [X]  |
+---------------------------------------------------------------------------------------------------------+
| [ Load SVG File... ]  /  or Drag & Drop SVG here                                                        |
+----------------------+------------------------------------------------+---------------------------------+
| Frame Source & Order | Animation Preview & Twitch Inspector           | Export & Presets                |
|                      |                                                |                                 |
| Mode:                | +--------------------------------------------+ | Preset:                         |
| (•) Layers  ( ) Pages| |                                            | | [ Twitch Emote (Square)       ] |
|                      | |        [ MAIN LIVE CANVAS ]                | | [ Custom Dimensions          ] |
| Frames List:         | |                                            | |                                 |
| [x] 1. Layer 1 (100ms| +--------------------------------------------+ | Sizing Options:                 |
| [x] 2. Layer 2 (100ms| [ |< ] [ > Play ] [ >| ]  Loop: [x] 10 FPS   | [x] Export Square               |
| [x] 3. Layer 3 (100ms|                                                |     (Centers on max side: 512px)|
| [ ] 4. Background(pin| --- Twitch Scale Emulation Preview -----------| Width:  [ 512  ] px (Max 4096)  |
|                      | Dark Theme:   [112px]  [56px]  [28px]          | Height: [ 512  ] px (Max 4096)  |
| [Up] [Down] [Delete] | Light Theme:  [112px]  [56px]  [28px]          | [x] Lock Aspect Ratio           |
|                      |                                                |                                 |
|                      |                                                | Quality: 256 colors | Dither: FS|
|                      |                                                | Estimated Size: ~340 KB (<1 MB) |
|                      |                                                |                                 |
|                      |                                                | [     Export GIF...      ]      |
+----------------------+------------------------------------------------+---------------------------------+
| Status: Ready | File: emote.svg | Resolution: 320x512 -> Square 512x512       | [======= Progress ======]       |
+---------------------------------------------------------------------------------------------------------+
```

### 4.1 Component Details
1. **Left Sidebar — Frame Management**:
   - File Open button & drag-and-drop listener.
   - Mode Selector: Radio buttons for `[•] Layers` vs `[ ] Pages`.
   - Reorderable frame list showing thumbnail, label, and duration input (ms).
   - "Pin as Background" toggle per layer.
2. **Center Stage — Animation Player & Twitch Emulation**:
   - High-DPI canvas preview showing live playback at full workspace size.
   - Playback bar: `[Play/Pause]`, `[Step Back]`, `[Step Forward]`, `[Loop]`, and FPS slider.
   - **Twitch Scale Emulation Dock**:
     - Real-time downscaled instances running at **112x112**, **56x56**, and **28x28**.
     - Side-by-side Dark Theme (`#18181B`) and Light Theme (`#FFFFFF`) swatches so the creator can immediately verify readability at chat scale.
3. **Right Sidebar — Export & Optimization**:
   - Preset selector:
     - *Twitch Emote (Auto-Resize Square, up to 4096x4096)*
     - *Discord Emote (128x128)*
     - *Custom Dimensions (1x1 to 4096x4096)*
   - **Export Square Checkbox**:
     - When checked: Automatically calculates $S = \max(\text{Width}_{\text{source}}, \text{Height}_{\text{source}})$ and centers the artwork within an $S \times S$ transparent canvas.
     - Target resolution override allows scaling to any square dimension up to 4096x4096px.
   - Quality controls: Color reduction (32-256), dither algorithm, alpha threshold.
   - Twitch 1MB file size safeguard with live size estimation.
   - "Export GIF..." button with file destination selector and progress bar.

---

## 5. Cross-Compilation & Packaging Strategy

Cross-compiling CGo desktop applications with OpenGL/GLFW dependencies across Windows, macOS, and Linux requires a deliberate strategy. InkAnim adopts a **two-pronged cross-platform architecture**:

### 5.1 Official Multi-Platform GUI Builds (`fyne-cross`)
The Fyne community provides [`fyne-cross`](https://github.com/fyne-io/fyne-cross), a Docker/Podman-backed cross-compilation tool that bundles pre-configured cross-compilers, SDKs, and graphics libraries.

- **Targets Supported**:
  - Windows: `fyne-cross windows -arch=amd64,arm64`
  - Linux: `fyne-cross linux -arch=amd64,arm64`
  - macOS: `fyne-cross darwin -arch=amd64,arm64` (Universal binaries)
- **CI/CD Automation (GitHub Actions)**:
  - A standardized workflow using `fyne-io/fyne-cross-action` to automatically build signed/zipped releases for Windows (.exe / .zip), macOS (.app / .dmg), and Linux (.tar.xz / .deb) on every tag.

### 5.2 Decoupled Core & Pure-Go CLI (`CGO_ENABLED=0`)
To guarantee that the core conversion engine can be cross-compiled **instantly without any CGo toolchain**:
- All SVG parsing, layer/page extraction, rasterization, and GIF encoding logic reside in pure Go packages (`internal/svg`, `internal/gif`, `pkg/inkanim`).
- **`cmd/inkanim-cli`**: A headless command-line interface that compiles with `CGO_ENABLED=0`:
  ```bash
  # Instant cross-compilation without C toolchains or Docker:
  GOOS=linux   GOARCH=amd64 CGO_ENABLED=0 go build -o inkanim-linux   ./cmd/inkanim-cli
  GOOS=darwin  GOARCH=arm64 CGO_ENABLED=0 go build -o inkanim-mac-m1 ./cmd/inkanim-cli
  GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o inkanim-win.exe ./cmd/inkanim-cli
  ```
  This is ideal for automated asset pipelines, bot scripts, and CI runners.
- **`cmd/inkanim`**: The rich Fyne v2 desktop application, built for the developer's native system locally or cross-compiled via `fyne-cross` for releases.

---

## 6. Directory Structure

```
InkAnim/
├── .github/
│   └── workflows/
│       └── release.yml          # GitHub Actions fyne-cross multi-platform release
├── cmd/
│   ├── inkanim/
│   │   └── main.go              # Fyne Desktop GUI application
│   └── inkanim-cli/
│       └── main.go              # Pure Go headless CLI (zero-CGo cross-compile)
├── internal/
│   ├── app/
│   │   ├── app.go               # Shared controller & session state
│   │   └── config.go            # User presets & configuration
│   ├── svg/
│   │   ├── parser.go            # Inkscape XML parser for layers and pages
│   │   ├── layer_extractor.go   # Layer tree isolation & DOM visibility builder
│   │   ├── page_extractor.go    # Inkscape 1.2+ multi-page viewBox calculator
│   │   └── renderer.go          # Pure Go vector rasterizer (oksvg / rasterx)
│   ├── gif/
│   │   ├── encoder.go           # Animated GIF sequence encoder (single GIF output)
│   │   ├── quantizer.go         # Palette quantization & alpha thresholding
│   │   └── twitch.go            # Twitch format validations (<1MB, square, max 60 frames)
│   └── ui/
│       ├── main_window.go       # Main Fyne window layout
│       ├── left_frames.go       # Frame management pane
│       ├── center_preview.go    # Canvas player pane
│       ├── right_export.go      # Export options pane
│       └── theme.go             # Twitch studio theme
├── docs/
│   └── DESIGN.md                # This document
├── FyneApp.toml                 # Fyne application metadata & icon definitions
├── go.mod
└── README.md
```
