package upstream

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	utls "github.com/refraction-networking/utls"
)

// The gateway's fingerprint gate is matched before any application bytes, so
// the offline-verifiable behaviour of the zen transport is its dialing
// discipline and the exact client hello it presents: it refuses every host
// that is not opencode.ai, and it completes a handshake only with the Bun
// shape. The live replay against opencode.ai is what proves the gateway still
// accepts it; these tests pin the shape so a regression is caught locally.

func TestDialOpenCodeRefusesAnotherHost(t *testing.T) {
	transport := OpenCodeTransport(http.ProxyFromEnvironment)
	request, err := http.NewRequest("GET", "https://example.com/", nil)
	if err != nil {
		t.Fatalf("build the request: %v", err)
	}
	response, err := transport.RoundTrip(request)
	if response != nil {
		_ = response.Body.Close()
	}
	if err == nil {
		t.Fatal("RoundTrip() error = nil, want the host refusal")
	}
	if !errors.Is(err, errOpenCodeHost) && !strings.Contains(err.Error(), "refuse a host") {
		t.Fatalf("RoundTrip() error = %v, want the host refusal", err)
	}
}

func TestDialOpenCodeOnlyFingerprintsItsOwnHost(t *testing.T) {
	transport := OpenCodeTransport(http.ProxyFromEnvironment)
	if transport.DialTLSContext == nil {
		t.Fatal("OpenCodeTransport has no custom TLS dialer")
	}
	conn, err := transport.DialTLSContext(context.Background(), "tcp", "api.example.org:443")
	if conn != nil {
		_ = conn.Close()
	}
	if err == nil {
		t.Fatal("dialed a host the zen transport must refuse")
	}
	if !errors.Is(err, errOpenCodeHost) {
		t.Fatalf("dial error = %v, want errOpenCodeHost", err)
	}
}

func TestOpenCodeSpecPresentsTheBunFingerprint(t *testing.T) {
	spec := openCodeSpec("opencode.ai")
	if spec == nil {
		t.Fatal("openCodeSpec() = nil")
	}
	if len(spec.CipherSuites) != len(openCodeCipherSuites) {
		t.Fatalf("cipher suites = %d, want %d", len(spec.CipherSuites), len(openCodeCipherSuites))
	}
	if spec.TLSVersMax != utls.VersionTLS13 || spec.TLSVersMin != utls.VersionTLS12 {
		t.Fatalf("versions = %x..%x, want 12..13", spec.TLSVersMin, spec.TLSVersMax)
	}
	// ALPN must offer http/1.1 alone; the fingerprint is matched before any
	// application bytes, and h2 would push the gateway down a different path.
	var alpn *utls.ALPNExtension
	var keyShare *utls.KeyShareExtension
	for _, extension := range spec.Extensions {
		switch extension := extension.(type) {
		case *utls.ALPNExtension:
			alpn = extension
		case *utls.KeyShareExtension:
			keyShare = extension
		}
	}
	if alpn == nil {
		t.Fatal("client hello offers no ALPN")
	}
	if len(alpn.AlpnProtocols) != 1 || alpn.AlpnProtocols[0] != "http/1.1" {
		t.Fatalf("ALPN = %v, want exactly [http/1.1]", alpn.AlpnProtocols)
	}
	if keyShare == nil || len(keyShare.KeyShares) != 1 || keyShare.KeyShares[0].Group != utls.X25519 {
		t.Fatalf("key share = %+v, want one X25519 group", keyShare)
	}
}

// TestDialOpenCodeTLSHandshakesWithALoopbackServer drives the real fingerprint
// handshake against a loopback TLS server and reads the negotiated protocol.
// It proves the Bun-shaped hello is not only syntactically well-formed but
// negotiates http/1.1 against a conforming server, which is what the gateway
// relies on.
func TestDialOpenCodeTLSHandshakesWithALoopbackServer(t *testing.T) {
	certificate := loopbackCertificate(t, "opencode.ai")
	root := x509.NewCertPool()
	root.AddCert(certificate.Leaf)

	listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{certificate},
		MinVersion:   tls.VersionTLS12,
		MaxVersion:   tls.VersionTLS13,
		NextProtos:   []string{"http/1.1"},
	})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = listener.Close() }()

	negotiated := make(chan string, 1)
	done := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			done <- err
			return
		}
		defer func() { _ = conn.Close() }()
		server := conn.(*tls.Conn)
		if err := server.Handshake(); err != nil {
			done <- err
			return
		}
		negotiated <- server.ConnectionState().NegotiatedProtocol
		done <- nil
	}()

	// Pin trust to the loopback certificate; the fingerprint is unchanged.
	config := &utls.Config{ServerName: "opencode.ai", RootCAs: root}

	addr := listener.Addr().String()
	conn, err := dialOpenCodeTLS(context.Background(), addr, config)
	if err != nil {
		t.Fatalf("dialOpenCodeTLS() error = %v", err)
	}
	defer func() { _ = conn.Close() }()
	if conn.RemoteAddr().String() != addr {
		t.Fatalf("RemoteAddr = %q, want %q", conn.RemoteAddr(), addr)
	}
	if err := <-done; err != nil {
		t.Fatalf("server handshake: %v", err)
	}
	if protocol := <-negotiated; protocol != "http/1.1" {
		t.Fatalf("negotiated protocol = %q, want http/1.1", protocol)
	}
}

func TestDialOpenCodeRejectsAMalformedAddress(t *testing.T) {
	if _, err := dialOpenCode(context.Background(), "not-a-host-port"); err == nil {
		t.Fatal("dialOpenCode() error = nil, want a split-address failure")
	}
}

func TestDialOpenCodeTLSReportsAFailedHandshake(t *testing.T) {
	// A plain TCP listener that never speaks TLS: the client sends its hello
	// and the handshake cannot complete.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = listener.Close() }()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()
	if _, err := dialOpenCodeTLS(context.Background(), listener.Addr().String(), &utls.Config{ServerName: "opencode.ai"}); err == nil {
		t.Fatal("dialOpenCodeTLS() error = nil, want a handshake failure")
	}
}

func TestDialOpenCodeTLSHonorsACancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := dialOpenCodeTLS(ctx, "127.0.0.1:1", &utls.Config{ServerName: "opencode.ai"}); err == nil {
		t.Fatal("dialOpenCodeTLS() error = nil, want a dial failure from the cancelled context")
	}
}

func TestExecutorRoutesOnlyTheFreeLaneToTheZenClient(t *testing.T) {
	plain := &http.Client{}
	zen := &http.Client{}
	executor := NewExecutor(plain, zen, nil)
	if executor.clientFor(&Exchange{}) != plain {
		t.Fatal("a plain exchange must travel over the plain client")
	}
	free := &Exchange{}
	free.Options.TemplateID = "opencode-free"
	if executor.clientFor(free) != zen {
		t.Fatal("the free lane must travel over the zen client")
	}
	executor = NewExecutor(plain, nil, nil)
	if executor.clientFor(free) != plain {
		t.Fatal("a nil zen client must fall back to the plain client")
	}
}

// loopbackCertificate builds a self-signed certificate valid for the given
// DNS names so a loopback handshake can verify against it.
func loopbackCertificate(t *testing.T, dnsName string) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatalf("generate serial: %v", err)
	}
	template := x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: dnsName},
		DNSNames:     []string{dnsName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}
}
