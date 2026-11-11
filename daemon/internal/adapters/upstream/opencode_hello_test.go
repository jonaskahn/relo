package upstream

import (
	"context"
	"encoding/binary"
	"net"
	"testing"
	"time"

	utls "github.com/refraction-networking/utls"
)

// The native client hello captured from OpenCode's own Bun client, read off
// the wire against opencode.ai. The gateway matches the client hello before
// any application bytes, so these are the fields it reads.
var (
	// capturedCipherSuites are the 17 suites Bun offered, in its order.
	capturedCipherSuites = []uint16{
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

	// capturedExtensionOrder are the extension types Bun sent, in its order.
	// The values are the IANA extension numbers; uTLS keeps its own copy
	// unexported.
	capturedExtensionOrder = []uint16{
		0,     // server_name
		23,    // extended_master_secret
		65281, // renegotiation_info
		10,    // supported_groups
		11,    // ec_point_formats
		35,    // session_ticket
		16,    // alpn
		5,     // status_request
		13,    // signature_algorithms
		18,    // sct
		51,    // key_share
		45,    // psk_key_exchange_modes
		43,    // supported_versions
	}
)

// clientHello is the parts of a client hello the gateway's fingerprint reads.
type clientHello struct {
	cipherSuites []uint16
	extensions   []uint16
	alpn         []string
	groups       []uint16
	keyShares    []uint16
	sigAlgos     []uint16
	versions     []uint16
}

// readClientHello parses the client hello the transport sent on the wire. It
// reads the bytes rather than the spec, so it catches a shape the spec says
// one thing and the handshake sends another.
func readClientHello(t *testing.T, raw []byte) clientHello {
	t.Helper()
	if len(raw) < 5 || raw[0] != 0x16 {
		t.Fatalf("first record is not a handshake: %x", raw[:min(4, len(raw))])
	}
	hello := raw[5:]
	if hello[0] != 0x01 {
		t.Fatalf("handshake type = %d, want a client hello", hello[0])
	}
	offset := 4 + 2 + 32
	offset += int(hello[offset]) + 1
	suites := make([]uint16, 0, 32)
	length := int(binary.BigEndian.Uint16(hello[offset:]))
	offset += 2
	for i := 0; i < length; i += 2 {
		suites = append(suites, binary.BigEndian.Uint16(hello[offset+i:]))
	}
	offset += length
	offset += int(hello[offset]) + 1

	var parsed clientHello
	parsed.cipherSuites = suites
	total := int(binary.BigEndian.Uint16(hello[offset:]))
	offset += 2
	end := offset + total
	for offset+4 <= end {
		kind := binary.BigEndian.Uint16(hello[offset:])
		size := int(binary.BigEndian.Uint16(hello[offset+2:]))
		payload := hello[offset+4 : offset+4+size]
		offset += 4 + size
		parsed.extensions = append(parsed.extensions, kind)
		switch kind {
		case 16:
			parsed.alpn = readALPN(payload)
		case 10:
			parsed.groups = readUint16List(payload)
		case 51:
			parsed.keyShares = readKeyShareGroups(payload)
		case 13:
			parsed.sigAlgos = readUint16List(payload)
		case 43:
			parsed.versions = readVersions(payload)
		}
	}
	return parsed
}

// readUint16List reads a list of uint16 values that is prefixed by its byte
// length, the shape supported_groups and signature_algorithms use.
func readUint16List(payload []byte) []uint16 {
	if len(payload) < 2 {
		return nil
	}
	count := int(binary.BigEndian.Uint16(payload))
	out := make([]uint16, 0, count/2)
	for i := 2; i+1 < len(payload); i += 2 {
		out = append(out, binary.BigEndian.Uint16(payload[i:]))
	}
	return out
}

func readALPN(payload []byte) []string {
	if len(payload) < 2 {
		return nil
	}
	count := int(binary.BigEndian.Uint16(payload))
	offset := 2
	out := make([]string, 0, count)
	for i := 0; i < count && offset < len(payload); i++ {
		size := int(payload[offset])
		offset++
		if offset+size > len(payload) {
			break
		}
		out = append(out, string(payload[offset:offset+size]))
		offset += size
	}
	return out
}

// readVersions reads the supported_versions list, which is prefixed by its
// length in bytes rather than in bytes times two.
func readVersions(payload []byte) []uint16 {
	if len(payload) < 1 {
		return nil
	}
	out := make([]uint16, 0, len(payload)/2)
	for i := 1; i+1 < len(payload); i += 2 {
		out = append(out, binary.BigEndian.Uint16(payload[i:]))
	}
	return out
}

func readKeyShareGroups(payload []byte) []uint16 {
	if len(payload) < 2 {
		return nil
	}
	count := int(binary.BigEndian.Uint16(payload))
	offset := 2
	end := 2 + count
	out := make([]uint16, 0, 2)
	for offset+4 <= end {
		group := binary.BigEndian.Uint16(payload[offset:])
		size := int(binary.BigEndian.Uint16(payload[offset+2:]))
		offset += 4 + size
		out = append(out, group)
	}
	return out
}

// captureHelloFrom drives the fingerprint handshake against a loopback
// listener and returns the client hello bytes the transport sent. The
// listener is a plain socket: it reads the hello and hangs up, so the
// handshake cannot complete, which is the point. The hello is what the
// gateway reads, and it is on the wire before the handshake fails.
func captureHelloFrom(t *testing.T) []byte {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = listener.Close() }()

	captured := make(chan []byte, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		buffer := make([]byte, 4096)
		n, _ := conn.Read(buffer)
		captured <- buffer[:n]
	}()

	config := &utls.Config{ServerName: openCodeHost, InsecureSkipVerify: true}
	conn, err := dialOpenCodeTLS(context.Background(), listener.Addr().String(), config)
	if err == nil {
		_ = conn.Close()
	}

	select {
	case raw := <-captured:
		return raw
	case <-time.After(5 * time.Second):
		t.Fatal("no client hello reached the listener")
	}
	return nil
}

// TestClientHelloOnTheWireMatchesTheCapturedNativeHello is the offline proof
// that the transport presents the client's own hello. The expectation is the
// hello captured from OpenCode's Bun client, read off the wire, so a drift in
// the spec, in uTLS, or in the cipher list is caught here rather than by the
// gateway refusing a request hours later.
func TestClientHelloOnTheWireMatchesTheCapturedNativeHello(t *testing.T) {
	hello := readClientHello(t, captureHelloFrom(t))

	if len(hello.cipherSuites) != len(capturedCipherSuites) {
		t.Fatalf("cipher suites = %d, want the %d the native client offered",
			len(hello.cipherSuites), len(capturedCipherSuites))
	}
	for i, suite := range capturedCipherSuites {
		if hello.cipherSuites[i] != suite {
			t.Fatalf("cipher suite %d = %#x, want %#x", i, hello.cipherSuites[i], suite)
		}
	}

	if len(hello.extensions) != len(capturedExtensionOrder) {
		t.Fatalf("extensions = %v, want the %d the native client sent",
			hello.extensions, len(capturedExtensionOrder))
	}
	for i, kind := range capturedExtensionOrder {
		if hello.extensions[i] != kind {
			t.Fatalf("extension %d = %d, want %d", i, hello.extensions[i], kind)
		}
	}

	if len(hello.alpn) != 1 || hello.alpn[0] != "http/1.1" {
		t.Fatalf("ALPN = %v, want [http/1.1]", hello.alpn)
	}

	wantGroups := []uint16{uint16(utls.X25519MLKEM768), uint16(utls.X25519), uint16(utls.CurveP256), uint16(utls.CurveP384)}
	if len(hello.groups) != len(wantGroups) {
		t.Fatalf("supported groups = %v, want %v", hello.groups, wantGroups)
	}
	for i, group := range wantGroups {
		if hello.groups[i] != group {
			t.Fatalf("supported group %d = %d, want %d", i, hello.groups[i], group)
		}
	}

	// The hybrid share first, X25519 as the fallback. The lengths matter too:
	// the hybrid key is what makes the share 1216 bytes.
	if len(hello.keyShares) != 2 {
		t.Fatalf("key shares = %v, want the hybrid group and X25519", hello.keyShares)
	}
	if hello.keyShares[0] != uint16(utls.X25519MLKEM768) || hello.keyShares[1] != uint16(utls.X25519) {
		t.Fatalf("key shares = %v, want %v then %v",
			hello.keyShares, uint16(utls.X25519MLKEM768), uint16(utls.X25519))
	}

	if len(hello.versions) != 2 ||
		hello.versions[0] != utls.VersionTLS13 || hello.versions[1] != utls.VersionTLS12 {
		t.Fatalf("supported versions = %v, want TLS 1.3 then 1.2", hello.versions)
	}
}

// TestClientHelloOnTheWireCarriesTheHybridKeyShare pins the length of the
// hybrid key share. A hello that offers the group but sends a shorter key is
// a different fingerprint, and the length is the only place that shows.
func TestClientHelloOnTheWireCarriesTheHybridKeyShare(t *testing.T) {
	raw := captureHelloFrom(t)
	hello := raw[5:]
	offset := 4 + 2 + 32
	offset += int(hello[offset]) + 1
	offset += int(binary.BigEndian.Uint16(hello[offset:])) + 2
	offset += int(hello[offset]) + 1
	total := int(binary.BigEndian.Uint16(hello[offset:]))
	offset += 2
	end := offset + total
	found := false
	for offset+4 <= end {
		kind := binary.BigEndian.Uint16(hello[offset:])
		size := int(binary.BigEndian.Uint16(hello[offset+2:]))
		payload := hello[offset+4 : offset+4+size]
		offset += 4 + size
		if kind != 51 {
			continue
		}
		count := int(binary.BigEndian.Uint16(payload))
		cursor := 2
		limit := 2 + count
		for cursor+4 <= limit {
			group := binary.BigEndian.Uint16(payload[cursor:])
			keyLength := int(binary.BigEndian.Uint16(payload[cursor+2:]))
			cursor += 4 + keyLength
			if group == uint16(utls.X25519MLKEM768) {
				// 1184 bytes of ML-KEM encapsulation key plus 32 bytes of
				// X25519 key: the hybrid share the native client sent.
				if keyLength != 1216 {
					t.Fatalf("hybrid key share = %d bytes, want 1216", keyLength)
				}
				found = true
			}
		}
	}
	if !found {
		t.Fatal("the client hello carried no hybrid key share")
	}
}
