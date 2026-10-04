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
VERSION="v1.0.5"

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
VERSION_FILE="$BINARY.version"

# If the daemon is already running, just exit, unless it runs a binary from
# another release: then stop it so the current one is downloaded and started.
# (claude plugin update replaces this script, never the binary.)
if [[ -f "$PID_FILE" ]]; then
    OLD_PID=$(cat "$PID_FILE")
    if process_exists "$OLD_PID"; then
        if [[ "$(cat "$VERSION_FILE" 2>/dev/null)" == "$VERSION" ]]; then
            echo "Discord Rich Presence already running (PID: $OLD_PID, sessions: $ACTIVE_SESSIONS)"
            exit 0
        fi
        echo "Updating the running daemon to ${VERSION}..."
        if $IS_WINDOWS; then
            taskkill //PID "$OLD_PID" //F > /dev/null 2>&1 || true
        else
            kill "$OLD_PID" 2>/dev/null || true
        fi
        sleep 1
    fi
fi

# Download the binary when it is missing or from another release. The
# version file beside it records which release it came from; installs from
# before it existed have none and update once.
if [[ ! -f "$BINARY" || "$(cat "$VERSION_FILE" 2>/dev/null)" != "$VERSION" ]]; then
    echo "Downloading cc-discord-presence ${VERSION} for ${OS}-${ARCH}..."

    DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${BINARY_NAME}"
    TMP_BINARY="$BINARY.download"

    # set -e is on: capture a failed download instead of exiting on it
    DOWNLOAD_STATUS=0
    if command -v curl &> /dev/null; then
        curl -fsSL "$DOWNLOAD_URL" -o "$TMP_BINARY" || DOWNLOAD_STATUS=$?
    elif command -v wget &> /dev/null; then
        wget -q "$DOWNLOAD_URL" -O "$TMP_BINARY" || DOWNLOAD_STATUS=$?
    else
        echo "Error: curl or wget required to download binary" >&2
        exit 1
    fi

    if [[ $DOWNLOAD_STATUS -eq 0 && -s "$TMP_BINARY" ]]; then
        mv -f "$TMP_BINARY" "$BINARY"
        if ! $IS_WINDOWS; then
            chmod +x "$BINARY"
        fi
        echo "$VERSION" > "$VERSION_FILE"
        echo "Downloaded successfully!"
    else
        # Keep a binary from an earlier release rather than none
        rm -f "$TMP_BINARY"
        echo "Warning: download failed; using the existing binary if present" >&2
    fi
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
