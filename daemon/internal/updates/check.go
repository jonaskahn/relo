// Package updates reads a Sparkle appcast and reports whether a newer
// Relo build is published.
package updates

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// MethodDownload opens the public download page.
	MethodDownload = "download"
	// MethodSparkle asks Relo.app to install through Sparkle.
	MethodSparkle = "sparkle"
	// DefaultTTL is how long one check is reused before Relo reads the
	// feed again.
	DefaultTTL   = 6 * time.Hour
	maxFeedBytes = 1 << 20
	fetchTimeout = 10 * time.Second
)

// Feed errors name why an update check has nothing to report: a feed that
// answered with failure, one with no releases, and one with no version.
var (
	ErrFeedStatus  = errors.New("the update feed answered")
	ErrFeedEmpty   = errors.New("the update feed has no items")
	ErrFeedVersion = errors.New("the update feed has no version")
)

// Status is what the console and the tray show after a check.
type Status struct {
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	Available bool   `json:"available"`
	URL       string `json:"url"`
	Method    string `json:"method"`
}

// Fetch reads one feed URL.
type Fetch func(ctx context.Context, url string) ([]byte, error)

// Checker caches one feed read.
type Checker struct {
	URL      string
	Download string
	Current  string
	Method   string
	InBundle func() bool
	Fetch    Fetch
	Now      func() time.Time
	TTL      time.Duration

	mu        sync.Mutex
	cached    Status
	fetchedAt time.Time
}

// HTTPFetch reads a feed over HTTP.
func HTTPFetch(client *http.Client) Fetch {
	if client == nil {
		client = &http.Client{Timeout: fetchTimeout}
	}
	return func(ctx context.Context, url string) ([]byte, error) {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		response, err := client.Do(request)
		if err != nil {
			return nil, err
		}
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("%w %s", ErrFeedStatus, response.Status)
		}
		return io.ReadAll(io.LimitReader(response.Body, maxFeedBytes))
	}
}

// Status reports the last good check, reading the feed when the cache is
// empty or stale. A missing URL or a failed read reports no update.
func (c *Checker) Status(ctx context.Context) Status {
	if c == nil || strings.TrimSpace(c.URL) == "" {
		current := ""
		if c != nil {
			current = c.Current
		}
		return Status{Current: current, Method: methodOf(c), URL: downloadOf(c)}
	}
	now := time.Now()
	if c.Now != nil {
		now = c.Now()
	}
	ttl := c.TTL
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	if cached, found := c.cachedStatus(now, ttl); found {
		return cached
	}

	// The feed is read without the lock, so one slow fetch never stacks
	// every caller of the console behind it.
	status := Status{Current: c.Current, Method: methodOf(c), URL: downloadOf(c)}
	body, err := c.fetch()(ctx, c.URL)
	if err == nil {
		c.applyAppcast(&status, body)
	}
	c.storeStatus(status, now)
	return status
}

func (c *Checker) storeStatus(status Status, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cached = status
	c.fetchedAt = now
}

func (c *Checker) applyAppcast(status *Status, body []byte) {
	if latest, _, parseErr := ParseAppcast(body); parseErr == nil {
		status.Latest = latest
		status.Available = Newer(latest, c.Current)
	}
}

func (c *Checker) cachedStatus(now time.Time, ttl time.Duration) (Status, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.fetchedAt.IsZero() && now.Sub(c.fetchedAt) < ttl {
		return c.cached, true
	}
	return Status{}, false
}

func (c *Checker) fetch() Fetch {
	if c.Fetch != nil {
		return c.Fetch
	}
	return HTTPFetch(nil)
}

func methodOf(c *Checker) string {
	if c != nil && strings.TrimSpace(c.Method) != "" {
		return c.Method
	}
	inBundle := InAppBundle
	if c != nil && c.InBundle != nil {
		inBundle = c.InBundle
	}
	if runtime.GOOS == "darwin" && inBundle() {
		return MethodSparkle
	}
	return MethodDownload
}

func downloadOf(c *Checker) string {
	if c != nil && strings.TrimSpace(c.Download) != "" {
		return c.Download
	}
	return "https://github.com/jonaskahn/relo/releases/latest"
}

// InAppBundle reports whether this process is the Relo.app Mac binary.
func InAppBundle() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	return strings.Contains(filepath.ToSlash(exe), ".app/Contents/MacOS/")
}

type appcast struct {
	Channel struct {
		Items []appcastItem `xml:"item"`
	} `xml:"channel"`
}

type appcastItem struct {
	ShortVersion string           `xml:"http://www.andymatuschak.org/xml-namespaces/sparkle shortVersionString"`
	Enclosure    appcastEnclosure `xml:"enclosure"`
}

type appcastEnclosure struct {
	URL          string `xml:"url,attr"`
	ShortVersion string `xml:"http://www.andymatuschak.org/xml-namespaces/sparkle shortVersionString,attr"`
}

// ParseAppcast reads the newest short version and enclosure URL from a
// Sparkle RSS feed. The first item is the latest.
func ParseAppcast(body []byte) (version, url string, err error) {
	var feed appcast
	if err := xml.Unmarshal(body, &feed); err != nil {
		return "", "", fmt.Errorf("parse the update feed: %w", err)
	}
	if len(feed.Channel.Items) == 0 {
		return "", "", ErrFeedEmpty
	}
	item := feed.Channel.Items[0]
	version = strings.TrimSpace(item.ShortVersion)
	if version == "" {
		version = strings.TrimSpace(item.Enclosure.ShortVersion)
	}
	url = strings.TrimSpace(item.Enclosure.URL)
	if version == "" {
		return "", "", ErrFeedVersion
	}
	return version, url, nil
}

// Newer reports whether latest is a higher dotted version than current.
func Newer(latest, current string) bool {
	left := versionParts(latest)
	right := versionParts(current)
	n := len(left)
	if len(right) > n {
		n = len(right)
	}
	for i := 0; i < n; i++ {
		var a, b int
		if i < len(left) {
			a = left[i]
		}
		if i < len(right) {
			b = right[i]
		}
		if a != b {
			return a > b
		}
	}
	return false
}

func versionParts(value string) []int {
	trimmed := strings.TrimSpace(value)
	trimmed = strings.TrimPrefix(trimmed, "v")
	if i := strings.IndexAny(trimmed, "-+"); i >= 0 {
		trimmed = trimmed[:i]
	}
	if trimmed == "" {
		return nil
	}
	fields := strings.Split(trimmed, ".")
	parts := make([]int, 0, len(fields))
	for _, field := range fields {
		n, err := strconv.Atoi(field)
		if err != nil {
			return parts
		}
		parts = append(parts, n)
	}
	return parts
}
