// Tray update fetch: reading daemon state over HTTP.
package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

const fetchTimeout = 3 * time.Second

const maxResponseBytes = 1 << 20

// ErrFetchStatus names why the tray has nothing fresh to show: a daemon that
// answered with failure.
var ErrFetchStatus = errors.New("the management API answered")

// UpdateInfo is the part of the update check the tray shows.
type UpdateInfo struct {
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	Available bool   `json:"available"`
	URL       string `json:"url"`
	Method    string `json:"method"`
}

// FetchUpdates reads whether a newer build is published.
func FetchUpdates(ctx context.Context, client *http.Client, baseURL, adminToken string) (UpdateInfo, error) {
	var info UpdateInfo
	if err := getJSON(ctx, client, baseURL+"/api/v1/updates", adminToken, &info); err != nil {
		return UpdateInfo{}, err
	}
	return info, nil
}

func getJSON(ctx context.Context, client *http.Client, url, adminToken string, into any) error {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+adminToken)
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%w %s", ErrFetchStatus, response.Status)
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, maxResponseBytes)).Decode(into); err != nil {
		return fmt.Errorf("read the management API response: %w", err)
	}
	return nil
}
