// The picker's egress proxy: the address Claude Desktop is told to send its
// network traffic through. It terminates TLS for claude.ai, to add Relo's models
// to the bootstrap the app reads its model picker from, and splices every other
// connection through untouched.
package proxy

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"
)

// PickerOptions configure one picker proxy.
type PickerOptions struct {
	// Upstream is where claude.ai's requests are relayed to.
	Upstream string
	// Authority issues the certificate presented for claude.ai.
	Authority *Authority
	// Catalog answers the Relo models added to the picker. Nil, or an empty
	// answer, leaves every bootstrap as the vendor sent it.
	Catalog func() []CatalogRow
	// Armed reports whether claude.ai is terminated at all. While it is false the
	// host is spliced through like every other, so the app never meets a
	// certificate its operating system does not trust. Nil is always armed.
	Armed func() bool
	// PreferredAddr is a loopback address to bind first, so the port Desktop's
	// profile names survives a restart.
	PreferredAddr string
	Logger        *slog.Logger
	// Dial dials a spliced host. Nil dials the network.
	Dial func(ctx context.Context, network, address string) (net.Conn, error)
	// Transport relays claude.ai's requests. Nil is a verified HTTPS client.
	Transport http.RoundTripper
}

// PickerProxy is one running picker proxy.
type PickerProxy struct {
	opts     PickerOptions
	listener net.Listener
	handler  http.Handler
	servers  sync.WaitGroup
	conns    sync.Map
}

// NewPicker returns a picker proxy over the given options.
func NewPicker(opts PickerOptions) (*PickerProxy, error) {
	switch {
	case opts.Upstream == "":
		return nil, errors.New("picker: no upstream to relay to")
	case opts.Authority == nil:
		return nil, errors.New("picker: no certificate authority")
	}
	target, err := url.Parse(opts.Upstream)
	if err != nil || target.Host == "" {
		return nil, fmt.Errorf("picker: the upstream %q is not an address", opts.Upstream)
	}
	if opts.Logger == nil {
		opts.Logger = slog.New(slog.DiscardHandler)
	}
	if opts.Transport == nil {
		opts.Transport = &http.Transport{
			Proxy: nil, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
			ForceAttemptHTTP2: true, MaxIdleConnsPerHost: 8,
		}
	}
	p := &PickerProxy{opts: opts}
	p.handler = p.reverse(target)
	return p, nil
}

// Start binds the loopback listener and serves until ctx is cancelled.
func (p *PickerProxy) Start(ctx context.Context) error {
	listener, err := listenLoopback(p.opts.PreferredAddr)
	if err != nil {
		return fmt.Errorf("bind the picker proxy: %w", err)
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

// Addr is the loopback address Desktop's profile names.
func (p *PickerProxy) Addr() string {
	if p.listener == nil {
		return ""
	}
	return p.listener.Addr().String()
}

// Wait blocks until the listener is closed and every connection is done.
func (p *PickerProxy) Wait() { p.servers.Wait() }

func (p *PickerProxy) accept() {
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

func (p *PickerProxy) armed() bool {
	return p.opts.Armed == nil || p.opts.Armed()
}

// serveConn answers one CONNECT. The proxy carries no credential, because a
// Desktop profile cannot hold one, so it answers loopback peers only, never a
// loopback target, and nothing but CONNECT.
func (p *PickerProxy) serveConn(conn net.Conn) {
	p.conns.Store(conn, struct{}{})
	defer func() {
		p.conns.Delete(conn)
		_ = conn.Close()
	}()
	if !loopbackPeer(conn) {
		return
	}
	_ = conn.SetDeadline(time.Now().Add(handshakeTimeout))
	buffered := bufio.NewReader(conn)
	request, err := http.ReadRequest(buffered)
	if err != nil {
		return
	}
	conn = &bufferedConn{Conn: conn, reader: buffered}
	if request.Method != http.MethodConnect {
		writeProxyStatus(conn, http.StatusMethodNotAllowed)
		return
	}
	if refuseLoopbackTarget(request.Host) != nil {
		writeProxyStatus(conn, http.StatusForbidden)
		return
	}
	_ = conn.SetDeadline(time.Time{})
	if p.terminates(request.Host) {
		p.terminate(conn)
		return
	}
	p.blind(conn, request.Host)
}

func loopbackPeer(conn net.Conn) bool {
	address, ok := conn.RemoteAddr().(*net.TCPAddr)
	return ok && address.IP.IsLoopback()
}

func (p *PickerProxy) terminates(target string) bool {
	host, _, err := net.SplitHostPort(target)
	if err != nil {
		host = target
	}
	return strings.EqualFold(host, HostClaudeAI) && p.armed()
}

func (p *PickerProxy) blind(conn net.Conn, target string) {
	var upstream net.Conn
	var err error
	if p.opts.Dial != nil {
		upstream, err = p.opts.Dial(context.Background(), "tcp", target)
	} else {
		upstream, err = net.DialTimeout("tcp", target, handshakeTimeout)
	}
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

// terminate speaks TLS for claude.ai with the shared root's leaf. The app asks
// for HTTP/2 and gets it: the *tls.Conn goes to the server as it is, so the
// server reads the protocol the handshake agreed.
func (p *PickerProxy) terminate(conn net.Conn) {
	if _, err := io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return
	}
	// The wrapper sits under TLS, so the server still receives the *tls.Conn
	// itself and the wrapper learns when that connection is closed, even if the
	// server hands it to an upgraded stream.
	signalled := &closeSignalledConn{Conn: conn, closed: make(chan struct{})}
	tlsConn := tls.Server(signalled, &tls.Config{
		GetCertificate: p.opts.Authority.GetCertificate,
		MinVersion:     tls.VersionTLS12,
		NextProtos:     []string{"h2", "http/1.1"},
	})
	if err := tlsConn.Handshake(); err != nil {
		return
	}
	server := &http.Server{Handler: p.handler, ReadHeaderTimeout: handshakeTimeout}
	_ = server.Serve(&ownedConnListener{conn: tlsConn, closed: signalled.closed})
}

// reverse relays a request to claude.ai. Streams are flushed as they arrive and
// an upgrade is switched, so nothing but the bootstrap is changed in transit.
func (p *PickerProxy) reverse(target *url.URL) http.Handler {
	return &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(target)
			r.Out.Host = target.Host
			if isDesktopBootstrap(r.In) {
				// Dropping the header lets the transport negotiate gzip itself and hand
				// back decoded bytes to rewrite.
				r.Out.Header.Del("Accept-Encoding")
			}
		},
		Transport:      p.opts.Transport,
		FlushInterval:  -1,
		ModifyResponse: p.modify,
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) {
			writeError(w, http.StatusBadGateway, "the upstream is unavailable")
		},
	}
}

// modify adds Relo's rows to a good bootstrap. Anything else about the response,
// and every bootstrap it cannot change, goes through as it came.
func (p *PickerProxy) modify(response *http.Response) error {
	if !p.armed() || p.opts.Catalog == nil || response.StatusCode != http.StatusOK ||
		!isDesktopBootstrap(response.Request) || response.Header.Get("Content-Encoding") != "" ||
		!strings.Contains(response.Header.Get("Content-Type"), "json") {
		return nil
	}
	rows := p.opts.Catalog()
	if len(rows) == 0 {
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxCatalogBytes+1))
	if err != nil || len(body) > maxCatalogBytes {
		// Too big or cut short: what was read goes first, the rest follows as it is.
		response.Body = readCloser{Reader: io.MultiReader(bytesReader(body), response.Body), closer: response.Body}
		return nil
	}
	rewritten, added := rewriteDesktopBootstrap(body, rows)
	if added == 0 {
		response.Body = readCloser{Reader: bytesReader(body), closer: response.Body}
		return nil
	}
	p.opts.Logger.Debug("picker bootstrap", "path", response.Request.URL.Path, "added", added)
	response.Body = readCloser{Reader: bytesReader(rewritten), closer: response.Body}
	response.ContentLength = int64(len(rewritten))
	for _, name := range []string{"Etag", "Digest", "Content-Md5", "Content-Length"} {
		response.Header.Del(name)
	}
	return nil
}

func bytesReader(b []byte) io.Reader { return bytes.NewReader(b) }

// readCloser reads from one source and closes another, which is how a body that
// was read and rewritten still releases the connection it came on.
type readCloser struct {
	io.Reader
	closer io.Closer
}

func (r readCloser) Close() error { return r.closer.Close() }

// ownedConnListener hands the server one connection, as it is, and keeps Serve
// running until that connection's transport is closed. The connection stays a
// *tls.Conn so the server reads the protocol the handshake agreed.
type ownedConnListener struct {
	conn   net.Conn
	once   sync.Once
	closed <-chan struct{}
}

func (l *ownedConnListener) Accept() (net.Conn, error) {
	var conn net.Conn
	l.once.Do(func() { conn = l.conn })
	if conn != nil {
		return conn, nil
	}
	<-l.closed
	return nil, net.ErrClosed
}

func (l *ownedConnListener) Close() error { return nil }

func (l *ownedConnListener) Addr() net.Addr { return l.conn.LocalAddr() }
