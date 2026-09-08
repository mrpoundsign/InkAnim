---
name: svg-render-tester
description: >-
  Systematically converts SVG bug reports and real-world SVGs into anonymized, deduplicated, and minimal synthetic unit tests for InkAnim's integration rendering suite.
---

# SVG Render Tester & Reduction Protocol

This skill provides a standardized protocol to transform bug reports and real-world SVGs into **anonymized, synthetic, and minimal** unit tests for InkAnim's integration rendering suite.

---

## 1. Core Principles

### 🔒 1. Strict Anonymization & Privacy
- **Never check in personal or proprietary artist files**: Real-world SVGs (from streams, user bug reports, or external repositories) must **never** be checked into the repository or test suite.
- **Synthesize and Anonymize**: Distill the problem down into generic geometric shapes (a simple polygon, a dummy circle, or placeholder curves) with random/generic dimensions and colors.

### 🎯 2. Orthogonality & Zero Duplication
- Each test fixture must isolate and test **exactly one distinct vector operation or edge case** (e.g. LPE fillet/chamfer, nested matrix transform stroke scaling, paint-order desugaring, rect rx/ry normalization).
- **Check Existing Tests First**: Before creating a new fixture, check the catalog. If an existing test already covers the operation, enhance the existing test or verify why the bug differs before adding anything new.

### ⚡ 3. Absolute Minimality
- A test fixture must be the absolute smallest valid SVG that exercises the operation:
  - No `<sodipodi:namedview>`, `<metadata>`, editor namespaces, or unreferenced definitions.
  - Minimal `<svg>` header with explicit `viewBox` and `width`/`height`.
  - Exactly one minimal element (or the minimal nested group) triggering the behavior.
  - Clean, human-readable coordinates.

### 🏛️ 4. Uncompromising Ground Truth (Authoring-Time Inkscape CLI)
- **The agent/developer MUST run headless Inkscape in the terminal to generate the golden file**:
  - `go run ./cmd/dev golden --generate <name>`
  - (Or direct CLI: `inkscape testdata/fixtures/<name>.svg --export-type=png --export-filename=testdata/fixtures/<name>.golden.png -w 128 -h 128`)
  - (Use `-w 512 -h 512` for 512×512 fixtures like `text_full_alphabet`).
- **Developer CLI (`cmd/dev`)**:
  - `go run ./cmd/dev golden <name>`: Compare fixture with golden and report pixel diff metrics / diff bounds.
  - `go run ./cmd/dev golden --generate <name>`: Auto-invoke headless Inkscape with proper dimensions.
  - `go run ./cmd/dev inspect <path> --color "#hex"`: Inspect pixel bounds, centroids, and cluster coordinates.
- **Tests DO NOT invoke Inkscape**: CI environments, Docker containers, and `go test` runners do not have Inkscape installed. If a `.golden.png` is missing, `TestAtomicFixtures` will fail immediately.
- **Always Version-Control Goldens**: Both `<name>.svg` and `<name>.golden.png` must be committed together to git.
- **Never bypass Inkscape CLI**: Never generate golden PNGs via temporary helper scripts, Python scripts, mock SVGs, or manually pre-calculated arc paths.
- **Fix the Fixture, Never Fake the Golden**: If headless Inkscape renders something unexpected, the fixture SVG itself is missing necessary Inkscape attributes (e.g., LPE metadata, path format). Adjust the fixture SVG so Inkscape naturally renders the ground truth.

---

## 2. The Bug-to-Fixture Workflow

```
[ Incoming Bug / Artist SVG ]
              │
              ▼
    1. Identify Failure
  (What specific SVG feature or Inkscape operation is failing?)
              │
              ▼
    2. Check for Duplicate
  (Does an existing test already cover this operation?)
      ├─ YES ──> Refine existing test or clarify edge case
      └─ NO  ──> Proceed to synthesis
              │
              ▼
    3. Synthesize Minimal Anonymized SVG
  (Generate synthetic generic shape exercising only that operation)
              │
              ▼
    4. Run Headless Inkscape in Terminal to Generate Ground-Truth Golden
  `inkscape testdata/fixtures/<name>.svg --export-type=png --export-filename=testdata/fixtures/<name>.golden.png -w 128 -h 128`
  (Must be run by agent/developer in shell before running tests; tests do NOT invoke Inkscape)
              │
              ▼
    5. Register in Integration Suite
  (Commit both .svg and .golden.png, then execute `go test ./internal/svg/...`)
```

---

## 3. Atomic Fixtures Catalog

Each fixture lives in `testdata/fixtures/` as `<operation_name>.svg` and `<operation_name>.golden.png`:

| Fixture Name | Isolated Operation Under Test |
| :--- | :--- |
| `lpe_fillet_chamfer` | Polygon path referencing `<inkscape:path-effect effect="fillet_chamfer">` |
| `paint_order_stroke_fill` | Path with `style="paint-order: stroke fill"` verifying stroke sits under fill |
| `transform_group_stroke` | Nested `<g transform="scale(...)">` verifying stroke width scales proportionally |
| `transform_group_gradient` | `userSpaceOnUse` linear and radial gradients inside transformed `<g>` groups verifying gradient coordinates scale and translate with the shape |
| `rect_rx_ry_mirror` | `<rect ry="15">` with omitted `rx` verifying spec-compliant rounded corners |
| `rect_rx_ry_zero` | `<rect>` with explicit `rx="0"` or `ry="0"` alongside positive radius verifying rounded corner normalization |
| `text_tspan_basic` | Text with `<tspan>` baseline offset converting to vector glyph contours |
| `text_font_fallback` | Text referencing uninstalled font families verifying fallback to system sans-serif |
| `text_full_alphabet` | Comprehensive character set (A-Z, a-z, 0-9, punctuation) verifying glyph geometry and holes |
| `crop_boundary_clip` | Drawing with paths extending outside viewBox verifying clean boundary clipping |
| `namedview_pagecolor` | Document background color (`sodipodi:namedview pagecolor` & `pageopacity`) synthesis |
| `dimension_units` | Physical length units (`in`, `mm`, `cm`, `pt`, `pc`) converted to standard 96 DPI pixels |
| `use_element_clone` | Reusable cloned objects (`<use href="#id">` and `<use xlink:href="#id">`) with transforms and translations |
| `image_embedded_raster` | Embedded raster graphic (`<image href="data:image/png;base64,...">`) with z-ordered compositing |
| `gradient_transform` | Multi-level stop inheritance via `xlink:href` and normalized 2D affine `gradientTransform` in linear and radial gradients |
| `gradient_stop_style` | Gradient `<stop>` elements with inline CSS `style="stop-color:...;stop-opacity:..."` normalized to explicit XML attributes |
| `preserve_aspect_ratio` | SVG standard `preserveAspectRatio` (default `xMidYMid meet`) with uniform scaling and letterbox/pillarbox alignment |
| `transform_shape_stroke` | Direct shape-level `transform="matrix(...)"` or `scale(...)` verifying `stroke-width` scales proportionally |
| `gradient_viewport_scale` | `userSpaceOnUse` linear and radial gradients scaled to non-native viewport resolutions verifying gradientTransform scales with document |
| `shape_polyline_polygon` | `<polygon>` and `<polyline>` coordinate points list |
| `shape_ellipse` | `<circle>` and `<ellipse>` radii and center coordinates |
| `shape_line` | `<line x1 y1 x2 y2>` coordinate endpoints with stroke caps |
| `path_bezier_curves` | Cubic (`C`, `S`) and quadratic (`Q`, `T`) Bézier curve segments |
| `path_arc_elliptic` | Elliptical arc commands (`A` / `a`) with large-arc and sweep flags |
| `path_compound_subpaths` | Compound paths with multiple subpaths (`M ... Z M ... Z`) and inner cutout holes |
| `path_relative_commands` | Relative path commands (`m`, `l`, `h`, `v`, `z`) |
| `stroke_join_cap` | Stroke line caps (`butt`, `round`, `square`) and line joins (`miter`, `round`, `bevel`) |
| `stroke_dasharray` | Stroke dash patterns (`stroke-dasharray`) and phase offsets (`stroke-dashoffset`) |
| `opacity_layers` | Group opacity (`<g opacity>`), `fill-opacity`, and `stroke-opacity` compositing |
| `color_formats` | 3-hex, 6-hex, `rgb(...)` integer/percent, and SVG standard color keywords |
| `gradient_radial` | Focused radial gradients (`<radialGradient cx cy r fx fy>`) with multi-color stops |
| `gradient_spread_reflect` | Gradient spread method `reflect` |
| `gradient_spread_repeat` | Gradient spread method `repeat` |
| `gradient_object_bounding_box` | Normalized bounding-box gradients (`gradientUnits="objectBoundingBox"`) |
| `gradient_stop_numeric` | Unitless numeric gradient stop offsets (`0`, `0.5`, `1`) |
| `transform_rotate_skew` | 2D affine rotate (`rotate(...)`) and shear (`skewX(...)`) transforms |
| `transform_matrix_2d` | Explicit 2D affine transformation matrices (`matrix(a, b, c, d, e, f)`) |
| `style_inline_presentation` | Inline CSS presentation attributes (`style="fill:...;stroke:...;..."`) |
| `style_css_class` | CSS `<style>` stylesheet blocks and class selectors (`.class`) |
| `viewbox_negative_origin` | Negative `viewBox` origin offsets (e.g. `viewBox="-64 -64 128 128"`) |
| `visibility_display_none` | Pruned elements and groups with `display="none"` or `style="display:none"` |
| `path_fill_rule_evenodd` | Inverted inner cutout winding for paths specifying `fill-rule="evenodd"` |

---

## 4. Test Execution & Artifact Storage

- **In-Memory Comparisons**: During test runs (`go test ./internal/svg/...`), rasterization and comparison execute 100% in-memory with zero file writes when tests pass.
- **Reference Goldens**: The canonical ground-truth images produced by headless Inkscape are version-controlled alongside the source SVGs:
  - `testdata/fixtures/<name>.svg`
  - `testdata/fixtures/<name>.golden.png`
- **Failure Artifacts (`testdata/scratch/`)**:
  - If a test exceeds perceptual mismatch thresholds, `AssertImageMatchesGolden` automatically outputs:
    - `testdata/scratch/<name>_diff.png` (Composite visual diff: dimmed artwork with mismatched pixels highlighted in neon magenta)
  - `testdata/scratch/` is git-ignored so temporary failure artifacts never pollute the git working tree or PR diffs.

