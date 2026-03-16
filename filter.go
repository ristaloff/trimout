package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type filterStats struct {
	Timestamp     string `json:"ts"`
	Session       string `json:"session"`
	Log           string `json:"log"`
	OriginalLines int    `json:"original_lines"`
	FilteredLines int    `json:"filtered_lines"`
	Action        string `json:"action"`
}

func runFilter(logPath, sessionID, metricsDir string) {
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
		logFilterStats(metricsDir, logPath, sessionID, totalLines, totalLines, "short")
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
			logFilterStats(metricsDir, logPath, sessionID, totalLines, totalLines, "errors")
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

			shownErrors := len(errorLines)
			logFilterStats(metricsDir, logPath, sessionID, totalLines, HeadLines+TailLines+shownErrors+3, "errors_capped")
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

	filteredLines := HeadLines + TailLines + 3
	logFilterStats(metricsDir, logPath, sessionID, totalLines, filteredLines, "compressed")
}

func logFilterStats(metricsDir, logPath, sessionID string, originalLines, filteredLines int, action string) {
	var metricsPath string
	if metricsDir != "" {
		metricsPath = filepath.Join(metricsDir, "filter-stats.jsonl")
	} else {
		metricsPath = FilterStatsPath()
	}
	os.MkdirAll(filepath.Dir(metricsPath), 0o755)

	entry := filterStats{
		Timestamp:     time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		Session:       sessionID,
		Log:           logPath,
		OriginalLines: originalLines,
		FilteredLines: filteredLines,
		Action:        action,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return
	}

	f, err := os.OpenFile(metricsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	f.Write(append(data, '\n'))
}
