//go:build !windows

package discord

import (
	"fmt"
	"net"
	"os"
)

func (c *Client) connectToDiscord() (Conn, error) {
	for i := 0; i < 10; i++ {
		if path := findSocketPath(i); path != "" {
			if conn, err := net.Dial("unix", path); err == nil {
				return conn, nil
			}
		}
	}
	return nil, fmt.Errorf("Discord IPC socket not found. Make sure Discord is running")
}

func findSocketPath(i int) string {
	dirs := []string{
		os.Getenv("XDG_RUNTIME_DIR"),
		os.Getenv("TMPDIR"),
		os.Getenv("TMP"),
		os.Getenv("TEMP"),
		"/tmp",
	}

	// Socket path patterns: standard, snap, and flatpak
	patterns := []string{
		"discord-ipc-%d",
		"snap.discord/discord-ipc-%d",
		"app/com.discordapp.Discord/discord-ipc-%d",
	}

	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		for _, pattern := range patterns {
			path := fmt.Sprintf("%s/"+pattern, dir, i)
			if _, err := os.Stat(path); err == nil {
				return path
			}
		}
	}
	return ""
}
