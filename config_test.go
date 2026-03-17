package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDataDir(t *testing.T) {
	got := DataDir()
	want := filepath.Join(os.TempDir(), "trimout-data")
	if got != want {
		t.Errorf("DataDir() = %q, want %q", got, want)
	}
}

func TestLogDir(t *testing.T) {
	got := LogDir()
	want := filepath.Join(os.TempDir(), "trimout-data", "logs")
	if got != want {
		t.Errorf("LogDir() = %q, want %q", got, want)
	}
}

func TestMetricsDir(t *testing.T) {
	got := MetricsDir()
	want := filepath.Join(os.TempDir(), "trimout-data", "metrics")
	if got != want {
		t.Errorf("MetricsDir() = %q, want %q", got, want)
	}
}

