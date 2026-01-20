package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestCalculateCost tests the cost calculation logic
func TestCalculateCost(t *testing.T) {
	tests := []struct {
		name         string
		modelID      string
		inputTokens  int64
		outputTokens int64
		wantCost     float64
	}{
		{
			name:         "Opus 4.5 - basic usage",
			modelID:      "claude-opus-4-5-20251101",
			inputTokens:  1_000_000,
			outputTokens: 100_000,
			wantCost:     15.0 + 7.5, // $15/M input + $75/M * 0.1 output
		},
		{
			name:         "Sonnet 4.5 - basic usage",
			modelID:      "claude-sonnet-4-5-20241022",
			inputTokens:  1_000_000,
			outputTokens: 100_000,
			wantCost:     3.0 + 1.5, // $3/M input + $15/M * 0.1 output
		},
		{
			name:         "Sonnet 4 - basic usage",
			modelID:      "claude-sonnet-4-20250514",
			inputTokens:  500_000,
			outputTokens: 50_000,
			wantCost:     1.5 + 0.75, // $3/M * 0.5 input + $15/M * 0.05 output
		},
		{
			name:         "Haiku 4.5 - basic usage",
			modelID:      "claude-haiku-4-5-20241022",
			inputTokens:  2_000_000,
			outputTokens: 200_000,
			wantCost:     2.0 + 1.0, // $1/M * 2 input + $5/M * 0.2 output
		},
		{
			name:         "Unknown model - defaults to Sonnet 4",
			modelID:      "claude-unknown-model-20991231",
			inputTokens:  1_000_000,
			outputTokens: 1_000_000,
			wantCost:     3.0 + 15.0, // Sonnet 4 pricing as fallback
		},
		{
			name:         "Zero tokens",
			modelID:      "claude-opus-4-5-20251101",
			inputTokens:  0,
			outputTokens: 0,
			wantCost:     0,
		},
		{
			name:         "Small token count",
			modelID:      "claude-sonnet-4-20250514",
			inputTokens:  1000,
			outputTokens: 500,
			wantCost:     0.003 + 0.0075, // $3/M * 0.001 + $15/M * 0.0005
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateCost(tt.modelID, tt.inputTokens, tt.outputTokens)
			// Use approximate comparison for floating point
			if diff := got - tt.wantCost; diff > 0.0001 || diff < -0.0001 {
				t.Errorf("calculateCost(%q, %d, %d) = %v, want %v", tt.modelID, tt.inputTokens, tt.outputTokens, got, tt.wantCost)
			}
		})
	}
}

// TestFormatModelName tests the model name formatting logic
func TestFormatModelName(t *testing.T) {
	tests := []struct {
		name    string
		modelID string
		want    string
	}{
		{
			name:    "Known model - Opus 4.5",
			modelID: "claude-opus-4-5-20251101",
			want:    "Opus 4.5",
		},
		{
			name:    "Known model - Sonnet 4.5",
			modelID: "claude-sonnet-4-5-20241022",
			want:    "Sonnet 4.5",
		},
		{
			name:    "Known model - Sonnet 4",
			modelID: "claude-sonnet-4-20250514",
			want:    "Sonnet 4",
		},
		{
			name:    "Known model - Haiku 4.5",
			modelID: "claude-haiku-4-5-20241022",
			want:    "Haiku 4.5",
		},
		{
			name:    "Unknown opus model - fallback",
			modelID: "claude-opus-5-20260101",
			want:    "Opus",
		},
		{
			name:    "Unknown sonnet model - fallback",
			modelID: "claude-sonnet-5-20260101",
			want:    "Sonnet",
		},
		{
			name:    "Unknown haiku model - fallback",
			modelID: "claude-haiku-5-20260101",
			want:    "Haiku",
		},
		{
			name:    "Completely unknown model - defaults to Claude",
			modelID: "some-unknown-model",
			want:    "Claude",
		},
		{
			name:    "Empty model ID",
			modelID: "",
			want:    "Claude",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatModelName(tt.modelID); got != tt.want {
				t.Errorf("formatModelName(%q) = %q, want %q", tt.modelID, got, tt.want)
			}
		})
	}
}

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
			wantTokens:  4500, // 1000+500+2000+1000
			wantModel:   "Sonnet 4",
			wantProject: "project",
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
			wantTokens:  600,
			wantModel:   "Haiku 4.5",
			wantProject: "myapp",
		},
		{
			name: "Multiple models uses last one",
			content: `{"type":"user","cwd":"/Users/test/multimodel"}
{"type":"assistant","message":{"model":"claude-haiku-4-5-20241022","usage":{"input_tokens":100,"output_tokens":50}}}
{"type":"assistant","message":{"model":"claude-opus-4-5-20251101","usage":{"input_tokens":200,"output_tokens":100}}}`,
			wantNil:     false,
			wantTokens:  450,
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

// TestModelPricingConsistency ensures model pricing and display names are in sync
func TestModelPricingConsistency(t *testing.T) {
	// All models in pricing should have display names
	for modelID := range modelPricing {
		if _, ok := modelDisplayNames[modelID]; !ok {
			t.Errorf("Model %q has pricing but no display name", modelID)
		}
	}

	// All models in display names should have pricing
	for modelID := range modelDisplayNames {
		if _, ok := modelPricing[modelID]; !ok {
			t.Errorf("Model %q has display name but no pricing", modelID)
		}
	}
}
