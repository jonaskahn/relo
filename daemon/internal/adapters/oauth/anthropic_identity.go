// Claude bootstrap identity: the account and workspace a token belongs to.
package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	claudeBootstrapEndpoint = "https://api.anthropic.com/api/claude_cli/bootstrap"
	claudeBootstrapModel    = "claude-opus-4-6"
	claudeIdentityTimeout   = 30 * time.Second
	claudeIdentityBodyLimit = 64 << 10
)

// ClaudeIdentity is the account and subscription workspace one Claude token
// draws its limits from. One login email can hold several workspaces.
type ClaudeIdentity struct {
	AccountID string
	Email     string
	OrgID     string
	OrgName   string
}

// ClaudeIdentitySource reads the identity Claude's CLI bootstrap reports
// for a token. The zero value reads the vendor endpoint with a bounded
// client, which is what login and credential backfill use.
type ClaudeIdentitySource struct {
	Endpoint   string
	HTTPClient *http.Client
}

type claudeBootstrapAccount struct {
	AccountUUID      string `json:"account_uuid"`
	AccountEmail     string `json:"account_email"`
	OrganizationUUID string `json:"organization_uuid"`
	OrganizationName string `json:"organization_name"`
}

// Fetch returns the identity the bootstrap endpoint reports for a token. A
// refusal or an unreadable answer is an error the caller degrades from.
func (s ClaudeIdentitySource) Fetch(ctx context.Context, accessToken string) (ClaudeIdentity, error) {
	body, err := s.get(ctx, accessToken)
	if err != nil {
		return ClaudeIdentity{}, err
	}
	return decodeClaudeIdentity(body)
}

func (s ClaudeIdentitySource) get(ctx context.Context, accessToken string) ([]byte, error) {
	endpoint := s.Endpoint
	if endpoint == "" {
		endpoint = claudeBootstrapEndpoint
	}
	client := s.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: claudeIdentityTimeout}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?entrypoint=cli&model="+claudeBootstrapModel, nil)
	if err != nil {
		return nil, fmt.Errorf("build the claude identity request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", defaultUserAgent)
	request.Header.Set("anthropic-beta", "oauth-2025-04-20")
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("read the claude identity: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	return readClaudeIdentityBody(response)
}

func readClaudeIdentityBody(response *http.Response) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(response.Body, claudeIdentityBodyLimit))
	if err != nil {
		return nil, fmt.Errorf("read the claude identity: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("claude identity answered %d", response.StatusCode)
	}
	return body, nil
}

func decodeClaudeIdentity(body []byte) (ClaudeIdentity, error) {
	var envelope struct {
		Account *claudeBootstrapAccount `json:"oauth_account"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return ClaudeIdentity{}, fmt.Errorf("decode the claude identity: %w", err)
	}
	if envelope.Account == nil {
		return ClaudeIdentity{}, fmt.Errorf("decode the claude identity: %w", ErrTokenResponse)
	}
	account := envelope.Account
	return ClaudeIdentity{
		AccountID: account.AccountUUID,
		Email:     account.AccountEmail,
		OrgID:     account.OrganizationUUID,
		OrgName:   account.OrganizationName,
	}, nil
}

// FillClaudeIdentity records a fetched identity on a credential without
// overwriting values a token response already provided.
func (c *OAuthCredential) FillClaudeIdentity(identity ClaudeIdentity) {
	if c == nil {
		return
	}
	c.AccountID = firstNonEmpty(c.AccountID, identity.AccountID)
	c.Email = firstNonEmpty(c.Email, identity.Email)
	if c.Extra[orgExtraKey] == "" {
		addExtra(c, orgExtraKey, identity.OrgID)
	}
	if c.Extra[orgNameExtraKey] == "" {
		addExtra(c, orgNameExtraKey, identity.OrgName)
	}
}

// claudeIdentityHook fills identity the token response left unset. A failed
// lookup never fails the login: the credential stays usable and a later
// request backfills what is missing.
func claudeIdentityHook(source ClaudeIdentitySource) func(context.Context, *OAuthCredential) error {
	return func(ctx context.Context, credential *OAuthCredential) error {
		if credential == nil || credential.AccountID != "" {
			return nil
		}
		identity, err := source.Fetch(ctx, credential.AccessToken)
		if err != nil {
			return nil
		}
		credential.FillClaudeIdentity(identity)
		return nil
	}
}
