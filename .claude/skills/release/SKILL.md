---
name: release
description: Prepare and create a new release for cc-discord-presence (project)
---

# Release Skill

When invoked, guide the user through creating a new release.

## Version Types

### Stable Release (e.g., v1.1.0)
- For production-ready features
- Tagged on `main` branch
- Shows as "Latest" on GitHub

### Dev/Pre-release (e.g., v1.1.0-dev)
- For testing new features before stable release
- Tagged on `dev` branch
- Marked as "Pre-release" on GitHub (not shown as "Latest")
- Use `-dev`, `-alpha`, `-beta`, or `-rc.1` suffixes

## Pre-release Checklist

1. **Get the new version number** from the user (e.g., v1.1.0 or v1.1.0-dev)

2. **Update CHANGELOG.md:**
   - Add a new section for the version under `## [Unreleased]`
   - Document all changes under `### Added`, `### Changed`, `### Fixed`, `### Removed`
   - Update the comparison links at the bottom

3. **Update version in these files:**
   - `scripts/start.sh` - update `VERSION="vX.X.X"`
   - `scripts/start.ps1` - update `$Version = "vX.X.X"`
   - `.claude-plugin/plugin.json` - update `"version": "X.X.X"` (no 'v' prefix)

4. **Commit the version bump:**
   ```bash
   git add CHANGELOG.md scripts/start.sh scripts/start.ps1 .claude-plugin/plugin.json
   git commit -m "Bump version to vX.X.X"
   git push origin <branch>
   ```

5. **Create and push tag:**
   ```bash
   git tag -a vX.X.X -m "Release vX.X.X"
   git push origin vX.X.X
   ```

6. **Wait for GitHub Actions** to build and create the release automatically.

7. **For dev/pre-releases only**, mark as pre-release:
   ```bash
   gh release edit vX.X.X --prerelease
   ```

## Files uploaded to release (via GitHub Actions):
- `cc-discord-presence-darwin-arm64`
- `cc-discord-presence-darwin-amd64`
- `cc-discord-presence-linux-amd64`
- `cc-discord-presence-linux-arm64`
- `cc-discord-presence-windows-amd64.exe`

## Merging dev to main (for stable releases)

When ready to promote a dev release to stable:
```bash
git checkout main
git merge dev
git push origin main
# Then follow the stable release process above
```
