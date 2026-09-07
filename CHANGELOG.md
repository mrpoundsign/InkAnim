# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.5] - 2026-09-07

### Added
- **Dynamic Estimated GIF Size on Export Button**: Display real-time estimated GIF export size directly on the primary export button (e.g. `Export Animated GIF (~23 KB)...`). Uses an active-pixel compression model that factors in resolution, transparent padding, frame count, and color palette tiers ([#32](https://github.com/mrpoundsign/InkAnim/issues/32)).
- **Integrated Profiling Flags**: Added `-cpuprofile`, `-memprofile`, and `-pprof` command-line flags across desktop GUI and CLI for in-depth profiling and diagnostics.

### Changed
- **Responsive Playback Controls Layout**: Restructured playback controls into a centered transport row and a dedicated frame label row with ellipsis truncation, preventing right export sidebar clipping on smaller displays and browser viewports ([#30](https://github.com/mrpoundsign/InkAnim/issues/30)).

### Performance
- **Parallelized Rasterization & Quantization Across CPU Cores**: Multi-threaded preview frame rasterization, export vector rasterization, and GIF color quantization via a lightweight worker pool bounded by `runtime.GOMAXPROCS(0)`. Reduces export times by over 60% on multi-core CPUs ([#27](https://github.com/mrpoundsign/InkAnim/issues/27)).
- **Preview Playback Optimization & Event-Driven Timing**: Eliminated redundant software bilinear downscaling during live playback by pre-rendering scaled thumbnail caches, switched main canvas to `ImageScaleFastest`, and replaced polling tickers with event-driven `time.AfterFunc` timers ([#25](https://github.com/mrpoundsign/InkAnim/issues/25)).
- **Pause Playback on Modal Dialogs & Export**: Automatically suspend preview playback during export generation and when modal dialogs are open, eliminating unnecessary thread contention and rendering load ([#25](https://github.com/mrpoundsign/InkAnim/issues/25)).
- **Eliminated Infinite Progress Bar Animation Leak**: Removed `widget.ProgressBarInfinite` from the export flow to resolve an upstream Fyne animation leak that continuously marked the canvas dirty at 60Hz+ and pegged CPU during and after modal dialogs ([#27](https://github.com/mrpoundsign/InkAnim/issues/27)).

---

## [0.1.4] - 2026-09-07

> [!NOTE]
> **Major Rendering Milestone**: Version 0.1.4 introduces an advanced in-memory SVG pre-processing pipeline and vector font engine that fixes many rendering issues across complex Inkscape SVGs. This release represents a significant improvement in rendering accuracy and visual fidelity—supporting text glyphs, unbaked Live Path Effects (fillets/chamfers), paint-order desugaring, transform stroke scaling, and resolution scaling without destructively modifying or flattening original SVG files on disk.

### Added
- **In-Memory SVG Text-to-Path Vector Converter**: Automatically converts SVG `<text>` and `<tspan>` elements (including CSS SVG 2 flowed text `shape-inside`) into standard `<path>` vector glyph contours before rasterization. Resolves system TrueType/OpenType fonts, provides cross-platform embedded font fallbacks for WebAssembly, applies kerning and uniform letter-spacing, and inherits ancestor group transform scaling and paint-order desugaring ([#21](https://github.com/mrpoundsign/InkAnim/issues/21)).
- **In-Memory SVG Pre-Processor for Inkscape Live Path Effects (LPE) & Paint-Order**: Seamlessly evaluates Inkscape `fillet_chamfer` LPEs on paths, desugars SVG `paint-order: stroke fill` across shapes, normalizes omitted `rect` `rx`/`ry` attributes, and scales stroke widths across nested `<g transform="...">` hierarchies ([#20](https://github.com/mrpoundsign/InkAnim/issues/20)).
- **Automated Golden Image Regression Test Harness**: Comprehensive perceptual image comparison harness (`golden_test.go`) validating rendered SVG frames against reference golden images with antialiasing tolerance, automated diff artifact generation, and `-update-golden` workflow tooling ([#23](https://github.com/mrpoundsign/InkAnim/issues/23)).
- **Version Number in About Button**: Display current application version directly on the top-right header button (e.g. `About (v0.1.4)`) for immediate version visibility.
- **CLI File Argument Loading**: Automatically opens SVG files supplied as first command-line arguments on application launch (e.g. `inkanim.exe ./testdata/hydrate.svg`).
- **GitHub Pages WebAssembly Demo & Landing Page**: Deployed an in-browser WebAssembly studio demo (`/demo`) and project landing page (`/`) via automated GitHub Actions artifact deployment. Features Twitch chat scale preview, direct desktop downloads, CLI installation instructions, and instant sample animation loading ([#16](https://github.com/mrpoundsign/InkAnim/issues/16)).
- **WebAssembly Build & Dev Server Tooling**: Added `wasm` and `serve` targets to `build.sh` and `build.ps1`, and created `cmd/wasm-serve` to serve the web application locally on `http://localhost:8080` with proper `application/wasm` MIME types ([#16](https://github.com/mrpoundsign/InkAnim/issues/16)).
- **Programmatic Sample SVG Loader**: Added `window.inkanimLoadBytes` bridge in WebAssembly platform driver to enable 1-click sample animation loading in web browsers ([#16](https://github.com/mrpoundsign/InkAnim/issues/16)).

### Fixed
- **ViewBox Offset Translation in Rasterization**: Fixed upstream `oksvg` matrix multiplication bug where `icon.SetTarget` applied unscaled viewBox translation offsets, causing SVGs with non-zero viewBox origins (Drawing bounds and multi-page layouts) to drift down/right and clip against canvas edges.
- **Stroke-Width in Drawing Crop Boundary**: Factored `stroke-width` (`LineWidth / 2` + join/miter allowance) into `ComputeDrawingRect` so outer strokes on all four edges are completely preserved without flat clipping in Drawing mode ([#17](https://github.com/mrpoundsign/InkAnim/issues/17)).

---

## [0.1.3] - 2026-09-06

### Added
- **About InkAnim Modal Dialog**: Added an "About InkAnim" modal dialog featuring the embedded app icon, version metadata, interactive GitHub Releases update checker with live status, project hyperlinks, and license credits ([#12](https://github.com/mrpoundsign/InkAnim/issues/12)).
- **GitHub Releases Update Checker**: Added asynchronous version update checker querying GitHub Releases API with Semantic Version parsing, comparison, and rate-limit error handling ([#10](https://github.com/mrpoundsign/InkAnim/issues/10)).
- **Vector Export Scaling**: Rasterize export frames directly from vector SVG at target export resolution (up to 4096px) to eliminate upscaling blurriness, while capping interactive preview rasterization to 512px max dimension for smooth playback ([#13](https://github.com/mrpoundsign/InkAnim/issues/13)).
- **Drawing Crop Boundary Mode**: Added unclipped "Drawing" crop boundary mode computing the bounding box of all SVG vector paths/elements (matching Inkscape's verbage), and included "Document" (root viewBox) as the primary option in the Page selection dropdown ([#15](https://github.com/mrpoundsign/InkAnim/issues/15)).
- **Boundary Crop Mode**: Added boundary crop mode toggle with interactive dotted cyan preview crop guides and auto-squaring support ([#14](https://github.com/mrpoundsign/InkAnim/issues/14)).
- **Developer Tooling & Scripts**: Added `build.sh` script, updated `build.ps1`, and integrated `golangci-lint-v2` into `.golangci.yml` and `.git/hooks/pre-push`.

### Changed
- **CLI Release Archive Naming**: Prefixed GoReleaser CLI release archives with `inkanim-cli_` (`inkanim-cli_<version>_<os>_<arch>.tar.gz` and `.zip`) to clearly distinguish CLI releases from desktop GUI releases ([#7](https://github.com/mrpoundsign/InkAnim/issues/7)).

---

## [0.1.2] - 2026-09-06

### Added
- **WebAssembly (WASM) Browser Support**:
  - In-browser file picker (`<input type="file" accept=".svg">`) with automatic asynchronous `FileReader` decoding.
  - Full-window drag-and-drop file loading directly onto the WebGL canvas.
  - Browser export download bridge streaming encoded animated GIFs directly as downloaded file blobs via temporary link triggers.
- **Platform-Agnostic File Dialog Abstraction**:
  - Separated desktop native modal dialogs from WebAssembly web browser APIs with build tags (`!wasm` vs `wasm`).

### Changed
- **Native OS File Dialogs**: Replaced small embedded canvas file dialogs with full-sized, resizable native OS modal file dialogs (`zenity` on Linux, File Explorer on Windows) for opening SVGs and saving GIFs ([#1](https://github.com/mrpoundsign/InkAnim/issues/1)).
- **Play/Pause Button States & Feedback**:
  - Play button displays a high-contrast red "Play" state when stopped and a green "Pause" state when running.
  - When loop is unchecked, playback stops automatically upon reaching the final frame; pressing Play again cleanly restarts playback from frame 0 ([#5](https://github.com/mrpoundsign/InkAnim/issues/5)).

### Fixed
- Fixed background animation ticker thread safety assertions by dispatching UI state changes on Fyne's main thread via `fyne.Do` ([#6](https://github.com/mrpoundsign/InkAnim/issues/6)).

---

## [0.1.1] - 2026-09-06

### Added
- Auto-play animation preview when loading multi-frame Inkscape SVGs.
- Cross-platform CI release workflows with GoReleaser and unified SHA256 checksums.

### Fixed
- Guaranteed pinned background layers render behind active animation frames ([#3](https://github.com/mrpoundsign/InkAnim/issues/3)).
- Animation ticker cancellation race condition during test executions and rapid document switches.
- UI text layout race condition by preserving pause state across refresh calls.

---

## [0.1.0] - 2026-09-05

### Added
- Initial release of InkAnim Studio.
- Multi-layer and multi-page Inkscape SVG animation parsing and preview.
- Twitch emote export profiles (28x28, 56x56, 112x112) with auto-squaring, frame duration overrides, and color quantization.
- Desktop GUI (Windows & Linux) and headless CLI (`inkanim-cli`).
