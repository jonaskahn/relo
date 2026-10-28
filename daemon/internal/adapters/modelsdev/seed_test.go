package modelsdev

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/catalog"
)

var seedDownloadedAt = time.Unix(1_700_000_000, 0)

func makeSeed(t *testing.T, dataset string, fetchedAt time.Time) Seed {
	t.Helper()
	seed, err := SeedFromDataset([]byte(dataset), fetchedAt)
	if err != nil {
		t.Fatalf("SeedFromDataset() error = %v", err)
	}
	return seed
}

func writeCache(t *testing.T, dir, dataset string, modTime time.Time) string {
	t.Helper()
	path := filepath.Join(dir, "modelsdev.json")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("create cache dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(dataset), 0644); err != nil {
		t.Fatalf("write cache: %v", err)
	}
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatalf("set cache time: %v", err)
	}
	return path
}

func seededDirectory(cacheDir, sourceURL string, client *http.Client, seed Seed) *Directory {
	dir := NewDirectoryWithSeed(cacheDir, sourceURL, client, seed)
	dir.now = func() time.Time { return seedDownloadedAt.Add(2 * time.Hour) }
	return dir
}

func TestSeedOnlyAttemptsTheNetworkAndServesTheSeedWhenItFails(t *testing.T) {
	dir := seededDirectory(t.TempDir(), DefaultURL, failingClient(t), makeSeed(t, sampleJSON, seedDownloadedAt))
	idx, err := dir.Get(context.Background(), ModeOnlineFirst)
	if err == nil {
		t.Fatal("Get() error = nil, want the failed fetch reported")
	}
	if idx == nil || !idx.State.Available || idx.State.Stale {
		t.Fatalf("state = %+v, want an available seed that is not stale", idx.State)
	}
	if idx.State.Origin != catalog.ModelsDevOriginSeed {
		t.Fatalf("origin = %q, want the seed", idx.State.Origin)
	}
	if idx.State.FetchedAtMs != seedDownloadedAt.UnixMilli() {
		t.Fatalf("fetched = %d, want the build timestamp", idx.State.FetchedAtMs)
	}
	if idx.State.LastError == "" || idx.State.LastAttemptAtMs == 0 {
		t.Fatalf("state = %+v, want the failure recorded beside the seed", idx.State)
	}
}

func TestSeedWithNoNetworkAttemptReportsItsBuildTime(t *testing.T) {
	dir := seededDirectory(t.TempDir(), DefaultURL, failingClient(t), makeSeed(t, sampleJSON, seedDownloadedAt))
	state := dir.CurrentState()
	if !state.Available || state.Stale || state.LastError != "" {
		t.Fatalf("state = %+v, want an available seed before any fetch", state)
	}
	if state.FetchedAtMs != seedDownloadedAt.UnixMilli() || state.LastAttemptAtMs != seedDownloadedAt.UnixMilli() {
		t.Fatalf("state = %+v, want the build timestamp", state)
	}
	if state.Origin != catalog.ModelsDevOriginSeed {
		t.Fatalf("origin = %q, want the seed", state.Origin)
	}
}

func TestEmptySeedFetchesWritesTheCacheAndCoalescesForAMinute(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("ETag", `"v1"`)
		_, _ = w.Write([]byte(sampleJSON))
	}))
	t.Cleanup(server.Close)

	cacheDir := t.TempDir()
	dir := NewDirectoryWithSeed(cacheDir, server.URL, server.Client(), Seed{})
	now := seedDownloadedAt
	dir.now = func() time.Time { return now }

	idx, err := dir.Get(context.Background(), ModeOnlineFirst)
	if err != nil || idx == nil || !idx.State.Available {
		t.Fatalf("Get() = %+v, %v; want a live dataset", idx, err)
	}
	if idx.State.Origin != catalog.ModelsDevOriginNetwork {
		t.Fatalf("origin = %q, want network", idx.State.Origin)
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "modelsdev.json")); err != nil {
		t.Fatalf("cache file: %v", err)
	}

	now = now.Add(59 * time.Second)
	if _, err := dir.Get(context.Background(), ModeOnlineFirst); err != nil {
		t.Fatalf("coalesced Get() error = %v", err)
	}
	if requests.Load() != 1 {
		t.Fatalf("requests = %d, want the copy this process holds to answer", requests.Load())
	}
}

func TestRevalidationIsConditionalAfterAMinute(t *testing.T) {
	var requests atomic.Int64
	var conditional atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("If-None-Match") == `"v1"` {
			conditional.Add(1)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		_, _ = w.Write([]byte(sampleJSON))
	}))
	t.Cleanup(server.Close)

	dir := NewDirectoryWithSeed(t.TempDir(), server.URL, server.Client(), Seed{})
	now := seedDownloadedAt
	dir.now = func() time.Time { return now }
	if _, err := dir.Get(context.Background(), ModeOnlineFirst); err != nil {
		t.Fatalf("first Get() error = %v", err)
	}
	now = now.Add(61 * time.Second)
	idx, err := dir.Get(context.Background(), ModeOnlineFirst)
	if err != nil {
		t.Fatalf("revalidated Get() error = %v", err)
	}
	if requests.Load() != 2 || conditional.Load() != 1 {
		t.Fatalf("requests = %d conditional = %d, want one conditional revalidation", requests.Load(), conditional.Load())
	}
	if idx.State.LastError != "" || idx.State.FetchedAtMs != seedDownloadedAt.UnixMilli() {
		t.Fatalf("state = %+v, want the revalidated copy kept", idx.State)
	}
}

func TestFailedReadsBackOffForFiveMinutes(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(server.Close)

	dir := seededDirectory(t.TempDir(), DefaultURL, serverClient(t, server), makeSeed(t, sampleJSON, seedDownloadedAt))
	if _, err := dir.Get(context.Background(), ModeOnlineFirst); err == nil {
		t.Fatal("Get() error = nil, want the failed fetch reported")
	}
	requestsAfterFailure := requests.Load()
	dir.now = func() time.Time { return seedDownloadedAt.Add(2*time.Hour + 4*time.Minute) }
	idx, err := dir.Get(context.Background(), ModeOnlineFirst)
	if err != nil || idx == nil || !idx.State.Available {
		t.Fatalf("backed-off Get() = %+v, %v; want the local copy", idx, err)
	}
	if requests.Load() != requestsAfterFailure {
		t.Fatalf("requests = %d, want no request inside the backoff", requests.Load())
	}
	dir.now = func() time.Time { return seedDownloadedAt.Add(2*time.Hour + 6*time.Minute) }
	if _, err := dir.Get(context.Background(), ModeOnlineFirst); err == nil {
		t.Fatal("Get() after the backoff error = nil, want another attempt")
	}
	if requests.Load() != requestsAfterFailure+1 {
		t.Fatalf("requests = %d, want one attempt after the backoff", requests.Load())
	}
}

func TestEmptySeedWithAFailedSourceAndAValidCacheServesTheCache(t *testing.T) {
	cacheDir := t.TempDir()
	writeCache(t, cacheDir, sampleJSON, seedDownloadedAt)
	dir := seededDirectory(cacheDir, DefaultURL, failingClient(t), Seed{})
	idx, err := dir.Get(context.Background(), ModeOnlineFirst)
	if err == nil {
		t.Fatal("Get() error = nil, want the failed fetch reported")
	}
	if idx == nil || !idx.State.Available || !idx.State.Stale {
		t.Fatalf("state = %+v, want an available stale cache", idx.State)
	}
	if idx.State.Origin != catalog.ModelsDevOriginCache || idx.State.LastError == "" {
		t.Fatalf("state = %+v, want the cache marked stale with the error", idx.State)
	}
	if idx.State.LastAttemptAtMs != seedDownloadedAt.Add(2*time.Hour).UnixMilli() {
		t.Fatalf("last attempt = %d, want the failed attempt time", idx.State.LastAttemptAtMs)
	}
}

func TestEmptySeedWithNoLocalCopyReportsUnavailableWithoutPanicking(t *testing.T) {
	dir := seededDirectory(t.TempDir(), DefaultURL, failingClient(t), Seed{})
	idx, err := dir.Get(context.Background(), ModeOnlineFirst)
	if err == nil {
		t.Fatal("Get() error = nil, want the failed fetch reported")
	}
	if idx == nil || idx.State.Available || idx.State.Stale {
		t.Fatalf("state = %+v, want an unavailable copy", idx.State)
	}
	if idx.State.LastError == "" {
		t.Fatal("LastError is empty, want the reason")
	}
	if cached, err := dir.Cached(); cached != nil || err != nil {
		t.Fatalf("Cached() = %+v, %v; want no local copy", cached, err)
	}
}

func TestCacheNewerThanSeedWins(t *testing.T) {
	cacheDir := t.TempDir()
	writeCache(t, cacheDir, sampleJSON, seedDownloadedAt.Add(time.Hour))
	dir := seededDirectory(cacheDir, DefaultURL, failingClient(t), makeSeed(t, sampleJSON, seedDownloadedAt))
	idx, err := dir.Get(context.Background(), ModeOnlineFirst)
	if err == nil {
		t.Fatal("Get() error = nil, want the failed fetch reported")
	}
	if idx == nil || idx.State.Origin != catalog.ModelsDevOriginCache || !idx.State.Stale {
		t.Fatalf("state = %+v, want the newer cache", idx.State)
	}
	if idx.State.FetchedAtMs != seedDownloadedAt.Add(time.Hour).UnixMilli() {
		t.Fatalf("fetched = %d, want the cache time", idx.State.FetchedAtMs)
	}
}

func TestCacheAtTheSeedTimestampWins(t *testing.T) {
	cacheDir := t.TempDir()
	writeCache(t, cacheDir, sampleJSON, seedDownloadedAt)
	dir := seededDirectory(cacheDir, DefaultURL, failingClient(t), makeSeed(t, sampleJSON, seedDownloadedAt))
	idx, err := dir.Cached()
	if err != nil || idx == nil || idx.State.Origin != catalog.ModelsDevOriginCache {
		t.Fatalf("Cached() = %+v, %v; want the cache at the seed's own time", idx, err)
	}
}

func TestSeedNewerThanCachedCopyWins(t *testing.T) {
	cacheDir := t.TempDir()
	writeCache(t, cacheDir, sampleJSON, seedDownloadedAt.Add(-time.Hour))
	dir := seededDirectory(cacheDir, DefaultURL, failingClient(t), makeSeed(t, sampleJSON, seedDownloadedAt))
	idx, err := dir.Get(context.Background(), ModeOnlineFirst)
	if err == nil {
		t.Fatal("Get() error = nil, want the failed fetch reported")
	}
	if idx == nil || idx.State.Origin != catalog.ModelsDevOriginSeed || idx.State.Stale {
		t.Fatalf("state = %+v, want the newer seed", idx.State)
	}
	if idx.State.FetchedAtMs != seedDownloadedAt.UnixMilli() {
		t.Fatalf("fetched = %d, want the seed time", idx.State.FetchedAtMs)
	}
}

func TestCorruptCacheFallsBackToTheSeed(t *testing.T) {
	cacheDir := t.TempDir()
	writeCache(t, cacheDir, `{"openai":`, seedDownloadedAt.Add(time.Hour))
	dir := seededDirectory(cacheDir, DefaultURL, failingClient(t), makeSeed(t, sampleJSON, seedDownloadedAt))
	idx, err := dir.Get(context.Background(), ModeOnlineFirst)
	if err == nil {
		t.Fatal("Get() error = nil, want the failed fetch reported")
	}
	if idx == nil || idx.State.Origin != catalog.ModelsDevOriginSeed || !idx.State.Available {
		t.Fatalf("state = %+v, want the seed when the cache is corrupt", idx.State)
	}
}

func TestInvalidSeedIsIgnored(t *testing.T) {
	valid := makeSeed(t, sampleJSON, seedDownloadedAt)
	broken := Seed{Gzip: valid.Gzip, FetchedAt: seedDownloadedAt}
	broken.Gzip = []byte("not gzip")
	dir := seededDirectory(t.TempDir(), DefaultURL, failingClient(t), broken)
	idx, err := dir.Get(context.Background(), ModeOnlineFirst)
	if err == nil {
		t.Fatal("Get() error = nil, want the failed fetch reported")
	}
	if idx == nil || idx.State.Available {
		t.Fatalf("state = %+v, want an unavailable copy when the seed is invalid", idx.State)
	}
}

func TestParseSeedRefusesInvalidGzip(t *testing.T) {
	if _, ok := ParseSeed([]byte("not gzip")); ok {
		t.Fatal("ParseSeed() accepted bytes that are not a gzip stream")
	}
}

func TestCustomSourceNeverUsesTheSeed(t *testing.T) {
	custom := "https://models.example.test/api.json"
	dir := NewDirectoryWithSeed(t.TempDir(), custom, failingClient(t), makeSeed(t, sampleJSON, seedDownloadedAt))
	if _, err := dir.Get(context.Background(), ModeOnlineFirst); err == nil {
		t.Fatal("Get() error = nil, want the custom source failure reported")
	}
	if state := dir.CurrentState(); state.Available {
		t.Fatalf("state = %+v, want no embedded seed for a custom source", state)
	}
}

func TestSuccessfulFetchReplacesTheSeedAndWritesAtomically(t *testing.T) {
	cacheDir := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(sampleJSON))
	}))
	t.Cleanup(server.Close)
	dir := seededDirectory(cacheDir, DefaultURL, serverClient(t, server), makeSeed(t, sampleJSON, seedDownloadedAt))
	idx, err := dir.Get(context.Background(), ModeOnlineFirst)
	if err != nil || idx.State.Origin != catalog.ModelsDevOriginNetwork {
		t.Fatalf("Get() = %+v, %v; want the live dataset", idx, err)
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "modelsdev.json.tmp")); !os.IsNotExist(err) {
		t.Fatalf("temporary cache file survived: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(cacheDir, "modelsdev.json")); err != nil || string(data) != sampleJSON {
		t.Fatalf("cache = %q, %v; want the served dataset", data, err)
	}
}

func TestMalformedResponsePreservesTheLastGoodCopy(t *testing.T) {
	cacheDir := t.TempDir()
	path := writeCache(t, cacheDir, sampleJSON, seedDownloadedAt.Add(time.Hour))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"openai":`))
	}))
	t.Cleanup(server.Close)
	dir := seededDirectory(cacheDir, DefaultURL, serverClient(t, server), makeSeed(t, sampleJSON, seedDownloadedAt))
	idx, err := dir.Get(context.Background(), ModeOnlineFirst)
	if err == nil {
		t.Fatal("Get() error = nil, want the malformed response reported")
	}
	if idx == nil || idx.State.Origin != catalog.ModelsDevOriginCache || !idx.State.Stale {
		t.Fatalf("state = %+v, want the previous cache kept", idx.State)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != sampleJSON {
		t.Fatalf("cache = %q, %v; want the previous good copy", data, err)
	}
}

func TestGeneratedSeedParsesWithProvidersModelsAndATimestamp(t *testing.T) {
	seed, ok := EmbeddedSeed()
	if !ok {
		t.Skip("this checkout has no generated seed; `make seed` embeds one")
	}
	idx, err := dirForSeed(t, seed).Cached()
	if err != nil || idx == nil || len(idx.Providers) == 0 || idx.TotalModels() == 0 {
		t.Fatalf("seed index = %+v, %v; want providers and models", idx, err)
	}
	if seed.FetchedAt.IsZero() {
		t.Fatal("seed timestamp is zero")
	}
}

func dirForSeed(t *testing.T, seed Seed) *Directory {
	t.Helper()
	return seededDirectory(t.TempDir(), DefaultURL, failingClient(t), seed)
}

func failingClient(t *testing.T) *http.Client {
	t.Helper()
	return &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errTransport
	})}
}

// serverClient sends the official URL to a local test server, so a seed is
// accepted while the live request still reaches the process under test.
func serverClient(t *testing.T, server *httptest.Server) *http.Client {
	t.Helper()
	return &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		clone := req.Clone(req.Context())
		clone.URL.Scheme = "http"
		clone.URL.Host = server.Listener.Addr().String()
		clone.Host = ""
		return http.DefaultTransport.RoundTrip(clone)
	})}
}

var errTransport = &transportError{}

type transportError struct{}

func (*transportError) Error() string { return "connection refused" }

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }
