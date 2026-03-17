# trimout

Output trimmer for AI coding agents. Compresses verbose build/test output
to reduce context window usage while preserving errors unfiltered.

## What trimout does to your commands

When configured, trimout intercepts build/test commands and filters output:

- **Short output** (<=30 lines): unchanged
- **Clean long output** (>30 lines, no errors): compressed to first 5 + last 5 lines
- **Errors detected**: passes through unfiltered so you can diagnose
- **Full output**: always saved to a log file you can read if needed

If output looks compressed, that's trimout working — you don't call it directly.

## When you see filtered output

- **Errors always pass through unfiltered** — you have everything you need
- **Compressed output means success** — details are in the log
- **Need full output?** Read the log file at the path shown

```
... (264 lines filtered)

Full output: /tmp/trimout-data/logs/20260316-183000.log
```

### Opting out

When you need unfiltered output (debugging, analyzing full logs, etc.),
add `# nofilter` to the command. Do not use workarounds like `| cat` —
they break the pipeline and lose metrics.

```bash
dotnet test --no-build # nofilter
```

This works anywhere in the command string, including piped and chained commands.

## Commands that get filtered

Matches anywhere in the command including pipes and chains:

- **dotnet** build, test, publish, restore, format, clean
- **npm/yarn/pnpm** install, ci, test, run; **npx** tsc, jest, vitest
- **cargo** build, test, clippy
- **go** build, test
- **pytest**, pip install, uv pip install, poetry install, mypy, tox
- **docker** build, docker compose build
- **make**, cmake, gradle, mvn

Everything else passes through untouched.

## Install

Requires Go 1.21+:

```bash
go install github.com/ristaloff/trimout@latest
```

If `~/go/bin` isn't on your PATH: `cp ~/go/bin/trimout ~/.local/bin/`

## Setup

### Claude Code

```bash
trimout install claude-code          # prints hook JSON with correct binary path
trimout install claude-code --check  # verify everything works
```

Add the output to `~/.claude/settings.json`, then **restart Claude Code**
(hooks load at session start).

### Other agents

Allowlist check — exits 0 with rewritten pipeline if matched, exits 1 if not:

```bash
if rewritten=$(trimout "dotnet build --no-restore"); then
  eval "$rewritten"    # runs: build | tee log | trimout filter
else
  dotnet build --no-restore  # not on allowlist — run normally
fi
```

Or pipe through the filter directly:

```bash
dotnet build 2>&1 | tee build.log | trimout filter --log build.log
```

## Logs and metrics

- Full unfiltered output: `/tmp/trimout-data/logs/`
- Metrics: `/tmp/trimout-data/metrics/tool-output.jsonl`
- Data is ephemeral (cleared on reboot) — this is by design for sandbox compatibility

## Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| Output looks empty after build | Redirects (`> file`) capture output before the filter | Remove redirects or add `# nofilter` |
| Errors not showing | Error pattern didn't match | Read the full log file, or rerun with `# nofilter` |
| `trimout: command not found` | Binary not on PATH | `which trimout`, use full path in hook config |
| Filter not activating | Command not on allowlist | `trimout --check "your command"` (exit 0 = matches) |

## CLI reference

Run `trimout --help` for full usage, flags, and examples.
Each subcommand also has help: `trimout filter --help`, `trimout hook --help`, etc.

Source: https://github.com/ristaloff/trimout
