# InkAnim — Agent & Project Knowledge Base (AGENTS.md)

## 1. User Preferences & Working Rules

> [!IMPORTANT]
> Always adhere strictly to these core working preferences:
> 1. **True Pair Programming Assistant (No "Vibe Coding")**: The user is an experienced, knowledgeable developer and lead. Never act like an autonomous black box. When investigating an issue or unexpected behavior, always communicate technical findings, underlying root causes, and mechanics to the user *first*. Discuss and align before creating GitHub issues or making code changes.
> 2. **Feedback & Explicit Approval on Decisions**: Always present clear options, findings, and proposed designs, then wait for user approval before moving forward. Never jump ahead to file tickets, create branches, or write code based on assumptions. Once a specific plan is approved, executing that agreed plan autonomously is expected, but any new findings, deviations, or decisions must pause for communication and approval.
> 3. **One ticket at a time**: We work on exactly one issue/ticket at a time unless explicitly directed otherwise. Exactly **1 commit per issue**.
> 4. **User tests GUI before committing**: Always stop and let the user manually test the GUI before any changes are committed to git.
> 5. **Prioritize Built-in Agent Tools Over Shelling Out**: Always use built-in tools (`grep_search`, `list_dir`, `view_file`, and Go MCP tools) for searching code, finding files, and inspecting contents. Do not shell out to PowerShell commands (e.g., `Select-String`, `grep`, `dir`, `cat`) when built-in tools are capable of performing the search or inspection. Only shell out via `run_command` when strictly necessary (e.g., git operations, building/running code, tests, or `gh` CLI).

---

## 2. Project Overview & Mission
**InkAnim** is an open-source, desktop and WebAssembly studio application written in Go that transforms layered or multi-page **Inkscape SVGs into optimized, production-ready animated GIFs**. It is especially tailored for Twitch streamers, emote creators, and web animators.

- **Primary Repository**: `mrpoundsign/InkAnim`
- **Stack**: Go 1.27.0, [Fyne v2](https://fyne.io/) (v2.8.1 GUI toolkit with WebGL/WASM support), standard library imaging/gif with Floyd-Steinberg dithering and neural/median-cut color quantization.
- **Targets**:
  - Native Desktop (Windows `inkanim.exe`, Linux)
  - WebAssembly (in-browser canvas via `fyne package -os web` / `fyne serve`)
  - Headless CLI (`inkanim-cli`)
  - *(Note: macOS/Darwin builds are explicitly omitted).*

---

## 3. Architecture & Codebase Map

```
InkAnim/
├── cmd/
│   ├── inkanim/          # Primary GUI desktop & WASM entry point (main.go)
│   └── inkanim-cli/      # Headless CLI for batch processing & automated export
├── pkg/
│   └── inksvg/           # Decoupled, pure-Go Inkscape SVG preprocessor, parser & rasterizer
│       ├── preprocess.go # XML stream preprocessor (paint-order, text-to-path, LPE, rect rx/ry)
│       ├── parser.go     # SVG XML parser (inkscape:groupmode="layer", sodipodi, pages)
│       ├── renderer.go   # Rasterization of SVG layers into RGBA frame images
│       ├── text.go       # Embedded DejaVu Sans & TrueType glyph outline converter
│       └── types.go      # Layer, Page, and Document models
├── internal/
│   ├── app/              # Core application session, layer state, mode, orchestration
│   │   ├── session.go    # Session state: Document, Layers, Mode, Duration, ExportOptions
│   │   └── session_test.go
│   ├── gif/              # GIF compilation, palette generation, twitch validation
│   │   ├── encoder.go    # Animated GIF encoding (WriteGIFToFile, WriteGIFToWriter)
│   │   ├── palette.go    # Color quantization, Floyd-Steinberg dithering
│   │   └── twitch.go     # Twitch emote specifications & validation checks
│   └── ui/               # Fyne GUI components
│       ├── main_window.go    # Top header, window layout, drag-and-drop, file loading
│       ├── left_frames.go    # Animation frames list, mode toggle (Layers/Pages), speed, pin BG
│       ├── center_preview.go # Animation playback engine, canvas preview, twitch scale preview
│       ├── right_export.go   # Export configuration (Twitch presets, square sizing, palette)
│       ├── theme.go          # Dark Twitch studio aesthetic styling
│       └── ui_test.go        # Headless Fyne UI unit tests
├── testdata/             # Sample multi-layer and multi-page Inkscape SVGs
├── wasm/                 # Generated WebAssembly artifacts (index.html, inkanim.wasm, etc.)
└── .git/hooks/pre-push   # Pre-push hook running `go test ./...`
```

---

## 4. Issue Tracking & Backlog

Active issues and feature requests are tracked exclusively via **[GitHub Issues](https://github.com/mrpoundsign/InkAnim/issues)** (the single source of truth):
- **Inspect Open Issues**: `gh issue list --state open`
- **View Specific Issue Details**: `gh issue view <issue-number>`
- **Historical Completed Work**: Documented in `CHANGELOG.md` with release tags.

---

## 5. WebAssembly (WASM) Findings

### Why "Open SVG" and "Export GIF" don't work out-of-the-box in WASM
- Fyne v2.8.1 includes a Web driver, but file dialogs are **unimplemented upstream**:
  In `fyne.io/fyne/v2/dialog/file_wasm.go`:
  ```go
  func fileOpenOSOverride(f *FileDialog) bool {
      // TODO #2737
      return true
  }
  func fileSaveOSOverride(f *FileDialog) bool {
      // TODO #2738
      return true
  }
  ```
  Returning `true` causes Fyne to silently drop the request without rendering any file picker.
- **Solution for WASM (Implemented in `internal/ui/file_dialog_wasm.go`)**:
  - Implemented a web-specific file open bridge using `syscall/js` that triggers a browser `<input type="file" accept=".svg">`, reads the selected file using `FileReader`, and passes bytes to `mw.loadData(bytes, filename)`.
  - Implemented an export bridge using `syscall/js` that converts the encoded GIF bytes into a `Blob` and triggers a browser download via a temporary `<a>` element.
  - Implemented browser window drag-and-drop (`dragover`/`drop` listeners on `document`) to load SVGs directly when dropped into the canvas.
  - Implemented programmatic sample loader bridge `window.inkanimLoadBytes(filename, uint8Array)` for instant sample loading in web demos.

### GitHub Pages & WebAssembly Deployment
- **Model**: Automated modern GitHub Pages deployment (artifact-based, zero extra branches) managed by `.github/workflows/pages.yml`.
- **Packaging**: `./build.sh wasm` (or `.\build.ps1 -Target wasm`) compiles the app into `build/gh-pages/demo/`, assembles the project landing page into `build/gh-pages/`, and copies sample assets into `build/gh-pages/samples/`.
- **Local Testing**: Run `./build.sh serve` (or `.\build.ps1 -Target serve`) to launch a local Go static server with `application/wasm` MIME support at `http://localhost:8080`.

---

## 6. Testing & Quality Requirements

1. **Mandatory Pre-Push Checks**:
   - Install and keep `.git/hooks/pre-push` active.
   - Run linter (`golangci-lint-v2`) and tests before pushing:
     ```bash
     ./build.sh lint && ./build.sh test
     # or
     .git/hooks/pre-push
     # or on Windows PowerShell:
     .\build.ps1 -Target lint; .\build.ps1 -Target test
     ```
2. **Mandatory GUI Verification**:
   - Never commit code until the user has tested and confirmed the GUI works as expected.
3. **Target Exclusions**:
   - macOS/Darwin builds must **never** be generated or included in release workflows.
4. **Fyne GUI Thread Safety**:
   - In Fyne v2, all UI mutations (such as `label.SetText()`, `button.Enable()`, `widget.Show()`, or canvas refreshes) triggered from background goroutines, tickers, or asynchronous callbacks **must** be dispatched on the main render thread via `fyne.Do(func() { ... })` or `fyne.DoAndWait(...)`.
   - Never mutate Fyne widget state directly from background threads; doing so triggers runtime thread-safety warnings (`*** Error in Fyne call thread, this should have been called in fyne.Do[AndWait] ***`) and risks race conditions.

---

## 7. SVG In-Memory Pre-Processing Architecture & Findings

### Why In-Memory Pre-Processing (`PreprocessSVG`) is Required
`oksvg` is a lightweight SVG vector rasterizer with notable upstream limitations:
1. **No Text Rendering**: `<text>` and `<tspan>` elements are completely unhandled and silently skipped.
2. **No `paint-order` Support**: SVG standard `paint-order: stroke fill` (strokes rendered under fills) is ignored; `oksvg` always renders fills first, then strokes on top.
3. **Unscaled Stroke Width in Transformed Groups**: When shapes are inside `<g transform="...">`, `oksvg` applies the CTM to path coordinates but leaves `stroke-width` unscaled, producing strokes that are too thick or too thin under scaling transforms.
4. **Omitted `rx` on `<rect>`**: If a `<rect>` defines `ry` but omits `rx`, `oksvg` fails to default `rx = ry` per SVG spec, rendering square 90° corners instead of rounded arcs.
5. **No Inkscape LPE Support**: Paths referencing unbaked Live Path Effects (e.g. `fillet_chamfer` for smooth rounded polygon corners) render as raw sharp vertices.

### The Architectural Pattern
**Never modify or destructively flatten SVG files on disk.** Instead, [`PreprocessSVG`](pkg/inksvg/preprocess.go) intercepts the XML token stream in memory before passing it to `oksvg`:
- **Stage 1: Font & Element Conversion (Issue #21)**: Convert `<text>` and `<tspan>` elements into `<path>` vector glyph contours early in the pipeline.
- **Stage 2: LPE Evaluation**: Evaluate `fillet_chamfer` LPEs on `<path>` elements referencing `<inkscape:path-effect>`.
- **Stage 3: Rect `rx`/`ry` Normalization**: Mirror `rx` and `ry` when either is omitted.
- **Stage 4: Group Transform Stroke Scaling**: Track 2D affine transformation matrices across nested `<g>` tags and scale `stroke-width` by cumulative ancestor scale factors.
- **Stage 5: Paint-Order Desugaring**: Desugar `paint-order: stroke fill` into consecutive stroke-only and fill-only elements.

### Critical Learnings for Issue #21 (Text-to-Path Converter)
1. **Pipeline Ordering**: Converting `<text>` to `<path>` early in `PreprocessSVG` allows generated text paths to automatically benefit from ancestor transform stroke scaling and `paint-order: stroke fill` desugaring. (In `testdata/Alert Icon.svg`, `text1` uses `paint-order: stroke fill markers` with a 13.4px stroke!).
2. **Y-Axis Inversion in Glyph Contours**: Font formats (`sfnt`, TrueType/OpenType) place the origin at the baseline with positive Y pointing UP towards the ascender, whereas SVG Y points DOWN. Coordinates must be inverted: `svgY = baselineY - (glyphY * fontSize / unitsPerEm)`.
3. **Inkscape Flowed Text (`shape-inside`)**: Inkscape 1.0+ flowed text objects (`shape-inside:url(...)`) typically embed explicit baseline anchors on `<tspan x="..." y="...">`. Using `<tspan>` coordinates avoids the need for a complex text-wrapping engine for simple labels.
4. **WASM / Cross-Platform Font Fallbacks**: System font paths (`C:\Windows\Fonts`, `/usr/share/fonts`) are inaccessible in WebAssembly. The converter embeds official DejaVu Sans TTFs (`pkg/inksvg/fonts/`) to ensure deterministic, zero-dependency cross-platform rendering.

### Future Modularization Roadmap (Issue #50)
The in-memory SVG preprocessor and rasterizer has been extracted from `internal/svg` into a decoupled, pure-Go public package (`pkg/inksvg`) with zero GUI/Fyne or internal dependencies (Phase 1, Issue #50). Once a companion project (e.g. the 60-frame flat motion tool) or external consumers spin up, it can be seamlessly extracted into a standalone GitHub repository (`github.com/mrpoundsign/inksvg`) without modifying package internals.

