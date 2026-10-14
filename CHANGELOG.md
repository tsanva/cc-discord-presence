# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed
- `start.sh` and `stop.sh`, which the plugin hooks run on every platform (Windows included, through Git Bash), trust the pid file only when that pid is a `cc-discord-presence` process, as `start.ps1` and `stop.ps1` do since 1.0.6
  - A pid reused after the daemon exited (reboot, Discord not running) made `start.sh` report the daemon as running without starting it, and `stop.sh` force-stop the unrelated program
- On Windows the daemon's output now reaches `~/.claude/discord-presence.log`, which the README points to; `start.sh` discarded it, so a failed Discord connection left no trace
  - The daemon writes the file itself (`--log <path>`): redirecting through `Start-Process` would make it inherit the hook's output pipe, and Claude Code would wait on the SessionStart hook until its timeout
- On Windows the daemon kept running, and Discord kept showing the card, after every Claude Code session had closed
  - SessionStart runs again after `/compact` (manual or automatic) with no SessionEnd to match, so the session refcount never got back to zero; a crashed session or a closed window did the same
  - Sessions are now tracked with one marker per session id from the hook input, in `~/.claude/discord-presence-sessions/`, replacing `discord-presence.refcount`; a marker whose transcript hasn't changed for 12 hours is dropped
- Reading a reply from Discord no longer assumes the whole frame arrives in one read; a frame split across reads was misparsed
- The card no longer disappears for good when Discord starts after Claude Code, or quits or restarts (for an update) during a session
  - The daemon exited when Discord wasn't running at startup, and never reconnected after losing the connection
  - It now retries every 3 seconds, notices a closed connection even while the card text stays the same, and sends the current activity again after reconnecting
- The JSONL fallback reads past transcript lines longer than 1 MB (large tool results); it stopped at the first one, so the card showed the model and token count from before it
- The JSONL fallback's model name and cost were wrong for most sessions
  - Models were looked up by exact ID in a table of four (one with an ID that doesn't exist), so current models showed as "Opus" and were priced as Sonnet 4; names now come from the model ID ("Opus 5.5"), Bedrock and Vertex AI forms included
  - Prices are the current list prices, including cache writes (5-minute and 1-hour) and reads, which were ignored, fast mode and US-only inference; each response is priced by its own model
  - Each response was counted once per content block, since Claude Code writes a line per block with the same usage
  - A model without a known price leaves the cost off the card instead of guessing
  - Claude Code's `<synthetic>` notices (API errors, interruptions) no longer replace the model and token count with "Claude" and 0
- The card no longer shows a session that is over for as long as the daemon runs
  - A session without activity for 15 minutes shows as "Idling in" while Claude Code is open in some terminal, and is cleared when Claude Code isn't running (it crashed, or its window closed without the SessionEnd hook)
  - The daemon exits by itself 30 seconds after the last Claude Code process is gone
- The elapsed timer counts from the start of the session, not of the daemon, which every session shares; from the JSONL fallback it counts from the first message after the last break of more than 15 minutes, so a resumed session doesn't show a day-long timer
- Closing one Claude Code session no longer takes the card away while other sessions are still running but have no marker: those started before the plugin was installed, or by an older version of it
  - `stop.sh` keeps the daemon while another Claude Code process runs
  - `stop.sh` records ended sessions in `~/.claude/discord-presence-ended/`, and the daemon no longer shows a closed session because its transcript is the freshest, unless it is resumed
- Sessions starting at the same time (several terminals opened at once) no longer start a daemon each; only the last was in the pid file, so the others were never stopped. `start.sh` now checks and starts the daemon under a lock, broken after a minute if a killed script left it

## [1.0.6] - 2026-10-12

### Fixed
- The daemon never started on Windows ([@10kkyvl](https://github.com/10kkyvl)) [#6](https://github.com/tsanva/cc-discord-presence/pull/6)
  - `start.ps1` sent stdout and stderr to the same file, which `Start-Process` refuses; stderr now goes to `~/.claude/discord-presence.log.err`
  - `start.ps1` and `stop.ps1` trust the pid file only when that pid is a `cc-discord-presence` process, so a pid reused after a reboot no longer reports the daemon as running or gets force-stopped

### Contributors
- Yehor ([@10kkyvl](https://github.com/10kkyvl)) - Windows daemon start fix

## [1.0.5] - 2026-10-04

### Changed
- The hooks module is now the primary data source in every host that loads it (terminal, desktop app, IDE extensions), not only the desktop app
  - It writes `~/.claude/discord-presence-module.json`; the daemon reads it first, then the statusline file, then JSONL
  - Updates on session start, prompt submit, each tool call and each response; clears its data on session end
  - The statusline setup is now only needed on Claude Code versions without hooks modules
- Tokens shown are the context size (the last request's input, cached tokens included) in every data source, instead of input + output
- The daemon only pushes to Discord when the displayed text changes

### Fixed
- Start scripts re-download the daemon when the installed binary is from another release, and restart a running daemon from an older release; previously a binary was only downloaded once, so updates to the daemon never reached existing installs (`claude plugin update` replaces the plugin files, not the binary in `~/.claude/bin/`)

### Upgrading from 1.0.4 or earlier
- Update the plugin, then start a new Claude Code session: the start script replaces the old daemon. If Discord still shows stale data, delete `~/.claude/bin/cc-discord-presence-*` and start a new session.

## [1.0.4] - 2026-10-04

### Added
- Desktop app support: a hooks module (`hooks/register.ts`) writes `~/.claude/discord-presence-data.json` from Claude desktop app sessions, which have no statusline
  - Previously the daemon kept showing whichever terminal session last wrote the file
  - Terminal sessions, IDE extensions and headless runs are unchanged
  - Claude Code versions without hooks module support ignore it

### Fixed
- `marketplace.json` entry version now matches `plugin.json`

## [1.0.3] - 2026-01-20

### Added
- Windows support with native named pipe IPC ([@8bury](https://github.com/8bury)) [#2](https://github.com/tsanva/cc-discord-presence/pull/2)
  - PowerShell start script (`start.ps1`)
  - Windows binary cross-compilation
  - go-winio for named pipe communication
- Comprehensive test suite for both main package and discord client
  - `main_test.go`: Tests for calculateCost, formatModelName, formatNumber, readStatusLineData, parseJSONLSession, path decoding, findMostRecentJSONL
  - `discord/client_test.go`: Tests for IPC protocol, SetActivity, send/receive, frame format verification

### Contributors
- pedro ([@8bury](https://github.com/8bury)) - Windows support
- Claude ([@claude](https://github.com/claude)) - Test suite

## [1.0.2] - 2025-12-31

### Fixed
- Replaced reference counting with PID-based session tracking
  - Previous refcount could drift if sessions crashed or were killed
  - Now each session registers with its PID in `~/.claude/discord-presence-sessions/`
  - Orphaned sessions (dead PIDs) are automatically cleaned up
  - Self-healing: no manual intervention needed after crashes

## [1.0.1] - 2025-12-31

### Fixed
- Multi-instance support: daemon now stays running when multiple Claude Code instances are open
  - Added reference counting to track active instances
  - Daemon only stops when all instances are closed

## [1.0.0] - 2025-12-30

### Added
- Initial release
- Discord Rich Presence showing project name, git branch, model, tokens, and cost
- Two data sources: statusline integration (accurate) and JSONL fallback (zero-config)
- Cross-platform support: macOS (arm64/amd64), Linux (amd64/arm64), Windows (amd64)
- Automatic binary download on first run
- GitHub Actions workflow for automated releases

[Unreleased]: https://github.com/tsanva/cc-discord-presence/compare/v1.0.6...HEAD
[1.0.6]: https://github.com/tsanva/cc-discord-presence/compare/v1.0.5...v1.0.6
[1.0.5]: https://github.com/tsanva/cc-discord-presence/compare/v1.0.4...v1.0.5
[1.0.4]: https://github.com/tsanva/cc-discord-presence/compare/v1.0.3...v1.0.4
[1.0.3]: https://github.com/tsanva/cc-discord-presence/compare/v1.0.2...v1.0.3
[1.0.2]: https://github.com/tsanva/cc-discord-presence/compare/v1.0.1...v1.0.2
[1.0.1]: https://github.com/tsanva/cc-discord-presence/compare/v1.0.0...v1.0.1
[1.0.0]: https://github.com/tsanva/cc-discord-presence/releases/tag/v1.0.0
