//go:build !unix && !windows

package platform

import "os"

func lockClaim(*os.File) error { return nil }

func unlockClaim(*os.File) error { return nil }

func tryLockClaim(*os.File) (bool, error) { return true, nil }
