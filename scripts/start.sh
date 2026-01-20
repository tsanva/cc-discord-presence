#!/bin/bash
# Start Discord Rich Presence daemon
# WARNING: Linux support is untested. Please report issues on GitHub.

set -e

# Configuration
CLAUDE_DIR="$HOME/.claude"
BIN_DIR="$CLAUDE_DIR/bin"
PID_FILE="$CLAUDE_DIR/discord-presence.pid"
LOG_FILE="$CLAUDE_DIR/discord-presence.log"
SESSIONS_DIR="$CLAUDE_DIR/discord-presence-sessions"
REFCOUNT_FILE="$CLAUDE_DIR/discord-presence.refcount"
REPO="tsanva/cc-discord-presence"
VERSION="v1.0.3"

# Detect platform
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
IS_WINDOWS=false
case "$OS" in
    mingw*|msys*|cygwin*) IS_WINDOWS=true; OS="windows" ;;
esac

# Cross-platform process check
process_exists() {
    local pid=$1
    if $IS_WINDOWS; then
        tasklist //FI "PID eq $pid" 2>/dev/null | grep -q "$pid"
    else
        kill -0 "$pid" 2>/dev/null
    fi
}

# Ensure directories exist
mkdir -p "$CLAUDE_DIR" "$BIN_DIR" "$SESSIONS_DIR"

# Session tracking: Windows uses refcount (PPID unreliable), Unix uses PID files
if $IS_WINDOWS; then
    CURRENT_COUNT=$(cat "$REFCOUNT_FILE" 2>/dev/null || echo "0")
    ACTIVE_SESSIONS=$((CURRENT_COUNT + 1))
    echo "$ACTIVE_SESSIONS" > "$REFCOUNT_FILE"
else
    SESSION_PID="${PPID:-$$}"
    echo "$SESSION_PID" > "$SESSIONS_DIR/$SESSION_PID"

    # Count active sessions and clean up orphans
    ACTIVE_SESSIONS=0
    for session_file in "$SESSIONS_DIR"/*; do
        [[ -f "$session_file" ]] || continue
        pid=$(basename "$session_file")
        if process_exists "$pid"; then
            ACTIVE_SESSIONS=$((ACTIVE_SESSIONS + 1))
        else
            rm -f "$session_file"
        fi
    done
fi

# If daemon is already running, just exit
if [[ -f "$PID_FILE" ]]; then
    OLD_PID=$(cat "$PID_FILE")
    if process_exists "$OLD_PID"; then
        echo "Discord Rich Presence already running (PID: $OLD_PID, sessions: $ACTIVE_SESSIONS)"
        exit 0
    fi
fi

# Detect architecture
ARCH=$(uname -m)
case "$ARCH" in
    x86_64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
esac

BINARY_NAME="cc-discord-presence-${OS}-${ARCH}"
if [[ "$OS" == "windows" ]]; then
    BINARY_NAME="${BINARY_NAME}.exe"
fi
BINARY="$BIN_DIR/$BINARY_NAME"

# Download binary if not present
if [[ ! -f "$BINARY" ]]; then
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

    if ! $IS_WINDOWS; then
        chmod +x "$BINARY"
    fi
    echo "Downloaded successfully!"
fi

if [[ ! -f "$BINARY" ]]; then
    echo "Error: Binary not found at $BINARY" >&2
    exit 1
fi

# Start the daemon in background
if $IS_WINDOWS; then
    # On Windows, convert path to Windows format and use PowerShell
    WIN_BINARY=$(cygpath -w "$BINARY" 2>/dev/null || echo "$BINARY")
    WIN_PID_FILE=$(cygpath -w "$PID_FILE" 2>/dev/null || echo "$PID_FILE")

    # Use PowerShell to start the process and capture PID (hidden window)
    powershell.exe -NoProfile -WindowStyle Hidden -Command '$process = Start-Process -FilePath "'"$WIN_BINARY"'" -WindowStyle Hidden -PassThru; $process.Id | Out-File -FilePath "'"$WIN_PID_FILE"'" -Encoding ASCII -NoNewline' 2>/dev/null
else
    nohup "$BINARY" > "$LOG_FILE" 2>&1 &
    echo $! > "$PID_FILE"
fi

echo "Discord Rich Presence started (PID: $(cat "$PID_FILE" 2>/dev/null || echo "unknown"), sessions: $ACTIVE_SESSIONS)"
