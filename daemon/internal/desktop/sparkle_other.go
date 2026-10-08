//go:build !darwin

package desktop

import "errors"

// ErrSparkleUnsupported reports a build without the Sparkle updater Relo.app ships.
var ErrSparkleUnsupported = errors.New("sparkle is only in Relo.app")

func requestSparkleCheck() error {
	return ErrSparkleUnsupported
}
