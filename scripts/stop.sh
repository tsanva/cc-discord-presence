#!/bin/bash
# Stop Discord Rich Presence daemon
# WARNING: Linux support is untested. Please report issues on GitHub.

PID_FILE="$HOME/.claude/discord-presence.pid"
SESSIONS_DIR="$HOME/.claude/discord-presence-sessions"

# Get the Claude Code session PID (parent process)
SESSION_PID="$$"
if [[ -n "$PPID" ]]; then
    SESSION_PID="$PPID"
fi

# Remove this session's file
rm -f "$SESSIONS_DIR/$SESSION_PID"

# Count remaining active sessions (cleanup orphans while counting)
count_active_sessions() {
    local count=0
    [[ -d "$SESSIONS_DIR" ]] || { echo 0; return; }
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

# Only kill daemon if no more active sessions
if [[ $ACTIVE_SESSIONS -gt 0 ]]; then
    echo "Discord Rich Presence still in use by $ACTIVE_SESSIONS session(s)"
    exit 0
fi

# Clean up sessions directory
rm -rf "$SESSIONS_DIR"

# Stop the daemon
if [[ -f "$PID_FILE" ]]; then
    PID=$(cat "$PID_FILE")
    if kill -0 "$PID" 2>/dev/null; then
        kill "$PID" 2>/dev/null
        echo "Discord Rich Presence stopped (PID: $PID)"
    fi
    rm -f "$PID_FILE"
else
    # Fallback: kill by process name
    pkill -f cc-discord-presence 2>/dev/null || true
fi

# Clean up old refcount file if it exists (migration from old version)
rm -f "$HOME/.claude/discord-presence.refcount"
