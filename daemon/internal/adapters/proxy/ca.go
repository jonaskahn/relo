// Relo's one local certificate authority: a root that may only vouch for the
// hosts Relo intercepts, and the leaf it presents for each. Every client Relo
// wires shares this root, so there is one certificate to trust, inspect and
// remove. A client is told to trust it through its own environment; an OS trust
// store is only touched on an operator's explicit ask.
package proxy

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // an OS lists certificates by SHA-1 fingerprint
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	caValidity   = 10 * 365 * 24 * time.Hour
	leafValidity = 365 * 24 * time.Hour
	// CommonName names Relo's root wherever an OS lists it.
	CommonName = "Relo Local CA"
	// serialBits bounds the random serial number every certificate carries.
	serialBits = 128
)

// The hosts Relo intercepts, and so the hosts its root is constrained to.
const (
	HostAnthropicAPI = "api.anthropic.com"
	HostClaudeAI     = "claude.ai"
)

// InterceptHosts is every host the one root may vouch for. A client added later
// that needs another host extends this list, which mints a new root, because a
// constraint cannot be widened on a certificate already trusted.
var InterceptHosts = []string{HostAnthropicAPI, HostClaudeAI}

// Authority is Relo's root and the leaves it issues, one per host.
type Authority struct {
	certificate *x509.Certificate
	key         *ecdsa.PrivateKey
	hosts       []string
	mu          sync.Mutex
	leaves      map[string]tls.Certificate
	// CAPath is the PEM file a client is told to trust.
	CAPath string
}

// ErrAuthorityUnconstrained reports a stored root that no longer carries the
// constraint it was minted with, or one that covers other hosts than this build
// intercepts. It is refused rather than used: a root an OS trusts without its
// constraint vouches for any host.
var ErrAuthorityUnconstrained = errors.New("proxy: the certificate authority is not constrained to the hosts Relo intercepts")

// LoadOrCreateAuthority reads the authority in dir, creating and persisting
// one when the directory holds none. A certificate that cannot be read as an
// authority is an error rather than a silent replacement: replacing it would
// leave a client trusting a root Relo no longer holds the key for.
func LoadOrCreateAuthority(dir string, hosts ...string) (*Authority, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create %s: %w", dir, err)
	}
	certPath := filepath.Join(dir, "ca.pem")
	keyPath := filepath.Join(dir, "ca.key")
	if _, err := os.Stat(certPath); err == nil {
		return loadAuthority(certPath, keyPath, hosts)
	}
	return createAuthority(certPath, keyPath, hosts)
}

func createAuthority(certPath, keyPath string, hosts []string) (*Authority, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate the intercept key: %w", err)
	}
	template, err := authorityTemplate(hosts)
	if err != nil {
		return nil, err
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("create the intercept certificate: %w", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("encode the intercept key: %w", err)
	}
	if err := writePEM(certPath, "CERTIFICATE", der); err != nil {
		return nil, err
	}
	if err := writePEM(keyPath, "EC PRIVATE KEY", keyDER); err != nil {
		return nil, err
	}
	return loadAuthority(certPath, keyPath, hosts)
}

// authorityTemplate is the root this build mints: one that may only sign for
// the intercepted hosts and refuses every IP address, with the constraint marked
// critical so a client that trusts it cannot be talked into trusting anything
// else.
func authorityTemplate(hosts []string) (*x509.Certificate, error) {
	serial, err := randomSerial()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber:                serial,
		Subject:                     pkix.Name{CommonName: CommonName, Organization: []string{"Relo"}},
		NotBefore:                   now.Add(-time.Hour),
		NotAfter:                    now.Add(caValidity),
		KeyUsage:                    x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid:       true,
		IsCA:                        true,
		PermittedDNSDomainsCritical: true,
		PermittedDNSDomains:         append([]string(nil), hosts...),
		ExcludedIPRanges: []*net.IPNet{
			{IP: net.IPv4zero, Mask: net.CIDRMask(0, 32)},
			{IP: net.IPv6zero, Mask: net.CIDRMask(0, 128)},
		},
	}
	return template, nil
}

func loadAuthority(certPath, keyPath string, hosts []string) (*Authority, error) {
	certificate, err := readCertificate(certPath)
	if err != nil {
		return nil, err
	}
	key, err := readKey(keyPath)
	if err != nil {
		return nil, err
	}
	authority := &Authority{
		certificate: certificate, key: key, hosts: append([]string(nil), hosts...),
		leaves: map[string]tls.Certificate{}, CAPath: certPath,
	}
	if !authority.HostsListed(hosts...) {
		return nil, fmt.Errorf("%s: %w", certPath, ErrAuthorityUnconstrained)
	}
	return authority, nil
}

func readCertificate(path string) (*x509.Certificate, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	block, _ := pem.Decode(content)
	if block == nil {
		return nil, fmt.Errorf("%s holds no certificate", path)
	}
	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return certificate, nil
}

func readKey(path string) (*ecdsa.PrivateKey, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	block, _ := pem.Decode(content)
	if block == nil {
		return nil, fmt.Errorf("%s holds no key", path)
	}
	key, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return key, nil
}

func issueLeaf(authority *x509.Certificate, key *ecdsa.PrivateKey, host string) (tls.Certificate, error) {
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("generate the leaf key: %w", err)
	}
	serial, err := randomSerial()
	if err != nil {
		return tls.Certificate{}, err
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: host},
		DNSNames:     []string{host},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(leafValidity),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, authority, &leafKey.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("issue the intercept leaf: %w", err)
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("parse the intercept leaf: %w", err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: leafKey, Leaf: certificate}, nil
}

func randomSerial() (*big.Int, error) {
	limit := new(big.Int).Lsh(big.NewInt(1), serialBits)
	serial, err := rand.Int(rand.Reader, limit)
	if err != nil {
		return nil, fmt.Errorf("pick a serial number: %w", err)
	}
	return serial, nil
}

func writePEM(path, kind string, der []byte) error {
	block := pem.EncodeToMemory(&pem.Block{Type: kind, Bytes: der})
	if err := os.WriteFile(path, block, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// CAPathFile answers the certificate file a client is told to trust.
func (a *Authority) CAPathFile() string {
	return a.CAPath
}

// LeafFor is the certificate the terminator presents for host. It is issued
// on first use and kept in memory, and refused for a host the root does not
// cover, so a leaf can never be minted that the constraint would reject.
func (a *Authority) LeafFor(host string) (tls.Certificate, error) {
	host = strings.ToLower(host)
	if !a.covers(host) {
		return tls.Certificate{}, fmt.Errorf("proxy: no leaf for %q: %w", host, ErrAuthorityUnconstrained)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if leaf, found := a.leaves[host]; found && time.Now().Before(leaf.Leaf.NotAfter.Add(-24*time.Hour)) {
		return leaf, nil
	}
	leaf, err := issueLeaf(a.certificate, a.key, host)
	if err != nil {
		return tls.Certificate{}, err
	}
	a.leaves[host] = leaf
	return leaf, nil
}

// GetCertificate answers a handshake with the leaf for the name the client asked
// for, which is how one listener serves every intercepted host.
func (a *Authority) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	leaf, err := a.LeafFor(hello.ServerName)
	if err != nil {
		return nil, err
	}
	return &leaf, nil
}

func (a *Authority) covers(host string) bool {
	for _, listed := range a.hosts {
		if host == listed {
			return true
		}
	}
	return false
}

// Certificate is the root, as a client's trust check reads it.
func (a *Authority) Certificate() *x509.Certificate {
	return a.certificate
}

// HostsListed reports whether the root is constrained to exactly these hosts,
// which is what keeps a trusted root from vouching for anything else.
func (a *Authority) HostsListed(hosts ...string) bool {
	c := a.certificate
	if !c.PermittedDNSDomainsCritical || len(c.PermittedDNSDomains) != len(hosts) {
		return false
	}
	for _, host := range hosts {
		if !slices.Contains(c.PermittedDNSDomains, host) {
			return false
		}
	}
	return true
}

// Constrained reports whether the root can vouch for its hosts and nothing else:
// a CA that may only certify, whose critical name constraint permits those hosts
// and excludes every IP address, and that is currently valid. It is what an
// operator's OS is asked to trust, so it is checked before the ask.
func (a *Authority) Constrained() bool {
	c := a.certificate
	if !c.IsCA || !c.BasicConstraintsValid || !a.HostsListed(a.hosts...) {
		return false
	}
	if len(c.PermittedIPRanges) != 0 || len(c.PermittedEmailAddresses) != 0 || len(c.PermittedURIDomains) != 0 {
		return false
	}
	if !excludesEveryIP(c.ExcludedIPRanges) {
		return false
	}
	now := time.Now()
	return now.After(c.NotBefore) && now.Before(c.NotAfter) && c.KeyUsage&x509.KeyUsageCertSign != 0
}

func excludesEveryIP(ranges []*net.IPNet) bool {
	var v4, v6 bool
	for _, block := range ranges {
		ones, bits := block.Mask.Size()
		switch {
		case ones == 0 && bits == 32:
			v4 = true
		case ones == 0 && bits == 128:
			v6 = true
		}
	}
	return v4 && v6
}

// SHA1 is the root's fingerprint as an OS lists it: upper-case hex, no colons.
func (a *Authority) SHA1() string {
	sum := sha1.Sum(a.certificate.Raw) //nolint:gosec // an OS lists certificates by SHA-1 fingerprint
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

// SHA256 is the root's fingerprint an operator can compare.
func (a *Authority) SHA256() string {
	sum := sha256.Sum256(a.certificate.Raw)
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

// PEM is the root as the bytes an OS is asked to trust, read from memory rather
// than from the file, which another process of the same user could swap.
func (a *Authority) PEM() []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: a.certificate.Raw})
}

// LeafPEM is the leaf the root issues for host, as the bytes an OS verifies the
// root's trust against.
func (a *Authority) LeafPEM(host string) ([]byte, error) {
	leaf, err := a.LeafFor(host)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leaf.Certificate[0]}), nil
}
