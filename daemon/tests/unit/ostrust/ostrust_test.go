package ostrust_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/ostrust"
)

const fingerprint = "AABBCCDDEEFF00112233445566778899AABBCCDD"

var root = ostrust.Root{
	PEM: []byte("-----BEGIN CERTIFICATE-----\nroot\n-----END CERTIFICATE-----\n"), SHA1: fingerprint,
	CommonName: "Relo Local CA", Host: "claude.ai", LeafPEM: []byte("leaf"),
}

// script is a Runner that answers each command from a table keyed by the first
// word the tool is asked, and records every command it saw.
type script struct {
	seen    []string
	answers map[string][]ostrust.Result
	hook    func(args []string)
}

func (s *script) run(_ context.Context, name string, args ...string) (ostrust.Result, error) {
	key := name
	if len(args) > 0 && strings.Contains(name, "security") {
		key = args[0]
	}
	s.seen = append(s.seen, key+" "+strings.Join(args, " "))
	if s.hook != nil {
		s.hook(args)
	}
	queue := s.answers[key]
	if len(queue) == 0 {
		return ostrust.Result{Code: 99}, nil
	}
	answer := queue[0]
	if len(queue) > 1 {
		s.answers[key] = queue[1:]
	}
	return answer, nil
}

func (s *script) ran(prefix string) bool {
	for _, command := range s.seen {
		if strings.HasPrefix(command, prefix) {
			return true
		}
	}
	return false
}

func listing(code int) ostrust.Result {
	if code != 0 {
		return ostrust.Result{Code: code}
	}
	return ostrust.Result{Stdout: "SHA-1 hash: AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD\n"}
}

func trustedScript(plist string) *script {
	s := &script{answers: map[string][]ostrust.Result{
		"find-certificate":      {listing(0)},
		"verify-cert":           {{}},
		"trust-settings-export": {{}},
	}}
	s.hook = func(args []string) {
		if args[0] == "trust-settings-export" {
			_ = os.WriteFile(args[1], []byte(plist), 0o600)
		}
	}
	return s
}

func mac(s *script) ostrust.Store { return ostrust.New("darwin", "/Users/x", s.run) }

func TestAnUntrustedRootIsReadAsUntrustedWithoutAskingTheKeychainToChange(t *testing.T) {
	s := &script{answers: map[string][]ostrust.Result{"find-certificate": {listing(1)}}}
	if got := mac(s).Inspect(context.Background(), root); got != ostrust.Untrusted {
		t.Fatalf("state = %q, want untrusted", got)
	}
	if s.ran("add-trusted-cert") || s.ran("delete-certificate") {
		t.Fatalf("an inspection changed the keychain: %v", s.seen)
	}
}

func TestATrustedRootNeedsTheKeychainTheLeafAndNoHostScope(t *testing.T) {
	plist := "<dict><key>" + fingerprint + "</key><dict><key>trustSettings</key><array><dict></dict></array></dict></dict>"
	if got := mac(trustedScript(plist)).Inspect(context.Background(), root); got != ostrust.Trusted {
		t.Fatalf("state = %q, want trusted", got)
	}
}

// A host-scoped trust setting is honoured by the OS but skipped by Chromium, so
// a root trusted that way would leave the app rejecting the leaf.
func TestAHostScopedTrustIsNotTrust(t *testing.T) {
	plist := "<dict><key>" + fingerprint + "</key><array><dict><key>kSecTrustSettingsPolicyString</key><string>claude.ai</string></dict></array></dict>"
	if got := mac(trustedScript(plist)).Inspect(context.Background(), root); got != ostrust.Untrusted {
		t.Fatalf("state = %q, want untrusted for a host-scoped setting", got)
	}
}

func TestWhatTheOSWillNotSayIsUnknownNeverTrusted(t *testing.T) {
	cases := map[string]*script{
		"the lookup fails": {answers: map[string][]ostrust.Result{"find-certificate": {listing(2)}}},
		"the verify fails": {answers: map[string][]ostrust.Result{"find-certificate": {listing(0)}, "verify-cert": {{Code: 5}}}},
		"settings cannot be exported": {answers: map[string][]ostrust.Result{
			"find-certificate": {listing(0)}, "verify-cert": {{}}, "trust-settings-export": {{Code: 1}}}},
	}
	for name, s := range cases {
		if got := mac(s).Inspect(context.Background(), root); got != ostrust.Unknown {
			t.Errorf("%s: state = %q, want unknown", name, got)
		}
	}
}

func TestTrustAddsAnSSLRootWithoutAHostPolicy(t *testing.T) {
	s := &script{answers: map[string][]ostrust.Result{"add-trusted-cert": {{}}}}
	if err := mac(s).Trust(context.Background(), root); err != nil {
		t.Fatalf("Trust() error = %v", err)
	}
	command := s.seen[0]
	for _, want := range []string{"-r trustRoot", "-p ssl", "login.keychain-db"} {
		if !strings.Contains(command, want) {
			t.Fatalf("command = %q, want %q", command, want)
		}
	}
	if strings.Contains(command, " -s ") {
		t.Fatalf("command = %q, want no host policy", command)
	}
}

func TestADeclinedTrustIsReportedAsDeclined(t *testing.T) {
	s := &script{answers: map[string][]ostrust.Result{"add-trusted-cert": {{Code: 1}}}}
	if err := mac(s).Trust(context.Background(), root); !errors.Is(err, ostrust.ErrDeclined) {
		t.Fatalf("Trust() error = %v, want %v", err, ostrust.ErrDeclined)
	}
}

func TestUntrustingARootThatIsNotThereAsksTheKeychainForNothing(t *testing.T) {
	s := &script{answers: map[string][]ostrust.Result{"find-certificate": {listing(1)}}}
	if err := mac(s).Untrust(context.Background(), root); err != nil {
		t.Fatalf("Untrust() error = %v", err)
	}
	if s.ran("remove-trusted-cert") || s.ran("delete-certificate") {
		t.Fatalf("untrust prompted for a root that is not there: %v", s.seen)
	}
}

func TestUntrustRemovesTheSettingAndTheCertificateThenChecks(t *testing.T) {
	s := &script{answers: map[string][]ostrust.Result{
		"find-certificate":    {listing(0), listing(1)},
		"remove-trusted-cert": {{}},
		"delete-certificate":  {{}},
	}}
	if err := mac(s).Untrust(context.Background(), root); err != nil {
		t.Fatalf("Untrust() error = %v", err)
	}
	if !s.ran("remove-trusted-cert") || !s.ran("delete-certificate delete-certificate -Z "+fingerprint) {
		t.Fatalf("commands = %v, want the setting removed and the certificate deleted by fingerprint", s.seen)
	}
	stuck := &script{answers: map[string][]ostrust.Result{
		"find-certificate": {listing(0)}, "remove-trusted-cert": {{}}, "delete-certificate": {{}},
	}}
	if err := mac(stuck).Untrust(context.Background(), root); !errors.Is(err, ostrust.ErrDeclined) {
		t.Fatalf("Untrust() with the certificate still listed error = %v, want %v", err, ostrust.ErrDeclined)
	}
}

func TestWindowsTrustsInTheCurrentUsersRootStoreOnly(t *testing.T) {
	s := &script{answers: map[string][]ostrust.Result{"powershell.exe": {{}}}}
	store := ostrust.New("windows", "", s.run)
	if err := store.Trust(context.Background(), root); err != nil {
		t.Fatalf("Trust() error = %v", err)
	}
	if !strings.Contains(s.seen[0], `Cert:\CurrentUser\Root`) || strings.Contains(s.seen[0], "LocalMachine") {
		t.Fatalf("command = %q, want the current user's store only", s.seen[0])
	}
}

func TestWindowsReadsTrustFromTheChainNotJustPresence(t *testing.T) {
	for output, want := range map[string]ostrust.State{
		"trusted": ostrust.Trusted, "absent": ostrust.Untrusted, "broken": ostrust.Untrusted, "garbled": ostrust.Unknown,
	} {
		s := &script{answers: map[string][]ostrust.Result{"powershell.exe": {{Stdout: output + "\n"}}}}
		if got := ostrust.New("windows", "", s.run).Inspect(context.Background(), root); got != want {
			t.Errorf("output %q: state = %q, want %q", output, got, want)
		}
	}
}

func TestAnUnsupportedOSChangesNothing(t *testing.T) {
	s := &script{}
	store := ostrust.New("linux", "", s.run)
	if got := store.Inspect(context.Background(), root); got != ostrust.Unsupported {
		t.Fatalf("state = %q, want unsupported", got)
	}
	if err := store.Trust(context.Background(), root); !errors.Is(err, ostrust.ErrUnsupported) {
		t.Fatalf("Trust() error = %v, want %v", err, ostrust.ErrUnsupported)
	}
	if len(s.seen) != 0 {
		t.Fatalf("commands = %v, want none", s.seen)
	}
}
