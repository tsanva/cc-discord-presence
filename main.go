package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/tsanva/cc-discord-presence/discord"
)

const (
	// Discord Application ID for "Clawd Code"
	ClientID = "1455326944060248250"

	// Polling interval as fallback
	PollInterval = 3 * time.Second

	// IdleTimeout is how long a session can go without activity before the
	// card shows it as idle, or clears if Claude Code isn't running. It
	// outlasts the longest single tool call (Claude Code caps a Bash command
	// at 10 minutes), so a long build or test run still shows as working.
	IdleTimeout = 15 * time.Minute

	// ExitGrace is how long the daemon outlives the last Claude Code process,
	// for a terminal closed without its SessionEnd hook.
	ExitGrace = 30 * time.Second
)

// modelPrice is a model's list price in dollars per million tokens. Cache
// writes cost 1.25x the input price (5-minute cache) or 2x (1-hour cache);
// CacheRead is the multiplier for cache hits.
type modelPrice struct {
	Input, Output         float64
	CacheRead             float64
	FastInput, FastOutput float64 // fast mode, where offered
	LongInput, LongOutput float64 // prompts over 100k tokens, where pricing depends on length
}

// modelPrices is keyed by formatModelName. Update it when models are
// released: https://platform.claude.com/docs/en/about-claude/pricing
var modelPrices = map[string]modelPrice{
	"Fable 5.1":  {Input: 10, Output: 50, CacheRead: 0.025},
	"Mythos 5.1": {Input: 10, Output: 50, CacheRead: 0.025},
	"Fable 5":    {Input: 10, Output: 50, CacheRead: 0.1},
	"Mythos 5":   {Input: 10, Output: 50, CacheRead: 0.1},
	"Opus 5.5":   {Input: 4, Output: 20, CacheRead: 0.05, FastInput: 8, FastOutput: 40},
	"Opus 5":     {Input: 5, Output: 25, CacheRead: 0.1, FastInput: 10, FastOutput: 50},
	"Opus 4.8":   {Input: 5, Output: 25, CacheRead: 0.1, FastInput: 10, FastOutput: 50},
	"Opus 4.7":   {Input: 5, Output: 25, CacheRead: 0.1},
	"Opus 4.6":   {Input: 5, Output: 25, CacheRead: 0.1},
	"Opus 4.5":   {Input: 5, Output: 25, CacheRead: 0.1},
	"Opus 4.1":   {Input: 15, Output: 75, CacheRead: 0.1},
	"Opus 4":     {Input: 15, Output: 75, CacheRead: 0.1},
	"Sonnet 5.5": {Input: 2, Output: 10, CacheRead: 0.05},
	"Sonnet 5":   {Input: 2, Output: 10, CacheRead: 0.1},
	"Sonnet 4.6": {Input: 3, Output: 15, CacheRead: 0.1},
	"Sonnet 4.5": {Input: 3, Output: 15, CacheRead: 0.1},
	"Sonnet 4":   {Input: 3, Output: 15, CacheRead: 0.1},
	"Haiku 5.5":  {Input: 0.10, Output: 0.50, CacheRead: 0.1, LongInput: 0.50, LongOutput: 2.50},
	"Haiku 4.5":  {Input: 1, Output: 5, CacheRead: 0.1},
	"Haiku 3.5":  {Input: 0.80, Output: 4, CacheRead: 0.1},
}

var (
	// claude-opus-4-5-20251101, claude-opus-5-5, claude-sonnet-4-20250514
	modelIDPattern = regexp.MustCompile(`^claude-([a-z]+)-(\d+)(?:-(\d{1,2}))?(?:-\d{8})?$`)
	// claude-3-5-haiku-20241022
	oldModelIDPattern = regexp.MustCompile(`^claude-(\d+)(?:-(\d{1,2}))?-([a-z]+)(?:-\d{8})?$`)
	// Bedrock's version suffix: us.anthropic.claude-sonnet-4-5-20250929-v1:0
	bedrockSuffix = regexp.MustCompile(`-v\d+(:\d+)?$`)
)

// parseModelID reads the family and version from a model ID, including the
// Bedrock and Vertex AI forms.
func parseModelID(id string) (family string, major, minor int, ok bool) {
	if i := strings.Index(id, "claude-"); i > 0 {
		id = id[i:] // us.anthropic.claude-…
	}
	id, _, _ = strings.Cut(id, "@") // claude-opus-4-5@20251101
	id = bedrockSuffix.ReplaceAllString(id, "")

	var majorStr, minorStr string
	if m := modelIDPattern.FindStringSubmatch(id); m != nil {
		family, majorStr, minorStr = m[1], m[2], m[3]
	} else if m := oldModelIDPattern.FindStringSubmatch(id); m != nil {
		family, majorStr, minorStr = m[3], m[1], m[2]
	} else {
		return "", 0, 0, false
	}
	major, _ = strconv.Atoi(majorStr)
	minor, _ = strconv.Atoi(minorStr)
	return family, major, minor, true
}

// messageCost prices one API response from its usage. ok is false for a
// model missing from modelPrices.
func messageCost(modelID string, u Usage) (cost float64, ok bool) {
	family, major, minor, ok := parseModelID(modelID)
	if !ok {
		return 0, false
	}
	p, ok := modelPrices[displayName(family, major, minor)]
	if !ok {
		return 0, false
	}

	input, output := p.Input, p.Output
	if p.LongInput > 0 && u.InputTokens+u.CacheCreationInputTokens+u.CacheReadInputTokens > 100_000 {
		input, output = p.LongInput, p.LongOutput
	}
	if u.Speed == "fast" && p.FastInput > 0 {
		input, output = p.FastInput, p.FastOutput
	}
	// Usage recorded before 1-hour caching has no split: all 5-minute writes
	write5m, write1h := u.CacheCreation.Ephemeral5m, u.CacheCreation.Ephemeral1h
	if write5m+write1h == 0 {
		write5m = u.CacheCreationInputTokens
	}

	cost = (float64(u.InputTokens)*input +
		float64(write5m)*input*1.25 +
		float64(write1h)*input*2 +
		float64(u.CacheReadInputTokens)*input*p.CacheRead +
		float64(u.OutputTokens)*output) / 1_000_000
	// US-only inference costs 1.1x on Claude 4.6 and later
	if u.InferenceGeo == "us" && (major > 4 || major == 4 && minor >= 6) {
		cost *= 1.1
	}
	return cost, true
}

func displayName(family string, major, minor int) string {
	name := strings.ToUpper(family[:1]) + family[1:] + " " + strconv.Itoa(major)
	if minor > 0 {
		name += "." + strconv.Itoa(minor)
	}
	return name
}

// StatusLineData matches Claude Code's statusline JSON structure. The hooks
// module (hooks/register.ts) writes the same shape to its own file.
type StatusLineData struct {
	SessionID string `json:"session_id"`
	Cwd       string `json:"cwd"`
	Model     struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
	} `json:"model"`
	Workspace struct {
		CurrentDir string `json:"current_dir"`
		ProjectDir string `json:"project_dir"`
	} `json:"workspace"`
	Cost struct {
		TotalCostUSD       float64 `json:"total_cost_usd"`
		TotalDurationMS    int64   `json:"total_duration_ms"`
		TotalAPIDurationMS int64   `json:"total_api_duration_ms"`
	} `json:"cost"`
	ContextWindow struct {
		TotalInputTokens  int64 `json:"total_input_tokens"`
		TotalOutputTokens int64 `json:"total_output_tokens"`
	} `json:"context_window"`
}

// SessionData holds parsed session information
type SessionData struct {
	ProjectName string
	ProjectPath string
	GitBranch   string
	ModelName   string
	TotalTokens int64
	TotalCost   float64
	CostKnown   bool // false when a model in the session has no known price
	StartTime   time.Time
	// LastActivity is when the session's data last changed: Claude Code
	// writes it on every prompt, tool call and response.
	LastActivity time.Time
	Idle         bool
}

// Usage is the usage block of one API response, as Claude Code records it.
type Usage struct {
	InputTokens              int64         `json:"input_tokens"`
	OutputTokens             int64         `json:"output_tokens"`
	CacheCreationInputTokens int64         `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int64         `json:"cache_read_input_tokens"`
	CacheCreation            CacheCreation `json:"cache_creation"`
	Speed                    string        `json:"speed"`         // "fast" in fast mode
	InferenceGeo             string        `json:"inference_geo"` // "us" for US-only inference
}

// CacheCreation splits cache writes by lifetime, which are priced differently.
type CacheCreation struct {
	Ephemeral5m int64 `json:"ephemeral_5m_input_tokens"`
	Ephemeral1h int64 `json:"ephemeral_1h_input_tokens"`
}

// JSONLMessage represents a message entry in JSONL files
type JSONLMessage struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Cwd       string `json:"cwd"`
	Message   struct {
		ID    string `json:"id"`
		Model string `json:"model"`
		Usage Usage  `json:"usage"`
	} `json:"message"`
}

var (
	claudeDir        string
	projectsDir      string
	dataFilePath     string
	moduleFilePath   string
	endedDir         string // sessions stop.sh saw end, one empty file per session id
	sessionStartTime = time.Now()
	discordClient    *discord.Client
	currentSource    string
	nudgeShown       bool
	lastPresence     string
)

// Where session data came from, in the order readSessionData tries them.
const (
	sourceModule     = "hooks module"
	sourceStatusLine = "statusline data"
	sourceJSONL      = "JSONL fallback"
)

func init() {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting home directory: %v\n", err)
		os.Exit(1)
	}
	claudeDir = filepath.Join(home, ".claude")
	projectsDir = filepath.Join(claudeDir, "projects")
	dataFilePath = filepath.Join(claudeDir, "discord-presence-data.json")
	moduleFilePath = filepath.Join(claudeDir, "discord-presence-module.json")
	endedDir = filepath.Join(claudeDir, "discord-presence-ended")
}

// setupLog sends all output to the file named by "--log <path>". The Windows
// start script passes it because it can't redirect the daemon itself:
// Start-Process redirection makes the daemon inherit the hook's output pipe,
// and Claude Code then waits on the hook until it times out.
func setupLog(args []string) error {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "--log" {
			f, err := os.OpenFile(args[i+1], os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
			if err != nil {
				return err
			}
			os.Stdout, os.Stderr = f, f
			return nil
		}
	}
	return nil
}

func main() {
	if err := setupLog(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "Error opening log file: %v\n", err)
	}

	fmt.Println(`
╔═══════════════════════════════════════════════════════════╗
║     Clawd Code - Discord Rich Presence                    ║
║     Show your Claude Code session on Discord!             ║
╚═══════════════════════════════════════════════════════════╝`)

	// Connect to Discord, now or whenever it starts, and again after it
	// restarts
	fmt.Println("🔗 Connecting to Discord...")
	discordClient = discord.NewClient(ClientID)
	discordClient.KeepConnected(PollInterval, func(format string, args ...any) {
		fmt.Printf(format+"\n", args...)
	})

	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\n⏹ Shutting down...")
		discordClient.Close()
		os.Exit(0)
	}()

	// Try initial read and show data source
	if session := refreshPresence(); session != nil {
		fmt.Printf("✓ Found active session: %s (using %s)\n", session.ProjectName, currentSource)
	} else {
		fmt.Println("⏳ Waiting for Claude Code session...")
	}

	fmt.Println("🎮 Discord Rich Presence is now active! Press Ctrl+C to stop.")

	// Start watching for changes
	watchForChanges()
}

func readStatusLineData() *SessionData {
	return readPresenceFile(dataFilePath)
}

// readModuleData reads the file the plugin's hooks module writes. The module
// blanks it to "{}" when its session ends, which reads as no data.
func readModuleData() *SessionData {
	return readPresenceFile(moduleFilePath)
}

func readPresenceFile(path string) *SessionData {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}

	var statusLine StatusLineData
	if err := json.Unmarshal(data, &statusLine); err != nil {
		return nil
	}

	if statusLine.SessionID == "" {
		return nil
	}

	projectPath := statusLine.Workspace.ProjectDir
	if projectPath == "" {
		projectPath = statusLine.Cwd
	}

	projectName := filepath.Base(projectPath)
	if projectName == "" || projectName == "." {
		projectName = "Unknown Project"
	}

	// The file records how long the session had run when it was written
	startTime := sessionStartTime
	if ms := statusLine.Cost.TotalDurationMS; ms > 0 {
		startTime = info.ModTime().Add(-time.Duration(ms) * time.Millisecond).Truncate(time.Second)
	}

	return &SessionData{
		ProjectName: projectName,
		ProjectPath: projectPath,
		GitBranch:   getGitBranch(projectPath),
		ModelName:   statusLine.Model.DisplayName,
		// Context size: the last request's input, cached tokens included.
		TotalTokens: statusLine.ContextWindow.TotalInputTokens,
		TotalCost:   statusLine.Cost.TotalCostUSD,
		CostKnown:   true, // Claude Code's own figure
		StartTime:   startTime,

		LastActivity: info.ModTime(),
	}
}

func getGitBranch(projectPath string) string {
	if projectPath == "" {
		return ""
	}

	cmd := exec.Command("git", "-C", projectPath, "rev-parse", "--abbrev-ref", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	branch := strings.TrimSpace(string(output))

	// If HEAD (no commits yet), try to get the branch name from symbolic-ref
	if branch == "HEAD" {
		cmd = exec.Command("git", "-C", projectPath, "symbolic-ref", "--short", "HEAD")
		output, err = cmd.Output()
		if err == nil {
			branch = strings.TrimSpace(string(output))
		}
	}

	return branch
}

// findMostRecentJSONL finds the most recently modified JSONL file in ~/.claude/projects/
func findMostRecentJSONL() (string, string, error) {
	if _, err := os.Stat(projectsDir); os.IsNotExist(err) {
		return "", "", fmt.Errorf("projects directory does not exist")
	}

	type jsonlFile struct {
		path        string
		projectPath string
		modTime     time.Time
	}

	var files []jsonlFile

	err := filepath.WalkDir(projectsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		if d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		// Extract project path from the directory structure
		// ~/.claude/projects/<encoded-path>/<session>.jsonl
		// Encoded path uses dashes: -Users-vasantpns-Developer-project
		relPath, _ := filepath.Rel(projectsDir, path)
		parts := strings.SplitN(relPath, string(filepath.Separator), 2)
		if len(parts) < 1 {
			return nil
		}

		// Skip sessions stop.sh saw end, unless resumed since. The session id
		// is the transcript's name, or the folder of its subagent transcripts.
		if len(parts) == 2 {
			sessionID, _, _ := strings.Cut(parts[1], string(filepath.Separator))
			sessionID = strings.TrimSuffix(sessionID, ".jsonl")
			if ended, err := os.Stat(filepath.Join(endedDir, sessionID)); err == nil && !info.ModTime().After(ended.ModTime()) {
				return nil
			}
		}

		// Decode the project path
		// Claude Code encodes paths: / becomes -, and literal - becomes --
		// Example: /Users/foo/my-project -> -Users-foo-my--project
		// Must decode -- to - FIRST, then decode single - to /
		encodedPath := parts[0]
		// Use a placeholder for double dashes (escaped literal dashes)
		projectPath := strings.ReplaceAll(encodedPath, "--", "\x00")
		// Convert single dashes to path separators
		projectPath = strings.ReplaceAll(projectPath, "-", "/")
		// Restore literal dashes from placeholder
		projectPath = strings.ReplaceAll(projectPath, "\x00", "-")

		files = append(files, jsonlFile{
			path:        path,
			projectPath: projectPath,
			modTime:     info.ModTime(),
		})

		return nil
	})

	if err != nil {
		return "", "", err
	}

	if len(files) == 0 {
		return "", "", fmt.Errorf("no JSONL files found")
	}

	// Sort by modification time, most recent first
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.After(files[j].modTime)
	})

	return files[0].path, files[0].projectPath, nil
}

// parseJSONLSession parses a JSONL file and extracts session data
func parseJSONLSession(jsonlPath, _ string) *SessionData {
	file, err := os.Open(jsonlPath)
	if err != nil {
		return nil
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil
	}

	type response struct {
		model string
		usage Usage
	}
	var (
		responses    []response         // in transcript order, one per message id
		seen         = map[string]int{} // message id -> index in responses
		projectPath  string
		stretchStart time.Time // first message of the current stretch of work
		lastAt       time.Time
	)

	// Lines can be megabytes long (large tool results). A bufio.Scanner stops
	// at its buffer limit, which silently dropped the rest of the file.
	reader := bufio.NewReader(file)
	for {
		line, readErr := reader.ReadBytes('\n')
		if readErr != nil && len(line) == 0 {
			break
		}
		var msg JSONLMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}

		// Extract cwd from any message that has it (usually first message)
		if msg.Cwd != "" && projectPath == "" {
			projectPath = msg.Cwd
		}

		// The timer counts from the start of the current stretch of work: a
		// gap longer than IdleTimeout (a session resumed later) starts anew.
		if at, err := time.Parse(time.RFC3339Nano, msg.Timestamp); err == nil {
			if stretchStart.IsZero() || at.Sub(lastAt) > IdleTimeout {
				stretchStart = at
			}
			lastAt = at
		}

		// Only process assistant messages with usage data. Claude Code
		// records its own notices (API errors, interruptions) as assistant
		// messages from model "<synthetic>".
		if msg.Type != "assistant" || msg.Message.Model == "" || strings.HasPrefix(msg.Message.Model, "<") {
			continue
		}
		r := response{msg.Message.Model, msg.Message.Usage}
		// Claude Code writes one line per content block of a response, each
		// repeating its usage: count each message id once.
		if i, ok := seen[msg.Message.ID]; ok && msg.Message.ID != "" {
			responses[i] = r
			continue
		}
		if msg.Message.ID != "" {
			seen[msg.Message.ID] = len(responses)
		}
		responses = append(responses, r)
	}

	if len(responses) == 0 {
		return nil
	}

	// Price each response by its own model; the model can change mid-session
	totalCost, costKnown := 0.0, true
	for _, r := range responses {
		cost, ok := messageCost(r.model, r.usage)
		totalCost += cost
		costKnown = costKnown && ok
	}

	last := responses[len(responses)-1]
	// Context size: the latest request's input, cached tokens included
	contextTokens := last.usage.InputTokens + last.usage.CacheCreationInputTokens + last.usage.CacheReadInputTokens
	modelName := formatModelName(last.model)

	projectName := filepath.Base(projectPath)
	if projectName == "" || projectName == "." {
		projectName = "Unknown Project"
	}

	// Transcripts without timestamps fall back to the daemon's start
	startTime := sessionStartTime
	if !stretchStart.IsZero() {
		startTime = stretchStart
	}

	return &SessionData{
		ProjectName: projectName,
		ProjectPath: projectPath,
		GitBranch:   getGitBranch(projectPath),
		ModelName:   modelName,
		TotalTokens: contextTokens,
		TotalCost:   totalCost,
		CostKnown:   costKnown,
		StartTime:   startTime,

		LastActivity: info.ModTime(),
	}
}

// formatModelName converts a model ID to its display name, "Opus 4.5"
func formatModelName(modelID string) string {
	family, major, minor, ok := parseModelID(modelID)
	if !ok {
		return "Claude"
	}
	return displayName(family, major, minor)
}

// readSessionData tries statusline data first, then falls back to JSONL parsing
func readSessionData() *SessionData {
	// The hooks module first: it runs in every host that loads it, the
	// desktop app included. Then the statusline wrapper, for Claude Code
	// versions without hooks modules. Then JSONL parsing as a last resort.
	if data := readModuleData(); data != nil {
		setSource(sourceModule)
		return data
	}
	if data := readStatusLineData(); data != nil {
		setSource(sourceStatusLine)
		return data
	}

	jsonlPath, projectPath, err := findMostRecentJSONL()
	if err != nil {
		return nil
	}

	setSource(sourceJSONL)
	if !nudgeShown {
		nudgeShown = true
		fmt.Println("\n💡 Tip: For accurate token/cost data, update Claude Code (the plugin's hooks module")
		fmt.Println("   needs a recent version) or configure the statusline wrapper.")
		fmt.Println("   See: https://github.com/tsanva/cc-discord-presence#statusline-setup")
	}

	return parseJSONLSession(jsonlPath, projectPath)
}

// setSource logs when the data source changes after startup.
func setSource(source string) {
	if currentSource != "" && currentSource != source {
		fmt.Printf("📊 Now using %s\n", source)
	}
	currentSource = source
}

// presenceText builds the two activity lines Discord shows.
func presenceText(session *SessionData) (details, state string) {
	verb := "Working on"
	if session.Idle {
		verb = "Idling in"
	}
	details = fmt.Sprintf("%s: %s", verb, session.ProjectName)
	if session.GitBranch != "" {
		details = fmt.Sprintf("%s: %s (%s)", verb, session.ProjectName, session.GitBranch)
	}

	// model | context size | cost (left out rather than guessed)
	state = fmt.Sprintf("%s | %s tokens", session.ModelName, formatNumber(session.TotalTokens))
	if session.CostKnown {
		state += fmt.Sprintf(" | $%.4f", session.TotalCost)
	}
	return details, state
}

// refreshPresence shows the current session on Discord, or empties the card
// when there is none. It returns the session shown.
func refreshPresence() *SessionData {
	session := readSessionData()
	if session == nil {
		clearPresence()
		return nil
	}
	if sessionIsIdle(session, time.Now()) {
		// While Claude Code is open in some terminal the card stays, marked
		// idle; otherwise the session crashed or closed without its hook.
		if len(claudeCodeProcesses()) == 0 {
			clearPresence()
			return nil
		}
		session.Idle = true
	}
	updatePresence(session)
	return session
}

// sessionIsIdle decides when nobody is working in the session any more:
// left open but unused, crashed, or closed without its SessionEnd hook.
func sessionIsIdle(session *SessionData, now time.Time) bool {
	if session.LastActivity.IsZero() {
		return false // unknown: don't hide an otherwise valid session
	}
	return now.Sub(session.LastActivity) > IdleTimeout
}

var (
	claudeCodeProcesses = listClaudeCode
	sawClaudeCode       bool
	lastSawClaudeCode   time.Time
)

// claudeCodeGone reports when Claude Code has been closed everywhere for
// ExitGrace. Where no Claude Code process was ever found (named differently
// than expected), it never reports gone; the SessionEnd hook stops the
// daemon there.
func claudeCodeGone(now time.Time) bool {
	if len(claudeCodeProcesses()) > 0 {
		sawClaudeCode, lastSawClaudeCode = true, now
		return false
	}
	return sawClaudeCode && now.Sub(lastSawClaudeCode) > ExitGrace
}

// exitIfClaudeCodeGone stops the daemon once Claude Code is closed
// everywhere, emptying the card first.
func exitIfClaudeCodeGone() {
	if !claudeCodeGone(time.Now()) {
		return
	}
	fmt.Println("⏹ Claude Code is no longer running, exiting")
	clearPresence()
	discordClient.Close()
	os.Exit(0)
}

func clearPresence() {
	if lastPresence == "" {
		return
	}
	if err := discordClient.ClearActivity(); err != nil && !errors.Is(err, discord.ErrNotConnected) {
		fmt.Fprintf(os.Stderr, "Error clearing presence: %v\n", err)
		return
	}
	// Cleared, or nothing to restore once Discord is back
	lastPresence = ""
}

func updatePresence(session *SessionData) {
	details, state := presenceText(session)

	// The data files change often (the hooks module writes after every tool
	// call) and the poll runs every few seconds; only send what Discord
	// would show differently.
	key := details + "\n" + state + "\n" + session.StartTime.String()
	if key == lastPresence {
		return
	}

	if err := discordClient.SetActivity(discord.Activity{
		Details:   details,
		State:     state,
		LargeText: "Clawd Code - Discord Rich Presence for Claude Code",
		StartTime: &session.StartTime,
	}); err != nil && !errors.Is(err, discord.ErrNotConnected) {
		fmt.Fprintf(os.Stderr, "Error updating presence: %v\n", err)
		return
	}
	// Sent, or kept by the client until Discord is back
	lastPresence = key
}

func formatNumber(n int64) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	} else if n >= 1_000 {
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	}
	return fmt.Sprintf("%d", n)
}

func watchForChanges() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		fmt.Println("Using polling mode for session tracking")
		pollForChanges()
		return
	}
	defer watcher.Close()

	// Watch the main claude dir (module and statusline data files)
	if err := watcher.Add(claudeDir); err != nil {
		fmt.Println("Using polling mode for session tracking")
		pollForChanges()
		return
	}

	// Also poll as backup (especially important for JSONL which is in subdirs)
	ticker := time.NewTicker(PollInterval)
	defer ticker.Stop()

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			// Respond to module and statusline data file changes
			if name := filepath.Base(event.Name); name == filepath.Base(moduleFilePath) || name == filepath.Base(dataFilePath) {
				refreshPresence()
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			fmt.Fprintf(os.Stderr, "Watcher error: %v\n", err)
		case <-ticker.C:
			exitIfClaudeCodeGone()
			// Poll reads from either statusline or JSONL fallback
			refreshPresence()
		}
	}
}

func pollForChanges() {
	ticker := time.NewTicker(PollInterval)
	defer ticker.Stop()

	for range ticker.C {
		exitIfClaudeCodeGone()
		refreshPresence()
	}
}
