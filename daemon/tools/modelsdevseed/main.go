// Command modelsdevseed downloads models.dev and writes the gzip dataset the
// daemon embeds in release builds.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jonaskahn/relo/internal/adapters/modelsdev"
)

const (
	defaultSource = "https://models.dev/api.json"
	defaultOutput = "internal/adapters/modelsdev/seeddata/modelsdev.json.gz"
)

func main() {
	force := flag.Bool("force", false, "download a fresh seed even when the existing one is valid")
	flag.Parse()

	result, err := generate(context.Background(), seedOptions{
		SourceURL: defaultSource, Output: defaultOutput, Force: *force,
		Client: nil, Timeout: 30 * time.Second, Attempts: 3, Now: time.Now,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "modelsdevseed:", err)
		os.Exit(1)
	}
	fmt.Printf("source: %s\n", result.SourceURL)
	fmt.Printf("downloaded: %s\n", result.FetchedAt.UTC().Format(time.RFC3339))
	fmt.Printf("providers: %d\nmodels: %d\n", result.Providers, result.Models)
	fmt.Printf("compressed: %d bytes\noutput: %s\n", result.CompressedBytes, result.Output)
	if result.Reused {
		fmt.Println("reused: the existing seed is valid; pass -force to download")
	}
}

func generate(ctx context.Context, opts seedOptions) (seedResult, error) {
	if opts.SourceURL == "" {
		opts.SourceURL = defaultSource
	}
	if opts.Output == "" {
		opts.Output = defaultOutput
	}
	if opts.Attempts <= 0 {
		opts.Attempts = 3
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 30 * time.Second
	}
	if !opts.Force {
		if existing, ok := readExistingSeed(opts.Output, opts.SourceURL); ok {
			return existing, nil
		}
	}

	data, err := downloadDataset(ctx, opts)
	if err != nil {
		return seedResult{}, err
	}
	providers, models, err := modelsdev.ValidateDataset(data)
	if err != nil {
		return seedResult{}, fmt.Errorf("validate %s: %w", opts.SourceURL, err)
	}
	if providers == 0 || models == 0 {
		return seedResult{}, fmt.Errorf("validate %s: the dataset describes no models", opts.SourceURL)
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	fetchedAt := now().UTC()
	seed, err := modelsdev.SeedFromDataset(data, fetchedAt)
	if err != nil {
		return seedResult{}, fmt.Errorf("compress the dataset: %w", err)
	}
	if err := writeSeed(opts.Output, seed.Gzip); err != nil {
		return seedResult{}, err
	}
	return seedResult{
		SourceURL: opts.SourceURL, FetchedAt: fetchedAt, Providers: providers,
		Models: models, CompressedBytes: len(seed.Gzip), Output: opts.Output,
	}, nil
}

func readExistingSeed(path, sourceURL string) (seedResult, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return seedResult{}, false
	}
	seed, ok := modelsdev.ParseSeed(data)
	if !ok {
		return seedResult{}, false
	}
	dataset, err := modelsdev.DecompressSeed(seed)
	if err != nil {
		return seedResult{}, false
	}
	providers, models, err := modelsdev.ValidateDataset(dataset)
	if err != nil {
		return seedResult{}, false
	}
	return seedResult{
		SourceURL: sourceURL, FetchedAt: seed.FetchedAt, Providers: providers,
		Models: models, CompressedBytes: len(seed.Gzip), Output: path, Reused: true,
	}, true
}

func writeSeed(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".modelsdev-*.tmp")
	if err != nil {
		return fmt.Errorf("create a temporary seed: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write the temporary seed: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close the temporary seed: %w", err)
	}
	if err := os.Chmod(tmpName, 0644); err != nil {
		return fmt.Errorf("make the seed readable: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}
