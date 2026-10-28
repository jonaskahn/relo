// Build-time seed: the models.dev dataset embedded in release artifacts.
package modelsdev

import (
	"bytes"
	"compress/gzip"
	"embed"
	"errors"
	"fmt"
	"io"
	"time"
)

const seedFileName = "seeddata/modelsdev.json.gz"

const maxDatasetBytes = 16 << 20

//go:embed seeddata/*
var seedFiles embed.FS

// Seed is a compressed models.dev dataset and the time it was downloaded.
// A zero Seed means no seed was available at build time.
type Seed struct {
	Gzip      []byte
	FetchedAt time.Time
}

// EmbeddedSeed reads the build-time dataset. A missing, unreadable, or
// invalid gzip is reported as no seed, never as an error that stops startup.
func EmbeddedSeed() (Seed, bool) {
	data, err := seedFiles.ReadFile(seedFileName)
	if err != nil || len(data) == 0 {
		return Seed{}, false
	}
	return ParseSeed(data)
}

// ParseSeed reads the gzip header of a seed. The dataset itself is parsed on
// first read, so startup never pays for a dataset this build does not use.
func ParseSeed(data []byte) (Seed, bool) {
	seed := Seed{Gzip: data}
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return Seed{}, false
	}
	defer func() { _ = reader.Close() }()
	seed.FetchedAt = reader.ModTime
	if seed.FetchedAt.IsZero() {
		return Seed{}, false
	}
	return seed, true
}

// DecompressSeed returns the dataset the seed carries.
func DecompressSeed(seed Seed) ([]byte, error) {
	return decompressSeed(seed)
}

// SeedFromDataset compresses a dataset for NewDirectoryWithSeed. The runtime
// keeps the compressed bytes in memory and decompresses them on first read.
func SeedFromDataset(data []byte, fetchedAt time.Time) (Seed, error) {
	var compressed bytes.Buffer
	writer, err := gzip.NewWriterLevel(&compressed, gzip.BestCompression)
	if err != nil {
		return Seed{}, err
	}
	writer.ModTime = fetchedAt
	if _, err := writer.Write(data); err != nil {
		return Seed{}, err
	}
	if err := writer.Close(); err != nil {
		return Seed{}, err
	}
	return Seed{Gzip: compressed.Bytes(), FetchedAt: fetchedAt}, nil
}

// decompressSeed returns the dataset a seed carries, or an error when the
// bytes are not a gzip stream.
func decompressSeed(seed Seed) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(seed.Gzip))
	if err != nil {
		return nil, err
	}
	defer func() { _ = reader.Close() }()
	data, err := io.ReadAll(io.LimitReader(reader, maxDatasetBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxDatasetBytes {
		return nil, fmt.Errorf("models.dev seed exceeds %d bytes", maxDatasetBytes)
	}
	return data, nil
}

// ValidateDataset parses a models.dev document with the runtime parser and
// reports how many providers and models it describes. The seed generator uses
// it so a dataset it accepts is one the daemon can read.
func ValidateDataset(data []byte) (int, int, error) {
	idx, err := parseDataset(data)
	if err != nil {
		return 0, 0, err
	}
	return len(idx.Providers), idx.TotalModels(), nil
}

func parseDataset(data []byte) (*Index, error) {
	idx, err := parseIndex(data)
	if err != nil {
		return nil, err
	}
	if len(idx.Providers) == 0 {
		return nil, errors.New("dataset describes no providers")
	}
	return idx, nil
}
