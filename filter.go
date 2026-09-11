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

// renderBlocks writes error blocks under a total line budget, marking the
// gaps between them. A block that would not fit is dropped whole rather than
// cut, because half a failure cannot be acted on but still costs its lines.
// Returns the text, the lines consumed, and how many blocks were dropped.
func renderBlocks(lines []string, ws []window, b errorBudget) (string, int, int) {
	var out strings.Builder
	kept := 0
	prevEnd := -1

	for i, w := range ws {
		size := w.end - w.start + 1
		capped := size > b.MaxLinesPerBlock
		if capped {
			size = b.MaxLinesPerBlock
		}
		// Always emit the first block: a single failure bigger than the
		// whole budget should still be shown, truncated, over showing none.
		if kept > 0 && kept+size > b.MaxSectionLines {
			return out.String(), kept, len(ws) - i
		}
		if prevEnd >= 0 && w.start > prevEnd+1 {
			fmt.Fprintf(&out, "... (%d lines)\n", w.start-prevEnd-1)
		}
		end := w.start + size - 1
		out.WriteString(strings.Join(lines[w.start:end+1], "\n"))
		out.WriteString("\n")
		if capped {
			fmt.Fprintf(&out, "... (+%d lines in this block)\n", w.end-end)
		}
		kept += size
		prevEnd = w.end
	}

	return out.String(), kept, 0
}

// compressWithErrors keeps the head, the error blocks with their context,
// and the tail. Used when output is too large to pass through whole.
func compressWithErrors(lines []string, errorIdx []int, logPath string) string {
	budget := defaultBudget
	total := len(lines)
	errorCount := len(errorIdx)

	// The head and tail blocks are emitted verbatim, so error windows cover
	// only the region between them. Without this clip, an error in the first
	// or last few lines prints twice and the omitted count double-counts it.
	midStart, midEnd := HeadLines, total-TailLines-1

	var blocks []window
	for _, w := range mergeWindows(errorIdx, total, budget.CtxBefore, budget.CtxAfter) {
		if w.end < midStart || w.start > midEnd {
			continue // already visible in the head or tail
		}
		if w.start < midStart {
			w.start = midStart
		}
		if w.end > midEnd {
			w.end = midEnd
		}
		blocks = append(blocks, w)
	}

	body, kept, dropped := renderBlocks(lines, blocks, budget)
	omitted := total - HeadLines - TailLines - kept
	if omitted < 0 {
		omitted = 0
	}

	var b strings.Builder
	b.WriteString(strings.Join(lines[:HeadLines], "\n"))
	b.WriteString("\n\n")
	if len(blocks) > 0 && blocks[0].start > midStart {
		fmt.Fprintf(&b, "... (%d lines)\n", blocks[0].start-midStart)
	}
	b.WriteString(body)
	fmt.Fprintf(&b, "\n%s\n\n", errorElision(omitted, errorCount, dropped, logPath))
	b.WriteString(strings.Join(lines[total-TailLines:], "\n"))

	return b.String()
}

// compressClean keeps the head and tail of long output with no errors.
func compressClean(lines []string, logPath string) string {
	omitted := len(lines) - HeadLines - TailLines

	var b strings.Builder
	b.WriteString(strings.Join(lines[:HeadLines], "\n"))
	fmt.Fprintf(&b, "\n\n%s\n\n", errorElision(omitted, 0, 0, logPath))
	b.WriteString(strings.Join(lines[len(lines)-TailLines:], "\n"))

	return b.String()
}

// errorElision is the summary for a compressed failing run. It must name
// the number of error blocks dropped: an agent shown a subset with no such
// signal concludes it has every failure and stops looking.
func errorElision(omitted, errorCount, dropped int, logPath string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "... (%d lines filtered", omitted)
	if errorCount > 0 {
		fmt.Fprintf(&b, " — %d errors detected", errorCount)
	}
	if dropped > 0 {
		fmt.Fprintf(&b, ", +%d more error blocks not shown", dropped)
	}
	if logPath != "" {
		fmt.Fprintf(&b, " — recall: %s recall %s", selfPath(), logPath)
	}
	b.WriteString(")")
	return b.String()
}
