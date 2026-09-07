#!/usr/bin/env bash
set -euo pipefail

# InkAnim Build Script (Bash)
# Usage:
#   ./build.sh          # Run tests and build CLI & GUI into build/
#   ./build.sh cli      # Build Pure-Go CLI into build/
#   ./build.sh gui      # Build Desktop GUI into build/
#   ./build.sh test     # Run unit tests
#   ./build.sh cross    # Cross-compile CLI for multiple targets
#   ./build.sh clean    # Clean build outputs

TARGET="${1:-all}"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BUILD_DIR="$ROOT_DIR/build"
mkdir -p "$BUILD_DIR"

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
    echo "==> Running golangci-lint-v2..."
    if command -v golangci-lint-v2 &>/dev/null; then
        golangci-lint-v2 run ./...
    elif command -v golangci-lint &>/dev/null; then
        golangci-lint run ./...
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

build_wasm() {
    echo "==> Packaging WebAssembly studio & assembling GitHub Pages site..."
    local PAGES_DIR="$BUILD_DIR/gh-pages"
    local WASM_TEMP="$BUILD_DIR/wasm-tmp"
    rm -rf "$PAGES_DIR" "$WASM_TEMP"
    mkdir -p "$PAGES_DIR/demo" "$PAGES_DIR/samples" "$WASM_TEMP"

    echo "==> Compiling WebAssembly binary..."
    (
        cd "$WASM_TEMP"
        go run fyne.io/fyne/v2/cmd/fyne@v2.8.1 package -os web --release \
            --sourceDir "$ROOT_DIR/cmd/inkanim" \
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
    fi
    go run ./cmd/wasm-serve -dir "$PAGES_DIR" -port 8080
}

run_tests() {
    local UPDATE_GOLDEN="${1:-false}"
    echo "==> Running unit tests..."
    if [ "$UPDATE_GOLDEN" = "true" ]; then
        go test -v ./internal/... -update-golden
    else
        go test -v ./internal/...
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
        build_cli
        build_gui
        ;;
    cli)
        build_cli
        ;;
    gui)
        build_gui
        ;;
    wasm)
        build_wasm
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
        echo "Unknown target: $TARGET. Available: all, cli, gui, wasm, serve, test, test-update-golden, lint, cross, clean"
        exit 1
        ;;
esac

