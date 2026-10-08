// OpenCode service supervision: restart through its binary.
package codingclients

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const openCodeRestartTimeout = 45 * time.Second

// OpenCode service errors name why its background server cannot be driven:
// a missing key and a missing launcher.
var (
	ErrOpenCodeKeyMissing      = errors.New("opencode: the integration has no key")
	ErrOpenCodeLauncherMissing = errors.New("opencode: the OpenCode launcher was not found; install OpenCode or put it on PATH")
)

// RestartOpenCodeService stores this integration's key in OpenCode's service
// config and restarts the background server. The server expands
// {env:RELO_OPENCODE_API_KEY} from its own environment, and a server that
// started before Relo published the variable keeps calling without one until
// it is started again with the key in that config. Home is the operator's
// home, which the launcher resolver scans when the daemon PATH holds no
// OpenCode.
func RestartOpenCodeService(ctx context.Context, home, apiKey string) error {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return ErrOpenCodeKeyMissing
	}
	if ctx == nil {
		ctx = context.Background()
	}
	binary, err := findClientBinary(ctx, home, "opencode")
	if err != nil {
		return ErrOpenCodeLauncherMissing
	}
	ctx, cancel := context.WithTimeout(ctx, openCodeRestartTimeout)
	defer cancel()
	envKey := EnvKeyName(ClientOpenCode)
	// set stops the server and writes the variable. restart then starts a
	// server that inherits that stored environment.
	if err := runOpenCode(ctx, home, binary, "service", "set", "env", envKey, apiKey); err != nil {
		return fmt.Errorf("store the OpenCode service key: %w", err)
	}
	if err := runOpenCode(ctx, home, binary, "service", "restart"); err != nil {
		return fmt.Errorf("restart the OpenCode service: %w", err)
	}
	return nil
}

func runOpenCode(ctx context.Context, home, binary string, args ...string) error {
	command := exec.CommandContext(ctx, binary, args...)
	command.Env = envWithLauncherNode(ctx, home, binary, os.Environ())
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w (%s)", err, strings.TrimSpace(string(output)))
	}
	return nil
}
