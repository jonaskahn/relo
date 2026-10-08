package templates

import (
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/oauth"
)

// TestGrokOAuthAuthorizerRoutesToTheCLIProxy covers the transport a Grok
// subscription needs: the token is minted for the Grok CLI's proxy, not the
// public API, so a signed-in request goes there carrying the token-auth header
// the CLI sends.
func TestGrokOAuthAuthorizerRoutesToTheCLIProxy(t *testing.T) {
	authorizer, found := Authorizer("grok")
	if !found {
		t.Fatal("Authorizer(grok) not found")
	}
	auth := authorizer(oauth.OAuthCredential{AccessToken: "token-1"})
	if auth.Token != "token-1" {
		t.Fatalf("token = %q, want the credential's own", auth.Token)
	}
	if auth.BaseURL != "https://cli-chat-proxy.grok.com/v1" {
		t.Fatalf("base url = %q, want the Grok CLI proxy", auth.BaseURL)
	}
	if auth.Headers["x-xai-token-auth"] != "xai-grok-cli" {
		t.Fatalf("x-xai-token-auth = %q, want the CLI scheme", auth.Headers["x-xai-token-auth"])
	}
	if auth.Headers["x-grok-client-version"] == "" {
		t.Fatal("x-grok-client-version is empty, want the client version the proxy expects")
	}
}

// TestGrokTemplateKeepsThePublicBaseURL keeps the stored connection on the
// public endpoint, so an API-key connection to xAI is untouched.
func TestGrokTemplateKeepsThePublicBaseURL(t *testing.T) {
	host, found := Curated("grok")
	if !found {
		t.Fatal("Curated(grok) not found")
	}
	if host.DefaultBaseURL != "https://api.x.ai/v1" {
		t.Fatalf("default base url = %q, want the public xAI endpoint", host.DefaultBaseURL)
	}
}
