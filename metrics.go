package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type metricsInput struct {
	ToolName  string `json:"tool_name"`
	ToolInput struct {
		Command string `json:"command"`
	} `json:"tool_input"`
	ToolResponse struct {
		Stdout   string `json:"stdout"`
		Stderr   string `json:"stderr"`
		Duration int    `json:"duration"`
		ExitCode int    `json:"exitCode"`
	} `json:"tool_response"`
	SessionID string `json:"session_id"`
}

type metricsEntry struct {
	Timestamp   string `json:"ts"`
	Session     string `json:"session"`
	Command     string `json:"command"`
	CmdPattern  string `json:"cmd_pattern"`
	StdoutBytes int    `json:"stdout_bytes"`
	StderrBytes int    `json:"stderr_bytes"`
	TotalBytes  int    `json:"total_bytes"`
	DurationMs  int    `json:"duration_ms"`
	ExitCode    int    `json:"exit_code"`
}

func runMetrics() {
	var input metricsInput
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		return
	}

	if input.ToolName != "Bash" {
		return
	}

	cmd := input.ToolInput.Command
	exitCode := input.ToolResponse.ExitCode

	// Strip filter pipeline suffix if command was rewritten by PreToolUse hook
	// Rewritten format: ( original_cmd ) 2>&1 | tee LOG | trimout filter ...
	if strings.Contains(cmd, " 2>&1 | tee ") && strings.Contains(cmd, " filter --log ") {
		parts := strings.SplitN(cmd, " 2>&1 | tee ", 2)
		// Read exit code from sidecar file written by the rewritten command
		if len(parts) == 2 {
			logPath := strings.SplitN(parts[1], " | ", 2)[0]
			exitCode = readExitCode(logPath+".exit", exitCode)
		}
		cmd = parts[0]
		// Strip subshell wrapper: "( cmd )" → "cmd"
		if strings.HasPrefix(cmd, "( ") && strings.HasSuffix(cmd, " )") {
			cmd = cmd[2 : len(cmd)-2]
		}
	}

	// Truncate long commands
	if len(cmd) > 200 {
		cmd = cmd[:200]
	}

	entry := metricsEntry{
		Timestamp:   time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		Session:     input.SessionID,
		Command:     cmd,
		CmdPattern:  classifyCommand(cmd),
		StdoutBytes: len(input.ToolResponse.Stdout),
		StderrBytes: len(input.ToolResponse.Stderr),
		TotalBytes:  len(input.ToolResponse.Stdout) + len(input.ToolResponse.Stderr),
		DurationMs:  input.ToolResponse.Duration,
		ExitCode:    exitCode,
	}

	metricsPath := filepath.Join(MetricsDir(), "tool-output.jsonl")
	os.MkdirAll(filepath.Dir(metricsPath), 0o755)

	data, err := json.Marshal(entry)
	if err != nil {
		return
	}

	f, err := os.OpenFile(metricsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	f.Write(append(data, '\n'))
}

// readExitCode reads an integer from a sidecar file and removes it.
// Returns fallback if the file is missing or unparseable.
func readExitCode(path string, fallback int) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return fallback
	}
	os.Remove(path)
	var code int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &code); err != nil {
		return fallback
	}
	return code
}

// classifyCommand extracts a pattern for aggregation (e.g. "dotnet build").
func classifyCommand(cmd string) string {
	tokens := strings.Fields(strings.TrimSpace(cmd))
	if len(tokens) == 0 {
		return "empty"
	}

	first := tokens[0]

	// Handle cd && chained commands
	if first == "cd" && strings.Contains(cmd, "&&") {
		parts := strings.SplitN(cmd, "&&", 2)
		if len(parts) == 2 {
			after := strings.Fields(strings.TrimSpace(parts[1]))
			if len(after) > 0 {
				first = after[0]
				tokens = after
			}
		}
	}

	// Handle common prefixes
	prefixes := map[string]bool{"sudo": true, "env": true, "time": true, "nice": true}
	if prefixes[first] && len(tokens) > 1 {
		tokens = tokens[1:]
		first = tokens[0]
	}

	second := ""
	if len(tokens) > 1 {
		second = tokens[1]
	}
	if strings.HasPrefix(second, "-") {
		second = ""
	}

	multiWord := map[string]bool{
		"dotnet": true, "docker": true, "git": true, "npm": true, "npx": true,
		"yarn": true, "pnpm": true, "cargo": true, "go": true, "az": true,
		"gh": true, "kubectl": true, "terraform": true, "make": true,
		"pip": true, "poetry": true, "uv": true, "pytest": true,
		"python3": true, "python": true,
	}

	if multiWord[first] {
		return strings.TrimSpace(first + " " + second)
	}
	return first
}
