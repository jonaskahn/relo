package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const validDataset = `{"openai":{"id":"openai","name":"OpenAI","models":{"gpt-5":{"id":"gpt-5","name":"GPT-5","cost":{"input":1,"output":2}}}}}`

func seedServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	return server
}

func seedOptionsFor(t *testing.T, server *httptest.Server) (seedOptions, string) {
	t.Helper()
	output := filepath.Join(t.TempDir(), "seeddata", "modelsdev.json.gz")
	return seedOptions{
		SourceURL: server.URL, Output: output, Attempts: 3, Timeout: 5 * time.Second,
		Now:   func() time.Time { return time.Unix(1_700_000_000, 0).UTC() },
		sleep: func(context.Context, time.Duration) error { return nil },
	}, output
}

func readGzip(t *testing.T, path string) ([]byte, time.Time) {
	t.Helper()
	compressed, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatalf("gzip seed: %v", err)
	}
	defer func() { _ = reader.Close() }()
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("decompress seed: %v", err)
	}
	return data, reader.ModTime
}

func TestGenerateWritesAValidSeedWithItsDownloadTime(t *testing.T) {
	options, output := seedOptionsFor(t, seedServer(t, http.StatusOK, validDataset))
	result, err := generate(context.Background(), options)
	if err != nil {
		t.Fatalf("generate() error = %v", err)
	}
	if result.Providers != 1 || result.Models != 1 || result.CompressedBytes == 0 {
		t.Fatalf("result = %+v, want one provider and one model", result)
	}
	data, modTime := readGzip(t, output)
	if string(data) != validDataset {
		t.Fatalf("dataset = %q, want the served body", data)
	}
	if !modTime.Equal(time.Unix(1_700_000_000, 0)) {
		t.Fatalf("ModTime = %v, want the recorded download time", modTime)
	}
}

func TestGenerateReusesAValidSeedAndFetchesAMissingOrCorruptOne(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		_, _ = io.WriteString(w, validDataset)
	}))
	t.Cleanup(server.Close)
	options, output := seedOptionsFor(t, server)

	if _, err := generate(context.Background(), options); err != nil {
		t.Fatalf("first generate() error = %v", err)
	}
	if _, err := generate(context.Background(), options); err != nil {
		t.Fatalf("reuse generate() error = %v", err)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want a valid seed reused", requests)
	}

	if err := os.WriteFile(output, []byte("not gzip"), 0644); err != nil {
		t.Fatalf("corrupt seed: %v", err)
	}
	if _, err := generate(context.Background(), options); err != nil {
		t.Fatalf("corrupt generate() error = %v", err)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want a corrupt seed downloaded", requests)
	}
	if err := os.Remove(output); err != nil {
		t.Fatalf("remove seed: %v", err)
	}
	if _, err := generate(context.Background(), options); err != nil {
		t.Fatalf("missing generate() error = %v", err)
	}
	if requests != 3 {
		t.Fatalf("requests = %d, want a missing seed downloaded", requests)
	}
}

func TestGenerateForceReplacesTheSeed(t *testing.T) {
	body := validDataset
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	options, output := seedOptionsFor(t, server)
	if _, err := generate(context.Background(), options); err != nil {
		t.Fatalf("first generate() error = %v", err)
	}
	body = strings.Replace(validDataset, "GPT-5", "GPT-5.1", 1)
	options.Force = true
	if _, err := generate(context.Background(), options); err != nil {
		t.Fatalf("forced generate() error = %v", err)
	}
	data, _ := readGzip(t, output)
	if !strings.Contains(string(data), "GPT-5.1") {
		t.Fatalf("dataset = %q, want the forced download", data)
	}
}

func TestGenerateFailuresLeaveThePreviousSeedUntouched(t *testing.T) {
	options, output := seedOptionsFor(t, seedServer(t, http.StatusOK, validDataset))
	if _, err := generate(context.Background(), options); err != nil {
		t.Fatalf("initial generate() error = %v", err)
	}
	before, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("read the first seed: %v", err)
	}

	cases := map[string]*httptest.Server{
		"invalid JSON":       seedServer(t, http.StatusOK, `{"openai":`),
		"empty body":         seedServer(t, http.StatusOK, ""),
		"empty dataset":      seedServer(t, http.StatusOK, `{}`),
		"no models":          seedServer(t, http.StatusOK, `{"openai":{"id":"openai","name":"OpenAI","models":{}}}`),
		"non-200":            seedServer(t, http.StatusBadGateway, validDataset),
		"oversized response": seedServer(t, http.StatusOK, strings.Repeat("x", maxDatasetBytes+1)),
	}
	for name, server := range cases {
		t.Run(name, func(t *testing.T) {
			failed := options
			failed.SourceURL = server.URL
			failed.Force = true
			if _, err := generate(context.Background(), failed); err == nil {
				t.Fatal("generate() error = nil, want the download refused")
			}
			after, err := os.ReadFile(output)
			if err != nil {
				t.Fatalf("read the seed after failure: %v", err)
			}
			if !bytes.Equal(before, after) {
				t.Fatal("a failed download replaced the previous seed")
			}
		})
	}
}

func TestGenerateTransportFailureLeavesThePreviousSeedUntouched(t *testing.T) {
	options, output := seedOptionsFor(t, seedServer(t, http.StatusOK, validDataset))
	if _, err := generate(context.Background(), options); err != nil {
		t.Fatalf("initial generate() error = %v", err)
	}
	before, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("read the first seed: %v", err)
	}
	failed := options
	failed.Force = true
	failed.Attempts = 1
	failed.Client = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("connection refused")
	})}
	if _, err := generate(context.Background(), failed); err == nil {
		t.Fatal("generate() error = nil, want the transport failure")
	}
	after, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("read the seed after failure: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("a transport failure replaced the previous seed")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }
