//go:build windows

package discord

import (
	"fmt"

	"github.com/Microsoft/go-winio"
)

func (c *Client) connectToDiscord() (Conn, error) {
	for i := 0; i < 10; i++ {
		pipePath := fmt.Sprintf(`\\.\pipe\discord-ipc-%d`, i)
		conn, err := winio.DialPipe(pipePath, nil)
		if err == nil {
			return conn, nil
		}
	}
	return nil, fmt.Errorf("Discord IPC pipe not found. Make sure Discord is running")
}
