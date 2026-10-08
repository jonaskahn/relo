//go:build !darwin

package desktop

import "log/slog"

func applyAccessoryPresence(_ *slog.Logger) {}
