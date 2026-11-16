package ostrust

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const securityTool = "/usr/bin/security"

// keychain trusts a root in the operator's login keychain. The trust carries no
// host policy: Chromium, which Claude Desktop is built on, skips a setting that
// does, so a host-scoped trust would leave the app rejecting the leaf. The
// root's critical name constraint is what limits it to its hosts.
type keychain struct {
	home string
	run  Runner
}

func (k *keychain) path() string {
	return filepath.Join(k.home, "Library", "Keychains", "login.keychain-db")
}

var sha1Line = regexp.MustCompile(`(?m)^SHA-1 hash:\s*([0-9A-Fa-f:]{40,59})\s*$`)

func listsFingerprint(output, sha1 string) bool {
	for _, match := range sha1Line.FindAllStringSubmatch(output, -1) {
		if strings.EqualFold(strings.ReplaceAll(match[1], ":", ""), sha1) {
			return true
		}
	}
	return false
}

// Inspect reads trust without changing it: the root is in the keychain, a leaf
// it issued verifies for the host, and the trust setting is not host-scoped.
// Anything the OS will not say is Unknown.
func (k *keychain) Inspect(ctx context.Context, root Root) State {
	if state := k.listedState(ctx, root); state != Trusted {
		return state
	}
	if state := k.verifiedState(ctx, root); state != Trusted {
		return state
	}
	scoped, readable := k.hostScoped(ctx, root.SHA1)
	switch {
	case !readable:
		return Unknown
	case scoped:
		return Untrusted
	}
	return Trusted
}

// listedState is Trusted when the root is in the keychain, which is only the
// first of the checks that make it trusted.
func (k *keychain) listedState(ctx context.Context, root Root) State {
	found, err := k.run(ctx, securityTool, "find-certificate", "-a", "-Z", "-c", root.CommonName, k.path())
	switch {
	case err != nil:
		return Unknown
	case found.Code == 1, found.Code == 0 && !listsFingerprint(found.Stdout, root.SHA1):
		return Untrusted
	case found.Code != 0:
		return Unknown
	}
	return Trusted
}

// verifiedState is Trusted when a leaf the root issued verifies for its host.
func (k *keychain) verifiedState(ctx context.Context, root Root) State {
	leaf, cleanup, err := privateCopy("leaf.pem", root.LeafPEM)
	if err != nil {
		return Unknown
	}
	defer cleanup()
	verified, err := k.run(ctx, securityTool, "verify-cert", "-q", "-L", "-c", leaf, "-p", "ssl", "-n", root.Host, "-k", k.path())
	switch {
	case err != nil:
		return Unknown
	case verified.Code == 1:
		return Untrusted
	case verified.Code != 0:
		return Unknown
	}
	return Trusted
}

// hostScoped reports whether the root's trust settings carry a policy string,
// and false in the second result when they cannot be read.
func (k *keychain) hostScoped(ctx context.Context, sha1 string) (scoped, readable bool) {
	dir, err := os.MkdirTemp("", "relo-trust-")
	if err != nil {
		return false, false
	}
	defer func() { _ = os.RemoveAll(dir) }()
	file := filepath.Join(dir, "trust-settings.plist")
	exported, err := k.run(ctx, securityTool, "trust-settings-export", file)
	if err != nil || exported.Code != 0 {
		return false, false
	}
	content, err := os.ReadFile(file)
	if err != nil {
		return false, false
	}
	xml := string(content)
	at := strings.Index(xml, "<key>"+sha1+"</key>")
	if at < 0 {
		return false, true
	}
	entry := xml[at:]
	if end := strings.Index(entry, "</array>"); end >= 0 {
		entry = entry[:end]
	}
	return strings.Contains(entry, "<key>kSecTrustSettingsPolicyString</key>"), true
}

// Trust adds the root to the login keychain as an SSL trust root, which is the
// step macOS answers with its own authorization prompt.
func (k *keychain) Trust(ctx context.Context, root Root) error {
	file, cleanup, err := privateCopy("ca.pem", root.PEM)
	if err != nil {
		return err
	}
	defer cleanup()
	result, err := k.run(ctx, securityTool, "add-trusted-cert", "-r", "trustRoot", "-p", "ssl", "-k", k.path(), file)
	if err != nil || result.Code != 0 {
		return ErrDeclined
	}
	return nil
}

// Untrust takes the trust setting and the certificate out. A root that is not
// there is already gone, and asking the keychain about it would only prompt.
func (k *keychain) Untrust(ctx context.Context, root Root) error {
	listed, err := k.listed(ctx, root)
	if err != nil {
		return err
	}
	if !listed {
		return nil
	}
	file, cleanup, err := privateCopy("ca.pem", root.PEM)
	if err != nil {
		return err
	}
	defer cleanup()
	removed, err := k.run(ctx, securityTool, "remove-trusted-cert", file)
	if err != nil {
		return err
	}
	deleted, err := k.run(ctx, securityTool, "delete-certificate", "-Z", root.SHA1, k.path())
	if err != nil {
		return err
	}
	if removed.Code != 0 || deleted.Code != 0 {
		return ErrDeclined
	}
	if still, err := k.listed(ctx, root); err != nil || still {
		return ErrDeclined
	}
	return nil
}

func (k *keychain) listed(ctx context.Context, root Root) (bool, error) {
	found, err := k.run(ctx, securityTool, "find-certificate", "-a", "-Z", "-c", root.CommonName, k.path())
	if err != nil {
		return false, err
	}
	switch found.Code {
	case 0:
		return listsFingerprint(found.Stdout, root.SHA1), nil
	case 1:
		return false, nil
	default:
		return false, ErrDeclined
	}
}
