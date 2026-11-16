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

// TestTrustingSystemWideOnMacAsksOnceThroughTheOSApprovalDialog pins the system
// scope on macOS: one elevated request, in the administrator trust domain and
// the System keychain, and never a direct write.
func TestTrustingSystemWideOnMacAsksOnceThroughTheOSApprovalDialog(t *testing.T) {
	s := &script{answers: map[string][]ostrust.Result{"/usr/bin/osascript": {{}}}}
	if err := mac(s).Trust(context.Background(), systemRoot()); err != nil {
		t.Fatalf("Trust() error = %v", err)
	}
	if s.ran("add-trusted-cert") {
		t.Fatal("the system trust was written without the approval dialog")
	}
	var request string
	for _, seen := range s.seen {
		if strings.HasPrefix(seen, "/usr/bin/osascript") {
			request = seen
		}
	}
	for _, want := range []string{
		"with administrator privileges", "'add-trusted-cert' '-d' '-r' 'trustRoot' '-p' 'ssl' '-k' '/Library/Keychains/System.keychain'",
	} {
		if !strings.Contains(request, want) {
			t.Fatalf("request = %s, want %q", request, want)
		}
	}
}

func TestADeclinedApprovalDialogLeavesTheSystemUntouched(t *testing.T) {
	s := &script{answers: map[string][]ostrust.Result{"/usr/bin/osascript": {{Code: 1, Stderr: "User canceled. (-128)"}}}}
	if err := mac(s).Trust(context.Background(), systemRoot()); !errors.Is(err, ostrust.ErrDeclined) {
		t.Fatalf("Trust() error = %v, want %v", err, ostrust.ErrDeclined)
	}
}

func TestAMacWithoutTheApprovalToolCannotInstallAutomatically(t *testing.T) {
	failing := func(context.Context, string, ...string) (ostrust.Result, error) {
		return ostrust.Result{}, errors.New("not found")
	}
	if err := ostrust.New("darwin", "/Users/x", failing).Trust(context.Background(), systemRoot()); !errors.Is(err, ostrust.ErrUnavailable) {
		t.Fatalf("Trust() error = %v, want %v", err, ostrust.ErrUnavailable)
	}
}

// TestRemovingTheSystemTrustIsOneRequestAndIsReadBack keeps the dialog to a
// single ask and confirms the root is gone before reporting success.
func TestRemovingTheSystemTrustIsOneRequestAndIsReadBack(t *testing.T) {
	s := &script{answers: map[string][]ostrust.Result{
		"find-certificate": {listing(0), listing(1)}, "/usr/bin/osascript": {{}},
	}}
	if err := mac(s).Untrust(context.Background(), systemRoot()); err != nil {
		t.Fatalf("Untrust() error = %v", err)
	}
	asked := 0
	for _, seen := range s.seen {
		if strings.HasPrefix(seen, "/usr/bin/osascript") {
			asked++
			for _, want := range []string{"remove-trusted-cert' '-d'", "delete-certificate' '-Z' '" + fingerprint + "' '/Library/Keychains/System.keychain'"} {
				if !strings.Contains(seen, want) {
					t.Fatalf("request = %s, want %q", seen, want)
				}
			}
		}
	}
	if asked != 1 {
		t.Fatalf("approval requests = %d, want one", asked)
	}
}

func TestTheSystemScopeIsReadFromTheSystemKeychainAndTheAdministratorDomain(t *testing.T) {
	s := trustedScript("<key>" + fingerprint + "</key><array></array>")
	s.hook = func(args []string) {
		if args[0] == "trust-settings-export" {
			_ = os.WriteFile(args[len(args)-1], []byte("<key>"+fingerprint+"</key><array></array>"), 0o600)
		}
	}
	if got := mac(s).Inspect(context.Background(), systemRoot()); got != ostrust.Trusted {
		t.Fatalf("state = %q, want trusted", got)
	}
	var sawKeychain, sawDomain bool
	for _, seen := range s.seen {
		sawKeychain = sawKeychain || strings.Contains(seen, "/Library/Keychains/System.keychain")
		sawDomain = sawDomain || strings.Contains(seen, "trust-settings-export -d ")
	}
	if !sawKeychain || !sawDomain {
		t.Fatalf("commands = %v, want the System keychain and the -d domain", s.seen)
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
		"sudo ", "'add-trusted-cert' '-d' '-r' 'trustRoot' '-p' 'ssl' '-k' '/Library/Keychains/System.keychain' '/Users/x/.relo/ca/ca.pem'",
	} {
		if !strings.Contains(steps.Install, want) {
			t.Errorf("install = %s, want %q", steps.Install, want)
		}
	}
	if !strings.Contains(steps.Remove, "'delete-certificate' '-Z' '"+fingerprint+"' '/Library/Keychains/System.keychain'") {
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
	mac := &script{answers: map[string][]ostrust.Result{"find-certificate": {listing(1)}}}
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
