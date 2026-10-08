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
		latest  string
		current string
		want    bool
	}{
		{"0.2.0", "0.1.0", true},
		{"0.1.0", "0.1.0", false},
		{"0.1.0", "0.2.0", false},
		{"v0.2.0", "0.1.9", true},
		{"0.2.0-beta", "0.1.0", true},
		{"", "0.1.0", false},
	}
	for _, tt := range tests {
		if got := updates.Newer(tt.latest, tt.current); got != tt.want {
			t.Fatalf("Newer(%q, %q) = %v, want %v", tt.latest, tt.current, got, tt.want)
		}
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
