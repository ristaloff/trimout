package main

import "regexp"

// Filtering constants.
const (
	HeadLines      = 5
	TailLines      = 5
	Threshold      = 30
	MaxPassthrough = 500
	MaxErrorLines  = 30
	LogRetentionD  = 7
)

// allowlistPatterns are word-boundary regexes matching known-verbose tool
// invocations. No ^ anchor — matches anywhere in the command.
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

func compilePatterns(raw []string) []*regexp.Regexp {
	out := make([]*regexp.Regexp, len(raw))
	for i, p := range raw {
		out[i] = regexp.MustCompile(p)
	}
	return out
}

// matchesAllowlist returns true if any allowlist pattern matches the command.
func matchesAllowlist(cmd string) bool {
	for _, re := range allowlistPatterns {
		if re.MatchString(cmd) {
			return true
		}
	}
	return false
}

// isErrorLine returns true if the line looks like a real error (not a false positive).
func isErrorLine(line string) bool {
	return errorDetect.MatchString(line) && !falsePositive.MatchString(line)
}
