// Admin token storage and the exemption routing unauthenticated paths take.
package server

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Admin-token storage is the file in the state directory the management
// guard reads the operator credential from.
const (
	AdminTokenFile = "admin-token"
	tokenBytes     = 32
	tokenFileMode  = 0o600
	bearerPrefix   = "Bearer "
)

// ErrEmptyToken reports a token file that exists but holds nothing.
var ErrEmptyToken = errors.New("token file is empty")

// TokenPath returns the path of a named token file inside a state directory.
func TokenPath(home, name string) string {
	return filepath.Join(home, name)
}

// EnsureToken reads the token file, generating and storing a fresh token
// the first time it is asked for one.
func EnsureToken(path string) (string, error) {
	token, err := readToken(path)
	if err == nil {
		return token, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	return createToken(path)
}

// ExemptPaths sends the listed exact paths to the open handler and every
// other path through the guarded one.
func ExemptPaths(paths []string, open, guarded http.Handler) http.Handler {
	return Exempt(paths, nil, open, guarded)
}

// Exempt sends the listed exact paths and path prefixes to the open handler
// and every other path through the guarded one. Exact paths are matched
// whole, so an exempt `/healthz` does not exempt `/healthz/extra`.
func Exempt(paths, prefixes []string, open, guarded http.Handler) http.Handler {
	exempt := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		exempt[path] = struct{}{}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, found := exempt[r.URL.Path]; found {
			open.ServeHTTP(w, r)
			return
		}
		for _, prefix := range prefixes {
			if strings.HasPrefix(r.URL.Path, prefix) {
				open.ServeHTTP(w, r)
				return
			}
		}
		guarded.ServeHTTP(w, r)
	})
}

func readToken(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read token file %s: %w", path, err)
	}
	token := strings.TrimSpace(string(content))
	if token == "" {
		return "", fmt.Errorf("%s: %w", path, ErrEmptyToken)
	}
	return token, nil
}

func createToken(path string) (string, error) {
	token, err := generateToken()
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(token), tokenFileMode); err != nil {
		return "", fmt.Errorf("write token file %s: %w", path, err)
	}
	return token, nil
}

func generateToken() (string, error) {
	raw := make([]byte, tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return hex.EncodeToString(raw), nil
}

func bearerToken(r *http.Request) ([]byte, bool) {
	value, found := strings.CutPrefix(r.Header.Get("Authorization"), bearerPrefix)
	if !found || value == "" {
		return nil, false
	}
	return []byte(value), true
}
