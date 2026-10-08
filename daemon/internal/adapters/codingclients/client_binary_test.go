package codingclients

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// writeExecutable records a launcher file the resolver can run.
func writeExecutable(t *testing.T, path string) {
	t.Helper()
	writeFile(t, path)
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatalf("make %s executable: %v", path, err)
	}
}

func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create the directory of %s: %v", path, err)
	}
	script := "#!/bin/sh\n"
	if err := os.WriteFile(path, []byte(script), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestFindClientBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the login-shell probe and the exec bit do not exist on Windows")
	}
	tempDir := t.TempDir()
	emptyPath := filepath.Join(tempDir, "empty-path")
	if err := os.MkdirAll(emptyPath, 0o700); err != nil {
		t.Fatalf("create the empty PATH directory: %v", err)
	}
	writeExecutable(t, filepath.Join(tempDir, "opencode"))

	// A login shell prints what an rc file would, then answers with the
	// launcher it holds; its directory also holds a file the answer could
	// name but a restart could not run.
	shellDir := filepath.Join(tempDir, "shell-bin")
	writeExecutable(t, filepath.Join(shellDir, "opencode"))
	writeFile(t, filepath.Join(shellDir, "opencode-plain"))
	probeShell := func(answer string) string {
		script := "#!/bin/sh\necho 'setup banner'\nprintf '%s\\n' " + answer + "\n"
		return script
	}
	shellLauncher := filepath.Join(shellDir, "probe-shell")
	writeExecutable(t, shellLauncher)
	if err := os.WriteFile(shellLauncher, []byte(probeShell(filepath.Join(shellDir, "opencode"))), 0o755); err != nil {
		t.Fatalf("write the probe shell: %v", err)
	}
	shellPlain := filepath.Join(shellDir, "probe-shell-plain")
	writeExecutable(t, shellPlain)
	if err := os.WriteFile(shellPlain, []byte(probeShell(filepath.Join(shellDir, "opencode-plain"))), 0o755); err != nil {
		t.Fatalf("write the plain probe shell: %v", err)
	}

	// A shell that resolves but finds nothing, plus one that dies, keep the
	// probe from deciding a launcher exists when its output says otherwise.
	shellSilent := filepath.Join(shellDir, "probe-shell-silent")
	writeExecutable(t, shellSilent)
	if err := os.WriteFile(shellSilent, []byte(probeShell("")), 0o755); err != nil {
		t.Fatalf("write the silent probe shell: %v", err)
	}
	silentDir := filepath.Join(tempDir, "silent-dir")
	if err := os.MkdirAll(silentDir, 0o700); err != nil {
		t.Fatalf("create the silent directory: %v", err)
	}
	writeExecutable(t, filepath.Join(silentDir, ".opencode", "bin", "opencode"))

	type resolution struct {
		path  string
		home  string
		shell string
	}
	tests := []struct {
		name string
		env  resolution
		want string
	}{
		{"the daemon PATH holds the launcher", resolution{path: tempDir, shell: "/bin/false"}, filepath.Join(tempDir, "opencode")},
		{"the login shell answers through a banner", resolution{path: emptyPath, shell: shellLauncher}, filepath.Join(shellDir, "opencode")},
		{"a shell answer that names nothing falls through", resolution{path: emptyPath, shell: shellSilent, home: silentDir}, filepath.Join(silentDir, ".opencode", "bin", "opencode")},
	}

	// Each row of the well-known list resolves when it is the only home
	// directory holding the launcher, newest nvm version included.
	rows := []struct{ name, dir string }{
		{"the client's own bin", filepath.Join(".opencode", "bin")},
		{"the user local bin", filepath.Join(".local", "bin")},
		{"the user bin", "bin"},
		{"the bun bin", filepath.Join(".bun", "bin")},
		{"the npm-global bin", filepath.Join(".npm-global", "bin")},
		{"the npm bin", filepath.Join(".npm", "bin")},
		{"the pnpm home", filepath.Join("Library", "pnpm")},
	}
	for _, row := range rows {
		home := filepath.Join(tempDir, "homes", strings.ReplaceAll(row.dir, string(filepath.Separator), "_"))
		writeExecutable(t, filepath.Join(home, row.dir, "opencode"))
		tests = append(tests, struct {
			name string
			env  resolution
			want string
		}{row.name, resolution{path: emptyPath, home: home, shell: "/bin/false"}, filepath.Join(home, row.dir, "opencode")})
	}

	nvmHome := filepath.Join(tempDir, "nvm-home")
	for _, version := range []string{"v9.9.9", "v10.0.0", "v22.10.1", "v24.21.0"} {
		writeExecutable(t, filepath.Join(nvmHome, ".nvm", "versions", "node", version, "bin", "opencode"))
	}
	tests = append(tests, struct {
		name string
		env  resolution
		want string
	}{"the newest nvm install answers", resolution{path: emptyPath, home: nvmHome, shell: "/bin/false"},
		filepath.Join(nvmHome, ".nvm", "versions", "node", "v24.21.0", "bin", "opencode")})

	// The plain probe answers with a file that exists but is not executable,
	// so the resolver has to keep looking.
	plainHome := filepath.Join(tempDir, "plain-home")
	tests = append(tests, struct {
		name string
		env  resolution
		want string
	}{"a non-executable shell answer is skipped", resolution{path: emptyPath, home: plainHome, shell: shellPlain}, ""})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PATH", tt.env.path)
			t.Setenv("SHELL", tt.env.shell)
			name := "opencode"
			if tt.want == "" {
				// A name no machine installs keeps the not-found step free
				// of whatever the real Homebrew prefixes hold.
				name = "relo-missing-client"
				tt.env.home = t.TempDir()
			}
			got, err := findClientBinary(context.Background(), tt.env.home, name)
			if tt.want == "" {
				if err == nil || !strings.Contains(err.Error(), "was not found") {
					t.Fatalf("findClientBinary() = %q, %v, want a not-found error", got, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("findClientBinary() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("findClientBinary() = %q, want %q", got, tt.want)
			}
		})
	}
}
