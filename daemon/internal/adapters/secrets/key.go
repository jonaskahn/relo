// Master key file: creation, decoding, and paths.
package secrets

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

const (
	// DefaultKeyFile names the vault key inside a state directory when the
	// configuration names no other file.
	DefaultKeyFile = "secret.key"
	keyFileMode    = 0o600
)

// KeyPath returns the vault key path of a state directory. A relative key
// file is resolved inside the state directory; an absolute one is used as is.
func KeyPath(home, keyFile string) string {
	if keyFile == "" {
		keyFile = DefaultKeyFile
	}
	if filepath.IsAbs(keyFile) {
		return keyFile
	}
	return filepath.Join(home, keyFile)
}

func loadOrCreateKey(home string, path string, logger *slog.Logger) ([]byte, error) {
	key, err := readKeyFile(path)
	if err == nil {
		return key, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if _, err := os.Stat(SecretsPath(home)); err == nil {
		return nil, fmt.Errorf("%s: %w", path, ErrKeyMissing)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("inspect secrets file %s: %w", SecretsPath(home), err)
	}
	key, err = createKey(path)
	if errors.Is(err, fs.ErrExist) {
		return readKeyFile(path)
	}
	if err != nil {
		return nil, err
	}
	if logger != nil {
		logger.Info("created secret key", "path", path)
	}
	return key, nil
}

func readKeyFile(path string) ([]byte, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read secret key file %s: %w", path, err)
	}
	return decodeKeyFile(path, payload)
}

func decodeKeyFile(path string, payload []byte) ([]byte, error) {
	trimmed := strings.TrimSpace(string(payload))
	if decoded, err := base64.StdEncoding.DecodeString(trimmed); err == nil && len(decoded) == masterKeySize {
		return decoded, nil
	}
	if len(payload) == masterKeySize {
		return payload, nil
	}
	if len(trimmed) == masterKeySize {
		return []byte(trimmed), nil
	}
	return nil, fmt.Errorf("%s: %w", path, ErrInvalidKey)
}

func createKey(path string) ([]byte, error) {
	key := make([]byte, masterKeySize)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate secret key: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, keyFileMode)
	if err != nil {
		return nil, fmt.Errorf("write secret key file %s: %w", path, err)
	}
	encoded := base64.StdEncoding.EncodeToString(key) + "\n"
	if _, err := file.WriteString(encoded); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("write secret key file %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("close secret key file %s: %w", path, err)
	}
	return key, nil
}
