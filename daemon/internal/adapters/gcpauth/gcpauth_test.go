package gcpauth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTokenSource(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.FormValue("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" {
			t.Errorf("unexpected grant_type")
		}
		if r.FormValue("assertion") == "" {
			t.Errorf("missing assertion")
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "ya29.mock_token_12345",
			"expires_in":   3600,
			"token_type":   "Bearer",
		})
	}))
	defer server.Close()

	sa := ServiceAccountKey{
		Type:        "service_account",
		ProjectID:   "test-project-123",
		PrivateKey:  string(pemBytes),
		ClientEmail: "service-acc@test-project-123.iam.gserviceaccount.com",
		TokenURI:    server.URL,
	}

	saJSON, err := json.Marshal(sa)
	if err != nil {
		t.Fatal(err)
	}

	ts, err := NewTokenSource(saJSON, server.Client())
	if err != nil {
		t.Fatalf("NewTokenSource failed: %v", err)
	}

	if ts.ProjectID() != "test-project-123" {
		t.Fatalf("expected project id test-project-123, got %s", ts.ProjectID())
	}

	token, err := ts.Token(context.Background())
	if err != nil {
		t.Fatalf("Token() failed: %v", err)
	}
	if token != "ya29.mock_token_12345" {
		t.Fatalf("unexpected token: %s", token)
	}

	// Verify cached
	tokenCached, err := ts.Token(context.Background())
	if err != nil {
		t.Fatalf("Token() cached failed: %v", err)
	}
	if tokenCached != token {
		t.Fatalf("token not cached")
	}
}

// TestTokenRefusalsAreInspectable covers the key-file and key refusals with
// errors.Is, so a caller can tell a missing field from an unparsable key.
func TestTokenRefusalsAreInspectable(t *testing.T) {
	if _, err := NewTokenSource([]byte(`{"type":"service_account"}`), nil); !errors.Is(err, ErrInvalidServiceAccount) {
		t.Fatalf("NewTokenSource without fields = %v, want ErrInvalidServiceAccount", err)
	}
	if _, err := parsePrivateKey("not a pem block"); !errors.Is(err, ErrUndecodablePEM) {
		t.Fatalf("parsePrivateKey without PEM = %v, want ErrUndecodablePEM", err)
	}
}
