# Stop Discord Rich Presence daemon (Windows)
# WARNING: Windows support is untested. Please report issues on GitHub.

# Configuration
$ClaudeDir = Join-Path $env:USERPROFILE ".claude"
$PidFile = Join-Path $ClaudeDir "discord-presence.pid"
$RefcountFile = Join-Path $ClaudeDir "discord-presence.refcount"

# Session tracking: Use refcount (PID-based tracking is unreliable on Windows)
$CurrentCount = 1
if (Test-Path $RefcountFile) {
    $CurrentCount = [int](Get-Content $RefcountFile -ErrorAction SilentlyContinue)
}
$ActiveSessions = [Math]::Max(0, $CurrentCount - 1)

if ($ActiveSessions -gt 0) {
    $ActiveSessions | Out-File -FilePath $RefcountFile -Encoding ASCII -NoNewline
    Write-Host "Discord Rich Presence still in use by $ActiveSessions session(s)"
    exit 0
}

# No more sessions, clean up refcount file
Remove-Item $RefcountFile -Force -ErrorAction SilentlyContinue

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
