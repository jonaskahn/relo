package ostrust

import (
	"context"
	"fmt"
	"strings"
)

// windowsStore trusts a root in the current user's Root store, which Windows
// answers with its own confirmation dialog, or in the machine's Root store for
// every user, which Windows answers with its own administrator approval.
type windowsStore struct{ run Runner }

// store is the certificate store a scope's trust lives in.
func store(scope Scope) string {
	if scope == ScopeSystem {
		return `Cert:\LocalMachine\Root`
	}
	return `Cert:\CurrentUser\Root`
}

// elevated runs a script as the administrator through Windows' own approval
// dialog. The script crosses as an encoded command, so it needs no quoting of
// its own, and its exit code comes back. Declining the dialog is ErrDeclined.
func (w *windowsStore) elevated(ctx context.Context, script string) error {
	outer := fmt.Sprintf(`$ErrorActionPreference='Stop'
$p = Start-Process -FilePath 'powershell.exe' -Verb RunAs -Wait -PassThru -WindowStyle Hidden -ArgumentList '-NoProfile','-NonInteractive','-EncodedCommand','%s'
exit $p.ExitCode`, encodePowerShell(script))
	result, err := w.powershell(ctx, outer)
	switch {
	case err != nil:
		return ErrUnavailable
	case result.Code != 0:
		return ErrDeclined
	}
	return nil
}

func (w *windowsStore) powershell(ctx context.Context, script string) (Result, error) {
	return w.run(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script)
}

// quote makes a path safe inside a single-quoted PowerShell string.
func quote(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }

// Inspect reads trust without changing it: the root is in the user's Root store,
// and a chain built for a leaf it issued ends at it.
func (w *windowsStore) Inspect(ctx context.Context, root Root) State {
	leaf, cleanup, err := privateCopy("leaf.pem", root.LeafPEM)
	if err != nil {
		return Unknown
	}
	defer cleanup()
	script := fmt.Sprintf(`$ErrorActionPreference='Stop'
if (-not (Test-Path '%s\%s')) { Write-Output 'absent'; exit 0 }
$leaf = New-Object System.Security.Cryptography.X509Certificates.X509Certificate2 (%s)
$chain = New-Object System.Security.Cryptography.X509Certificates.X509Chain
$chain.ChainPolicy.RevocationMode = 'NoCheck'
if (-not $chain.Build($leaf)) { Write-Output 'broken'; exit 0 }
$top = $chain.ChainElements[$chain.ChainElements.Count - 1].Certificate.Thumbprint
if ($top -eq '%s') { Write-Output 'trusted' } else { Write-Output 'broken' }`, store(root.Scope), root.SHA1, quote(leaf), root.SHA1)
	result, err := w.powershell(ctx, script)
	if err != nil || result.Code != 0 {
		return Unknown
	}
	switch strings.TrimSpace(result.Stdout) {
	case "trusted":
		return Trusted
	case "absent", "broken":
		return Untrusted
	}
	return Unknown
}

// Trust imports the root into the current user's Root store.
func (w *windowsStore) Trust(ctx context.Context, root Root) error {
	file, cleanup, err := privateCopy("ca.pem", root.PEM)
	if err != nil {
		return err
	}
	defer cleanup()
	script := fmt.Sprintf(`$ErrorActionPreference='Stop'; Import-Certificate -FilePath %s -CertStoreLocation '%s' | Out-Null`, quote(file), store(root.Scope))
	if root.Scope == ScopeSystem {
		return w.elevated(ctx, script)
	}
	result, err := w.powershell(ctx, script)
	if err != nil || result.Code != 0 {
		return ErrDeclined
	}
	return nil
}

// absent reports whether the root is certainly not in its store, which needs no
// approval to read, so removing something that is not there never asks for one.
func (w *windowsStore) absent(ctx context.Context, root Root) bool {
	script := fmt.Sprintf(`if (Test-Path '%s\%s') { exit 10 } else { exit 0 }`, store(root.Scope), root.SHA1)
	result, err := w.powershell(ctx, script)
	return err == nil && result.Code == 0
}

// Untrust removes the root from the current user's Root store, by thumbprint.
func (w *windowsStore) Untrust(ctx context.Context, root Root) error {
	script := fmt.Sprintf(`$ErrorActionPreference='Stop'
$path = '%s\%s'
if (Test-Path $path) { Remove-Item -LiteralPath $path -Force }
if (Test-Path $path) { exit 3 }`, store(root.Scope), root.SHA1)
	if root.Scope == ScopeSystem {
		if w.absent(ctx, root) {
			return nil
		}
		return w.elevated(ctx, script)
	}
	result, err := w.powershell(ctx, script)
	if err != nil || result.Code != 0 {
		return ErrDeclined
	}
	return nil
}
