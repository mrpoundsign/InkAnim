#!/usr/bin/env bash
set -euo pipefail

# InkAnim Build Script (Bash)
# Usage:
#   ./build.sh                    # Run lint, tests, and build Desktop Studio GUI
#   ./build.sh gui                # Build Desktop Studio GUI into build/bin/
#   ./build.sh gui-win            # Build Desktop Studio GUI for Windows via MinGW into build/bin/
#   ./build.sh web                # Assemble static GitHub Pages site into build/web/
#   ./build.sh test               # Run unit tests
#   ./build.sh test-update-golden # Regenerate golden test images
#   ./build.sh lint               # Run go fix -diff & golangci-lint-v2
#   ./build.sh clean              # Clean build outputs

TARGET="${1:-all}"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BUILD_DIR="$ROOT_DIR/build"
BIN_DIR="$BUILD_DIR/bin"
mkdir -p "$BUILD_DIR" "$BIN_DIR"

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

build_gui() {
    echo "==> Building inkanim Desktop Studio GUI..."
    CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o "$BIN_DIR/inkanim" ./cmd/inkanim
    echo "✓ Successfully built $BIN_DIR/inkanim"
}

build_gui_win() {
    echo "==> Building inkanim Desktop Studio GUI for Windows (via MinGW)..."
    CC=x86_64-w64-mingw32-gcc CGO_ENABLED=1 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-H windowsgui -s -w" -o "$BIN_DIR/inkanim.exe" ./cmd/inkanim
    echo "✓ Successfully built $BIN_DIR/inkanim.exe"
}

build_web() {
    echo "==> Assembling static GitHub Pages site..."
    local PAGES_DIR="$BUILD_DIR/web"
    rm -rf "$PAGES_DIR"
    mkdir -p "$PAGES_DIR/samples"

    cp -r "$ROOT_DIR/web/landing/"* "$PAGES_DIR/"
    if [ -d "$ROOT_DIR/web/samples" ] && [ -n "$(ls -A "$ROOT_DIR/web/samples")" ]; then
        cp -r "$ROOT_DIR/web/samples/"* "$PAGES_DIR/samples/"
    fi

    echo "✓ Static GitHub Pages site assembled in $PAGES_DIR"
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

clean_artifacts() {
    echo "==> Cleaning build artifacts..."
    rm -rf "$BUILD_DIR" inkanim inkanim.exe dist/
    mkdir -p "$BIN_DIR"
    echo "✓ Cleaned."
}

case "$TARGET" in
    all)
        run_lint
        run_tests
        build_gui
        ;;
    gui)
        build_gui
        ;;
    gui-win)
        build_gui_win
        ;;
    web)
        build_web
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
    clean)
        clean_artifacts
        ;;
    *)
        echo "Unknown target: $TARGET. Available: all, gui, gui-win, web, test, test-update-golden, lint, clean"
        exit 1
        ;;
esac
