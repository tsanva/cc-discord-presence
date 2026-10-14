//go:build !windows

package main

import (
	"os/exec"
	"strconv"
	"strings"
)

// listClaudeCode returns the pids of running Claude Code processes, named
// claude. The macOS desktop app is named Claude, which pgrep -x doesn't match.
func listClaudeCode() []int {
	out, _ := exec.Command("pgrep", "-x", "claude").Output()
	var pids []int
	for _, field := range strings.Fields(string(out)) {
		if pid, err := strconv.Atoi(field); err == nil {
			pids = append(pids, pid)
		}
	}
	return pids
}
