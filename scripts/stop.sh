#!/bin/bash
# Stop Discord Rich Presence daemon
# WARNING: Linux support is untested. Please report issues on GitHub.

PID_FILE="$HOME/.claude/discord-presence.pid"
SESSIONS_DIR="$HOME/.claude/discord-presence-sessions"
REFCOUNT_FILE="$HOME/.claude/discord-presence.refcount"

# Detect Windows (Git Bash/MSYS/Cygwin)
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
IS_WINDOWS=false
case "$OS" in
    mingw*|msys*|cygwin*) IS_WINDOWS=true ;;
esac

# Process check function (cross-platform)
process_exists() {
    local pid=$1
    if $IS_WINDOWS; then
        tasklist //FI "PID eq $pid" 2>/dev/null | grep -q "$pid"
    else
        kill -0 "$pid" 2>/dev/null
    fi
}

# Kill process function (cross-platform)
kill_process() {
    local pid=$1
    if $IS_WINDOWS; then
        taskkill //F //PID "$pid" >/dev/null 2>&1
    else
        kill "$pid" 2>/dev/null
    fi
}

# Session tracking depends on platform
if $IS_WINDOWS; then
    # Windows: Use refcount approach
    if [[ -f "$REFCOUNT_FILE" ]]; then
        CURRENT_COUNT=$(cat "$REFCOUNT_FILE" 2>/dev/null || echo "1")
    else
        CURRENT_COUNT=1
    fi
    ACTIVE_SESSIONS=$((CURRENT_COUNT - 1))
    if [[ $ACTIVE_SESSIONS -lt 0 ]]; then
        ACTIVE_SESSIONS=0
    fi

    if [[ $ACTIVE_SESSIONS -gt 0 ]]; then
        echo "$ACTIVE_SESSIONS" > "$REFCOUNT_FILE"
        echo "Discord Rich Presence still in use by $ACTIVE_SESSIONS session(s)"
        exit 0
    fi

    # No more sessions, clean up refcount file
    rm -f "$REFCOUNT_FILE"
else
    # Unix: Use PID-based tracking
    SESSION_PID="$$"
    if [[ -n "$PPID" ]]; then
        SESSION_PID="$PPID"
    fi
    rm -f "$SESSIONS_DIR/$SESSION_PID"

    # Count remaining active sessions (cleanup orphans while counting)
    count_active_sessions() {
        local count=0
        [[ -d "$SESSIONS_DIR" ]] || { echo 0; return; }
        for session_file in "$SESSIONS_DIR"/*; do
            [[ -f "$session_file" ]] || continue
            local pid
            pid=$(basename "$session_file")
            if process_exists "$pid"; then
                count=$((count + 1))
            else
                rm -f "$session_file"
            fi
        done
        echo "$count"
    }

    ACTIVE_SESSIONS=$(count_active_sessions)

    if [[ $ACTIVE_SESSIONS -gt 0 ]]; then
        echo "Discord Rich Presence still in use by $ACTIVE_SESSIONS session(s)"
        exit 0
    fi

    # Clean up sessions directory
    rm -rf "$SESSIONS_DIR"
fi

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
