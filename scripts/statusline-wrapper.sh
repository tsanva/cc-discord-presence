#!/bin/bash
# Statusline wrapper for Discord Rich Presence
# Saves statusline data for the Discord daemon, then pipes to original statusline (if exists)

DATA_FILE="$HOME/.claude/discord-presence-data.json"
ORIGINAL_STATUSLINE="$HOME/.claude/statusline.sh"

# Read JSON from stdin
read -r json_data

# Save for Discord presence (atomic write)
echo "$json_data" > "${DATA_FILE}.tmp" && mv "${DATA_FILE}.tmp" "$DATA_FILE"

# If there's an original statusline, pass the data to it
if [[ -x "$ORIGINAL_STATUSLINE" ]]; then
    echo "$json_data" | "$ORIGINAL_STATUSLINE"
fi
