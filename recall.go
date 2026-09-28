package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Recall view sizes. Larger than the inline filter's, because reaching for
// recall is an explicit request for more — but still bounded, so recovering
// context can never cost more than having skipped filtering.
const (
	RecallHeadLines     = 20
	RecallTailLines     = 20
	RecallSectionLines  = 400
	RecallLinesPerBlock = 80
	RecallCtxBefore     = 3
	RecallCtxAfter      = 10
)

// runRecall prints a bounded view of a saved log. The filter's elision
// marker names this command so that an agent recovering the removed lines
// gets the diagnosis rather than the whole file.
func runRecall(args []string) {
	var (
		logPath string
		headN   int
		tailN   int
		all     bool
	)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--all":
			all = true
		case "--head", "--tail":
			if i+1 >= len(args) {
				fmt.Fprintf(os.Stderr, "trimout recall: %s needs a line count\n", args[i])
				os.Exit(2)
			}
			n, err := strconv.Atoi(args[i+1])
			if err != nil || n < 1 {
				fmt.Fprintf(os.Stderr, "trimout recall: %s wants a positive number, got %q\n",
					args[i], args[i+1])
				os.Exit(2)
			}
			if args[i] == "--head" {
				headN = n
			} else {
				tailN = n
			}
			i++
		default:
			if logPath == "" {
				logPath = args[i]
			}
		}
	}

	if logPath == "" {
		fmt.Fprintln(os.Stderr, "trimout recall: no log file given")
		os.Exit(2)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "trimout recall: %v\n", err)
		os.Exit(1)
	}

	input := strings.ReplaceAll(string(data), "\r", "")
	fmt.Println(recallView(input, logPath, headN, tailN, all))
}

// recallView selects the bounded slice of a log to show.
func recallView(input, logPath string, headN, tailN int, all bool) string {
	if all {
		return strings.TrimRight(input, "\n")
	}

	lines := splitLines(input)
	total := len(lines)

	switch {
	case headN > 0:
		return sliceView(lines, 0, min(headN, total), logPath)
	case tailN > 0:
		return sliceView(lines, max(0, total-tailN), total, logPath)
	}

	// Default: the errors with their context, which is what the removed
	// lines were hiding. Fall back to head+tail when there are none.
	if idx := errorIndexes(lines); len(idx) > 0 {
		return recallErrors(lines, idx, logPath)
	}

	if total <= RecallHeadLines+RecallTailLines {
		return strings.Join(lines, "\n")
	}

	var b strings.Builder
	b.WriteString(strings.Join(lines[:RecallHeadLines], "\n"))
	fmt.Fprintf(&b, "\n\n... (%d lines — %s)\n\n",
		total-RecallHeadLines-RecallTailLines, recallMore(logPath))
	b.WriteString(strings.Join(lines[total-RecallTailLines:], "\n"))
	return b.String()
}

// recallErrors renders the error blocks with wide context, under budget.
func recallErrors(lines []string, idx []int, logPath string) string {
	ws := mergeWindows(idx, len(lines), RecallCtxBefore, RecallCtxAfter)
	body, kept, _ := renderBlocks(lines, ws, errorBudget{MaxSectionLines: RecallSectionLines, MaxLinesPerBlock: RecallLinesPerBlock})

	var b strings.Builder
	b.WriteString(strings.TrimRight(body, "\n"))
	if hidden := len(lines) - kept; hidden > 0 {
		fmt.Fprintf(&b, "\n\n... (%d lines not shown — %s)", hidden, recallMore(logPath))
	}
	return b.String()
}

// sliceView renders an explicit head/tail request.
func sliceView(lines []string, from, to int, logPath string) string {
	var b strings.Builder
	b.WriteString(strings.Join(lines[from:to], "\n"))
	if hidden := len(lines) - (to - from); hidden > 0 {
		fmt.Fprintf(&b, "\n\n... (%d lines not shown — %s)", hidden, recallMore(logPath))
	}
	return b.String()
}

func recallMore(logPath string) string {
	return fmt.Sprintf("trimout recall %s --all", logPath)
}
