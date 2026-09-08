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
    4. Generate Ground-Truth Golden via Headless Inkscape
  `inkscape fixture.svg --export-type=png -o fixture.golden.png -w 128 -h 128`
              │
              ▼
    5. Register in Integration Suite
  (Add to `testdata/fixtures/` and execute `go test ./internal/svg/...`)
```

---

## 3. Atomic Fixtures Catalog

Each fixture lives in `testdata/fixtures/` as `<operation_name>.svg` and `<operation_name>.golden.png`:

| Fixture Name | Isolated Operation Under Test |
| :--- | :--- |
| `lpe_fillet_chamfer` | Polygon path referencing `<inkscape:path-effect effect="fillet_chamfer">` |
| `paint_order_stroke_fill` | Path with `style="paint-order: stroke fill"` verifying stroke sits under fill |
| `transform_group_stroke` | Nested `<g transform="scale(...)">` verifying stroke width scales proportionally |
| `rect_rx_ry_mirror` | `<rect ry="15">` with omitted `rx` verifying spec-compliant rounded corners |
| `text_tspan_basic` | Text with `<tspan>` baseline offset converting to vector glyph contours |
| `crop_boundary_clip` | Drawing with paths extending outside viewBox verifying clean boundary clipping |

---

## 4. Test Execution & Artifact Storage

- **In-Memory Comparisons**: During test runs (`go test ./internal/svg/...`), rasterization and comparison execute 100% in-memory with zero file writes when tests pass.
- **Reference Goldens**: The canonical ground-truth images produced by headless Inkscape are version-controlled alongside the source SVGs:
  - `testdata/fixtures/<name>.svg`
  - `testdata/fixtures/<name>.golden.png`
- **Failure Artifacts (`testdata/scratch/`)**:
  - If a test exceeds perceptual mismatch thresholds, `AssertImageMatchesGolden` automatically outputs:
    - `testdata/scratch/<name>_actual.png` (InkAnim's rasterization)
    - `testdata/scratch/<name>_diff.png` (Annotated visual diff highlighting mismatched pixels)
  - `testdata/scratch/` is git-ignored so temporary failure artifacts never pollute the git working tree or PR diffs.

