package main

import "testing"

func TestMatchesAllowlist(t *testing.T) {
	tests := []struct {
		cmd  string
		want bool
	}{
		// Allowlisted commands
		{"dotnet build", true},
		{"dotnet build --no-restore", true},
		{"dotnet test", true},
		{"dotnet publish -c Release", true},
		{"dotnet restore", true},
		{"dotnet format", true},
		{"dotnet clean", true},
		{"npm install", true},
		{"npm ci", true},
		{"npm test", true},
		{"npm run build", true},
		{"npx tsc", true},
		{"npx jest", true},
		{"npx vitest", true},
		{"yarn install", true},
		{"yarn build", true},
		{"yarn test", true},
		{"pnpm install", true},
		{"pnpm build", true},
		{"pnpm test", true},
		{"cargo build", true},
		{"cargo test", true},
		{"cargo clippy", true},
		{"go build ./...", true},
		{"go test ./...", true},
		{"pytest", true},
		{"pytest tests/ -v", true},
		{"pip install -r requirements.txt", true},
		{"pip3 install flask", true},
		{"uv pip install -r req.txt", true},
		{"poetry install", true},
		{"docker build .", true},
		{"docker compose build", true},
		{"make", true},
		{"make test", true},
		{"cmake --build build", true},
		{"gradle build", true},
		{"mvn package", true},
		{"mypy src", true},
		{"tox", true},
		{"python3 -m pytest", true},

		// Compound commands containing allowlisted
		{"cd /src && dotnet build", true},
		{"DOTNET_CLI_TELEMETRY_OPTOUT=1 dotnet build", true},
		{"dotnet test | grep FAIL", true},
		{"dotnet build && dotnet test", true},
		{"npm install && npm run build && npm test", true},
		{"dotnet build > out.log", true},

		// Non-allowlisted commands
		{"git status", false},
		{"git diff", false},
		{"ls -la", false},
		{"echo hello", false},
		{"pwd", false},
		{"dotnet --version", false},
		{"dotnet ef migrations add Init", false},
		{"docker compose up -d", false},
		{"docker compose up", false},
		{"cargo run --release", false},

		// Word boundary checks
		{"makedepend src/*.c", false},
	}

	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			got := matchesAllowlist(tt.cmd)
			if got != tt.want {
				t.Errorf("matchesAllowlist(%q) = %v, want %v", tt.cmd, got, tt.want)
			}
		})
	}
}

func TestIsErrorLine(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		// Real errors
		{"error: something broke", true},
		{"Error: Cannot find module", true},
		{"ERROR: Build failed", true},
		{"FAILED tests/test_auth.py::test_login", true},
		{"fatal error: something terrible", true},
		{"java.lang.NullPointerException: oops", true},
		{"FAIL\tpkg/broken\t0.01s", true},

		// False positives (should NOT be detected as errors)
		{"0 Error(s)", false},
		{"    0 Error(s)", false},
		{"Failed:     0", false},
		{"Passed!  - Failed:     0, Passed:   142", false},

		// Clean lines
		{"Build succeeded.", false},
		{"filler line 42", false},
		{"  TinyTail.Web -> /src/bin/Debug/net9.0/TinyTail.Web.dll", false},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			got := isErrorLine(tt.line)
			if got != tt.want {
				t.Errorf("isErrorLine(%q) = %v, want %v", tt.line, got, tt.want)
			}
		})
	}
}
