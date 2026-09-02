# Stop Discord Rich Presence daemon (Windows)
# WARNING: Windows support is untested. Please report issues on GitHub.

# Configuration
$ClaudeDir = Join-Path $env:USERPROFILE ".claude"
$PidFile = Join-Path $ClaudeDir "discord-presence.pid"
$SessionsDir = Join-Path $ClaudeDir "discord-presence-sessions-win"

# Session tracking: mirror start.ps1 -- drop this session's PID marker, then
# recount by actual liveness (self-healing) rather than trusting a counter.
$SessionPid = $env:CLAUDE_PID
if (-not $SessionPid) { $SessionPid = $PID }
Remove-Item (Join-Path $SessionsDir $SessionPid) -Force -ErrorAction SilentlyContinue

$ActiveSessions = 0
Get-ChildItem -Path $SessionsDir -File -ErrorAction SilentlyContinue | ForEach-Object {
    $sessionProcess = Get-Process -Id $_.Name -ErrorAction SilentlyContinue
    if ($sessionProcess) {
        $ActiveSessions++
    } else {
        Remove-Item $_.FullName -Force -ErrorAction SilentlyContinue
    }
}

if ($ActiveSessions -gt 0) {
    Write-Host "Discord Rich Presence still in use by $ActiveSessions session(s)"
    exit 0
}

Remove-Item $SessionsDir -Recurse -Force -ErrorAction SilentlyContinue

# Stop the daemon
if (Test-Path $PidFile) {
    $ProcessId = Get-Content $PidFile -ErrorAction SilentlyContinue
    if ($ProcessId) {
        $Process = Get-Process -Id $ProcessId -ErrorAction SilentlyContinue
        if ($Process) {
            Stop-Process -Id $ProcessId -Force -ErrorAction SilentlyContinue
            Write-Host "Discord Rich Presence stopped (PID: $ProcessId)"
        }
    }
    Remove-Item $PidFile -Force -ErrorAction SilentlyContinue
} else {
    # Try to find and kill by process name
    $Processes = Get-Process -Name "cc-discord-presence*" -ErrorAction SilentlyContinue
    if ($Processes) {
        $Processes | Stop-Process -Force
        Write-Host "Discord Rich Presence stopped"
    }
}
