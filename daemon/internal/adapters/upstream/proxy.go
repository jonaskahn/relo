// Proxy selection belongs to one upstream request, never to the whole client.
package upstream

import (
	"context"
	"net/http"
	"net/url"
)

type proxyContextKey struct{}

// WithProxy marks an upstream request for the operator's configured proxy.
func WithProxy(ctx context.Context, proxy *url.URL) context.Context {
	return context.WithValue(ctx, proxyContextKey{}, proxy)
}

// RequestProxy returns only the proxy explicitly selected for this request.
func RequestProxy(request *http.Request) (*url.URL, error) {
	proxy, _ := request.Context().Value(proxyContextKey{}).(*url.URL)
	return proxy, nil
}
