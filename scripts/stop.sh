#!/bin/bash
# Stop Discord Rich Presence daemon
# WARNING: Linux support is untested. Please report issues on GitHub.

# Configuration
CLAUDE_DIR="$HOME/.claude"
PID_FILE="$CLAUDE_DIR/discord-presence.pid"
SESSIONS_DIR="$CLAUDE_DIR/discord-presence-sessions"

# Detect platform
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
IS_WINDOWS=false
case "$OS" in
    mingw*|msys*|cygwin*) IS_WINDOWS=true ;;
esac

# Cross-platform process operations
process_exists() {
    local pid=$1
    if $IS_WINDOWS; then
        tasklist //FI "PID eq $pid" 2>/dev/null | grep -q "$pid"
    else
        kill -0 "$pid" 2>/dev/null
    fi
}

kill_process() {
    local pid=$1
    if $IS_WINDOWS; then
        taskkill //F //PID "$pid" >/dev/null 2>&1
    else
        kill "$pid" 2>/dev/null
    fi
}

# Session tracking: mirror start.sh -- drop this session's PID marker, then
# recount by actual liveness (self-healing) rather than trusting a counter.
if $IS_WINDOWS; then
    # Must match the SESSION_PID source in start.sh (see comment there).
    SESSION_PID="${CLAUDE_PID:-$PPID}"
else
    SESSION_PID="${PPID:-$$}"
fi
rm -f "$SESSIONS_DIR/$SESSION_PID"

# Count remaining active sessions and clean up orphans
ACTIVE_SESSIONS=0
if [[ -d "$SESSIONS_DIR" ]]; then
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

if [[ $ACTIVE_SESSIONS -gt 0 ]]; then
    echo "Discord Rich Presence still in use by $ACTIVE_SESSIONS session(s)"
    exit 0
fi
rm -rf "$SESSIONS_DIR"

# Stop the daemon
if [[ -f "$PID_FILE" ]]; then
    PID=$(cat "$PID_FILE")
    if process_exists "$PID"; then
        kill_process "$PID"
        echo "Discord Rich Presence stopped (PID: $PID)"
    fi
    rm -f "$PID_FILE"
else
    # Fallback: kill by process name
    if $IS_WINDOWS; then
        taskkill //F //IM "cc-discord-presence-windows-amd64.exe" >/dev/null 2>&1 || true
    else
        pkill -f cc-discord-presence 2>/dev/null || true
    fi
fi
