package oauth_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/oauth"
)

func TestLocalCredentialPaths(t *testing.T) {
	t.Run("command code reads the installed cli file from home", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("XDG_DATA_HOME", "")
		directory := filepath.Join(home, ".commandcode")
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatalf("create %s: %v", directory, err)
		}
		if err := os.WriteFile(filepath.Join(directory, "auth.json"), []byte(`{"apiKey":"cc-key"}`), 0o600); err != nil {
			t.Fatalf("write auth file: %v", err)
		}
		server, _ := newProvider(t)
		server.handle("/alpha/whoami", jsonHandler(`{"userId":"user-9"}`))
		flow := oauth.NewCommandCodeFlow(oauth.WithEndpoints(oauth.Endpoints{APIBaseURL: server.url("")}))
		_, credential := loginManual(t, flow, "")
		if credential.AccessToken != "cc-key" {
			t.Fatalf("credential = %+v, want the imported key", credential)
		}
	})

	t.Run("devin reads the cli file from the data home", func(t *testing.T) {
		data := t.TempDir()
		t.Setenv("XDG_DATA_HOME", data)
		directory := filepath.Join(data, "devin")
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatalf("create %s: %v", directory, err)
		}
		credentials := tomlLine("windsurf_api_key", "devin-session-token$xdg")
		if err := os.WriteFile(filepath.Join(directory, "credentials.toml"), []byte(credentials), 0o600); err != nil {
			t.Fatalf("write credentials: %v", err)
		}
		_, credential := loginManual(t, oauth.NewDevinFlow(), "")
		if credential.AccessToken != "devin-session-token$xdg" {
			t.Fatalf("credential = %+v, want the imported token", credential)
		}
	})

	t.Run("meta muse reads the pointer from home", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		directory := filepath.Join(home, ".config", "muse")
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatalf("create %s: %v", directory, err)
		}
		pointer := `{"providers":{"meta":{"storage":"keychain"}}}`
		if err := os.WriteFile(filepath.Join(directory, "auth.json"), []byte(pointer), 0o600); err != nil {
			t.Fatalf("write pointer: %v", err)
		}
		flow := oauth.NewMetaMuseFlow(oauth.WithKeychain(&fakeKeychain{value: "LLM|home-key"}))
		_, credential := loginManual(t, flow, "")
		if credential.AccessToken != "LLM|home-key" {
			t.Fatalf("credential = %+v, want the keychain key", credential)
		}
	})
}

func TestXAIFlowValidate(t *testing.T) {
	flow := oauth.NewXAIDeviceFlow()
	if err := flow.Validate(context.Background(), &oauth.OAuthCredential{AccessToken: "a"}); err != nil {
		t.Fatalf("Validate() error = %v, want a token without an expiry to pass", err)
	}
	if err := flow.Validate(context.Background(), nil); !errors.Is(err, oauth.ErrTokenResponse) {
		t.Fatalf("Validate() error = %v, want %v", err, oauth.ErrTokenResponse)
	}
}
