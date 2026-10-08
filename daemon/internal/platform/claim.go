// The daemon claim is the lock a headless run holds while it decides to
// bind, so the daemon agent and the tray cannot both become the listener.
package platform

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/jonaskahn/relo/internal/config"
)

const claimFileName = "daemon.lock"

type daemonClaim struct {
	file *os.File
	once sync.Once
}

func (c *daemonClaim) release() {
	if c == nil {
		return
	}
	c.once.Do(func() {
		if c.file == nil {
			return
		}
		_ = unlockClaim(c.file)
		_ = c.file.Close()
		c.file = nil
	})
}

func claimDaemon(ctx context.Context, home string, cfg *config.Config) (*daemonClaim, bool, error) {
	claim, err := acquireDaemonClaim(home)
	if err != nil {
		return nil, false, err
	}
	if healthyRelo(ctx, cfg.Server.Addr(), cfg.Server.Port) {
		claim.release()
		return nil, true, nil
	}
	return claim, false, nil
}

// DaemonClaimHeld reports whether some process already holds the startup
// claim. The tray waits for that process to publish instead of spawning
// another daemon.
func DaemonClaimHeld(home string) bool {
	file, err := os.OpenFile(claimPath(home), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return false
	}
	defer func() { _ = file.Close() }()
	acquired, err := tryLockClaim(file)
	if err != nil || !acquired {
		return err == nil && !acquired
	}
	_ = unlockClaim(file)
	return false
}

func acquireDaemonClaim(home string) (*daemonClaim, error) {
	if err := config.EnsureReloHome(home); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(claimPath(home), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open the daemon claim: %w", err)
	}
	if err := lockClaim(file); err != nil {
		_ = file.Close()
		return nil, err
	}
	return &daemonClaim{file: file}, nil
}

func claimPath(home string) string {
	return filepath.Join(home, claimFileName)
}

func healthyRelo(ctx context.Context, address string, port int) bool {
	if port <= 0 || address == "" {
		return false
	}
	owner, found := portOwnerPID(port)
	if !found || !reloProcess(owner) {
		return false
	}
	return HealthOf(ctx, address)
}
