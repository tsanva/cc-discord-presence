package discord

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Opcodes for Discord IPC
const (
	opHandshake = 0
	opFrame     = 1
)

// Activity represents Discord Rich Presence activity
type Activity struct {
	Details    string     `json:"details,omitempty"`
	State      string     `json:"state,omitempty"`
	LargeImage string     `json:"large_image,omitempty"`
	LargeText  string     `json:"large_text,omitempty"`
	SmallImage string     `json:"small_image,omitempty"`
	SmallText  string     `json:"small_text,omitempty"`
	StartTime  *time.Time `json:"-"`
}

// Conn interface for both unix sockets and windows pipes
type Conn interface {
	Read(b []byte) (n int, err error)
	Write(b []byte) (n int, err error)
	Close() error
}

// Client handles Discord RPC connection
type Client struct {
	clientID string
	conn     Conn
}

// NewClient creates a new Discord RPC client
func NewClient(clientID string) *Client {
	return &Client{clientID: clientID}
}

// Connect establishes connection to Discord
func (c *Client) Connect() error {
	conn, err := c.connectToDiscord()
	if err != nil {
		return err
	}
	c.conn = conn

	// Send handshake
	handshake := map[string]interface{}{
		"v":         1,
		"client_id": c.clientID,
	}
	if err := c.send(opHandshake, handshake); err != nil {
		c.conn.Close()
		return fmt.Errorf("handshake failed: %w", err)
	}

	// Read handshake response
	if _, err := c.receive(); err != nil {
		c.conn.Close()
		return fmt.Errorf("handshake response failed: %w", err)
	}

	return nil
}

// SetActivity updates the Discord Rich Presence
func (c *Client) SetActivity(activity Activity) error {
	if c.conn == nil {
		return fmt.Errorf("not connected")
	}

	// Build timestamps if StartTime is set
	var timestamps map[string]int64
	if activity.StartTime != nil {
		timestamps = map[string]int64{
			"start": activity.StartTime.Unix(),
		}
	}

	// Build assets
	assets := map[string]string{}
	if activity.LargeImage != "" {
		assets["large_image"] = activity.LargeImage
	}
	if activity.LargeText != "" {
		assets["large_text"] = activity.LargeText
	}
	if activity.SmallImage != "" {
		assets["small_image"] = activity.SmallImage
	}
	if activity.SmallText != "" {
		assets["small_text"] = activity.SmallText
	}

	activityData := map[string]interface{}{}
	if activity.Details != "" {
		activityData["details"] = activity.Details
	}
	if activity.State != "" {
		activityData["state"] = activity.State
	}
	if len(assets) > 0 {
		activityData["assets"] = assets
	}
	if timestamps != nil {
		activityData["timestamps"] = timestamps
	}

	payload := map[string]interface{}{
		"cmd": "SET_ACTIVITY",
		"args": map[string]interface{}{
			"pid":      os.Getpid(),
			"activity": activityData,
		},
		"nonce": fmt.Sprintf("%d", time.Now().UnixNano()),
	}

	return c.send(opFrame, payload)
}

// Close disconnects from Discord
func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

func (c *Client) send(opcode int, data interface{}) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}

	// Discord IPC frame: [opcode:4bytes][length:4bytes][payload]
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, int32(opcode))
	binary.Write(buf, binary.LittleEndian, int32(len(payload)))
	buf.Write(payload)

	_, err = c.conn.Write(buf.Bytes())
	return err
}

func (c *Client) receive() ([]byte, error) {
	header := make([]byte, 8)
	if _, err := c.conn.Read(header); err != nil {
		return nil, err
	}

	length := binary.LittleEndian.Uint32(header[4:8])
	payload := make([]byte, length)
	if _, err := c.conn.Read(payload); err != nil {
		return nil, err
	}

	return payload, nil
}
