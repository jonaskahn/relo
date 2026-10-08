package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/access"
	appaccess "github.com/jonaskahn/relo/internal/application/access"
)

// keySummary is one stored key as the API reports it, never with a token.
type keySummary struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	Client     string `json:"client"`
	Hint       string `json:"token_hint"`
	Generation int    `json:"generation"`
	Status     string `json:"status"`
}

// issuedKey is the one response shape that carries a token.
type issuedKey struct {
	Key   keySummary `json:"key"`
	Token string     `json:"token"`
}

func TestAccessKeyAPI(t *testing.T) {
	harness := newHarness(t)

	t.Run("a key is issued and its token returned once", func(t *testing.T) {
		var issued issuedKey
		code := harness.managementJSON(t, http.MethodPost, "/api/v1/clients/keys",
			`{"name":"api-agent","kind":"agent","client":"codex"}`, &issued)
		if code != http.StatusCreated {
			t.Fatalf("status = %d, want 201", code)
		}
		if issued.Token == "" {
			t.Fatal("the create response carried no token")
		}
		if id, _, err := access.Parse(issued.Token); err != nil || id != issued.Key.ID {
			t.Fatalf("the token names %q, want the key id %q", id, issued.Key.ID)
		}
		if issued.Key.Status != access.StatusActive {
			t.Fatalf("status = %q, want active", issued.Key.Status)
		}
	})

	t.Run("the listing never carries a token", func(t *testing.T) {
		response := harness.management(http.MethodGet, "/api/v1/clients/keys", adminToken, nil)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", response.Code)
		}
		body := response.Body.String()
		if strings.Contains(body, "rlo_ak_") {
			t.Fatalf("the listing carries a token: %s", body)
		}
		if !strings.Contains(body, "api-agent") || !strings.Contains(body, "token_hint") {
			t.Fatalf("body = %s, want the key and its hint", body)
		}
	})

	t.Run("a shared key refuses a client", func(t *testing.T) {
		code := harness.managementJSON(t, http.MethodPost, "/api/v1/clients/keys",
			`{"name":"team","kind":"shared","client":"codex"}`, nil)
		if code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", code)
		}
	})

	t.Run("an unknown client is refused", func(t *testing.T) {
		code := harness.managementJSON(t, http.MethodPost, "/api/v1/clients/keys",
			`{"name":"odd","kind":"agent","client":"nope"}`, nil)
		if code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", code)
		}
	})

	t.Run("a duplicate name is refused", func(t *testing.T) {
		code := harness.managementJSON(t, http.MethodPost, "/api/v1/clients/keys",
			`{"name":"api-agent","kind":"agent","client":"codex"}`, nil)
		if code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", code)
		}
	})

	t.Run("an unknown field is refused", func(t *testing.T) {
		code := harness.managementJSON(t, http.MethodPost, "/api/v1/clients/keys",
			`{"name":"x","kind":"shared","nope":true}`, nil)
		if code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", code)
		}
	})

	t.Run("a key is renamed and its expiry set", func(t *testing.T) {
		id := harness.listAccessKeys(t)[0].ID
		var updated keySummary
		code := harness.managementJSON(t, http.MethodPatch, "/api/v1/clients/keys/"+id,
			`{"name":"renamed"}`, &updated)
		if code != http.StatusOK {
			t.Fatalf("status = %d, want 200", code)
		}
		if updated.Name != "renamed" || updated.ID != id {
			t.Fatalf("updated = %+v, want the same key with the new name", updated)
		}
	})

	t.Run("rotating returns a new token and retires the old one", func(t *testing.T) {
		id := harness.createKey("rotate-me", appaccess.NewAccessKey{
			Name: "rotate-me", Kind: appaccess.Agent, Client: access.CustomClient,
		}).ID
		first := harness.createKey("unused", appaccess.NewAccessKey{
			Name: "unused", Kind: appaccess.Agent, Client: access.CustomClient,
		})
		var issued issuedKey
		code := harness.managementJSON(t, http.MethodPost, "/api/v1/clients/keys/"+id+"/rotate", "", &issued)
		if code != http.StatusOK {
			t.Fatalf("status = %d, want 200", code)
		}
		if issued.Token == "" || issued.Key.ID != id {
			t.Fatalf("rotated = %+v, want the same key with a new token", issued)
		}
		if issued.Key.Generation != 2 {
			t.Fatalf("generation = %d, want 2", issued.Key.Generation)
		}
		if first.Token == issued.Token {
			t.Fatal("another key's token came back from the rotation")
		}
	})

	t.Run("an unknown key is not found", func(t *testing.T) {
		code := harness.managementJSON(t, http.MethodPatch, "/api/v1/clients/keys/nope", `{"name":"x"}`, nil)
		if code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", code)
		}
	})

	t.Run("deleting revokes the key and the data plane stops accepting it", func(t *testing.T) {
		issued := harness.createKey("doomed", appaccess.NewAccessKey{
			Name: "doomed", Kind: appaccess.Agent, Client: access.CustomClient,
		})
		if response := harness.dataPlane(http.MethodPost, "/v1/chat/completions", issued.Token, strings.NewReader(streamingBody())); response.Code != http.StatusOK {
			t.Fatalf("status = %d, want the key accepted before revocation", response.Code)
		}
		code := harness.managementJSON(t, http.MethodDelete, "/api/v1/clients/keys/"+issued.ID, "", nil)
		if code != http.StatusNoContent {
			t.Fatalf("status = %d, want 204", code)
		}
		if response := harness.dataPlane(http.MethodPost, "/v1/chat/completions", issued.Token, strings.NewReader(streamingBody())); response.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401 after revocation", response.Code)
		}
	})

	t.Run("expired deletions remove only expired operator keys", func(t *testing.T) {
		expired := harness.createKey("api-lapsed", appaccess.NewAccessKey{
			Name: "api-lapsed", Kind: appaccess.Shared,
			ExpiresAtMs: harness.clock.Now().Add(time.Hour).UnixMilli(),
		})
		revoked := harness.createKey("api-retired", appaccess.NewAccessKey{
			Name: "api-retired", Kind: appaccess.Shared,
		})
		code := harness.managementJSON(t, http.MethodDelete, "/api/v1/clients/keys/"+revoked.ID, "", nil)
		if code != http.StatusNoContent {
			t.Fatalf("revoke status = %d, want 204", code)
		}
		harness.clock.Add(2 * time.Hour)

		var report struct {
			Deleted int `json:"deleted"`
		}
		code = harness.managementJSON(t, http.MethodPost, "/api/v1/clients/keys/deletions", `{}`, &report)
		if code != http.StatusOK {
			t.Fatalf("status = %d, want 200", code)
		}
		if report.Deleted != 1 {
			t.Fatalf("deleted = %d, want the expired operator key", report.Deleted)
		}
		if _, err := harness.keys.AccessKey(context.Background(), expired.ID); err == nil {
			t.Fatal("the expired key is still listed")
		}
		if key, err := harness.keys.AccessKey(context.Background(), revoked.ID); err != nil || key.Status != access.StatusRevoked {
			t.Fatalf("revoked key = %+v, %v, want it kept", key, err)
		}

		code = harness.managementJSON(t, http.MethodPost, "/api/v1/clients/keys/deletions",
			`{"ids":["`+revoked.ID+`"]}`, nil)
		if code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 for a revoked key", code)
		}
	})
}

// listAccessKeys reads the stored keys through the management API.
func (h *harness) listAccessKeys(t *testing.T) []keySummary {
	t.Helper()
	response := h.management(http.MethodGet, "/api/v1/clients/keys", adminToken, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	var report struct {
		Items []keySummary `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &report); err != nil {
		t.Fatalf("decode the listing: %v", err)
	}
	if len(report.Items) == 0 {
		t.Fatal("the listing is empty")
	}
	return report.Items
}

// createKey stores one key directly, which the API tests use as a fixture.
func (h *harness) createKey(name string, request appaccess.NewAccessKey) appaccess.IssuedAccessKey {
	h.t.Helper()
	issued, err := h.keys.CreateAccessKey(context.Background(), request)
	if err != nil {
		h.t.Fatalf("CreateAccessKey(%s) error = %v", name, err)
	}
	return issued
}
