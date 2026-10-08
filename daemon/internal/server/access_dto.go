// Access DTOs: the client-key shapes the management API serves.
package server

import (
	appaccess "github.com/jonaskahn/relo/internal/application/access"
)

type accessKeyResponse struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Kind         string `json:"kind"`
	Client       string `json:"client,omitempty"`
	Hint         string `json:"token_hint"`
	Generation   int    `json:"generation"`
	Status       string `json:"status"`
	ExpiresAtMs  int64  `json:"expires_at_ms,omitempty"`
	CreatedAtMs  int64  `json:"created_at_ms"`
	UpdatedAtMs  int64  `json:"updated_at_ms,omitempty"`
	LastUsedAtMs int64  `json:"last_used_at_ms,omitempty"`
	Owner        string `json:"owner,omitempty"`
}

func toAccessKeyResponse(key appaccess.AccessKey) accessKeyResponse {
	return accessKeyResponse{
		ID: key.ID, Name: key.Name, Kind: key.Kind, Client: key.Client,
		Hint: key.Hint, Generation: key.Generation, Status: key.Status,
		ExpiresAtMs: key.ExpiresAtMs, CreatedAtMs: key.CreatedAtMs,
		UpdatedAtMs: key.UpdatedAtMs, LastUsedAtMs: key.LastUsedAtMs,
		Owner: key.Owner,
	}
}

func toAccessKeyList(keys []appaccess.AccessKey) []accessKeyResponse {
	listed := make([]accessKeyResponse, 0, len(keys))
	for _, key := range keys {
		listed = append(listed, toAccessKeyResponse(key))
	}
	return listed
}
