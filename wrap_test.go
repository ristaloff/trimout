package main

import (
	"strings"
	"testing"
)

func TestBuildRewrittenCommand(t *testing.T) {
	got := buildRewrittenCommand("dotnet build", "/tmp/test.log", "/usr/bin/trimout", "sess-1")

	if got == "" {
		t.Fatal("buildRewrittenCommand returned empty string")
	}

	checks := []struct {
		substr string
		desc   string
	}{
		{"( dotnet build )", "subshell wrap"},
		{"tee /tmp/test.log", "tee with log path"},
		{"/usr/bin/trimout filter", "filter subcommand"},
		{"--log /tmp/test.log", "log flag"},
		{"--session sess-1", "session flag"},
		{"PIPESTATUS[0]", "exit code preservation"},
		{"/tmp/test.log.exit", "exit code sidecar"},
	}

	for _, c := range checks {
		if !strings.Contains(got, c.substr) {
			t.Errorf("rewritten command missing %s: %q not in %q", c.desc, c.substr, got)
		}
	}
}

func TestBuildRewrittenCommandSpecialChars(t *testing.T) {
	got := buildRewrittenCommand(`dotnet test --filter "Name=Foo"`, "/tmp/t.log", "trimout", "s")
	if !strings.Contains(got, `dotnet test --filter "Name=Foo"`) {
		t.Errorf("special chars not preserved: %s", got)
	}
}

func TestBuildLogFile(t *testing.T) {
	got := buildLogFile("/tmp/logs")
	if !strings.Contains(got, "/tmp/logs/") {
		t.Errorf("buildLogFile should use given dir: %s", got)
	}
	if !strings.Contains(got, ".log") {
		t.Errorf("buildLogFile should end with .log: %s", got)
	}
}
