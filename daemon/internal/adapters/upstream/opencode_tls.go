// The OpenCode free gateway authenticates the client's TLS/HTTP fingerprint
// rather than the request: it answers FreeUsageLimitError to every transport
// that is not OpenCode's own Bun fetch, even when the headers and body are
// byte-identical. Relo reproduces that fingerprint for opencode.ai only, so a
// free connection works while every other provider keeps the stock transport.
//
// The fingerprint is brittle on purpose and belongs to a deliberate access
// control, not a public contract: if the gateway changes it, the free lane
// quietly answers 429 again and this file is where it would be relearned.
package upstream

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	utls "github.com/refraction-networking/utls"
)

// openCodeHost names the one host this transport is allowed to speak to. The
// dialer refuses anything else, so a mistake can never weaken another vendor's
// connection.
const openCodeHost = "opencode.ai"

// errOpenCodeHost reports a request the zen transport refused to dial, because
// its fingerprint belongs to opencode.ai alone.
var errOpenCodeHost = errors.New("opencode TLS: refuse a host outside opencode.ai")

// openCodeCipherSuites is OpenCode's Bun client hello: TLS 1.3 first, then the
// same ECDHE and RSA suites Bun offers.
var openCodeCipherSuites = []uint16{
	utls.TLS_AES_128_GCM_SHA256,
	utls.TLS_AES_256_GCM_SHA384,
	utls.TLS_CHACHA20_POLY1305_SHA256,
	utls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
	utls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
	utls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
	utls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
	utls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
	utls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
	utls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
	utls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
	utls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
	utls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
	utls.TLS_RSA_WITH_AES_128_GCM_SHA256,
	utls.TLS_RSA_WITH_AES_256_GCM_SHA384,
	utls.TLS_RSA_WITH_AES_128_CBC_SHA,
	utls.TLS_RSA_WITH_AES_256_CBC_SHA,
}

// openCodeSignatureAlgorithms is Bun's ordering: ECDSA-P256, then RSA-PSS and
// PKCS1 RSA, then the longer curves and Ed25519, with SHA1 last.
var openCodeSignatureAlgorithms = []utls.SignatureScheme{
	utls.ECDSAWithP256AndSHA256,
	utls.PSSWithSHA256,
	utls.PKCS1WithSHA256,
	utls.ECDSAWithP384AndSHA384,
	utls.PSSWithSHA384,
	utls.PKCS1WithSHA384,
	utls.PSSWithSHA512,
	utls.PKCS1WithSHA512,
	utls.Ed25519,
	utls.ECDSAWithP521AndSHA512,
	utls.PKCS1WithSHA1,
}

// openCodeSpec is the Bun-shaped client hello. ALPN offers http/1.1 only, and
// the key share leaves the group alone so uTLS generates the X25519 key.
func openCodeSpec(serverName string) *utls.ClientHelloSpec {
	return &utls.ClientHelloSpec{
		TLSVersMin:         utls.VersionTLS12,
		TLSVersMax:         utls.VersionTLS13,
		CipherSuites:       openCodeCipherSuites,
		CompressionMethods: []byte{0},
		Extensions: []utls.TLSExtension{
			&utls.SNIExtension{ServerName: serverName},
			&utls.ExtendedMasterSecretExtension{},
			&utls.RenegotiationInfoExtension{Renegotiation: utls.RenegotiateNever},
			&utls.SupportedCurvesExtension{Curves: []utls.CurveID{utls.X25519, utls.CurveP256, utls.CurveP384}},
			&utls.SupportedPointsExtension{SupportedPoints: []byte{0}},
			&utls.SessionTicketExtension{},
			&utls.ALPNExtension{AlpnProtocols: []string{"http/1.1"}},
			&utls.StatusRequestExtension{},
			&utls.SignatureAlgorithmsExtension{SupportedSignatureAlgorithms: openCodeSignatureAlgorithms},
			&utls.SCTExtension{},
			&utls.KeyShareExtension{KeyShares: []utls.KeyShare{{Group: utls.X25519}}},
			&utls.PSKKeyExchangeModesExtension{Modes: []uint8{1}},
			&utls.SupportedVersionsExtension{Versions: []uint16{utls.VersionTLS13, utls.VersionTLS12}},
		},
	}
}

// dialOpenCode dials a TCP connection to addr and completes a TLS handshake
// that presents Bun's client hello. It speaks only to opencode.ai and only
// http/1.1, because the fingerprint is matched before any application bytes.
func dialOpenCode(ctx context.Context, addr string) (net.Conn, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(host, openCodeHost) {
		return nil, fmt.Errorf("%w: %q", errOpenCodeHost, host)
	}
	return dialOpenCodeTLS(ctx, addr, &utls.Config{ServerName: openCodeHost})
}

// dialOpenCodeTLS dials addr and completes the fingerprint handshake with the
// supplied uTLS config. The config carries the SNI and trust roots; the client
// hello shape comes from openCodeSpec, so a test can pin trust to a loopback
// certificate without changing the fingerprint. It is split from dialOpenCode
// so the handshake can be driven locally, while dialOpenCode stays the one
// host-locked entry point for live traffic.
func dialOpenCodeTLS(ctx context.Context, addr string, config *utls.Config) (net.Conn, error) {
	raw, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	conn := utls.UClient(raw, config, utls.HelloCustom)
	if err := conn.ApplyPreset(openCodeSpec(config.ServerName)); err != nil {
		_ = raw.Close()
		return nil, err
	}
	if err := conn.HandshakeContext(ctx); err != nil {
		_ = raw.Close()
		return nil, err
	}
	return conn, nil
}

// OpenCodeTransport returns a transport whose TLS handshake reproduces the
// OpenCode client fingerprint. Every request it sends is confined to
// opencode.ai by dialOpenCode; callers route only the free lane to it.
func OpenCodeTransport(proxy func(*http.Request) (*url.URL, error)) *http.Transport {
	base := http.DefaultTransport.(*http.Transport).Clone()
	base.Proxy = proxy
	base.DialTLSContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return dialOpenCode(ctx, addr)
	}
	// The fingerprint offers http/1.1 only, so the h2 probe is noise here.
	base.ForceAttemptHTTP2 = false
	return base
}
