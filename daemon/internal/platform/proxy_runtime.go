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
	// bind answers what a native model id is bound to for one integration.
	bind func(integrationID, model string) (proxy.Binding, bool)

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
	Bind   func(integrationID, model string) (proxy.Binding, bool)
	Logger *slog.Logger
}

// NewProxyRuntime returns the intercept runtime.
func NewProxyRuntime(options ProxyRuntimeOptions) *ProxyRuntime {
	logger := options.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &ProxyRuntime{
		home: options.Home, logger: logger, routes: options.Routes, bind: options.Bind,
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
	pair, err := r.pair(id, dir, target, authority)
	if err != nil {
		return appintegration.ProxyEnv{}, err
	}
	runCtx, cancel := context.WithCancel(r.cancelled)
	if err := pair.Start(runCtx); err != nil {
		cancel()
		return appintegration.ProxyEnv{}, err
	}
	env := appintegration.ProxyEnv{URL: pair.URL(), CAPath: authority.CAPathFile()}
	r.running[id] = &runningProxy{proxy: pair, cancel: cancel, env: env}
	r.logger.Info("proxy mode intercept listening", "integration", id, "address", pair.Addr())
	return env, nil
}

// pair builds one intercept over the certificate and the token in dir. The
// token is read from disk rather than held, so the address a client carries
// survives a restart.
func (r *ProxyRuntime) pair(id, dir string, target appintegration.ProxyTarget, authority *proxy.Authority) (*proxy.Proxy, error) {
	token, err := proxyToken(dir)
	if err != nil {
		return nil, err
	}
	var bind func(model string) (proxy.Binding, bool)
	if r.bind != nil {
		bind = func(model string) (proxy.Binding, bool) { return r.bind(id, model) }
	}
	return proxy.New(proxy.Options{
		Host: target.Host, Upstream: "https://" + target.Host, DataPlaneURL: target.DataPlaneURL,
		Token: token, Authority: authority, Routes: r.routes, Bind: bind, Logger: r.logger,
		DataPlaneKey: target.Key,
	})
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
	r.mu.Lock()
	running, found := r.running[id]
	delete(r.running, id)
	r.mu.Unlock()
	if found {
		running.cancel()
		running.proxy.Wait()
	}
	dir := filepath.Join(r.home, "proxy-mode", id)
	if err := os.RemoveAll(dir); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// Stop ends every intercept, which is what a daemon shutdown does.
func (r *ProxyRuntime) Stop() {
	r.mu.Lock()
	ids := make([]string, 0, len(r.running))
	for id := range r.running {
		ids = append(ids, id)
	}
	r.mu.Unlock()
	for _, id := range ids {
		_ = r.Retire(id)
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
