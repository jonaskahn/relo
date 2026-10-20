package updates

import "testing"

const sampleCatalog = `{
  "latest": "v0.2.0",
  "latest_beta": "v0.2.0-beta.1.abcdef12",
  "releases": [
    {
      "tag": "v0.2.0",
      "version": "0.2.0",
      "page": "https://github.com/jonaskahn/relo/releases/tag/v0.2.0",
      "notes": "## What's Changed",
      "files": [
        {"name": "Relo-0.2.0-universal.zip", "url": "https://example.com/universal.zip", "os": "macos", "arch": "universal", "kind": "app", "format": "zip"},
        {"name": "Relo-0.2.0-arm64.zip", "url": "https://example.com/arm64.zip", "os": "macos", "arch": "arm64", "kind": "app", "format": "zip"},
        {"name": "Relo-Setup-0.2.0-amd64.deb", "url": "https://example.com/amd64.deb", "os": "linux", "arch": "amd64", "kind": "installer", "format": "deb"},
        {"name": "Relo-Setup-0.2.0-arm64.deb", "url": "https://example.com/arm64.deb", "os": "linux", "arch": "arm64", "kind": "installer", "format": "deb"},
        {"name": "Relo-Setup-0.2.0-arm64.rpm", "url": "https://example.com/arm64.rpm", "os": "linux", "arch": "arm64", "kind": "installer", "format": "rpm"},
        {"name": "Relo-Setup-0.2.0-amd64.exe", "url": "https://example.com/amd64.exe", "os": "windows", "arch": "amd64", "kind": "installer", "format": "exe"},
        {"name": "relo-0.2.0-darwin-arm64.tar.gz", "url": "https://example.com/archive.tar.gz", "os": "macos", "arch": "arm64", "kind": "archive", "format": "tar.gz"},
        {"name": "checksums.txt", "url": "https://example.com/checksums.txt", "os": "any", "arch": "any", "kind": "checksums", "format": "txt"}
      ]
    }
  ]
}`

func TestParseManifest(t *testing.T) {
	catalog, err := parseManifest([]byte(sampleCatalog))
	if err != nil {
		t.Fatalf("parseManifest() error = %v", err)
	}
	if catalog.Latest != "v0.2.0" || catalog.LatestBeta != "v0.2.0-beta.1.abcdef12" {
		t.Fatalf("catalog = %+v, want both markers", catalog)
	}
	if len(catalog.Releases) != 1 || len(catalog.Releases[0].Files) != 8 {
		t.Fatalf("catalog = %+v, want one release with every file", catalog)
	}
}

func TestParseManifestRejectsGarbage(t *testing.T) {
	if _, err := parseManifest([]byte("not json")); err == nil {
		t.Fatal("parseManifest() error = nil, want a parse failure")
	}
}

func TestManifestFind(t *testing.T) {
	catalog, err := parseManifest([]byte(sampleCatalog))
	if err != nil {
		t.Fatalf("parseManifest() error = %v", err)
	}
	tests := []struct {
		name    string
		version string
		want    bool
	}{
		{name: "a bare version", version: "0.2.0", want: true},
		{name: "a v prefixed version", version: "v0.2.0", want: true},
		{name: "a version no release carries", version: "0.3.0", want: false},
		{name: "no version at all", version: "", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, found := catalog.find(test.version); found != test.want {
				t.Fatalf("find(%q) found = %v, want %v", test.version, found, test.want)
			}
		})
	}
}

func TestDownloadsFor(t *testing.T) {
	catalog, err := parseManifest([]byte(sampleCatalog))
	if err != nil {
		t.Fatalf("parseManifest() error = %v", err)
	}
	files := catalog.Releases[0].Files
	tests := []struct {
		name   string
		goos   string
		goarch string
		want   []string
	}{
		{name: "Apple silicon takes the arm64 app", goos: "darwin", goarch: "arm64", want: []string{"Relo-0.2.0-arm64.zip"}},
		{name: "an Intel Mac takes the universal app", goos: "darwin", goarch: "amd64", want: []string{"Relo-0.2.0-universal.zip"}},
		{name: "arm64 Linux takes both packages", goos: "linux", goarch: "arm64", want: []string{"Relo-Setup-0.2.0-arm64.deb", "Relo-Setup-0.2.0-arm64.rpm"}},
		{name: "amd64 Linux takes its own package", goos: "linux", goarch: "amd64", want: []string{"Relo-Setup-0.2.0-amd64.deb"}},
		{name: "Windows takes the installer", goos: "windows", goarch: "amd64", want: []string{"Relo-Setup-0.2.0-amd64.exe"}},
		{name: "a platform the catalog never names", goos: "plan9", goarch: "amd64"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			downloads := downloadsFor(files, test.goos, test.goarch)
			if len(downloads) != len(test.want) {
				t.Fatalf("downloadsFor(%s/%s) = %v, want %v", test.goos, test.goarch, downloads, test.want)
			}
			for index, download := range downloads {
				if download.Name != test.want[index] {
					t.Fatalf("downloadsFor(%s/%s)[%d] = %q, want %q", test.goos, test.goarch, index, download.Name, test.want[index])
				}
			}
		})
	}
}
