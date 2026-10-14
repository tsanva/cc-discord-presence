package discord

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

// fakeDiscord stands in for the Discord desktop app's IPC pipe: it answers
// the handshake, acknowledges every command, and can quit and come back.
type fakeDiscord struct {
	mu      sync.Mutex
	running bool
	server  net.Conn
	dials   int
	details chan string // activity details of each SET_ACTIVITY received
}

func newFakeDiscord(running bool) *fakeDiscord {
	return &fakeDiscord{running: running, details: make(chan string, 16)}
}

func (f *fakeDiscord) dial() (Conn, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dials++
	if !f.running {
		return nil, errors.New("Discord IPC pipe not found")
	}
	client, server := net.Pipe()
	f.server = server
	go f.serve(server)
	return client, nil
}

func (f *fakeDiscord) serve(conn net.Conn) {
	if _, _, err := readTestFrame(conn); err != nil { // handshake
		return
	}
	writeTestFrame(conn, opFrame, []byte(`{"cmd":"DISPATCH","evt":"READY"}`))
	for {
		_, payload, err := readTestFrame(conn)
		if err != nil {
			return
		}
		var cmd struct {
			Args struct {
				Activity struct {
					Details string `json:"details"`
				} `json:"activity"`
			} `json:"args"`
		}
		json.Unmarshal(payload, &cmd)
		f.details <- cmd.Args.Activity.Details
		writeTestFrame(conn, opFrame, []byte(`{"cmd":"SET_ACTIVITY","evt":null}`))
	}
}

// quit closes the pipe as Discord does when it exits or restarts.
func (f *fakeDiscord) quit() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.running = false
	if f.server != nil {
		f.server.Close()
	}
}

func (f *fakeDiscord) start() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.running = true
}

func (f *fakeDiscord) expectActivity(t *testing.T, want string) {
	t.Helper()
	select {
	case got := <-f.details:
		if got != want {
			t.Fatalf("Discord received activity %q, want %q", got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("Discord never received activity %q", want)
	}
}

func readTestFrame(r io.Reader) (int32, []byte, error) {
	header := make([]byte, 8)
	if _, err := io.ReadFull(r, header); err != nil {
		return 0, nil, err
	}
	payload := make([]byte, binary.LittleEndian.Uint32(header[4:]))
	_, err := io.ReadFull(r, payload)
	return int32(binary.LittleEndian.Uint32(header[:4])), payload, err
}

func writeTestFrame(w io.Writer, opcode int32, payload []byte) {
	frame := make([]byte, 8, 8+len(payload))
	binary.LittleEndian.PutUint32(frame[:4], uint32(opcode))
	binary.LittleEndian.PutUint32(frame[4:], uint32(len(payload)))
	w.Write(append(frame, payload...))
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		if cond() {
			return
		}
	}
	t.Fatalf("timed out waiting until %s", what)
}

// Discord restarting (an update, quitting and reopening) closes the pipe while
// the card text stays the same, so nothing new would be sent to bring it back.
func TestKeepConnected_RestoresActivityAfterDiscordRestart(t *testing.T) {
	discord := newFakeDiscord(true)
	client := NewClient("test")
	client.dial = discord.dial
	client.KeepConnected(20*time.Millisecond, t.Logf)
	defer client.Close()

	waitFor(t, "connected", client.Connected)
	if err := client.SetActivity(Activity{Details: "Working on: kadens"}); err != nil {
		t.Fatalf("SetActivity: %v", err)
	}
	discord.expectActivity(t, "Working on: kadens")

	discord.quit()
	waitFor(t, "the client notices Discord quit", func() bool { return !client.Connected() })
	discord.start()

	discord.expectActivity(t, "Working on: kadens")
}

// Claude Code can start before Discord does; the daemon used to exit then.
func TestKeepConnected_ShowsActivityOnceDiscordStarts(t *testing.T) {
	discord := newFakeDiscord(false)
	client := NewClient("test")
	client.dial = discord.dial
	client.KeepConnected(20*time.Millisecond, t.Logf)
	defer client.Close()

	if err := client.SetActivity(Activity{Details: "Working on: kadens"}); !errors.Is(err, ErrNotConnected) {
		t.Errorf("SetActivity before Discord runs = %v, want ErrNotConnected", err)
	}
	discord.start()

	discord.expectActivity(t, "Working on: kadens")
}

func TestClearActivity_EmptiesTheCard(t *testing.T) {
	discord := newFakeDiscord(true)
	client := NewClient("test")
	client.dial = discord.dial
	client.KeepConnected(20*time.Millisecond, t.Logf)
	defer client.Close()
	waitFor(t, "connected", client.Connected)

	client.SetActivity(Activity{Details: "Working on: kadens"})
	discord.expectActivity(t, "Working on: kadens")
	if err := client.ClearActivity(); err != nil {
		t.Fatalf("ClearActivity: %v", err)
	}

	discord.expectActivity(t, "") // SET_ACTIVITY without an activity
}

// A cleared card must stay cleared when Discord reconnects.
func TestClearActivity_NotRestoredAfterReconnect(t *testing.T) {
	discord := newFakeDiscord(true)
	client := NewClient("test")
	client.dial = discord.dial
	client.KeepConnected(20*time.Millisecond, t.Logf)
	defer client.Close()
	waitFor(t, "connected", client.Connected)
	client.SetActivity(Activity{Details: "Working on: kadens"})
	discord.expectActivity(t, "Working on: kadens")
	client.ClearActivity()
	discord.expectActivity(t, "")

	discord.quit()
	waitFor(t, "the client notices Discord quit", func() bool { return !client.Connected() })
	discord.start()
	waitFor(t, "reconnected", client.Connected)

	select {
	case got := <-discord.details:
		t.Errorf("after reconnecting the client sent activity %q, want nothing", got)
	case <-time.After(200 * time.Millisecond):
	}
}
