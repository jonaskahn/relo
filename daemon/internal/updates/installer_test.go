package updates

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallerFor(t *testing.T) {
	tests := []struct {
		name       string
		executable string
		goos       string
		want       string
	}{
		{
			name:       "the app bundle installs through Sparkle",
			executable: "/Applications/Relo.app/Contents/MacOS/relo",
			goos:       "darwin",
			want:       InstallerSparkle,
		},
		{
			name:       "an npm global install",
			executable: "/Users/me/.nvm/versions/node/v22/lib/node_modules/@jonaskahn/relo/bin/relo-darwin-arm64",
			goos:       "darwin",
			want:       InstallerNPM,
		},
		{
			name:       "an npm global install on Windows",
			executable: "C:/Users/me/AppData/Roaming/npm/node_modules/@jonaskahn/relo/bin/relo-windows-amd64.exe",
			goos:       "windows",
			want:       InstallerNPM,
		},
		{
			name:       "the stable Homebrew Cellar",
			executable: "/opt/homebrew/Cellar/relo/0.1.3/bin/relo",
			goos:       "darwin",
			want:       InstallerBrew,
		},
		{
			name:       "the beta Homebrew Cellar",
			executable: "/opt/homebrew/Cellar/relo-beta/0.1.4-beta.27.8d9c8af7/bin/relo",
			goos:       "darwin",
			want:       InstallerBrew,
		},
		{
			name:       "the Windows setup",
			executable: "C:/Users/me/AppData/Local/Programs/Relo/relo.exe",
			goos:       "windows",
			want:       InstallerSetup,
		},
		{
			name:       "the Linux package",
			executable: "/usr/bin/relo",
			goos:       "linux",
			want:       InstallerSetup,
		},
		{
			name:       "a build nothing installed",
			executable: "/home/me/dev/relo",
			goos:       "linux",
			want:       InstallerUnknown,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := installerFor(tt.executable, tt.goos); got != tt.want {
				t.Fatalf("installerFor(%q, %q) = %q, want %q", tt.executable, tt.goos, got, tt.want)
			}
		})
	}
}

func TestInstallerForScriptReceipt(t *testing.T) {
	tests := []struct {
		name string
		goos string
		want string
	}{
		{name: "the shell installer", goos: "linux", want: InstallerShell},
		{name: "the PowerShell installer", goos: "windows", want: InstallerPowerShell},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executable := installedBinary(t, t.TempDir(), "channel=script\nversion=0.1.3\n")
			if got := installerFor(executable, tt.goos); got != tt.want {
				t.Fatalf("installerFor(%q, %q) = %q, want %q", executable, tt.goos, got, tt.want)
			}
		})
	}
}

func TestInstallerForScriptReceiptBeatsPackagingPaths(t *testing.T) {
	scriptDir := filepath.Join(t.TempDir(), "AppData", "Local", "Programs", "Relo", "bin")
	scripted := installedBinary(t, scriptDir, "channel=script\n")
	if got := installerFor(scripted, "windows"); got != InstallerPowerShell {
		t.Fatalf("installerFor(%q, windows) = %q, want %q", scripted, got, InstallerPowerShell)
	}

	setupDir := filepath.Join(t.TempDir(), "AppData", "Local", "Programs", "Relo")
	setup := filepath.Join(setupDir, "relo.exe")
	if got := installerFor(setup, "windows"); got != InstallerSetup {
		t.Fatalf("installerFor(%q, windows) = %q, want %q", setup, got, InstallerSetup)
	}
}

func TestInstallerForForeignReceipt(t *testing.T) {
	executable := installedBinary(t, t.TempDir(), "channel=manual\n")
	if got := installerFor(executable, "linux"); got != InstallerUnknown {
		t.Fatalf("installerFor(%q, linux) = %q, want %q", executable, got, InstallerUnknown)
	}
}

func TestInstallerForHomebrewSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "Cellar", "relo", "0.1.3", "bin", "relo")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("create the Cellar: %v", err)
	}
	if err := os.WriteFile(target, nil, 0o755); err != nil {
		t.Fatalf("write the binary: %v", err)
	}
	link := filepath.Join(root, "bin", "relo")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatalf("create the link directory: %v", err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("link the binary: %v", err)
	}

	if got := installerFor(link, "darwin"); got != InstallerBrew {
		t.Fatalf("installerFor(%q, darwin) = %q, want %q", link, got, InstallerBrew)
	}
}

func TestInstallerForBrokenSymlink(t *testing.T) {
	link := filepath.Join(t.TempDir(), "relo")
	if err := os.Symlink(filepath.Join(t.TempDir(), "missing"), link); err != nil {
		t.Fatalf("link the missing binary: %v", err)
	}
	if got := installerFor(link, "linux"); got != InstallerUnknown {
		t.Fatalf("installerFor(%q, linux) = %q, want %q", link, got, InstallerUnknown)
	}
}

func TestInstallerOf(t *testing.T) {
	if got := installerOf(nil); got != InstallerUnknown {
		t.Fatalf("installerOf(nil) = %q, want %q", got, InstallerUnknown)
	}
	if got := installerOf(&Checker{Installer: InstallerBrew}); got != InstallerBrew {
		t.Fatalf("installerOf(Brew) = %q, want %q", got, InstallerBrew)
	}
}

func installedBinary(t *testing.T, dir, receipt string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create the install directory: %v", err)
	}
	executable := filepath.Join(dir, "relo")
	if err := os.WriteFile(executable, nil, 0o755); err != nil {
		t.Fatalf("write the binary: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, scriptReceipt), []byte(receipt), 0o600); err != nil {
		t.Fatalf("write the receipt: %v", err)
	}
	return executable
}
