package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// metricsPath returns the path where runMetrics writes.
func metricsPath() string {
	return filepath.Join(MetricsDir(), "tool-output.jsonl")
}

// captureMetricsOutput runs runMetrics with the given stdin.
// Cleans the metrics file before running to isolate test output.
func captureMetricsOutput(t *testing.T, input string) {
	t.Helper()

	// Ensure metrics dir exists, truncate file for isolation
	os.MkdirAll(MetricsDir(), 0o755)
	os.Remove(metricsPath())

	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }()

	r, w, _ := os.Pipe()
	os.Stdin = r
	go func() {
		io.WriteString(w, input)
		w.Close()
	}()

	runMetrics()
}

func makeMetricsInput(cmd, stdout string, exitCode int) string {
	data := map[string]any{
		"tool_name":  "Bash",
		"tool_input": map[string]string{"command": cmd},
		"tool_response": map[string]any{
			"stdout":   stdout,
			"stderr":   "",
			"exitCode": exitCode,
			"duration": 100,
		},
		"session_id": "test",
	}
	b, _ := json.Marshal(data)
	return string(b)
}

func TestMetricsFileCreated(t *testing.T) {
	captureMetricsOutput(t, makeMetricsInput("echo hello", "hello", 0))

	if _, err := os.Stat(metricsPath()); os.IsNotExist(err) {
		t.Error("metrics file not created")
	}
}

func TestMetricsRewrittenCommandStripped(t *testing.T) {
	rewritten := "( dotnet test ) 2>&1 | tee /tmp/x.log | /usr/local/bin/trimout filter --log /tmp/x.log --session sess; exit ${PIPESTATUS[0]}"
	captureMetricsOutput(t, makeMetricsInput(rewritten, "ok", 0))

	data, err := os.ReadFile(metricsPath())
	if err != nil {
		t.Fatal(err)
	}

	var entry metricsEntry
	json.Unmarshal(bytes.TrimSpace(data), &entry)
	if entry.Command != "dotnet test" {
		t.Errorf("command not stripped: got %q, want %q", entry.Command, "dotnet test")
	}
}

func TestMetricsCommandPatternClassification(t *testing.T) {
	captureMetricsOutput(t, makeMetricsInput("dotnet build --no-restore", "ok", 0))

	data, err := os.ReadFile(metricsPath())
	if err != nil {
		t.Fatal(err)
	}

	var entry metricsEntry
	json.Unmarshal(bytes.TrimSpace(data), &entry)
	if entry.CmdPattern != "dotnet build" {
		t.Errorf("pattern = %q, want %q", entry.CmdPattern, "dotnet build")
	}
}

func TestMetricsWrongToolNameSkipped(t *testing.T) {
	captureMetricsOutput(t, `{"tool_name":"Edit","tool_input":{"file_path":"/tmp/x"},"tool_response":{},"session_id":"t"}`)

	if _, err := os.Stat(metricsPath()); !os.IsNotExist(err) {
		t.Error("wrong tool name should not write metrics")
	}
}

func TestMetricsMalformedJSON(t *testing.T) {
	captureMetricsOutput(t, "{ broken }")

	if _, err := os.Stat(metricsPath()); !os.IsNotExist(err) {
		t.Error("malformed JSON should not write metrics")
	}
}

func TestMetricsOutputIsValidJSONL(t *testing.T) {
	captureMetricsOutput(t, makeMetricsInput("git status", "on branch main", 0))

	data, err := os.ReadFile(metricsPath())
	if err != nil {
		t.Fatal(err)
	}

	var v any
	if err := json.Unmarshal(bytes.TrimSpace(data), &v); err != nil {
		t.Errorf("metrics output is not valid JSON: %v", err)
	}
}

func TestMetricsExitCodeFromSidecar(t *testing.T) {
	// Create a sidecar file simulating a failed build
	sidecar := filepath.Join(t.TempDir(), "test.log.exit")
	os.WriteFile(sidecar, []byte("1"), 0o644)
	logPath := strings.TrimSuffix(sidecar, ".exit")

	// Simulate a rewritten command pointing to our sidecar
	rewritten := "( dotnet test ) 2>&1 | tee " + logPath + " | /usr/local/bin/trimout filter --log " + logPath + " --session sess; _ec=${PIPESTATUS[0]}; printf '%d' $_ec > " + sidecar + "; exit $_ec"
	captureMetricsOutput(t, makeMetricsInput(rewritten, "error: build failed", 1))

	data, err := os.ReadFile(metricsPath())
	if err != nil {
		t.Fatal(err)
	}

	var entry metricsEntry
	json.Unmarshal(bytes.TrimSpace(data), &entry)
	if entry.ExitCode != 1 {
		t.Errorf("exit_code = %d, want 1", entry.ExitCode)
	}

	// Sidecar should be cleaned up
	if _, err := os.Stat(sidecar); !os.IsNotExist(err) {
		t.Error("sidecar file not cleaned up")
	}
}

func TestMetricsExitCodeFallbackWhenNoSidecar(t *testing.T) {
	// Rewritten command but no sidecar file exists
	rewritten := "( dotnet test ) 2>&1 | tee /tmp/nonexistent.log | /usr/local/bin/trimout filter --log /tmp/nonexistent.log --session sess; _ec=${PIPESTATUS[0]}; printf '%d' $_ec > /tmp/nonexistent.log.exit; exit $_ec"
	captureMetricsOutput(t, makeMetricsInput(rewritten, "ok", 0))

	data, err := os.ReadFile(metricsPath())
	if err != nil {
		t.Fatal(err)
	}

	var entry metricsEntry
	json.Unmarshal(bytes.TrimSpace(data), &entry)
	if entry.ExitCode != 0 {
		t.Errorf("exit_code = %d, want 0 (fallback)", entry.ExitCode)
	}
}

func TestClassifyCommand(t *testing.T) {
	tests := []struct {
		cmd  string
		want string
	}{
		{"dotnet build --no-restore", "dotnet build"},
		{"git status", "git status"},
		{"echo hello", "echo"},
		{"cd /src && dotnet test --no-build", "dotnet test"},
		{"sudo dotnet build", "dotnet build"},
		{"npm run build", "npm run"},
		{"cargo test -- --test-threads=1", "cargo test"},
		{"", "empty"},
	}

	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			got := classifyCommand(tt.cmd)
			if got != tt.want {
				t.Errorf("classifyCommand(%q) = %q, want %q", tt.cmd, got, tt.want)
			}
		})
	}
}
