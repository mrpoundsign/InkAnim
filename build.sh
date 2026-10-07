#!/usr/bin/env bash
set -euo pipefail

# InkAnim Build Script (Bash)
# Usage:
#   ./build.sh          # Run tests and build CLI & GUI into build/
#   ./build.sh cli      # Build Pure-Go CLI into build/
#   ./build.sh gui      # Build Desktop GUI into build/
#   ./build.sh gui-win    # Build Desktop GUI for Windows via MinGW into build/
#   ./build.sh ext-win    # Build Inkscape extension for Windows into build/
#   ./build.sh ext-linux  # Build Inkscape extension for Linux into build/
#   ./build.sh ext-darwin # Build Inkscape extension for macOS into build/
#   ./build.sh ext-all    # Build all available Inkscape extensions
#   ./build.sh wasm       # Package WebAssembly demo into build/gh-pages
#   ./build.sh wasm-check # Verify WebAssembly compilation
#   ./build.sh test       # Run unit tests
#   ./build.sh cross      # Cross-compile CLI for multiple targets
#   ./build.sh clean      # Clean build outputs

TARGET="${1:-all}"
TAG="${TAG:-snapshot}"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BUILD_DIR="$ROOT_DIR/build"
OUTPUT_DIR="${OUTPUT_DIR:-$BUILD_DIR}"
mkdir -p "$BUILD_DIR" "$OUTPUT_DIR"

if ! command -v go &>/dev/null; then
    if [ -x "/usr/local/go/bin/go" ]; then
        export PATH="/usr/local/go/bin:$PATH"
    elif [ -x "$HOME/go/bin/go" ]; then
        export PATH="$HOME/go/bin:$PATH"
    fi
fi
if [ -d "$HOME/go/bin" ]; then
    export PATH="$PATH:$HOME/go/bin"
fi

run_lint() {
    echo "==> Verifying code modernizations (go fix -diff)..."
    if ! go fix -diff ./...; then
        echo "Error: Unfixed code modernization diffs found. Run 'go fix ./...' to apply them."
        exit 1
    fi
    echo "✓ Code modernizations verified!"

    echo "==> Running golangci-lint-v2..."
    if command -v golangci-lint-v2 &>/dev/null; then
        golangci-lint-v2 run ./...
    else
        echo "Error: golangci-lint-v2 not found in PATH"
        exit 1
    fi
    echo "✓ Lint passed!"
}

build_cli() {
    echo "==> Building inkanim-cli (Pure Go, CGO_ENABLED=0)..."
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "$BUILD_DIR/inkanim-cli" ./cmd/inkanim-cli
    echo "✓ Successfully built $BUILD_DIR/inkanim-cli"
}

build_gui() {
    echo "==> Building inkanim Desktop GUI..."
    CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o "$BUILD_DIR/inkanim" ./cmd/inkanim
    echo "✓ Successfully built $BUILD_DIR/inkanim"
}

build_gui_win() {
    echo "==> Building inkanim Desktop GUI for Windows (via MinGW)..."
    CC=x86_64-w64-mingw32-gcc CGO_ENABLED=1 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-H windowsgui -s -w" -o "$BUILD_DIR/inkanim.exe" ./cmd/inkanim
    echo "✓ Successfully built $BUILD_DIR/inkanim.exe"
}

package_zip() {
    local ext_dir="$1"
    local zip_file="$2"
    rm -f "$zip_file"
    mkdir -p "$(dirname "$zip_file")"
    if command -v zip &>/dev/null; then
        (cd "$ext_dir" && zip -q -r "$zip_file" .)
    else
        python3 -c "
import os, stat, zipfile
ext_dir = os.path.abspath('$ext_dir')
zip_file = os.path.abspath('$zip_file')
with zipfile.ZipFile(zip_file, 'w', zipfile.ZIP_DEFLATED) as zf:
    for root, dirs, files in os.walk(ext_dir):
        for f in files:
            full = os.path.join(root, f)
            rel = os.path.relpath(full, ext_dir).replace('\\\\', '/')
            st = os.stat(full)
            zi = zipfile.ZipInfo(rel)
            zi.external_attr = (st.st_mode & 0xFFFF) << 16
            with open(full, 'rb') as fp:
                zf.writestr(zi, fp.read())
"
    fi
}

package_tar_xz() {
    local ext_dir="$1"
    local tar_file="$2"
    rm -f "$tar_file"
    mkdir -p "$(dirname "$tar_file")"
    tar -cJf "$tar_file" -C "$ext_dir" .
}

prepare_ext_bundle() {
    local target_dir="$1"
    local bin_cmd="$2"
    mkdir -p "$target_dir"
    cp "$ROOT_DIR"/extensions/inkscape/*.inx "$target_dir/"
    cp "$ROOT_DIR"/assets/icon.png "$target_dir/"

    for inx_file in "$target_dir"/*.inx; do
        if [ "$(uname -s)" = "Darwin" ]; then
            sed -i '' "s|<command location=\"inx\">.*</command>|<command location=\"inx\">$bin_cmd</command>|g" "$inx_file"
        else
            sed -i "s|<command location=\"inx\">.*</command>|<command location=\"inx\">$bin_cmd</command>|g" "$inx_file"
        fi
    done
}

build_ext_win() {
    echo "==> Building Inkscape extension for Windows (via MinGW)..."
    local ext_dir="$BUILD_DIR/inkscape-ext/windows-amd64"
    local bin_dir="$ext_dir/bin"
    rm -rf "$ext_dir"
    mkdir -p "$bin_dir"

    CC=x86_64-w64-mingw32-gcc CGO_ENABLED=1 GOOS=windows GOARCH=amd64 \
        go build -trimpath -ldflags="-H windowsgui -s -w -extldflags=-mwindows" \
        -o "$bin_dir/inkanim-ext.exe" ./cmd/inkanim-ext

    prepare_ext_bundle "$ext_dir" "bin/inkanim-ext.exe"

    local zip_file="$OUTPUT_DIR/InkAnim_${TAG}_inkscape-extension_windows_amd64.zip"
    package_zip "$ext_dir" "$zip_file"

    echo "✓ Windows extension package created:"
    ls -lh "$bin_dir/inkanim-ext.exe"
    ls -lh "$zip_file"
}

build_ext_linux() {
    echo "==> Building Inkscape extension for Linux..."
    local ext_dir="$BUILD_DIR/inkscape-ext/linux-amd64"
    local bin_dir="$ext_dir/bin"
    rm -rf "$ext_dir"
    mkdir -p "$bin_dir"

    CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
        go build -trimpath -ldflags="-s -w" \
        -o "$bin_dir/inkanim-ext" ./cmd/inkanim-ext

    chmod +x "$bin_dir/inkanim-ext"
    prepare_ext_bundle "$ext_dir" "bin/inkanim-ext"

    local tar_file="$OUTPUT_DIR/InkAnim_${TAG}_inkscape-extension_linux_amd64.tar.xz"
    package_tar_xz "$ext_dir" "$tar_file"

    echo "✓ Linux extension package created:"
    ls -lh "$bin_dir/inkanim-ext"
    ls -lh "$tar_file"
}

build_ext_darwin() {
    echo "==> Building Inkscape extension for macOS..."
    if [ "$(uname -s)" != "Darwin" ]; then
        echo "Error: ext-darwin requires building on macOS (Darwin) due to CGO/Fyne toolchain requirements."
        exit 1
    fi

    local ext_dir="$BUILD_DIR/inkscape-ext/darwin-all"
    local bin_dir="$ext_dir/bin"
    local tmp_dir="$BUILD_DIR/ext-darwin-tmp"
    rm -rf "$ext_dir" "$tmp_dir"
    mkdir -p "$bin_dir" "$tmp_dir"

    local built_universal=false
    echo "  -> Compiling arm64..."
    if CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o "$tmp_dir/inkanim-ext-arm64" ./cmd/inkanim-ext 2>/dev/null; then
        echo "  -> Compiling amd64..."
        if CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o "$tmp_dir/inkanim-ext-amd64" ./cmd/inkanim-ext 2>/dev/null; then
            if command -v lipo &>/dev/null; then
                echo "  -> Creating universal binary via lipo..."
                lipo -create -output "$bin_dir/inkanim-ext" "$tmp_dir/inkanim-ext-arm64" "$tmp_dir/inkanim-ext-amd64"
                built_universal=true
            fi
        fi
    fi

    if [ "$built_universal" != "true" ]; then
        echo "  -> Fallback: Compiling for host macOS architecture..."
        CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o "$bin_dir/inkanim-ext" ./cmd/inkanim-ext
    fi
    rm -rf "$tmp_dir"

    chmod +x "$bin_dir/inkanim-ext"
    prepare_ext_bundle "$ext_dir" "bin/inkanim-ext"

    local zip_file="$OUTPUT_DIR/InkAnim_${TAG}_inkscape-extension_darwin_all.zip"
    package_zip "$ext_dir" "$zip_file"

    echo "✓ macOS extension package created:"
    ls -lh "$bin_dir/inkanim-ext"
    ls -lh "$zip_file"
}

build_ext_all() {
    echo "==> Building all available Inkscape extension packages..."
    if [ "$(uname -s)" = "Darwin" ]; then
        build_ext_darwin
    else
        build_ext_linux
        if command -v x86_64-w64-mingw32-gcc &>/dev/null; then
            build_ext_win
        else
            echo "Skipping ext-win: x86_64-w64-mingw32-gcc not found."
        fi
    fi
}

verify_wasm() {
    echo "==> Verifying WebAssembly compilation..."
    GOOS=js GOARCH=wasm go build -tags migrated_fynedo -o /dev/null ./cmd/inkanim
    echo "✓ WebAssembly compilation verified!"
}

build_wasm() {
    echo "==> Packaging WebAssembly demo & assembling GitHub Pages site..."
    local PAGES_DIR="$BUILD_DIR/gh-pages"
    local WASM_TEMP="$BUILD_DIR/wasm-tmp"
    rm -rf "$PAGES_DIR" "$WASM_TEMP"
    mkdir -p "$PAGES_DIR/demo" "$PAGES_DIR/samples" "$WASM_TEMP"

    echo "==> Compiling WebAssembly binary..."
    (
        cd "$WASM_TEMP"
        go run fyne.io/tools/cmd/fyne@latest package -os web --release \
            --tags migrated_fynedo \
            --source-dir "$ROOT_DIR/cmd/inkanim" \
            --icon "$ROOT_DIR/assets/icon.png" \
            --name InkAnim
    )

    cp "$WASM_TEMP/wasm/"* "$PAGES_DIR/demo/"
    rm -rf "$WASM_TEMP"

    # Overlay our custom demo template
    cp "$ROOT_DIR/web/demo/index.html" "$PAGES_DIR/demo/index.html"

    # Copy landing page assets to root
    cp -r "$ROOT_DIR/web/landing/"* "$PAGES_DIR/"

    # Copy sample SVGs
    cp -r "$ROOT_DIR/web/samples/"* "$PAGES_DIR/samples/"

    echo "✓ WebAssembly demo and landing page assembled in $PAGES_DIR"
}

serve_web() {
    local PAGES_DIR="$BUILD_DIR/gh-pages"
    if [ ! -f "$PAGES_DIR/demo/InkAnim.wasm" ]; then
        echo "Web site not built yet. Building first..."
        build_wasm
    else
        # Overlay latest HTML/CSS templates so changes reflect immediately
        cp "$ROOT_DIR/web/demo/index.html" "$PAGES_DIR/demo/index.html"
        cp -r "$ROOT_DIR/web/landing/"* "$PAGES_DIR/"
        cp -r "$ROOT_DIR/web/samples/"* "$PAGES_DIR/samples/"
    fi
    go run ./cmd/wasm-serve -dir "$PAGES_DIR" -port 8080
}

run_tests() {
    local UPDATE_GOLDEN="${1:-false}"
    echo "==> Running unit tests..."
    if [ "$UPDATE_GOLDEN" = "true" ]; then
        go test -v ./internal/... ./pkg/... -update-golden
    else
        go test -v ./internal/... ./pkg/...
    fi
    echo "✓ All tests passed!"
}

cross_compile_cli() {
    echo "==> Cross-compiling inkanim-cli for Windows and Linux..."
    mkdir -p "$BUILD_DIR/dist"
    targets=(
        "windows/amd64/$BUILD_DIR/dist/inkanim-cli-windows-amd64.exe"
        "windows/arm64/$BUILD_DIR/dist/inkanim-cli-windows-arm64.exe"
        "linux/amd64/$BUILD_DIR/dist/inkanim-cli-linux-amd64"
        "linux/arm64/$BUILD_DIR/dist/inkanim-cli-linux-arm64"
        "darwin/amd64/$BUILD_DIR/dist/inkanim-cli-darwin-amd64"
        "darwin/arm64/$BUILD_DIR/dist/inkanim-cli-darwin-arm64"
    )

    for item in "${targets[@]}"; do
        IFS="/" read -r os arch out <<< "$item"
        echo "Building $os/$arch -> $out ..."
        CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags="-s -w" -o "$out" ./cmd/inkanim-cli
    done
    echo "✓ Cross-compilation completed in $BUILD_DIR/dist/"
}

clean_artifacts() {
    echo "==> Cleaning build artifacts..."
    rm -rf "$BUILD_DIR" inkanim inkanim.exe inkanim-cli inkanim-cli.exe dist/
    mkdir -p "$BUILD_DIR"
    echo "✓ Cleaned."
}

case "$TARGET" in
    all)
        run_lint
        run_tests
        verify_wasm
        build_cli
        build_gui
        ;;
    cli)
        build_cli
        ;;
    gui)
        build_gui
        ;;
    gui-win)
        build_gui_win
        ;;
    ext-win)
        build_ext_win
        ;;
    ext-linux)
        build_ext_linux
        ;;
    ext-darwin)
        build_ext_darwin
        ;;
    ext-all)
        build_ext_all
        ;;
    wasm)
        build_wasm
        ;;
    wasm-check)
        verify_wasm
        ;;
    serve)
        serve_web
        ;;
    test)
        run_tests
        ;;
    test-update-golden)
        run_tests "true"
        ;;
    lint)
        run_lint
        ;;
    cross)
        cross_compile_cli
        ;;
    clean)
        clean_artifacts
        ;;
    *)
        echo "Unknown target: $TARGET. Available: all, cli, gui, gui-win, ext-win, ext-linux, ext-darwin, ext-all, wasm, serve, test, test-update-golden, lint, cross, clean"
        exit 1
        ;;
esac

