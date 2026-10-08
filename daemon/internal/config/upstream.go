// Upstream is the provider-call policy an operator configures for the relay:
// how long one attempt may stay silent before it is cancelled and retried,
// and the random waits between retries.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// UpstreamConfig is the provider-call policy stored in the startup file.
type UpstreamConfig struct {
	TimeoutSeconds int `toml:"timeout_seconds"`
	// RetryBackoff is the wait before each retry as a low–high second range,
	// in retry order. It is the connection's own windows when one is set,
	// otherwise the global ones.
	RetryBackoff [][2]int `toml:"retry_backoff"`
	// FailoverCooldownSeconds is the first step of the denylist walk a
	// refused account or route member goes through after a rate limit or
	// server fault, in seconds: the next failure of the same entry waits the
	// next, longer preset. Zero starts the walk with no wait at all.
	// A success resets the entry's walk; a wait expires on its own.
	FailoverCooldownSeconds int `toml:"failover_cooldown_seconds"`
}

// UpstreamTimeoutPresets are the call waits an operator may pick, in
// seconds: how long a provider call tolerates silence between bytes. The
// shipped default is 300. The copy they return keeps callers from mutating
// the shipped choices.
func UpstreamTimeoutPresets() []int {
	return []int{180, 300, 600, 900, 1800}
}

// ErrInvalidUpstreamTimeout reports a call wait that is not one of the
// presets.
var ErrInvalidUpstreamTimeout = errors.New(
	"must be one of: 180, 300, 600, 900, 1800")

// ErrInvalidUpstreamRetryBackoff reports retry windows the relay cannot use.
var ErrInvalidUpstreamRetryBackoff = errors.New(
	"must be three ranges with 0 <= low < high <= 600")

// FailoverCooldownPresets are the escalating denylist waits an operator may
// pick, in seconds: the chosen preset is the first step, and each further
// failure of the same account or route member moves to the next, longer one.
// The shipped default is zero: the first failure keeps no wait. The copy they
// return keeps callers from mutating the shipped choices.
func FailoverCooldownPresets() []int {
	return []int{0, 180, 300, 900, 1800, 3600, 18000}
}

// ErrInvalidUpstreamFailoverCooldown reports a denylist wait nothing offers.
var ErrInvalidUpstreamFailoverCooldown = errors.New(
	"must be one of: 0, 180, 300, 900, 1800, 3600, 18000")

const maxRetryWindowSeconds = 600

// ValidateUpstreamTimeout reports a call wait the relay cannot use.
func ValidateUpstreamTimeout(seconds int) error {
	for _, preset := range UpstreamTimeoutPresets() {
		if seconds == preset {
			return nil
		}
	}
	return fmt.Errorf("upstream.timeout_seconds: %w", ErrInvalidUpstreamTimeout)
}

// ValidateUpstreamRetryBackoff reports retry windows outside the three
// low–high ranges a request can wait through.
func ValidateUpstreamRetryBackoff(windows [][2]int) error {
	if len(windows) != len(DefaultUpstreamRetryBackoff()) {
		return fmt.Errorf("upstream.retry_backoff: %w", ErrInvalidUpstreamRetryBackoff)
	}
	for _, window := range windows {
		if window[0] < 0 || window[1] <= window[0] || window[1] > maxRetryWindowSeconds {
			return fmt.Errorf("upstream.retry_backoff: %w", ErrInvalidUpstreamRetryBackoff)
		}
	}
	return nil
}

// ValidateUpstreamFailoverCooldown reports a denylist wait nothing offers.
func ValidateUpstreamFailoverCooldown(seconds int) error {
	for _, preset := range FailoverCooldownPresets() {
		if seconds == preset {
			return nil
		}
	}
	return fmt.Errorf("upstream.failover_cooldown_seconds: %w", ErrInvalidUpstreamFailoverCooldown)
}

// FailoverBackoff turns a chosen preset into the escalating waits future
// failures walk through: the preset itself, then every longer preset in
// turn. An entry that fails again without a success between moves one step
// down the list, and failures past its end stay on the longest wait.
func FailoverBackoff(seconds int) []time.Duration {
	steps := make([]time.Duration, 0, len(FailoverCooldownPresets()))
	for _, preset := range FailoverCooldownPresets() {
		if preset >= seconds {
			steps = append(steps, time.Duration(preset)*time.Second)
		}
	}
	return steps
}

// ReadUpstreamTimeout reads the call wait from the startup file. A missing
// file or key answers the default, and a stored value this build no longer
// offers also reads as the default, so an upgrade never leaves the relay
// using a value the console cannot show.
func ReadUpstreamTimeout(path string) (int, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return DefaultUpstreamTimeoutSeconds, nil
	}
	if err != nil {
		return 0, err
	}
	var file struct {
		Upstream UpstreamConfig `toml:"upstream"`
	}
	meta, err := toml.Decode(string(data), &file)
	if err != nil {
		return 0, err
	}
	if !meta.IsDefined("upstream", "timeout_seconds") {
		return DefaultUpstreamTimeoutSeconds, nil
	}
	if err := ValidateUpstreamTimeout(file.Upstream.TimeoutSeconds); err != nil {
		return DefaultUpstreamTimeoutSeconds, nil
	}
	return file.Upstream.TimeoutSeconds, nil
}

// ReadUpstreamRetryBackoff reads the retry windows from the startup file. A
// missing file or key answers the default, and stored windows this build no
// longer accepts also read as the default.
func ReadUpstreamRetryBackoff(path string) ([][2]int, error) {
	defaults := append([][2]int(nil), DefaultUpstreamRetryBackoff()...)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return defaults, nil
	}
	if err != nil {
		return nil, err
	}
	var file struct {
		Upstream UpstreamConfig `toml:"upstream"`
	}
	meta, err := toml.Decode(string(data), &file)
	if err != nil {
		return nil, err
	}
	if !meta.IsDefined("upstream", "retry_backoff") {
		return defaults, nil
	}
	if err := ValidateUpstreamRetryBackoff(file.Upstream.RetryBackoff); err != nil {
		return defaults, nil
	}
	return file.Upstream.RetryBackoff, nil
}

// UpdateUpstreamTimeout stores the call wait, keeping the rest of the startup
// file.
func UpdateUpstreamTimeout(path string, seconds int) error {
	if err := ValidateUpstreamTimeout(seconds); err != nil {
		return err
	}
	return updateUpstream(path, "timeout_seconds", fmt.Sprintf("%d", seconds))
}

// UpdateUpstreamRetryBackoff stores the three retry windows, keeping the rest
// of the startup file.
func UpdateUpstreamRetryBackoff(path string, windows [][2]int) error {
	if err := ValidateUpstreamRetryBackoff(windows); err != nil {
		return err
	}
	return updateUpstream(path, "retry_backoff", formatRetryBackoff(windows))
}

// ReadUpstreamFailoverCooldown reads the denylist wait from the startup file.
// A missing file or key answers the default, and a stored value this build
// no longer offers also reads as the default.
func ReadUpstreamFailoverCooldown(path string) (int, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return DefaultFailoverCooldownSeconds, nil
	}
	if err != nil {
		return 0, err
	}
	var file struct {
		Upstream UpstreamConfig `toml:"upstream"`
	}
	meta, err := toml.Decode(string(data), &file)
	if err != nil {
		return 0, err
	}
	if !meta.IsDefined("upstream", "failover_cooldown_seconds") {
		return DefaultFailoverCooldownSeconds, nil
	}
	if err := ValidateUpstreamFailoverCooldown(file.Upstream.FailoverCooldownSeconds); err != nil {
		return DefaultFailoverCooldownSeconds, nil
	}
	return file.Upstream.FailoverCooldownSeconds, nil
}

// UpdateUpstreamFailoverCooldown stores the denylist wait, keeping the rest
// of the startup file.
func UpdateUpstreamFailoverCooldown(path string, seconds int) error {
	if err := ValidateUpstreamFailoverCooldown(seconds); err != nil {
		return err
	}
	return updateUpstream(path, "failover_cooldown_seconds", fmt.Sprintf("%d", seconds))
}

func updateUpstream(path, key, value string) error {
	appearanceMu.Lock()
	defer appearanceMu.Unlock()
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	updated := replaceUpstreamSetting(string(data), key, value)
	return writeFileAtomically(path, updated)
}

func replaceUpstreamSetting(source, key, value string) string {
	return upsertTableKey(source, "upstream", key, value)
}

func formatRetryBackoff(windows [][2]int) string {
	parts := make([]string, 0, len(windows))
	for _, window := range windows {
		parts = append(parts, fmt.Sprintf("[%d, %d]", window[0], window[1]))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}
