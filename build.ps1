# InkAnim Build Script (PowerShell)
# Usage:
#   .\build.ps1                # Build both Desktop GUI and CLI
#   .\build.ps1 -Target gui    # Build Desktop GUI (with Zig CGo)
#   .\build.ps1 -Target cli    # Build Pure-Go CLI (Zero-CGo)
#   .\build.ps1 -Target test   # Run all unit tests
#   .\build.ps1 -Target cross  # Cross-compile CLI for Windows, Linux, macOS
#   .\build.ps1 -Target check  # Run goreleaser check
#   .\build.ps1 -Target clean  # Clean build artifacts

param(
    [ValidateSet("all", "gui", "cli", "test", "cross", "check", "clean")]
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

$ZigExe = Find-Tool "zig" @(
    "$env:LOCALAPPDATA\Microsoft\WinGet\Packages\zig.zig_Microsoft.Winget.Source_8wekyb3d8bbwe\zig-windows-x86_64-0.16.0\zig.exe",
    "C:\Program Files\zig\zig.exe"
)

function Build-CLI {
    Write-Host "`n==> Building inkanim-cli (Pure Go, CGO_ENABLED=0)..." -ForegroundColor Cyan
    $env:CGO_ENABLED = "0"
    & $GoExe build -trimpath -ldflags="-s -w" -o inkanim-cli.exe ./cmd/inkanim-cli
    if ($LASTEXITCODE -eq 0) {
        Write-Host "[OK] Successfully built inkanim-cli.exe" -ForegroundColor Green
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

    & $GoExe build -trimpath -ldflags="-s -w" -o inkanim.exe ./cmd/inkanim
    if ($LASTEXITCODE -eq 0) {
        Write-Host "[OK] Successfully built inkanim.exe" -ForegroundColor Green
    } else {
        Write-Error "Failed to build inkanim.exe."
    }
}

function Run-Tests {
    Write-Host "`n==> Running unit tests..." -ForegroundColor Cyan
    $env:CGO_ENABLED = "0"
    & $GoExe test -tags ci ./... -v
    if ($LASTEXITCODE -eq 0) {
        Write-Host "[OK] All tests passed!" -ForegroundColor Green
    } else {
        Write-Error "Tests failed."
    }
}

function Cross-Compile-CLI {
    Write-Host "`n==> Cross-compiling inkanim-cli for Windows, Linux, and macOS..." -ForegroundColor Cyan
    if (-not (Test-Path "dist")) { New-Item -ItemType Directory -Path "dist" | Out-Null }

    $targets = @(
        @{ OS = "windows"; Arch = "amd64"; Out = "dist/inkanim-cli-windows-amd64.exe" },
        @{ OS = "windows"; Arch = "arm64"; Out = "dist/inkanim-cli-windows-arm64.exe" },
        @{ OS = "linux";   Arch = "amd64"; Out = "dist/inkanim-cli-linux-amd64" },
        @{ OS = "linux";   Arch = "arm64"; Out = "dist/inkanim-cli-linux-arm64" },
        @{ OS = "darwin";  Arch = "amd64"; Out = "dist/inkanim-cli-darwin-amd64" },
        @{ OS = "darwin";  Arch = "arm64"; Out = "dist/inkanim-cli-darwin-arm64" }
    )

    $env:CGO_ENABLED = "0"
    foreach ($t in $targets) {
        Write-Host "Building for $($t.OS)/$($t.Arch) -> $($t.Out) ..." -ForegroundColor DarkGray
        $env:GOOS = $t.OS
        $env:GOARCH = $t.Arch
        & $GoExe build -trimpath -ldflags="-s -w" -o $t.Out ./cmd/inkanim-cli
    }
    Write-Host "[OK] Cross-compilation completed in dist/" -ForegroundColor Green
}

function Run-Check {
    Write-Host "`n==> Checking goreleaser configuration..." -ForegroundColor Cyan
    $goreleaser = Find-Tool "goreleaser" @()
    if (-not $goreleaser) {
        Write-Error "goreleaser not found in PATH."
    }
    & $goreleaser check
}

function Clean-Artifacts {
    Write-Host "`n==> Cleaning build artifacts..." -ForegroundColor Yellow
    Remove-Item inkanim.exe, inkanim-cli.exe -ErrorAction SilentlyContinue
    if (Test-Path "dist") { Remove-Item "dist" -Recurse -Force }
    Write-Host "[OK] Cleaned." -ForegroundColor Green
}

switch ($Target) {
    "all"   { Run-Tests; Build-CLI; Build-GUI }
    "gui"   { Build-GUI }
    "cli"   { Build-CLI }
    "test"  { Run-Tests }
    "cross" { Cross-Compile-CLI }
    "check" { Run-Check }
    "clean" { Clean-Artifacts }
}
