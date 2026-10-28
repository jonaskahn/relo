// Download and write the seed dataset with bounded retries.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

const maxDatasetBytes = 16 << 20

var errNoDataset = errors.New("models.dev returned an empty dataset")

type seedOptions struct {
	SourceURL string
	Output    string
	Force     bool
	Client    *http.Client
	Timeout   time.Duration
	Attempts  int
	Now       func() time.Time
	// sleep is injectable so tests do not wait out the retry delay.
	sleep func(context.Context, time.Duration) error
}

type seedResult struct {
	SourceURL       string
	FetchedAt       time.Time
	Providers       int
	Models          int
	CompressedBytes int
	Output          string
	Reused          bool
}

func downloadDataset(ctx context.Context, opts seedOptions) ([]byte, error) {
	client := opts.Client
	if client == nil {
		client = &http.Client{Timeout: opts.Timeout}
	}
	sleep := opts.sleep
	if sleep == nil {
		sleep = sleepContext
	}
	var lastErr error
	for attempt := 0; attempt < opts.Attempts; attempt++ {
		body, err := downloadOnce(ctx, client, opts.SourceURL)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if attempt+1 >= opts.Attempts {
			break
		}
		if err := sleep(ctx, time.Duration(attempt+1)*time.Second); err != nil {
			return nil, err
		}
	}
	return nil, fmt.Errorf("download %s: %w", opts.SourceURL, lastErr)
}

func downloadOnce(ctx context.Context, client *http.Client, sourceURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "relo-modelsdevseed")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxDatasetBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxDatasetBytes {
		return nil, fmt.Errorf("dataset exceeds %d bytes", maxDatasetBytes)
	}
	if len(data) == 0 {
		return nil, errNoDataset
	}
	return data, nil
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
