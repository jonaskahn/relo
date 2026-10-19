package server

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	apptemplates "github.com/jonaskahn/relo/internal/application/templatesettings"
	"github.com/jonaskahn/relo/internal/routing"
)

func TestOpenCodeFreeToolCutoffDefaultsOnWithoutSettings(t *testing.T) {
	if !(&Server{}).openCodeFreeToolCutoff(attemptSpec{}) {
		t.Fatal("tool cutoff = false, want the safe default")
	}
}

func TestOpenCodeFreeToolCutoffReadsTheTemplateSetting(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		service := apptemplates.New(apptemplates.Options{
			Store: cutoffStore{enabled: enabled}, Connections: cutoffConnections{},
		})
		server := &Server{opts: Options{TemplateSettings: service}}
		request := httptest.NewRequest("POST", "/", nil)
		got := server.openCodeFreeToolCutoff(attemptSpec{
			request: request, candidate: routing.Candidate{ProviderID: "opencode-free"},
		})
		if got != enabled {
			t.Fatalf("tool cutoff = %t, want the stored %t", got, enabled)
		}
	}
}

func TestOpenCodeFreeToolCutoffFailsSafe(t *testing.T) {
	service := apptemplates.New(apptemplates.Options{
		Store: cutoffStore{err: errors.New("unreadable")}, Connections: cutoffConnections{},
	})
	server := &Server{opts: Options{TemplateSettings: service}}
	request := httptest.NewRequest("POST", "/", nil)
	if !server.openCodeFreeToolCutoff(attemptSpec{
		request: request, candidate: routing.Candidate{ProviderID: "opencode-free"},
	}) {
		t.Fatal("tool cutoff = false, want the safe default when the setting cannot be read")
	}
}

type cutoffStore struct {
	enabled bool
	err     error
}

func (s cutoffStore) Get(context.Context, string) (bool, bool, bool, bool, error) {
	if s.err != nil {
		return false, false, false, false, s.err
	}
	return true, true, s.enabled, true, nil
}

func (cutoffStore) Save(context.Context, string, *bool, *bool, int64) error { return nil }

func (cutoffStore) SaveToolCutoff(context.Context, string, bool, int64) error { return nil }

func (cutoffStore) Prefixes(context.Context, string) ([]string, bool, error) { return nil, true, nil }

func (cutoffStore) SavePrefixes(context.Context, string, []string, int64) error { return nil }

type cutoffConnections struct{}

func (cutoffConnections) TemplateID(string) (string, bool) { return "opencode-free", true }
