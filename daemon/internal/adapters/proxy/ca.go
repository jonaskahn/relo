// The intercept's certificate authority: a process-scoped root a client is
// told to trust through its own environment, and the leaf Relo presents for
// the vendor's host. Nothing here is installed in an OS trust store.
package proxy

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

const (
	caValidity   = 10 * 365 * 24 * time.Hour
	leafValidity = 365 * 24 * time.Hour
	caCommonName = "Relo Proxy Mode CA"
	// serialBits bounds the random serial number every certificate carries.
	serialBits = 128
)

// Authority is the intercept's root and the leaf it issues for one host.
type Authority struct {
	certificate *x509.Certificate
	key         *ecdsa.PrivateKey
	leaf        tls.Certificate
	// CAPath is the PEM file a client is told to trust.
	CAPath string
}

// LoadOrCreateAuthority reads the authority in dir, creating and persisting
// one when the directory holds none. A certificate that cannot be read as an
// authority is an error rather than a silent replacement: replacing it would
// leave a client trusting a root Relo no longer holds the key for.
func LoadOrCreateAuthority(dir, host string) (*Authority, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create %s: %w", dir, err)
	}
	certPath := filepath.Join(dir, "ca.pem")
	keyPath := filepath.Join(dir, "ca.key")
	if _, err := os.Stat(certPath); err == nil {
		return loadAuthority(certPath, keyPath, host)
	}
	return createAuthority(certPath, keyPath, host)
}

func createAuthority(certPath, keyPath, host string) (*Authority, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate the intercept key: %w", err)
	}
	template, err := authorityTemplate(host)
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
	return loadAuthority(certPath, keyPath, host)
}

// authorityTemplate is the root this build mints: one that may only sign for
// the intercepted host, with the constraint marked critical so a client that
// trusts it cannot be talked into trusting anything else.
func authorityTemplate(host string) (*x509.Certificate, error) {
	serial, err := randomSerial()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	return &x509.Certificate{
		SerialNumber:                serial,
		Subject:                     pkix.Name{CommonName: caCommonName, Organization: []string{"Relo"}},
		NotBefore:                   now.Add(-time.Hour),
		NotAfter:                    now.Add(caValidity),
		KeyUsage:                    x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid:       true,
		IsCA:                        true,
		PermittedDNSDomainsCritical: true,
		PermittedDNSDomains:         []string{host},
	}, nil
}

func loadAuthority(certPath, keyPath, host string) (*Authority, error) {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", certPath, err)
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", keyPath, err)
	}
	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil {
		return nil, fmt.Errorf("%s holds no certificate", certPath)
	}
	certificate, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", certPath, err)
	}
	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, fmt.Errorf("%s holds no key", keyPath)
	}
	key, err := x509.ParseECPrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", keyPath, err)
	}
	leaf, err := issueLeaf(certificate, key, host)
	if err != nil {
		return nil, err
	}
	return &Authority{certificate: certificate, key: key, leaf: leaf, CAPath: certPath}, nil
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

// Leaf is the certificate the terminator presents for its host.
func (a *Authority) Leaf() tls.Certificate {
	return a.leaf
}

// Certificate is the root, as a client's trust check reads it.
func (a *Authority) Certificate() *x509.Certificate {
	return a.certificate
}

// HostsListed reports whether the authority is constrained to host alone,
// which is what keeps a trusted root from vouching for anything else.
func (a *Authority) HostsListed(host string) bool {
	return len(a.certificate.PermittedDNSDomains) == 1 &&
		a.certificate.PermittedDNSDomains[0] == host &&
		a.certificate.PermittedDNSDomainsCritical
}
