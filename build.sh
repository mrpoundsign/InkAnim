#!/usr/bin/env bash
set -euo pipefail

# InkAnim Build Script (Bash)
# Usage:
#   ./build.sh          # Run tests and build CLI & GUI
#   ./build.sh cli      # Build Pure-Go CLI
#   ./build.sh gui      # Build Desktop GUI
#   ./build.sh test     # Run unit tests
#   ./build.sh cross    # Cross-compile CLI for multiple targets
#   ./build.sh clean    # Clean build outputs

TARGET="${1:-all}"

build_cli() {
    echo "==> Building inkanim-cli (Pure Go, CGO_ENABLED=0)..."
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o inkanim-cli ./cmd/inkanim-cli
    echo "✓ Successfully built inkanim-cli"
}

build_gui() {
    echo "==> Building inkanim Desktop GUI..."
    CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o inkanim ./cmd/inkanim
    echo "✓ Successfully built inkanim"
}

run_tests() {
    echo "==> Running unit tests..."
    CGO_ENABLED=0 go test -tags ci ./... -v
    echo "✓ All tests passed!"
}

cross_compile_cli() {
    echo "==> Cross-compiling inkanim-cli for Windows, Linux, and macOS..."
    mkdir -p dist
    targets=(
        "windows/amd64/dist/inkanim-cli-windows-amd64.exe"
        "windows/arm64/dist/inkanim-cli-windows-arm64.exe"
        "linux/amd64/dist/inkanim-cli-linux-amd64"
        "linux/arm64/dist/inkanim-cli-linux-arm64"
        "darwin/amd64/dist/inkanim-cli-darwin-amd64"
        "darwin/arm64/dist/inkanim-cli-darwin-arm64"
    )

    for item in "${targets[@]}"; do
        IFS="/" read -r os arch out <<< "$item"
        echo "Building $os/$arch -> $out ..."
        CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags="-s -w" -o "$out" ./cmd/inkanim-cli
    done
    echo "✓ Cross-compilation completed in dist/"
}

clean_artifacts() {
    echo "==> Cleaning build artifacts..."
    rm -rf inkanim inkanim.exe inkanim-cli inkanim-cli.exe dist/
    echo "✓ Cleaned."
}

case "$TARGET" in
    all)
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
    test)
        run_tests
        ;;
    cross)
        cross_compile_cli
        ;;
    clean)
        clean_artifacts
        ;;
    *)
        echo "Unknown target: $TARGET. Available: all, cli, gui, test, cross, clean"
        exit 1
        ;;
esac
