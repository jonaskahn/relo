// Shutdown closes serving immediately and bounds the remaining record writes.
package server

import (
	"context"
	"errors"
	"net/http"
	"time"
)

func (s *Server) closeServing() {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.stopping = true
		s.stopDeadline = time.Now().Add(RecordSaveTimeout)
		servers := append([]*http.Server{}, s.servers...)
		s.mu.Unlock()
		s.cancel()
		time.AfterFunc(RecordSaveTimeout, s.cancelPersistence)
		for _, listener := range servers {
			s.closeErr = errors.Join(s.closeErr, listener.Close())
		}
	})
}

// Shutdown closes every connection and allows at most 500 ms for final records.
// Repeated callers share the same teardown and never extend its deadline.
func (s *Server) Shutdown(ctx context.Context) error {
	s.closeServing()
	s.shutdownOnce.Do(func() {
		go func() {
			s.opts.Logger.Info("daemon stopping", "cause", "serving closed")
			defer close(s.shutdownDone)
			defer s.cancelPersistence()
			budget, cancel := context.WithDeadline(context.Background(), s.stopDeadline)
			defer cancel()
			handlers := make(chan struct{})
			go func() { s.requests.Wait(); close(handlers) }()
			select {
			case <-handlers:
			case <-budget.Done():
			}
			err := s.bookkeeping.stop(budget)
			// Expired advisory writes do not make an intentional stop a failed run.
			if errors.Is(err, context.DeadlineExceeded) {
				err = nil
			}
			s.shutdownErr = errors.Join(s.closeErr, err)
		}()
	})
	select {
	case <-s.shutdownDone:
		return s.shutdownErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Server) admit() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopping {
		return false
	}
	s.requests.Add(1)
	return true
}

func (s *Server) trackRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.admit() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		defer s.requests.Done()
		ctx, cancel := context.WithCancel(r.Context())
		detach := context.AfterFunc(s.lifetime, cancel)
		defer detach()
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) runOperation(run func(context.Context)) {
	if !s.admit() {
		return
	}
	go func() { defer s.requests.Done(); run(s.lifetime) }()
}

type persistenceValues struct {
	context.Context
	values context.Context
}

func (c persistenceValues) Value(key any) any { return c.values.Value(key) }

func (s *Server) recordContext(ctx context.Context) context.Context {
	return persistenceValues{Context: s.persistence, values: context.WithoutCancel(ctx)}
}
