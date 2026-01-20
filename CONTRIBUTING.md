# Contributing to cc-discord-presence

Thank you for your interest in contributing! This document provides guidelines for contributing to the project.

## Getting Started

1. Fork the repository
2. Clone your fork: `git clone https://github.com/YOUR_USERNAME/cc-discord-presence.git`
3. Create a feature branch: `git checkout -b feature/your-feature-name`

## Development Setup

### Prerequisites

- Go 1.25 or later
- Discord desktop app (for testing Rich Presence)
- Claude Code (for end-to-end testing)

### Building

```bash
# Build for your current platform
go build -o cc-discord-presence .

# Cross-compile for all platforms
./scripts/build.sh
```

## Before Submitting a Pull Request

### 1. Run the Test Suite

**This is required.** All tests must pass before submitting a PR.

```bash
# Run all tests
go test -v ./...

# Run tests with coverage
go test -cover ./...
```

### 2. Test Manually

If your changes affect Discord Rich Presence:
1. Start Discord
2. Run the binary: `./cc-discord-presence`
3. Start a Claude Code session
4. Verify the presence appears correctly in Discord

### 3. Update Documentation

- Update `CHANGELOG.md` under `## [Unreleased]` with your changes
- Update `README.md` if you're adding new features or changing behavior
- Add code comments for complex logic

### 4. Code Style

- Follow standard Go conventions (`go fmt`)
- Keep functions focused and reasonably sized
- Add tests for new functionality

## Pull Request Process

1. Ensure all tests pass
2. Update documentation as needed
3. Write a clear PR description explaining:
   - What the change does
   - Why it's needed
   - How to test it
4. Link any related issues

## What Gets You Credited

Contributors are credited in:
- `CHANGELOG.md` under the `### Contributors` section for each release
- GitHub release notes

Your GitHub username will be linked, along with a description of your contribution.

## Questions?

Feel free to open an issue for any questions or concerns.
