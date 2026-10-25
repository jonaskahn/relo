package templates

import (
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/oauth"
	"github.com/jonaskahn/relo/internal/catalog"
)

// The account grant is not a bearer token: it belongs in the harness header on
// the one host it may reach. Pinning the base URL here is what keeps a
// connection that names another host from sending the grant there.
func TestDeepSeekAccountTemplate(t *testing.T) {
	template, found := Get("deepseek-account", nil)
	if !found {
		t.Fatal("deepseek-account template not found")
	}
	if template.Kind != KindSignIn || template.Auth != catalog.AuthOAuth {
		t.Fatalf("kind/auth = %q/%q, want a sign-in account", template.Kind, template.Auth)
	}
	if template.DefaultFormat != catalog.FormatAnthropic {
		t.Errorf("default format = %q, want the Anthropic Messages wire", template.DefaultFormat)
	}
	if template.DefaultBaseURL != oauth.DeepSeekAccountBaseURL {
		t.Errorf("base url = %q, want %q", template.DefaultBaseURL, oauth.DeepSeekAccountBaseURL)
	}
	// The grant authorizes inference and no listing, so the roster is hand
	// entered or the connection would offer models nothing proved reachable.
	if template.ModelsFormat != catalog.ModelsNone || template.ModelsSource != "manual" {
		t.Errorf("models = %q/%q, want a hand-entered roster", template.ModelsSource, template.ModelsFormat)
	}
	if len(template.LoginMethods) != 1 || template.LoginMethods[0].Flow != oauth.FlowDeepSeekAccount {
		t.Fatalf("login methods = %+v, want the account import", template.LoginMethods)
	}
	if template.LoginMethods[0].Kind != LoginCLI {
		t.Errorf("login kind = %q, want the installed-app import", template.LoginMethods[0].Kind)
	}

	authorize, found := Authorizer("deepseek-account")
	if !found {
		t.Fatal("no authorizer for the DeepSeek account")
	}
	auth := authorize(oauth.OAuthCredential{AccessToken: "grant", AccountID: "acct-7"})
	if auth.BaseURL != oauth.DeepSeekAccountBaseURL {
		t.Errorf("authorized base url = %q, want the pinned host", auth.BaseURL)
	}
	if auth.KeyHeader != oauth.DeepSeekAccountHeader {
		t.Errorf("key header = %q, want %q", auth.KeyHeader, oauth.DeepSeekAccountHeader)
	}
	if auth.Token != "grant" {
		t.Errorf("token = %q, want the stored grant", auth.Token)
	}
}
