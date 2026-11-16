package ostrust_test

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/jonaskahn/relo/internal/adapters/ostrust"
)

const sha256Fingerprint = "00112233445566778899AABBCCDDEEFF00112233445566778899AABBCCDDEEFF"

func systemRoot() ostrust.Root {
	scoped := root
	scoped.Scope, scoped.SHA256 = ostrust.ScopeSystem, sha256Fingerprint
	return scoped
}

// table is a Runner that answers from the full command line, so a test can tell
// `sh -c command -v pkexec` from `sh -c command -v certutil`.
type table struct {
	seen    []string
	answers map[string]ostrust.Result
	missing map[string]bool
}

func (t *table) run(_ context.Context, name string, args ...string) (ostrust.Result, error) {
	line := name + " " + strings.Join(args, " ")
	t.seen = append(t.seen, line)
	for prefix := range t.missing {
		if strings.HasPrefix(line, prefix) {
			return ostrust.Result{}, errors.New("not found")
		}
	}
	for prefix, result := range t.answers {
		if strings.HasPrefix(line, prefix) {
			return result, nil
		}
	}
	return ostrust.Result{Code: 99}, nil
}

func (t *table) ran(fragment string) int {
	count := 0
	for _, line := range t.seen {
		if strings.Contains(line, fragment) {
			count++
		}
	}
	return count
}

func (t *table) line(fragment string) string {
	for _, line := range t.seen {
		if strings.Contains(line, fragment) {
			return line
		}
	}
	return ""
}

// TestTrustingSystemWideOnMacWritesTheAdministratorDomainAsTheUser pins the
// system scope on macOS: the trust setting is written by the user, whom macOS
// asks itself, and never through a root shell, which cannot be asked and is
// refused ("no user interaction was possible").
func TestTrustingSystemWideOnMacWritesTheAdministratorDomainAsTheUser(t *testing.T) {
	s := &script{answers: map[string][]ostrust.Result{"add-trusted-cert": {{}}}}
	if err := mac(s).Trust(context.Background(), systemRoot()); err != nil {
		t.Fatalf("Trust() error = %v", err)
	}
	if s.ran("/usr/bin/osascript") {
		t.Fatal("the trust setting was written through a root shell")
	}
	command := s.seen[0]
	if !strings.Contains(command, "add-trusted-cert -d -r trustRoot -p ssl") || strings.Contains(command, " -k ") {
		t.Fatalf("command = %q, want the administrator domain and no keychain", command)
	}
}

func TestADeclinedSystemTrustSaysWhatTheOSSaid(t *testing.T) {
	s := &script{answers: map[string][]ostrust.Result{"add-trusted-cert": {{Code: 1, Stderr: "User canceled. (-128)"}}}}
	err := mac(s).Trust(context.Background(), systemRoot())
	if !errors.Is(err, ostrust.ErrDeclined) || !strings.Contains(err.Error(), "User canceled") {
		t.Fatalf("Trust() error = %v, want a decline carrying the OS's words", err)
	}
}

// TestRemovingTheSystemTrustTakesTheSettingAndTheKeychainCopyAndReadsBack keeps
// the administrator request to the one step that needs it, the keychain file.
func TestRemovingTheSystemTrustTakesTheSettingAndTheKeychainCopyAndReadsBack(t *testing.T) {
	entry := "<key>" + fingerprint + "</key><array></array>"
	s := &script{answers: map[string][]ostrust.Result{
		"find-certificate": {listing(0), listing(1)}, "remove-trusted-cert": {{}},
		"trust-settings-export": {{}}, "/usr/bin/osascript": {{}},
	}}
	exports := 0
	s.hook = func(args []string) {
		if args[0] == "trust-settings-export" {
			exports++
			content := entry
			if exports > 1 {
				content = ""
			}
			_ = os.WriteFile(args[len(args)-1], []byte(content), 0o600)
		}
	}
	if err := mac(s).Untrust(context.Background(), systemRoot()); err != nil {
		t.Fatalf("Untrust() error = %v", err)
	}
	if !s.ran("remove-trusted-cert remove-trusted-cert -d") {
		t.Fatalf("commands = %v, want the setting removed from the administrator domain", s.seen)
	}
	asked := 0
	for _, seen := range s.seen {
		if strings.HasPrefix(seen, "/usr/bin/osascript") {
			asked++
			if want := "delete-certificate' '-Z' '" + fingerprint + "' '/Library/Keychains/System.keychain'"; !strings.Contains(seen, want) || strings.Contains(seen, "remove-trusted-cert") {
				t.Fatalf("request = %s, want only the keychain delete", seen)
			}
		}
	}
	if asked != 1 {
		t.Fatalf("approval requests = %d, want one", asked)
	}
}

// TestTheSystemScopeNeedsAnEntryInTheAdministratorDomain pins the fix for a
// root that is in the System keychain and trusted by the user alone: a verified
// leaf says nothing about the domain a virtual machine copies from.
func TestTheSystemScopeNeedsAnEntryInTheAdministratorDomain(t *testing.T) {
	read := func(plist string) ostrust.State {
		s := trustedScript(plist)
		s.hook = func(args []string) {
			if args[0] == "trust-settings-export" {
				_ = os.WriteFile(args[len(args)-1], []byte(plist), 0o600)
			}
		}
		return mac(s).Inspect(context.Background(), systemRoot())
	}
	if got := read("<key>mitmproxy</key><array></array>"); got != ostrust.Untrusted {
		t.Fatalf("no entry: state = %q, want untrusted", got)
	}
	if got := read("<key>" + fingerprint + "</key><array></array>"); got != ostrust.Trusted {
		t.Fatalf("entry: state = %q, want trusted", got)
	}
	scoped := "<key>" + fingerprint + "</key><array><dict><key>kSecTrustSettingsPolicyString</key><string>claude.ai</string></dict></array>"
	if got := read(scoped); got != ostrust.Untrusted {
		t.Fatalf("host-scoped: state = %q, want untrusted", got)
	}
}

func TestTheSystemScopeIsReadFromTheAdministratorDomain(t *testing.T) {
	s := &script{answers: map[string][]ostrust.Result{"trust-settings-export": {{}}}}
	s.hook = func(args []string) {
		_ = os.WriteFile(args[len(args)-1], []byte("<key>"+fingerprint+"</key><array></array>"), 0o600)
	}
	if got := mac(s).Inspect(context.Background(), systemRoot()); got != ostrust.Trusted {
		t.Fatalf("state = %q, want trusted", got)
	}
	if !strings.Contains(strings.Join(s.seen, "\n"), "trust-settings-export -d ") {
		t.Fatalf("commands = %v, want the -d domain", s.seen)
	}
}

func decodeEncodedCommand(t *testing.T, line string) string {
	t.Helper()
	_, encoded, found := strings.Cut(line, "'-EncodedCommand','")
	if !found {
		t.Fatalf("command = %s, want an encoded command", line)
	}
	encoded, _, _ = strings.Cut(encoded, "'")
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	units := make([]uint16, len(raw)/2)
	for index := range units {
		units[index] = uint16(raw[2*index]) | uint16(raw[2*index+1])<<8
	}
	return string(utf16.Decode(units))
}

// TestTrustingSystemWideOnWindowsGoesThroughAnElevatedMachineStoreImport pins the
// Windows system scope: the machine's Root store, behind UAC.
func TestTrustingSystemWideOnWindowsGoesThroughAnElevatedMachineStoreImport(t *testing.T) {
	s := &script{answers: map[string][]ostrust.Result{"powershell.exe": {{}}}}
	if err := ostrust.New("windows", "", s.run).Trust(context.Background(), systemRoot()); err != nil {
		t.Fatalf("Trust() error = %v", err)
	}
	if len(s.seen) != 1 || !strings.Contains(s.seen[0], "-Verb RunAs") {
		t.Fatalf("commands = %v, want one elevated request", s.seen)
	}
	if inner := decodeEncodedCommand(t, s.seen[0]); !strings.Contains(inner, `Import-Certificate`) ||
		!strings.Contains(inner, `Cert:\LocalMachine\Root`) {
		t.Fatalf("inner script = %s, want an import into the machine Root store", inner)
	}
	declined := &script{answers: map[string][]ostrust.Result{"powershell.exe": {{Code: 1}}}}
	if err := ostrust.New("windows", "", declined.run).Trust(context.Background(), systemRoot()); !errors.Is(err, ostrust.ErrDeclined) {
		t.Fatalf("declined Trust() error = %v, want %v", err, ostrust.ErrDeclined)
	}
}

func TestTheWindowsSystemScopeIsReadFromTheMachineStore(t *testing.T) {
	s := &script{answers: map[string][]ostrust.Result{"powershell.exe": {{Stdout: "trusted\n"}}}}
	if got := ostrust.New("windows", "", s.run).Inspect(context.Background(), systemRoot()); got != ostrust.Trusted {
		t.Fatalf("state = %q, want trusted", got)
	}
	if !strings.Contains(s.seen[0], `Cert:\LocalMachine\Root`) {
		t.Fatalf("command = %s, want the machine store", s.seen[0])
	}
}

func linuxTable(tools ...string) *table {
	answers := map[string]ostrust.Result{}
	for _, tool := range tools {
		answers["sh -c command -v '"+tool+"'"] = ostrust.Result{}
	}
	return &table{answers: answers}
}

func TestADebianSystemInstallRunsThroughPolkitAndUpdatesTheBundle(t *testing.T) {
	tb := linuxTable("update-ca-certificates", "pkexec")
	tb.answers["pkexec "] = ostrust.Result{}
	if err := ostrust.New("linux", "/home/x", tb.run).Trust(context.Background(), systemRoot()); err != nil {
		t.Fatalf("Trust() error = %v", err)
	}
	line := tb.line("pkexec sh -c")
	for _, want := range []string{"/usr/local/share/ca-certificates/relo-local-ca.crt", "update-ca-certificates"} {
		if !strings.Contains(line, want) {
			t.Fatalf("elevated command = %s, want %q", line, want)
		}
	}
}

func TestWithNoWayToElevateALinuxInstallIsLeftToTheOperator(t *testing.T) {
	tb := linuxTable("update-ca-certificates")
	if err := ostrust.New("linux", "/home/x", tb.run).Trust(context.Background(), systemRoot()); !errors.Is(err, ostrust.ErrUnavailable) {
		t.Fatalf("Trust() error = %v, want %v", err, ostrust.ErrUnavailable)
	}
	if tb.ran("install -m") > 0 {
		t.Fatal("something was installed without an approval")
	}
}

func TestALinuxSystemRootIsTrustedOnlyWhenItsFileAndTheBundleAgree(t *testing.T) {
	cases := map[string]struct {
		sum    ostrust.Result
		verify ostrust.Result
		want   ostrust.State
	}{
		"installed and verified":  {ostrust.Result{Stdout: strings.ToLower(sha256Fingerprint) + "  /x\n"}, ostrust.Result{}, ostrust.Trusted},
		"another certificate":     {ostrust.Result{Stdout: strings.Repeat("ab", 32) + "  /x\n"}, ostrust.Result{}, ostrust.Untrusted},
		"file absent":             {ostrust.Result{Code: 1}, ostrust.Result{}, ostrust.Untrusted},
		"the bundle rejects leaf": {ostrust.Result{Stdout: strings.ToLower(sha256Fingerprint) + "  /x\n"}, ostrust.Result{Code: 2}, ostrust.Untrusted},
	}
	for name, test := range cases {
		tb := linuxTable("update-ca-certificates")
		tb.answers["sha256sum "] = test.sum
		tb.answers["openssl verify"] = test.verify
		if got := ostrust.New("linux", "/home/x", tb.run).Inspect(context.Background(), systemRoot()); got != test.want {
			t.Errorf("%s: state = %q, want %q", name, got, test.want)
		}
	}
}

func TestALinuxUserRootNeedsTheNSSTool(t *testing.T) {
	tb := &table{}
	store := ostrust.New("linux", "/home/x", tb.run)
	if got := store.Inspect(context.Background(), root); got != ostrust.Unknown {
		t.Fatalf("state = %q, want unknown without certutil", got)
	}
	if err := store.Trust(context.Background(), root); !errors.Is(err, ostrust.ErrUnavailable) {
		t.Fatalf("Trust() error = %v, want %v", err, ostrust.ErrUnavailable)
	}
}

// TestTheManualStepsAreTheCommandsTheAutomaticPathRuns keeps the instructions an
// operator copies from drifting away from what Relo itself does.
func TestTheManualStepsAreTheCommandsTheAutomaticPathRuns(t *testing.T) {
	steps := ostrust.Manual("darwin", systemRoot(), "/Users/x/.relo/ca/ca.pem")
	for _, want := range []string{
		"'add-trusted-cert' '-d' '-r' 'trustRoot' '-p' 'ssl' '/Users/x/.relo/ca/ca.pem'",
	} {
		if !strings.Contains(steps.Install, want) {
			t.Errorf("install = %s, want %q", steps.Install, want)
		}
	}
	if strings.Contains(steps.Install, "sudo") || !strings.Contains(steps.Remove, "sudo '/usr/bin/security' 'delete-certificate' '-Z' '"+fingerprint+"' '/Library/Keychains/System.keychain'") {
		t.Errorf("remove = %s, want the System keychain delete", steps.Remove)
	}
	if steps.DocsURL != ostrust.DocsURL || steps.SHA256 != sha256Fingerprint || steps.CertPath == "" {
		t.Errorf("steps = %+v, want the page, the fingerprint and the path", steps)
	}
	windows := ostrust.Manual("windows", systemRoot(), `C:\x\ca.pem`)
	if windows.Install != `certutil -addstore -f Root "C:\x\ca.pem"` || windows.Remove != "certutil -delstore Root "+fingerprint {
		t.Errorf("windows = %+v", windows)
	}
	linux := ostrust.Manual("linux", systemRoot(), "/home/x/.relo/ca/ca.pem")
	for _, want := range []string{"Debian/Ubuntu:", "Fedora/RHEL:", "Arch:", "update-ca-certificates"} {
		if !strings.Contains(linux.Install, want) {
			t.Errorf("linux install = %s, want %q", linux.Install, want)
		}
	}
	if other := ostrust.Manual("plan9", systemRoot(), "/x"); other.Install != "" || other.CertPath != "/x" {
		t.Errorf("unsupported OS = %+v, want the path and no commands", other)
	}
}

// TestRemovingWhatIsNotThereNeverAsksForAnApproval keeps a removal that has
// nothing to do silent, on each OS: it is what lets Desktop leave proxy mode
// without a dialog when no certificate was ever installed.
func TestRemovingWhatIsNotThereNeverAsksForAnApproval(t *testing.T) {
	mac := &script{answers: map[string][]ostrust.Result{"find-certificate": {listing(1)}, "trust-settings-export": {{}}}}
	mac.hook = func(args []string) { _ = os.WriteFile(args[len(args)-1], []byte(""), 0o600) }
	if err := ostrust.New("darwin", "/Users/x", mac.run).Untrust(context.Background(), systemRoot()); err != nil {
		t.Fatalf("macOS Untrust() error = %v", err)
	}
	if mac.ran("/usr/bin/osascript") {
		t.Fatal("macOS asked for an approval to remove nothing")
	}
	windows := &script{answers: map[string][]ostrust.Result{"powershell.exe": {{}}}}
	if err := ostrust.New("windows", "", windows.run).Untrust(context.Background(), systemRoot()); err != nil {
		t.Fatalf("Windows Untrust() error = %v", err)
	}
	if len(windows.seen) != 1 || strings.Contains(windows.seen[0], "RunAs") {
		t.Fatalf("Windows commands = %v, want one read and no elevation", windows.seen)
	}
	bare := &table{}
	if err := ostrust.New("linux", "/home/x", bare.run).Untrust(context.Background(), systemRoot()); err != nil {
		t.Fatalf("Linux Untrust() error = %v", err)
	}
	if err := ostrust.New("linux", "/home/x", bare.run).Untrust(context.Background(), root); err != nil {
		t.Fatalf("Linux user Untrust() error = %v", err)
	}
}
