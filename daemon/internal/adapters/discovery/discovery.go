// Model lister: fetching a provider's published roster.
package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/catalog"
)

// Listing errors name the refusals callers map to their own wording: a
// vendor with no list to read, a key the vendor rejected, and a list that
// answered with failure.
var (
	ErrUnsupported  = catalog.ErrListUnsupported
	ErrListRejected = catalog.ErrListRejected
	ErrListStatus   = errors.New("the model list answered")
)

const (
	requestTimeout = 30 * time.Second
	maxBodyBytes   = 8 << 20
	maxModels      = 5000
	pageSize       = 1000
	// refusalBodyLimit bounds how much of a refusal body is read to name the
	// vendor's reason, so a hostile answer cannot be read in full.
	refusalBodyLimit = 64 << 10
	antigravityPath  = "/v1internal:fetchAvailableModels"
)

// Listed is one model a listing returned.
type Listed = catalog.Listed

// Target names the endpoint and credential one listing runs against.
type Target = catalog.ListTarget

// Lister lists the models one provider publishes.
type Lister struct {
	client *http.Client
}

// New returns a lister that reads listings over the given client.
func New(client *http.Client) *Lister {
	if client == nil {
		client = &http.Client{Timeout: requestTimeout}
	}
	return &Lister{client: client}
}

// List returns the models one provider publishes, in the dialect it declares.
func (l *Lister) List(ctx context.Context, target Target) ([]Listed, error) {
	switch target.Format {
	case catalog.ModelsOpenAI:
		return l.listOpenAI(ctx, target)
	case catalog.ModelsAnthropic:
		return l.listAnthropic(ctx, target)
	case catalog.ModelsGemini:
		return l.listGemini(ctx, target)
	case catalog.ModelsAntigravity:
		return l.listAntigravity(ctx, target)
	case catalog.ModelsBedrock:
		return l.listBedrock(ctx, target)
	case catalog.ModelsKiro:
		return l.listKiro(ctx, target)
	case catalog.ModelsCodex:
		return l.listCodex(ctx, target)
	default:
		return nil, fmt.Errorf("%s: %w", target.Format, ErrUnsupported)
	}
}

func (l *Lister) get(ctx context.Context, target Target, path string) ([]byte, error) {
	request, err := listRequest(ctx, target, path)
	if err != nil {
		return nil, err
	}

	response, err := l.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("list the models of %s: %w", target.BaseURL, err)
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBodyBytes))
	if err != nil {
		return nil, fmt.Errorf("read the model list: %w", err)
	}
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("%d: %w", response.StatusCode, ErrListRejected)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("%w %d", ErrListStatus, response.StatusCode)
	}
	return body, nil
}

func listRequest(ctx context.Context, target Target, path string) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet,
		strings.TrimSuffix(target.BaseURL, "/")+path, nil)
	if err != nil {
		return nil, fmt.Errorf("build the model list request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	applyHeaders(request, target.Auth.Headers)
	applyHeaders(request, target.Headers)
	applyListCredential(request, target)

	if target.Auth.Signer != nil {
		_ = target.Auth.Signer(request)
	}
	if target.OpenCodeFree {
		free, err := wire.OpenCodeFreeHeaders("listing", "")
		if err != nil {
			return nil, err
		}
		applyHeaders(request, free)
	}
	return request, nil
}

func applyListCredential(request *http.Request, target Target) {
	if target.Auth.KeyHeader != "" && target.Auth.Token != "" {
		wire.ApplyCredential(request, wire.CodecOpts{
			CredentialRef: target.Auth.Token,
			KeyHeader:     string(target.Auth.KeyHeader),
		})
	} else if target.Auth.Token != "" && request.Header.Get("Authorization") == "" {
		request.Header.Set("Authorization", "Bearer "+target.Auth.Token)
	}
}

func applyHeaders(request *http.Request, headers map[string]string) {
	for name, value := range headers {
		if value != "" {
			request.Header.Set(name, value)
		}
	}
}

func decode(body []byte, target any) error {
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode the model list: %w", err)
	}
	return nil
}

func truncate(models []Listed) []Listed {
	if len(models) > maxModels {
		return models[:maxModels]
	}
	return models
}

func withQuery(base string, values url.Values) string {
	if len(values) == 0 {
		return base
	}
	return base + "?" + values.Encode()
}
