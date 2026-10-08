// Pool strategies: the names routing may choose from.
package sqlite

import (
	"fmt"

	"github.com/jonaskahn/relo/internal/account"
)

// Strategy names the providers table accepts, mirroring its CHECK constraint
// so a caller learns before the write.
const (
	StrategyRoundRobin  = "round-robin"
	StrategyLeastLoaded = "least-loaded"
	StrategyRandom      = "random"
)

// Strategies returns the accepted pool strategy names.
func Strategies() []string {
	return []string{StrategyRoundRobin, StrategyLeastLoaded, StrategyRandom}
}

// CheckStrategy reports whether a name is one the pool can build. An empty
// name means the provider keeps the global strategy.
func CheckStrategy(name string) error {
	if name == "" {
		return nil
	}
	for _, known := range Strategies() {
		if name == known {
			return nil
		}
	}
	return fmt.Errorf("%s: %w", name, account.ErrUnknownStrategy)
}
