#!/bin/bash
set -e

# InkAnim Web Deployment Script
# 
# Builds the WebAssembly payload and syncs it to the public GCS bucket.
# Requires the gcloud CLI (gsutil) to be installed and authenticated.

cd "$(dirname "$0")/.."

echo "==> Building WebAssembly payload..."
./build.sh wasm

echo "==> Syncing build/web/ to gs://inkanim.poundsigndesign.com/ ..."
# Use rsync with -d to delete remote files that no longer exist locally
gsutil -m rsync -r -d build/web/ gs://inkanim.poundsigndesign.com/

echo "==> Done! Deployment live at https://inkanim.poundsigndesign.com/"
