package main

import (
	"fmt"
	"os"
)

var version = "dev"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "--version", "-v":
		fmt.Printf("trimout %s\n", version)
	case "--help", "-h":
		printUsage()
	case "filter":
		if hasHelpFlag(os.Args[2:]) {
			printFilterHelp()
			return
		}
		logPath, sessionID := parseFilterArgs(os.Args[2:])
		runFilter(logPath, sessionID)
	case "hook":
		if hasHelpFlag(os.Args[2:]) {
			printHookHelp()
			return
		}
		runHook()
	case "metrics":
		if hasHelpFlag(os.Args[2:]) {
			printMetricsHelp()
			return
		}
		runMetrics()
	case "install":
		if hasHelpFlag(os.Args[2:]) {
			printInstallHelp()
			return
		}
		runInstall(os.Args[2:])
	default:
		// Default: treat all args as the rewrite command
		if hasHelpFlag(os.Args[1:]) {
			printUsage()
			return
		}
		runRewrite(os.Args[1:])
	}
}

func hasHelpFlag(args []string) bool {
	for _, a := range args {
		if a == "--help" || a == "-h" {
			return true
		}
	}
	return false
}

func printUsage() {
	fmt.Fprintln(os.Stderr, `trimout — output trimmer for AI coding agents

Compresses verbose build/test output to reduce context window usage
while preserving errors unfiltered for diagnosis.

Usage:
  trimout [flags] "command"      Check allowlist and output rewritten pipeline
  trimout filter                 Stdin→stdout text filter (the core engine)
  trimout hook                   Claude Code PreToolUse adapter
  trimout metrics                Claude Code PostToolUse adapter
  trimout install <agent>        Set up hooks for an agent (e.g. claude-code)
  trimout install <agent> --check  Verify installation is healthy
  trimout --version              Print version

If the command matches the allowlist, prints a rewritten bash pipeline
to stdout (exit 0). If not, prints nothing (exit 1). Exit 2 on bad usage.

The rewritten pipeline requires bash (uses PIPESTATUS for exit code
preservation). It saves full output to a log file and pipes through
trimout filter for compression.

Integration:
  if rewritten=$(trimout "dotnet build"); then
    eval "$rewritten"
  else
    dotnet build  # not on allowlist — run normally
  fi

Or pipe directly:
  dotnet build 2>&1 | tee build.log | trimout filter --log build.log

Opt out of trimming for any command with # nofilter anywhere in the string:
  dotnet test --no-build # nofilter

Flags:
  --log-dir DIR    Directory for full output logs (default: /tmp/trimout-data/logs/)
  --session ID     Session identifier for metrics correlation
  --check          Only check if the command would be trimmed; no output, exit 0/1
  -h, --help       Show this help

Data:
  Logs and metrics are written to /tmp/trimout-data/ (ephemeral).`)
}

func printFilterHelp() {
	fmt.Fprintln(os.Stderr, `trimout filter — stdin→stdout text filter

Reads command output from stdin and writes filtered output to stdout.
Short output (<=30 lines) passes through unchanged. Clean long output
is compressed to the first 5 and last 5 lines with a log file pointer.
Output containing errors passes through unfiltered for diagnosis.

Usage:
  trimout filter [flags]

Exit code: always 0. Errors in the command output do not affect the
exit code — they are preserved in the output text for diagnosis.

Flags:
  --log FILE          Path to the full output log (shown in the filter pointer)
  --session ID        Session identifier for metrics correlation
  -h, --help          Show this help

Positional args are also supported: trimout filter <log> <session>`)
}

func printHookHelp() {
	fmt.Fprintln(os.Stderr, `trimout hook — Claude Code PreToolUse adapter

Reads Claude Code's PreToolUse hook JSON from stdin. If the command
matches the allowlist and does not contain '# nofilter', outputs a
JSON response that rewrites the command into a trimmed pipeline.
Returns empty output (no rewrite) for non-matching commands.

This subcommand is Claude Code-specific. For other agents, run
trimout directly with the command as an argument.

Usage:
  trimout hook < <hook-json>

Setup:
  trimout install claude-code`)
}

func printMetricsHelp() {
	fmt.Fprintln(os.Stderr, `trimout metrics — Claude Code PostToolUse adapter

Reads Claude Code's PostToolUse hook JSON from stdin and appends
execution metrics (command pattern, byte counts, exit code) to
/tmp/trimout-data/metrics/tool-output.jsonl.

This subcommand is Claude Code-specific. For other agents, filter
statistics are written automatically by 'trimout filter'.

Usage:
  trimout metrics < <hook-json>

Setup:
  trimout install claude-code`)
}

// parseFilterArgs extracts --log and --session from args.
// Also accepts positional args for backward compat: filter <log> <session>
func parseFilterArgs(args []string) (logPath, sessionID string) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--log":
			if i+1 < len(args) {
				logPath = args[i+1]
				i++
			}
		case "--session":
			if i+1 < len(args) {
				sessionID = args[i+1]
				i++
			}
		default:
			// Positional fallback: first unknown = log, second = session
			if logPath == "" {
				logPath = args[i]
			} else if sessionID == "" {
				sessionID = args[i]
			}
		}
	}
	return
}
