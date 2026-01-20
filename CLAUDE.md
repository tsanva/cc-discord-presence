# CLAUDE.md - Project Context for Claude Code

## Project Overview

**cc-discord-presence** is a Discord Rich Presence plugin for Claude Code. It displays real-time session information on Discord, including project name, git branch, model, session duration, token usage, and cost.

## Tech Stack

- **Language**: Go 1.25
- **Key Dependencies**:
  - `github.com/fsnotify/fsnotify` - Cross-platform file watching
  - `github.com/Microsoft/go-winio` - Windows named pipe support (Discord IPC)

## Project Structure

```
cc-discord-presence/
├── main.go               # Main entry - session tracking, data parsing, presence updates
├── discord/
│   ├── client.go         # Discord IPC client, Conn interface, presence logic
│   ├── conn_unix.go      # Unix socket connection (macOS/Linux)
│   └── conn_windows.go   # Named pipe connection (Windows, uses go-winio)
├── scripts/
│   ├── build.sh          # Cross-compile binaries for all platforms
│   ├── start.sh          # Plugin hook: starts daemon on SessionStart
│   ├── stop.sh           # Plugin hook: stops daemon on SessionEnd
│   ├── statusline-wrapper.sh  # Wrapper script (copied to ~/.claude/)
│   └── setup-statusline.sh    # One-time setup for statusline integration
├── .claude-plugin/
│   └── plugin.json       # Plugin manifest with SessionStart/SessionEnd hooks
├── go.mod
└── go.sum
```

## Key Concepts

### Discord IPC Protocol
- Unix socket at `/tmp/discord-ipc-{0-9}` (macOS/Linux)
- Named pipe on Windows
- Frame format: `[opcode:4 LE][length:4 LE][JSON payload]`
- Opcodes: 0 = Handshake, 1 = Frame

### Session Data Sources (Priority Order)

1. **Statusline Data** (`~/.claude/discord-presence-data.json`)
   - Most accurate - uses Claude Code's own calculations
   - Requires user to configure statusline wrapper
   - Provides: model display name, cost, tokens directly from Claude

2. **JSONL Fallback** (`~/.claude/projects/<encoded-path>/*.jsonl`)
   - Zero configuration needed
   - Parses session transcript files
   - Calculates cost based on model pricing table
   - Finds most recently modified file (handles multiple instances)

### Plugin Hooks
- **SessionStart**: Launches daemon via `scripts/start.sh`
- **SessionEnd**: Stops daemon via `scripts/stop.sh`
- Uses PID file at `~/.claude/discord-presence.pid`

### Session Tracking (Platform-specific)
- **macOS/Linux**: PID-based tracking via files in `~/.claude/discord-presence-sessions/`
- **Windows**: Refcount-based tracking via `~/.claude/discord-presence.refcount` (PPID unreliable on Windows)

### Model Pricing (Update when new models release)
Located at top of `main.go` in `modelPricing` and `modelDisplayNames` maps.

| Model | Input $/1M | Output $/1M |
|-------|-----------|-------------|
| Opus 4.5 | $15 | $75 |
| Sonnet 4.5 | $3 | $15 |
| Sonnet 4 | $3 | $15 |
| Haiku 4.5 | $1 | $5 |

## Development Commands

```bash
go build -o cc-discord-presence .   # Build binary
go run .                             # Run directly
./cc-discord-presence                # Run built binary
```

## Configuration

- Client ID `1455326944060248250` is hardcoded (shared "Clawd Code" Discord app)
- App icon set in Discord Developer Portal (used automatically for Rich Presence)

## Important Notes

- Polls every 3 seconds + uses file watcher
- Discord must be running for RPC to connect
- Graceful shutdown on SIGINT/SIGTERM
- Shows nudge message when using JSONL fallback encouraging statusline setup

## Releasing

Binaries are downloaded from GitHub Releases on first run. To create a new release:

1. **Update version** in these files:
   - `scripts/start.sh` - `VERSION="vX.X.X"`
   - `scripts/start.ps1` - `$Version = "vX.X.X"`
   - `.claude-plugin/plugin.json` - `"version": "X.X.X"` (no 'v' prefix)

2. **Build all binaries:**
   ```bash
   ./scripts/build.sh
   ```

3. **Commit and tag:**
   ```bash
   git add scripts/start.sh scripts/start.ps1 .claude-plugin/plugin.json
   git commit -m "Bump version to vX.X.X"
   git tag vX.X.X
   git push origin main --tags
   ```

4. **Create GitHub release:**
   ```bash
   gh release create vX.X.X bin/* --title "vX.X.X" --generate-notes
   ```
