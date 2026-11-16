package ostrust

import (
	"context"
	"path/filepath"
	"strings"
)

// linuxStore trusts a root in the system's certificate bundle, for every app and
// user, which needs the administrator and is asked for through polkit or a sudo
// that needs no password; or in the user's own NSS database, which Chromium and
// Electron apps read, which needs nothing.
type linuxStore struct {
	home string
	run  Runner
}

// anchorFamily is one way a Linux system keeps extra trusted roots.
type anchorFamily struct {
	tool   string
	anchor string
	bundle string
}

// The families, in the order they are looked for. A tool that is present picks
// the family, because the distribution's name says less than its tools do.
var anchorFamilies = []anchorFamily{
	{tool: "update-ca-certificates", anchor: "/usr/local/share/ca-certificates/relo-local-ca.crt", bundle: "/etc/ssl/certs/ca-certificates.crt"},
	{tool: "update-ca-trust", anchor: "/etc/pki/ca-trust/source/anchors/relo-local-ca.crt", bundle: "/etc/pki/tls/certs/ca-bundle.crt"},
	{tool: "trust", bundle: "/etc/ca-certificates/extracted/tls-ca-bundle.pem"},
}

// has reports whether a command exists on this machine.
func (l *linuxStore) has(ctx context.Context, tool string) bool {
	result, err := l.run(ctx, "sh", "-c", "command -v "+shellQuote(tool))
	return err == nil && result.Code == 0
}

func (l *linuxStore) family(ctx context.Context) (anchorFamily, bool) {
	for _, candidate := range anchorFamilies {
		if l.has(ctx, candidate.tool) {
			return candidate, true
		}
	}
	return anchorFamily{}, false
}

func (l *linuxStore) nssDB() string { return "sql:" + filepath.Join(l.home, ".pki", "nssdb") }

// Inspect reads trust without changing it.
func (l *linuxStore) Inspect(ctx context.Context, root Root) State {
	if root.Scope == ScopeSystem {
		return l.inspectSystem(ctx, root)
	}
	return l.inspectUser(ctx, root)
}

func (l *linuxStore) inspectSystem(ctx context.Context, root Root) State {
	family, found := l.family(ctx)
	if !found {
		return Unknown
	}
	if state := l.installed(ctx, family, root); state != Trusted {
		return state
	}
	leaf, cleanup, err := privateCopy("leaf.pem", root.LeafPEM)
	if err != nil {
		return Unknown
	}
	defer cleanup()
	verified, err := l.run(ctx, "openssl", "verify", "-CAfile", family.bundle, leaf)
	switch {
	case err != nil:
		return Unknown
	case verified.Code != 0:
		return Untrusted
	}
	return Trusted
}

// installed is Trusted when the family's store holds this root: its file, with
// the same fingerprint, or its entry in the p11-kit anchors.
func (l *linuxStore) installed(ctx context.Context, family anchorFamily, root Root) State {
	if family.anchor == "" {
		listed, err := l.run(ctx, "trust", "list", "--filter=ca-anchors")
		switch {
		case err != nil || listed.Code != 0:
			return Unknown
		case !strings.Contains(listed.Stdout, root.CommonName):
			return Untrusted
		}
		return Trusted
	}
	sum, err := l.run(ctx, "sha256sum", family.anchor)
	switch {
	case err != nil:
		return Unknown
	case sum.Code != 0:
		return Untrusted
	}
	if fields := strings.Fields(sum.Stdout); len(fields) == 0 || !strings.EqualFold(fields[0], root.SHA256) {
		return Untrusted
	}
	return Trusted
}

func (l *linuxStore) inspectUser(ctx context.Context, root Root) State {
	if !l.has(ctx, "certutil") {
		return Unknown
	}
	listed, err := l.run(ctx, "certutil", "-d", l.nssDB(), "-L", "-n", root.CommonName)
	switch {
	case err != nil:
		return Unknown
	case listed.Code != 0:
		return Untrusted
	}
	verified, err := l.run(ctx, "certutil", "-d", l.nssDB(), "-V", "-n", root.CommonName, "-u", "C")
	switch {
	case err != nil:
		return Unknown
	case verified.Code != 0:
		return Untrusted
	}
	return Trusted
}

// Trust adds the root to the scope's store.
func (l *linuxStore) Trust(ctx context.Context, root Root) error {
	file, cleanup, err := privateCopy("ca.pem", root.PEM)
	if err != nil {
		return err
	}
	defer cleanup()
	if root.Scope != ScopeSystem {
		return l.trustUser(ctx, root, file)
	}
	family, found := l.family(ctx)
	if !found {
		return ErrUnavailable
	}
	return l.elevate(ctx, installScript(family, file))
}

func (l *linuxStore) trustUser(ctx context.Context, root Root, file string) error {
	if !l.has(ctx, "certutil") {
		return ErrUnavailable
	}
	if listed, err := l.run(ctx, "certutil", "-d", l.nssDB(), "-L"); err == nil && listed.Code != 0 {
		if _, err := l.run(ctx, "certutil", "-d", l.nssDB(), "-N", "--empty-password"); err != nil {
			return ErrUnavailable
		}
	}
	added, err := l.run(ctx, "certutil", "-d", l.nssDB(), "-A", "-t", "C,,", "-n", root.CommonName, "-i", file)
	if err != nil || added.Code != 0 {
		return ErrDeclined
	}
	return nil
}

// Untrust takes the root out of the scope's store and reads it back. A store
// with no tool on this machine holds nothing Relo put there, so there is nothing
// to take out.
func (l *linuxStore) Untrust(ctx context.Context, root Root) error {
	if l.Inspect(ctx, root) == Untrusted {
		return nil
	}
	file, cleanup, err := privateCopy("ca.pem", root.PEM)
	if err != nil {
		return err
	}
	defer cleanup()
	if err := l.remove(ctx, root, file); err != nil {
		return err
	}
	if l.Inspect(ctx, root) == Trusted {
		return ErrDeclined
	}
	return nil
}

// remove runs the removal for the root's scope.
func (l *linuxStore) remove(ctx context.Context, root Root, file string) error {
	if root.Scope == ScopeSystem {
		family, found := l.family(ctx)
		if !found {
			return nil
		}
		return l.elevate(ctx, removeScript(family, file))
	}
	if !l.has(ctx, "certutil") {
		return nil
	}
	if removed, err := l.run(ctx, "certutil", "-d", l.nssDB(), "-D", "-n", root.CommonName); err != nil || removed.Code != 0 {
		return ErrDeclined
	}
	return nil
}

// installScript and removeScript are the shell commands an install and a removal
// run as the administrator. The manual steps are built from the same families.
func installScript(family anchorFamily, file string) string {
	switch family.tool {
	case "update-ca-certificates", "update-ca-trust":
		return shellLine("install", "-m", "0644", file, family.anchor) + " && " + shellQuote(family.tool)
	default:
		return shellLine("trust", "anchor", "--store", file)
	}
}

func removeScript(family anchorFamily, file string) string {
	switch family.tool {
	case "update-ca-certificates":
		return shellLine("rm", "-f", family.anchor) + " && " + shellLine(family.tool, "--fresh")
	case "update-ca-trust":
		return shellLine("rm", "-f", family.anchor) + " && " + shellQuote(family.tool)
	default:
		return shellLine("trust", "anchor", "--remove", file)
	}
}

// elevate runs a shell script as the administrator: through polkit's dialog when
// there is one, else through a sudo that needs no password. Neither is
// ErrUnavailable, and a request that is turned down is ErrDeclined.
func (l *linuxStore) elevate(ctx context.Context, script string) error {
	var result Result
	var err error
	switch {
	case l.has(ctx, "pkexec"):
		result, err = l.run(ctx, "pkexec", "sh", "-c", script)
	case l.passwordlessSudo(ctx):
		result, err = l.run(ctx, "sudo", "-n", "sh", "-c", script)
	default:
		return ErrUnavailable
	}
	if err != nil {
		return ErrUnavailable
	}
	if result.Code != 0 {
		return ErrDeclined
	}
	return nil
}

func (l *linuxStore) passwordlessSudo(ctx context.Context) bool {
	if !l.has(ctx, "sudo") {
		return false
	}
	result, err := l.run(ctx, "sudo", "-n", "true")
	return err == nil && result.Code == 0
}
