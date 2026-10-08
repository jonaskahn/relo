// Encrypted fallback store for secrets.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

const (
	secretsFileName = "secrets.enc"
	secretFileMode  = 0o600
	formatVersion   = 0x01
	masterKeySize   = 32
)

// EncryptedStore keeps every secret in one AES-256-GCM encrypted file,
// which lets Relo work on hosts without a usable keychain.
type EncryptedStore struct {
	mu   sync.Mutex
	key  []byte
	path string
}

// SecretsPath returns the encrypted secrets file inside a state directory.
func SecretsPath(home string) string {
	return filepath.Join(home, secretsFileName)
}

// NewEncryptedStore returns an encrypted-vault store. The key must be exactly
// 32 bytes; Relo keeps it in a file of its own.
func NewEncryptedStore(masterKey []byte, filePath string) (SecretStore, error) {
	if len(masterKey) != masterKeySize {
		return nil, ErrInvalidKey
	}
	key := make([]byte, masterKeySize)
	copy(key, masterKey)
	return &EncryptedStore{key: key, path: filePath}, nil
}

// Set stores secret under ref, rewriting the whole secrets file.
func (s *EncryptedStore) Set(ref string, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	secrets, err := s.read()
	if err != nil {
		return err
	}
	secrets[ref] = value
	return s.write(secrets)
}

// Get resolves the secret stored under ref.
func (s *EncryptedStore) Get(ref string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	secrets, err := s.read()
	if err != nil {
		return "", err
	}
	value, found := secrets[ref]
	if !found {
		return "", fmt.Errorf("%s: %w", ref, ErrNotFound)
	}
	return value, nil
}

// Delete removes the secret stored under ref.
func (s *EncryptedStore) Delete(ref string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	secrets, err := s.read()
	if err != nil {
		return err
	}
	if _, found := secrets[ref]; !found {
		return fmt.Errorf("%s: %w", ref, ErrNotFound)
	}
	delete(secrets, ref)
	return s.write(secrets)
}

// Mode reports the encrypted-file backend.
func (s *EncryptedStore) Mode() SecretMode {
	return ModeEncryptedFile
}

func (s *EncryptedStore) read() (map[string]string, error) {
	payload, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read secrets file %s: %w", s.path, err)
	}
	plaintext, err := s.decrypt(payload)
	if err != nil {
		return nil, err
	}
	secrets := map[string]string{}
	if err := json.Unmarshal(plaintext, &secrets); err != nil {
		return nil, fmt.Errorf("decode secrets file %s: %w", s.path, ErrDecrypt)
	}
	return secrets, nil
}

func (s *EncryptedStore) write(secrets map[string]string) error {
	plaintext, err := json.Marshal(secrets)
	if err != nil {
		return fmt.Errorf("encode secrets: %w", err)
	}
	payload, err := s.encrypt(plaintext)
	if err != nil {
		return err
	}
	return writeAtomic(s.path, payload)
}

func (s *EncryptedStore) encrypt(plaintext []byte) ([]byte, error) {
	gcm, err := s.gcm()
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}
	return gcm.Seal(append([]byte{formatVersion}, nonce...), nonce, plaintext, nil), nil
}

func (s *EncryptedStore) decrypt(payload []byte) ([]byte, error) {
	gcm, err := s.gcm()
	if err != nil {
		return nil, err
	}
	header := 1 + gcm.NonceSize()
	if len(payload) <= header || payload[0] != formatVersion {
		return nil, fmt.Errorf("%s: %w", s.path, ErrDecrypt)
	}
	plaintext, err := gcm.Open(nil, payload[1:header], payload[header:], nil)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", s.path, ErrDecrypt)
	}
	return plaintext, nil
}

func (s *EncryptedStore) gcm() (cipher.AEAD, error) {
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}
	return gcm, nil
}

func writeAtomic(path string, payload []byte) error {
	temp := path + ".tmp"
	if err := os.WriteFile(temp, payload, secretFileMode); err != nil {
		return fmt.Errorf("write secrets file %s: %w", temp, err)
	}
	if err := os.Rename(temp, path); err != nil {
		return fmt.Errorf("replace secrets file %s: %w", path, err)
	}
	return nil
}
