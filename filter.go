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

	// Split into lines — match bash behavior: echo "x" | wc -l = 1
	lines := strings.Split(input, "\n")
	// Trim trailing empty line from final newline (strings.Split artifact)
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	totalLines := len(lines)

	// Short output: pass through unchanged
	if totalLines <= Threshold {
		fmt.Fprintln(os.Stdout, input)
		return
	}

	// Check for errors/warnings/failures
	errorCount := 0
	for _, line := range lines {
		if isErrorLine(line) {
			errorCount++
		}
	}

	if errorCount > 0 {
		if totalLines <= MaxPassthrough {
			// Small enough to pass through entirely
			fmt.Fprintln(os.Stdout, input)
		} else {
			// Too large — show errors + head/tail + pointer
			var errorLines []string
			for _, line := range lines {
				if isErrorLine(line) {
					errorLines = append(errorLines, line)
					if len(errorLines) >= MaxErrorLines {
						break
					}
				}
			}

			headBlock := strings.Join(lines[:HeadLines], "\n")
			tailBlock := strings.Join(lines[totalLines-TailLines:], "\n")
			omitted := totalLines - HeadLines - TailLines

			fmt.Fprintln(os.Stdout, headBlock)
			fmt.Fprintf(os.Stdout, "\n%s\n", strings.Join(errorLines, "\n"))
			if logPath != "" {
				fmt.Fprintf(os.Stdout, "\n... (%d lines filtered — %d errors detected — full: %s)\n\n", omitted, errorCount, logPath)
			} else {
				fmt.Fprintf(os.Stdout, "\n... (%d lines filtered — %d errors detected)\n\n", omitted, errorCount)
			}
			fmt.Fprintln(os.Stdout, tailBlock)
		}
		return
	}

	// Clean long output: compress to head + tail + log pointer
	headBlock := strings.Join(lines[:HeadLines], "\n")
	tailBlock := strings.Join(lines[totalLines-TailLines:], "\n")
	omitted := totalLines - HeadLines - TailLines

	fmt.Fprintln(os.Stdout, headBlock)
	if logPath != "" {
		fmt.Fprintf(os.Stdout, "\n... (%d lines filtered — full: %s)\n\n", omitted, logPath)
	} else {
		fmt.Fprintf(os.Stdout, "\n... (%d lines filtered)\n\n", omitted)
	}
	fmt.Fprintln(os.Stdout, tailBlock)
}
