#!/bin/bash
# Automatic statusline setup for cc-discord-presence

CLAUDE_DIR="$HOME/.claude"
SETTINGS_FILE="$CLAUDE_DIR/settings.json"
WRAPPER_SRC="$(dirname "$0")/statusline-wrapper.sh"
WRAPPER_DEST="$CLAUDE_DIR/statusline-wrapper.sh"

echo "🔧 Setting up statusline integration for cc-discord-presence..."

# Check if settings.json exists
if [[ ! -f "$SETTINGS_FILE" ]]; then
    echo "Creating $SETTINGS_FILE..."
    echo '{}' > "$SETTINGS_FILE"
fi

# Copy wrapper script
cp "$WRAPPER_SRC" "$WRAPPER_DEST"
chmod +x "$WRAPPER_DEST"
echo "✓ Copied statusline wrapper to $WRAPPER_DEST"

# Check if jq is available
if ! command -v jq &> /dev/null; then
    echo ""
    echo "⚠️  jq not found. Please manually add to $SETTINGS_FILE:"
    echo '  "statusLine": {"command": "~/.claude/statusline-wrapper.sh", "type": "command"}'
    exit 1
fi

# Backup existing statusline if configured
EXISTING_CMD=$(jq -r '.statusLine.command // empty' "$SETTINGS_FILE" 2>/dev/null)
if [[ -n "$EXISTING_CMD" && "$EXISTING_CMD" != *"statusline-wrapper.sh" ]]; then
    # Expand ~ to $HOME for the check
    EXPANDED_CMD="${EXISTING_CMD/#\~/$HOME}"
    if [[ -f "$EXPANDED_CMD" ]]; then
        cp "$EXPANDED_CMD" "$CLAUDE_DIR/statusline.sh"
        chmod +x "$CLAUDE_DIR/statusline.sh"
        echo "✓ Backed up existing statusline to ~/.claude/statusline.sh"
    fi
fi

# Update settings.json
jq '.statusLine = {"command": "~/.claude/statusline-wrapper.sh", "type": "command"}' "$SETTINGS_FILE" > "$SETTINGS_FILE.tmp" \
    && mv "$SETTINGS_FILE.tmp" "$SETTINGS_FILE"
echo "✓ Updated $SETTINGS_FILE"

echo ""
echo "✅ Setup complete! Restart Claude Code to apply changes."
