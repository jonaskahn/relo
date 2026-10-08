package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	appaccess "github.com/jonaskahn/relo/internal/application/access"
	appintegration "github.com/jonaskahn/relo/internal/application/integration"
	"github.com/jonaskahn/relo/internal/config"
)

// dataPlaneProbeService builds a service whose data plane is one test server, so a
// probe can be asserted on without a daemon listening on a fixed port. Both
// client protocols point at the same server: what tells them apart is the
// path and the credential header a probe sends, which is what the tests read.
func dataPlaneProbeService(t *testing.T, handler http.Handler) (*harness, *appintegration.Service, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	port, err := strconv.Atoi(strings.TrimPrefix(server.URL, "http://127.0.0.1:"))
	if err != nil {
		t.Fatalf("parse the test server address %s: %v", server.URL, err)
	}
	h := newHarness(t)
	settings := &config.Config{}
	settings.Server.Bind = "127.0.0.1"
	settings.Server.DataPlane.OpenAI = port
	settings.Server.DataPlane.Anthropic = port
	return h, integrationUseCases(t, h, settings, t.TempDir()), server
}

// writeIntegrationKey stores the raw client key one agent runs with, which is
// the file every probe reads.
func writeIntegrationKey(t *testing.T, home, id, key string) {
	t.Helper()
	dir := filepath.Join(home, "integrations", id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("create the integration directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "key"), []byte(key+"\n"), 0o600); err != nil {
		t.Fatalf("write the integration key: %v", err)
	}
}

// TestIntegrationModelsSpeakTheClientsProtocol is the difference between a
// model list and the model list a client sees: the probe travels the port the
// agent is pointed at and sends the credential the way that protocol expects
// it, so the identifiers an operator picks from are the ones the agent sends.
func TestIntegrationModelsSpeakTheClientsProtocol(t *testing.T) {
	cases := []struct {
		agent    string
		protocol string
	}{
		{"codex", "openai"},
		{"claude-code", "anthropic"},
	}
	for _, testCase := range cases {
		t.Run(testCase.agent, func(t *testing.T) {
			var gotPath, gotBearer, gotKey, gotVersion string
			h, built, _ := dataPlaneProbeService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				gotBearer = r.Header.Get("Authorization")
				gotKey = r.Header.Get("x-api-key")
				gotVersion = r.Header.Get("anthropic-version")
				w.Header().Set("Content-Type", "application/json")
				if r.Header.Get("anthropic-version") == "" {
					_, _ = w.Write([]byte(`{"models":[{"slug":"relo/google-antigravity/gemini-3.8-flash-high","display_name":"Gemini 3.8 Flash (High)"}]}`))
					return
				}
				_, _ = w.Write([]byte(`{"data":[{"id":"relo/google-antigravity/gemini-3.8-flash-high","display_name":"Gemini 3.8 Flash (High)"}]}`))
			}))
			writeIntegrationKey(t, h.home, testCase.agent, "rlo_ak_secret")

			read, err := built.IntegrationModels(context.Background(), testCase.agent)
			if err != nil {
				t.Fatalf("IntegrationModels() error = %v", err)
			}
			if !read.OK || read.Status != http.StatusOK {
				t.Fatalf("read = %+v, want the data plane's answer", read)
			}
			if gotPath != "/v1/models" {
				t.Fatalf("path = %q, want the model listing", gotPath)
			}
			if testCase.protocol == "anthropic" {
				if gotKey != "rlo_ak_secret" || gotVersion == "" || gotBearer != "" {
					t.Fatalf("anthropic headers = key %q version %q bearer %q, want an API key and the version", gotKey, gotVersion, gotBearer)
				}
			} else if gotBearer != "Bearer rlo_ak_secret" || gotKey != "" {
				t.Fatalf("openai headers = bearer %q key %q, want the bearer token alone", gotBearer, gotKey)
			}
			if len(read.Models) != 1 || read.Models[0].ID != "relo/google-antigravity/gemini-3.8-flash-high" {
				t.Fatalf("models = %+v, want the listed model", read.Models)
			}
			if read.Models[0].Name != "Gemini 3.8 Flash (High)" {
				t.Fatalf("name = %q, want the display name the shape carried", read.Models[0].Name)
			}
		})
	}
}

// TestIntegrationModelsReportARefusal asserts a key the data plane refuses is
// reported rather than raised, because the console shows it beside the account.
func TestIntegrationModelsReportARefusal(t *testing.T) {
	h, built, _ := dataPlaneProbeService(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"api.keys.invalid"}}`))
	}))
	writeIntegrationKey(t, h.home, "codex", "rlo_ak_stale")

	read, err := built.IntegrationModels(context.Background(), "codex")
	if err != nil {
		t.Fatalf("IntegrationModels() error = %v", err)
	}
	if read.OK || read.Status != http.StatusUnauthorized || read.Message != "the data plane refused the key" {
		t.Fatalf("read = %+v, want the refusal named", read)
	}
	if len(read.Models) != 0 {
		t.Fatalf("models = %+v, want none from a refused read", read.Models)
	}
}

// TestVerifyIntegrationRegeneratesAMissingSettingsFile is the check a 200 from
// the data plane does not cover: the agent's own file has to carry the block.
// A file that is gone is written again and reported as fixed; a file that is
// present but different is refused and left for an explicit repair.
func TestVerifyIntegrationRegeneratesAMissingSettingsFile(t *testing.T) {
	_, built, _ := dataPlaneProbeService(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"relo-openai-gpt-4o"}]}`))
	}))
	answer, err := built.EnableIntegration(context.Background(), "pi")
	if err != nil {
		t.Fatalf("EnableIntegration() error = %v", err)
	}
	models := ""
	for _, file := range answer.Integration.Files {
		if strings.HasSuffix(file.Path, "models.json") {
			models = file.Path
		}
	}
	if models == "" {
		t.Fatalf("files = %+v, want the pi catalog", answer.Integration.Files)
	}
	if err := os.Remove(models); err != nil {
		t.Fatalf("remove the catalog: %v", err)
	}

	verify, err := built.VerifyIntegration(context.Background(), "pi")
	if err != nil {
		t.Fatalf("VerifyIntegration() error = %v", err)
	}
	if !verify.OK || !verify.Config || verify.Status != http.StatusOK {
		t.Fatalf("verify = %+v, want the key accepted and the file written again", verify)
	}
	settings := checkNamed(verify.Checks, appintegration.CheckSettings)
	if !settings.OK || !settings.Fixed {
		t.Fatalf("settings check = %+v, want passing and fixed", settings)
	}
	if _, err := os.Stat(models); err != nil {
		t.Fatalf("the catalog was not written again: %v", err)
	}

	if err := os.WriteFile(models, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("drift the catalog: %v", err)
	}
	verify, err = built.VerifyIntegration(context.Background(), "pi")
	if err != nil {
		t.Fatalf("VerifyIntegration() error = %v", err)
	}
	if verify.OK || verify.Status != http.StatusOK {
		t.Fatalf("verify = %+v, want the key accepted and the changed block refused", verify)
	}
	if !strings.Contains(verify.Message, "models.json") {
		t.Fatalf("message = %q, want the catalog path", verify.Message)
	}
	if content, err := os.ReadFile(models); err != nil || string(content) != "{}\n" {
		t.Fatalf("catalog = %q, want the changed file left alone", content)
	}
}

func checkNamed(checks []appintegration.VerifyCheck, name string) appintegration.VerifyCheck {
	for _, check := range checks {
		if check.Name == name {
			return check
		}
	}
	return appintegration.VerifyCheck{}
}

// TestVerifyIntegrationAcceptsTheProviderBlock is the other half of that check:
// a block Relo wrote and a key the data plane accepts both have to pass.
func TestVerifyIntegrationAcceptsTheProviderBlock(t *testing.T) {
	_, built, _ := dataPlaneProbeService(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"relo-openai-gpt-4o"}]}`))
	}))
	if _, err := built.EnableIntegration(context.Background(), "pi"); err != nil {
		t.Fatalf("EnableIntegration() error = %v", err)
	}

	verify, err := built.VerifyIntegration(context.Background(), "pi")
	if err != nil {
		t.Fatalf("VerifyIntegration() error = %v", err)
	}
	if !verify.OK || !verify.Config || verify.Status != http.StatusOK {
		t.Fatalf("verify = %+v, want the block and the key accepted", verify)
	}
}

// TestVerifyIntegrationReadsTheShapeTheClientSees covers the count a verify
// reports: an Anthropic agent's listing is the Anthropic shape, which carries
// no OpenAI object marker at all.
func TestVerifyIntegrationReadsTheShapeTheClientSees(t *testing.T) {
	h, built, _ := dataPlaneProbeService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("anthropic-version") == "" {
			_, _ = w.Write([]byte(`{"object":"list","data":[]}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"type":"model","id":"relo/openai-codex/gpt-6-luna"}],"has_more":false}`))
	}))
	writeIntegrationKey(t, h.home, "claude-code", "rlo_ak_secret")

	verify, err := built.VerifyIntegration(context.Background(), "claude-code")
	if err != nil {
		t.Fatalf("VerifyIntegration() error = %v", err)
	}
	if !verify.OK || verify.Models != 1 {
		t.Fatalf("verify = %+v, want the model the Anthropic shape listed", verify)
	}
}

// TestIntegrationChatSendsATurnThroughTheKey is the proof a listing cannot
// give: the turn travels the surface the client speaks, with the key it runs
// with, and the answer is read back out of that surface's own shape.
func TestIntegrationChatSendsATurnThroughTheKey(t *testing.T) {
	var gotPath, gotBearer, gotKey, gotVersion, gotBody string
	h, built, _ := dataPlaneProbeService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotBearer = r.Header.Get("Authorization")
		gotKey = r.Header.Get("x-api-key")
		gotVersion = r.Header.Get("anthropic-version")
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/messages" {
			_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"bonjour"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"hi there"}}]}`))
	}))
	writeIntegrationKey(t, h.home, "codex", "rlo_ak_secret")
	writeIntegrationKey(t, h.home, "claude-code", "rlo_ak_secret")

	t.Run("openai", func(t *testing.T) {
		answer, err := built.IntegrationChat(context.Background(), "codex", appintegration.IntegrationChatRequest{
			Model: "relo/google-antigravity/gemini-3.8-flash-high", Prompt: "hello",
		})
		if err != nil {
			t.Fatalf("IntegrationChat() error = %v", err)
		}
		if !answer.OK || answer.Text != "hi there" {
			t.Fatalf("answer = %+v, want the text the shape carried", answer)
		}
		if gotPath != "/v1/chat/completions" || gotBearer != "Bearer rlo_ak_secret" {
			t.Fatalf("request = %q bearer %q, want the chat surface and the key", gotPath, gotBearer)
		}
		var body map[string]any
		if err := json.Unmarshal([]byte(gotBody), &body); err != nil {
			t.Fatalf("decode the request body: %v", err)
		}
		if body["model"] != "relo/google-antigravity/gemini-3.8-flash-high" || body["stream"] != false {
			t.Fatalf("body = %v, want the model and one complete answer", body)
		}
		messages, _ := body["messages"].([]any)
		if len(messages) != 1 {
			t.Fatalf("messages = %v, want the prompt alone", body["messages"])
		}
	})

	t.Run("anthropic", func(t *testing.T) {
		answer, err := built.IntegrationChat(context.Background(), "claude-code", appintegration.IntegrationChatRequest{
			Model: "relo/google-antigravity/claude-sonnet-4-6", Prompt: "bonjour",
		})
		if err != nil {
			t.Fatalf("IntegrationChat() error = %v", err)
		}
		if !answer.OK || answer.Text != "bonjour" {
			t.Fatalf("answer = %+v, want the text blocks the shape carried", answer)
		}
		if gotPath != "/v1/messages" || gotKey != "rlo_ak_secret" || gotVersion == "" {
			t.Fatalf("request = %q key %q version %q, want the messages surface and the key", gotPath, gotKey, gotVersion)
		}
	})
}

// TestIntegrationChatNamesARefusal asserts a turn the data plane refused
// reports what it said, which is what an operator acts on.
func TestIntegrationChatNamesARefusal(t *testing.T) {
	h, built, _ := dataPlaneProbeService(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"Individual quota reached"}}`))
	}))
	writeIntegrationKey(t, h.home, "codex", "rlo_ak_secret")

	answer, err := built.IntegrationChat(context.Background(), "codex", appintegration.IntegrationChatRequest{
		Model: "relo/google-antigravity/gemini-3.8-flash-high", Prompt: "hello",
	})
	if err != nil {
		t.Fatalf("IntegrationChat() error = %v", err)
	}
	if answer.OK || answer.Status != http.StatusTooManyRequests || answer.Error != "Individual quota reached" {
		t.Fatalf("answer = %+v, want the refusal named", answer)
	}
}

// TestIntegrationProbeRefusesAnUnwiredAgent covers the two ways a probe has
// nothing to send: an agent nothing minted a key for, and an identifier no
// agent matches.
func TestIntegrationProbeRefusesAnUnwiredAgent(t *testing.T) {
	_, built, _ := dataPlaneProbeService(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	ctx := context.Background()

	if _, err := built.IntegrationModels(ctx, "codex"); !errors.Is(err, appaccess.ErrNotFound) {
		t.Fatalf("IntegrationModels() error = %v, want the missing key sentinel", err)
	}
	if _, err := built.IntegrationChat(ctx, "nothing", appintegration.IntegrationChatRequest{Model: "m", Prompt: "p"}); !errors.Is(err, appintegration.ErrUnknownIntegration) {
		t.Fatalf("IntegrationChat() error = %v, want the unknown integration sentinel", err)
	}
	if _, err := built.IntegrationChat(ctx, "codex", appintegration.IntegrationChatRequest{Prompt: "p"}); !errors.Is(err, appintegration.ErrInvalidQuery) {
		t.Fatalf("IntegrationChat() error = %v, want a refused empty model", err)
	}
}
