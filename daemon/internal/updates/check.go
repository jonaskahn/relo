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

// Newer reports whether latest outranks current: a higher dotted version, or
// the same version where current carries a prerelease and latest does not.
func Newer(latest, current string) bool {
	return compare(parseVersion(latest), parseVersion(current)) > 0
}

// parsedVersion is one version as the feed and the running build state it: the
// dotted numbers, and the prerelease identifiers behind them. A beta exists to
// be replaced by the release it leads to, so the identifiers only decide the
// order within a version -- never across it.
type parsedVersion struct {
	numbers    []int
	prerelease []string
}

func parseVersion(value string) parsedVersion {
	trimmed := strings.TrimPrefix(strings.TrimSpace(value), "v")
	// Build metadata states no order, and the numbers stop at the first field
	// that is not one, so a feed naming a version loosely still compares on
	// the part it does state.
	if i := strings.IndexByte(trimmed, '+'); i >= 0 {
		trimmed = trimmed[:i]
	}
	var prerelease []string
	if i := strings.IndexByte(trimmed, '-'); i >= 0 {
		if tail := trimmed[i+1:]; tail != "" {
			prerelease = strings.Split(tail, ".")
		}
		trimmed = trimmed[:i]
	}
	var numbers []int
	if trimmed != "" {
		for _, field := range strings.Split(trimmed, ".") {
			n, err := strconv.Atoi(field)
			if err != nil {
				break
			}
			numbers = append(numbers, n)
		}
	}
	return parsedVersion{numbers: numbers, prerelease: prerelease}
}

// compare orders two versions by their numbers first, so a prerelease of a
// newer version still outranks an older release, and by the prerelease only
// where the numbers are equal.
func compare(left, right parsedVersion) int {
	if order := compareNumbers(left.numbers, right.numbers); order != 0 {
		return order
	}
	return comparePrerelease(left.prerelease, right.prerelease)
}

func compareNumbers(left, right []int) int {
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
			return compareInts(a, b)
		}
	}
	return 0
}

// comparePrerelease reads the two tails: no identifiers is a release and
// outranks any prerelease of the same version, which is the whole reason a beta
// never reads as newer than the release that replaces it. Two prereleases are
// read left to right, and a longer tail wins a shared prefix.
func comparePrerelease(left, right []string) int {
	if len(left) == 0 && len(right) == 0 {
		return 0
	}
	if len(left) == 0 {
		return 1
	}
	if len(right) == 0 {
		return -1
	}
	n := len(left)
	if len(right) < n {
		n = len(right)
	}
	for i := 0; i < n; i++ {
		if rank := compareIdentifier(left[i], right[i]); rank != 0 {
			return rank
		}
	}
	return compareInts(len(left), len(right))
}

// compareIdentifier orders one identifier. Two numbers order as numbers, and a
// number always ranks below a word, so `beta.417` precedes `beta.418` whatever
// the commit hash in the last field says. A number too large to read orders as
// the word it is written as, which no version Relo ships reaches.
func compareIdentifier(left, right string) int {
	leftNumber, leftIsNumber := identifierNumber(left)
	rightNumber, rightIsNumber := identifierNumber(right)
	switch {
	case leftIsNumber && rightIsNumber:
		return compareInts(leftNumber, rightNumber)
	case leftIsNumber:
		return -1
	case rightIsNumber:
		return 1
	default:
		return strings.Compare(left, right)
	}
}

func identifierNumber(value string) (int, bool) {
	if value == "" {
		return 0, false
	}
	n, err := strconv.Atoi(value)
	return n, err == nil
}

func compareInts(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
