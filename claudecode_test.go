package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"
)

// startProcessAt runs the test binary, idle, as the executable at path.
func startProcessAt(t *testing.T, path string) int {
	t.Helper()
	src, _ := os.Executable()
	os.MkdirAll(filepath.Dir(path), 0755)
	copyFile(t, src, path)
	cmd := exec.Command(path)
	cmd.Env = append(os.Environ(), "CC_FAKE_DAEMON=1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	return cmd.Process.Pid
}

func TestListClaudeCodeFindsClaudeCode(t *testing.T) {
	name := "claude"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	pid := startProcessAt(t, filepath.Join(t.TempDir(), "npm", "claude-code", "bin", name))
	time.Sleep(200 * time.Millisecond) // let it show up in the process table

	if pids := listClaudeCode(); !slices.Contains(pids, pid) {
		t.Errorf("listClaudeCode() = %v, missing Claude Code at pid %d", pids, pid)
	}
}

// The Claude desktop app's executable is also claude.exe.
func TestListClaudeCodeSkipsDesktopApp(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("the desktop app is named Claude on macOS, which pgrep -x claude doesn't match")
	}
	pid := startProcessAt(t, filepath.Join(t.TempDir(), "AnthropicClaude", "app-1.2.3", "claude.exe"))
	time.Sleep(200 * time.Millisecond)

	if pids := listClaudeCode(); slices.Contains(pids, pid) {
		t.Errorf("listClaudeCode() = %v includes the desktop app at pid %d", pids, pid)
	}
}
