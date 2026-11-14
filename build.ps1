# InkAnim Build Script (PowerShell)
# Usage:
#   .\build.ps1                # Run lint, tests, and build Desktop Studio GUI
#   .\build.ps1 -Target gui       # Build Desktop Studio GUI (with Zig CGo) -> build\bin\inkanim.exe
#   .\build.ps1 -Target installer # Build Windows Guided Setup Installer -> build\bin\InkAnim-Setup.exe
#   .\build.ps1 -Target web       # Assemble static GitHub Pages site -> build\web\
#   .\build.ps1 -Target test      # Run all unit tests
#   .\build.ps1 -Target lint      # Run go fix -diff & golangci-lint-v2
#   .\build.ps1 -Target check     # Run goreleaser check
#   .\build.ps1 -Target clean     # Clean build artifacts

param(
    [ValidateSet("all", "gui", "installer", "web", "test", "test-update-golden", "lint", "check", "clean")]
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
$BinDir = Join-Path $BuildDir "bin"
if (-not (Test-Path $BuildDir)) { New-Item -ItemType Directory -Path $BuildDir | Out-Null }
if (-not (Test-Path $BinDir)) { New-Item -ItemType Directory -Path $BinDir | Out-Null }

$ZigExe = Find-Tool "zig" @(
    "$env:LOCALAPPDATA\Microsoft\WinGet\Packages\zig.zig_Microsoft.Winget.Source_8wekyb3d8bbwe\zig-windows-x86_64-0.16.0\zig.exe",
    "C:\Program Files\zig\zig.exe"
)

function New-WinResSyso {
    param(
        [string]$OutPrefix,
        [string]$ProductName,
        [string]$Description,
        [string]$OriginalFilename
    )
    $iconPath = Join-Path $PSScriptRoot "assets\icon.png"
    if (-not (Test-Path $iconPath)) { return }

    $ver = "0.0.0"
    if ($env:VERSION) {
        $ver = $env:VERSION.TrimStart('v')
    }

    & $GoExe tool go-winres simply `
        --arch amd64 `
        --out $OutPrefix `
        --icon $iconPath `
        --manifest gui `
        --product-name $ProductName `
        --file-description $Description `
        --product-version $ver `
        --file-version $ver `
        --original-filename $OriginalFilename
    if ($LASTEXITCODE -ne 0) {
        Write-Error "Failed to generate Windows resource (.syso) for $OriginalFilename."
    }
}

function Build-GUI {
    Write-Host "`n==> Building inkanim Desktop Studio GUI (with CGo)..." -ForegroundColor Cyan
    if (-not $ZigExe) {
        Write-Warning "Zig compiler not found. Attempting build with default system C compiler..."
        $env:CGO_ENABLED = "1"
    } else {
        Write-Host "Using Zig C compiler: $ZigExe" -ForegroundColor DarkGray
        $env:CGO_ENABLED = "1"
        $env:CC = "zig cc"
    }

    if (-not (Test-Path $BinDir)) { New-Item -ItemType Directory -Path $BinDir | Out-Null }
    $outPath = Join-Path $BinDir "inkanim.exe"
    $sysoPrefix = Join-Path $PSScriptRoot "cmd\inkanim\rsrc"
    $sysoPath = "${sysoPrefix}_windows_amd64.syso"
    try {
        New-WinResSyso -OutPrefix $sysoPrefix -ProductName "InkAnim" -Description "InkAnim Desktop Studio" -OriginalFilename "inkanim.exe"
        & $GoExe build -trimpath -ldflags="-s -w -H=windowsgui -extldflags=-Wl,--subsystem,windows" -o $outPath ./cmd/inkanim
        if ($LASTEXITCODE -eq 0) {
            Write-Host "[OK] Successfully built $outPath" -ForegroundColor Green
        } else {
            Write-Error "Failed to build inkanim.exe."
        }
    } finally {
        Remove-Item $sysoPath -Force -ErrorAction SilentlyContinue
    }
}

function Build-Installer {
    $guiExe = Join-Path $BinDir "inkanim.exe"
    if (-not (Test-Path $guiExe)) {
        Build-GUI
    }

    Write-Host "`n==> Building InkAnim Windows Guided Setup Installer..." -ForegroundColor Cyan
    if (-not $ZigExe) {
        $env:CGO_ENABLED = "1"
    } else {
        $env:CGO_ENABLED = "1"
        $env:CC = "zig cc"
    }

    $payloadDir = Join-Path $PSScriptRoot "cmd\installer\payload"
    if (-not (Test-Path $payloadDir)) { New-Item -ItemType Directory -Path $payloadDir | Out-Null }

    $cleanupPayload = {
        Get-ChildItem -Path $payloadDir -Force | Where-Object { $_.Name -ne "placeholder.txt" } | Remove-Item -Recurse -Force -ErrorAction SilentlyContinue
    }

    try {
        & $cleanupPayload
        Copy-Item $guiExe (Join-Path $payloadDir "inkanim.exe") -Force
        if (Test-Path (Join-Path $PSScriptRoot "LICENSE")) { Copy-Item (Join-Path $PSScriptRoot "LICENSE") (Join-Path $payloadDir "LICENSE") -Force }
        if (Test-Path (Join-Path $PSScriptRoot "README.md")) { Copy-Item (Join-Path $PSScriptRoot "README.md") (Join-Path $payloadDir "README.md") -Force }
        $sampleSvg = Join-Path $PSScriptRoot "testdata\pendulum.svg"
        if (Test-Path $sampleSvg) { Copy-Item $sampleSvg (Join-Path $payloadDir "pendulum.svg") -Force }

        $ldflags = "-s -w -H=windowsgui -extldflags=-Wl,--subsystem,windows"
        if ($env:VERSION) {
            $ver = $env:VERSION.TrimStart('v')
            $ldflags = "$ldflags -X main.AppVersion=$ver"
        }

        $sysoPrefix = Join-Path $PSScriptRoot "cmd\installer\rsrc"
        $sysoPath = "${sysoPrefix}_windows_amd64.syso"
        New-WinResSyso -OutPrefix $sysoPrefix -ProductName "InkAnim Setup" -Description "InkAnim Guided Setup Installer" -OriginalFilename "InkAnim-Setup.exe"

        $setupOut = Join-Path $BinDir "InkAnim-Setup.exe"
        & $GoExe build -trimpath -ldflags="$ldflags" -o $setupOut ./cmd/installer
        if ($LASTEXITCODE -eq 0) {
            Write-Host "[OK] Successfully built $setupOut" -ForegroundColor Green
        } else {
            Write-Error "Failed to build InkAnim-Setup.exe."
        }
    } finally {
        & $cleanupPayload
        Remove-Item (Join-Path $PSScriptRoot "cmd\installer\rsrc_windows_amd64.syso") -Force -ErrorAction SilentlyContinue
    }
}

function Run-Tests {
    param([switch]$UpdateGolden)
    Write-Host "`n==> Running unit tests..." -ForegroundColor Cyan
    if ($ZigExe) {
        $env:CGO_ENABLED = "1"
        $env:CC = "zig cc"
    }
    if ($UpdateGolden) {
        & $GoExe test -v ./cmd/... ./internal/... ./pkg/... -update-golden
    } else {
        & $GoExe test -v ./cmd/... ./internal/... ./pkg/...
    }
    if ($LASTEXITCODE -eq 0) {
        Write-Host "[OK] All tests passed!" -ForegroundColor Green
    } else {
        Write-Error "Tests failed."
    }
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
    Write-Host "`n==> Verifying code modernization (go fix -diff)..." -ForegroundColor Cyan
    if ($ZigExe) {
        $env:CGO_ENABLED = "1"
        $env:CC = "zig cc"
    }
    $fixDiff = & $GoExe fix -diff ./...
    if ($LASTEXITCODE -ne 0) {
        Write-Error "go fix -diff failed."
    }
    if ($fixDiff) {
        Write-Host $fixDiff
        Write-Error "Unfixed code modernization diffs found. Run 'go fix ./...' to apply them."
    }
    Write-Host "[OK] Code modernizations verified!" -ForegroundColor Green

    Write-Host "`n==> Running golangci-lint-v2..." -ForegroundColor Cyan
    $linter = Find-Tool "golangci-lint-v2" @(
        (Join-Path $env:USERPROFILE "go\bin\golangci-lint-v2.exe")
    )
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

function Build-Web {
    Write-Host "`n==> Assembling static GitHub Pages site..." -ForegroundColor Cyan
    $pagesDir = Join-Path $BuildDir "web"
    if (Test-Path $pagesDir) { Remove-Item $pagesDir -Recurse -Force }
    New-Item -ItemType Directory -Path (Join-Path $pagesDir "samples") -Force | Out-Null

    Copy-Item (Join-Path $PSScriptRoot "web\landing\*") $pagesDir -Recurse -Force
    $samplesSrc = Join-Path $PSScriptRoot "web\samples"
    if (Test-Path $samplesSrc) {
        Copy-Item (Join-Path $samplesSrc "*") (Join-Path $pagesDir "samples") -Recurse -Force
    }

    Write-Host "[OK] Static GitHub Pages site assembled in $pagesDir" -ForegroundColor Green
}

function Clean-Artifacts {
    Write-Host "`n==> Cleaning build artifacts..." -ForegroundColor Yellow
    Remove-Item inkanim.exe -ErrorAction SilentlyContinue
    if (Test-Path $BuildDir) { Remove-Item $BuildDir -Recurse -Force }
    New-Item -ItemType Directory -Path $BinDir -Force | Out-Null
    if (Test-Path "dist") { Remove-Item "dist" -Recurse -Force }
    Write-Host "[OK] Cleaned." -ForegroundColor Green
}

switch ($Target) {
    "all"                 { Run-Lint; Run-Tests; Build-GUI }
    "gui"                 { Build-GUI }
    "installer"           { Build-Installer }
    "web"                 { Build-Web }
    "test"                { Run-Tests }
    "test-update-golden"  { Run-Tests -UpdateGolden }
    "lint"                { Run-Lint }
    "check"               { Run-Check }
    "clean"               { Clean-Artifacts }
}
