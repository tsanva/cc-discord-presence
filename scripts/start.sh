#!/bin/bash
# Start Discord Rich Presence daemon
# WARNING: Linux support is untested. Please report issues on GitHub.

set -e

PLUGIN_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN_DIR="$PLUGIN_ROOT/bin"
PID_FILE="$HOME/.claude/discord-presence.pid"
LOG_FILE="$HOME/.claude/discord-presence.log"
REPO="tsanva/cc-discord-presence"
VERSION="v1.0.0"

# Ensure directories exist
mkdir -p "$HOME/.claude"
mkdir -p "$BIN_DIR"

# Detect platform and architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
    x86_64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
esac

BINARY_NAME="cc-discord-presence-${OS}-${ARCH}"
BINARY="$BIN_DIR/$BINARY_NAME"

# Download binary if not present
if [[ ! -x "$BINARY" ]]; then
    echo "Downloading cc-discord-presence for ${OS}-${ARCH}..."

    DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${BINARY_NAME}"

    if command -v curl &> /dev/null; then
        curl -fsSL "$DOWNLOAD_URL" -o "$BINARY"
    elif command -v wget &> /dev/null; then
        wget -q "$DOWNLOAD_URL" -O "$BINARY"
    else
        echo "Error: curl or wget required to download binary" >&2
        exit 1
    fi

    chmod +x "$BINARY"
    echo "Downloaded successfully!"
fi

if [[ ! -x "$BINARY" ]]; then
    echo "Error: Binary not found at $BINARY" >&2
    exit 1
fi

# Kill any existing instance
if [[ -f "$PID_FILE" ]]; then
    OLD_PID=$(cat "$PID_FILE")
    if kill -0 "$OLD_PID" 2>/dev/null; then
        kill "$OLD_PID" 2>/dev/null
        sleep 0.5
    fi
    rm -f "$PID_FILE"
fi

# Start the daemon in background
nohup "$BINARY" > "$LOG_FILE" 2>&1 &
echo $! > "$PID_FILE"

echo "Discord Rich Presence started (PID: $(cat "$PID_FILE"))"
