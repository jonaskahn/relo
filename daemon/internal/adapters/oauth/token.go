// Vendor token shapes: decoding success and error responses.
package oauth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jonaskahn/relo/internal/clock"
)

const (
	defaultTokenLifetime = time.Hour
	defaultRetries       = 2
	retryBaseDelay       = 500 * time.Millisecond
	maxRetryDelay        = 8 * time.Second
	maxRetryAfter        = 60 * time.Second
	maxRetryJitter       = 2 * time.Second
	tokenBodyLimit       = 1 << 20
)

type tokenResponse struct {
	AccessToken      string     `json:"access_token"`
	RefreshToken     string     `json:"refresh_token"`
	IDToken          string     `json:"id_token"`
	TokenType        string     `json:"token_type"`
	Scope            string     `json:"scope"`
	ExpiresIn        seconds    `json:"expires_in"`
	Error            tokenError `json:"error"`
	ErrorDescription string     `json:"error_description"`

	body []byte
}

type tokenError struct {
	Code        string
	Description string
}

// UnmarshalJSON reads the string and the object form of the error member.
func (e *tokenError) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil
	}
	if trimmed[0] == '"' {
		return json.Unmarshal(trimmed, &e.Code)
	}
	var object struct {
		Code        string `json:"code"`
		Type        string `json:"type"`
		Message     string `json:"message"`
		Description string `json:"error_description"`
	}
	if err := json.Unmarshal(trimmed, &object); err != nil {
		return fmt.Errorf("decode the token error: %w", err)
	}
	e.Code = firstNonEmpty(object.Code, object.Type)
	e.Description = firstNonEmpty(object.Description, object.Message)
	return nil
}

type seconds float64

// UnmarshalJSON accepts both numeric and quoted second counts.
func (s *seconds) UnmarshalJSON(data []byte) error {
	text := strings.Trim(string(data), "\"")
	if text == "" || text == "null" {
		return nil
	}
	value, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return fmt.Errorf("parse seconds: %w", err)
	}
	*s = seconds(value)
	return nil
}

type responseError struct {
	Status      int
	Code        string
	Description string
}

// Error describes the refusal without echoing the response body.
func (e responseError) Error() string {
	if e.Code == "" {
		return fmt.Sprintf("token endpoint answered %d", e.Status)
	}
	if e.Description == "" {
		return fmt.Sprintf("token endpoint answered %d (%s)", e.Status, e.Code)
	}
	return fmt.Sprintf("token endpoint answered %d (%s: %s)", e.Status, e.Code, e.Description)
}

type tokenClient struct {
	http    *http.Client
	clock   clock.Clock
	headers map[string]string
	retries int
}

func (c tokenClient) attempts() int {
	if c.retries <= 0 {
		return defaultRetries
	}
	return c.retries
}

func (c tokenClient) postForm(ctx context.Context, endpoint string, form url.Values) (tokenResponse, error) {
	request, err := formRequest(ctx, endpoint, form, c.headers)
	if err != nil {
		return tokenResponse{}, err
	}
	return c.decode(request)
}

func (c tokenClient) postJSON(ctx context.Context, endpoint string, payload any, headers map[string]string) (tokenResponse, error) {
	request, err := jsonRequest(ctx, endpoint, payload, mergeHeaders(c.headers, headers))
	if err != nil {
		return tokenResponse{}, err
	}
	return c.decode(request)
}

func (c tokenClient) postJSONRaw(ctx context.Context, endpoint string, payload any, headers map[string]string) ([]byte, int, error) {
	request, err := jsonRequest(ctx, endpoint, payload, mergeHeaders(c.headers, headers))
	if err != nil {
		return nil, 0, err
	}
	return c.fetch(request)
}

func (c tokenClient) postFormRaw(ctx context.Context, endpoint string, form url.Values, headers map[string]string) ([]byte, int, error) {
	request, err := formRequest(ctx, endpoint, form, mergeHeaders(c.headers, headers))
	if err != nil {
		return nil, 0, err
	}
	return c.fetch(request)
}

func (c tokenClient) get(ctx context.Context, endpoint string, headers map[string]string) ([]byte, int, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("build request for %s: %w", endpoint, err)
	}
	request.Header.Set("Accept", "application/json")
	for name, value := range mergeHeaders(c.headers, headers) {
		request.Header.Set(name, value)
	}
	return c.fetch(request)
}

func (c tokenClient) decode(request *http.Request) (tokenResponse, error) {
	body, status, err := c.fetch(request)
	if err != nil {
		return tokenResponse{}, err
	}
	response, parseErr := parseTokenResponse(body)
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return response, responseError{
			Status:      status,
			Code:        response.Error.Code,
			Description: firstNonEmpty(response.Error.Description, response.ErrorDescription),
		}
	}
	if parseErr != nil {
		return tokenResponse{}, parseErr
	}
	// A refusal arrives on a 2xx status from providers that answer the token
	// endpoint with GitHub's shape, where the error member is the whole
	// answer. Reading it as a token response would hand a poll an answer it
	// cannot use, so the error member decides first.
	if response.Error.Code != "" {
		return response, responseError{
			Status:      status,
			Code:        response.Error.Code,
			Description: firstNonEmpty(response.Error.Description, response.ErrorDescription),
		}
	}
	return response, nil
}

func (c tokenClient) fetch(request *http.Request) ([]byte, int, error) {
	for attempt := 0; ; attempt++ {
		response, err := c.http.Do(replay(request))
		if err != nil {
			if attempt >= c.attempts() {
				return nil, 0, fmt.Errorf("call %s: %w", request.URL.Host, err)
			}
			if err := c.wait(request.Context(), retryDelay(attempt, 0)); err != nil {
				return nil, 0, err
			}
			continue
		}
		body, readErr := readBounded(response.Body)
		if readErr != nil {
			return nil, response.StatusCode, readErr
		}
		if !retryableStatus(response.StatusCode) || attempt >= c.attempts() {
			return body, response.StatusCode, nil
		}
		if err := c.wait(request.Context(), retryDelay(attempt, retryAfter(response.Header))); err != nil {
			return nil, response.StatusCode, err
		}
	}
}

func (c tokenClient) wait(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.clock.After(delay):
		return nil
	}
}

func replay(request *http.Request) *http.Request {
	if request.Body == nil || request.GetBody == nil {
		return request
	}
	body, err := request.GetBody()
	if err != nil {
		return request
	}
	clone := request.Clone(request.Context())
	clone.Body = body
	return clone
}

func readBounded(body io.ReadCloser) ([]byte, error) {
	defer func() { _ = body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(body, tokenBodyLimit))
	if err != nil {
		return nil, fmt.Errorf("read the response body: %w", err)
	}
	return raw, nil
}

func parseTokenResponse(body []byte) (tokenResponse, error) {
	response := tokenResponse{}
	if len(bytes.TrimSpace(body)) == 0 {
		return response, nil
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return tokenResponse{}, fmt.Errorf("decode the token response: %w", err)
	}
	response.body = body
	return response, nil
}

func (r tokenResponse) decodeInto(target any) error {
	if err := json.Unmarshal(r.body, target); err != nil {
		return fmt.Errorf("decode the token response details: %w", err)
	}
	return nil
}

func formRequest(ctx context.Context, endpoint string, form url.Values, headers map[string]string) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("build token request: %w", err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	return request, nil
}

func jsonRequest(ctx context.Context, endpoint string, payload any, headers map[string]string) (*http.Request, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode request body: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request for %s: %w", endpoint, err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	return request, nil
}

func mergeHeaders(base, extra map[string]string) map[string]string {
	if len(base) == 0 {
		return extra
	}
	merged := make(map[string]string, len(base)+len(extra))
	for name, value := range base {
		merged[name] = value
	}
	for name, value := range extra {
		merged[name] = value
	}
	return merged
}

func retryableStatus(status int) bool {
	return status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
}

func retryDelay(attempt int, wait time.Duration) time.Duration {
	if wait > 0 {
		return min(wait, maxRetryAfter) + jitter()
	}
	return min(retryBaseDelay<<attempt, maxRetryDelay) + jitter()
}

func jitter() time.Duration {
	return time.Duration(rand.Int64N(int64(maxRetryJitter)))
}

func retryAfter(header http.Header) time.Duration {
	raw := strings.TrimSpace(header.Get("Retry-After"))
	if raw == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(raw); err == nil {
		return max(time.Duration(seconds)*time.Second, 0)
	}
	if when, err := http.ParseTime(raw); err == nil {
		return max(time.Until(when), 0)
	}
	return 0
}

func (r tokenResponse) lifetime() time.Duration {
	if r.ExpiresIn <= 0 {
		return defaultTokenLifetime
	}
	return time.Duration(float64(r.ExpiresIn) * float64(time.Second))
}

func (r tokenResponse) credential(now time.Time, scopes []string) (OAuthCredential, error) {
	if r.AccessToken == "" {
		return OAuthCredential{}, ErrTokenResponse
	}
	credential := OAuthCredential{
		AccessToken:  r.AccessToken,
		RefreshToken: r.RefreshToken,
		ExpiresAt:    now.Add(r.lifetime()),
		Scope:        firstNonEmpty(r.Scope, strings.Join(scopes, " ")),
	}
	credential.AccountID, credential.Email = IdentityFromTokens(r.IDToken, r.AccessToken)
	return credential, nil
}

type refreshOptions struct {
	client   tokenClient
	endpoint string
	clientID string
	now      time.Time
	cred     *OAuthCredential
	scopes   []string
}

func refreshToken(ctx context.Context, opts refreshOptions) (*OAuthCredential, error) {
	cred := opts.cred
	if cred == nil || cred.RefreshToken == "" {
		return nil, ErrNoRefreshToken
	}
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {opts.clientID},
		"refresh_token": {cred.RefreshToken},
	}
	response, err := opts.client.postForm(ctx, opts.endpoint, form)
	if err != nil {
		return nil, fmt.Errorf("refresh the access token: %w", refreshFailure(err))
	}
	refreshed, err := response.credential(opts.now, opts.scopes)
	if err != nil {
		return nil, err
	}
	merged := mergeCredential(*cred, refreshed)
	return &merged, nil
}

func refreshFailure(err error) error {
	var refusal responseError
	if errors.As(err, &refusal) && definitiveRefusal(refusal.Code, refusal.Description) {
		return fmt.Errorf("%s: %w", firstNonEmpty(refusal.Code, refusal.Description), ErrRefreshRejected)
	}
	return err
}

func definitiveRefusal(code, description string) bool {
	combined := strings.ToLower(code + " " + description)
	for _, signal := range []string{"invalid_grant", "invalid_token", "unauthorized_client", "revoked"} {
		if strings.Contains(combined, signal) {
			return true
		}
	}
	return strings.Contains(combined, "refresh") && strings.Contains(combined, "expir")
}

func mergeCredential(previous, refreshed OAuthCredential) OAuthCredential {
	merged := refreshed.Clone()
	merged.RefreshToken = firstNonEmpty(refreshed.RefreshToken, previous.RefreshToken)
	merged.AccountID = firstNonEmpty(refreshed.AccountID, previous.AccountID)
	merged.Email = firstNonEmpty(refreshed.Email, previous.Email)
	merged.Scope = firstNonEmpty(refreshed.Scope, previous.Scope)
	if len(previous.Extra) == 0 {
		return merged
	}
	extra := previous.Clone().Extra
	for key, value := range merged.Extra {
		extra[key] = value
	}
	merged.Extra = extra
	return merged
}

func addExtra(credential *OAuthCredential, key, value string) {
	if value == "" {
		return
	}
	if credential.Extra == nil {
		credential.Extra = map[string]string{}
	}
	credential.Extra[key] = value
}
