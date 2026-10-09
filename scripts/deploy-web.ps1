$ErrorActionPreference = 'Stop'

# InkAnim Web Deployment Script
# 
# Builds the WebAssembly payload and syncs it to the public GCS bucket.
# Requires the gcloud CLI (gsutil) to be installed and authenticated.

$repoRoot = Split-Path -Parent $MyInvocation.MyCommand.Definition | Split-Path -Parent
Set-Location $repoRoot

Write-Host "==> Building WebAssembly payload..." -ForegroundColor Cyan
.\build.ps1 -Target wasm

Write-Host "==> Syncing build/web/ to gs://inkanim.poundsigndesign.com/ ..." -ForegroundColor Cyan
# Use rsync with -d to delete remote files that no longer exist locally
gsutil -m rsync -r -d build/web/ gs://inkanim.poundsigndesign.com/

Write-Host "==> Done! Deployment live at https://inkanim.poundsigndesign.com/" -ForegroundColor Green
