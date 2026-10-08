package codingclients_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/codingclients"
)

func TestResolveClaudeAuth(t *testing.T) {
	absent := func() (bool, bool) { return false, false }
	home := t.TempDir()
	configDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("create the config dir: %v", err)
	}
	if got := codingclients.ResolveClaudeAuth(configDir, absent); got != codingclients.ClaudeAuthProxy {
		t.Fatalf("ResolveClaudeAuth() = %q, want proxy when nothing is stored", got)
	}

	if err := os.WriteFile(filepath.Join(configDir, ".credentials.json"), []byte(`{"ok":true}`), 0o600); err != nil {
		t.Fatalf("write credentials: %v", err)
	}
	if got := codingclients.ResolveClaudeAuth(configDir, absent); got != codingclients.ClaudeAuthLogin {
		t.Fatalf("ResolveClaudeAuth() = %q, want login when credentials exist", got)
	}

	if err := os.Remove(filepath.Join(configDir, ".credentials.json")); err != nil {
		t.Fatalf("remove credentials: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".claude.json"), []byte(`{"oauthAccount":{"emailAddress":"a@b.c"}}`), 0o600); err != nil {
		t.Fatalf("write claude.json: %v", err)
	}
	if got := codingclients.ResolveClaudeAuth(configDir, absent); got != codingclients.ClaudeAuthLogin {
		t.Fatalf("ResolveClaudeAuth() = %q, want login when oauthAccount is set", got)
	}

	if err := os.WriteFile(filepath.Join(home, ".claude.json"), []byte("{"), 0o600); err != nil {
		t.Fatalf("write a broken claude.json: %v", err)
	}
	if err := os.Remove(filepath.Join(configDir, ".credentials.json")); err != nil && !os.IsNotExist(err) {
		t.Fatalf("remove credentials: %v", err)
	}
	if got := codingclients.ResolveClaudeAuth(configDir, absent); got != codingclients.ClaudeAuthLogin {
		t.Fatalf("ResolveClaudeAuth() = %q, want login when a source cannot be read", got)
	}
	if got := codingclients.ResolveClaudeAuth(configDir, func() (bool, bool) { return false, true }); got != codingclients.ClaudeAuthLogin {
		t.Fatalf("ResolveClaudeAuth() = %q, want login when the keychain cannot be read", got)
	}
	if got := codingclients.ResolveClaudeAuth(t.TempDir(), func() (bool, bool) { return true, false }); got != codingclients.ClaudeAuthLogin {
		t.Fatalf("ResolveClaudeAuth() = %q, want login when the keychain has the item", got)
	}
}
