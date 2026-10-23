#!/usr/bin/env bash

# Build a versioned Windows release package.
# Usage: RELEASE_VERSION=v1.2.3 bash scripts/release.sh
# A release tag pointing exactly at HEAD can be used instead of RELEASE_VERSION.

set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$PROJECT_ROOT"

for dependency in git go flutter cygpath; do
    if ! command -v "$dependency" >/dev/null 2>&1; then
        echo "$dependency is required to build a Windows release." >&2
        exit 1
    fi
done

if [ -n "${RELEASE_VERSION:-}" ]; then
    VERSION="$RELEASE_VERSION"
else
    VERSION="$(git describe --tags --exact-match HEAD 2>/dev/null || true)"
    if [ -z "$VERSION" ]; then
        echo "The current commit must have an exact release tag or RELEASE_VERSION must be set." >&2
        exit 1
    fi
fi

if [[ ! "$VERSION" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
    echo "Invalid release version: $VERSION (expected vMAJOR.MINOR.PATCH, optionally with a prerelease suffix)." >&2
    exit 1
fi

HOST_OS="$(go env GOOS 2>/dev/null || true)"
HOST_ARCH="$(go env GOARCH 2>/dev/null || true)"
if [ "$HOST_OS/$HOST_ARCH" != "windows/amd64" ]; then
    echo "AgentPulse desktop releases must be built on Windows x64; current host is $HOST_OS/$HOST_ARCH." >&2
    exit 1
fi

POWERSHELL_BIN="$(command -v powershell.exe || command -v powershell || command -v pwsh || true)"
if [ -z "$POWERSHELL_BIN" ]; then
    echo "PowerShell is required to package the Windows release." >&2
    exit 1
fi

OUTPUT_DIR="${OUTPUT_DIR:-$PROJECT_ROOT/dist}"
mkdir -p "$OUTPUT_DIR"
OUTPUT_DIR="$(cd "$OUTPUT_DIR" && pwd)"
STAGING_DIR="$(mktemp -d "$OUTPUT_DIR/.agentpulse-release.XXXXXX")"
trap 'rm -rf -- "$STAGING_DIR"' EXIT

BUILD_SCRIPT="$(cygpath -w "$PROJECT_ROOT/scripts/build-windows.ps1")"
STAGING_DIR_WINDOWS="$(cygpath -aw "$STAGING_DIR")"
ARCHIVE_SOURCE="$STAGING_DIR/AgentPulse-Windows-x64.zip"
ARCHIVE_DESTINATION="$OUTPUT_DIR/AgentPulse-$VERSION-windows-amd64.zip"

echo "Building AgentPulse $VERSION for Windows x64..."
"$POWERSHELL_BIN" -NoProfile -ExecutionPolicy Bypass -File "$BUILD_SCRIPT" \
    -BuildDesktop -Version "$VERSION" -OutputDirectory "$STAGING_DIR_WINDOWS"

if [ ! -f "$ARCHIVE_SOURCE" ]; then
    echo "Windows release package was not produced: $ARCHIVE_SOURCE" >&2
    exit 1
fi

cp -f "$ARCHIVE_SOURCE" "$ARCHIVE_DESTINATION"
echo "Built AgentPulse $VERSION: $ARCHIVE_DESTINATION"
