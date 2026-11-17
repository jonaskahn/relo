// Proxy-pool selection: one sticky proxy per client connection, rotated only
// when a proxy itself fails or a whole connection walk ends in a limit.
package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/jonaskahn/relo/internal/config"
	appproxy "github.com/jonaskahn/relo/internal/proxy"
)

type proxyConnState struct {
	mu sync.Mutex
	id string
}

type proxyConnKey struct{}

func withProxyConn(ctx context.Context, conn net.Conn) context.Context {
	return context.WithValue(ctx, proxyConnKey{}, &proxyConnState{})
}

func proxyConnFrom(ctx context.Context) *proxyConnState {
	state, _ := ctx.Value(proxyConnKey{}).(*proxyConnState)
	return state
}

func (s *Server) stickyProxy(ctx context.Context) string {
	state := proxyConnFrom(ctx)
	if state == nil {
		return ""
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.id
}

func (s *Server) pinProxy(ctx context.Context, id string) {
	state := proxyConnFrom(ctx)
	if state == nil {
		return
	}
	state.mu.Lock()
	state.id = id
	state.mu.Unlock()
}

func (s *Server) unpinProxy(ctx context.Context, id string) {
	state := proxyConnFrom(ctx)
	if state == nil {
		return
	}
	state.mu.Lock()
	if state.id == id {
		state.id = ""
	}
	state.mu.Unlock()
}

// selectProxy picks the request's proxy. An empty selection means the
// request goes direct, which is today's behavior for an empty pool or an
// unconfigured connection.
func (s *Server) selectProxy(ctx context.Context, use bool, exclude []string) (appproxy.Selection, error) {
	if !use || s.opts.Proxies == nil {
		return appproxy.Selection{}, nil
	}
	items, err := s.opts.Proxies.Items(ctx)
	if err != nil {
		return appproxy.Selection{}, err
	}
	if len(items) == 0 {
		return appproxy.Selection{}, nil
	}
	selection, err := s.opts.Proxies.SelectSticky(ctx, s.stickyProxy(ctx), exclude)
	if errors.Is(err, appproxy.ErrNoEnabled) {
		return appproxy.Selection{}, nil
	}
	return selection, err
}

func (s *Server) coolProxy(ctx context.Context, selection appproxy.Selection, retryAfter time.Duration) {
	if selection.ID == "" || s.opts.Proxies == nil {
		return
	}
	s.opts.Proxies.Cool(selection.ID, retryAfter)
	s.unpinProxy(ctx, selection.ID)
}

func (s *Server) proxyContext(ctx context.Context, selection appproxy.Selection) context.Context {
	if selection.URL == "" {
		return ctx
	}
	return config.WithOutboundProxy(ctx, selection.URL)
}

func (s *Server) proxyCoolingRefusal(w http.ResponseWriter, r *http.Request, anthropic bool, cause error) {
	wait := time.Second
	var all *appproxy.AllCoolingError
	if errors.As(cause, &all) {
		wait = all.RetryAfter
	}
	w.Header().Set("Retry-After", fmt.Sprintf("%d", max(1, int(wait.Seconds()+0.5))))
	if anthropic {
		s.writeAnthropicDocument(w, http.StatusServiceUnavailable, "api_error", s.translator(r).Text("api.settings.proxies.all_cooling", nil))
		return
	}
	s.fail(w, r, refusal{
		Status: http.StatusServiceUnavailable, Code: "proxy_pool_cooling",
		Message: "api.settings.proxies.all_cooling",
	})
}
