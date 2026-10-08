// Token refresh guard: single-flighting OAuth refreshes.
package upstream

import (
	"errors"
	"sync"

	"github.com/jonaskahn/relo/internal/adapters/oauth"
)

// ErrRefreshInFlight reports a refresh that another request is already
// running, which is not a failure: the caller retries against the token the
// other refresh is about to store.
var ErrRefreshInFlight = errors.New("the credential is already refreshing")

type refreshGuard struct {
	mu       sync.Mutex
	inflight map[string]*refreshCall
}

type refreshCall struct {
	done       chan struct{}
	credential oauth.OAuthCredential
	err        error
}

func newRefreshGuard() *refreshGuard {
	return &refreshGuard{inflight: map[string]*refreshCall{}}
}

func (g *refreshGuard) run(id string, fn func() (oauth.OAuthCredential, error)) (oauth.OAuthCredential, error) {
	g.mu.Lock()
	if running, found := g.inflight[id]; found {
		g.mu.Unlock()
		<-running.done
		return running.credential, running.err
	}
	call := &refreshCall{done: make(chan struct{})}
	g.inflight[id] = call
	g.mu.Unlock()
	call.credential, call.err = fn()
	close(call.done)
	g.mu.Lock()
	delete(g.inflight, id)
	g.mu.Unlock()
	return call.credential, call.err
}
