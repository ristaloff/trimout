# trimout

Output trimmer for AI coding agents. Compresses verbose build/test
output to reduce context window usage while preserving errors
unfiltered for diagnosis.

Works with any agent that executes shell commands — Claude Code, Cursor,
Cline, Aider, Codex, or custom setups.

Zero dependencies. Single static binary.

## Quick start

### Claude Code

Add hooks to `~/.claude/settings.json` — see [Claude Code setup](#claude-code-setup) below.
Once configured, all matching commands are trimmed automatically via hooks.

### Any agent (generic)

Check if a command should be trimmed, and get the rewritten pipeline:

```bash
if rewritten=$(trimout "dotnet build --no-restore"); then
  eval "$rewritten"    # runs the trimmed pipeline
else
  dotnet build --no-restore  # not on allowlist — run normally
fi
```

Or pipe output directly (you manage the log file):

```bash
dotnet build 2>&1 | tee build.log | trimout filter --log build.log
# --log tells trimout where the full output is saved (for the pointer text)
```

## What it does

- **Short output** (<=30 lines): passes through unchanged
- **Clean long output** (>30 lines, no errors): compressed to first 5 + last 5 lines with a log pointer
- **Errors detected** (<=500 lines): passes through entirely so you can diagnose
- **Errors detected** (>500 lines): shows head/tail + up to 30 extracted error lines
- **Full output**: always saved to a log file (7-day retention)

### Opt out

Add `# nofilter` anywhere in the command string to bypass trimming:

```
dotnet test --no-build # nofilter
```

This works in piped and chained commands too — `# nofilter` anywhere
in the full command string disables trimming for the entire command.

## Supported commands

Trimming activates for commands matching these patterns (word-boundary regex,
matches anywhere in the command including pipes and chains):

| Ecosystem | Commands |
|-----------|----------|
| .NET | `dotnet build`, `test`, `publish`, `restore`, `format`, `clean` |
| Node | `npm install/ci/test/run`, `npx tsc/jest/vitest`, `yarn`, `pnpm` |
| Rust | `cargo build`, `test`, `clippy` |
| Go | `go build`, `test` |
| Python | `pytest`, `python3 -m pytest`, `pip install`, `uv pip install`, `poetry install`, `mypy`, `tox` |
| Containers | `docker build`, `docker compose build` |
| Build systems | `make`, `cmake`, `gradle`, `mvn` |

Non-matching commands (git, ls, echo, etc.) pass through untouched.

Patterns are word-boundary regexes — see
[patterns.go](patterns.go) for the full list.

## Usage

```
trimout "command"                  Check allowlist + output rewritten pipeline
trimout --check "command"          Just check if command would be trimmed (exit 0/1)
trimout --log-dir DIR "command"    Custom log directory
trimout --session ID "command"     Custom session ID
trimout filter [--log F] [--session S]   Stdin→stdout text filter (the core engine)
trimout hook                       Claude Code PreToolUse adapter
trimout metrics                    Claude Code PostToolUse adapter
trimout --version                  Print version
trimout --help                     Help (also works per subcommand)
```

### Default behavior

When called with a command string as the argument, trimout checks the
allowlist. If the command matches, it prints a rewritten bash pipeline
to stdout (exit 0). If not, it prints nothing (exit 1). Exit 2 on
bad usage (no args, unknown subcommand).

The rewritten pipeline requires **bash** (uses `PIPESTATUS` for exit
code preservation). It saves full output to a log file and pipes through
`trimout filter` for compression.

### `filter` subcommand

Pure stdin→stdout text filter. Reads command output, compresses if clean
and long, passes through if errors detected or output is short.

This is the portable core — any integration can pipe through it.

### `hook` / `metrics` subcommands

Protocol-specific adapters for Claude Code's PreToolUse and PostToolUse
hook system. These read/write Claude Code's JSON hook protocol on
stdin/stdout. See [Claude Code setup](#claude-code-setup).

## Installation

### From source (requires Go 1.21+)

```bash
go install github.com/ristaloff/trimout@latest
```

Or clone and build:

```bash
git clone https://github.com/ristaloff/trimout.git
cd trimout
make install  # installs to ~/.local/bin/
```

## Configuration

### Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `TRIMOUT_HOME` | `~/.local/state/trimout` | Base directory for logs and metrics |
| `XDG_STATE_HOME` | `~/.local/state` | XDG state directory (used if `TRIMOUT_HOME` not set) |

### Claude Code setup

Add to `~/.claude/settings.json`:

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "$HOME/.local/bin/trimout hook"
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
            "command": "$HOME/.local/bin/trimout metrics"
          }
        ]
      }
    ]
  }
}
```

Adjust the path if `trimout` is installed elsewhere (check with `which trimout`).
If installed via `go install`, the binary is typically in `~/go/bin/`.

Logs and metrics go to `$TRIMOUT_HOME` (default: `~/.local/state/trimout/`).
Set `TRIMOUT_HOME=$HOME/.claude` if you want them alongside Claude Code's data.

## Architecture

```
┌─────────────────────────────────────────────────┐
│  Agent (Claude Code, Cursor, Aider, ...)        │
│                                                  │
│  "dotnet build --no-restore"                     │
└──────────────┬──────────────────────────────────┘
               │
     ┌─────────▼─────────┐
     │  trimout "cmd"     │  ← or `trimout hook` for Claude Code
     │  (check allowlist, │
     │   build pipeline)  │
     └─────────┬─────────┘
               │ rewritten command
     ┌─────────▼──────────────────────────────────┐
     │ ( dotnet build ) 2>&1                       │
     │   | tee LOG                                 │
     │   | trimout filter --log LOG                │
     │ ; _ec=${PIPESTATUS[0]}; ... exit $_ec       │
     └─────────┬──────────────────────────────────┘
               │
     ┌─────────▼─────────┐
     │ trimout filter     │  ← the core engine
     │  (compress/pass)   │
     └───────────────────┘
```

### Metrics

Filter statistics are written to `$TRIMOUT_HOME/metrics/filter-stats.jsonl`:

```json
{"ts":"...","session":"...","log":"...","original_lines":274,"filtered_lines":13,"action":"compressed"}
```

Actions: `short` (passthrough), `compressed` (head+tail), `errors` (full passthrough),
`errors_capped` (errors extracted from large output).

## Error detection

Lines are classified as errors if they match (case-insensitive):

```
error:  error[  error   fail  failed  fatal  exception  Error:
```

With false-positive exclusions for success summaries:

```
Failed:     0       0 Error(s)
```

## Testing

```bash
go test -v ./...
```

## License

MIT
