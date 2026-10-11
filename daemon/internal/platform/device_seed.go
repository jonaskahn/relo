// Device seed: the stable installation secret device ids derive from.
package platform

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
)

const (
	deviceSeedFileName = "device.seed"
	deviceSeedMode     = 0o600
	deviceSeedBytes    = 32
)

func deviceSeedPath(home string) string {
	return filepath.Join(home, deviceSeedFileName)
}

func deviceSeed(home string) string {
	path := deviceSeedPath(home)
	if raw, err := os.ReadFile(path); err == nil && len(raw) >= deviceSeedBytes {
		return hex.EncodeToString(raw[:deviceSeedBytes])
	}
	seed := make([]byte, deviceSeedBytes)
	if _, err := rand.Read(seed); err != nil {
		return ""
	}
	if err := os.WriteFile(path, seed, deviceSeedMode); err != nil {
		return hex.EncodeToString(seed)
	}
	return hex.EncodeToString(seed)
}
