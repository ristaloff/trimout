package main

import (
	"os"
	"path/filepath"
)

// DataDir returns the base directory for trimout session data (logs, metrics).
// Always uses /tmp/trimout-data/ — works in any sandbox, no config needed.
// Data is ephemeral (lost on reboot) by design. Persistent metrics
// collection is the responsibility of the calling framework.
func DataDir() string {
	return filepath.Join(os.TempDir(), "trimout-data")
}

// LogDir returns the directory for full output logs.
func LogDir() string {
	return filepath.Join(DataDir(), "logs")
}

// MetricsDir returns the directory for metrics JSONL files.
func MetricsDir() string {
	return filepath.Join(DataDir(), "metrics")
}

