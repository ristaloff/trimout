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

		// Command position — a tool name that is not the command must not
		// match, or output the caller explicitly asked for gets compressed.
		{`git commit -m "make it work"`, false},
		{`echo "remember to run pytest"`, false},
		{`grep -rn "make" .`, false},
		{"cat notes.md | grep mvn", false},
		{"echo make", false},
		{"git log --grep=mvn", false},
		{`git commit -m 'cmake tweaks'`, false},
		{"./configure --with-make", false},
		{"ls /opt/gradle", false},

		// Command position — still matched where the tool really runs
		{"rm -rf build && make", true},
		{"make || echo failed", true},
		{"(cd src; make)", true},
		{"echo $(make -n)", true},

		// Wrappers and runners are peeled
		{"sudo make install", true},
		{"env FOO=1 dotnet build", true},
		{"nohup npm test", true},
		{"uv run pytest", true},
		{"poetry run pytest tests/", true},
		{"bundle exec pytest", true},
		{"timeout 300 cargo test", true},
		{"timeout 5m go test ./...", true},

		// Wrappers invoked with their own options. A wrapper's options and
		// their values are stepped over; the first ordinary word is the
		// command.
		{"sudo -E make install", true},
		{"nice -n 10 make", true},
		{"stdbuf -oL make", true},
		{"env -u GOFLAGS go build ./...", true},
		{"timeout -k 5 300 cargo test", true},

		// ...but the scan stops at the first ordinary word, so an
		// allowlisted name further along an unrelated command is not a match.
		{"sudo find / -name make", false},
		{"sudo rm -rf /var/make", false},

		// Build commands inside conditionals and loops
		{"if [ -f Makefile ]; then make; fi", true},
		{"for d in a b; do make -C $d; done", true},
		{"while true; do npm test; done", true},
		{"if make; then echo ok; fi", true},
		{"for f in *.txt; do echo $f; done", false},
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

func TestCommandSegments(t *testing.T) {
	tests := []struct {
		cmd  string
		want []string
	}{
		{"make", []string{"make"}},
		{"a && b", []string{"a ", "", " b"}},
		{"a | b", []string{"a ", " b"}},
		{"a; b", []string{"a", " b"}},
		{`echo "a | b"`, []string{`echo "a | b"`}},
		{`echo 'a && b'`, []string{`echo 'a && b'`}},
	}

	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			got := commandSegments(tt.cmd)
			if len(got) != len(tt.want) {
				t.Fatalf("commandSegments(%q) = %q, want %q", tt.cmd, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("commandSegments(%q)[%d] = %q, want %q", tt.cmd, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestCommandHead(t *testing.T) {
	tests := []struct {
		seg  string
		want string
	}{
		{"make test", "make test"},
		{"  make test  ", "make test"},
		{"sudo make", "make"},
		{"FOO=bar BAZ=1 make", "make"},
		{`FOO="a b" make`, "make"},
		{"env make", "make"},
		{"uv run pytest", "pytest"},
		{"timeout 300 cargo test", "cargo test"},
		{"timeout -k 5 300 cargo test", "cargo test"},
		{"", ""},
		{"sudo", ""},
		{"nice -n 10 make", "make"},
		{"sudo -E make install", "make install"},
		{"env -u GOFLAGS go build", "go build"},
		{"sudo find / -name make", "find / -name make"},
		{"then make", "make"},
		{"do npm test", "npm test"},
	}

	for _, tt := range tests {
		t.Run(tt.seg, func(t *testing.T) {
			if got := commandHead(tt.seg); got != tt.want {
				t.Errorf("commandHead(%q) = %q, want %q", tt.seg, got, tt.want)
			}
		})
	}
}
