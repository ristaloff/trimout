package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHomeDefault(t *testing.T) {
	t.Setenv("TRIMOUT_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")
	got := Home()
	want := filepath.Join(os.Getenv("HOME"), ".local", "state", "trimout")
	if got != want {
		t.Errorf("Home() = %q, want %q", got, want)
	}
}

func TestHomeXDGStateHome(t *testing.T) {
	t.Setenv("TRIMOUT_HOME", "")
	t.Setenv("XDG_STATE_HOME", "/tmp/xdg-state")
	got := Home()
	if got != "/tmp/xdg-state/trimout" {
		t.Errorf("Home() = %q, want /tmp/xdg-state/trimout", got)
	}
}

func TestHomeEnvOverride(t *testing.T) {
	t.Setenv("TRIMOUT_HOME", "/tmp/custom-filter")
	got := Home()
	if got != "/tmp/custom-filter" {
		t.Errorf("Home() = %q, want /tmp/custom-filter", got)
	}
}

func TestLogDir(t *testing.T) {
	t.Setenv("TRIMOUT_HOME", "/tmp/cf")
	got := LogDir()
	if got != "/tmp/cf/logs" {
		t.Errorf("LogDir() = %q, want /tmp/cf/logs", got)
	}
}

func TestMetricsDir(t *testing.T) {
	t.Setenv("TRIMOUT_HOME", "/tmp/cf")
	got := MetricsDir()
	if got != "/tmp/cf/metrics" {
		t.Errorf("MetricsDir() = %q, want /tmp/cf/metrics", got)
	}
}

func TestFilterStatsPath(t *testing.T) {
	t.Setenv("TRIMOUT_HOME", "/tmp/cf")
	got := FilterStatsPath()
	want := filepath.Join("/tmp/cf", "metrics", "filter-stats.jsonl")
	if got != want {
		t.Errorf("FilterStatsPath() = %q, want %q", got, want)
	}
}
