package main

import (
	"regexp"
	"strings"
)

// Filtering constants.
const (
	HeadLines      = 5
	TailLines      = 5
	Threshold      = 30
	MaxPassthrough = 500
	LogRetentionD  = 7
)

// errorBudget bounds the error section of a compressed failing run.
//
// Two properties, both learned from the fixtures in testdata/:
//
// A failure is a block, not a line. The line matching the error pattern is
// usually a header ("Error Message:") while the lines that explain the
// failure — expected/actual, the stack frame carrying file:line — match
// nothing themselves. Budgeting by line alone makes the number of surviving
// failures depend on how verbose the tool is.
//
// Whole blocks are dropped, never truncated. Tools differ in whether they
// separate failures with progress output (go) or print them back to back
// (dotnet, python); in the latter case neighbouring failures merge into one
// block. Cutting such a block mid-way lands in the middle of a failure and
// leaves it undiagnosable while still spending the lines, so a block that
// does not fit is dropped and counted instead.
type errorBudget struct {
	MaxSectionLines  int
	MaxLinesPerBlock int
	CtxBefore        int
	CtxAfter         int
}

// defaultBudget is chosen by the eval in eval_test.go, not by taste. Run
// `go test -run TestEvalSweep -v` to see the trade-off these values sit on.
var defaultBudget = errorBudget{
	// CtxAfter is the one value the eval pins: at 10 the go panic and the
	// python traceback lose their cause line (32/33 diagnosable), at 12
	// every fixture is fully diagnosable (33/33). Above 12 costs lines and
	// recovers nothing.
	CtxAfter: 12,
	// CtxBefore is small on purpose — the lines before a failure are the
	// build noise that preceded it, not part of the diagnosis.
	CtxBefore: 2,
	// Diagnosability is flat in MaxSectionLines across the fixtures, because
	// test runners print failures back to back and the merged block is
	// always emitted. It is therefore a cost knob for the case the eval
	// cannot model well — failures scattered through a long build — and
	// whatever it drops is counted in the summary line.
	MaxSectionLines: 120,
	// Backstop for one pathological block (a suite failing in hundreds of
	// contiguous places). When it bites, the cut is stated inline.
	MaxLinesPerBlock: 60,
}

// allowlistPatterns are word-boundary regexes matching known-verbose tool
// invocations. Compiled anchored (see compilePatterns) and tested only
// against the head of each command segment, so a tool name appearing as an
// argument or inside a quoted string never triggers a rewrite.
var allowlistPatterns = compilePatterns([]string{
	`\bdotnet build\b`,
	`\bdotnet test\b`,
	`\bdotnet publish\b`,
	`\bdotnet restore\b`,
	`\bdotnet format\b`,
	`\bdotnet clean\b`,
	`\bnpm install\b`,
	`\bnpm ci\b`,
	`\bnpm test\b`,
	`\bnpm run\b`,
	`\bnpx tsc\b`,
	`\bnpx jest\b`,
	`\bnpx vitest\b`,
	`\byarn install\b`,
	`\byarn build\b`,
	`\byarn test\b`,
	`\bpnpm install\b`,
	`\bpnpm build\b`,
	`\bpnpm test\b`,
	`\bcargo build\b`,
	`\bcargo test\b`,
	`\bcargo clippy\b`,
	`\bgo build\b`,
	`\bgo test\b`,
	`\bpytest\b`,
	`\bpython3? -m pytest\b`,
	`\bpip install\b`,
	`\bpip3 install\b`,
	`\buv pip install\b`,
	`\bpoetry install\b`,
	`\bdocker build\b`,
	`\bdocker compose build\b`,
	`\bmake\b`,
	`\bcmake\b`,
	`\bgradle\b`,
	`\bmvn\b`,
	`\bmypy\b`,
	`\btox\b`,
})

// errorDetect matches error/failure lines in build output.
// Case-insensitive matching applied at call site.
var errorDetect = regexp.MustCompile(`(?i)(\berror[: \[]|\bfail\b|failed|fatal|exception|Error[:$])`)

// falsePositive excludes success-summary patterns that look like errors.
var falsePositive = regexp.MustCompile(`(?i)(failed:[[:space:]]+0|0[[:space:]]+error)`)

// envAssign matches a leading VAR= environment assignment.
var envAssign = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

// optionValue matches the argument shapes a wrapper's own options take —
// a duration or count, or an environment variable name. Used only after a
// known wrapper, to step over its options without a per-wrapper table.
var optionValue = regexp.MustCompile(`^(\d+(\.\d+)?[smhd]?|[A-Z][A-Z0-9_]*)$`)

// shellKeywords precede a command inside a conditional or loop body. The
// segment splitter breaks on ";" so these arrive at the head of a segment
// (`for d in a b; do make -C $d; done` -> " do make -C $d").
var shellKeywords = map[string]bool{
	"then": true, "do": true, "else": true, "elif": true,
	"if": true, "while": true, "until": true, "!": true,
}

// wrapperPrefixes run another command without changing which command runs,
// so the allowlist test looks past them. Single-token only — a wrapper that
// consumes its own arguments is handled explicitly (see commandHead) or not
// at all, because guessing wrong makes an argument look like the command.
var wrapperPrefixes = map[string]bool{
	"sudo":    true,
	"env":     true,
	"command": true,
	"exec":    true,
	"nohup":   true,
	"nice":    true,
	"time":    true,
	"stdbuf":  true,
	"timeout": true,
}

// runnerPrefixes are two-token runners that execute a tool inside a managed
// environment. Without these, anchoring would lose real invocations that the
// previous substring match caught (e.g. `uv run pytest`).
var runnerPrefixes = map[string]string{
	"uv":     "run",
	"poetry": "run",
	"pdm":    "run",
	"rye":    "run",
	"bundle": "exec",
	"pnpm":   "exec",
	"npm":    "exec",
}

func compilePatterns(raw []string) []*regexp.Regexp {
	out := make([]*regexp.Regexp, len(raw))
	for i, p := range raw {
		// Anchored: the pattern must match where the command name starts.
		out[i] = regexp.MustCompile("^" + p)
	}
	return out
}

// commandSegments splits a command string into executable segments on
// unquoted shell operators. Quoted content is copied verbatim and never
// treated as a boundary, so a tool name inside "..." cannot start a segment.
func commandSegments(cmd string) []string {
	var (
		segs  []string
		cur   strings.Builder
		quote byte
	)
	flush := func() {
		segs = append(segs, cur.String())
		cur.Reset()
	}
	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		if quote != 0 {
			if c == '\\' && quote == '"' && i+1 < len(cmd) {
				cur.WriteByte(c)
				i++
				cur.WriteByte(cmd[i])
				continue
			}
			cur.WriteByte(c)
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"':
			quote = c
			cur.WriteByte(c)
		case '\\':
			if i+1 < len(cmd) {
				i++
				cur.WriteByte(c)
				cur.WriteByte(cmd[i])
			}
		case '|', '&', ';', '\n', '(', ')', '{', '}', '`':
			// Operator, subshell, or command substitution — whatever
			// follows starts a fresh command.
			flush()
		default:
			cur.WriteByte(c)
		}
	}
	flush()
	return segs
}

// splitToken returns the first token and the remainder, splitting on the
// first whitespace that falls outside quotes. Quote awareness matters for
// env assignments with spaces in the value (FOO="a b" make) — splitting
// naively would leave the value's tail looking like the command.
func splitToken(s string) (tok, rest string) {
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '\'' || c == '"':
			quote = c
		case c == '\\' && i+1 < len(s):
			i++
		case c == ' ' || c == '\t':
			return s[:i], strings.TrimLeft(s[i:], " \t")
		}
	}
	return s, ""
}

// commandHead strips leading environment assignments and process wrappers
// from a segment and returns the text starting at the actual command name.
// Returns "" when the segment holds no command.
func commandHead(seg string) string {
	s := strings.TrimSpace(seg)
	for s != "" {
		tok, rest := splitToken(s)

		if envAssign.MatchString(tok) || shellKeywords[tok] {
			s = rest
			continue
		}

		if wrapperPrefixes[tok] {
			// Step over the wrapper's own options and their values, then
			// stop at the first ordinary word — that is the command. Going
			// further would let `sudo find / -name make` look like a make
			// invocation.
			s = rest
			for s != "" {
				next, after := splitToken(s)
				if strings.HasPrefix(next, "-") || optionValue.MatchString(next) {
					s = after
					continue
				}
				break
			}
			continue
		}

		if sub, ok := runnerPrefixes[tok]; ok {
			next, after := splitToken(rest)
			if next == sub && after != "" {
				s = after
				continue
			}
		}

		return s
	}
	return ""
}

// matchesAllowlist reports whether any segment of the command invokes an
// allowlisted tool. The tool name must appear in command position: a
// pattern that only occurs in an argument, a path, or a quoted string does
// not match, because compressing output the caller explicitly asked for
// (a grep result, a commit message) loses data rather than noise.
func matchesAllowlist(cmd string) bool {
	for _, seg := range commandSegments(cmd) {
		head := commandHead(seg)
		if head == "" {
			continue
		}
		for _, re := range allowlistPatterns {
			if re.MatchString(head) {
				return true
			}
		}
	}
	return false
}

// isErrorLine returns true if the line looks like a real error (not a false positive).
func isErrorLine(line string) bool {
	return errorDetect.MatchString(line) && !falsePositive.MatchString(line)
}
