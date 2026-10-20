// Manifest reads the release catalog the published site serves, so the console
// can show the changelog and the download that fits the machine reading it.
package updates

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	// DefaultManifestURL is the catalog every build reads for what a feed
	// cannot carry: the release notes and the per-platform download.
	DefaultManifestURL = "https://relo.ifelse.one/releases.json"
	// The catalog lists every release with every asset, so it outgrows the
	// feed cap; the read stays bounded all the same.
	maxManifestBytes = 4 << 20
	kindApp          = "app"
	kindInstaller    = "installer"
)

// Download is one file a release carries for the machine asking.
type Download struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	Size   int64  `json:"size,omitempty"`
	Arch   string `json:"arch,omitempty"`
	Format string `json:"format,omitempty"`
}

type manifest struct {
	Latest     string            `json:"latest"`
	LatestBeta string            `json:"latest_beta"`
	Releases   []manifestRelease `json:"releases"`
}

type manifestRelease struct {
	Tag     string         `json:"tag"`
	Version string         `json:"version"`
	Page    string         `json:"page"`
	Notes   string         `json:"notes"`
	Files   []manifestFile `json:"files"`
}

type manifestFile struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	Size   int64  `json:"size"`
	OS     string `json:"os"`
	Arch   string `json:"arch"`
	Kind   string `json:"kind"`
	Format string `json:"format"`
}

func parseManifest(body []byte) (manifest, error) {
	var catalog manifest
	if err := json.Unmarshal(body, &catalog); err != nil {
		return manifest{}, fmt.Errorf("parse the release catalog: %w", err)
	}
	return catalog, nil
}

func (m manifest) find(version string) (manifestRelease, bool) {
	wanted := strings.TrimPrefix(strings.TrimSpace(version), "v")
	if wanted == "" {
		return manifestRelease{}, false
	}
	for _, release := range m.Releases {
		if release.Version == wanted || strings.TrimPrefix(release.Tag, "v") == wanted {
			return release, true
		}
	}
	return manifestRelease{}, false
}

func downloadsFor(files []manifestFile, goos, goarch string) []Download {
	platform := platformOf(goos)
	if platform == "" {
		return nil
	}
	chosen := make([]Download, 0, len(files))
	covered := make(map[string]bool, len(files))
	// A universal build fills only the formats the exact architecture left
	// uncovered, so a Mac is never offered both its own zip and the universal.
	for _, arch := range []string{goarch, "universal"} {
		for _, file := range files {
			if covered[file.Format] || !downloadable(file, platform, arch) {
				continue
			}
			chosen = append(chosen, downloadEntry(file))
			covered[file.Format] = true
		}
	}
	return chosen
}

func downloadable(file manifestFile, platform, arch string) bool {
	if file.OS != platform || file.Arch != arch {
		return false
	}
	return file.Kind == kindApp || file.Kind == kindInstaller
}

func downloadEntry(file manifestFile) Download {
	return Download{Name: file.Name, URL: file.URL, Size: file.Size, Arch: file.Arch, Format: file.Format}
}

func platformOf(goos string) string {
	switch goos {
	case "darwin":
		return "macos"
	case "linux", "windows":
		return goos
	}
	return ""
}

func (c *Checker) manifestURL() string {
	if c != nil && strings.TrimSpace(c.ManifestURL) != "" {
		return c.ManifestURL
	}
	return DefaultManifestURL
}

func (c *Checker) manifestFetch() Fetch {
	if c != nil && c.Fetch != nil {
		return c.Fetch
	}
	return httpFetch(nil, maxManifestBytes)
}
