//go:build !windows

package discord

import (
	"fmt"
	"net"
	"os"
)

func (c *Client) connectToDiscord() (Conn, error) {
	for i := 0; i < 10; i++ {
		path := getSocketPath(i)
		if path != "" {
			conn, err := net.Dial("unix", path)
			if err == nil {
				return conn, nil
			}
		}
	}
	return nil, fmt.Errorf("Discord IPC socket not found")
}

func getSocketPath(i int) string {
	// Try various temp directories
	dirs := []string{
		os.Getenv("XDG_RUNTIME_DIR"),
		os.Getenv("TMPDIR"),
		os.Getenv("TMP"),
		os.Getenv("TEMP"),
		"/tmp",
	}
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		path := fmt.Sprintf("%s/discord-ipc-%d", dir, i)
		if _, err := os.Stat(path); err == nil {
			return path
		}
		// Also try snap/flatpak paths
		snapPath := fmt.Sprintf("%s/snap.discord/discord-ipc-%d", dir, i)
		if _, err := os.Stat(snapPath); err == nil {
			return snapPath
		}
		flatpakPath := fmt.Sprintf("%s/app/com.discordapp.Discord/discord-ipc-%d", dir, i)
		if _, err := os.Stat(flatpakPath); err == nil {
			return flatpakPath
		}
	}
	return ""
}
