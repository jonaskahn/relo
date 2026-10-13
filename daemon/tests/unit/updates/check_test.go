package updates_test

import (
	"context"
	"errors"
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
			Fetch: func(context.Context, string) ([]byte, error) {
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
