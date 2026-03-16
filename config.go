package main

import (
	"os"
	"path/filepath"
)

// Home returns the base directory for trimout data.
// Uses TRIMOUT_HOME if set, then XDG_STATE_HOME/trimout,
// then falls back to ~/.local/state/trimout/.
func Home() string {
	if v := os.Getenv("TRIMOUT_HOME"); v != "" {
		return v
	}
	stateHome := os.Getenv("XDG_STATE_HOME")
	if stateHome == "" {
		stateHome = filepath.Join(os.Getenv("HOME"), ".local", "state")
	}
	return filepath.Join(stateHome, "trimout")
}

// LogDir returns the directory for full output logs.
func LogDir() string {
	return filepath.Join(Home(), "logs")
}

// MetricsDir returns the directory for metrics JSONL files.
func MetricsDir() string {
	return filepath.Join(Home(), "metrics")
}

// FilterStatsPath returns the path for filter statistics JSONL.
func FilterStatsPath() string {
	return filepath.Join(MetricsDir(), "filter-stats.jsonl")
}
