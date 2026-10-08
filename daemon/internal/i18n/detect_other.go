//go:build !unix && !windows

package i18n

// System reports no locale on a platform whose locale Relo does not read,
// which leaves the configured language in charge.
func System() string { return "" }
