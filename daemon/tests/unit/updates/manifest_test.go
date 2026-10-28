package updates_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jonaskahn/relo/internal/updates"
)

const releaseCatalog = `{
  "latest": "v0.2.0",
  "latest_beta": null,
  "releases": [
    {
      "tag": "v0.2.0",
      "version": "0.2.0",
      "page": "https://github.com/jonaskahn/relo/releases/tag/v0.2.0",
      "notes": "## What's Changed\n* A new thing.\n",
      "files": [
        {"name": "Relo-0.2.0-universal.zip", "url": "https://example.com/universal.zip", "os": "macos", "arch": "universal", "kind": "app", "format": "zip"},
        {"name": "Relo-0.2.0-arm64.zip", "url": "https://example.com/arm64.zip", "os": "macos", "arch": "arm64", "kind": "app", "format": "zip"},
        {"name": "Relo-Setup-0.2.0-amd64.deb", "url": "https://example.com/amd64.deb", "os": "linux", "arch": "amd64", "kind": "installer", "format": "deb"},
        {"name": "Relo-Setup-0.2.0-arm64.deb", "url": "https://example.com/arm64.deb", "os": "linux", "arch": "arm64", "kind": "installer", "format": "deb"},
        {"name": "Relo-Setup-0.2.0-amd64.exe", "url": "https://example.com/amd64.exe", "os": "windows", "arch": "amd64", "kind": "installer", "format": "exe"}
      ]
    }
  ]
}`

const olderCatalog = `{
  "latest": "v0.0.9",
  "latest_beta": null,
  "releases": [
    {
      "tag": "v0.0.9",
      "version": "0.0.9",
      "page": "https://example.com/v0.0.9",
      "notes": "old",
      "files": []
    }
  ]
}`

func catalogChecker(catalog []byte, failure error) *updates.Checker {
	return &updates.Checker{
		URL:         "https://example.com/appcast.xml",
		ManifestURL: "https://example.com/releases.json",
		Current:     "0.1.0",
		Fetch: func(_ context.Context, url string) ([]byte, error) {
			if url != "https://example.com/releases.json" {
				return []byte(sampleAppcast), nil
			}
			if failure != nil {
				return nil, failure
			}
			return catalog, nil
		},
	}
}

func TestStatusCarriesTheCatalog(t *testing.T) {
	status := catalogChecker([]byte(releaseCatalog), nil).Status(context.Background())
	if !status.Available || status.Latest != "0.2.0" {
		t.Fatalf("status = %+v, want an update to 0.2.0", status)
	}
	if status.Notes == "" || status.Page != "https://github.com/jonaskahn/relo/releases/tag/v0.2.0" {
		t.Fatalf("status = %+v, want the notes and the release page", status)
	}
	if len(status.Downloads) == 0 {
		t.Fatalf("status = %+v, want a download for this platform", status)
	}
	for _, download := range status.Downloads {
		if download.URL == "" {
			t.Fatalf("download = %+v, want a URL", download)
		}
	}
}

func TestStatusSurvivesAFailedCatalog(t *testing.T) {
	status := catalogChecker(nil, errors.New("offline")).Status(context.Background())
	if !status.Available || status.Latest != "0.2.0" {
		t.Fatalf("status = %+v, want the feed's answer to stand", status)
	}
	if status.Notes != "" || len(status.Downloads) != 0 {
		t.Fatalf("status = %+v, want no catalog details after a failed read", status)
	}
}

func TestStatusIgnoresACatalogWithoutTheRelease(t *testing.T) {
	status := catalogChecker([]byte(olderCatalog), nil).Status(context.Background())
	if !status.Available || status.Latest != "0.2.0" {
		t.Fatalf("status = %+v, want the feed's answer to stand", status)
	}
	if status.Notes != "" || status.Page != "" || len(status.Downloads) != 0 {
		t.Fatalf("status = %+v, want nothing from a release the catalog does not carry", status)
	}
}
