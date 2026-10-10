// Google Antigravity sign-in flow and its project hook.
package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/jonaskahn/relo/internal/adapters/antigravity"
	"github.com/jonaskahn/relo/internal/clock"
)

const (
	providerGoogleAntigravity = FlowGoogleAntigravity
	antigravityClientID       = "1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com"
	antigravityClientSecret   = "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"
	antigravityAuthURL        = "https://accounts.google.com/o/oauth2/v2/auth"
	antigravityTokenURL       = "https://oauth2.googleapis.com/token"
	antigravityProdAPI        = "https://cloudcode-pa.googleapis.com"
	antigravityDailyAPI       = "https://daily-cloudcode-pa.googleapis.com"
	antigravityAPIVersion     = "v1internal"
	antigravityPort           = 51121
	antigravityPath           = "/oauth-callback"
	// antigravityAccept is the accept the IDE sends on these calls, kept
	// because the header set is the fingerprint the backend reads.
	antigravityAccept = "*/*"
)

// Antigravity errors name why its project hook failed: the Cloud project
// that could not be onboarded.
var (
	antigravityScopes = []string{
		"https://www.googleapis.com/auth/cloud-platform",
		"https://www.googleapis.com/auth/userinfo.email",
		"https://www.googleapis.com/auth/userinfo.profile",
		"https://www.googleapis.com/auth/cclog",
		"https://www.googleapis.com/auth/experimentsandconfigs",
	}
	ErrOnboardingFailed = errors.New("the Cloud Code Assist project could not be onboarded")
)

// NewGoogleAntigravityFlow returns the browser PKCE login for a Google
// Antigravity account, including the Cloud Code Assist project the
// requests are billed to.
func NewGoogleAntigravityFlow(options ...Option) OAuthFlow {
	settings := newOptions(options...)
	client := settings.tokenClient(nil)
	return newAuthCodeFlow(authCodeConfig{
		providerID:   providerGoogleAntigravity,
		clientID:     antigravityClientID,
		clientSecret: antigravityClientSecret,
		authURL:      option(settings.Endpoints.AuthURL, antigravityAuthURL),
		tokenURL:     option(settings.Endpoints.TokenURL, antigravityTokenURL),
		scopes:       antigravityScopes,
		port:         antigravityPort,
		path:         antigravityPath,
		redirectHost: "127.0.0.1",
		authParams:   map[string]string{"access_type": "offline", "prompt": "consent"},
		afterLogin:   antigravityProjectHook(settings, client),
		exchange:     antigravityExchange,
		refresh: func(ctx context.Context, client tokenClient, request refreshRequest) (*OAuthCredential, error) {
			return antigravityRefresh(ctx, settings, client, request)
		},
	}, settings)
}

func antigravityProjectHook(settings Options, client tokenClient) func(context.Context, *OAuthCredential) error {
	return func(ctx context.Context, credential *OAuthCredential) error {
		project, err := discoverAntigravityProject(ctx, settings, client, credential.AccessToken)
		if err != nil {
			return err
		}
		if project == "" {
			return ErrOnboardingFailed
		}
		addExtra(credential, projectExtraKey, project)
		return nil
	}
}

func antigravityExchange(ctx context.Context, client tokenClient, request exchangeRequest) (tokenResponse, error) {
	response, err := client.postForm(ctx, request.TokenURL, antigravityTokenForm(request))
	if err != nil {
		return tokenResponse{}, err
	}
	// A Google authorization that asked for offline access returns a refresh
	// token. Storing a credential without one would read as connected and stop
	// working an hour later.
	if response.RefreshToken == "" {
		return tokenResponse{}, ErrNoRefreshToken
	}
	return response, nil
}

func antigravityTokenForm(request exchangeRequest) url.Values {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {request.ClientID},
		"client_secret": {request.ClientSecret},
		"code":          {request.Code},
		"code_verifier": {request.Verifier},
		"redirect_uri":  {request.RedirectURI},
	}
	return form
}

type antigravityAnswer struct {
	stage  string
	status int
	code   string
	detail string
}

// Error describes the answer and carries nothing about the credential.
func (a antigravityAnswer) Error() string {
	description := fmt.Sprintf("%s answered %d", a.stage, a.status)
	if a.code != "" {
		description += " (" + a.code + ")"
	}
	if a.detail != "" {
		description += ": " + a.detail
	}
	return description
}

func antigravityHeaders(accessToken string) map[string]string {
	return map[string]string{
		"Authorization": "Bearer " + accessToken,
		"User-Agent":    antigravity.UserAgent(),
		"Accept":        antigravityAccept,
	}
}

type antigravityLookup struct {
	project string
	answer  error
}

func discoverAntigravityProject(ctx context.Context, settings Options, client tokenClient, accessToken string) (string, error) {
	lookup, err := loadCodeAssistProject(ctx, settings, client, accessToken)
	if err != nil {
		return "", err
	}
	if lookup.project != "" {
		return lookup.project, nil
	}
	project, err := onboardAntigravityProject(ctx, settings, client, accessToken)
	if err != nil {
		return "", withLookupAnswer(err, lookup.answer)
	}
	return project, nil
}

func withLookupAnswer(err, answer error) error {
	if answer == nil {
		return err
	}
	return fmt.Errorf("%w (after %v)", err, answer)
}

func loadCodeAssistProject(ctx context.Context, settings Options, client tokenClient, accessToken string) (antigravityLookup, error) {
	endpoint := option(settings.Endpoints.APIBaseURL, antigravityProdAPI) + "/" + antigravityAPIVersion + ":loadCodeAssist"
	payload := map[string]any{"metadata": map[string]string{"ideType": "ANTIGRAVITY"}}
	body, status, err := client.postJSONRaw(ctx, endpoint, payload, antigravityHeaders(accessToken))
	if err != nil {
		return antigravityLookup{}, fmt.Errorf("load the Cloud Code Assist project: %w", err)
	}
	if status < 200 || status >= 300 {
		code, detail := antigravity.Refusal(body)
		return antigravityLookup{answer: antigravityAnswer{stage: "loadCodeAssist", status: status, code: code, detail: detail}}, nil
	}
	if project := projectIDFrom(body); project != "" {
		return antigravityLookup{project: project}, nil
	}
	return antigravityLookup{answer: antigravityAnswer{stage: "loadCodeAssist", status: status, detail: "the answer named no project"}}, nil
}

// The onboarding is a long-running operation: the call names an operation and
// the account is only ready once that operation reports done. The wait is a
// second a step, bounded, so a sign-in cannot hang on an operation the
// endpoint never finishes.
const (
	antigravityOnboardPoll   = time.Second
	antigravityOnboardTries  = 30
	antigravityOperationPath = "/" + antigravityAPIVersion + "/operations/"
)

func onboardAntigravityProject(ctx context.Context, settings Options, client tokenClient, accessToken string) (string, error) {
	operation, err := startAntigravityOnboarding(ctx, settings, client, accessToken)
	if err != nil {
		return "", err
	}
	return awaitAntigravityOnboarding(ctx, settings, client, operation)
}

func startAntigravityOnboarding(ctx context.Context, settings Options, client tokenClient, accessToken string) (operationPoll, error) {
	endpoint := option(settings.Endpoints.DailyAPIBaseURL, antigravityDailyAPI) + "/" + antigravityAPIVersion + ":onboardUser"
	body, status, err := client.postJSONRaw(ctx, endpoint, onboardPayload(), antigravityHeaders(accessToken))
	if err != nil {
		return operationPoll{}, fmt.Errorf("onboard the Cloud Code Assist project: %w", err)
	}
	if status < 200 || status >= 300 {
		return operationPoll{}, fmt.Errorf("%w: %v", ErrOnboardingFailed, antigravityOnboardAnswer(body, status))
	}
	if project, done := onboardOperation(body); done {
		// An operation that carries its project is already finished, so it
		// needs no polling at all.
		return operationPoll{project: project, token: accessToken}, nil
	}
	name := operationName(body)
	if name == "" {
		return operationPoll{}, fmt.Errorf("%w: %v", ErrOnboardingFailed, antigravityOperationAnswer(body, status))
	}
	return operationPoll{name: name, token: accessToken}, nil
}

// operationPoll is the started onboarding a sign-in waits on: either the
// project it produced outright, or the operation to ask until it does.
type operationPoll struct {
	name    string
	project string
	token   string
}

func (o operationPoll) endpoint(settings Options) string {
	base := option(settings.Endpoints.DailyAPIBaseURL, antigravityDailyAPI) + antigravityOperationPath
	return base + strings.TrimPrefix(o.name, "operations/")
}

func antigravityOnboardAnswer(body []byte, status int) antigravityAnswer {
	code, detail := antigravity.Refusal(body)
	return antigravityAnswer{stage: "onboardUser", status: status, code: code, detail: detail}
}

func awaitAntigravityOnboarding(ctx context.Context, settings Options, client tokenClient, operation operationPoll) (string, error) {
	if operation.project != "" {
		return operation.project, nil
	}
	var answer error
	for range antigravityOnboardTries {
		body, status, err := client.get(ctx, operation.endpoint(settings), antigravityHeaders(operation.token))
		if err != nil {
			return "", fmt.Errorf("read the onboarding operation: %w", err)
		}
		project, done := onboardOperation(body)
		if done {
			return project, nil
		}
		answer = antigravityOperationAnswer(body, status)
		if status < 200 || status >= 300 {
			return "", fmt.Errorf("%w: %v", ErrOnboardingFailed, answer)
		}
		if err := waitFor(ctx, client.clock, antigravityOnboardPoll); err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("%w: %v", ErrOnboardingFailed, answer)
}

// onboardOperation reads the operation an onboarding names: the project once it
// is finished, and whether that is so. Only a successful answer reaches it; an
// operation that finishes without a project is reported as finished with none,
// so the caller refuses it rather than polling for a project never coming.
func onboardOperation(body []byte) (project string, done bool) {
	operation := struct {
		Done     bool            `json:"done"`
		Response json.RawMessage `json:"response"`
	}{}
	if err := json.Unmarshal(body, &operation); err != nil {
		return "", true
	}
	if !operation.Done {
		return "", false
	}
	return projectIDFrom(operation.Response), true
}

// operationName reads the operation an onboarding started, empty when the
// answer refused or named none.
func operationName(body []byte) string {
	operation := struct {
		Name string `json:"name"`
	}{}
	if err := json.Unmarshal(body, &operation); err != nil {
		return ""
	}
	return operation.Name
}

func antigravityOperationAnswer(body []byte, status int) antigravityAnswer {
	if status < 200 || status >= 300 {
		code, detail := antigravity.Refusal(body)
		return antigravityAnswer{stage: "operations", status: status, code: code, detail: detail}
	}
	return antigravityAnswer{stage: "operations", status: status, detail: "the operation never reported done"}
}

func antigravityRefresh(ctx context.Context, settings Options, client tokenClient, request refreshRequest) (*OAuthCredential, error) {
	previous := request.Credential
	if previous.RefreshToken == "" {
		return nil, ErrNoRefreshToken
	}
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {request.ClientID},
		"client_secret": {antigravityClientSecret},
		"refresh_token": {previous.RefreshToken},
	}
	response, err := client.postForm(ctx, request.Endpoint, form)
	if err != nil {
		return nil, fmt.Errorf("refresh the access token: %w", refreshFailure(err))
	}
	refreshed, err := response.credential(client.clock.Now(), request.Scopes)
	if err != nil {
		return nil, err
	}
	merged := mergeCredential(previous, refreshed)
	if merged.Extra[projectExtraKey] != "" {
		return &merged, nil
	}
	return attachAntigravityProject(ctx, settings, client, merged)
}

func attachAntigravityProject(ctx context.Context, settings Options, client tokenClient, merged OAuthCredential) (*OAuthCredential, error) {
	// Best effort: a refreshed token stays usable without a project, and the
	// request path reports the missing project on its own. Failing the refresh
	// here would retire an account over a lookup the sign-in already covered.
	project, err := discoverAntigravityProject(ctx, settings, client, merged.AccessToken)
	if err != nil || project == "" {
		return &merged, nil
	}
	addExtra(&merged, projectExtraKey, project)
	return &merged, nil
}

func onboardPayload() map[string]any {
	return map[string]any{
		"tierId":   "free-tier",
		"metadata": map[string]string{"ideType": "ANTIGRAVITY"},
	}
}

func projectIDFrom(body []byte) string {
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(body, &fields); err != nil {
		return ""
	}
	for _, key := range []string{"cloudaicompanionProject", "projectId", "project"} {
		raw, found := fields[key]
		if !found {
			continue
		}
		if identifier := identifierFrom(raw); identifier != "" {
			return identifier
		}
	}
	return ""
}

func identifierFrom(raw json.RawMessage) string {
	text := ""
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	object := struct {
		ID string `json:"id"`
	}{}
	if err := json.Unmarshal(raw, &object); err == nil {
		return object.ID
	}
	return ""
}

func bearerHeader(accessToken string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + accessToken}
}

func waitFor(ctx context.Context, clk clock.Clock, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-clk.After(delay):
		return nil
	}
}
