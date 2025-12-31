#!/bin/bash
# Start Discord Rich Presence daemon
# WARNING: Linux support is untested. Please report issues on GitHub.

set -e

PLUGIN_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN_DIR="$PLUGIN_ROOT/bin"
PID_FILE="$HOME/.claude/discord-presence.pid"
LOG_FILE="$HOME/.claude/discord-presence.log"
REF_COUNT_FILE="$HOME/.claude/discord-presence.refcount"
REPO="tsanva/cc-discord-presence"
VERSION="v1.0.1-dev"

# Ensure directories exist
mkdir -p "$HOME/.claude"
mkdir -p "$BIN_DIR"

# Reference counting for multiple instances
REF_COUNT=0
if [[ -f "$REF_COUNT_FILE" ]]; then
    REF_COUNT=$(cat "$REF_COUNT_FILE" 2>/dev/null || echo 0)
fi
REF_COUNT=$((REF_COUNT + 1))
echo "$REF_COUNT" > "$REF_COUNT_FILE"

# If daemon is already running, just increment count and exit
if [[ -f "$PID_FILE" ]]; then
    OLD_PID=$(cat "$PID_FILE")
    if kill -0 "$OLD_PID" 2>/dev/null; then
        echo "Discord Rich Presence already running (PID: $OLD_PID, instances: $REF_COUNT)"
        exit 0
    fi
fi

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

# Start the daemon in background
nohup "$BINARY" > "$LOG_FILE" 2>&1 &
echo $! > "$PID_FILE"

echo "Discord Rich Presence started (PID: $(cat "$PID_FILE"), instances: $REF_COUNT)"
