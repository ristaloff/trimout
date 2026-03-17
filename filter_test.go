package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
)

// captureFilterOutput runs runFilter with the given stdin and captures stdout.
func captureFilterOutput(t *testing.T, input, logPath, sessionID string) string {
	t.Helper()

	oldStdin := os.Stdin
	oldStdout := os.Stdout
	defer func() {
		os.Stdin = oldStdin
		os.Stdout = oldStdout
	}()

	r, w, _ := os.Pipe()
	os.Stdin = r
	go func() {
		io.WriteString(w, input)
		w.Close()
	}()

	outR, outW, _ := os.Pipe()
	os.Stdout = outW

	runFilter(logPath, sessionID)
	outW.Close()

	var buf bytes.Buffer
	io.Copy(&buf, outR)
	return buf.String()
}

func filler(n int) string {
	var lines []string
	for i := 1; i <= n; i++ {
		lines = append(lines, fmt.Sprintf("filler line %d", i))
	}
	return strings.Join(lines, "\n")
}

func countOutputLines(s string) int {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

func TestFilterShortPassthrough(t *testing.T) {
	input := "Build succeeded.\n0 errors\n"
	result := captureFilterOutput(t, input, "/tmp/test.log", "test")
	lines := countOutputLines(result)
	if lines > 3 {
		t.Errorf("short output: got %d lines, expected ≤3", lines)
	}
}

func TestFilterCleanLongCompressed(t *testing.T) {
	input := filler(50) + "\nBuild succeeded.\n"
	result := captureFilterOutput(t, input, "/tmp/test.log", "test")
	if !strings.Contains(result, "lines filtered") {
		t.Error("clean long output not compressed — no filter marker found")
	}
}

func TestFilterErrorsSmallPassthrough(t *testing.T) {
	input := filler(40) + "\nerror: broken\n"
	result := captureFilterOutput(t, input, "/tmp/test.log", "test")
	lines := countOutputLines(result)
	if lines != 41 {
		t.Errorf("errors <500: got %d lines, expected 41", lines)
	}
}

func TestFilterErrorsLargeCapped(t *testing.T) {
	input := filler(600) + "\nerror: broken\nFAILED\n"
	result := captureFilterOutput(t, input, "/tmp/test.log", "test")
	lines := countOutputLines(result)
	if lines >= 100 {
		t.Errorf("errors >500: got %d lines, expected <100", lines)
	}
}

func TestFilterStandaloneFAILTab(t *testing.T) {
	input := filler(40) + "\nFAIL\tpkg/broken\t0.01s\n"
	result := captureFilterOutput(t, input, "/tmp/test.log", "test")
	lines := countOutputLines(result)
	if lines != 41 {
		t.Errorf("FAIL\\t: got %d lines, expected 41", lines)
	}
}

func TestFilterZeroErrorNoFalsePositive(t *testing.T) {
	input := filler(50) + "\n0 Error(s)\n"
	result := captureFilterOutput(t, input, "/tmp/test.log", "test")
	if !strings.Contains(result, "lines filtered") {
		t.Error("'0 Error(s)' was treated as error — should compress")
	}
}

func TestFilterFAILEDDetected(t *testing.T) {
	input := filler(40) + "\nFAILED tests/test_auth.py::test_login\n"
	result := captureFilterOutput(t, input, "/tmp/test.log", "test")
	lines := countOutputLines(result)
	if lines != 41 {
		t.Errorf("FAILED: got %d lines, expected 41", lines)
	}
}

func TestFilterExceptionDetected(t *testing.T) {
	input := filler(40) + "\njava.lang.NullPointerException: oops\n"
	result := captureFilterOutput(t, input, "/tmp/test.log", "test")
	lines := countOutputLines(result)
	if lines != 41 {
		t.Errorf("exception: got %d lines, expected 41", lines)
	}
}

func TestFilterFatalDetected(t *testing.T) {
	input := filler(40) + "\nfatal error: something terrible\n"
	result := captureFilterOutput(t, input, "/tmp/test.log", "test")
	lines := countOutputLines(result)
	if lines != 41 {
		t.Errorf("fatal: got %d lines, expected 41", lines)
	}
}

func TestFilterEmptyInput(t *testing.T) {
	result := captureFilterOutput(t, "", "/tmp/test.log", "test")
	// Should not crash — any output is fine
	_ = result
}

func TestFilterLogPointerIncludesPath(t *testing.T) {
	input := filler(50) + "\nBuild succeeded.\n"
	logPath := "/tmp/test-special.log"
	result := captureFilterOutput(t, input, logPath, "test")
	if !strings.Contains(result, logPath) {
		t.Error("log pointer does not include path")
	}
}

func TestFilterExactlyAtThreshold(t *testing.T) {
	input := filler(30) + "\n"
	result := captureFilterOutput(t, input, "/tmp/test.log", "test")
	if strings.Contains(result, "lines filtered") {
		t.Error("30 lines should passthrough, not compress")
	}
}

func TestFilterJustOverThreshold(t *testing.T) {
	input := filler(31) + "\n"
	result := captureFilterOutput(t, input, "/tmp/test.log", "test")
	if !strings.Contains(result, "lines filtered") {
		t.Error("31 lines should compress, not passthrough")
	}
}

func TestFilterMaxPassthroughBoundary(t *testing.T) {
	// 499 filler + 1 error = 500 lines → passthrough
	input := filler(499) + "\nerror: x\n"
	result := captureFilterOutput(t, input, "/tmp/test.log", "test")
	lines := countOutputLines(result)
	if lines != 500 {
		t.Errorf("500 lines with error: got %d, expected 500", lines)
	}
}

func TestFilterOverMaxPassthroughCapped(t *testing.T) {
	// 500 filler + 1 error = 501 lines → capped
	input := filler(500) + "\nerror: x\n"
	result := captureFilterOutput(t, input, "/tmp/test.log", "test")
	lines := countOutputLines(result)
	if lines >= 100 {
		t.Errorf("501 lines with error: got %d, expected <100", lines)
	}
}

func TestFilterCarriageReturnsStripped(t *testing.T) {
	input := "line1\r\nline2\r\n"
	result := captureFilterOutput(t, input, "/tmp/test.log", "test")
	if strings.Contains(result, "\r") {
		t.Error("carriage returns not stripped")
	}
}

func TestFilterNoLogPath(t *testing.T) {
	input := filler(50) + "\n"
	result := captureFilterOutput(t, input, "", "test")
	if !strings.Contains(result, "lines filtered") {
		t.Error("should still compress without log path")
	}
	if strings.Contains(result, "full:") {
		t.Error("should not show 'full:' when no log path")
	}
}
