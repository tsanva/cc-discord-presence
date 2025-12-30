#!/bin/bash
# Build binaries for all supported platforms
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
BIN_DIR="$PROJECT_ROOT/bin"

echo "🔨 Building cc-discord-presence binaries..."

# Clean and create bin directory
rm -rf "$BIN_DIR"
mkdir -p "$BIN_DIR"

cd "$PROJECT_ROOT"

# Build for each platform
platforms=(
    "darwin/arm64"
    "darwin/amd64"
    "linux/amd64"
    "linux/arm64"
    "windows/amd64"
)

for platform in "${platforms[@]}"; do
    GOOS="${platform%/*}"
    GOARCH="${platform#*/}"

    output="$BIN_DIR/cc-discord-presence-${GOOS}-${GOARCH}"
    if [[ "$GOOS" == "windows" ]]; then
        output="${output}.exe"
    fi

    echo "  Building ${GOOS}/${GOARCH}..."
    GOOS="$GOOS" GOARCH="$GOARCH" go build -o "$output" .
done

echo ""
echo "✅ Build complete! Binaries:"
ls -la "$BIN_DIR"
