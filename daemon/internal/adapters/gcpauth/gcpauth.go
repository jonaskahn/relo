// Package gcpauth resolves GCP credentials and exchanges them for the
// tokens Google and Vertex endpoints accept.
package gcpauth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Token errors name the refusals callers map to their own wording: a key
// file missing its fields, a token endpoint that refused, and a private key
// that cannot be parsed.
var (
	ErrInvalidServiceAccount = errors.New("invalid service account key: missing client_email or private_key")
	ErrTokenStatus           = errors.New("token request returned status")
	ErrUndecodablePEM        = errors.New("failed to decode PEM block for private key")
	ErrNonRSAKey             = errors.New("PKCS#8 key is not an RSA private key")
)

// ServiceAccountKey represents a Google service account credentials JSON.
type ServiceAccountKey struct {
	Type        string `json:"type"`
	ProjectID   string `json:"project_id"`
	PrivateKey  string `json:"private_key"`
	ClientEmail string `json:"client_email"`
	TokenURI    string `json:"token_uri"`
}

// TokenSource provides cached access tokens for a Google service account.
type TokenSource struct {
	mu         sync.Mutex
	key        ServiceAccountKey
	parsedKey  *rsa.PrivateKey
	httpClient *http.Client
	token      string
	expiresAt  time.Time
}

// NewTokenSource returns a TokenSource from service account JSON bytes.
func NewTokenSource(saJSON []byte, client *http.Client) (*TokenSource, error) {
	var key ServiceAccountKey
	if err := json.Unmarshal(saJSON, &key); err != nil {
		return nil, fmt.Errorf("parse service account JSON: %w", err)
	}
	if key.ClientEmail == "" || key.PrivateKey == "" {
		return nil, ErrInvalidServiceAccount
	}
	if key.TokenURI == "" {
		key.TokenURI = "https://oauth2.googleapis.com/token"
	}

	parsedKey, err := parsePrivateKey(key.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("parse RSA private key: %w", err)
	}

	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}

	return &TokenSource{
		key:        key,
		parsedKey:  parsedKey,
		httpClient: client,
	}, nil
}

// Token returns a valid OAuth2 access token, refreshing it if needed.
func (ts *TokenSource) Token(ctx context.Context) (string, error) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	// 2-minute margin
	if ts.token != "" && time.Now().Add(2*time.Minute).Before(ts.expiresAt) {
		return ts.token, nil
	}

	token, expiresAt, err := ts.fetchToken(ctx)
	if err != nil {
		return "", err
	}
	ts.token = token
	ts.expiresAt = expiresAt
	return token, nil
}

// ProjectID returns the project_id declared in the service account.
func (ts *TokenSource) ProjectID() string {
	return ts.key.ProjectID
}

func (ts *TokenSource) fetchToken(ctx context.Context) (string, time.Time, error) {
	now := time.Now().UTC()
	assertion, err := ts.jwtAssertion(now)
	if err != nil {
		return "", time.Time{}, err
	}
	body, status, err := ts.postTokenAssertion(ctx, assertion)
	if err != nil {
		return "", time.Time{}, err
	}
	if status < 200 || status >= 300 {
		return "", time.Time{}, fmt.Errorf("%w %d: %s", ErrTokenStatus, status, string(body))
	}
	return parseTokenGrant(body, now)
}

func (ts *TokenSource) jwtAssertion(now time.Time) (string, error) {
	exp := now.Add(time.Hour)

	headerJSON, _ := json.Marshal(map[string]string{
		"alg": "RS256",
		"typ": "JWT",
	})
	payloadJSON, _ := json.Marshal(map[string]any{
		"iss":   ts.key.ClientEmail,
		"scope": "https://www.googleapis.com/auth/cloud-platform",
		"aud":   ts.key.TokenURI,
		"exp":   exp.Unix(),
		"iat":   now.Unix(),
	})

	signingInput := base64RawURL(headerJSON) + "." + base64RawURL(payloadJSON)
	hashed := sha256.Sum256([]byte(signingInput))

	sig, err := rsa.SignPKCS1v15(rand.Reader, ts.parsedKey, crypto.SHA256, hashed[:])
	if err != nil {
		return "", fmt.Errorf("sign JWT assertion: %w", err)
	}
	return signingInput + "." + base64RawURL(sig), nil
}

func (ts *TokenSource) postTokenAssertion(ctx context.Context, assertion string) ([]byte, int, error) {
	data := url.Values{
		"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"},
		"assertion":  {assertion},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ts.key.TokenURI, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := ts.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("request token from %s: %w", ts.key.TokenURI, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, 0, fmt.Errorf("read token response: %w", err)
	}
	return body, resp.StatusCode, nil
}

func parseTokenGrant(body []byte, now time.Time) (string, time.Time, error) {
	var res struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
		TokenType   string `json:"token_type"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return "", time.Time{}, fmt.Errorf("decode token response: %w", err)
	}

	expiresIn := res.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 3600
	}

	return res.AccessToken, now.Add(time.Duration(expiresIn) * time.Second), nil
}

func base64RawURL(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

func parsePrivateKey(pemStr string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, ErrUndecodablePEM
	}

	// Try PKCS8 then PKCS1
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if rsaKey, ok := key.(*rsa.PrivateKey); ok {
			return rsaKey, nil
		}
		return nil, ErrNonRSAKey
	}

	return x509.ParsePKCS1PrivateKey(block.Bytes)
}
