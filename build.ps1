# InkAnim Build Script (PowerShell)
# Usage:
#   .\build.ps1                # Build both Desktop GUI and CLI
#   .\build.ps1 -Target gui    # Build Desktop GUI (with Zig CGo)
#   .\build.ps1 -Target cli    # Build Pure-Go CLI (Zero-CGo)
#   .\build.ps1 -Target test   # Run all unit tests
#   .\build.ps1 -Target lint   # Run golangci-lint
#   .\build.ps1 -Target cross  # Cross-compile CLI for Windows, Linux, macOS
#   .\build.ps1 -Target check  # Run goreleaser check
#   .\build.ps1 -Target clean  # Clean build artifacts

param(
    [ValidateSet("all", "gui", "cli", "wasm", "serve", "test", "lint", "cross", "check", "clean")]
    [string]$Target = "all"
)

$ErrorActionPreference = "Stop"

# Helper to find executable in PATH or standard install paths
function Find-Tool($name, $fallbackPaths) {
    $cmd = Get-Command $name -ErrorAction SilentlyContinue
    if ($cmd) { return $cmd.Source }
    foreach ($p in $fallbackPaths) {
        if (Test-Path $p) { return $p }
    }
    return $null
}

# Resolve Go
$GoExe = Find-Tool "go" @("C:\Program Files\Go\bin\go.exe")
if (-not $GoExe) {
    Write-Error "Go executable not found. Please install Go or ensure it is in PATH."
}

# Resolve Zig (for CGo GUI builds)
$userPath = [System.Environment]::GetEnvironmentVariable("Path", "User")
$sysPath = [System.Environment]::GetEnvironmentVariable("Path", "Machine")
$env:Path = "$userPath;$sysPath;$env:Path"

$BuildDir = Join-Path $PSScriptRoot "build"
if (-not (Test-Path $BuildDir)) { New-Item -ItemType Directory -Path $BuildDir | Out-Null }

$ZigExe = Find-Tool "zig" @(
    "$env:LOCALAPPDATA\Microsoft\WinGet\Packages\zig.zig_Microsoft.Winget.Source_8wekyb3d8bbwe\zig-windows-x86_64-0.16.0\zig.exe",
    "C:\Program Files\zig\zig.exe"
)

function Build-CLI {
    Write-Host "`n==> Building inkanim-cli (Pure Go, CGO_ENABLED=0)..." -ForegroundColor Cyan
    $env:CGO_ENABLED = "0"
    $outPath = Join-Path $BuildDir "inkanim-cli.exe"
    & $GoExe build -trimpath -ldflags="-s -w" -o $outPath ./cmd/inkanim-cli
    if ($LASTEXITCODE -eq 0) {
        Write-Host "[OK] Successfully built $outPath" -ForegroundColor Green
    } else {
        Write-Error "Failed to build inkanim-cli."
    }
}

function Build-GUI {
    Write-Host "`n==> Building inkanim Desktop GUI (with CGo)..." -ForegroundColor Cyan
    if (-not $ZigExe) {
        Write-Warning "Zig compiler not found. Attempting build with default system C compiler..."
        $env:CGO_ENABLED = "1"
    } else {
        Write-Host "Using Zig C compiler: $ZigExe" -ForegroundColor DarkGray
        $env:CGO_ENABLED = "1"
        $env:CC = "zig cc"
    }

    $outPath = Join-Path $BuildDir "inkanim.exe"
    & $GoExe build -trimpath -ldflags="-s -w" -o $outPath ./cmd/inkanim
    if ($LASTEXITCODE -eq 0) {
        Write-Host "[OK] Successfully built $outPath" -ForegroundColor Green
    } else {
        Write-Error "Failed to build inkanim.exe."
    }
}

function Run-Tests {
    Write-Host "`n==> Running unit tests..." -ForegroundColor Cyan
    & $GoExe test -v ./internal/...
    if ($LASTEXITCODE -eq 0) {
        Write-Host "[OK] All tests passed!" -ForegroundColor Green
    } else {
        Write-Error "Tests failed."
    }
}

function Cross-Compile-CLI {
    Write-Host "`n==> Cross-compiling inkanim-cli for Windows and Linux..." -ForegroundColor Cyan
    $distDir = Join-Path $BuildDir "dist"
    if (-not (Test-Path $distDir)) { New-Item -ItemType Directory -Path $distDir | Out-Null }

    $targets = @(
        @{ OS = "windows"; Arch = "amd64"; Out = (Join-Path $distDir "inkanim-cli-windows-amd64.exe") },
        @{ OS = "windows"; Arch = "arm64"; Out = (Join-Path $distDir "inkanim-cli-windows-arm64.exe") },
        @{ OS = "linux";   Arch = "amd64"; Out = (Join-Path $distDir "inkanim-cli-linux-amd64") },
        @{ OS = "linux";   Arch = "arm64"; Out = (Join-Path $distDir "inkanim-cli-linux-arm64") }
    )

    $env:CGO_ENABLED = "0"
    foreach ($t in $targets) {
        Write-Host "Building for $($t.OS)/$($t.Arch) -> $($t.Out) ..." -ForegroundColor DarkGray
        $env:GOOS = $t.OS
        $env:GOARCH = $t.Arch
        & $GoExe build -trimpath -ldflags="-s -w" -o $t.Out ./cmd/inkanim-cli
    }
    Write-Host "[OK] Cross-compilation completed in $distDir" -ForegroundColor Green
}

function Run-Check {
    Write-Host "`n==> Checking goreleaser configuration..." -ForegroundColor Cyan
    $goreleaser = Find-Tool "goreleaser" @()
    if (-not $goreleaser) {
        Write-Error "goreleaser not found in PATH."
    }
    & $goreleaser check
}

function Run-Lint {
    Write-Host "`n==> Running golangci-lint-v2..." -ForegroundColor Cyan
    $linter = Find-Tool "golangci-lint-v2" @(
        (Join-Path $env:USERPROFILE "go\bin\golangci-lint-v2.exe"),
        (Join-Path $env:USERPROFILE "go\bin\golangci-lint.exe")
    )
    if (-not $linter) {
        $linter = Find-Tool "golangci-lint" @(
            (Join-Path $env:USERPROFILE "go\bin\golangci-lint.exe")
        )
    }
    if (-not $linter) {
        Write-Error "golangci-lint-v2 not found in PATH or ~/go/bin."
    }
    & $linter run ./...
    if ($LASTEXITCODE -eq 0) {
        Write-Host "[OK] Lint passed!" -ForegroundColor Green
    } else {
        Write-Error "Lint checks failed."
    }
}

function Build-WASM {
    Write-Host "`n==> Packaging WebAssembly studio & assembling GitHub Pages site..." -ForegroundColor Cyan
    $pagesDir = Join-Path $BuildDir "gh-pages"
    $wasmTemp = Join-Path $BuildDir "wasm-tmp"
    if (Test-Path $pagesDir) { Remove-Item $pagesDir -Recurse -Force }
    if (Test-Path $wasmTemp) { Remove-Item $wasmTemp -Recurse -Force }
    New-Item -ItemType Directory -Path (Join-Path $pagesDir "demo") -Force | Out-Null
    New-Item -ItemType Directory -Path (Join-Path $pagesDir "samples") -Force | Out-Null
    New-Item -ItemType Directory -Path $wasmTemp -Force | Out-Null

    Write-Host "Compiling WebAssembly binary via Fyne..." -ForegroundColor DarkGray
    Push-Location $wasmTemp
    try {
        & $GoExe run fyne.io/fyne/v2/cmd/fyne@v2.8.1 package -os web --release `
            --sourceDir (Join-Path $PSScriptRoot "cmd\inkanim") `
            --icon (Join-Path $PSScriptRoot "assets\icon.png") `
            --name InkAnim
    } finally {
        Pop-Location
    }

    Copy-Item (Join-Path $wasmTemp "wasm\*") (Join-Path $pagesDir "demo") -Force
    Remove-Item $wasmTemp -Recurse -Force

    # Overlay our custom demo template
    Copy-Item (Join-Path $PSScriptRoot "web\demo\index.html") (Join-Path $pagesDir "demo\index.html") -Force

    # Copy landing page assets to root
    Copy-Item (Join-Path $PSScriptRoot "web\landing\*") $pagesDir -Recurse -Force

    # Copy sample SVGs
    Copy-Item (Join-Path $PSScriptRoot "web\samples\*") (Join-Path $pagesDir "samples") -Recurse -Force

    Write-Host "[OK] WebAssembly demo and landing page assembled in $pagesDir" -ForegroundColor Green
}

function Serve-Web {
    $pagesDir = Join-Path $BuildDir "gh-pages"
    $wasmBinary = Join-Path $pagesDir "demo\InkAnim.wasm"
    if (-not (Test-Path $wasmBinary)) {
        Write-Host "Web site not built yet. Building first..." -ForegroundColor DarkGray
        Build-WASM
    }
    & $GoExe run ./cmd/wasm-serve -dir $pagesDir -port 8080
}

function Clean-Artifacts {
    Write-Host "`n==> Cleaning build artifacts..." -ForegroundColor Yellow
    Remove-Item inkanim.exe, inkanim-cli.exe -ErrorAction SilentlyContinue
    if (Test-Path $BuildDir) { Remove-Item $BuildDir -Recurse -Force }
    New-Item -ItemType Directory -Path $BuildDir | Out-Null
    if (Test-Path "dist") { Remove-Item "dist" -Recurse -Force }
    Write-Host "[OK] Cleaned." -ForegroundColor Green
}

switch ($Target) {
    "all"   { Run-Lint; Run-Tests; Build-CLI; Build-GUI }
    "gui"   { Build-GUI }
    "cli"   { Build-CLI }
    "wasm"  { Build-WASM }
    "serve" { Serve-Web }
    "test"  { Run-Tests }
    "lint"  { Run-Lint }
    "cross" { Cross-Compile-CLI }
    "check" { Run-Check }
    "clean" { Clean-Artifacts }
}

