// Proxy mode's runtime: the intercept pair one integration is pointed at,
// started once and kept until that integration is turned off.
package platform

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/jonaskahn/relo/internal/adapters/proxy"
	appintegration "github.com/jonaskahn/relo/internal/application/integration"
)

var _ appintegration.ProxyRuntime = (*ProxyRuntime)(nil)

// ProxyRuntime starts and stops the intercept proxy mode writes into a
// client's settings. One pair runs per integration, on a loopback port the
// kernel picks, so nothing else on the machine has to be coordinated.
type ProxyRuntime struct {
	home   string
	logger *slog.Logger
	// routes reports whether a model is one Relo answers. A model it does not
	// claim is relayed to the vendor with the caller's own credential.
	routes func(model string) bool
	// bind answers what a native model id is bound to for one client.
	bind func(client, model string) (proxy.Binding, bool)
	// catalog answers the models the picker gains, and invalidate drops the
	// catalog a client cached, which is stale whenever the intercept starts or
	// stops.
	catalog    func() []proxy.CatalogRow
	invalidate func()

	mu      sync.Mutex
	running map[string]*runningProxy
	// cancelled ends every pair when the daemon stops.
	cancelled context.Context
}

type runningProxy struct {
	proxy  *proxy.Proxy
	cancel context.CancelFunc
	env    appintegration.ProxyEnv
}

// ProxyRuntimeOptions configure the intercept runtime.
type ProxyRuntimeOptions struct {
	// Home is the state directory the intercept's certificate lives in.
	Home string
	// Routes reports whether a model is one Relo serves.
	Routes func(model string) bool
	// Bind answers what a native model id is bound to for one integration.
	Bind func(integrationID, model string) (proxy.Binding, bool)
	// Catalog answers the models added to Claude Code's picker.
	Catalog func() []proxy.CatalogRow
	// Invalidate drops the catalog Claude Code cached.
	Invalidate func()
	Logger     *slog.Logger
}

// NewProxyRuntime returns the intercept runtime.
func NewProxyRuntime(options ProxyRuntimeOptions) *ProxyRuntime {
	logger := options.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &ProxyRuntime{
		home: options.Home, logger: logger, routes: options.Routes, bind: options.Bind,
		catalog: options.Catalog, invalidate: options.Invalidate,
		running: map[string]*runningProxy{}, cancelled: context.Background(),
	}
}

// Ensure starts the intercept for one integration, or returns the address of
// the one already running. The listener is bound before the address is
// answered, so a caller writes settings only once the proxy is really up.
func (r *ProxyRuntime) Ensure(ctx context.Context, id string, target appintegration.ProxyTarget) (appintegration.ProxyEnv, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if running, found := r.running[id]; found {
		return running.env, nil
	}
	dir := filepath.Join(r.home, "proxy-mode", id)
	authority, err := proxy.LoadOrCreateAuthority(dir, target.Host)
	if err != nil {
		return appintegration.ProxyEnv{}, err
	}
	pair, err := r.pair(dir, target, authority)
	if err != nil {
		return appintegration.ProxyEnv{}, err
	}
	runCtx, cancel := context.WithCancel(r.cancelled)
	if err := pair.Start(runCtx); err != nil {
		cancel()
		return appintegration.ProxyEnv{}, err
	}
	rememberAddress(dir, pair.Addr(), r.logger)
	env := appintegration.ProxyEnv{URL: pair.URL(), CAPath: authority.CAPathFile()}
	r.running[id] = &runningProxy{proxy: pair, cancel: cancel, env: env}
	r.dropCatalog()
	r.logger.Info("proxy mode intercept listening", "integration", id, "address", pair.Addr())
	return env, nil
}

// pair builds one intercept over the certificate and the token in dir. The
// token is read from disk rather than held, so the address a client carries
// survives a restart.
func (r *ProxyRuntime) pair(dir string, target appintegration.ProxyTarget, authority *proxy.Authority) (*proxy.Proxy, error) {
	token, err := proxyToken(dir)
	if err != nil {
		return nil, err
	}
	return proxy.New(proxy.Options{
		Host: target.Host, Upstream: "https://" + target.Host, DataPlaneURL: target.DataPlaneURL,
		Token: token, Authority: authority, Routes: r.routes, Bind: r.bind, Catalog: r.catalog, Logger: r.logger,
		DataPlaneKey: target.Key, Admits: target.Admits, PreferredAddr: lastAddress(dir),
	})
}

func (r *ProxyRuntime) dropCatalog() {
	if r.invalidate != nil {
		r.invalidate()
	}
}

// Status reports the intercept's loopback address, and whether it is up.
func (r *ProxyRuntime) Status(id string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	running, found := r.running[id]
	if !found {
		return "", false
	}
	return running.proxy.Addr(), true
}

// Retire stops one integration's intercept and removes the certificate a
// client was told to trust, so a removed integration leaves nothing behind.
func (r *ProxyRuntime) Retire(id string) error {
	r.halt(id)
	r.dropCatalog()
	dir := filepath.Join(r.home, "proxy-mode", id)
	if err := os.RemoveAll(dir); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// halt stops one integration's listener and waits for its connections.
func (r *ProxyRuntime) halt(id string) {
	r.mu.Lock()
	running, found := r.running[id]
	delete(r.running, id)
	r.mu.Unlock()
	if found {
		running.cancel()
		running.proxy.Wait()
	}
}

// Stop ends every intercept, which is what a daemon shutdown does. The token
// and the certificate stay on disk: the client's settings carry that token and
// trust that certificate, so the next start must find both again.
func (r *ProxyRuntime) Stop() {
	r.mu.Lock()
	ids := make([]string, 0, len(r.running))
	for id := range r.running {
		ids = append(ids, id)
	}
	r.mu.Unlock()
	for _, id := range ids {
		r.halt(id)
	}
}

// lastAddress is the address the intercept listened on last time. A session a
// client holds open names that port, so a restart tries it first and a client
// only needs restarting when something else has taken it.
func lastAddress(dir string) string {
	content, err := os.ReadFile(filepath.Join(dir, "address"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(content))
}

// rememberAddress records where the intercept listens. A failed write costs
// only the next restart its port, so it is logged and not raised.
func rememberAddress(dir, address string, logger *slog.Logger) {
	if lastAddress(dir) == address {
		return
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		logger.Warn("remember the intercept's address", "error", err)
		return
	}
	if err := os.WriteFile(filepath.Join(dir, "address"), []byte(address+"\n"), 0o600); err != nil {
		logger.Warn("remember the intercept's address", "error", err)
	}
}

// proxyToken reads the intercept's token, creating one on first use. The
// client's proxy address carries it, so it is re-read from disk rather than
// being held only in memory.
func proxyToken(dir string) (string, error) {
	path := filepath.Join(dir, "token")
	content, err := os.ReadFile(path)
	if err == nil {
		if token := strings.TrimSpace(string(content)); token != "" {
			return token, nil
		}
	}
	token, err := newProxyToken()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(token+"\n"), 0o600); err != nil {
		return "", err
	}
	return token, nil
}

func newProxyToken() (string, error) {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}
