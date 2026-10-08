package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/server"
	"github.com/jonaskahn/relo/internal/updates"
)

const sampleAppcast = `<?xml version="1.0" encoding="utf-8"?>
<rss version="2.0" xmlns:sparkle="http://www.andymatuschak.org/xml-namespaces/sparkle">
  <channel>
    <item>
      <sparkle:shortVersionString>0.2.0</sparkle:shortVersionString>
      <enclosure url="https://example.com/Relo-0.2.0-universal.zip"
        sparkle:shortVersionString="0.2.0"
        type="application/octet-stream"/>
    </item>
  </channel>
</rss>
`

func TestUpdatesRoute(t *testing.T) {
	t.Run("the console can read an update notice", func(t *testing.T) {
		harness := newHarness(t)
		harness.server = harness.newServerWithOptions(harness.cfg, []account.PoolEntry{defaultEntry()},
			func(options *server.Options) {
				options.Updates = &updates.Checker{
					URL:     "https://example.com/appcast.xml",
					Current: "0.1.0",
					Fetch: func(context.Context, string) ([]byte, error) {
						return []byte(sampleAppcast), nil
					},
				}
			})
		recorder := harness.management(http.MethodGet, "/api/v1/updates", adminToken, nil)
		if recorder.Code != http.StatusOK {
			t.Fatalf("GET /api/v1/updates = %d, want 200 (body %s)", recorder.Code, recorder.Body.String())
		}
		var notice updates.Status
		if err := json.Unmarshal(recorder.Body.Bytes(), &notice); err != nil {
			t.Fatalf("decode the update notice: %v", err)
		}
		if !notice.Available || notice.Latest != "0.2.0" {
			t.Fatalf("notice = %+v, want an update to 0.2.0", notice)
		}
	})
}
