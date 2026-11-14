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
**InkAnim** is an open-source native desktop studio written in Go that transforms **Inkscape SVGs** and `.inka` projects into optimized, production-ready animated GIFs using visual timeline keyframes, canvas gizmos, and declarative object motion syntax (IAMS). It is especially tailored for Twitch streamers, emote creators, and web animators.

- **Primary Repository**: `mrpoundsign/InkAnim`
- **Visual Identity**: Obsidian Slate, Chrome White & Electric Sky Blue (`#38BDF8`) theme derived from the studio icon.
- **Stack**: Go 1.27.0, [Fyne v2](https://fyne.io/) (v2.8.1 native Desktop GUI toolkit), standard library `image/gif` with ordered Bayer 8×8 dithering and 15-bit color quantization.
- **Targets**:
  - Native Desktop Studio: Windows (`build/bin/inkanim.exe`), Linux (`build/bin/inkanim`), macOS (`InkAnim.app`)
  - Static Documentation & Showcase Site: GitHub Pages at `https://mrpoundsign.com/InkAnim/`

---

## 3. Architecture & Codebase Map

```
InkAnim/
├── cmd/
│   └── inkanim/          # Primary native Desktop Studio entry point (main.go)
├── pkg/
│   └── inksvg/           # Decoupled, pure-Go Inkscape SVG preprocessor, IAMS parser & rasterizer
│       ├── preprocess.go # XML stream preprocessor (paint-order, text-to-path, LPE, rect rx/ry)
│       ├── parser.go     # SVG XML parser & IAMS motion directive evaluator
│       ├── renderer.go   # Rasterization of SVG frames into RGBA images
│       ├── text.go       # Embedded DejaVu Sans & TrueType glyph outline converter
│       └── types.go      # Frame, Document, and MotionConfig models
├── internal/
│   ├── app/              # Core application session, .inka project model, timeline state, boundary mode
│   ├── gif/              # GIF compilation, palette generation, Twitch validation
│   │   ├── encoder.go    # Animated GIF encoding (WriteGIFToFile, WriteGIFToWriter)
│   │   ├── quantizer.go  # Color quantization, Bayer 8×8 dithering, palette generation
│   │   └── twitch.go     # Twitch emote specifications & validation checks
│   └── ui/               # Fyne Desktop Studio GUI components
│       ├── main_window.go    # Top header, menus, .inka project workflow, drag-and-drop, file loading
│       ├── timeline_panel.go # Seconds-first animation timeline, 1:1 Inkscape hierarchy, action modals
│       ├── canvas_gizmo.go   # Interactive canvas spline, placement, and pivot gizmos
│       ├── center_preview.go # Animation playback engine, canvas preview, Twitch scale preview
│       ├── right_export.go   # Export configuration (Twitch presets, square sizing, palette)
│       ├── editor_panel.go   # Studio undo/redo shortcuts, debounced preview updates, node management
│       ├── editor_state.go   # SVG DOM state, undo/redo snapshots, element deletion
│       ├── theme.go          # Obsidian & Electric Sky Blue studio theme
│       └── ui_test.go        # Headless Fyne UI unit tests
├── testdata/             # Sample IAMS motion SVGs and fixtures
├── web/                  # Static landing page (web/landing/), docs, and sample showcase (web/samples/)
└── .git/hooks/           # pre-commit (go fix -diff & golangci-lint-v2) and pre-push (go test)
```

---

## 4. Issue Tracking & Backlog

Active issues and feature requests are tracked exclusively via **[GitHub Issues](https://github.com/mrpoundsign/InkAnim/issues)** (the single source of truth):
- **Inspect Open Issues**: `gh issue list --state open`
- **View Specific Issue Details**: `gh issue view <issue-number>`
- **Historical Completed Work**: Documented in `CHANGELOG.md` with release tags.

---

## 5. GitHub Pages & Static Site Deployment

- **Hosting**: Static documentation and sample showcase hosted on GitHub Pages at `https://mrpoundsign.com/InkAnim/`.
- **Workflow**: Automated artifact-based deployment managed by `.github/workflows/pages.yml` on pushes to `main` touching `web/**` or `.github/workflows/pages.yml` (and manual `workflow_dispatch`).
- **Local Assembly**: Run `.\build.ps1 -Target web` (Windows PowerShell) or `./build.sh web` (Linux/macOS Bash) to copy `web/landing/*` and `web/samples/*` into `build/web/`.

---

## 6. Testing & Quality Requirements

1. **Mandatory Pre-Commit & Pre-Push Checks**:
   - Install and keep `.git/hooks/pre-commit` and `.git/hooks/pre-push` active.
   - **Enforced on Every Commit (`.git/hooks/pre-commit`)**: Code modernizations (`go fix -diff ./...`) and linter (`golangci-lint-v2 run ./...`) must pass on every commit.
   - **Enforced Before Push (`.git/hooks/pre-push`)**: Full unit & UI test suite (`go test ./internal/... ./pkg/...`) must pass.
   - Run full lint, test, and build checks locally:
     ```bash
     ./build.sh lint && ./build.sh test
     # or on Windows PowerShell:
     .\build.ps1 -Target lint; .\build.ps1 -Target test
     ```
2. **Mandatory GUI Verification**:
   - Never commit code until the user has manually tested `build/bin/inkanim.exe` and confirmed the Desktop Studio GUI works as expected.
3. **Cross-Platform Release Builds**:
   - Native Desktop Studio builds for Windows, Linux, and macOS are supported via `.github/workflows/release.yml` (MinGW Windows cross-compilation, Linux tar.xz packaging, and native macOS `.app` bundling) with consolidated SHA-256 checksums.
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
4. **Deterministic Cross-Platform Font Fallbacks**: Embedded official DejaVu Sans TTFs (`pkg/inksvg/fonts/`) ensure identical, zero-dependency glyph rendering across Windows, macOS, and Linux without relying on system font directories.

### Future Modularization Roadmap (Issue #50)
The in-memory SVG preprocessor and rasterizer has been extracted from `internal/svg` into a decoupled, pure-Go public package (`pkg/inksvg`) with zero GUI/Fyne or internal dependencies (Phase 1, Issue #50). Once a companion project or external consumers spin up, it can be seamlessly extracted into a standalone GitHub repository (`github.com/mrpoundsign/inksvg`) without modifying package internals.

---

## 8. Unified Installer & Self-Updater (`@skill:goreleaser-installer-updater`)

- Uses `github.com/mrpoundsign/appinstaller` (`../appinstaller`), GoReleaser v2 OSS (`.goreleaser.yaml`), and the global Antigravity skill `@skill:goreleaser-installer-updater`.
- `cmd/installer` packages and installs `inkanim.exe` (Desktop Studio) cleanly without CLI binaries or platform build-tag stubs.
