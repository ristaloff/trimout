package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// runInstall handles "trimout install <agent> [--check]"
func runInstall(args []string) {
	if len(args) == 0 {
		printInstallHelp()
		os.Exit(2)
	}

	checkOnly := false
	var target string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--check":
			checkOnly = true
		case "claude-code":
			target = "claude-code"
		default:
			fmt.Fprintf(os.Stderr, "Unknown argument: %s\n", args[i])
			printInstallHelp()
			os.Exit(2)
		}
	}

	if target == "" {
		fmt.Fprintln(os.Stderr, "No agent specified. Currently supported: claude-code")
		printInstallHelp()
		os.Exit(2)
	}

	switch target {
	case "claude-code":
		if checkOnly {
			runInstallCheckClaudeCode()
		} else {
			runInstallClaudeCode()
		}
	}
}

// runInstallClaudeCode checks current state and either confirms hooks are
// installed or prints the JSON snippet to add. Does not modify settings.json
// directly — the file is shared with other tools and rewriting it would
// destroy formatting, key order, and escape sequences.
func runInstallClaudeCode() {
	settingsPath := expandHome("~/.claude/settings.json")
	self := selfPath()

	hookCmd := buildHookCommand(self, "hook")
	metricsCmd := buildHookCommand(self, "metrics")

	// Check current state
	settings, err := readJSONFile(settingsPath)
	if err == nil {
		hooks, _ := settings["hooks"].(map[string]interface{})
		if hooks != nil {
			preOK := hasTrimoutHook(hooks, "PreToolUse")
			postOK := hasTrimoutHook(hooks, "PostToolUse")

			if preOK && postOK {
				// Check if paths need updating
				prePath := trimoutHookBinary(hooks, "PreToolUse")
				postPath := trimoutHookBinary(hooks, "PostToolUse")
				if prePath == self && postPath == self {
					fmt.Fprintln(os.Stderr, "Already installed — no changes needed.")
					return
				}
				// Paths differ — show what to update
				fmt.Fprintln(os.Stderr, "Hooks exist but binary path needs updating.")
				fmt.Fprintln(os.Stderr, "")
				fmt.Fprintf(os.Stderr, "Replace the trimout commands in %s with:\n", settingsPath)
				fmt.Fprintln(os.Stderr, "")
				fmt.Fprintf(os.Stderr, "  PreToolUse:  \"%s\"\n", hookCmd)
				fmt.Fprintf(os.Stderr, "  PostToolUse: \"%s\"\n", metricsCmd)
				return
			}
		}
	}

	// Check if file exists at all
	_, fileErr := os.Stat(settingsPath)
	fileExists := fileErr == nil

	if !fileExists {
		// No settings file — output a complete, ready-to-save file
		fmt.Fprintf(os.Stderr, "No settings file found. Save the following to %s:\n\n", settingsPath)
		fmt.Fprintf(os.Stdout, `{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "%s"
          }
        ]
      }
    ],
    "PostToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "%s"
          }
        ]
      }
    ]
  }
}
`, hookCmd, metricsCmd)
	} else {
		// File exists but no trimout hooks — print the hooks fragment
		fmt.Fprintf(os.Stderr, "Add the following to the \"hooks\" object in %s:\n\n", settingsPath)
		fmt.Fprintf(os.Stdout, `"PreToolUse": [
  {
    "matcher": "Bash",
    "hooks": [
      {
        "type": "command",
        "command": "%s"
      }
    ]
  }
],
"PostToolUse": [
  {
    "matcher": "Bash",
    "hooks": [
      {
        "type": "command",
        "command": "%s"
      }
    ]
  }
]
`, hookCmd, metricsCmd)
	}

	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Restart Claude Code for hooks to take effect.")
}

// runInstallCheckClaudeCode verifies trimout is correctly installed for Claude Code
func runInstallCheckClaudeCode() {
	passed := 0
	failed := 0

	// 1. Binary on PATH
	which, err := exec.LookPath("trimout")
	if err != nil {
		fmt.Fprintln(os.Stderr, "FAIL  Binary not on PATH")
		failed++
	} else {
		fmt.Fprintf(os.Stderr, "OK    Binary: %s\n", which)
		passed++
	}

	// 2. Global settings.json has hooks
	settingsPath := expandHome("~/.claude/settings.json")
	settings, err := readJSONFile(settingsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL  Cannot read %s: %v\n", settingsPath, err)
		failed++
	} else {
		hooks, _ := settings["hooks"].(map[string]interface{})
		if hooks == nil {
			fmt.Fprintf(os.Stderr, "FAIL  No hooks in %s\n", settingsPath)
			failed++
		} else {
			for _, event := range []string{"PreToolUse", "PostToolUse"} {
				if path := trimoutHookBinary(hooks, event); path == "" {
					fmt.Fprintf(os.Stderr, "FAIL  %s hook missing\n", event)
					failed++
				} else if _, err := os.Stat(path); err != nil {
					fmt.Fprintf(os.Stderr, "FAIL  %s hook binary not found: %s\n", event, path)
					fmt.Fprintln(os.Stderr, "      Run 'trimout install claude-code' to fix.")
					failed++
				} else {
					fmt.Fprintf(os.Stderr, "OK    %s hook configured (%s)\n", event, path)
					passed++
				}
			}
		}
	}

	// 3. Data dir
	dataDir := DataDir()
	fmt.Fprintf(os.Stderr, "OK    Data dir: %s (ephemeral)\n", dataDir)
	passed++

	// 4. Scan for broken project-level settings
	brokenProjects := scanBrokenProjectSettings()
	if len(brokenProjects) > 0 {
		for _, p := range brokenProjects {
			fmt.Fprintf(os.Stderr, "FAIL  Broken project settings: %s (%s)\n", p.path, p.reason)
		}
		failed += len(brokenProjects)
	} else {
		fmt.Fprintln(os.Stderr, "OK    No broken project settings found")
		passed++
	}

	// 5. Warn if trimout hooks found in project-level settings
	misplaced := scanMisplacedHooks()
	for _, p := range misplaced {
		fmt.Fprintf(os.Stderr, "WARN  Trimout hooks in project settings: %s\n", p)
		fmt.Fprintln(os.Stderr, "      Hooks should be in ~/.claude/settings.json (global) only.")
	}

	// Summary
	fmt.Fprintf(os.Stderr, "\n%d passed, %d failed\n", passed, failed)
	if failed > 0 {
		fmt.Fprintln(os.Stderr, "Run 'trimout install claude-code' to fix hook configuration.")
		os.Exit(1)
	}
}

type brokenSettings struct {
	path   string
	reason string
}

// walkProjectSettingsFiles walks common git roots and calls fn for each
// .claude/settings.json or .claude/settings.local.json found.
func walkProjectSettingsFiles(fn func(path string)) {
	home, _ := os.UserHomeDir()
	roots := []string{
		filepath.Join(home, "git"),
		filepath.Join(home, "projects"),
		filepath.Join(home, "src"),
		filepath.Join(home, "repos"),
		filepath.Join(home, "code"),
	}

	skipDirs := map[string]bool{
		"node_modules": true, ".git": true, "vendor": true, "bin": true, "obj": true,
	}

	for _, root := range roots {
		if _, err := os.Stat(root); err != nil {
			continue
		}
		_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			rel, _ := filepath.Rel(root, path)
			if strings.Count(rel, string(filepath.Separator)) > 4 {
				return filepath.SkipDir
			}
			if info.IsDir() && skipDirs[info.Name()] {
				return filepath.SkipDir
			}
			if !info.IsDir() && strings.Contains(path, ".claude/") &&
				(info.Name() == "settings.json" || info.Name() == "settings.local.json") {
				fn(path)
			}
			return nil
		})
	}
}

// scanBrokenProjectSettings finds .claude/settings.json files that are
// empty or contain invalid JSON, which can silently break settings merging.
func scanBrokenProjectSettings() []brokenSettings {
	var broken []brokenSettings
	walkProjectSettingsFiles(func(path string) {
		if reason := validateSettingsFile(path); reason != "" {
			broken = append(broken, brokenSettings{path: path, reason: reason})
		}
	})
	return broken
}

// scanMisplacedHooks finds project-level settings files that contain trimout
// hooks. Trimout hooks belong in ~/.claude/settings.json (global) — project-level
// hooks would require every collaborator to have trimout installed.
func scanMisplacedHooks() []string {
	var misplaced []string
	walkProjectSettingsFiles(func(path string) {
		settings, err := readJSONFile(path)
		if err != nil {
			return
		}
		hooks, _ := settings["hooks"].(map[string]interface{})
		if hooks != nil && (hasTrimoutHook(hooks, "PreToolUse") || hasTrimoutHook(hooks, "PostToolUse")) {
			misplaced = append(misplaced, path)
		}
	})
	return misplaced
}

// validateSettingsFile checks if a .claude/settings.json is valid
func validateSettingsFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return "unreadable"
	}
	if len(data) == 0 {
		return "empty file (0 bytes) — may override global hooks"
	}
	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		return fmt.Sprintf("invalid JSON: %v", err)
	}
	return ""
}

// trimoutHookBinary returns the binary path from a trimout hook command,
// or empty string if no trimout hook is configured for the event.
// Handles commands like "/path/to/trimout hook" and "ENVVAR=val /path/to/trimout hook".
func trimoutHookBinary(hooks map[string]interface{}, event string) string {
	entries, ok := hooks[event].([]interface{})
	if !ok {
		return ""
	}
	for _, entry := range entries {
		m, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}
		innerHooks, ok := m["hooks"].([]interface{})
		if !ok {
			continue
		}
		for _, h := range innerHooks {
			hm, ok := h.(map[string]interface{})
			if !ok {
				continue
			}
			cmd, _ := hm["command"].(string)
			if !strings.Contains(cmd, "trimout") {
				continue
			}
			// Extract binary path: split on spaces, find the token containing "trimout"
			for _, token := range strings.Fields(cmd) {
				if strings.Contains(token, "trimout") && !strings.Contains(token, "=") {
					// Expand $HOME if present
					token = os.Expand(token, func(key string) string {
						if key == "HOME" {
							home, _ := os.UserHomeDir()
							return home
						}
						return os.Getenv(key)
					})
					return token
				}
			}
		}
	}
	return ""
}

// hasTrimoutHook checks if a hook event already has a trimout command
func hasTrimoutHook(hooks map[string]interface{}, event string) bool {
	return trimoutHookBinary(hooks, event) != ""
}

// buildHookCommand constructs the hook command string using the absolute
// binary path. Always uses the full path — hooks run in Claude Code's
// shell environment which may not have the same PATH as the user's
// interactive shell.
func buildHookCommand(binaryPath, subcommand string) string {
	return fmt.Sprintf("%s %s", binaryPath, subcommand)
}

// readJSONFile reads and parses a JSON file into a map
func readJSONFile(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("empty file")
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// expandHome replaces leading ~ with the user's home directory
func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

func printInstallHelp() {
	fmt.Fprintln(os.Stderr, `trimout install — set up trimout hooks for an AI coding agent

Usage:
  trimout install claude-code          Print hook configuration to add
  trimout install claude-code --check  Verify installation

The install command prints the hook configuration for the specified
agent. Running it multiple times is safe — it detects existing hooks
and reports whether they need updating.

Flags:
  --check    Verify agent-specific setup: hooks configured,
             binary exists, no broken project settings

Supported agents:
  claude-code    Claude Code (PreToolUse/PostToolUse hooks)`)
}
