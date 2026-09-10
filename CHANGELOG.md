# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.6-pre5] - 2026-09-08

### Added
- **Preview Canvas Background Switcher**: Added a background preset dropdown to the center playback toolbar supporting 5 preview backgrounds matching test environments: Checkerboard (transparency grid), Twitch Dark (#18181b), Twitch Light (#ffffff), Discord Dark (#313338), and Vibrant Gradient (135° diagonal pink-purple-blue). Transparent SVGs now render against realistic streaming and chat backdrops without affecting output GIF encoding ([#67](https://github.com/mrpoundsign/InkAnim/issues/67)).
- **"Ping-Pong" (Bounce / Reverse) Loop Playback & GIF Export**: Added support for Ping-Pong looping across both the live preview playback engine and the exported GIF encoding pipeline. When enabled on animations with 3 or more frames, playback cycles from the first to last frame and reverses back down without repeating turnaround frames ($2N-2$ total sequence), ensuring seamless bouncing loops in external players like Twitch, Discord, and browsers. Automatically disabled for documents with fewer than 3 frames ([#2](https://github.com/mrpoundsign/InkAnim/issues/2)).
- **CLI Ping-Pong Flag**: Added `-pingpong` command-line flag to `inkanim-cli` for batch exporting bounced loop animations ([#2](https://github.com/mrpoundsign/InkAnim/issues/2)).
- **WYSIWYG Palette Quantization & Dithering Preview**: Live animation preview canvas and Twitch scale thumbnails reflect the active color palette quantization (2–256 colors) and Floyd-Steinberg dithering in real time. Added a "WYSIWYG" toggle in playback controls to effortlessly compare color-quantized GIF frames against the unquantized 32-bit true-color rasterization ([#34](https://github.com/mrpoundsign/InkAnim/issues/34)).
- **Reusable NumericCommitInput Component**: Created a numeric-only input component with bounds checking and commit via an "OK" button or Enter key ([#33](https://github.com/mrpoundsign/InkAnim/issues/33)).
- **Custom Palette Color Limit**: Added a "Custom" option to the palette dropdown supporting any color count between 2 and 256.
- **Bulk Differential Testing Scanner (`cmd/dev scan`)**: Added a dedicated `scan` subcommand to `cmd/dev` that recursively scans directories of SVGs, computes perceptual pixel differences against headless Inkscape ground truth, generates visual diff PNGs with neon magenta highlights on failures, and prints a comprehensive summary table with per-feature breakdown metrics ([#59](https://github.com/mrpoundsign/InkAnim/issues/59)).
- **Automated Golden Test Suite & GitHub Actions CI Gate**: Added an integration test suite validating pixel-level rendering accuracy against headless Inkscape goldens, enforced automatically via GitHub Actions on all PRs and pushes ([#40](https://github.com/mrpoundsign/InkAnim/issues/40)).
- **Developer Golden & Diff CLI (`cmd/dev`)**: Added developer CLI tool with subcommands `golden --generate` and `diff` for canonical headless Inkscape golden generation and visual diff inspection ([#51](https://github.com/mrpoundsign/InkAnim/issues/51)).

### Changed
- **Streamlined Animation to Layers-Only**: Simplified frame extraction exclusively to Inkscape layers (`inkscape:groupmode="layer"`), eliminating confusing dual-rendering modes while retaining full support for multi-page SVGs (`<inkscape:page>`) as artboard and camera crop boundaries ([#39](https://github.com/mrpoundsign/InkAnim/issues/39)).
- **Unified Right-Side Tabbed Sidebar**: Consolidated Left Frames panel and Right Export panel into a single tabbed sidebar ("Frames" and "Export" tabs) on the right side of the window, freeing up ~240px of horizontal space for a much larger, more immersive animation preview canvas ([#35](https://github.com/mrpoundsign/InkAnim/issues/35)).
- **Collapsible Bottom Scale Inspector**: Added a "Scale Inspector" checkbox toggle in playback controls to hide/show the bottom Twitch chat-scale preview dock, allowing the main animation canvas to expand to full window height ([#35](https://github.com/mrpoundsign/InkAnim/issues/35)).
- **Clean Export & Speed Preset Visibility**: Resolution, Dimensions, and Global Speed text inputs are hidden by default and only revealed when choosing "Custom", preventing UI clutter and partial-keystroke re-calculations ([#33](https://github.com/mrpoundsign/InkAnim/issues/33)).
- **Updated Landing Page Copy & Simplified Downloads**: Clarified animation capabilities, added Discord emote support, and consolidated desktop downloads to direct GitHub Releases links.
- **Web Studio & WebAssembly Edition**: Positioned the WebAssembly edition as a complete in-browser studio, adding a collapsible "Sample Animations" toolbar with one-click preview loading of bundled sample SVGs.

### Fixed
- **WebAssembly Numeric Input Deadlock & Crash**: Added `[Migrations] fyneDo = true` to `FyneApp.toml` (and linked to `cmd/inkanim/FyneApp.toml`) with `--tags migrated_fynedo` to bypass upstream Fyne v2.8 `EnsureMain() -> DoAndWait()` channel deadlocks during `glfw-js` DOM keystrokes. Wrapped `NumericCommitInput` commits in `fyne.Do`, removed redundant layer list refreshes from global speed changes, and converted the per-frame duration override into a non-blocking `commitEntry` ([#66](https://github.com/mrpoundsign/InkAnim/issues/66)).
- **WebAssembly Text Entry Deadlock**: Intercepted canvas blur event when Fyne focuses hidden `#dummyEntry` element, preventing upstream `glfw-js` focus lost callback from deadlocking the WebAssembly runtime ([#33](https://github.com/mrpoundsign/InkAnim/issues/33)).
- **SVG Drop Shadow Filter Compositing (`<filter>` / `<feGaussianBlur>` / `<feOffset>` / `<feFlood>`)**: Added dynamic raster extraction, Gaussian blur, and Porter-Duff compositing for Inkscape and SVG drop shadow filter chains in `RenderSVGToRGBA`, resolving upstream `oksvg` filter omissions and rendering blurred drop shadows underneath shapes with 99.98% Inkscape pixel fidelity ([#60](https://github.com/mrpoundsign/InkAnim/issues/60)).
- **SVG Standard `stroke-linejoin="miter"` Defaulting**: Injected default `stroke-linejoin="miter"` on stroked shapes and groups in `PreprocessSVG` per W3C SVG specification, overcoming upstream `oksvg` defaulting to `rasterx.Bevel` and restoring sharp 90° miter joins across drawings ([#64](https://github.com/mrpoundsign/InkAnim/issues/64)).
- **SVG `<clipPath>` and `clip-path` Masking**: Implemented dynamic alpha-mask extraction and layered raster compositing for arbitrary clipping paths defined in `<defs>` and referenced via `clip-path="url(#...)"`, resolving upstream `rasterx`/`oksvg` lack of path clipping and correctly restricting shapes to clipping boundaries ([#63](https://github.com/mrpoundsign/InkAnim/issues/63)).
- **Vector Subpath Normalization for `fill-rule="evenodd"`**: Evaluated subpath nesting hierarchies and signed polygon area (Shoelace formula) in `PreprocessSVG` to invert the contour winding of inner cutouts for paths specifying `fill-rule="evenodd"`, resolving upstream `rasterx`/`oksvg` NonZero limitations and rendering interior cutouts and holes cleanly ([#62](https://github.com/mrpoundsign/InkAnim/issues/62)).
- **Hidden Element & Group Pruning (`display="none"` / `style="display:none"`)**: Intercepted and pruned non-rendering shapes and container groups with `display="none"` or `style="display:none"` during `PreprocessSVG` while preserving Inkscape animation layers (`inkscape:groupmode="layer"`), preventing hidden elements, editor guides, and draft artwork from appearing in rendered frames and output GIFs ([#61](https://github.com/mrpoundsign/InkAnim/issues/61)).
- **Root Viewport Transform Composition on `userSpaceOnUse` Gradients**: Pre-composed the root viewport scaling and translation matrix onto `gradientTransform` attributes for `userSpaceOnUse` linear and radial gradients in `RenderSVGToRGBA`, fixing severe color shifts, misalignment, and clamped rendering when SVGs are scaled to non-native preview and export dimensions ([#58](https://github.com/mrpoundsign/InkAnim/issues/58)).
- **Implicit Repeated Arc Command Desugaring in Path `d` Data**: Desugared implicit repeated `a` and `A` commands into explicit consecutive arc commands during `PreprocessSVG`, circumventing upstream `oksvg` multi-arc indexing bugs and restoring missing/corrupted arc curves across drawings ([#57](https://github.com/mrpoundsign/InkAnim/issues/57)).
- **Group-Transformed `userSpaceOnUse` Gradient Alignment**: Composed ancestor group transformation matrices onto `gradientTransform` for `userSpaceOnUse` linear and radial gradients during `PreprocessSVG`, and normalized group `scale(s)` transforms into canonical `matrix(...)` syntax, fixing misaligned and zero-height clamped gradient fills across drawings authored with transformed groups ([#56](https://github.com/mrpoundsign/InkAnim/issues/56)).
- **Gradient Stop Style Normalization (`stop-color` / `stop-opacity`)**: Extracted `stop-color` and `stop-opacity` properties from inline CSS `style="..."` attributes on `<stop>` elements and promoted them to explicit XML attributes in `PreprocessSVG`, preventing upstream `oksvg` from silently dropping stop colors and defaulting to solid black fills on Inkscape-authored gradients ([#55](https://github.com/mrpoundsign/InkAnim/issues/55)).
- **Uninstalled Font Family Fallback to System Sans-Serif**: Updated `FontManager.ResolveFont` to fall back to the system's standard `sans-serif` font (`DejaVu Sans` on Linux, `Segoe UI` on Windows) when custom or uninstalled font families are requested, ensuring consistent glyph metrics and text layout matching standard desktop SVG user agents ([#54](https://github.com/mrpoundsign/InkAnim/issues/54)).
- **Rect Corner Radius Zero Normalization (`rx="0"` / `ry="0"`)**: Mirrored non-zero corner radii when either `rx` or `ry` is explicitly set to zero on `<rect>` elements in `PreprocessSVG`, preventing upstream `oksvg` from incorrectly rasterizing sharp 90° corners on Inkscape-authored rounded rectangles ([#53](https://github.com/mrpoundsign/InkAnim/issues/53)).
- **Shape-Level Transform Stroke Scaling**: Multiplied ancestor group scale factors by shape-level `transform` attributes (`matrix`, `scale`) in `PreprocessSVG` so that paths, rects, and polygons with direct affine transformations render strokes at true visual thickness without ballooning or obscuring fill content ([#52](https://github.com/mrpoundsign/InkAnim/issues/52)).
- **ViewBox Aspect Preservation & Letterbox Scaling (`preserveAspectRatio`)**: Supported standard SVG `preserveAspectRatio` (defaulting to `xMidYMid meet`) in `RenderSVGToRGBA`, applying uniform scaling and centered letterboxing/pillarboxing to prevent distortion when rendering non-square SVGs into square or custom export dimensions ([#48](https://github.com/mrpoundsign/InkAnim/issues/48)).
- **SVG Gradient Stop Inheritance & Transform Normalization**: Resolved multi-level color stop inheritance across `<linearGradient>` and `<radialGradient>` templates referencing color palettes via `xlink:href` / `href`, and normalized 2D affine `gradientTransform` attributes to canonical `matrix(...)` syntax so linear and radial gradients render with full color fidelity and proper positioning under upstream rasterizers ([#49](https://github.com/mrpoundsign/InkAnim/issues/49)).
- **Embedded Base64 Raster Images (`<image>` tags)**: Added decoding and layered compositing for embedded PNG, JPEG, and WebP raster graphics (`<image href="data:image/png;base64,...">`), interleaving raster drawings with vector paths to guarantee pixel-accurate z-ordering, affine transforms, and opacity ([#47](https://github.com/mrpoundsign/InkAnim/issues/47)).
- **Expanded and Inlined SVG `<use>` Element Clones**: Resolved reusable element definitions (`<use href="#id">` and `<use xlink:href="#id">`) during in-memory pre-processing, synthesizing `<g>` groups with full transform matrices and translations so cloned shapes and symbols render accurately and inherit downstream stroke scaling and styling ([#46](https://github.com/mrpoundsign/InkAnim/issues/46)).
- **Physical Dimension Unit Conversion**: Converted CSS/SVG physical length units (`"in"`, `"mm"`, `"cm"`, `"pt"`, `"pc"`) to standard 96 DPI pixels during SVG parsing and root preprocessing, eliminating distorted or squished viewports on SVGs authored with physical dimensions ([#45](https://github.com/mrpoundsign/InkAnim/issues/45)).
- **Inkscape Document Background Page Color & Opacity**: Extracted `sodipodi:namedview pagecolor` and `inkscape:pageopacity` during SVG pre-processing to accurately render document canvas background colors ([#43](https://github.com/mrpoundsign/InkAnim/issues/43)).
- **LPE `fillet_chamfer` Vertex Wrapping & Closing Arc**: Fixed closed polygon handling in `internal/svg/lpe.go#filletSegments` to round all vertices including vertex 0 across `Z` subpaths and correctly calculate non-inverted closing tangent arcs ([#41](https://github.com/mrpoundsign/InkAnim/issues/41)).
- **Cross-Platform Font Resolution for SVG Text Rendering**: Enhanced font manager with standard system font aliases (`DejaVu Sans`, `Liberation Sans`, `Segoe UI`, `Arial`) and embedded fallbacks for consistent text rendering across native desktop and WASM ([#42](https://github.com/mrpoundsign/InkAnim/issues/42)).

### Performance & Tooling
- **Optimized GIF Color Quantization & Hot Path Lookups**: Overhauled color quantization and palette generation in `internal/gif/quantizer.go` with flat pre-unpacked palette entries, scaled integer squared Euclidean distance math, a 4096-entry direct-mapped L1 CPU color cache (32 KB), Go `slices.SortFunc` (pdqsort), and direct linear byte slice scanning (`Pix`). ([#26](https://github.com/mrpoundsign/InkAnim/issues/26))
  - `QuantizeFrame (No Dither)`: **134.6 ms -> 8.63 ms** (**15.6x faster**, -93.6% latency).
  - `GeneratePalette`: **47.2 ms -> 38.4 ms** (**1.23x faster**, -18.6% latency).
  - `EncodeAnimatedGIF (No Dither)`: **245.7 ms -> 195.4 ms** (**1.26x faster**, -20.4% latency).
- **Eliminated Redundant In-Memory GIF Buffering**: Replaced `bytes.Buffer` and `io.MultiWriter` with a zero-allocation `countingWriter` in `WriteGIFToWriter`, eliminating duplicated in-memory payload buffering during GIF export ([#28](https://github.com/mrpoundsign/InkAnim/issues/28)).
- **Performance Linters & Go Modernization**: Enabled `prealloc`, `perfsprint`, `gocritic`, and `makezero` in `golangci-lint-v2`. Modernized codebase with Go sequence iterators (`strings.SplitSeq`), integer range loops (`for range n`), built-in `min`/`max`, preallocated slice capacities, and optimized string/error formatting ([#29](https://github.com/mrpoundsign/InkAnim/issues/29)).

---

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
