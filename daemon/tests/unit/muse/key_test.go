package muse_test

import (
	"encoding/json"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/muse"
)

func TestKeyResponseReadsVendorFields(t *testing.T) {
	response, err := muse.Decode([]byte(`{
		"api_key":"LLM|key",
		"user_id":" user-1 ",
		"user_email":" Muse@Example.com ",
		"subs_tier_id":"tier-id"
	}`))
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if id, email := response.Identity(); id != "user-1" || email != "muse@example.com" {
		t.Fatalf("Identity() = %q/%q, want the trimmed identity", id, email)
	}
	if tier := response.Tier(); tier != "tier-id" {
		t.Fatalf("Tier() = %q, want the tier identifier", tier)
	}

	byEmail, err := muse.Decode([]byte(`{"user_email":"muse@example.com"}`))
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if id, _ := byEmail.Identity(); id != "muse@example.com" {
		t.Fatalf("Identity() id = %q, want the email fallback", id)
	}

	if _, err := muse.Decode([]byte(`{`)); err == nil {
		t.Fatal("Decode() error = nil, want malformed JSON refused")
	}
}

func TestWindowReadsVendorResetFormats(t *testing.T) {
	cases := []struct {
		name string
		body string
		want int64
	}{
		{name: "empty", body: `{}`, want: 0},
		{name: "null", body: `{"resets_at":null}`, want: 0},
		{name: "seconds", body: `{"resets_at":1800000000}`, want: 1800000000},
		{name: "milliseconds", body: `{"resets_at":1800000000000}`, want: 1800000000},
		{name: "quoted seconds", body: `{"resets_at":"1800000000"}`, want: 1800000000},
		{name: "RFC3339", body: `{"resets_at":"2030-01-08T00:00:00Z"}`, want: 1894060800},
		{name: "unreadable", body: `{"resets_at":true}`, want: 0},
		{name: "unparseable text", body: `{"resets_at":"tomorrow"}`, want: 0},
		{name: "negative", body: `{"resets_at":-1}`, want: 0},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			var window muse.Window
			if err := json.Unmarshal([]byte(tt.body), &window); err != nil {
				t.Fatalf("decode window: %v", err)
			}
			if got := window.ResetAt(); got != tt.want {
				t.Fatalf("ResetAt() = %d, want %d", got, tt.want)
			}
		})
	}

	defaultWindow := muse.Window{}
	if got := defaultWindow.Seconds(); got != 5*60 {
		t.Fatalf("Seconds() = %d, want the rolling default", got)
	}
	zeroDuration := 0.0
	zeroWindow := muse.Window{WindowDurationMins: &zeroDuration}
	if got := zeroWindow.Seconds(); got != 5*60 {
		t.Fatalf("Seconds() = %d, want the default for a zero duration", got)
	}
	duration := 90.0
	durationWindow := muse.Window{WindowDurationMins: &duration}
	if got := durationWindow.Seconds(); got != 90*60 {
		t.Fatalf("Seconds() = %d, want the vendor duration", got)
	}
}
