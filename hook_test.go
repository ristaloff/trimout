package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
)

// captureHookOutput runs runHook with the given stdin and captures stdout.
func captureHookOutput(t *testing.T, input string) string {
	t.Helper()
	t.Setenv("TRIMOUT_HOME", t.TempDir())

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

	runHook()
	outW.Close()

	var buf bytes.Buffer
	io.Copy(&buf, outR)
	return buf.String()
}

func makeHookInput(cmd, sessionID string) string {
	data := map[string]any{
		"tool_name":  "Bash",
		"tool_input": map[string]string{"command": cmd},
		"session_id": sessionID,
	}
	b, _ := json.Marshal(data)
	return string(b)
}

func TestHookAllowlistedRewritten(t *testing.T) {
	result := captureHookOutput(t, makeHookInput("dotnet build", "test-123"))
	if !strings.Contains(result, "tee") {
		t.Error("allowlisted command not rewritten — expected 'tee' in output")
	}
}

func TestHookNonAllowlistedSkipped(t *testing.T) {
	result := captureHookOutput(t, makeHookInput("git status", "test"))
	if result != "" {
		t.Errorf("non-allowlisted command produced output: %s", result)
	}
}

func TestHookNofilterOptOut(t *testing.T) {
	result := captureHookOutput(t, makeHookInput("dotnet build # nofilter", "test"))
	if result != "" {
		t.Errorf("# nofilter command produced output: %s", result)
	}
}

func TestHookPipedCommand(t *testing.T) {
	result := captureHookOutput(t, makeHookInput("dotnet test | grep FAIL", "test"))
	if !strings.Contains(result, "tee") {
		t.Error("piped command not rewritten")
	}
}

func TestHookChainCommand(t *testing.T) {
	result := captureHookOutput(t, makeHookInput("dotnet build && dotnet test", "test"))
	if !strings.Contains(result, "tee") {
		t.Error("chain command not rewritten")
	}
}

func TestHookSubshellWrapFormat(t *testing.T) {
	result := captureHookOutput(t, makeHookInput("dotnet build && dotnet test", "test"))
	var out hookOutput
	if err := json.Unmarshal([]byte(result), &out); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	cmd := out.HookSpecificOutput.UpdatedInput.Command
	if !strings.HasPrefix(cmd, "( ") {
		t.Errorf("rewrite doesn't start with '( ': %s", cmd)
	}
	if !strings.Contains(cmd, "( dotnet build && dotnet test )") {
		t.Errorf("rewrite missing subshell-wrapped command: %s", cmd)
	}
}

func TestHookSessionIDInRewrite(t *testing.T) {
	result := captureHookOutput(t, makeHookInput("npm test", "sess-xyz"))
	if !strings.Contains(result, "sess-xyz") {
		t.Error("session ID not found in rewritten command")
	}
}

func TestHookOutputStructure(t *testing.T) {
	result := captureHookOutput(t, makeHookInput("cargo test", "test"))
	var out hookOutput
	if err := json.Unmarshal([]byte(result), &out); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if out.HookSpecificOutput.HookEventName != "PreToolUse" {
		t.Errorf("hookEventName = %q, want PreToolUse", out.HookSpecificOutput.HookEventName)
	}
	if out.HookSpecificOutput.PermissionDecision != "allow" {
		t.Errorf("permissionDecision = %q, want allow", out.HookSpecificOutput.PermissionDecision)
	}
	cmd := out.HookSpecificOutput.UpdatedInput.Command
	if !strings.Contains(cmd, "PIPESTATUS") {
		t.Error("rewrite missing PIPESTATUS")
	}
	if !strings.Contains(cmd, ".exit") {
		t.Error("rewrite missing exit code sidecar")
	}
}

func TestHookOutputIsValidJSON(t *testing.T) {
	result := captureHookOutput(t, makeHookInput("dotnet build", "test"))
	var v any
	if err := json.Unmarshal([]byte(result), &v); err != nil {
		t.Errorf("output is not valid JSON: %v\nOutput: %s", err, result)
	}
}

func TestHookMalformedJSON(t *testing.T) {
	result := captureHookOutput(t, "{ invalid json !!! }")
	if result != "" {
		t.Errorf("malformed JSON produced output: %s", result)
	}
}

func TestHookEmptyStdin(t *testing.T) {
	result := captureHookOutput(t, "")
	if result != "" {
		t.Errorf("empty stdin produced output: %s", result)
	}
}

func TestHookMissingToolInput(t *testing.T) {
	result := captureHookOutput(t, `{"tool_name":"Bash","session_id":"test"}`)
	if result != "" {
		t.Errorf("missing tool_input produced output: %s", result)
	}
}

func TestHookWrongToolName(t *testing.T) {
	input := `{"tool_name":"Edit","tool_input":{"file_path":"/tmp/x"},"session_id":"t"}`
	result := captureHookOutput(t, input)
	// Edit tool doesn't have a command field, so it should produce empty command → skip
	if result != "" {
		t.Errorf("wrong tool name produced output: %s", result)
	}
}

func TestHookDockerComposeUpExcluded(t *testing.T) {
	result := captureHookOutput(t, makeHookInput("docker compose up -d", "test"))
	if result != "" {
		t.Errorf("docker compose up should be excluded: %s", result)
	}
}

func TestHookCargoRunExcluded(t *testing.T) {
	result := captureHookOutput(t, makeHookInput("cargo run --release", "test"))
	if result != "" {
		t.Errorf("cargo run should be excluded: %s", result)
	}
}

func TestHookSpecialCharacters(t *testing.T) {
	cases := []string{
		`dotnet test --filter "Name=Foo"`,
		`dotnet test --filter 'FullyQualifiedName~Foo'`,
		`dotnet build "/path/with spaces/project.csproj"`,
	}
	for _, cmd := range cases {
		t.Run(cmd, func(t *testing.T) {
			result := captureHookOutput(t, makeHookInput(cmd, "test"))
			var v any
			if err := json.Unmarshal([]byte(result), &v); err != nil {
				t.Errorf("invalid JSON for special char command: %v\nOutput: %s", err, result)
			}
		})
	}
}

func TestHookMultipleEcosystems(t *testing.T) {
	cmds := []string{
		"dotnet test", "npm install", "cargo build", "go test",
		"pytest", "pip install -r req.txt", "make", "gradle build",
		"mvn package", "mypy src",
	}
	for _, cmd := range cmds {
		t.Run(cmd, func(t *testing.T) {
			result := captureHookOutput(t, makeHookInput(cmd, "test"))
			if !strings.Contains(result, "tee") {
				t.Errorf("command %q not rewritten", cmd)
			}
		})
	}
}
