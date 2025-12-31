# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/tsanva/cc-discord-presence/compare/v1.0.2...HEAD
[1.0.2]: https://github.com/tsanva/cc-discord-presence/compare/v1.0.1...v1.0.2
[1.0.1]: https://github.com/tsanva/cc-discord-presence/compare/v1.0.0...v1.0.1
[1.0.0]: https://github.com/tsanva/cc-discord-presence/releases/tag/v1.0.0
