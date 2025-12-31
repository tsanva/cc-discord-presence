# Stop Discord Rich Presence daemon (Windows)
# WARNING: Windows support is untested. Please report issues on GitHub.

$ClaudeDir = Join-Path $env:USERPROFILE ".claude"
$PidFile = Join-Path $ClaudeDir "discord-presence.pid"
$SessionsDir = Join-Path $ClaudeDir "discord-presence-sessions"

# Get the parent process ID (Claude Code session)
$SessionPid = $PID
try {
    $ParentPid = (Get-CimInstance Win32_Process -Filter "ProcessId = $PID").ParentProcessId
    if ($ParentPid) {
        $SessionPid = $ParentPid
    }
} catch {}

# Remove this session's file
$SessionFile = Join-Path $SessionsDir $SessionPid
if (Test-Path $SessionFile) {
    Remove-Item $SessionFile -Force -ErrorAction SilentlyContinue
}

# Count remaining active sessions (cleanup orphans while counting)
function Get-ActiveSessionCount {
    $count = 0
    if (-not (Test-Path $SessionsDir)) { return 0 }

    Get-ChildItem $SessionsDir -File | ForEach-Object {
        $pid = $_.Name
        try {
            $process = Get-Process -Id $pid -ErrorAction SilentlyContinue
            if ($process) {
                $count++
            } else {
                # Orphaned session file, clean it up
                Remove-Item $_.FullName -Force -ErrorAction SilentlyContinue
            }
        } catch {
            # Process doesn't exist, clean up
            Remove-Item $_.FullName -Force -ErrorAction SilentlyContinue
        }
    }
    return $count
}

$ActiveSessions = Get-ActiveSessionCount

# Only kill daemon if no more active sessions
if ($ActiveSessions -gt 0) {
    Write-Host "Discord Rich Presence still in use by $ActiveSessions session(s)"
    exit 0
}

# Clean up sessions directory
if (Test-Path $SessionsDir) {
    Remove-Item $SessionsDir -Recurse -Force -ErrorAction SilentlyContinue
}

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

# Clean up old refcount file if it exists (migration from old version)
$RefCountFile = Join-Path $ClaudeDir "discord-presence.refcount"
if (Test-Path $RefCountFile) {
    Remove-Item $RefCountFile -Force -ErrorAction SilentlyContinue
}
