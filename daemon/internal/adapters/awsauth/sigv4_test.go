package awsauth

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestSign(t *testing.T) {
	bodyStr := `{"prompt":"hello"}`
	req, err := http.NewRequest("POST", "https://bedrock-runtime.us-east-1.amazonaws.com/model/anthropic.claude-3-sonnet/converse", strings.NewReader(bodyStr))
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(bodyStr)
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	err = Sign(SignOptions{Request: req, Body: body,
		AccessKey: "AKIAEXAMPLE", SecretKey: "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY",
		Region: "us-east-1", Service: "bedrock", Now: now})
	if err != nil {
		t.Fatal(err)
	}

	auth := req.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "AWS4-HMAC-SHA256 Credential=AKIAEXAMPLE/20260925/us-east-1/bedrock/aws4_request") {
		t.Fatalf("unexpected auth header: %s", auth)
	}
	if !strings.Contains(auth, "Signature=") {
		t.Fatalf("auth header missing signature: %s", auth)
	}
}

func TestSignSessionToken(t *testing.T) {
	req, err := http.NewRequest("POST", "https://bedrock-runtime.us-east-1.amazonaws.com/model/x/converse", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	if err := Sign(SignOptions{Request: req,
		AccessKey: "AKIAEXAMPLE", SecretKey: "secret", SessionToken: "session-token",
		Region: "us-east-1", Service: "bedrock", Now: now}); err != nil {
		t.Fatal(err)
	}
	if req.Header.Get("x-amz-security-token") != "session-token" {
		t.Fatalf("security token header = %q, want the session token", req.Header.Get("x-amz-security-token"))
	}
	auth := req.Header.Get("Authorization")
	if !strings.Contains(auth, "SignedHeaders=content-type;host;x-amz-content-sha256;x-amz-date;x-amz-security-token") {
		t.Fatalf("signed headers = %q, want content-type and the session token included", auth)
	}
}

func TestSignZeroTime(t *testing.T) {
	req, err := http.NewRequest("POST", "https://bedrock-runtime.us-east-1.amazonaws.com/model/x/converse", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := Sign(SignOptions{Request: req,
		AccessKey: "AKIAEXAMPLE", SecretKey: "secret", Region: "us-east-1", Service: "bedrock"}); err != nil {
		t.Fatal(err)
	}
	if req.Header.Get("x-amz-date") == "" {
		t.Fatal("x-amz-date was not stamped for a zero time")
	}
}

func TestCanonicalQueryString(t *testing.T) {
	tests := []struct {
		name   string
		values url.Values
		want   string
	}{
		{"no values sorts to nothing", url.Values{}, ""},
		{"one pair passes through", url.Values{"a": {"1"}}, "a=1"},
		{"keys sort", url.Values{"b": {"2"}, "a": {"1"}}, "a=1&b=2"},
		{"values sort", url.Values{"k": {"b", "a"}}, "k=a&k=b"},
		{"spaces encode", url.Values{"q": {"hello world"}}, "q=hello+world"},
		{"keys encode", url.Values{"a b": {"c"}}, "a+b=c"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := canonicalQueryString(tc.values); got != tc.want {
				t.Fatalf("canonicalQueryString(%v) = %q, want %q", tc.values, got, tc.want)
			}
		})
	}
}

func TestReadAndReplaceBody(t *testing.T) {
	t.Run("a missing body reads as nothing", func(t *testing.T) {
		req, err := http.NewRequest("POST", "https://example.test/", nil)
		if err != nil {
			t.Fatal(err)
		}
		body, err := ReadAndReplaceBody(req)
		if err != nil || body != nil {
			t.Fatalf("ReadAndReplaceBody() = %q, %v, want nil, nil", body, err)
		}
	})

	t.Run("a body reads and is restored", func(t *testing.T) {
		req, err := http.NewRequest("POST", "https://example.test/", strings.NewReader("abc"))
		if err != nil {
			t.Fatal(err)
		}
		body, err := ReadAndReplaceBody(req)
		if err != nil || string(body) != "abc" {
			t.Fatalf("ReadAndReplaceBody() = %q, %v, want abc, nil", body, err)
		}
		restored, err := io.ReadAll(req.Body)
		if err != nil || string(restored) != "abc" {
			t.Fatalf("restored body = %q, %v, want abc, nil", restored, err)
		}
	})
}
