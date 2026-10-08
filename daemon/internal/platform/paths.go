// Platform paths: home resolution and data-plane addresses.
package platform

import (
	"os"

	"github.com/jonaskahn/relo/internal/config"
)

func homeDirectory(explicit string) string {
	if explicit != "" {
		return explicit
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

func reloPath(explicit string) string {
	if explicit != "" {
		return explicit
	}
	executable, err := os.Executable()
	if err != nil {
		return ""
	}
	return executable
}

func dataPlaneBaseURL(cfg *config.Config, protocol string) string {
	if cfg == nil {
		return ""
	}
	address := cfg.DataPlaneAddr(protocol)
	if address == "" {
		return ""
	}
	return "http://" + address
}
