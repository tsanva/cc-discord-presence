#!/bin/bash
# Start Discord Rich Presence daemon
# WARNING: Linux support is untested. Please report issues on GitHub.

set -e

PLUGIN_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN_DIR="$PLUGIN_ROOT/bin"
PID_FILE="$HOME/.claude/discord-presence.pid"
LOG_FILE="$HOME/.claude/discord-presence.log"
SESSIONS_DIR="$HOME/.claude/discord-presence-sessions"
REPO="tsanva/cc-discord-presence"
VERSION="v1.0.2"

# Ensure directories exist
mkdir -p "$HOME/.claude"
mkdir -p "$BIN_DIR"
mkdir -p "$SESSIONS_DIR"

# Get the Claude Code session PID (parent process)
SESSION_PID="$$"
# Try to get the actual parent PID if available
if [[ -n "$PPID" ]]; then
    SESSION_PID="$PPID"
fi

# Register this session
echo "$SESSION_PID" > "$SESSIONS_DIR/$SESSION_PID"

# Count active sessions (cleanup orphans while counting)
count_active_sessions() {
    local count=0
    for session_file in "$SESSIONS_DIR"/*; do
        [[ -f "$session_file" ]] || continue
        local pid
        pid=$(basename "$session_file")
        # Check if process is still running
        if kill -0 "$pid" 2>/dev/null; then
            count=$((count + 1))
        else
            # Orphaned session file, clean it up
            rm -f "$session_file"
        fi
    done
    echo "$count"
}

ACTIVE_SESSIONS=$(count_active_sessions)

# If daemon is already running, just exit
if [[ -f "$PID_FILE" ]]; then
    OLD_PID=$(cat "$PID_FILE")
    if kill -0 "$OLD_PID" 2>/dev/null; then
        echo "Discord Rich Presence already running (PID: $OLD_PID, sessions: $ACTIVE_SESSIONS)"
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

echo "Discord Rich Presence started (PID: $(cat "$PID_FILE"), sessions: $ACTIVE_SESSIONS)"
