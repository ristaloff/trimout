package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var nofilterRe = regexp.MustCompile(`#\s*nofilter`)

// hookInput is the JSON structure received from Claude Code PreToolUse hooks.
type hookInput struct {
	ToolName  string `json:"tool_name"`
	ToolInput struct {
		Command string `json:"command"`
	} `json:"tool_input"`
	SessionID string `json:"session_id"`
}

// hookOutput is the JSON structure returned to Claude Code.
type hookOutput struct {
	HookSpecificOutput struct {
		HookEventName      string `json:"hookEventName"`
		PermissionDecision string `json:"permissionDecision"`
		UpdatedInput       struct {
			Command string `json:"command"`
		} `json:"updatedInput"`
	} `json:"hookSpecificOutput"`
}

func runHook() {
	var input hookInput
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		// No output = no rewrite. Log to stderr for debugging.
		fmt.Fprintf(os.Stderr, "[trimout hook: %v]\n", err)
		return
	}

	cmd := input.ToolInput.Command
	if cmd == "" {
		return
	}

	// Opt-out: # nofilter
	if nofilterRe.MatchString(cmd) {
		return
	}

	// Check allowlist
	if !matchesAllowlist(cmd) {
		return
	}

	// Build log path and prune old logs
	logDir := LogDir()
	os.MkdirAll(logDir, 0o755)
	pruneOldLogs(logDir)

	logFile := buildLogFile(logDir)
	self := selfPath()

	// Rewrite command using shared builder
	rewritten := buildRewrittenCommand(cmd, logFile, self, input.SessionID)

	// Build output
	var out hookOutput
	out.HookSpecificOutput.HookEventName = "PreToolUse"
	out.HookSpecificOutput.PermissionDecision = "allow"
	out.HookSpecificOutput.UpdatedInput.Command = rewritten

	json.NewEncoder(os.Stdout).Encode(out)
}

// pruneOldLogs removes .log files older than LogRetentionD days.
func pruneOldLogs(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-time.Duration(LogRetentionD) * 24 * time.Hour)
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !(strings.HasSuffix(name, ".log") || strings.HasSuffix(name, ".log.exit")) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}
