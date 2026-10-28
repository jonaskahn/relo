package modelsdev

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEmbeddedSeedIsOptional(t *testing.T) {
	seed, ok := EmbeddedSeed()
	if ok && seed.FetchedAt.IsZero() {
		t.Fatalf("EmbeddedSeed() = %+v, true; want a download time", seed)
	}
}

func TestASeedFileAnsweredWithDataIsParsed(t *testing.T) {
	seed := makeSeed(t, sampleJSON, seedDownloadedAt)
	got, ok := seedFromEmbeddedFile(seed.Gzip, nil)
	if !ok || !got.FetchedAt.Equal(seedDownloadedAt) {
		t.Fatalf("seedFromEmbeddedFile() = %+v, %v; want the dataset", got, ok)
	}
	if _, ok := seedFromEmbeddedFile(nil, errors.New("missing")); ok {
		t.Fatal("seedFromEmbeddedFile() accepted a read failure")
	}
}

func TestParseSeedReadsTheDownloadTime(t *testing.T) {
	seed := makeSeed(t, sampleJSON, seedDownloadedAt)
	parsed, ok := ParseSeed(seed.Gzip)
	if !ok || !parsed.FetchedAt.Equal(seedDownloadedAt) {
		t.Fatalf("ParseSeed() = %+v, %v; want the recorded download time", parsed, ok)
	}

	var unstamped bytes.Buffer
	writer := gzip.NewWriter(&unstamped)
	if _, err := writer.Write([]byte(sampleJSON)); err != nil {
		t.Fatalf("write gzip: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	if _, ok := ParseSeed(unstamped.Bytes()); ok {
		t.Fatal("ParseSeed() accepted a seed with no download time")
	}
}

func TestDecompressSeedRoundTripsAndRejectsBrokenStreams(t *testing.T) {
	seed := makeSeed(t, sampleJSON, seedDownloadedAt)
	data, err := DecompressSeed(seed)
	if err != nil || string(data) != sampleJSON {
		t.Fatalf("DecompressSeed() = %q, %v; want the dataset", data, err)
	}

	truncated := Seed{Gzip: seed.Gzip[:len(seed.Gzip)-8]}
	if _, err := DecompressSeed(truncated); err == nil {
		t.Fatal("DecompressSeed() accepted a truncated stream")
	}
}

func TestDecompressSeedRejectsAnOversizedDataset(t *testing.T) {
	seed, err := SeedFromDataset(bytes.Repeat([]byte("x"), maxDatasetBytes+1), seedDownloadedAt)
	if err != nil {
		t.Fatalf("SeedFromDataset() error = %v", err)
	}
	if _, err := DecompressSeed(seed); err == nil {
		t.Fatal("DecompressSeed() accepted a dataset above the limit")
	}
}

func TestValidateDatasetCountsProvidersAndModels(t *testing.T) {
	providers, models, err := ValidateDataset([]byte(sampleJSON))
	if err != nil || providers != 2 || models != 2 {
		t.Fatalf("ValidateDataset() = %d, %d, %v; want two of each", providers, models, err)
	}
	if _, _, err := ValidateDataset([]byte(`{}`)); err == nil {
		t.Fatal("ValidateDataset() accepted a dataset without providers")
	}
}

func TestDirectoryDefaultsAndForceRefresh(t *testing.T) {
	dir := NewDirectory("", "", nil)
	if dir.sourceURL != DefaultURL || dir.httpClient == nil {
		t.Fatalf("directory = %+v, want the official source and a default client", dir)
	}
	dir.now = nil
	if dir.nowTime().IsZero() {
		t.Fatal("nowTime() = zero, want the wall clock fallback")
	}
}

func TestForceRefreshSkipsTheMemoryCopy(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		_, _ = w.Write([]byte(sampleJSON))
	}))
	t.Cleanup(server.Close)
	dir := NewDirectory(t.TempDir(), server.URL, server.Client())
	if _, err := dir.Get(t.Context(), ModeOnlineFirst); err != nil {
		t.Fatalf("first Get() error = %v", err)
	}
	if _, err := dir.Get(t.Context(), ModeForce); err != nil {
		t.Fatalf("forced Get() error = %v", err)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want the forced read to ask again", requests)
	}
}

func TestDatasetFetchFailures(t *testing.T) {
	t.Run("unusable source URL", func(t *testing.T) {
		dir := NewDirectory(t.TempDir(), "://bad", nil)
		if _, err := dir.Get(t.Context(), ModeOnlineFirst); err == nil {
			t.Fatal("Get() error = nil, want the request refused")
		}
	})

	t.Run("not modified without a local copy", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotModified)
		}))
		t.Cleanup(server.Close)
		dir := NewDirectory(t.TempDir(), server.URL, server.Client())
		idx, err := dir.Get(t.Context(), ModeOnlineFirst)
		if err == nil || idx == nil || idx.State.Available {
			t.Fatalf("Get() = %+v, %v; want the empty answer reported", idx, err)
		}
	})

	t.Run("body read failure", func(t *testing.T) {
		client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(failingReader{}), Header: http.Header{}}, nil
		})}
		dir := NewDirectory(t.TempDir(), DefaultURL, client)
		if _, err := dir.Get(t.Context(), ModeOnlineFirst); err == nil {
			t.Fatal("Get() error = nil, want the body failure")
		}
	})

	t.Run("oversized body", func(t *testing.T) {
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(io.LimitReader(zeroReader{}, maxDatasetBytes+1)),
			Header:     http.Header{},
		}
		if _, err := receiveDataset(resp); err == nil {
			t.Fatal("receiveDataset() accepted an oversized body")
		}
	})
}

func TestBackoffReportsTheRecordedFailure(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(server.Close)
	dir := NewDirectory(t.TempDir(), server.URL, server.Client())
	now := seedDownloadedAt
	dir.now = func() time.Time { return now }
	if _, err := dir.Get(t.Context(), ModeOnlineFirst); err == nil {
		t.Fatal("Get() error = nil, want the failed fetch reported")
	}
	now = now.Add(time.Minute)
	idx, err := dir.Get(t.Context(), ModeOnlineFirst)
	if err == nil || idx == nil || idx.State.Available {
		t.Fatalf("backed-off Get() = %+v, %v; want the recorded failure", idx, err)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want the backoff to skip the network", requests)
	}
}

func TestAnUnsavedLiveCopyAnswersAfterALaterFailure(t *testing.T) {
	failing := false
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		if failing {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(sampleJSON))
	}))
	t.Cleanup(server.Close)

	cacheDir := t.TempDir()
	blocker := filepath.Join(cacheDir, "not-a-directory")
	if err := os.WriteFile(blocker, []byte("x"), 0644); err != nil {
		t.Fatalf("write cache blocker: %v", err)
	}
	dir := NewDirectory(cacheDir, server.URL, server.Client())
	dir.cacheFile = filepath.Join(blocker, "modelsdev.json")
	now := seedDownloadedAt
	dir.now = func() time.Time { return now }
	if _, err := dir.Get(t.Context(), ModeOnlineFirst); err != nil {
		t.Fatalf("first Get() error = %v", err)
	}

	failing = true
	now = now.Add(2 * time.Minute)
	idx, err := dir.Get(t.Context(), ModeOnlineFirst)
	if err == nil || idx == nil || !idx.State.Available || !idx.State.Stale {
		t.Fatalf("second Get() = %+v, %v; want the live copy kept as stale", idx, err)
	}
}

func TestCurrentStateReportsACorruptCache(t *testing.T) {
	cacheDir := t.TempDir()
	writeCache(t, cacheDir, `{"openai":`, seedDownloadedAt)
	dir := NewDirectory(cacheDir, DefaultURL, failingClient(t))
	state := dir.CurrentState()
	if state.Available || state.LastError == "" {
		t.Fatalf("state = %+v, want the corrupt cache reported", state)
	}
}

func TestLoadCacheReusesTheParsedCopy(t *testing.T) {
	cacheDir := t.TempDir()
	writeCache(t, cacheDir, sampleJSON, seedDownloadedAt)
	dir := NewDirectory(cacheDir, DefaultURL, failingClient(t))
	first, err := dir.loadCacheLocked()
	if err != nil || first == nil {
		t.Fatalf("first load = %+v, %v", first, err)
	}
	second, err := dir.loadCacheLocked()
	if err != nil || second == nil || second.idx != first.idx {
		t.Fatalf("second load = %+v, %v; want the parsed copy", second, err)
	}
}

func TestASeedThatIsNotADatasetIsIgnored(t *testing.T) {
	seed := makeSeed(t, `{}`, seedDownloadedAt)
	dir := NewDirectoryWithSeed(t.TempDir(), DefaultURL, failingClient(t), seed)
	if idx, err := dir.Cached(); idx != nil || err != nil {
		t.Fatalf("Cached() = %+v, %v; want the seed refused", idx, err)
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = ' '
	}
	return len(p), nil
}
