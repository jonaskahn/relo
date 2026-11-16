// The intercept pair: a loopback CONNECT proxy that terminates TLS for the
// vendor's host and hands every other host to the network untouched.
package proxy

import (
	"bufio"
	"context"
	"crypto/subtle"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	// messagesPath and countTokensPath are the two requests Relo may answer
	// itself. Every other path on the host belongs to the vendor.
	messagesPath    = "/v1/messages"
	countTokensPath = "/v1/messages/count_tokens"
	// maxBodyBytes bounds a request Relo buffers to read its model.
	maxBodyBytes     = 32 << 20
	handshakeTimeout = 30 * time.Second
)

// ErrBadProxyAuth reports a CONNECT without the token the intercept requires.
var ErrBadProxyAuth = errors.New("proxy authorization required")

// Options configure one intercept pair.
type Options struct {
	// Host is the vendor host the pair terminates TLS for.
	Host string
	// Upstream is the address every relayed request is forwarded to.
	Upstream string
	// DataPlaneURL is Relo's own Anthropic surface, where a routed request
	// goes instead of the vendor.
	DataPlaneURL string
	// DataPlaneKey answers the client key a routed request presents. The
	// caller's own credential belongs to the vendor, so it is replaced rather
	// than forwarded: Relo never holds it, and the vendor never sees it.
	DataPlaneKey func(client string) string
	// Token is the secret a client authenticates to the proxy with.
	Token string
	// Authority issues the certificate presented for Host.
	Authority *Authority
	// Routes reports whether a model is one Relo answers. A model it does not
	// claim is relayed to the vendor with the caller's own credential.
	Routes func(model string) bool
	// Bind answers what a native model id is bound to for one client. A bound id is not
	// Relo's by name, so it is read before Routes; an id it does not bind
	// falls through to Routes and then to the vendor.
	Bind func(client, model string) (Binding, bool)
	// Admits reports whether a client's requests are Relo's to route. A client
	// it refuses is relayed to the vendor untouched, whatever it asks for. Nil
	// admits every client.
	Admits func(client string) bool
	// Catalog answers the Relo models added to the model picker the vendor's
	// catalog describes. Nil, or an empty answer, leaves every catalog as the
	// vendor sent it.
	Catalog func() []CatalogRow
	Logger  *slog.Logger
	// Dial dials the vendor. Nil dials the network.
	Dial func(ctx context.Context, network, address string) (net.Conn, error)
}

// Proxy is one running intercept pair.
type Proxy struct {
	opts     Options
	listener net.Listener
	client   *http.Client
	servers  sync.WaitGroup
	// conns are the tunnels still open, so a shutdown closes them rather than
	// waiting for a client that keeps its connection pooled.
	conns sync.Map
}

// New returns an intercept pair over the given options.
func New(opts Options) (*Proxy, error) {
	switch {
	case opts.Host == "":
		return nil, errors.New("proxy: no host to intercept")
	case opts.Upstream == "":
		return nil, errors.New("proxy: no upstream to relay to")
	case opts.Token == "":
		return nil, errors.New("proxy: no token protects the intercept")
	case opts.Authority == nil:
		return nil, errors.New("proxy: no certificate authority")
	}
	if opts.Logger == nil {
		opts.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	transport := &http.Transport{
		Proxy:               nil,
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
		ForceAttemptHTTP2:   true,
		MaxIdleConnsPerHost: 8,
	}
	if opts.Dial != nil {
		transport.DialContext = opts.Dial
	}
	return &Proxy{opts: opts, client: &http.Client{Transport: transport, Timeout: 0}}, nil
}

// Start binds the loopback listener and serves until ctx is cancelled. The
// address is bound before anything else happens, so a caller rewrites a
// client's settings only once the intercept is really listening.
func (p *Proxy) Start(ctx context.Context) error {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("bind the intercept: %w", err)
	}
	p.listener = listener
	p.servers.Add(1)
	go func() {
		defer p.servers.Done()
		<-ctx.Done()
		_ = p.listener.Close()
		p.conns.Range(func(key, _ any) bool {
			if conn, ok := key.(net.Conn); ok {
				_ = conn.Close()
			}
			return true
		})
	}()
	go p.accept()
	return nil
}

// Addr is the loopback address clients point their proxy variable at.
func (p *Proxy) Addr() string {
	if p.listener == nil {
		return ""
	}
	return p.listener.Addr().String()
}

// URL is the authenticated address a client's proxy variable carries.
func (p *Proxy) URL() string {
	if p.Addr() == "" {
		return ""
	}
	return "http://relo:" + p.opts.Token + "@" + p.Addr()
}

// Wait blocks until the listener is closed and every connection is done.
func (p *Proxy) Wait() {
	p.servers.Wait()
}

func (p *Proxy) accept() {
	for {
		conn, err := p.listener.Accept()
		if err != nil {
			return
		}
		p.servers.Add(1)
		go func() {
			defer p.servers.Done()
			p.serveConn(conn)
		}()
	}
}

func (p *Proxy) serveConn(conn net.Conn) {
	p.conns.Store(conn, struct{}{})
	defer func() {
		p.conns.Delete(conn)
		_ = conn.Close()
	}()
	_ = conn.SetDeadline(time.Now().Add(handshakeTimeout))
	// The CONNECT request is read through a buffered reader, which may already
	// hold the first bytes of the stream that follows. Those bytes belong to
	// the tunnel, so the connection handed on reads them back first.
	buffered := bufio.NewReader(conn)
	request, err := http.ReadRequest(buffered)
	if err != nil {
		return
	}
	conn = &bufferedConn{Conn: conn, reader: buffered}
	if request.Method != http.MethodConnect {
		// A client pointed its proxy variable at an https origin still sends
		// CONNECT for every request. A plain request is not a shape this pair
		// serves, and answering it would pretend the network was reachable.
		writeProxyStatus(conn, http.StatusMethodNotAllowed)
		return
	}
	if !p.authorized(request) {
		writeProxyStatus(conn, http.StatusProxyAuthRequired)
		return
	}
	if err := refuseLoopbackTarget(request.Host); err != nil {
		writeProxyStatus(conn, http.StatusForbidden)
		return
	}
	_ = conn.SetDeadline(time.Time{})
	if !p.intercepts(request.Host) {
		p.blind(conn, request.Host)
		return
	}
	p.terminate(conn)
}

func (p *Proxy) authorized(request *http.Request) bool {
	provided := request.Header.Get("Proxy-Authorization")
	const prefix = "Basic "
	if !strings.HasPrefix(provided, prefix) {
		return false
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(provided, prefix))
	if err != nil {
		return false
	}
	_, password, found := strings.Cut(string(decoded), ":")
	if !found {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(password), []byte(p.opts.Token)) == 1
}

// intercepts reports whether this CONNECT carries the host the pair answers
// for. Every other host is spliced straight through to the network.
func (p *Proxy) intercepts(target string) bool {
	host, _, err := net.SplitHostPort(target)
	if err != nil {
		host = target
	}
	return strings.EqualFold(host, p.opts.Host)
}

func (p *Proxy) blind(conn net.Conn, target string) {
	upstream, err := p.dial("tcp", target)
	if err != nil {
		writeProxyStatus(conn, http.StatusBadGateway)
		return
	}
	defer func() { _ = upstream.Close() }()
	if _, err := io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return
	}
	splice(conn, upstream)
}

func (p *Proxy) terminate(conn net.Conn) {
	if _, err := io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return
	}
	tlsConn := tls.Server(conn, &tls.Config{
		Certificates: []tls.Certificate{p.opts.Authority.Leaf()},
		MinVersion:   tls.VersionTLS12,
		NextProtos:   []string{"http/1.1"},
	})
	if err := tlsConn.Handshake(); err != nil {
		return
	}
	server := &http.Server{
		Handler:           http.HandlerFunc(p.handle),
		ReadHeaderTimeout: handshakeTimeout,
	}
	// The server handshakes a *tls.Conn a second time when it is handed one
	// directly, so the tunnel hands over an opaque wrapper: the handshake
	// above is the only one.
	_ = server.Serve(newTunnelListener(tlsConn))
}

func (p *Proxy) dial(network, address string) (net.Conn, error) {
	if p.opts.Dial != nil {
		return p.opts.Dial(context.Background(), network, address)
	}
	return net.DialTimeout(network, address, handshakeTimeout)
}

func copyHeaders(dst, src http.Header) {
	for name, values := range src {
		if hopByHop(name) || strings.EqualFold(name, "proxy-authorization") {
			continue
		}
		for _, value := range values {
			dst.Add(name, value)
		}
	}
}

func hopByHop(name string) bool {
	switch strings.ToLower(name) {
	case "connection", "keep-alive", "proxy-authenticate", "proxy-authorization",
		"te", "trailer", "transfer-encoding", "upgrade", "host", "content-length":
		return true
	default:
		return false
	}
}

func flush(w http.ResponseWriter, body io.Reader) {
	flusher, canFlush := w.(http.Flusher)
	buffer := make([]byte, 32<<10)
	for {
		read, err := body.Read(buffer)
		if read > 0 {
			if _, writeErr := w.Write(buffer[:read]); writeErr != nil {
				return
			}
			if canFlush {
				flusher.Flush()
			}
		}
		if err != nil {
			return
		}
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{"type": "relo_proxy", "message": message},
	})
}

func writeProxyStatus(conn net.Conn, status int) {
	reason := http.StatusText(status)
	_, _ = fmt.Fprintf(conn, "HTTP/1.1 %d %s\r\nContent-Length: 0\r\n\r\n", status, reason)
}

// refuseLoopbackTarget keeps the intercept from being used to reach this
// machine through its own proxy, which would make it an open door.
func refuseLoopbackTarget(target string) error {
	host, _, err := net.SplitHostPort(target)
	if err != nil {
		host = target
	}
	if strings.EqualFold(host, "localhost") {
		return errors.New("proxy: a loopback target is refused")
	}
	if address, err := net.ResolveIPAddr("ip", host); err == nil && address.IP.IsLoopback() {
		return errors.New("proxy: a loopback target is refused")
	}
	return nil
}

// splice copies bytes both ways until either side closes.
func splice(left, right net.Conn) {
	done := make(chan struct{}, 2)
	copyOne := func(dst, src net.Conn) {
		_, _ = io.Copy(dst, src)
		done <- struct{}{}
	}
	go copyOne(left, right)
	go copyOne(right, left)
	<-done
}

// tunnelListener serves one connection, the one a CONNECT tunnel carries, and
// keeps Serve running until that connection is closed. Keeping it open is what
// lets a client reuse the tunnel for its next request; returning early would
// close the tunnel between them.
type tunnelListener struct {
	conn   net.Conn
	once   sync.Once
	closed chan struct{}
}

func newTunnelListener(conn net.Conn) *tunnelListener {
	wrapped := &closeSignalledConn{Conn: &tunnelConn{Conn: conn}, closed: make(chan struct{})}
	return &tunnelListener{conn: wrapped, closed: wrapped.closed}
}

func (l *tunnelListener) Accept() (net.Conn, error) {
	var conn net.Conn
	l.once.Do(func() { conn = l.conn })
	if conn != nil {
		return conn, nil
	}
	<-l.closed
	return nil, net.ErrClosed
}

// Close leaves the tunnel alone: the connection's own close is what signals
// the listener, and closing it here would cut an answer short.
func (l *tunnelListener) Close() error { return nil }

func (l *tunnelListener) Addr() net.Addr { return l.conn.LocalAddr() }

// tunnelConn hides the TLS session from the server, which would otherwise
// handshake it a second time.
type tunnelConn struct {
	net.Conn
}

// closeSignalledConn reports when the server is done with the connection, so
// the listener's next Accept can stop waiting.
type closeSignalledConn struct {
	net.Conn
	once   sync.Once
	closed chan struct{}
}

func (c *closeSignalledConn) Close() error {
	c.once.Do(func() { close(c.closed) })
	return c.Conn.Close()
}

// bufferedConn is a connection whose first reads come from the buffer the
// CONNECT request was parsed out of, so no byte of the tunnel is lost.
type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) { return c.reader.Read(p) }
