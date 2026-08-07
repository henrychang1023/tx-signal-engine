#!/usr/bin/env bash
# Cross-compiles the web server into standalone, dependency-free binaries
# (the frontend is embedded via go:embed) for the common desktop platforms.
# Usage: scripts/build-release.sh
set -euo pipefail

cd "$(dirname "$0")/.."

DIST=dist
rm -rf "$DIST"
mkdir -p "$DIST"

build() {
	local goos=$1 goarch=$2 ext=$3
	local name="tx-signal-engine-server-${goos}-${goarch}${ext}"
	echo "building $name"
	GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 go build -o "$DIST/$name" ./cmd/server
}

build windows amd64 .exe
build darwin  amd64 ""
build darwin  arm64 ""
build linux   amd64 ""

echo
echo "done — binaries in $DIST/:"
ls -la "$DIST"
