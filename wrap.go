package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// buildRewrittenCommand constructs the pipeline that wraps a command
// with tee (for logging) and trimout filter (for compression).
// Exit code is preserved via PIPESTATUS and written to a sidecar file.
func buildRewrittenCommand(cmd, logFile, filterBinary, sessionID string) string {
	return fmt.Sprintf(
		"( %s ) 2>&1 | tee %s | %s filter --log %s --session %s; _ec=${PIPESTATUS[0]}; printf '%%d' $_ec > %s.exit; exit $_ec",
		cmd, logFile, filterBinary, logFile, sessionID, logFile,
	)
}

// buildLogFile creates a timestamped log file path in the given directory.
func buildLogFile(logDir string) string {
	timestamp := time.Now().UTC().Format("20060102T150405Z")
	return filepath.Join(logDir, fmt.Sprintf("%s-%d.log", timestamp, os.Getpid()))
}

// selfPath returns the path to the currently running binary.
func selfPath() string {
	self, err := os.Executable()
	if err != nil {
		return "trimout"
	}
	return self
}

// runRewrite is the generic (agent-agnostic) entry point.
// It checks the allowlist, builds a rewritten command, and prints it to stdout.
// Exit 0 = command was rewritten (output on stdout).
// Exit 1 = command not on allowlist or opted out (no output).
func runRewrite(args []string) {
	var logDir, sessionID, cmd string
	checkOnly := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--log-dir":
			if i+1 < len(args) {
				logDir = args[i+1]
				i++
			}
		case "--session":
			if i+1 < len(args) {
				sessionID = args[i+1]
				i++
			}
		case "--check":
			checkOnly = true
		default:
			if cmd == "" {
				cmd = args[i]
			}
		}
	}

	if cmd == "" {
		fmt.Fprintln(os.Stderr, "trimout: no command provided")
		os.Exit(1)
	}

	// Opt-out: # nofilter
	if nofilterRe.MatchString(cmd) {
		os.Exit(1)
	}

	// Check allowlist
	if !matchesAllowlist(cmd) {
		os.Exit(1)
	}

	// --check: just report whether the command would be trimmed
	if checkOnly {
		os.Exit(0)
	}

	// Resolve log directory
	if logDir == "" {
		logDir = LogDir()
	}
	os.MkdirAll(logDir, 0o755)
	pruneOldLogs(logDir)

	logFile := buildLogFile(logDir)
	self := selfPath()

	if sessionID == "" {
		sessionID = fmt.Sprintf("trimout-%d", os.Getpid())
	}

	rewritten := buildRewrittenCommand(cmd, logFile, self, sessionID)
	fmt.Println(rewritten)
}
