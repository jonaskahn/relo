package updates_test

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/updates"
)

const sampleAppcast = `<?xml version="1.0" encoding="utf-8"?>
<rss version="2.0" xmlns:sparkle="http://www.andymatuschak.org/xml-namespaces/sparkle">
  <channel>
    <title>Relo Updates</title>
    <item>
      <title>Version 0.2.0</title>
      <sparkle:shortVersionString>0.2.0</sparkle:shortVersionString>
      <enclosure url="https://github.com/jonaskahn/relo/releases/download/v0.2.0/Relo-0.2.0-universal.zip"
        sparkle:shortVersionString="0.2.0"
        type="application/octet-stream"/>
    </item>
    <item>
      <title>Version 0.1.0</title>
      <sparkle:shortVersionString>0.1.0</sparkle:shortVersionString>
      <enclosure url="https://example.com/old.zip" type="application/octet-stream"/>
    </item>
  </channel>
</rss>
`

const sampleReleases = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom" xml:lang="en-US">
  <title>Release notes from relo</title>
  <entry>
    <title>v0.1.3 - Hot Noodle 🍜</title>
    <link rel="alternate" type="text/html" href="https://github.com/jonaskahn/relo/releases/tag/v0.1.3"/>
  </entry>
  <entry>
    <title>v0.1.4-beta.28.1a2b3c4d</title>
    <link rel="alternate" type="text/html" href="https://github.com/jonaskahn/relo/releases/tag/v0.1.4-beta.28.1a2b3c4d"/>
  </entry>
  <entry>
    <title>v0.1.4-beta.27.8d9c8af7</title>
    <link rel="alternate" type="text/html" href="https://github.com/jonaskahn/relo/releases/tag/v0.1.4-beta.27.8d9c8af7"/>
  </entry>
</feed>
`

const betaCatalog = `{
  "latest": "v0.1.4",
  "latest_beta": "v0.1.4-beta.28.1a2b3c4d",
  "releases": [
    {
      "tag": "v0.1.4-beta.28.1a2b3c4d",
      "version": "0.1.4-beta.28.1a2b3c4d",
      "page": "https://github.com/jonaskahn/relo/releases/tag/v0.1.4-beta.28.1a2b3c4d",
      "notes": "## What's Changed\n* The newest beta.\n",
      "files": [
        {"name": "relo-0.1.4-beta.28.1a2b3c4d-darwin-arm64.tar.gz", "url": "https://example.com/beta.tar.gz", "os": "macos", "arch": "arm64", "kind": "archive", "format": "tar.gz"}
      ]
    }
  ]
}`

func TestInAppBundle(t *testing.T) {
	if updates.InAppBundle() {
		t.Fatal("InAppBundle() = true, want false in the test binary")
	}
}

func TestParseAppcast(t *testing.T) {
	t.Run("the newest item is the first one", func(t *testing.T) {
		version, url, err := updates.ParseAppcast([]byte(sampleAppcast))
		if err != nil {
			t.Fatalf("ParseAppcast() error = %v", err)
		}
		if version != "0.2.0" {
			t.Fatalf("version = %q, want 0.2.0", version)
		}
		if url != "https://github.com/jonaskahn/relo/releases/download/v0.2.0/Relo-0.2.0-universal.zip" {
			t.Fatalf("url = %q", url)
		}
	})

	t.Run("a bad feed is reported", func(t *testing.T) {
		if _, _, err := updates.ParseAppcast([]byte("<not>xml")); err == nil {
			t.Fatal("ParseAppcast() error = nil, want a parse failure")
		}
	})

	t.Run("an empty channel is reported", func(t *testing.T) {
		body := []byte(`<rss><channel></channel></rss>`)
		if _, _, err := updates.ParseAppcast(body); !errors.Is(err, updates.ErrFeedEmpty) {
			t.Fatalf("ParseAppcast() error = %v, want ErrFeedEmpty", err)
		}
	})

	t.Run("a versionless item is reported", func(t *testing.T) {
		body := []byte(`<rss><channel><item><title>x</title></item></channel></rss>`)
		if _, _, err := updates.ParseAppcast(body); !errors.Is(err, updates.ErrFeedVersion) {
			t.Fatalf("ParseAppcast() error = %v, want ErrFeedVersion", err)
		}
	})
}

func TestNewer(t *testing.T) {
	tests := []struct {
		name    string
		latest  string
		current string
		want    bool
	}{
		{name: "a newer version", latest: "0.2.0", current: "0.1.0", want: true},
		{name: "the same version", latest: "0.1.0", current: "0.1.0", want: false},
		{name: "an older version", latest: "0.1.0", current: "0.2.0", want: false},
		{name: "a v prefix", latest: "v0.2.0", current: "0.1.9", want: true},
		{name: "a prerelease of a newer version", latest: "0.2.0-beta", current: "0.1.0", want: true},
		{name: "no version at all", latest: "", current: "0.1.0", want: false},
		// A beta is published before the release it leads to, so the release is
		// the newer of the two and has to be offered to the build it replaces.
		{
			name:    "the release that supersedes a beta",
			latest:  "0.1.0",
			current: "0.1.0-beta.1.98e02113",
			want:    true,
		},
		{name: "a beta never outranks its release", latest: "0.1.0-beta.1", current: "0.1.0", want: false},
		{name: "a later beta", latest: "0.1.0-beta.2", current: "0.1.0-beta.1", want: true},
		{
			name:    "the same beta",
			latest:  "0.1.0-beta.1.98e02113",
			current: "0.1.0-beta.1.98e02113",
			want:    false,
		},
		{name: "two builds of one run", latest: "0.1.0-beta.1.a", current: "0.1.0-beta.1.b", want: false},
		{name: "the run number orders two builds", latest: "0.1.0-beta.418.a", current: "0.1.0-beta.417.z", want: true},
		{name: "a build suffix states no order", latest: "0.1.1+abc", current: "0.1.1", want: false},
		// The tail is read field by field, so a number always ranks below a
		// word whatever the build hash beside it says, and a tail that stops
		// early is the shorter of the two.
		{name: "a number ranks below a word", latest: "0.1.0-beta.a", current: "0.1.0-beta.1", want: true},
		{name: "a word ranks above a number", latest: "0.1.0-beta.1", current: "0.1.0-beta.a", want: false},
		{name: "the shorter tail ranks below", latest: "0.1.0-beta", current: "0.1.0-beta.1", want: false},
		{name: "the longer tail ranks above", latest: "0.1.0-beta.1", current: "0.1.0-beta", want: true},
		{name: "an empty tail field ranks below a word", latest: "0.1.0-beta.a", current: "0.1.0-beta.", want: true},
		// A version field that is not a number ends the reading: the rest of
		// the version is not compared, so what follows decides nothing.
		{name: "a field that is not a number ends the version", latest: "0.1.x.9", current: "0.1.0", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := updates.Newer(tt.latest, tt.current); got != tt.want {
				t.Fatalf("Newer(%q, %q) = %v, want %v", tt.latest, tt.current, got, tt.want)
			}
		})
	}
}

func TestChecker(t *testing.T) {
	t.Run("a newer feed reports an update", func(t *testing.T) {
		checker := &updates.Checker{
			URL:     "https://example.com/appcast.xml",
			Current: "0.1.0",
			Fetch: func(context.Context, string) ([]byte, error) {
				return []byte(sampleAppcast), nil
			},
		}
		status := checker.Status(context.Background())
		if !status.Available || status.Latest != "0.2.0" || status.Current != "0.1.0" {
			t.Fatalf("status = %+v, want an update to 0.2.0", status)
		}
		if status.Method != updates.MethodDownload {
			t.Fatalf("method = %q, want %s", status.Method, updates.MethodDownload)
		}
		if status.URL != "https://github.com/jonaskahn/relo/releases/latest" {
			t.Fatalf("url = %q, want the download page", status.URL)
		}
	})

	t.Run("the same version is not an update", func(t *testing.T) {
		checker := &updates.Checker{
			URL:     "https://example.com/appcast.xml",
			Current: "0.2.0",
			Fetch: func(context.Context, string) ([]byte, error) {
				return []byte(sampleAppcast), nil
			},
		}
		status := checker.Status(context.Background())
		if status.Available {
			t.Fatalf("status = %+v, want no update", status)
		}
	})

	t.Run("an empty URL turns the check off", func(t *testing.T) {
		checker := &updates.Checker{Current: "0.1.0"}
		status := checker.Status(context.Background())
		if status.Available || status.Current != "0.1.0" {
			t.Fatalf("status = %+v, want the current version and no update", status)
		}
	})

	t.Run("a failed fetch reports no update", func(t *testing.T) {
		checker := &updates.Checker{
			URL:     "https://example.com/appcast.xml",
			Current: "0.1.0",
			Fetch: func(context.Context, string) ([]byte, error) {
				return nil, errors.New("offline")
			},
		}
		status := checker.Status(context.Background())
		if status.Available {
			t.Fatalf("status = %+v, want no update after a failed fetch", status)
		}
	})

	t.Run("a cached result is reused", func(t *testing.T) {
		calls := 0
		now := time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)
		checker := &updates.Checker{
			URL:     "https://example.com/appcast.xml",
			Current: "0.1.0",
			Now:     func() time.Time { return now },
			TTL:     time.Hour,
			Fetch: func(_ context.Context, url string) ([]byte, error) {
				if url == updates.DefaultManifestURL {
					return nil, errors.New("no catalog")
				}
				calls++
				return []byte(sampleAppcast), nil
			},
		}
		_ = checker.Status(context.Background())
		now = now.Add(30 * time.Minute)
		_ = checker.Status(context.Background())
		if calls != 1 {
			t.Fatalf("fetch calls = %d, want 1", calls)
		}
	})

	t.Run("a slow fetch does not stack callers behind it", func(t *testing.T) {
		arrived := make(chan struct{}, 2)
		release := make(chan struct{})
		checker := &updates.Checker{
			URL:     "https://example.com/appcast.xml",
			Current: "0.1.0",
			Fetch: func(context.Context, string) ([]byte, error) {
				arrived <- struct{}{}
				<-release
				return []byte(sampleAppcast), nil
			},
		}
		finished := make(chan struct{}, 2)
		for i := 0; i < 2; i++ {
			go func() {
				checker.Status(context.Background())
				finished <- struct{}{}
			}()
		}
		// Both callers reach the feed instead of waiting on each other's read.
		for i := 0; i < 2; i++ {
			select {
			case <-arrived:
			case <-time.After(2 * time.Second):
				t.Fatal("a caller waited behind another caller's feed read")
			}
		}
		close(release)
		for i := 0; i < 2; i++ {
			<-finished
		}
	})

	t.Run("a Relo.app process reports sparkle", func(t *testing.T) {
		checker := &updates.Checker{
			URL:     "https://example.com/appcast.xml",
			Current: "0.1.0",
			InBundle: func() bool {
				return true
			},
			Fetch: func(context.Context, string) ([]byte, error) {
				return []byte(sampleAppcast), nil
			},
		}
		if runtime.GOOS != "darwin" {
			checker.Method = updates.MethodSparkle
		}
		status := checker.Status(context.Background())
		if status.Method != updates.MethodSparkle {
			t.Fatalf("method = %q, want %s", status.Method, updates.MethodSparkle)
		}
	})

	t.Run("a configured download page is what the notice opens", func(t *testing.T) {
		checker := &updates.Checker{
			URL:      "https://example.com/appcast.xml",
			Download: "https://relo.ifelse.one/#download",
			Current:  "0.1.0",
			Fetch: func(context.Context, string) ([]byte, error) {
				return []byte(sampleAppcast), nil
			},
		}
		status := checker.Status(context.Background())
		if status.URL != "https://relo.ifelse.one/#download" {
			t.Fatalf("url = %q, want the configured download page", status.URL)
		}
	})
}

func TestParseReleases(t *testing.T) {
	t.Run("the highest prerelease wins", func(t *testing.T) {
		version, url, err := updates.ParseReleases([]byte(sampleReleases))
		if err != nil {
			t.Fatalf("ParseReleases() error = %v", err)
		}
		if version != "0.1.4-beta.28.1a2b3c4d" {
			t.Fatalf("version = %q, want the newer beta", version)
		}
		if url != "https://github.com/jonaskahn/relo/releases/tag/v0.1.4-beta.28.1a2b3c4d" {
			t.Fatalf("url = %q, want the beta tag page", url)
		}
	})

	t.Run("an empty feed is reported", func(t *testing.T) {
		body := []byte(`<feed xmlns="http://www.w3.org/2005/Atom"></feed>`)
		if _, _, err := updates.ParseReleases(body); !errors.Is(err, updates.ErrFeedEmpty) {
			t.Fatalf("ParseReleases() error = %v, want ErrFeedEmpty", err)
		}
	})

	t.Run("a feed with no prerelease is reported", func(t *testing.T) {
		body := []byte(`<feed xmlns="http://www.w3.org/2005/Atom">
		  <entry><link href="https://github.com/jonaskahn/relo/releases/tag/v0.1.3"/></entry>
		</feed>`)
		if _, _, err := updates.ParseReleases(body); !errors.Is(err, updates.ErrFeedPrerelease) {
			t.Fatalf("ParseReleases() error = %v, want ErrFeedPrerelease", err)
		}
	})

	t.Run("a malformed feed is reported", func(t *testing.T) {
		if _, _, err := updates.ParseReleases([]byte("<not>xml")); err == nil {
			t.Fatal("ParseReleases() error = nil, want a parse failure")
		}
	})

	t.Run("an entry without a tag link is skipped", func(t *testing.T) {
		body := []byte(`<feed xmlns="http://www.w3.org/2005/Atom">
		  <entry><link href="https://github.com/jonaskahn/relo/releases"/></entry>
		  <entry><link href="https://github.com/jonaskahn/relo/releases/tag/v0.1.4-beta.2.aaaaaaaa"/></entry>
		</feed>`)
		version, _, err := updates.ParseReleases(body)
		if err != nil {
			t.Fatalf("ParseReleases() error = %v", err)
		}
		if version != "0.1.4-beta.2.aaaaaaaa" {
			t.Fatalf("version = %q, want the tag entry", version)
		}
	})
}

func TestCheckerBeta(t *testing.T) {
	const betaFeed = "https://example.com/releases.atom"

	t.Run("a beta build reads the releases feed", func(t *testing.T) {
		checker := &updates.Checker{
			URL:         "https://example.com/appcast.xml",
			ReleasesURL: betaFeed,
			Current:     "0.1.4-beta.27.8d9c8af7",
			Installer:   updates.InstallerShell,
			Fetch: func(_ context.Context, url string) ([]byte, error) {
				if url != betaFeed {
					return nil, fmt.Errorf("unexpected feed %s", url)
				}
				return []byte(sampleReleases), nil
			},
		}
		status := checker.Status(context.Background())
		if !status.Available || status.Latest != "0.1.4-beta.28.1a2b3c4d" {
			t.Fatalf("status = %+v, want an update to the next beta", status)
		}
		if status.URL != "https://github.com/jonaskahn/relo/releases/tag/v0.1.4-beta.28.1a2b3c4d" {
			t.Fatalf("url = %q, want the beta tag page", status.URL)
		}
		if status.Installer != updates.InstallerShell {
			t.Fatalf("installer = %q, want %q", status.Installer, updates.InstallerShell)
		}
	})

	t.Run("a v-prefixed beta is still a prerelease", func(t *testing.T) {
		checker := &updates.Checker{
			URL:         "https://example.com/appcast.xml",
			ReleasesURL: betaFeed,
			Current:     "v0.1.4-beta.27.8d9c8af7",
			Fetch: func(_ context.Context, url string) ([]byte, error) {
				if url != betaFeed {
					return nil, fmt.Errorf("unexpected feed %s", url)
				}
				return []byte(sampleReleases), nil
			},
		}
		if status := checker.Status(context.Background()); !status.Available {
			t.Fatalf("status = %+v, want an update", status)
		}
	})

	t.Run("the current beta is not an update", func(t *testing.T) {
		checker := &updates.Checker{
			URL:         "https://example.com/appcast.xml",
			ReleasesURL: betaFeed,
			Current:     "0.1.4-beta.28.1a2b3c4d",
			Fetch: func(context.Context, string) ([]byte, error) {
				return []byte(sampleReleases), nil
			},
		}
		if status := checker.Status(context.Background()); status.Available {
			t.Fatalf("status = %+v, want no update", status)
		}
	})

	t.Run("a stable build never reads the releases feed", func(t *testing.T) {
		checker := &updates.Checker{
			URL:         "https://example.com/appcast.xml",
			ReleasesURL: betaFeed,
			Current:     "0.1.0",
			Fetch: func(_ context.Context, url string) ([]byte, error) {
				if url == betaFeed {
					return nil, errors.New("the beta feed must not be read")
				}
				return []byte(sampleAppcast), nil
			},
		}
		status := checker.Status(context.Background())
		if !status.Available || status.Latest != "0.2.0" {
			t.Fatalf("status = %+v, want the appcast update", status)
		}
	})

	t.Run("an empty URL turns the beta check off", func(t *testing.T) {
		checker := &updates.Checker{
			ReleasesURL: betaFeed,
			Current:     "0.1.4-beta.27.8d9c8af7",
			Fetch: func(context.Context, string) ([]byte, error) {
				return nil, errors.New("a disabled check must not read a feed")
			},
		}
		status := checker.Status(context.Background())
		if status.Available {
			t.Fatalf("status = %+v, want no update", status)
		}
		if status.Installer != updates.InstallerUnknown {
			t.Fatalf("installer = %q, want %q", status.Installer, updates.InstallerUnknown)
		}
	})

	t.Run("a failed beta fetch reports no update and keeps the installer", func(t *testing.T) {
		checker := &updates.Checker{
			URL:         "https://example.com/appcast.xml",
			ReleasesURL: betaFeed,
			Current:     "0.1.4-beta.27.8d9c8af7",
			Installer:   updates.InstallerBrew,
			Fetch: func(context.Context, string) ([]byte, error) {
				return nil, errors.New("offline")
			},
		}
		status := checker.Status(context.Background())
		if status.Available {
			t.Fatalf("status = %+v, want no update after a failed fetch", status)
		}
		if status.Installer != updates.InstallerBrew {
			t.Fatalf("installer = %q, want %q", status.Installer, updates.InstallerBrew)
		}
	})
}

func TestCheckerBetaCatalog(t *testing.T) {
	const (
		betaFeed    = "https://example.com/releases.atom"
		catalogURL  = "https://example.com/releases.json"
		betaCurrent = "0.1.4-beta.27.8d9c8af7"
	)
	checker := func(fetch updates.Fetch) *updates.Checker {
		return &updates.Checker{
			URL:         "https://example.com/appcast.xml",
			ReleasesURL: betaFeed,
			ManifestURL: catalogURL,
			Current:     betaCurrent,
			Installer:   updates.InstallerShell,
			Fetch:       fetch,
		}
	}

	t.Run("the catalog names the newest beta without reading the feed", func(t *testing.T) {
		status := checker(func(_ context.Context, url string) ([]byte, error) {
			switch url {
			case catalogURL:
				return []byte(betaCatalog), nil
			case betaFeed:
				return nil, errors.New("the catalog already answered")
			default:
				return nil, fmt.Errorf("unexpected feed %s", url)
			}
		}).Status(context.Background())
		if !status.Available || status.Latest != "0.1.4-beta.28.1a2b3c4d" {
			t.Fatalf("status = %+v, want the catalog's newest beta", status)
		}
		if status.URL != "https://github.com/jonaskahn/relo/releases/tag/v0.1.4-beta.28.1a2b3c4d" {
			t.Fatalf("url = %q, want the beta tag page", status.URL)
		}
		if status.Notes == "" {
			t.Fatalf("status = %+v, want the release notes", status)
		}
		if status.Installer != updates.InstallerShell {
			t.Fatalf("installer = %q, want %q", status.Installer, updates.InstallerShell)
		}
	})

	t.Run("a catalog without a beta falls back to the releases feed", func(t *testing.T) {
		status := checker(func(_ context.Context, url string) ([]byte, error) {
			switch url {
			case catalogURL:
				return []byte(`{"latest":"v0.1.3","latest_beta":null,"releases":[]}`), nil
			case betaFeed:
				return []byte(sampleReleases), nil
			default:
				return nil, fmt.Errorf("unexpected feed %s", url)
			}
		}).Status(context.Background())
		if !status.Available || status.Latest != "0.1.4-beta.28.1a2b3c4d" {
			t.Fatalf("status = %+v, want the feed's newest beta", status)
		}
	})

	t.Run("an unreadable catalog falls back to the releases feed", func(t *testing.T) {
		status := checker(func(_ context.Context, url string) ([]byte, error) {
			if url == betaFeed {
				return []byte(sampleReleases), nil
			}
			return nil, errors.New("offline")
		}).Status(context.Background())
		if !status.Available || status.Latest != "0.1.4-beta.28.1a2b3c4d" {
			t.Fatalf("status = %+v, want the feed's newest beta", status)
		}
	})

	t.Run("a beta check is fresh again after half a minute", func(t *testing.T) {
		calls := 0
		now := time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC)
		instance := checker(func(_ context.Context, url string) ([]byte, error) {
			calls++
			switch url {
			case catalogURL:
				return []byte(betaCatalog), nil
			default:
				return nil, fmt.Errorf("unexpected feed %s", url)
			}
		})
		instance.Now = func() time.Time { return now }
		_ = instance.Status(context.Background())
		now = now.Add(29 * time.Second)
		_ = instance.Status(context.Background())
		if calls != 1 {
			t.Fatalf("fetch calls = %d, want the cached read", calls)
		}
		now = now.Add(time.Second)
		_ = instance.Status(context.Background())
		if calls != 2 {
			t.Fatalf("fetch calls = %d, want a fresh read after 30s", calls)
		}
	})
}
