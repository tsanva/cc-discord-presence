package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
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
)

// Model pricing per million tokens (December 2025)
// Update these when new models are released: https://www.anthropic.com/pricing
var modelPricing = map[string]struct{ Input, Output float64 }{
	"claude-opus-4-5-20251101":   {15.0, 75.0},
	"claude-sonnet-4-5-20241022": {3.0, 15.0},
	"claude-sonnet-4-20250514":   {3.0, 15.0},
	"claude-haiku-4-5-20241022":  {1.0, 5.0},
}

// Model display names - add new model IDs here when released
var modelDisplayNames = map[string]string{
	"claude-opus-4-5-20251101":   "Opus 4.5",
	"claude-sonnet-4-5-20241022": "Sonnet 4.5",
	"claude-sonnet-4-20250514":   "Sonnet 4",
	"claude-haiku-4-5-20241022":  "Haiku 4.5",
}

// StatusLineData matches Claude Code's statusline JSON structure
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
	StartTime   time.Time
}

// JSONLMessage represents a message entry in JSONL files
type JSONLMessage struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Cwd       string `json:"cwd"`
	Message   struct {
		Model string `json:"model"`
		Usage struct {
			InputTokens  int64 `json:"input_tokens"`
			OutputTokens int64 `json:"output_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

var (
	claudeDir        string
	projectsDir      string
	dataFilePath     string
	sessionStartTime = time.Now()
	discordClient    *discord.Client
	usingFallback    bool
	nudgeShown       bool
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
}

func main() {
	fmt.Println(`
╔═══════════════════════════════════════════════════════════╗
║     Clawd Code - Discord Rich Presence                    ║
║     Show your Claude Code session on Discord!             ║
╚═══════════════════════════════════════════════════════════╝`)

	// Connect to Discord
	fmt.Println("🔗 Connecting to Discord...")
	discordClient = discord.NewClient(ClientID)
	if err := discordClient.Connect(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to connect to Discord: %v\n", err)
		fmt.Fprintln(os.Stderr, "   Make sure Discord is running and try again.")
		os.Exit(1)
	}
	fmt.Println("✓ Discord RPC connected!")

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
	if session := readSessionData(); session != nil {
		updatePresence(session)
		if usingFallback {
			fmt.Printf("✓ Found active session: %s (using JSONL fallback)\n", session.ProjectName)
		} else {
			fmt.Printf("✓ Found active session: %s (using statusline data)\n", session.ProjectName)
		}
	} else {
		fmt.Println("⏳ Waiting for Claude Code session...")
	}

	fmt.Println("🎮 Discord Rich Presence is now active! Press Ctrl+C to stop.")

	// Start watching for changes
	watchForChanges()
}

func readStatusLineData() *SessionData {
	data, err := os.ReadFile(dataFilePath)
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

	return &SessionData{
		ProjectName: projectName,
		ProjectPath: projectPath,
		GitBranch:   getGitBranch(projectPath),
		ModelName:   statusLine.Model.DisplayName,
		TotalTokens: statusLine.ContextWindow.TotalInputTokens + statusLine.ContextWindow.TotalOutputTokens,
		TotalCost:   statusLine.Cost.TotalCostUSD,
		StartTime:   sessionStartTime,
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

	var (
		totalInputTokens  int64
		totalOutputTokens int64
		lastModel         string
		projectPath       string
	)

	scanner := bufio.NewScanner(file)
	// Increase buffer size for large lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		var msg JSONLMessage
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue
		}

		// Extract cwd from any message that has it (usually first message)
		if msg.Cwd != "" && projectPath == "" {
			projectPath = msg.Cwd
		}

		// Only process assistant messages with usage data
		if msg.Type == "assistant" && msg.Message.Model != "" {
			lastModel = msg.Message.Model
			totalInputTokens += msg.Message.Usage.InputTokens
			totalOutputTokens += msg.Message.Usage.OutputTokens
		}
	}

	if lastModel == "" {
		return nil
	}

	// Calculate cost based on model pricing
	totalCost := calculateCost(lastModel, totalInputTokens, totalOutputTokens)

	// Get display name for model
	modelName := formatModelName(lastModel)

	projectName := filepath.Base(projectPath)
	if projectName == "" || projectName == "." {
		projectName = "Unknown Project"
	}

	// Use daemon start time for elapsed time display
	// This shows how long Discord presence has been active, not total session time
	return &SessionData{
		ProjectName: projectName,
		ProjectPath: projectPath,
		GitBranch:   getGitBranch(projectPath),
		ModelName:   modelName,
		TotalTokens: totalInputTokens + totalOutputTokens,
		TotalCost:   totalCost,
		StartTime:   sessionStartTime,
	}
}

// calculateCost calculates the cost based on token usage and model pricing
func calculateCost(modelID string, inputTokens, outputTokens int64) float64 {
	pricing, ok := modelPricing[modelID]
	if !ok {
		// Default to Sonnet 4 pricing if unknown model
		pricing = modelPricing["claude-sonnet-4-20250514"]
	}

	inputCost := float64(inputTokens) / 1_000_000 * pricing.Input
	outputCost := float64(outputTokens) / 1_000_000 * pricing.Output

	return inputCost + outputCost
}

// formatModelName converts model ID to display name
func formatModelName(modelID string) string {
	if name, ok := modelDisplayNames[modelID]; ok {
		return name
	}

	// Try to extract a reasonable name from the model ID
	if strings.Contains(modelID, "opus") {
		return "Opus"
	}
	if strings.Contains(modelID, "sonnet") {
		return "Sonnet"
	}
	if strings.Contains(modelID, "haiku") {
		return "Haiku"
	}

	return "Claude"
}

// readSessionData tries statusline data first, then falls back to JSONL parsing
func readSessionData() *SessionData {
	// First try statusline data (most accurate)
	if data := readStatusLineData(); data != nil {
		if usingFallback {
			usingFallback = false
			fmt.Println("📊 Now using statusline data (more accurate)")
		}
		return data
	}

	// Fall back to JSONL parsing
	jsonlPath, projectPath, err := findMostRecentJSONL()
	if err != nil {
		return nil
	}

	if !usingFallback && !nudgeShown {
		usingFallback = true
		nudgeShown = true
		fmt.Println("\n💡 Tip: For more accurate token/cost data, configure the statusline wrapper.")
		fmt.Println("   See: https://github.com/tsanva/cc-discord-presence#statusline-setup")
	}

	return parseJSONLSession(jsonlPath, projectPath)
}

func updatePresence(session *SessionData) {
	// Build details line with prefix
	details := fmt.Sprintf("Working on: %s", session.ProjectName)
	if session.GitBranch != "" {
		details = fmt.Sprintf("Working on: %s (%s)", session.ProjectName, session.GitBranch)
	}

	// Build state line: model | tokens | cost
	state := fmt.Sprintf("%s | %s tokens | $%.4f",
		session.ModelName,
		formatNumber(session.TotalTokens),
		session.TotalCost)

	if err := discordClient.SetActivity(discord.Activity{
		Details:   details,
		State:     state,
		LargeText: "Clawd Code - Discord Rich Presence for Claude Code",
		StartTime: &session.StartTime,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "Error updating presence: %v\n", err)
	}
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

	// Watch both the main claude dir (for statusline data) and projects dir (for JSONL fallback)
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
			// Respond to statusline data file changes
			if filepath.Base(event.Name) == "discord-presence-data.json" {
				if session := readSessionData(); session != nil {
					updatePresence(session)
				}
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			fmt.Fprintf(os.Stderr, "Watcher error: %v\n", err)
		case <-ticker.C:
			// Poll reads from either statusline or JSONL fallback
			if session := readSessionData(); session != nil {
				updatePresence(session)
			}
		}
	}
}

func pollForChanges() {
	ticker := time.NewTicker(PollInterval)
	defer ticker.Stop()

	for range ticker.C {
		if session := readSessionData(); session != nil {
			updatePresence(session)
		}
	}
}
