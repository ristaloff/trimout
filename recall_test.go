package main

import (
	"strings"
	"testing"
)

func logLines(n int) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		b.WriteString("ok line ")
		b.WriteString(strings.Repeat("x", 1))
		b.WriteString("\n")
	}
	return b.String()
}

// Recall exists so that recovering removed lines costs less context than
// never having filtered. An unbounded default would defeat that.
func TestRecallDefaultIsBounded(t *testing.T) {
	input := logLines(2000)
	got := recallView(input, "/tmp/test.log", 0, 0, false)

	if countOutputLines(got) > RecallHeadLines+RecallTailLines+5 {
		t.Errorf("default view unbounded: %d lines", countOutputLines(got))
	}
	if !strings.Contains(got, "--all") {
		t.Error("bounded view must name the escape hatch")
	}
}

// The point of recall is the diagnosis the inline filter had to drop.
func TestRecallSurfacesErrorsWithContext(t *testing.T) {
	var b strings.Builder
	b.WriteString(logLines(500))
	b.WriteString("error: build failed\n")
	b.WriteString("  expected 42\n")
	b.WriteString("  actual 0\n")
	b.WriteString(logLines(500))

	got := recallView(b.String(), "/tmp/test.log", 0, 0, false)

	for _, want := range []string{"error: build failed", "expected 42", "actual 0"} {
		if !strings.Contains(got, want) {
			t.Errorf("recall lost %q", want)
		}
	}
	if countOutputLines(got) > RecallErrorBlockLines+5 {
		t.Errorf("error view unbounded: %d lines", countOutputLines(got))
	}
}

func TestRecallAllIsUnbounded(t *testing.T) {
	input := logLines(2000)
	got := recallView(input, "/tmp/test.log", 0, 0, true)
	if countOutputLines(got) != 2000 {
		t.Errorf("--all returned %d lines, want 2000", countOutputLines(got))
	}
}

func TestRecallHeadAndTail(t *testing.T) {
	input := logLines(100)

	head := recallView(input, "/tmp/test.log", 3, 0, false)
	if n := countOutputLines(strings.SplitN(head, "\n\n", 2)[0]); n != 3 {
		t.Errorf("--head 3 returned %d lines", n)
	}

	tail := recallView(input, "/tmp/test.log", 0, 3, false)
	if n := countOutputLines(strings.SplitN(tail, "\n\n", 2)[0]); n != 3 {
		t.Errorf("--tail 3 returned %d lines", n)
	}

	for _, view := range []string{head, tail} {
		if !strings.Contains(view, "97 lines not shown") {
			t.Errorf("view omits hidden-line count:\n%s", view)
		}
	}
}

// A log shorter than the window needs no elision marker.
func TestRecallShortLogShownWhole(t *testing.T) {
	input := logLines(5)
	got := recallView(input, "/tmp/test.log", 0, 0, false)
	if countOutputLines(got) != 5 {
		t.Errorf("short log returned %d lines, want 5", countOutputLines(got))
	}
	if strings.Contains(got, "not shown") {
		t.Error("short log must not claim hidden lines")
	}
}

// A head/tail request larger than the log must not over-read or claim
// hidden lines that do not exist.
func TestRecallHeadBeyondLogLength(t *testing.T) {
	input := logLines(4)
	got := recallView(input, "/tmp/test.log", 50, 0, false)
	if countOutputLines(got) != 4 {
		t.Errorf("returned %d lines, want 4", countOutputLines(got))
	}
	if strings.Contains(got, "not shown") {
		t.Error("must not claim hidden lines when the whole log fits")
	}
}
