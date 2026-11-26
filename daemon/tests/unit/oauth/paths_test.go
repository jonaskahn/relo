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
