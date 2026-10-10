// The worker that keeps the Antigravity client version current.
package platform

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"time"

	"github.com/jonaskahn/relo/internal/adapters/antigravity"
)

// ErrManifestUnreadable reports a manifest that answered but named no version.
var ErrManifestUnreadable = errors.New("the Antigravity manifest named no version")

// The vendor gates newer models by client version, so the version is checked
// every twelve to twenty-four hours. The interval is drawn per cycle because
// every Relo asking at one instant is the pattern a fixed schedule makes.
const (
	antigravityVersionDelay  = 90 * time.Second
	antigravityVersionBase   = 12 * time.Hour
	antigravityVersionSpread = 12 * time.Hour
	antigravityManifestLimit = 64 << 10
	antigravityManifestWait  = 30 * time.Second
)

func startAntigravityVersion(ctx context.Context, built deps) {
	client := &http.Client{Timeout: antigravityManifestWait}
	go func() {
		if !sleep(ctx, antigravityVersionDelay) {
			return
		}
		for {
			refreshAntigravityVersion(ctx, client, built)
			if !sleep(ctx, antigravityVersionBase+rand.N(antigravityVersionSpread)) {
				return
			}
		}
	}()
}

func refreshAntigravityVersion(ctx context.Context, client *http.Client, built deps) {
	version, err := fetchAntigravityVersion(ctx, client)
	if err != nil {
		built.logger.Debug("keep the Antigravity version", "error", err)
		return
	}
	antigravity.SetVersion(version)
}

func fetchAntigravityVersion(ctx context.Context, client *http.Client) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, antigravity.ManifestURL, nil)
	if err != nil {
		return "", fmt.Errorf("build the manifest request: %w", err)
	}
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("fetch the manifest: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch the manifest: %w: status %d", ErrManifestUnreadable, response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, antigravityManifestLimit))
	if err != nil {
		return "", fmt.Errorf("read the manifest: %w", err)
	}
	if version := antigravity.ParseVersion(string(body)); version != "" {
		return version, nil
	}
	return "", ErrManifestUnreadable
}
