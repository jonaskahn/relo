package platform

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/jonaskahn/relo/internal/adapters/ostrust"
	"github.com/jonaskahn/relo/internal/adapters/proxy"
)

// trustRoot describes Relo's root to an OS store: its certificate and
// fingerprints, and a leaf for one host that proves the trust, in the user's own
// store or system-wide.
func trustRoot(authority *proxy.Authority, host string, scope ostrust.Scope) (ostrust.Root, error) {
	leaf, err := authority.LeafPEM(host)
	if err != nil {
		return ostrust.Root{}, err
	}
	return ostrust.Root{
		PEM: authority.PEM(), SHA1: authority.SHA1(), SHA256: authority.SHA256(), CommonName: proxy.CommonName,
		Host: host, LeafPEM: leaf, Scope: scope,
	}, nil
}

// retractTimeout bounds the whole retraction, which may wait on an operator at an
// approval dialog.
const retractTimeout = 2 * time.Minute

// RetractTrust takes Relo's root out of the OS's trust, the user's own and
// system-wide, because Relo is being removed from this machine. It is for the
// uninstall, which has no running picker to ask. It does nothing where Relo never
// created a root, and it never creates one. Each scope it could not clear is
// answered with the steps to clear it by hand.
func RetractTrust(ctx context.Context, home string) []ostrust.ManualSteps {
	switch runtime.GOOS {
	case "darwin", "windows", "linux":
	default:
		return nil
	}
	directory := AuthorityDir(home)
	certPath := filepath.Join(directory, "ca.pem")
	if _, err := os.Stat(certPath); err != nil {
		return nil
	}
	authority, err := proxy.LoadOrCreateAuthority(directory, proxy.InterceptHosts...)
	if err != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, retractTimeout)
	defer cancel()
	store := ostrust.New(runtime.GOOS, homeDirectory(""), nil)
	var left []ostrust.ManualSteps
	for _, scope := range []ostrust.Scope{ostrust.ScopeUser, ostrust.ScopeSystem} {
		root, err := trustRoot(authority, proxy.HostAnthropicAPI, scope)
		if err != nil {
			continue
		}
		if store.Untrust(ctx, root) != nil {
			left = append(left, ostrust.Manual(runtime.GOOS, root, certPath))
		}
	}
	return left
}
