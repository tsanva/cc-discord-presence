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
├── hooks/
│   ├── hooks.json        # Registers the hooks module (empty "hooks" keeps older versions loading)
│   └── register.ts       # Hooks module: writes discord-presence-module.json
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

1. **Hooks Module** (`~/.claude/discord-presence-module.json`)
   - Most accurate - Claude Code's own figures via the hooks module API
   - Zero configuration; runs in every host that loads hooks modules (terminal, desktop app, IDEs)
   - Same JSON shape as the statusline data; blanked to `{}` on session end

2. **Statusline Data** (`~/.claude/discord-presence-data.json`)
   - Same accuracy, for Claude Code versions without hooks modules
   - Requires user to configure statusline wrapper

3. **JSONL Fallback** (`~/.claude/projects/<encoded-path>/*.jsonl`)
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
go test -v ./...                     # Run all tests
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines. Key points:
- Run `go test -v ./...` before submitting PRs
- Update CHANGELOG.md with your changes
- Contributors are credited in release notes

## Configuration

- Client ID `1455326944060248250` is hardcoded (shared "Clawd Code" Discord app)
- App icon set in Discord Developer Portal (used automatically for Rich Presence)

## Important Notes

- Polls every 3 seconds + uses file watcher
- Discord must be running for RPC to connect
- Graceful shutdown on SIGINT/SIGTERM
- Shows nudge message when using JSONL fallback
- Tokens shown are context size (the last request's input, cached tokens included), not a session total
- Only pushes to Discord when the displayed text changes
- start.sh/start.ps1 re-download the binary when `<binary>.version` doesn't match `VERSION`

## Releasing

Binaries are downloaded from GitHub Releases by the start scripts, into `~/.claude/bin/` (outside the plugin folder, so `claude plugin update` never replaces them). The scripts re-download when `<binary>.version` doesn't match their version.

Users get a new plugin version from `main`, and the start scripts on `main` point at the new release, so the release must exist before `main` moves:

1. **On a branch, update the version** in all four places, then open a PR:
   - `scripts/start.sh` - `VERSION="vX.X.X"`
   - `scripts/start.ps1` - `$Version = "vX.X.X"`
   - `.claude-plugin/plugin.json` - `"version": "X.X.X"` (no 'v' prefix)
   - `.claude-plugin/marketplace.json` - `"version": "X.X.X"`

2. **Tag the PR's head commit** (rebased on current `main`) and push the tag:
   ```bash
   git tag -a vX.X.X -m "vX.X.X" && git push origin vX.X.X
   ```
   The Release workflow (`.github/workflows/release.yml`) checks that all four versions match the tag, runs the tests, builds all binaries and creates the GitHub release. If it fails, nothing has reached users: fix, delete the tag and release, retag.

3. **After the release succeeds, fast-forward `main`** to the tagged commit (GitHub marks the PR merged):
   ```bash
   git push origin vX.X.X^{commit}:main
   ```

4. **Replace the generated release notes** with hand-written ones: `gh release edit vX.X.X --notes-file notes.md`
