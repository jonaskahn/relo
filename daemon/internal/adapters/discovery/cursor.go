// Cursor model listing over the agent RPCs.
package discovery

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/jonaskahn/relo/internal/adapters/cursor"
	"github.com/jonaskahn/relo/internal/catalog"
)

// Cursor client version the listing announces. The service gates protocol
// features on it, so it stays pinned to the verified reference behavior.
const cursorClientVersion = "cli-2026.09.02-c22c1a3"

func (l *Lister) listCursor(ctx context.Context, target Target) ([]Listed, error) {
	token := target.Auth.Token
	if token == "" {
		return nil, fmt.Errorf("cursor: %w", ErrListRejected)
	}
	base := strings.TrimSuffix(target.BaseURL, "/")
	if base == "" {
		base = cursor.DefaultBaseURL
	}
	usableBody, usableStatus := l.postCursor(ctx, base, cursor.GetUsableModelsPath, cursor.BuildUsableModelsRequest(), token, target)
	availableBody, availableStatus := l.postCursor(ctx, base, cursor.AvailableModelsPath, cursor.BuildAvailableModelsRequest(), token, target)
	if usableStatus.rejected || availableStatus.rejected {
		return nil, fmt.Errorf("cursor: %w", ErrListRejected)
	}
	if !usableStatus.ok && !availableStatus.ok {
		return nil, fmt.Errorf("cursor: %w", ErrListStatus)
	}
	return cursorListed(usableBody, usableStatus.ok, availableBody, availableStatus.ok)
}

type cursorStatus struct {
	ok       bool
	rejected bool
}

func (l *Lister) postCursor(ctx context.Context, base, path string, payload []byte, token string, target Target) ([]byte, cursorStatus) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, base+path, bytes.NewReader(payload))
	if err != nil {
		return nil, cursorStatus{}
	}
	for name, value := range cursor.Headers(token, cursorClientVersion, cursorRequestID()) {
		request.Header.Set(name, value)
	}
	applyHeaders(request, target.Auth.Headers)
	applyHeaders(request, target.Headers)
	applyListCredential(request, target)
	response, err := l.client.Do(request)
	if err != nil {
		return nil, cursorStatus{}
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBodyBytes))
	if err != nil {
		return nil, cursorStatus{}
	}
	switch {
	case response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden:
		return body, cursorStatus{rejected: true}
	case response.StatusCode < 200 || response.StatusCode >= 300:
		return body, cursorStatus{}
	default:
		return body, cursorStatus{ok: true}
	}
}

func cursorRequestID() string {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "relo-listing"
	}
	raw[6] = raw[6]&0x0f | 0x40
	raw[8] = raw[8]&0x3f | 0x80
	return hex.EncodeToString(raw[:4]) + "-" + hex.EncodeToString(raw[4:6]) + "-" +
		hex.EncodeToString(raw[6:8]) + "-" + hex.EncodeToString(raw[8:10]) + "-" + hex.EncodeToString(raw[10:])
}

func cursorListed(usableBody []byte, usableOK bool, availableBody []byte, availableOK bool) ([]Listed, error) {
	var usable []cursor.UsableModel
	if usableOK {
		parsed, err := cursor.ParseUsableModels(usableBody)
		if err != nil {
			return nil, fmt.Errorf("decode the cursor model list: %w", err)
		}
		usable = parsed
	}
	var rich []cursor.RichModel
	if availableOK {
		parsed, err := cursor.ParseAvailableModels(availableBody)
		if err != nil {
			return nil, fmt.Errorf("decode the cursor model list: %w", err)
		}
		rich = parsed
	}
	if availableOK && len(rich) > 0 {
		return truncate(cursor.GroupsToListed(cursor.GroupVariants(cursor.JoinModels(usable, usableOK, rich)))), nil
	}
	return truncate(usableListed(usable)), nil
}

func usableListed(usable []cursor.UsableModel) []catalog.Listed {
	listed := make([]catalog.Listed, 0, len(usable))
	for _, model := range usable {
		entry := catalog.Listed{ID: model.ID, Name: model.Name}
		if model.Reasoning {
			stated := true
			entry.SupportsReasoning = &stated
		}
		listed = append(listed, entry)
	}
	return listed
}
