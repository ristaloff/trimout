package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The eval measures the only thing that matters when a build fails: can the
// agent fix the failure from the filtered output alone, without re-running
// the command or reading the log? Fixtures are real output captured from
// real failing runs (go test, dotnet xunit, python unittest, gcc); each
// carries a .facts manifest naming the strings a fix depends on.
//
// Run the sweep that chooses the budget constants with:
//
//	go test -run TestEvalSweep -v

type failure struct {
	name  string
	facts []string
}

type fixture struct {
	name     string
	raw      string
	failures []failure
}

func loadFixtures(t *testing.T) []fixture {
	t.Helper()

	paths, err := filepath.Glob("testdata/fixtures/*.txt")
	if err != nil || len(paths) == 0 {
		t.Fatalf("no fixtures found: %v", err)
	}
	sort.Strings(paths)

	var out []fixture
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		name := strings.TrimSuffix(filepath.Base(p), ".txt")

		f, err := os.Open(filepath.Join("testdata/fixtures", name+".facts"))
		if err != nil {
			t.Fatalf("read facts for %s: %v", name, err)
		}
		var failures []failure
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.Split(line, "|")
			failures = append(failures, failure{name: parts[0], facts: parts})
		}
		f.Close()

		out = append(out, fixture{name: name, raw: string(raw), failures: failures})
	}
	return out
}

// short abbreviates a fixture name for the sweep table.
func short(s string) string {
	if len(s) > 4 {
		return s[:4]
	}
	return s
}

// noise returns filler resembling the progress output a real build emits
// between failures.
func noise(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("  Building project Component.%03d -> bin/Debug/net10.0/Component.%03d.dll", i, i)
	}
	return out
}

// bigLog embeds a fixture's output in enough build noise to exceed
// MaxPassthrough, which is where the error budget starts to bind. copies
// repeats the failure region to model a suite failing in many places.
func bigLog(fx fixture, copies, noisePerCopy int) string {
	var lines []string
	body := strings.Split(strings.TrimRight(fx.raw, "\n"), "\n")
	for i := 0; i < copies; i++ {
		lines = append(lines, noise(noisePerCopy)...)
		lines = append(lines, body...)
	}
	lines = append(lines, noise(noisePerCopy)...)
	return strings.Join(lines, "\n") + "\n"
}

// retained counts how many of a failure's facts survive in the output.
func retained(out string, f failure) int {
	n := 0
	for _, fact := range f.facts {
		if strings.Contains(out, fact) {
			n++
		}
	}
	return n
}

// A failure is diagnosable only when every fact survives — a test name with
// no assertion, or an assertion with no file:line, does not let an agent fix
// anything.
func diagnosable(out string, fx fixture) (ok, total int) {
	for _, f := range fx.failures {
		total++
		if retained(out, f) == len(f.facts) {
			ok++
		}
	}
	return ok, total
}

// TestEvalDiagnosticRetention is the regression guard: every failure in a
// realistically large failing run must stay diagnosable. This is the test
// that a line-budgeted error section fails.
func TestEvalDiagnosticRetention(t *testing.T) {
	for _, fx := range loadFixtures(t) {
		t.Run(fx.name, func(t *testing.T) {
			input := bigLog(fx, 1, 300)
			if len(splitLines(input)) <= MaxPassthrough {
				t.Fatalf("fixture %s did not exceed the passthrough threshold", fx.name)
			}

			out := compress(input, "/tmp/test.log")
			ok, total := diagnosable(out, fx)
			if ok != total {
				var missing []string
				for _, f := range fx.failures {
					if retained(out, f) != len(f.facts) {
						missing = append(missing, f.name)
					}
				}
				t.Errorf("%d/%d failures diagnosable; lost %v\n--- filtered ---\n%s",
					ok, total, missing, out)
			}
		})
	}
}

// TestEvalManyFailures covers the case the budget exists for: a suite that
// fails in many places at once. Every failure need not survive, but the
// output must say how many were dropped — silently showing a subset is how
// an agent concludes it has the whole picture when it does not.
func TestEvalManyFailures(t *testing.T) {
	for _, fx := range loadFixtures(t) {
		t.Run(fx.name, func(t *testing.T) {
			const copies = 12
			input := bigLog(fx, copies, 40)
			out := compress(input, "/tmp/test.log")

			marker := fx.failures[0].facts[len(fx.failures[0].facts)-1]
			shown := strings.Count(out, marker)
			want := copies

			if shown < want && !strings.Contains(out, "more") {
				t.Errorf("showed %d/%d occurrences of %q but never said how many were dropped\n--- filtered ---\n%s",
					shown, want, marker, out)
			}
		})
	}
}

// TestEvalSweep is a tuning tool, not an assertion. It prints how the budget
// knobs trade diagnostic retention against output size on real fixtures, so
// the constants are chosen from data rather than picked.
func TestEvalSweep(t *testing.T) {
	if testing.Short() {
		t.Skip("sweep is a tuning tool")
	}
	fixtures := loadFixtures(t)
	saved := defaultBudget
	defer func() { defaultBudget = saved }()

	t.Log("section-lines x ctx-after -> diagnosable failures per fixture, output lines")
	for _, section := range []int{30, 60, 90, 120, 160, 200} {
		for _, after := range []int{10, 12, 14, 16} {
			defaultBudget = errorBudget{
				MaxSectionLines:  section,
				MaxLinesPerBlock: saved.MaxLinesPerBlock,
				CtxBefore:        saved.CtxBefore,
				CtxAfter:         after,
			}
			var report []string
			totalOK, totalFail, totalLines := 0, 0, 0
			for _, fx := range fixtures {
				input := bigLog(fx, 1, 300)
				out := compress(input, "/tmp/test.log")
				ok, total := diagnosable(out, fx)
				n := countOutputLines(out)
				totalOK += ok
				totalFail += total
				totalLines += n
				report = append(report, fmt.Sprintf("%s %d/%d", short(fx.name), ok, total))
			}
			t.Logf("  section=%3d ctxAfter=%2d -> %s | %d/%d diagnosable, %d lines",
				section, after, strings.Join(report, "  "), totalOK, totalFail, totalLines)
		}
	}
}
