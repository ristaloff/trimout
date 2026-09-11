package main

import (
	"fmt"
	"io"
	"os"
	"strings"
)

func runFilter(logPath, sessionID string) {
	// Read all stdin, strip carriage returns
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[trimout: error reading stdin]")
		return
	}

	input := strings.ReplaceAll(string(raw), "\r", "")

	emit(os.Stdout, neverWorse(input, compress(input, logPath)))
}

// emit writes the text with exactly one trailing newline. Appending one
// unconditionally would add a byte to output that already ends in a
// newline, which is enough on its own to break the never-worse guarantee
// on short passthrough.
func emit(w io.Writer, text string) {
	if strings.HasSuffix(text, "\n") {
		fmt.Fprint(w, text)
		return
	}
	fmt.Fprintln(w, text)
}

// neverWorse returns filtered unless emitting it would cost more than not
// filtering at all. trimout exists to shrink output; a compressed form that
// is larger than the raw text is a loss on both counts — more context spent
// and less information carried — so the raw text wins, ties included.
func neverWorse(raw, filtered string) string {
	if len(filtered) >= len(raw) {
		return raw
	}
	return filtered
}

// splitLines splits output into lines, matching bash's line counting
// (echo "x" | wc -l == 1) by dropping the artifact of a trailing newline.
func splitLines(input string) []string {
	lines := strings.Split(input, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// compress applies the filtering rules and returns the text to emit.
// Returning input unchanged means "pass through".
func compress(input, logPath string) string {
	lines := splitLines(input)

	// Short output: pass through unchanged
	if len(lines) <= Threshold {
		return input
	}

	errorIdx := errorIndexes(lines)
	if len(errorIdx) > 0 {
		// Small enough to pass through entirely for diagnosis
		if len(lines) <= MaxPassthrough {
			return input
		}
		return compressWithErrors(lines, errorIdx, logPath)
	}

	return compressClean(lines, logPath)
}

// errorIndexes returns the line numbers that look like real errors.
func errorIndexes(lines []string) []int {
	var idx []int
	for i, line := range lines {
		if isErrorLine(line) {
			idx = append(idx, i)
		}
	}
	return idx
}

// window is an inclusive run of lines kept around one or more errors.
type window struct {
	start, end int
}

// mergeWindows expands each error line into a context window of the given
// size and merges windows that overlap or touch, so a multi-line failure
// block survives as one block rather than as disconnected header lines.
func mergeWindows(errorIdx []int, totalLines, before, after int) []window {
	var ws []window
	for _, i := range errorIdx {
		start := i - before
		if start < 0 {
			start = 0
		}
		end := i + after
		if end > totalLines-1 {
			end = totalLines - 1
		}
		if n := len(ws); n > 0 && start <= ws[n-1].end+1 {
			if end > ws[n-1].end {
				ws[n-1].end = end
			}
			continue
		}
		ws = append(ws, window{start, end})
	}
	return ws
}

// renderWindows writes the error windows under a line budget, marking the
// gaps between them. Returns the rendered text and the number of lines kept.
func renderWindows(lines []string, ws []window, budget int) (string, int) {
	var b strings.Builder
	kept := 0
	prevEnd := -1

	for _, w := range ws {
		if kept >= budget {
			break
		}
		if prevEnd >= 0 && w.start > prevEnd+1 {
			fmt.Fprintf(&b, "... (%d lines)\n", w.start-prevEnd-1)
		}
		end := w.end
		if n := end - w.start + 1; kept+n > budget {
			end = w.start + (budget - kept) - 1
		}
		b.WriteString(strings.Join(lines[w.start:end+1], "\n"))
		b.WriteString("\n")
		kept += end - w.start + 1
		prevEnd = end
	}

	return b.String(), kept
}

// compressWithErrors keeps the head, the error blocks with their context,
// and the tail. Used when output is too large to pass through whole.
func compressWithErrors(lines []string, errorIdx []int, logPath string) string {
	errorCount := len(errorIdx)
	if len(errorIdx) > MaxErrorLines {
		errorIdx = errorIdx[:MaxErrorLines]
	}

	ws := mergeWindows(errorIdx, len(lines), ErrCtxBefore, ErrCtxAfter)
	body, kept := renderWindows(lines, ws, MaxErrorBlockLines)

	omitted := len(lines) - HeadLines - TailLines - kept
	if omitted < 0 {
		omitted = 0
	}

	var b strings.Builder
	b.WriteString(strings.Join(lines[:HeadLines], "\n"))
	b.WriteString("\n\n")
	b.WriteString(body)
	fmt.Fprintf(&b, "\n%s\n\n", elision(omitted, errorCount, logPath))
	b.WriteString(strings.Join(lines[len(lines)-TailLines:], "\n"))

	return b.String()
}

// compressClean keeps the head and tail of long output with no errors.
func compressClean(lines []string, logPath string) string {
	omitted := len(lines) - HeadLines - TailLines

	var b strings.Builder
	b.WriteString(strings.Join(lines[:HeadLines], "\n"))
	fmt.Fprintf(&b, "\n\n%s\n\n", elision(omitted, 0, logPath))
	b.WriteString(strings.Join(lines[len(lines)-TailLines:], "\n"))

	return b.String()
}

// elision renders the marker that replaces the removed lines. It names a
// runnable recovery command rather than a bare path: reading the log with
// cat puts every removed line back into the context window, which costs
// more than never having filtered.
func elision(omitted, errorCount int, logPath string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "... (%d lines filtered", omitted)
	if errorCount > 0 {
		fmt.Fprintf(&b, " — %d errors detected", errorCount)
	}
	if logPath != "" {
		fmt.Fprintf(&b, " — recall: trimout recall %s", logPath)
	}
	b.WriteString(")")
	return b.String()
}
