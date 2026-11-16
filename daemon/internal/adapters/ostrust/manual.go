package ostrust

import (
	"fmt"
	"strings"
)

// DocsURL is Relo's own page on trusting its certificate by hand.
const DocsURL = "https://github.com/jonaskahn/relo/blob/main/docs/guides/trust-the-relo-certificate.md"

// ManualSteps is what an operator needs to install or remove the certificate
// without Relo's help: where it is, what it must be, and the commands that do
// it. A console shows them and never builds a command of its own.
type ManualSteps struct {
	OS       string
	Scope    Scope
	CertPath string
	SHA1     string
	SHA256   string
	DocsURL  string
	Install  string
	Remove   string
	Check    string
}

// Manual describes the manual install and removal of a root on one OS. The
// commands come from the same builders the automatic path runs, so the two
// cannot disagree.
func Manual(goos string, root Root, certPath string) ManualSteps {
	steps := ManualSteps{
		OS: goos, Scope: root.Scope, CertPath: certPath, SHA1: root.SHA1, SHA256: root.SHA256, DocsURL: DocsURL,
	}
	switch goos {
	case "darwin":
		steps.Install, steps.Remove, steps.Check = darwinManual(root, certPath)
	case "windows":
		steps.Install, steps.Remove, steps.Check = windowsManual(root, certPath)
	case "linux":
		steps.Install, steps.Remove, steps.Check = linuxManual(root, certPath)
	}
	return steps
}

func darwinManual(root Root, certPath string) (install, remove, check string) {
	keychainPath, sudo := "~/Library/Keychains/login.keychain-db", ""
	if root.Scope == ScopeSystem {
		keychainPath, sudo = systemKeychain, "sudo "
	}
	install = sudo + shellLine(append([]string{securityTool}, addTrustedCertArgs(root.Scope, keychainPath, certPath)...)...)
	remove = sudo + shellLine(append([]string{securityTool}, removeTrustedCertArgs(root.Scope, certPath)...)...) +
		"; " + sudo + shellLine(append([]string{securityTool}, deleteCertificateArgs(root.SHA1, keychainPath)...)...)
	check = shellLine(securityTool, "dump-trust-settings") + " | grep " + shellQuote(root.CommonName)
	if root.Scope == ScopeSystem {
		check = shellLine(securityTool, "dump-trust-settings", "-d") + " | grep " + shellQuote(root.CommonName)
	}
	return install, remove, check
}

func windowsManual(root Root, certPath string) (install, remove, check string) {
	user := ""
	if root.Scope != ScopeSystem {
		user = "-user "
	}
	install = fmt.Sprintf(`certutil %s-addstore -f Root "%s"`, user, certPath)
	remove = fmt.Sprintf("certutil %s-delstore Root %s", user, root.SHA1)
	check = fmt.Sprintf("certutil %s-store Root %s", user, root.SHA1)
	return install, remove, check
}

func linuxManual(root Root, certPath string) (install, remove, check string) {
	if root.Scope != ScopeSystem {
		db := "sql:$HOME/.pki/nssdb"
		install = fmt.Sprintf(`certutil -d %s -A -t "C,," -n %s -i %s`, db, shellQuote(root.CommonName), shellQuote(certPath))
		remove = fmt.Sprintf("certutil -d %s -D -n %s", db, shellQuote(root.CommonName))
		return install, remove, fmt.Sprintf("certutil -d %s -L -n %s", db, shellQuote(root.CommonName))
	}
	var installs, removes, checks []string
	for _, family := range anchorFamilies {
		label := map[string]string{"update-ca-certificates": "Debian/Ubuntu", "update-ca-trust": "Fedora/RHEL", "trust": "Arch"}[family.tool]
		installs = append(installs, label+": sudo sh -c "+shellQuote(installScript(family, certPath)))
		removes = append(removes, label+": sudo sh -c "+shellQuote(removeScript(family, certPath)))
		if family.anchor != "" {
			checks = append(checks, label+": sha256sum "+shellQuote(family.anchor))
		} else {
			checks = append(checks, label+": trust list --filter=ca-anchors | grep "+shellQuote(root.CommonName))
		}
	}
	return strings.Join(installs, "\n"), strings.Join(removes, "\n"), strings.Join(checks, "\n")
}
