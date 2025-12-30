# Privacy Policy

**Last updated:** December 2025

## Overview

cc-discord-presence is an open-source application that displays your Claude Code session information on Discord. This privacy policy explains what data the application accesses and how it's handled.

## Data Collection

**cc-discord-presence does not collect, store, or transmit any personal data.**

The application runs entirely on your local machine and:

- **Does not** send data to any external servers (except Discord's local IPC)
- **Does not** have analytics or telemetry
- **Does not** create user accounts
- **Does not** store any data outside your local machine

## Data Accessed Locally

The application reads the following local data to display your Discord Rich Presence:

| Data | Source | Purpose |
|------|--------|---------|
| Project name | Claude Code session files | Display on Discord |
| Git branch | Local git repository | Display on Discord |
| Model name | Claude Code session files | Display on Discord |
| Token count | Claude Code session files | Display on Discord |
| Session cost | Claude Code session files | Display on Discord |

This data is read from files in `~/.claude/` on your machine and sent only to your local Discord client via IPC (Inter-Process Communication). It is **not** transmitted over the internet.

## Discord Integration

This application uses Discord's Rich Presence feature via local IPC. Your presence information is displayed according to Discord's own privacy settings. Please refer to [Discord's Privacy Policy](https://discord.com/privacy) for how Discord handles presence data.

## Open Source

This application is fully open source. You can audit the code at:
https://github.com/tsanva/cc-discord-presence

## Contact

For privacy questions or concerns, please open an issue on the GitHub repository.
