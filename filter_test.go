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
	if strings.Contains(result, "recall") {
		t.Error("should not suggest recall when there is no log path")
	}
}

func TestNeverWorse(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		filtered string
		want     string
	}{
		{"keeps filtered when smaller", strings.Repeat("a", 400), "ok", "ok"},
		{"falls back when filtered bigger", "ab", "a much longer form", "ab"},
		{"tie keeps raw", "abcd", "wxyz", "abcd"},
		{"empty raw returns raw", "", "0 matches", ""},
		{"empty filtered returns filtered", "data", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := neverWorse(tt.raw, tt.filtered); got != tt.want {
				t.Errorf("neverWorse(%q, %q) = %q, want %q", tt.raw, tt.filtered, got, tt.want)
			}
		})
	}
}

// Filtering must never cost more bytes than not filtering. Short repetitive
// output with a long log path is the case that breaks a naive head+tail.
func TestFilterNeverExceedsRawBytes(t *testing.T) {
	longPath := "/tmp/trimout-data/logs/" + strings.Repeat("x", 80) + ".log"
	for _, n := range []int{1, 29, 30, 31, 32, 40} {
		input := strings.Repeat("x\n", n)
		result := captureFilterOutput(t, input, longPath, "test")
		if len(result) > len(input) {
			t.Errorf("n=%d: filtered %d bytes > raw %d bytes", n, len(result), len(input))
		}
	}
}

// A failure block is multi-line: the line matching the error pattern is
// often just a header, and the lines that explain the failure carry no
// error keyword of their own.
func TestFilterKeepsErrorContext(t *testing.T) {
	var b strings.Builder
	b.WriteString(filler(300))
	b.WriteString("\n  Failed MyApp.Tests.CalcTest.Divide [12 ms]\n")
	b.WriteString("  Error Message:\n")
	b.WriteString("   Assert.Equal() Failure: Values differ\n")
	b.WriteString("   Expected: 42\n")
	b.WriteString("   Actual:   0\n")
	b.WriteString("  Stack Trace:\n")
	b.WriteString("     at MyApp.Calc.Divide() in /src/Calc.cs:line 18\n")
	b.WriteString(filler(300))
	b.WriteString("\n")

	result := captureFilterOutput(t, b.String(), "/tmp/test.log", "test")

	for _, want := range []string{
		"Failed MyApp.Tests.CalcTest.Divide",
		"Expected: 42",
		"Actual:   0",
		"Calc.cs:line 18",
	} {
		if !strings.Contains(result, want) {
			t.Errorf("filtered output lost %q\n--- got ---\n%s", want, result)
		}
	}
}

// The error section must stay bounded when a run fails in many places.
func TestFilterErrorContextRespectsBudget(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 200; i++ {
		fmt.Fprintf(&b, "error: failure number %d\n", i)
		b.WriteString(filler(3))
		b.WriteString("\n")
	}

	result := captureFilterOutput(t, b.String(), "/tmp/test.log", "test")
	lines := countOutputLines(result)
	max := HeadLines + TailLines + defaultBudget.MaxSectionLines + 20
	if lines > max {
		t.Errorf("error section unbounded: %d lines > %d", lines, max)
	}
	// Bounded is not enough: dropping content silently is what makes an
	// agent stop looking. Whichever form the drop took — whole blocks over
	// the section budget, or a single merged block over the per-block cap —
	// it has to be stated.
	if !strings.Contains(result, "more error blocks not shown") &&
		!strings.Contains(result, "lines in this block") {
		t.Errorf("dropped error content was not reported:\n%s", result)
	}
}

// The elision marker must name a runnable recovery command, not a bare
// path — reading the log with cat defeats the filtering.
func TestFilterElisionNamesRecall(t *testing.T) {
	result := captureFilterOutput(t, filler(100)+"\n", "/tmp/test.log", "test")
	// The binary name is resolved via selfPath(), so assert on the shape:
	// a runnable "<binary> recall <log>", not a bare path.
	if !strings.Contains(result, "recall /tmp/test.log") {
		t.Errorf("elision marker lacks recall command:\n%s", result)
	}
	if !strings.Contains(result, selfPath()+" recall") {
		t.Errorf("recall command is not an absolute path (breaks when PATH differs):\n%s", result)
	}
}

func TestMergeWindows(t *testing.T) {
	tests := []struct {
		name   string
		idx    []int
		total  int
		before int
		after  int
		want   []window
	}{
		{"single", []int{10}, 100, 2, 5, []window{{8, 15}}},
		{"merges overlapping", []int{10, 12}, 100, 2, 5, []window{{8, 17}}},
		{"merges touching", []int{10, 18}, 100, 2, 5, []window{{8, 23}}},
		{"keeps distant separate", []int{10, 50}, 100, 2, 5, []window{{8, 15}, {48, 55}}},
		{"clamps to bounds", []int{0, 99}, 100, 2, 5, []window{{0, 5}, {97, 99}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeWindows(tt.idx, tt.total, tt.before, tt.after)
			if len(got) != len(tt.want) {
				t.Fatalf("mergeWindows() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("window %d = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}
