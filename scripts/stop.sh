#!/bin/bash
# Stop Discord Rich Presence daemon
# WARNING: Linux support is untested. Please report issues on GitHub.

PID_FILE="$HOME/.claude/discord-presence.pid"
REF_COUNT_FILE="$HOME/.claude/discord-presence.refcount"

# Decrement reference count
REF_COUNT=0
if [[ -f "$REF_COUNT_FILE" ]]; then
    REF_COUNT=$(cat "$REF_COUNT_FILE" 2>/dev/null || echo 0)
fi
REF_COUNT=$((REF_COUNT - 1))

# Don't go below 0
if [[ $REF_COUNT -lt 0 ]]; then
    REF_COUNT=0
fi

echo "$REF_COUNT" > "$REF_COUNT_FILE"

# Only kill daemon if no more instances are using it
if [[ $REF_COUNT -gt 0 ]]; then
    echo "Discord Rich Presence still in use by $REF_COUNT instance(s)"
    exit 0
fi

# Clean up ref count file
rm -f "$REF_COUNT_FILE"

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
