# Stop Discord Rich Presence daemon (Windows)
# WARNING: Windows support is untested. Please report issues on GitHub.

$ClaudeDir = Join-Path $env:USERPROFILE ".claude"
$PidFile = Join-Path $ClaudeDir "discord-presence.pid"
$RefCountFile = Join-Path $ClaudeDir "discord-presence.refcount"

# Decrement reference count
$RefCount = 0
if (Test-Path $RefCountFile) {
    $RefCount = [int](Get-Content $RefCountFile -ErrorAction SilentlyContinue)
}
$RefCount--

# Don't go below 0
if ($RefCount -lt 0) {
    $RefCount = 0
}

$RefCount | Out-File -FilePath $RefCountFile -Encoding ASCII

# Only kill daemon if no more instances are using it
if ($RefCount -gt 0) {
    Write-Host "Discord Rich Presence still in use by $RefCount instance(s)"
    exit 0
}

# Clean up ref count file
Remove-Item $RefCountFile -Force -ErrorAction SilentlyContinue

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
