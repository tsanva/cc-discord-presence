package discord

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
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
	dial     func() (Conn, error)

	mu      sync.Mutex
	conn    Conn      // nil while disconnected
	last    *Activity // sent again after a reconnect
	writeMu sync.Mutex
	dropped chan struct{}
	done    chan struct{}
	closing sync.Once
}

// NewClient creates a new Discord RPC client
func NewClient(clientID string) *Client {
	c := &Client{
		clientID: clientID,
		dropped:  make(chan struct{}, 1),
		done:     make(chan struct{}),
	}
	c.dial = c.connectToDiscord
	return c
}

// Connected reports whether the client holds a live connection to Discord.
func (c *Client) Connected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn != nil
}

// KeepConnected connects in the background and reconnects whenever Discord
// goes away (not started yet, quit, restarted for an update), retrying every
// interval. After each reconnect it sends the last activity again, since
// Discord forgets it with the connection. Close stops it.
func (c *Client) KeepConnected(interval time.Duration, logf func(format string, args ...any)) {
	go func() {
		waiting := false // the current outage was logged
		for {
			if !c.Connected() {
				if err := c.Connect(); err != nil {
					if !waiting {
						logf("⏳ Waiting for Discord: %v", err)
						waiting = true
					}
				} else {
					waiting = false
					logf("✓ Discord RPC connected!")
					c.resend()
				}
			}
			select {
			case <-c.done:
				return
			case <-c.dropped:
				logf("⚠ Lost the connection to Discord, reconnecting")
			case <-time.After(interval):
			}
		}
	}()
}

// Connect establishes connection to Discord
func (c *Client) Connect() error {
	conn, err := c.dial()
	if err != nil {
		return err
	}

	// Send handshake
	handshake := map[string]interface{}{
		"v":         1,
		"client_id": c.clientID,
	}
	if err := writeFrame(conn, opHandshake, handshake); err != nil {
		conn.Close()
		return fmt.Errorf("handshake failed: %w", err)
	}

	// Read handshake response
	if _, err := readFrame(conn); err != nil {
		conn.Close()
		return fmt.Errorf("handshake response failed: %w", err)
	}

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()
	go c.readLoop(conn)
	return nil
}

// readLoop consumes Discord's replies, and notices when Discord goes away
// (the pipe closes) even while there is nothing to send.
func (c *Client) readLoop(conn Conn) {
	for {
		if _, err := readFrame(conn); err != nil {
			c.drop(conn)
			return
		}
	}
}

// drop forgets conn if it is still the current connection.
func (c *Client) drop(conn Conn) {
	c.mu.Lock()
	if c.conn != conn {
		c.mu.Unlock()
		return
	}
	c.conn = nil
	c.mu.Unlock()
	conn.Close()
	select {
	case c.dropped <- struct{}{}:
	default:
	}
}

// ErrNotConnected is returned by SetActivity while Discord isn't reachable.
// The activity is kept and sent once KeepConnected reconnects.
var ErrNotConnected = errors.New("not connected")

// SetActivity updates the Discord Rich Presence
func (c *Client) SetActivity(activity Activity) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	c.mu.Lock()
	c.last = &activity
	c.mu.Unlock()
	return c.sendActivity(activity)
}

// ClearActivity empties the card. It also forgets the last activity, so a
// reconnect doesn't bring the card back.
func (c *Client) ClearActivity() error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	c.mu.Lock()
	c.last = nil
	c.mu.Unlock()
	// SET_ACTIVITY without an activity clears it
	return c.send(opFrame, map[string]interface{}{
		"cmd":   "SET_ACTIVITY",
		"args":  map[string]interface{}{"pid": os.Getpid()},
		"nonce": fmt.Sprintf("%d", time.Now().UnixNano()),
	})
}

// resend sends the last activity again, after a reconnect.
func (c *Client) resend() {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	c.mu.Lock()
	last := c.last
	c.mu.Unlock()
	if last != nil {
		c.sendActivity(*last)
	}
}

func (c *Client) sendActivity(activity Activity) error {
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

// Close disconnects from Discord and stops KeepConnected
func (c *Client) Close() error {
	c.closing.Do(func() { close(c.done) })
	c.mu.Lock()
	conn := c.conn
	c.conn = nil
	c.mu.Unlock()
	if conn != nil {
		return conn.Close()
	}
	return nil
}

// send writes one frame on the current connection, dropping the connection
// when the write fails.
func (c *Client) send(opcode int, data interface{}) error {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()
	if conn == nil {
		return ErrNotConnected
	}
	if err := writeFrame(conn, opcode, data); err != nil {
		c.drop(conn)
		return err
	}
	return nil
}

func (c *Client) receive() ([]byte, error) {
	return readFrame(c.conn)
}

func writeFrame(conn Conn, opcode int, data interface{}) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}

	// Discord IPC frame: [opcode:4bytes][length:4bytes][payload]
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, int32(opcode))
	binary.Write(buf, binary.LittleEndian, int32(len(payload)))
	buf.Write(payload)

	_, err = conn.Write(buf.Bytes())
	return err
}

// readFrame reads one frame. A single Read may return part of it, so both
// the header and the payload are read in full.
func readFrame(conn Conn) ([]byte, error) {
	header := make([]byte, 8)
	if _, err := io.ReadFull(conn, header); err != nil {
		return nil, err
	}

	length := binary.LittleEndian.Uint32(header[4:8])
	payload := make([]byte, length)
	if _, err := io.ReadFull(conn, payload); err != nil {
		return nil, err
	}

	return payload, nil
}
