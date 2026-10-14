package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tsanva/cc-discord-presence/discord"
)

// TestFormatNumber tests the number formatting logic
func TestFormatNumber(t *testing.T) {
	tests := []struct {
		name string
		n    int64
		want string
	}{
		{name: "Zero", n: 0, want: "0"},
		{name: "Small number", n: 123, want: "123"},
		{name: "999 - no suffix", n: 999, want: "999"},
		{name: "1000 - K suffix", n: 1000, want: "1.0K"},
		{name: "1500 - K suffix", n: 1500, want: "1.5K"},
		{name: "10000 - K suffix", n: 10000, want: "10.0K"},
		{name: "999999 - K suffix", n: 999999, want: "1000.0K"},
		{name: "1000000 - M suffix", n: 1_000_000, want: "1.0M"},
		{name: "1500000 - M suffix", n: 1_500_000, want: "1.5M"},
		{name: "10000000 - M suffix", n: 10_000_000, want: "10.0M"},
		{name: "Large number", n: 150_000_000, want: "150.0M"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatNumber(tt.n); got != tt.want {
				t.Errorf("formatNumber(%d) = %q, want %q", tt.n, got, tt.want)
			}
		})
	}
}

// TestReadStatusLineData tests reading and parsing statusline JSON
func TestReadStatusLineData(t *testing.T) {
	// Save original values
	origDataFilePath := dataFilePath
	origSessionStartTime := sessionStartTime
	defer func() {
		dataFilePath = origDataFilePath
		sessionStartTime = origSessionStartTime
	}()

	// Use a fixed start time for testing
	sessionStartTime = time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "cc-discord-presence-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	tests := []struct {
		name        string
		fileContent string
		wantNil     bool
		wantProject string
		wantModel   string
		wantTokens  int64
	}{
		{
			name: "Valid statusline data",
			fileContent: `{
				"session_id": "abc123",
				"cwd": "/Users/test/myproject",
				"model": {"id": "claude-opus-4-5-20251101", "display_name": "Opus 4.5"},
				"workspace": {"current_dir": "/Users/test/myproject", "project_dir": "/Users/test/myproject"},
				"cost": {"total_cost_usd": 0.50, "total_duration_ms": 1000, "total_api_duration_ms": 500},
				"context_window": {"total_input_tokens": 10000, "total_output_tokens": 5000}
			}`,
			wantNil:     false,
			wantProject: "myproject",
			wantModel:   "Opus 4.5",
			wantTokens:  10000, // context size only, output excluded
		},
		{
			name: "Missing session_id - should return nil",
			fileContent: `{
				"session_id": "",
				"cwd": "/Users/test/myproject",
				"model": {"id": "claude-opus-4-5-20251101", "display_name": "Opus 4.5"}
			}`,
			wantNil: true,
		},
		{
			name:        "Invalid JSON",
			fileContent: `{invalid json`,
			wantNil:     true,
		},
		{
			name: "Missing project_dir uses cwd",
			fileContent: `{
				"session_id": "abc123",
				"cwd": "/Users/test/fallback-project",
				"model": {"id": "claude-sonnet-4-20250514", "display_name": "Sonnet 4"},
				"workspace": {},
				"cost": {},
				"context_window": {}
			}`,
			wantNil:     false,
			wantProject: "fallback-project",
			wantModel:   "Sonnet 4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Write test file
			testFile := filepath.Join(tmpDir, "test-statusline.json")
			if err := os.WriteFile(testFile, []byte(tt.fileContent), 0644); err != nil {
				t.Fatalf("Failed to write test file: %v", err)
			}
			dataFilePath = testFile

			got := readStatusLineData()

			if tt.wantNil {
				if got != nil {
					t.Errorf("readStatusLineData() = %+v, want nil", got)
				}
				return
			}

			if got == nil {
				t.Fatal("readStatusLineData() = nil, want non-nil")
			}

			if got.ProjectName != tt.wantProject {
				t.Errorf("ProjectName = %q, want %q", got.ProjectName, tt.wantProject)
			}
			if got.ModelName != tt.wantModel {
				t.Errorf("ModelName = %q, want %q", got.ModelName, tt.wantModel)
			}
			if got.TotalTokens != tt.wantTokens {
				t.Errorf("TotalTokens = %d, want %d", got.TotalTokens, tt.wantTokens)
			}
		})
	}

	// Test file not found
	t.Run("File not found", func(t *testing.T) {
		dataFilePath = filepath.Join(tmpDir, "nonexistent.json")
		if got := readStatusLineData(); got != nil {
			t.Errorf("readStatusLineData() with missing file = %+v, want nil", got)
		}
	})
}

// TestParseJSONLSession tests parsing JSONL transcript files
func TestParseJSONLSession(t *testing.T) {
	// Save and restore original sessionStartTime
	origSessionStartTime := sessionStartTime
	defer func() { sessionStartTime = origSessionStartTime }()
	sessionStartTime = time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	tmpDir, err := os.MkdirTemp("", "cc-discord-presence-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	tests := []struct {
		name        string
		content     string
		wantNil     bool
		wantTokens  int64
		wantModel   string
		wantProject string
	}{
		{
			name: "Valid session with multiple messages",
			content: `{"type":"user","cwd":"/Users/test/project"}
{"type":"assistant","message":{"model":"claude-sonnet-4-20250514","usage":{"input_tokens":1000,"output_tokens":500}}}
{"type":"user"}
{"type":"assistant","message":{"model":"claude-sonnet-4-20250514","usage":{"input_tokens":2000,"output_tokens":1000}}}`,
			wantNil:     false,
			wantTokens:  2000, // last request's input: context size
			wantModel:   "Sonnet 4",
			wantProject: "project",
		},
		{
			name: "Cached tokens count toward context size",
			content: `{"type":"user","cwd":"/Users/test/cached"}
{"type":"assistant","message":{"model":"claude-sonnet-4-20250514","usage":{"input_tokens":2,"cache_creation_input_tokens":446,"cache_read_input_tokens":53664,"output_tokens":294}}}`,
			wantNil:     false,
			wantTokens:  54112, // 2+446+53664
			wantModel:   "Sonnet 4",
			wantProject: "cached",
		},
		{
			name:    "Empty file",
			content: "",
			wantNil: true,
		},
		{
			name: "No assistant messages",
			content: `{"type":"user","cwd":"/Users/test/project"}
{"type":"user"}`,
			wantNil: true,
		},
		{
			name: "Mixed valid and invalid JSON lines",
			content: `{"type":"user","cwd":"/Users/test/myapp"}
invalid json line
{"type":"assistant","message":{"model":"claude-haiku-4-5-20241022","usage":{"input_tokens":500,"output_tokens":100}}}`,
			wantNil:     false,
			wantTokens:  500,
			wantModel:   "Haiku 4.5",
			wantProject: "myapp",
		},
		{
			name: "Multiple models uses last one",
			content: `{"type":"user","cwd":"/Users/test/multimodel"}
{"type":"assistant","message":{"model":"claude-haiku-4-5-20241022","usage":{"input_tokens":100,"output_tokens":50}}}
{"type":"assistant","message":{"model":"claude-opus-4-5-20251101","usage":{"input_tokens":200,"output_tokens":100}}}`,
			wantNil:     false,
			wantTokens:  200,
			wantModel:   "Opus 4.5", // Last model used
			wantProject: "multimodel",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testFile := filepath.Join(tmpDir, "test.jsonl")
			if err := os.WriteFile(testFile, []byte(tt.content), 0644); err != nil {
				t.Fatalf("Failed to write test file: %v", err)
			}

			got := parseJSONLSession(testFile, "")

			if tt.wantNil {
				if got != nil {
					t.Errorf("parseJSONLSession() = %+v, want nil", got)
				}
				return
			}

			if got == nil {
				t.Fatal("parseJSONLSession() = nil, want non-nil")
			}

			if got.TotalTokens != tt.wantTokens {
				t.Errorf("TotalTokens = %d, want %d", got.TotalTokens, tt.wantTokens)
			}
			if got.ModelName != tt.wantModel {
				t.Errorf("ModelName = %q, want %q", got.ModelName, tt.wantModel)
			}
			if got.ProjectName != tt.wantProject {
				t.Errorf("ProjectName = %q, want %q", got.ProjectName, tt.wantProject)
			}
		})
	}
}

// TestPathDecoding tests the path decoding logic used in findMostRecentJSONL
func TestPathDecoding(t *testing.T) {
	// This tests the path decoding algorithm used in findMostRecentJSONL
	// The encoding is: / becomes -, and literal - becomes --
	// Decoding: -- → placeholder → - to / → placeholder to -

	tests := []struct {
		name    string
		encoded string
		want    string
	}{
		{
			name:    "Simple path",
			encoded: "-Users-foo-project",
			want:    "/Users/foo/project",
		},
		{
			name:    "Path with literal dash",
			encoded: "-Users-foo-my--project",
			want:    "/Users/foo/my-project",
		},
		{
			name:    "Path with multiple literal dashes",
			encoded: "-Users-foo--bar-my----special----project",
			want:    "/Users/foo-bar/my--special--project",
		},
		{
			name:    "Root path",
			encoded: "-",
			want:    "/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Apply the same decoding logic as findMostRecentJSONL
			decoded := tt.encoded
			decoded = replaceDoubleDash(decoded, "\x00")
			decoded = replaceSingleDash(decoded, "/")
			decoded = replacePlaceholder(decoded, "-")

			if decoded != tt.want {
				t.Errorf("decoding %q = %q, want %q", tt.encoded, decoded, tt.want)
			}
		})
	}
}

// Helper functions to match the decoding logic in findMostRecentJSONL
func replaceDoubleDash(s, replacement string) string {
	result := ""
	i := 0
	for i < len(s) {
		if i+1 < len(s) && s[i] == '-' && s[i+1] == '-' {
			result += replacement
			i += 2
		} else {
			result += string(s[i])
			i++
		}
	}
	return result
}

func replaceSingleDash(s, replacement string) string {
	result := ""
	for _, c := range s {
		if c == '-' {
			result += replacement
		} else {
			result += string(c)
		}
	}
	return result
}

func replacePlaceholder(s, replacement string) string {
	result := ""
	for _, c := range s {
		if c == '\x00' {
			result += replacement
		} else {
			result += string(c)
		}
	}
	return result
}

// TestFindMostRecentJSONL tests finding the most recent JSONL file
func TestFindMostRecentJSONL(t *testing.T) {
	// Save original projectsDir
	origProjectsDir := projectsDir
	defer func() { projectsDir = origProjectsDir }()

	tmpDir, err := os.MkdirTemp("", "cc-discord-presence-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	t.Run("No projects directory", func(t *testing.T) {
		projectsDir = filepath.Join(tmpDir, "nonexistent")
		_, _, err := findMostRecentJSONL()
		if err == nil {
			t.Error("Expected error for nonexistent directory, got nil")
		}
	})

	t.Run("Empty projects directory", func(t *testing.T) {
		emptyDir := filepath.Join(tmpDir, "empty")
		os.MkdirAll(emptyDir, 0755)
		projectsDir = emptyDir

		_, _, err := findMostRecentJSONL()
		if err == nil {
			t.Error("Expected error for empty directory, got nil")
		}
	})

	t.Run("Single JSONL file", func(t *testing.T) {
		singleDir := filepath.Join(tmpDir, "single")
		projectDir := filepath.Join(singleDir, "-Users-test-myproject")
		os.MkdirAll(projectDir, 0755)
		projectsDir = singleDir

		jsonlFile := filepath.Join(projectDir, "session.jsonl")
		os.WriteFile(jsonlFile, []byte(`{"type":"test"}`), 0644)

		path, projectPath, err := findMostRecentJSONL()
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if path != jsonlFile {
			t.Errorf("path = %q, want %q", path, jsonlFile)
		}
		if projectPath != "/Users/test/myproject" {
			t.Errorf("projectPath = %q, want %q", projectPath, "/Users/test/myproject")
		}
	})

	t.Run("Multiple JSONL files - returns most recent", func(t *testing.T) {
		multiDir := filepath.Join(tmpDir, "multi")
		projectDir := filepath.Join(multiDir, "-Users-test-project")
		os.MkdirAll(projectDir, 0755)
		projectsDir = multiDir

		// Create older file
		oldFile := filepath.Join(projectDir, "old.jsonl")
		os.WriteFile(oldFile, []byte(`{"type":"old"}`), 0644)
		oldTime := time.Now().Add(-time.Hour)
		os.Chtimes(oldFile, oldTime, oldTime)

		// Create newer file
		newFile := filepath.Join(projectDir, "new.jsonl")
		os.WriteFile(newFile, []byte(`{"type":"new"}`), 0644)

		path, _, err := findMostRecentJSONL()
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if path != newFile {
			t.Errorf("Expected newest file %q, got %q", newFile, path)
		}
	})
}

// TestReadSessionDataPriority tests that the hooks module's file wins over the
// statusline file, and that a blanked module file falls back to it.
func TestReadSessionDataPriority(t *testing.T) {
	origData, origModule, origProjects := dataFilePath, moduleFilePath, projectsDir
	origSource, origNudge := currentSource, nudgeShown
	defer func() {
		dataFilePath, moduleFilePath, projectsDir = origData, origModule, origProjects
		currentSource, nudgeShown = origSource, origNudge
	}()

	tmpDir := t.TempDir()
	dataFilePath = filepath.Join(tmpDir, "discord-presence-data.json")
	moduleFilePath = filepath.Join(tmpDir, "discord-presence-module.json")
	projectsDir = filepath.Join(tmpDir, "projects") // empty: no JSONL fallback
	nudgeShown = true

	presence := func(project string) string {
		return `{"session_id":"s-` + project + `","cwd":"/work/` + project + `",` +
			`"model":{"id":"claude-opus-5-5","display_name":"Opus 5.5"},` +
			`"workspace":{"current_dir":"/work/` + project + `","project_dir":"/work/` + project + `"},` +
			`"context_window":{"total_input_tokens":1234}}`
	}
	write := func(path, content string) {
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	tests := []struct {
		name        string
		module      string // "" means no file
		statusLine  string
		wantProject string
		wantSource  string
	}{
		{"module wins over statusline", presence("from-module"), presence("from-statusline"), "from-module", sourceModule},
		{"blanked module falls back", "{}", presence("from-statusline"), "from-statusline", sourceStatusLine},
		{"missing module falls back", "", presence("from-statusline"), "from-statusline", sourceStatusLine},
		{"module alone", presence("from-module"), "", "from-module", sourceModule},
		{"neither", "", "", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Remove(moduleFilePath)
			os.Remove(dataFilePath)
			if tt.module != "" {
				write(moduleFilePath, tt.module)
			}
			if tt.statusLine != "" {
				write(dataFilePath, tt.statusLine)
			}
			currentSource = ""

			got := readSessionData()
			if tt.wantProject == "" {
				if got != nil {
					t.Fatalf("readSessionData() = %+v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("readSessionData() = nil, want non-nil")
			}
			if got.ProjectName != tt.wantProject {
				t.Errorf("ProjectName = %q, want %q", got.ProjectName, tt.wantProject)
			}
			if currentSource != tt.wantSource {
				t.Errorf("currentSource = %q, want %q", currentSource, tt.wantSource)
			}
			if got.TotalTokens != 1234 {
				t.Errorf("TotalTokens = %d, want 1234", got.TotalTokens)
			}
		})
	}
}

// TestPresenceText tests the activity lines Discord shows.
func TestPresenceText(t *testing.T) {
	details, state := presenceText(&SessionData{
		ProjectName: "kadens",
		GitBranch:   "main",
		ModelName:   "Opus 5.5",
		TotalTokens: 54112,
		TotalCost:   1.27,
		CostKnown:   true,
	})
	if details != "Working on: kadens (main)" {
		t.Errorf("details = %q", details)
	}
	if state != "Opus 5.5 | 54.1K tokens | $1.2700" {
		t.Errorf("state = %q", state)
	}
}

// The Windows start script hands the log path to the daemon instead of
// redirecting its output (see start.sh).
func TestSetupLogSendsOutputToFile(t *testing.T) {
	origOut, origErr := os.Stdout, os.Stderr
	path := filepath.Join(t.TempDir(), "discord-presence.log")

	if err := setupLog([]string{"--log", path}); err != nil {
		t.Fatalf("setupLog: %v", err)
	}
	fmt.Println("to stdout")
	fmt.Fprintln(os.Stderr, "to stderr")
	if os.Stdout != origOut {
		os.Stdout.Close()
	}
	os.Stdout, os.Stderr = origOut, origErr

	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "to stdout") || !strings.Contains(string(data), "to stderr") {
		t.Errorf("log file holds %q, want both stdout and stderr output", data)
	}
}

func TestSetupLogWithoutFlagKeepsOutput(t *testing.T) {
	origOut := os.Stdout
	if err := setupLog(nil); err != nil {
		t.Fatalf("setupLog: %v", err)
	}
	if os.Stdout != origOut {
		t.Error("setupLog without --log replaced os.Stdout")
	}
}

// While Discord is away the client keeps the activity and sends it once it
// reconnects, so the daemon must not retry (and log an error) every poll.
func TestUpdatePresenceWhileDiscordIsAway(t *testing.T) {
	origClient, origLast := discordClient, lastPresence
	defer func() { discordClient, lastPresence = origClient, origLast }()
	discordClient = discord.NewClient("test") // never connected
	lastPresence = ""

	updatePresence(&SessionData{ProjectName: "kadens", ModelName: "Opus 5.5", StartTime: time.Now()})

	if lastPresence == "" {
		t.Error("activity not recorded as handed over; every poll would resend it and log an error")
	}
}

// Transcripts hold lines of several megabytes (large tool results). A
// bufio.Scanner stops at its buffer limit and the rest of the file was
// silently ignored, so the card showed an old model and token count.
func TestParseJSONLSessionReadsPastLongLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	longLine := `{"type":"user","message":{"content":"` + strings.Repeat("x", 3<<20) + `"}}`
	content := `{"type":"user","cwd":"/work/kadens"}` + "\n" +
		`{"type":"assistant","message":{"model":"claude-haiku-4-5-20241022","usage":{"input_tokens":10}}}` + "\n" +
		longLine + "\n" +
		`{"type":"assistant","message":{"model":"claude-opus-4-5-20251101","usage":{"input_tokens":50000}}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	got := parseJSONLSession(path, "")

	if got.ModelName != "Opus 4.5" || got.TotalTokens != 50000 {
		t.Errorf("model = %q, tokens = %d; want the message after the long line (Opus 4.5, 50000)", got.ModelName, got.TotalTokens)
	}
}

func TestFormatModelName(t *testing.T) {
	tests := []struct {
		modelID string
		want    string
	}{
		{"claude-opus-4-5-20251101", "Opus 4.5"},
		{"claude-sonnet-4-20250514", "Sonnet 4"},
		{"claude-haiku-4-5-20251001", "Haiku 4.5"},
		{"claude-opus-5", "Opus 5"},
		{"claude-opus-5-5", "Opus 5.5"},
		{"claude-fable-5-1", "Fable 5.1"},
		{"claude-sonnet-5-20260101", "Sonnet 5"},
		{"claude-3-5-haiku-20241022", "Haiku 3.5"},
		{"claude-3-opus-20240229", "Opus 3"},
		{"us.anthropic.claude-sonnet-4-5-20250929-v1:0", "Sonnet 4.5"}, // Bedrock
		{"claude-opus-4-5@20251101", "Opus 4.5"},                       // Vertex AI
		{"<synthetic>", "Claude"},
		{"some-unknown-model", "Claude"},
		{"", "Claude"},
	}
	for _, tt := range tests {
		if got := formatModelName(tt.modelID); got != tt.want {
			t.Errorf("formatModelName(%q) = %q, want %q", tt.modelID, got, tt.want)
		}
	}
}

// Prices from https://platform.claude.com/docs/en/about-claude/pricing
func TestMessageCost(t *testing.T) {
	const M = 1_000_000
	tests := []struct {
		name    string
		modelID string
		usage   Usage
		want    float64
	}{
		{"input and output", "claude-opus-4-5-20251101", Usage{InputTokens: M, OutputTokens: M / 10}, 5 + 2.5},
		{"older Opus", "claude-opus-4-1-20250805", Usage{InputTokens: M}, 15},
		{"date-suffixed Sonnet 4", "claude-sonnet-4-20250514", Usage{InputTokens: M / 2, OutputTokens: M / 20}, 1.5 + 0.75},
		{"old naming", "claude-3-5-haiku-20241022", Usage{InputTokens: M}, 0.8},
		{"output", "claude-opus-5-5", Usage{OutputTokens: M}, 20},
		{"cache read at 0.05x", "claude-opus-5-5", Usage{CacheReadInputTokens: M}, 0.20},
		{"cache read at 0.025x", "claude-fable-5-1", Usage{CacheReadInputTokens: M}, 0.25},
		{"cache read at 0.1x", "claude-sonnet-4-6", Usage{CacheReadInputTokens: M}, 0.30},
		{"1h cache write at 2x", "claude-opus-5-5", Usage{CacheCreationInputTokens: M, CacheCreation: CacheCreation{Ephemeral1h: M}}, 8},
		{"5m cache write at 1.25x", "claude-opus-5-5", Usage{CacheCreationInputTokens: M, CacheCreation: CacheCreation{Ephemeral5m: M}}, 5},
		{"cache write without TTL split is 5m", "claude-opus-4-5-20251101", Usage{CacheCreationInputTokens: M}, 6.25},
		{"fast mode", "claude-opus-5-5", Usage{InputTokens: M, OutputTokens: M, Speed: "fast"}, 8 + 40},
		{"fast mode cache read", "claude-opus-5", Usage{CacheReadInputTokens: M, Speed: "fast"}, 1},
		{"US-only inference on 4.6+", "claude-opus-4-6", Usage{InputTokens: M, InferenceGeo: "us"}, 5.5},
		{"US-only inference before 4.6", "claude-haiku-4-5-20251001", Usage{InputTokens: M, InferenceGeo: "us"}, 1},
		{"Haiku 5.5 short prompt", "claude-haiku-5-5", Usage{InputTokens: 50_000}, 0.005},
		{"Haiku 5.5 prompt over 100k", "claude-haiku-5-5", Usage{InputTokens: 50_000, CacheReadInputTokens: 100_000}, 0.025 + 0.005},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := messageCost(tt.modelID, tt.usage)
			if !ok {
				t.Fatalf("messageCost(%q) has no price", tt.modelID)
			}
			if diff := got - tt.want; diff > 1e-9 || diff < -1e-9 {
				t.Errorf("messageCost(%q, %+v) = %v, want %v", tt.modelID, tt.usage, got, tt.want)
			}
		})
	}

	if _, ok := messageCost("claude-unknown-9", Usage{InputTokens: M}); ok {
		t.Error("an unknown model got a price; the card should show no cost rather than a guess")
	}
}

func writeTranscript(t *testing.T, lines ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "session.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

// Claude Code writes one line per content block of a response (thinking,
// text, each tool call), all carrying the same message id and usage.
func TestParseJSONLSessionCountsEachMessageOnce(t *testing.T) {
	block := `{"type":"assistant","message":{"id":"msg_1","model":"claude-opus-5-5","usage":{"input_tokens":2,"cache_creation_input_tokens":1000000,"cache_read_input_tokens":0,"output_tokens":1000000,"cache_creation":{"ephemeral_1h_input_tokens":1000000,"ephemeral_5m_input_tokens":0}}}}`
	path := writeTranscript(t,
		`{"type":"user","cwd":"/work/kadens"}`,
		block, block, block,
		`{"type":"assistant","message":{"id":"msg_2","model":"claude-opus-5-5","usage":{"input_tokens":3,"cache_creation_input_tokens":0,"cache_read_input_tokens":1000000,"output_tokens":0}}}`,
	)

	got := parseJSONLSession(path, "")

	// msg_1: 1M 1h write ($8) + 1M output ($20); msg_2: 1M cache read ($0.20)
	want := 8 + 20 + 0.20 + float64(2+3)*4/1_000_000
	if diff := got.TotalCost - want; diff > 1e-9 || diff < -1e-9 || !got.CostKnown {
		t.Errorf("TotalCost = %v (known %v), want %v", got.TotalCost, got.CostKnown, want)
	}
	if got.ModelName != "Opus 5.5" {
		t.Errorf("ModelName = %q, want %q", got.ModelName, "Opus 5.5")
	}
	if got.TotalTokens != 1_000_003 {
		t.Errorf("TotalTokens = %d, want the last request's context, 1000003", got.TotalTokens)
	}
}

// Claude Code records API errors and interruptions as assistant messages
// with model "<synthetic>" and zero usage.
func TestParseJSONLSessionIgnoresSyntheticMessages(t *testing.T) {
	path := writeTranscript(t,
		`{"type":"user","cwd":"/work/kadens"}`,
		`{"type":"assistant","message":{"id":"msg_1","model":"claude-opus-5-5","usage":{"input_tokens":5,"cache_read_input_tokens":40000,"output_tokens":10}}}`,
		`{"type":"assistant","message":{"id":"msg_2","model":"<synthetic>","usage":{"input_tokens":0,"output_tokens":0}}}`,
	)

	got := parseJSONLSession(path, "")

	if got.ModelName != "Opus 5.5" || got.TotalTokens != 40005 {
		t.Errorf("model = %q, tokens = %d; want Opus 5.5 and 40005 from the last real response", got.ModelName, got.TotalTokens)
	}
}

func TestParseJSONLSessionUnknownModelHasNoCost(t *testing.T) {
	path := writeTranscript(t,
		`{"type":"user","cwd":"/work/kadens"}`,
		`{"type":"assistant","message":{"id":"msg_1","model":"claude-future-9","usage":{"input_tokens":1000}}}`,
	)

	if got := parseJSONLSession(path, ""); got.CostKnown {
		t.Errorf("CostKnown = true with TotalCost %v for a model without a price", got.TotalCost)
	}
}

func TestPresenceTextWithoutCost(t *testing.T) {
	_, state := presenceText(&SessionData{ModelName: "Claude", TotalTokens: 54112})
	if state != "Claude | 54.1K tokens" {
		t.Errorf("state = %q, want the cost left out", state)
	}
}

// The hooks module and statusline report Claude Code's own cost figure.
func TestReadPresenceFileCostIsKnown(t *testing.T) {
	path := filepath.Join(t.TempDir(), "module.json")
	os.WriteFile(path, []byte(`{"session_id":"s1","cwd":"/work/kadens","model":{"display_name":"Opus 5.5"},"cost":{"total_cost_usd":1.27}}`), 0644)

	got := readPresenceFile(path)

	if got == nil || !got.CostKnown || got.TotalCost != 1.27 {
		t.Errorf("readPresenceFile = %+v, want TotalCost 1.27 marked known", got)
	}
}

func TestSessionDataCarriesLastActivity(t *testing.T) {
	hourAgo := time.Now().Add(-time.Hour).Truncate(time.Second)

	module := filepath.Join(t.TempDir(), "module.json")
	os.WriteFile(module, []byte(`{"session_id":"s1","cwd":"/work/kadens"}`), 0644)
	os.Chtimes(module, hourAgo, hourAgo)
	if got := readPresenceFile(module); got == nil || !got.LastActivity.Equal(hourAgo) {
		t.Errorf("readPresenceFile LastActivity = %v, want the file's mtime %v", got, hourAgo)
	}

	transcript := writeTranscript(t,
		`{"type":"user","cwd":"/work/kadens"}`,
		`{"type":"assistant","message":{"id":"m1","model":"claude-opus-5-5","usage":{"input_tokens":1}}}`)
	os.Chtimes(transcript, hourAgo, hourAgo)
	if got := parseJSONLSession(transcript, ""); got == nil || !got.LastActivity.Equal(hourAgo) {
		t.Errorf("parseJSONLSession LastActivity = %v, want the file's mtime %v", got, hourAgo)
	}
}

// useSessionFiles points the daemon at empty session data in a temp dir and a
// Discord client that isn't connected, restoring both afterwards.
func useSessionFiles(t *testing.T) (module string) {
	t.Helper()
	origData, origModule, origProjects := dataFilePath, moduleFilePath, projectsDir
	origClient, origLast, origNudge := discordClient, lastPresence, nudgeShown
	origProcesses := claudeCodeProcesses
	t.Cleanup(func() {
		dataFilePath, moduleFilePath, projectsDir = origData, origModule, origProjects
		discordClient, lastPresence, nudgeShown = origClient, origLast, origNudge
		claudeCodeProcesses = origProcesses
	})
	claudeCodeProcesses = func() []int { return []int{4242} } // one session open
	dir := t.TempDir()
	dataFilePath = filepath.Join(dir, "discord-presence-data.json")
	moduleFilePath = filepath.Join(dir, "discord-presence-module.json")
	projectsDir = filepath.Join(dir, "projects")
	discordClient = discord.NewClient("test")
	nudgeShown = true
	return moduleFilePath
}

func TestRefreshPresenceClearsCardWithoutSession(t *testing.T) {
	useSessionFiles(t)
	lastPresence = "Working on: kadens"

	refreshPresence()

	if lastPresence != "" {
		t.Errorf("card not cleared with no session data (lastPresence %q)", lastPresence)
	}
}

func TestRefreshPresenceShowsActiveSession(t *testing.T) {
	module := useSessionFiles(t)
	os.WriteFile(module, []byte(`{"session_id":"s1","cwd":"/work/kadens","model":{"display_name":"Opus 5.5"}}`), 0644)

	refreshPresence()

	if !strings.Contains(lastPresence, "kadens") {
		t.Errorf("active session not shown (lastPresence %q)", lastPresence)
	}
}

// A crashed session, or one whose terminal was closed without its SessionEnd
// hook, must not keep the card.
func TestRefreshPresenceClearsIdleSessionWhenClaudeCodeIsClosed(t *testing.T) {
	module := useSessionFiles(t)
	claudeCodeProcesses = func() []int { return nil }
	os.WriteFile(module, []byte(`{"session_id":"s1","cwd":"/work/kadens","model":{"display_name":"Opus 5.5"}}`), 0644)
	longAgo := time.Now().Add(-IdleTimeout - time.Minute)
	os.Chtimes(module, longAgo, longAgo)
	lastPresence = "Working on: kadens"

	refreshPresence()

	if lastPresence != "" {
		t.Errorf("card kept for a session idle longer than IdleTimeout (lastPresence %q)", lastPresence)
	}
}

func TestSessionIsIdle(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name     string
		lastSeen time.Duration // before now
		want     bool
	}{
		{"just active", time.Minute, false},
		{"inside the timeout", IdleTimeout - time.Minute, false},
		{"past the timeout", IdleTimeout + time.Minute, true},
		{"a day ago", 24 * time.Hour, true},
	}
	for _, tt := range tests {
		session := &SessionData{LastActivity: now.Add(-tt.lastSeen)}
		if got := sessionIsIdle(session, now); got != tt.want {
			t.Errorf("%s: sessionIsIdle = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestSessionIsIdleWithoutActivityTime(t *testing.T) {
	if sessionIsIdle(&SessionData{}, time.Now()) {
		t.Error("a session with no known activity time was hidden as idle")
	}
}

// The elapsed timer showed how long the daemon had run, which is shared by
// every session and survives none of them.
func TestReadPresenceFileStartsTimerAtSessionStart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "module.json")
	os.WriteFile(path, []byte(`{"session_id":"s1","cwd":"/work/kadens","cost":{"total_duration_ms":7200000}}`), 0644)
	written := time.Now().Add(-time.Minute).Truncate(time.Second)
	os.Chtimes(path, written, written)

	got := readPresenceFile(path)

	if want := written.Add(-2 * time.Hour); got == nil || !got.StartTime.Equal(want) {
		t.Errorf("StartTime = %v, want %v (written 2h into the session)", got.StartTime, want)
	}
}

func transcriptLine(typ string, at time.Time) string {
	return `{"type":"` + typ + `","cwd":"/work/kadens","timestamp":"` + at.UTC().Format(time.RFC3339Nano) + `",` +
		`"message":{"id":"m` + at.Format("150405") + `","model":"claude-opus-5-5","usage":{"input_tokens":1}}}`
}

func TestParseJSONLSessionStartsTimerAtFirstMessage(t *testing.T) {
	start := time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)
	path := writeTranscript(t,
		transcriptLine("user", start),
		transcriptLine("assistant", start.Add(5*time.Minute)),
		transcriptLine("assistant", start.Add(5*time.Minute+IdleTimeout-time.Second))) // a gap just inside

	if got := parseJSONLSession(path, ""); !got.StartTime.Equal(start) {
		t.Errorf("StartTime = %v, want the first message at %v", got.StartTime, start)
	}
}

// A session resumed after a break (the next morning, after lunch) starts a
// new stretch of work rather than showing "20:00:00 elapsed".
func TestParseJSONLSessionRestartsTimerAfterIdleGap(t *testing.T) {
	start := time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)
	resumed := start.Add(IdleTimeout + time.Hour)
	path := writeTranscript(t,
		transcriptLine("user", start),
		transcriptLine("assistant", start.Add(5*time.Minute)),
		transcriptLine("user", resumed),
		transcriptLine("assistant", resumed.Add(2*time.Minute)))

	if got := parseJSONLSession(path, ""); !got.StartTime.Equal(resumed) {
		t.Errorf("StartTime = %v, want the resumed stretch at %v", got.StartTime, resumed)
	}
}

// stop.sh records ended sessions; their transcripts are the freshest right
// after they close, but the card should move on to a session still running.
func TestFindMostRecentJSONLSkipsEndedSessions(t *testing.T) {
	origProjects, origEnded := projectsDir, endedDir
	defer func() { projectsDir, endedDir = origProjects, origEnded }()
	dir := t.TempDir()
	projectsDir, endedDir = filepath.Join(dir, "projects"), filepath.Join(dir, "ended")
	os.MkdirAll(filepath.Join(projectsDir, "C--work-kadens"), 0755)
	os.MkdirAll(endedDir, 0755)

	write := func(path string, age time.Duration) {
		os.WriteFile(path, []byte("{}\n"), 0644)
		at := time.Now().Add(-age)
		os.Chtimes(path, at, at)
	}
	running := filepath.Join(projectsDir, "C--work-kadens", "running.jsonl")
	closed := filepath.Join(projectsDir, "C--work-kadens", "closed.jsonl")
	write(running, 5*time.Minute)
	write(closed, time.Minute)
	write(filepath.Join(endedDir, "closed"), 0)

	if got, _, _ := findMostRecentJSONL(); got != running {
		t.Errorf("picked %s, want the session still running", filepath.Base(got))
	}

	// Resumed after it ended: its transcript changes again
	write(closed, -time.Minute)
	if got, _, _ := findMostRecentJSONL(); got != closed {
		t.Errorf("picked %s, want the resumed session", filepath.Base(got))
	}
}

// While Claude Code is open in some terminal, an idle session shows as idle,
// as editor integrations do, rather than disappearing.
func TestRefreshPresenceShowsIdleSessionWhileClaudeCodeIsOpen(t *testing.T) {
	module := useSessionFiles(t)
	os.WriteFile(module, []byte(`{"session_id":"s1","cwd":"/work/kadens","model":{"display_name":"Opus 5.5"}}`), 0644)
	longAgo := time.Now().Add(-IdleTimeout - time.Minute)
	os.Chtimes(module, longAgo, longAgo)

	refreshPresence()

	if !strings.HasPrefix(lastPresence, "Idling in: kadens") {
		t.Errorf("idle session with Claude Code open shown as %q, want it idling", lastPresence)
	}
}

func TestPresenceTextIdle(t *testing.T) {
	details, _ := presenceText(&SessionData{ProjectName: "kadens", GitBranch: "main", Idle: true})
	if details != "Idling in: kadens (main)" {
		t.Errorf("details = %q", details)
	}
}

// The daemon outlives a terminal closed without its SessionEnd hook (window
// closed, crash); it stops once Claude Code is gone everywhere.
func TestClaudeCodeGone(t *testing.T) {
	orig := claudeCodeProcesses
	origSeen, origLastSeen := sawClaudeCode, lastSawClaudeCode
	defer func() { claudeCodeProcesses, sawClaudeCode, lastSawClaudeCode = orig, origSeen, origLastSeen }()
	sawClaudeCode, lastSawClaudeCode = false, time.Time{}
	var running []int
	claudeCodeProcesses = func() []int { return running }
	now := time.Now()

	if claudeCodeGone(now.Add(time.Hour)) {
		t.Error("gone although no Claude Code process was ever found: on such a system the hooks stop the daemon")
	}
	running = []int{4242}
	if claudeCodeGone(now) {
		t.Error("gone while Claude Code runs")
	}
	running = nil
	if claudeCodeGone(now.Add(ExitGrace - time.Second)) {
		t.Error("gone before ExitGrace passed")
	}
	if !claudeCodeGone(now.Add(ExitGrace + time.Second)) {
		t.Error("not gone after Claude Code closed everywhere for longer than ExitGrace")
	}
}
