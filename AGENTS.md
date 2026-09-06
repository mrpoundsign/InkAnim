# InkAnim — Agent & Project Knowledge Base (AGENTS.md)

## 1. User Preferences & Working Rules

> [!IMPORTANT]
> Always adhere strictly to these core working preferences:
> 1. **One ticket at a time**: We work on exactly one issue/ticket at a time unless explicitly directed otherwise. Exactly **1 commit per issue**.
> 2. **User tests GUI before committing**: Always stop and let the user manually test the GUI before any changes are committed to git.
> 3. **Feedback & Approval on Decisions**: Always provide clear feedback on decisions and proposed designs and wait for approval. Never run ahead with assumptions or unapproved actions.

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
├── internal/
│   ├── app/              # Core application session, layer state, mode, orchestration
│   │   ├── session.go    # Session state: Document, Layers, Mode, Duration, ExportOptions
│   │   └── session_test.go
│   ├── svg/              # Inkscape SVG parsing & layer extraction
│   │   ├── parser.go     # SVG XML parser (inkscape:groupmode="layer", sodipodi, pages)
│   │   ├── renderer.go   # Rasterization of SVG layers into RGBA frame images
│   │   └── types.go      # Layer, Page, and Document models
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
- **Solution for WASM**:
  - Implement a web-specific file open bridge using `syscall/js` that triggers a browser `<input type="file" accept=".svg">`, reads the selected file using `FileReader`, and passes bytes to `mw.loadData(bytes, filename)`.
  - Implement an export bridge using `syscall/js` that converts the encoded GIF bytes into a `Blob` and triggers a browser download via a temporary `<a>` element.
  - Implement browser window drag-and-drop (`dragover`/`drop` listeners) to load SVGs directly when dropped into the canvas.

*(Note: Prototypes for these fixes are safely stored in `git stash` to be applied cleanly per the user's instructions).*

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
