# trimout

Output trimmer for AI coding agents. Compresses verbose build/test
output to reduce context window usage while preserving errors unfiltered.
Works with Claude Code, Cursor, Cline, Aider, Codex, or custom setups.

Single static binary.

## Before & after

A `dotnet build` produces ~274 lines of verbose output. trimout compresses
it to 13 — the first 5, the last 5, a filtered-line count, and a log pointer:

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

Errors pass through unfiltered — full context when something breaks.

### Context savings

| Scenario | Raw lines | Trimmed | Raw bytes | Trimmed | Reduction |
|----------|----------:|--------:|----------:|--------:|----------:|
| `dotnet build` — multi-project | 129 | 13 | 40 KB | 1.4 KB | 96.5% |
| `npm install` — large dependency tree | 507 | 13 | 33 KB | 0.5 KB | 98.4% |
| `dotnet build` — single project | 268 | 13 | 17 KB | 0.4 KB | 97.6% |
| Build with errors | 45 | 45 | 1.1 KB | 1.1 KB | 0% |

Errors always pass through unfiltered — 0% reduction on failures is by design.

Each filtered build saves roughly 5,000-10,000 tokens of context window
depending on output verbosity (~4 bytes per token across most LLM tokenizers).

## Install

Requires Go 1.21+:

```bash
go install github.com/ristaloff/trimout@latest
```

If `~/go/bin` isn't on your `PATH`, copy the binary:

```bash
cp ~/go/bin/trimout ~/.local/bin/
```

Or clone and build to `~/.local/bin/` directly:

```bash
git clone https://github.com/ristaloff/trimout.git
cd trimout
make install
```

Verify:

```bash
trimout --version
trimout --check "dotnet build"  # exit 0 = would be trimmed
```

## Quick start

### Claude Code

Run `trimout install claude-code` to get the hook configuration for
your `~/.claude/settings.json`. The install command detects your binary
path and prints the exact JSON to add.

```bash
trimout install claude-code          # print hooks to add
trimout install claude-code --check  # verify installation
```

### Any agent (generic)

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

## What it does

`trimout "cmd"` checks the allowlist and builds a pipeline around
`trimout filter`, the core text filter. Both use the same logic:

- **Short output** (<=30 lines): passes through unchanged
- **Clean long output** (>30 lines, no errors): compressed to first 5 + last 5 lines with a log pointer
- **Errors detected** (<=500 lines): passes through entirely so you can diagnose
- **Errors detected** (>500 lines): shows head/tail + up to 30 extracted error lines
- **Full output**: always saved to a log file in `/tmp/trimout-data/logs/`

### Opt out

Add `# nofilter` anywhere in the command string to bypass trimming:

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

Exit codes: 0 = match/success, 1 = no match, 2 = bad usage.
The rewritten pipeline requires **bash** (`PIPESTATUS` for exit code preservation).

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

Metrics are written to `/tmp/trimout-data/metrics/tool-output.jsonl` by the
PostToolUse hook. Each entry includes the command, byte counts, duration,
exit code, and for filtered commands: `original_lines`, `filtered_lines`,
and `filtered: true`.

## Error detection

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
