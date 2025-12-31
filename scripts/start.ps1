# Start Discord Rich Presence daemon (Windows)
# WARNING: Windows support is untested. Please report issues on GitHub.

$ErrorActionPreference = "Stop"

$PluginRoot = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
$BinDir = Join-Path $PluginRoot "bin"
$ClaudeDir = Join-Path $env:USERPROFILE ".claude"
$PidFile = Join-Path $ClaudeDir "discord-presence.pid"
$LogFile = Join-Path $ClaudeDir "discord-presence.log"
$SessionsDir = Join-Path $ClaudeDir "discord-presence-sessions"
$Repo = "tsanva/cc-discord-presence"
$Version = "v1.0.2"

# Ensure directories exist
if (-not (Test-Path $ClaudeDir)) {
    New-Item -ItemType Directory -Path $ClaudeDir | Out-Null
}
if (-not (Test-Path $BinDir)) {
    New-Item -ItemType Directory -Path $BinDir | Out-Null
}
if (-not (Test-Path $SessionsDir)) {
    New-Item -ItemType Directory -Path $SessionsDir | Out-Null
}

# Get the parent process ID (Claude Code session)
$SessionPid = $PID
try {
    $ParentPid = (Get-CimInstance Win32_Process -Filter "ProcessId = $PID").ParentProcessId
    if ($ParentPid) {
        $SessionPid = $ParentPid
    }
} catch {}

# Register this session
$SessionPid | Out-File -FilePath (Join-Path $SessionsDir $SessionPid) -Encoding ASCII

# Count active sessions (cleanup orphans while counting)
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
