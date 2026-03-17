# trimout

Output trimmer for AI coding agents. Compresses verbose build/test
output to reduce context window usage while preserving errors unfiltered.

Single static binary. Works with Claude Code, Cursor, Cline, Aider,
Codex, or custom setups.

## The problem

A `dotnet test` run produces 2,000+ lines. A `npm install` dumps 500.
Most of this is progress noise — but it all enters the agent's context
window, pushing out the code and conversation that matter.

Claude Code truncates output over 30,000 characters, but most build
output is well under that — it goes in uncompressed. And when truncation
does kick in, it blindly removes the middle, potentially losing error
context.

trimout compresses clean output and preserves errors:

```
$ dotnet build
  Microsoft (R) Build Engine version 17.9.6+a4ecab324
  Build started 3/16/2026 6:30:00 PM.
  Project "/src/MyApp/MyApp.csproj" on node 1 (default targets).
  CoreCompile:
    /usr/bin/dotnet exec /usr/share/dotnet/sdk/8.0.303/Roslyn/bincore/csc.dll ...

... (264 lines filtered)

  Build succeeded.
      0 Warning(s)
      0 Error(s)

  Time Elapsed 00:00:03.42

Full output: /tmp/trimout-data/logs/20260316-183000.log
```

274 lines → 13. Errors pass through unfiltered.

## Get started (Claude Code)

### 1. Install

Requires [Go](https://go.dev/dl/) 1.21+:

```bash
go install github.com/ristaloff/trimout@latest
```

If `~/go/bin` isn't on your `PATH`: `cp ~/go/bin/trimout ~/.local/bin/`

### 2. Configure

```bash
trimout install claude-code
```

Copy the output into `~/.claude/settings.json`. If the file doesn't
exist yet, the command prints a complete file you can save directly.

### 3. Restart Claude Code

Hooks load at session start. **You must restart** for trimout to take effect.

### 4. Verify

```bash
trimout install claude-code --check
```

All checks should pass. If something is wrong, the output tells you
exactly what to fix.

## Logs & metrics

- Full unfiltered output: `/tmp/trimout-data/logs/`
- Per-command metrics: `/tmp/trimout-data/metrics/tool-output.jsonl`
- Ephemeral — cleared on reboot by design

Each metrics entry includes command, byte counts, duration, exit code,
and for filtered commands: `original_lines`, `filtered_lines`.

## Impact

Per-command reduction from real development sessions:

| Scenario | Raw lines | Trimmed | Reduction |
|----------|----------:|--------:|----------:|
| `dotnet test` — full suite | 2,269 | 12 | 99.5% |
| `dotnet test` — integration tests | 1,863 | 43 | 97.7% |
| `dotnet test` — subset | 194 | 13 | 93.3% |
| `dotnet build` — multi-project | 38 | 12 | 68.4% |
| `dotnet test` — with errors | 100 | 99 | 0% |

Errors always pass through unfiltered — 0% reduction on failures is by design.

### Session-level (early data — 3 sessions, will update)

| Metric | Value |
|--------|------:|
| Filtered commands | 130 |
| Lines saved | 7,899 (83%) |
| Tokens saved | ~158,000 |
| Bash context reduced | 49% |
| Avg saved per session | ~53,000 tokens |

Nearly half of all Bash tool context was build/test noise — context
the agent can use for reasoning instead.

## How it works

- **Short output** (<=30 lines): passes through unchanged
- **Clean long output** (>30 lines, no errors): compressed to first 5 + last 5 lines with a log pointer
- **Errors detected** (<=500 lines): passes through entirely so you can diagnose
- **Errors detected** (>500 lines): shows head/tail + up to 30 extracted error lines
- **Full output**: always saved to `/tmp/trimout-data/logs/`

### Opt out

Add `# nofilter` anywhere in the command string:

```
dotnet test --no-build # nofilter
```

## Supported commands

Matches anywhere in the command including pipes and chains (word-boundary regex):

| Ecosystem | Commands |
|-----------|----------|
| .NET | `dotnet build`, `test`, `publish`, `restore`, `format`, `clean` |
| Node | `npm install/ci/test/run`, `npx tsc/jest/vitest`, `yarn`, `pnpm` |
| Rust | `cargo build`, `test`, `clippy` |
| Go | `go build`, `test` |
| Python | `pytest`, `python3 -m pytest`, `pip install`, `uv pip install`, `poetry install`, `mypy`, `tox` |
| Containers | `docker build`, `docker compose build` |
| Build systems | `make`, `cmake`, `gradle`, `mvn` |

Non-matching commands pass through untouched.
Full list: [patterns.go](patterns.go).

## Other agents

**Command wrapper** — checks the allowlist, returns a rewritten pipeline:

```bash
if rewritten=$(trimout "dotnet build --no-restore"); then
  eval "$rewritten"    # runs: build | tee log | trimout filter
else
  dotnet build --no-restore  # not on allowlist — run normally
fi
```

**Pipe filter** — you control the pipeline, trimout filters stdin:

```bash
dotnet build 2>&1 | tee build.log | trimout filter --log build.log
```

`trimout "cmd"` checks the allowlist and builds a pipeline around
`trimout filter`, the core engine. Use the wrapper for convenience,
or the pipe filter for custom integrations.

## Reference

### Usage

```
trimout "command"                  Check allowlist + output rewritten pipeline
trimout --check "command"          Just check if command would be trimmed (exit 0/1)
trimout --log-dir DIR "command"    Custom log directory
trimout --session ID "command"     Custom session ID
trimout filter [--log F] [--session S]   Stdin→stdout text filter
trimout hook                       Claude Code PreToolUse adapter
trimout metrics                    Claude Code PostToolUse adapter
trimout install <agent>            Print hook configuration
trimout install <agent> --check    Verify installation
trimout --version                  Print version
trimout --help                     Help (also works per subcommand)
```

Exit codes: 0 = match/success, 1 = no match, 2 = bad usage.

### Architecture

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

### Error detection

Lines are classified as errors if they match (case-insensitive):

```
error:  error[  error   fail  failed  fatal  exception  Error:
```

With false-positive exclusions for success summaries:

```
Failed:     0       0 Error(s)
```

## Agent documentation

[`AGENTS.md`](AGENTS.md) provides agent-oriented docs — what filtered output
looks like, how to opt out, and troubleshooting. Agents that support the
convention read it automatically.

## Testing

```bash
go test -v ./...
```

## License

MIT
