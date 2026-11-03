package integration

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/codingclients"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/tests/testkit"
)

// desktopFiles backs the Desktop port with the real adapter over a temporary
// library.
type desktopFiles struct{ testFiles }

func (desktopFiles) Digest(content string) string {
	return codingclients.Digest(content)
}

func (desktopFiles) DesktopProfilePath(library string) string {
	return codingclients.DesktopProfilePath(library)
}

func (desktopFiles) DesktopProfileLists(content, baseURL string, models []ModelRef) bool {
	return codingclients.DesktopProfileLists(content, baseURL, clientRefs(models))
}

func (desktopFiles) UpdateDesktopModels(library, baseURL string, models []ModelRef) ([]byte, error) {
	return codingclients.UpdateDesktopModels(library, baseURL, clientRefs(models))
}

// desktopStore keeps the Desktop record and its files in memory.
type desktopStore struct {
	claudeEnabled
	record Record
	files  []FileRecord
}

func (s *desktopStore) Integration(_ context.Context, id string) (Record, bool, error) {
	if id != "claude-desktop" {
		return Record{}, false, nil
	}
	return s.record, true, nil
}

func (s *desktopStore) SaveIntegration(_ context.Context, record Record) error {
	s.record = record
	return nil
}

func (s *desktopStore) Files(context.Context, string) ([]FileRecord, error) {
	return s.files, nil
}

func (s *desktopStore) SaveFile(_ context.Context, record FileRecord) error {
	for index, file := range s.files {
		if file.Kind == record.Kind {
			s.files[index] = record
			return nil
		}
	}
	s.files = append(s.files, record)
	return nil
}

// TestRefreshClaudeDesktopAddsBehavesAsWithAnUnchangedRoster is the catalog
// path: the roster digest still matches, yet the profile lacks the metadata,
// so the service rewrites the models without touching the credential.
func TestRefreshClaudeDesktopAddsBehavesAsWithAnUnchangedRoster(t *testing.T) {
	db := testkit.OpenTestDB(t)
	upstream := testkit.MockUpstream(t, map[string]string{})
	testkit.CustomProvider(t, db, testkit.Upstream{
		ID: "openai", Label: "OpenAI", APIFormat: catalog.FormatOpenAIChat,
		BaseURL: upstream.URL, Auth: catalog.AuthAPIKey,
		Models: []string{"gpt-5.5-pro"}, ModelsFormat: catalog.ModelsNone, ContextWindow: 128_000,
	})
	service := NewService(ServiceOptions{Catalog: testkit.ReloadCatalog(t, db, activeAccounts{})})

	library := t.TempDir()
	models := service.claudePickerModels()
	path := codingclients.DesktopProfilePath(library)
	old := `{"inferenceGatewayApiKey":"rlo_ak_test","inferenceGatewayBaseUrl":"http://127.0.0.1:10202",` +
		`"modelDiscoveryEnabled":false,"theme":"dark","inferenceModels":[{"name":"claude-opus-4-8-r000000000001",` +
		`"labelOverride":"GPT Pro on OpenAI","anthropicFamilyTier":"opus","isFamilyDefault":true}]}`
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create the library: %v", err)
	}
	if err := os.WriteFile(path, []byte(old), 0o600); err != nil {
		t.Fatalf("write the old profile: %v", err)
	}
	store := &desktopStore{
		record: Record{ID: "claude-desktop", Enabled: true, ModelsDigest: codingclients.ModelsDigest(clientRefs(models))},
		files:  []FileRecord{{IntegrationID: "claude-desktop", Kind: KindClaudeDesktop, Path: path, Digest: codingclients.Digest(old)}},
	}
	service.store = store
	service.files = desktopFiles{}
	service.paths = testPaths{home: t.TempDir(), anthropicBase: "http://127.0.0.1:10202", desktopLib: library}

	service.RefreshClaudeDesktop(context.Background())

	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the profile: %v", err)
	}
	for _, want := range []string{"rlo_ak_test", "\"theme\": \"dark\"", "\"behavesAs\": \"claude-opus-4-6\""} {
		if !strings.Contains(string(written), want) {
			t.Fatalf("profile = %s, want %s kept", written, want)
		}
	}
	if store.files[0].Digest != codingclients.Digest(string(written)) {
		t.Fatal("the recorded digest does not describe the written profile")
	}

	// A second refresh has nothing to do and leaves the file alone.
	before, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat the profile: %v", err)
	}
	service.RefreshClaudeDesktop(context.Background())
	after, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat the profile again: %v", err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("a settled profile was rewritten")
	}
}

// TestRefreshClaudeDesktopLeavesAnEditedProfileAlone keeps an operator edit
// for Repair: the catalog path never overwrites a profile Relo no longer owns.
func TestRefreshClaudeDesktopLeavesAnEditedProfileAlone(t *testing.T) {
	service := NewService(ServiceOptions{Catalog: emptyCatalog(t)})
	library := t.TempDir()
	path := codingclients.DesktopProfilePath(library)
	edited := `{"inferenceGatewayBaseUrl":"http://127.0.0.1:10202","modelDiscoveryEnabled":false,"inferenceModels":[]}`
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create the library: %v", err)
	}
	if err := os.WriteFile(path, []byte(edited), 0o600); err != nil {
		t.Fatalf("write the edited profile: %v", err)
	}
	service.store = &desktopStore{
		record: Record{ID: "claude-desktop", Enabled: true},
		files:  []FileRecord{{IntegrationID: "claude-desktop", Kind: KindClaudeDesktop, Path: path, Digest: codingclients.Digest("something else")}},
	}
	service.files = desktopFiles{}
	service.paths = testPaths{home: t.TempDir(), anthropicBase: "http://127.0.0.1:10202", desktopLib: library}

	service.RefreshClaudeDesktop(context.Background())

	written, err := os.ReadFile(path)
	if err != nil || string(written) != edited {
		t.Fatalf("profile = %s, %v, want the edit kept", written, err)
	}
}

// emptyCatalog is a catalog with no providers, so the published list is empty
// and every rendered profile differs.
func emptyCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	return testkit.ReloadCatalog(t, testkit.OpenTestDB(t), activeAccounts{})
}
