package ostrust

import (
	"context"
	"fmt"
	"strings"
)

// windowsStore trusts a root in the current user's Root store, which Windows
// answers with its own confirmation dialog. It never touches the machine store.
type windowsStore struct{ run Runner }

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
if (-not (Test-Path 'Cert:\CurrentUser\Root\%s')) { Write-Output 'absent'; exit 0 }
$leaf = New-Object System.Security.Cryptography.X509Certificates.X509Certificate2 (%s)
$chain = New-Object System.Security.Cryptography.X509Certificates.X509Chain
$chain.ChainPolicy.RevocationMode = 'NoCheck'
if (-not $chain.Build($leaf)) { Write-Output 'broken'; exit 0 }
$top = $chain.ChainElements[$chain.ChainElements.Count - 1].Certificate.Thumbprint
if ($top -eq '%s') { Write-Output 'trusted' } else { Write-Output 'broken' }`, root.SHA1, quote(leaf), root.SHA1)
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
	script := fmt.Sprintf(`$ErrorActionPreference='Stop'; Import-Certificate -FilePath %s -CertStoreLocation 'Cert:\CurrentUser\Root' | Out-Null`, quote(file))
	result, err := w.powershell(ctx, script)
	if err != nil || result.Code != 0 {
		return ErrDeclined
	}
	return nil
}

// Untrust removes the root from the current user's Root store, by thumbprint.
func (w *windowsStore) Untrust(ctx context.Context, root Root) error {
	script := fmt.Sprintf(`$ErrorActionPreference='Stop'
$path = 'Cert:\CurrentUser\Root\%s'
if (Test-Path $path) { Remove-Item -LiteralPath $path -Force }
if (Test-Path $path) { exit 3 }`, root.SHA1)
	result, err := w.powershell(ctx, script)
	if err != nil || result.Code != 0 {
		return ErrDeclined
	}
	return nil
}
