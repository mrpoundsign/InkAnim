# InkAnim — Design Document
**Inkscape SVG to Animated GIF Studio (Go & Fyne)**

---

## 1. Executive Summary

**InkAnim** is a cross-platform desktop studio and interactive WebAssembly demo written in **Go** using the **Fyne Toolkit**. It enables designers and digital animators to convert Inkscape vector artwork into high-quality, production-ready **animated GIFs** optimized for Twitch emotes, Discord stickers, and web graphics.

Unlike legacy tools that require duplicating artwork across dozens of canvases, InkAnim powers animation through the **Inkscape Animation & Motion System (IAMS)**—a declarative object-level motion syntax that translates trajectories, rotations, scale transforms, fades, depth layering, and gradient sweeps into fluid multi-frame sequences.

Exporting focuses strictly on producing an optimized, compliant **single animated GIF file** (up to 4096x4096px), featuring an **"Export Square"** mode that centers artwork on transparent padding and a live **Twitch Multi-Scale Inspector** (emulating 112px, 56px, and 28px chat rendering side-by-side).

---

## 2. Visual Identity & Color System

The InkAnim visual brand derives directly from the studio icon (`assets/icon.svg` / `assets/icon.png`), featuring an **Obsidian, Chrome Slate & Electric Sky Blue** theme.

### 2.1 Brand Icon Anatomy
- **Tilted Canvas Sheets**: Layered semi-transparent studio sheets rotated in perspective representing frame animation.
- **Vector Tangent Handles & Anchors**: Dynamic S-curve trajectory path with Bézier control knots representing IAMS declarative object motion.
- **Hero Sparkle Core**: Brilliant Chrome White and Electric Sky Blue 4-point vector star traveling the motion path.

### 2.2 Color Tokens & Specifications

| Token | Hex / RGBA | Role / Usage |
| :--- | :--- | :--- |
| **Obsidian Deep (Base 1)** | `#020203` | Root window background, deep drop shadows |
| **Obsidian Slate (Base 2)** | `#0A0A0C` | Studio viewport canvas background |
| **Dark Charcoal (Surface 1)** | `#141416` | Panels, toolbars, modal dialogs, cards |
| **Elevated Slate (Surface 2)** | `#1E293B` | Input background, inactive tabs, subtle borders |
| **Slate Border / Grid** | `#475569` | Frame dividers, vector grid points, secondary strokes |
| **Muted Text / Guides** | `#64748B` | Guide paths, ghost trajectories, placeholder text |
| **Secondary Text / Slate** | `#94A3B8` | Subtitles, labels, metadata, secondary icons |
| **Primary Text / Chrome** | `#FFFFFF` | Core star highlight, primary headings, anchor knots |
| **Electric Sky Blue (Primary)** | `#38BDF8` | Primary action buttons, active trajectory, brand accent |
| **Deep Sky (Primary Dark)** | `#0284C7` | Active button press, gradient stop anchors |
| **Selection Tint** | `rgba(56, 189, 248, 0.25)` | Selected frames, active focus indicators |

---

## 3. Key Requirements & Features

1. **Multi-Platform GUI**:
   - Modern, lightweight desktop interface built with **Fyne v2** and compiled to WebAssembly via WebGL.
   - Obsidian & Sky Blue studio theme with live emote-testing backgrounds (Dark Mode `#18181B`, Light Mode `#FFFFFF`, Checkerboard).
2. **Animation Generation System**:
   - **IAMS Object Motion**: Compact motion directives attached to guide paths or groups (`Move {...}`, `Rot {...}`, `Scale {...}`, `Fade {...}`, `Depth {...}`, `Color {...}`). Synthesizes frames dynamically in memory without duplicating artwork.
3. **Document & Boundary Framing**:
   - **Page Boundary**: Uses the document's native page viewBox (`<svg viewBox="...">`).
   - **Drawing Boundary**: Automatically computes the tight bounding box surrounding all visible vector paths.
4. **Timeline & Playback**:
   - Interactive live animation player (Play, Pause, Step Forward/Back, Loop).
   - Global FPS control (10–30 FPS) with per-frame duration overrides (ms).
   - Dynamic frame reordering and enable/disable toggling.
5. **Export & Sizing Presets**:
   - **Twitch Emote Auto-Resize**: Single square animated GIF (up to 4096x4096px, default 512x512) centered with transparent padding.
   - **"Export Square" Mode**: Automatically takes the wider/longer dimension ($S = \max(\text{Width}, \text{Height})$) and centers the artwork within an $S \times S$ transparent canvas.
   - **Custom Sizing**: Custom width & height, scale multipliers (0.5x, 1x, 2x, 4x), up to 4096x4096px.
6. **Twitch Emote Scale Inspector (Live Emulation)**:
   - Simultaneous live playback at **112x112**, **56x56**, and **28x28** over both Twitch Dark (`#18181B`) and Twitch Light (`#FFFFFF`) backgrounds.
7. **Color & Alpha Optimization**:
   - High-fidelity palette quantization (32–256 colors) with Floyd-Steinberg dithering and clean alpha thresholding.

---

## 4. Architecture & Data Flow

```
+---------------------------------------------------------------------------------+
|                               InkAnim Application                               |
|                                                                                 |
|   +-----------------------+     +-------------------------------------------+   |
|   |  pkg/inksvg Parser    |     |              Fyne GUI Player              |   |
|   | - IAMS Motion Engine  | --> | - Frame Reorderer & Timeline              |   |
|   | - Dynamic Synthesizer |     | - Main Canvas Preview                     |   |
|   | - Boundary Calculator |     | - Twitch Inspector (112px, 56px, 28px)    |   |
|   +-----------------------+     +-------------------------------------------+   |
|               |                                       |                         |
|               v                                       v                         |
|   +-----------------------+                 +-------------------------------+   |
|   | Vector Rasterizer     | --------------> | Quantizer & GIF Encoder       |   |
|   | (Pure Go oksvg/rasterx|                 | (internal/gif + Clean Alpha)  |   |
|   +-----------------------+                 +-------------------------------+   |
|                                                               |                 |
|                                                               v                 |
|                                             +-------------------------------+   |
|                                             | Export: Square / Custom       |   |
|                                             | (Max: 4096 x 4096)            |   |
|                                             +-------------------------------+   |
+---------------------------------------------------------------------------------+
```

### 4.1 Animation Mechanics

Artwork remains in a single resting position. Motion paths or shape tags specify motion over a given frame range:
```xml
<g id="spaceship" inkscape:label="Spaceship">
  <path id="ship_art" d="..." fill="#38bdf8"/>
  <path id="flight_path" 
        inkscape:label="Move {f:1-30 ease:in-out rev orient:true}" 
        d="M 10 50 C 30 10, 70 90, 90 50" 
        style="stroke:#475569;fill:none;"/>
</g>
```
- **Parsing**: `pkg/inksvg` extracts motion directives from `inkscape:label` or element tags.
- **Synthesis**: Transforms (translation, rotation, scale, opacity, z-order) are calculated for each frame $t \in [1, N]$.
- **In-Memory Culling**: Motion guide paths are automatically excluded from the final render.

---

## 5. UI Layout Specifications

The Fyne application adopts a 3-column studio workstation layout:

```
+---------------------------------------------------------------------------------------------------------+
|  InkAnim — Inkscape SVG to Animated GIF Studio                                             [_] [O] [X]  |
+---------------------------------------------------------------------------------------------------------+
| [ Open SVG... ]  [ Reload ]  emote.svg                                                                   |
+----------------------+------------------------------------------------+---------------------------------+
| Frame Source & Order | Animation Preview & Twitch Inspector           | Export & Presets                |
|                      |                                                |                                 |
| Crop Boundary:       | +--------------------------------------------+ | Preset:                         |
| (•) Page  ( ) Drawing| |                                            | | [ Twitch Emote (Square)       ] |
|                      | |        [ MAIN LIVE CANVAS ]                | | [ Custom Dimensions          ] |
| Frames List:         | |                                            | |                                 |
| [x] 1. Frame 1 (100ms| +--------------------------------------------+ | Sizing Options:                 |
| [x] 2. Frame 2 (100ms| [ |< ] [ > Play ] [ >| ]  Loop: [x] 10 FPS   | [x] Export Square               |
| [x] 3. Frame 3 (100ms|                                                |     (Centers on max side: 512px)|
| [x] 4. Frame 4 (100ms| --- Twitch Scale Emulation Preview -----------| Width:  [ 512  ] px (Max 4096)  |
|                      | Dark Theme:   [112px]  [56px]  [28px]          | Height: [ 512  ] px (Max 4096)  |
| [Up] [Down] [Delete] | Light Theme:  [112px]  [56px]  [28px]          | [x] Lock Aspect Ratio           |
|                      |                                                |                                 |
|                      |                                                | Quality: 256 colors | Dither: FS|
|                      |                                                | Estimated Size: ~340 KB (Optim) |
|                      |                                                |                                 |
|                      |                                                | [     Export GIF...      ]      |
+----------------------+------------------------------------------------+---------------------------------+
| Status: Ready | File: emote.svg | Mode: Timeline (30 frames) | Size: 512x512                            |
+---------------------------------------------------------------------------------------------------------+
```

---

## 6. Project Structure

```
InkAnim/
├── assets/                      # Application icons and embedded graphics
│   ├── icon.svg                 # Master vector icon (Obsidian, Chrome Slate, Sky Blue)
│   ├── icon.png                 # Rendered 512x512 app icon
│   └── assets.go                # Embedded Go static resource
├── cmd/
│   ├── inkanim/                 # Desktop GUI & WebAssembly entry point
│   │   └── main.go
│   ├── inkanim-cli/             # Pure-Go headless CLI (CGO_ENABLED=0)
│   │   └── main.go
│   └── wasm-serve/              # Static preview server for WebAssembly
├── pkg/
│   └── inksvg/                  # Decoupled SVG preprocessor, IAMS parser & rasterizer
│       ├── preprocess.go        # XML preprocessor (paint-order, text-to-path, LPEs)
│       ├── parser.go            # XML parser & IAMS motion directive evaluator
│       ├── renderer.go          # Pure-Go vector rasterization into RGBA frames
│       ├── text.go              # Embedded DejaVu Sans vector font converter
│       └── types.go             # Document, Layer, MotionConfig models
├── internal/
│   ├── app/                     # Session state and frame pipeline orchestration
│   ├── gif/                     # Animated GIF encoding, quantization & Twitch checks
│   └── ui/                      # Fyne GUI components and Studio theme
├── web/                         # Web landing page and WebAssembly demo distribution
│   ├── landing/                 # Responsive landing page, samples gallery & docs
│   └── demo/                    # In-browser WebAssembly demo template
├── testdata/                    # Sample IAMS SVGs and test fixtures
├── build.sh                     # Linux/macOS build & packaging script
└── build.ps1                    # Windows PowerShell build script
```
