// Admin is the external-access choice stored in the startup file: whether the
// console may be reached through a forwarded address while the sign-in is off.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"regexp"

	"github.com/BurntSushi/toml"
)

var adminHeader = regexp.MustCompile(`^\s*\[admin\]\s*(?:#.*)?$`)
var adminAllowExternalKey = regexp.MustCompile(
	`^(\s*)allow_external(\s*=\s*)(?:true|false)(\s*(?:#.*)?)(\r?\n?)$`)

// ReadAdminAllowExternal reads the external-access choice from the startup
// file. A missing file is false.
func ReadAdminAllowExternal(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var file struct {
		Admin AdminConfig `toml:"admin"`
	}
	if _, err := toml.Decode(string(data), &file); err != nil {
		return false, err
	}
	return file.Admin.AllowExternal, nil
}

// ValidateAdminChange reports whether storing allow would leave a
// configuration the next start accepts. Turning external access off on a
// non-loopback bind with the sign-in off is the one change that would not.
func ValidateAdminChange(path string, allow bool) error {
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return validateAdminChange(data, allow)
}

func validateAdminChange(data []byte, allow bool) error {
	if allow {
		return nil
	}
	cfg := DefaultConfig()
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return err
	}
	cfg.Admin.AllowExternal = false
	return validateAdmin(&cfg)
}

// UpdateAdminAllowExternal stores the external-access choice, keeping the
// rest of the startup file.
func UpdateAdminAllowExternal(path string, allow bool) error {
	appearanceMu.Lock()
	defer appearanceMu.Unlock()
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := validateAdminChange(data, allow); err != nil {
		return err
	}
	return writeFileAtomically(path, replaceAdminAllowExternal(string(data), allow))
}

func replaceAdminAllowExternal(source string, allow bool) string {
	value := fmt.Sprintf("%t", allow)
	return rewriteTableKey(source, adminHeader, adminAllowExternalKey, "admin", "allow_external", value)
}
