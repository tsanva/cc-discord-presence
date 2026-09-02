# Start Discord Rich Presence daemon (Windows)
# WARNING: Windows support is untested. Please report issues on GitHub.

$ErrorActionPreference = "Stop"

# Configuration
$ClaudeDir = Join-Path $env:USERPROFILE ".claude"
$BinDir = Join-Path $ClaudeDir "bin"
$PidFile = Join-Path $ClaudeDir "discord-presence.pid"
$LogFile = Join-Path $ClaudeDir "discord-presence.log"
$SessionsDir = Join-Path $ClaudeDir "discord-presence-sessions-win"
$Repo = "tsanva/cc-discord-presence"
$Version = "v1.0.3"

# Ensure directories exist
New-Item -ItemType Directory -Path $ClaudeDir -Force | Out-Null
New-Item -ItemType Directory -Path $BinDir -Force | Out-Null
New-Item -ItemType Directory -Path $SessionsDir -Force | Out-Null

# Migration: drop the old refcount file, replaced by PID-based tracking below
Remove-Item (Join-Path $ClaudeDir "discord-presence.refcount") -Force -ErrorAction SilentlyContinue

# Session tracking: register this session by PID and re-derive the active count
# from actual process liveness on every call (self-healing). A plain counter
# can only be trusted if every session also runs its matching decrement -- but
# that never happens for a session that ends abruptly (window closed, process
# killed, crash), so the count only ever grows and the daemon never stops.
$SessionPid = $env:CLAUDE_PID
if (-not $SessionPid) { $SessionPid = $PID }
New-Item -ItemType File -Path (Join-Path $SessionsDir $SessionPid) -Force | Out-Null

$ActiveSessions = 0
Get-ChildItem -Path $SessionsDir -File -ErrorAction SilentlyContinue | ForEach-Object {
    $sessionProcess = Get-Process -Id $_.Name -ErrorAction SilentlyContinue
    if ($sessionProcess) {
        $ActiveSessions++
    } else {
        Remove-Item $_.FullName -Force -ErrorAction SilentlyContinue
    }
}

# If daemon is already running, just exit
if (Test-Path $PidFile) {
    $OldPid = Get-Content $PidFile -ErrorAction SilentlyContinue
    if ($OldPid) {
        $Process = Get-Process -Id $OldPid -ErrorAction SilentlyContinue
        if ($Process) {
            Write-Host "Discord Rich Presence already running (PID: $OldPid, sessions: $ActiveSessions)"
            exit 0
        }
    }
}

$BinaryName = "cc-discord-presence-windows-amd64.exe"
$Binary = Join-Path $BinDir $BinaryName

# Download binary if not present
if (-not (Test-Path $Binary)) {
    Write-Host "Downloading cc-discord-presence for windows-amd64..."

    $DownloadUrl = "https://github.com/$Repo/releases/download/$Version/$BinaryName"

    try {
        Invoke-WebRequest -Uri $DownloadUrl -OutFile $Binary -UseBasicParsing
        Write-Host "Downloaded successfully!"
    } catch {
        Write-Error "Failed to download binary: $_"
        exit 1
    }
}

if (-not (Test-Path $Binary)) {
    Write-Error "Error: Binary not found at $Binary"
    exit 1
}

# Start the daemon in background
$Process = Start-Process -FilePath $Binary -NoNewWindow -PassThru -RedirectStandardOutput $LogFile -RedirectStandardError $LogFile
$Process.Id | Out-File -FilePath $PidFile -Encoding ASCII

Write-Host "Discord Rich Presence started (PID: $($Process.Id), sessions: $ActiveSessions)"
