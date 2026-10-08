// Strategy names: resolving configured pool strategies.
package account

import (
	"errors"
	"fmt"
)

// ErrUnknownStrategy reports a strategy name no builder matches.
var ErrUnknownStrategy = errors.New("unknown pool strategy")

// Strategy names the pool can build, matching the names provider settings
// store.
const (
	StrategyRoundRobin  = "round-robin"
	StrategyLeastLoaded = "least-loaded"
	StrategyRandom      = "random"
)

// StrategyNamed builds the strategy one name selects. An empty name is the
// default strategy, so a provider without settings keeps rotating by quota
// headroom.
func StrategyNamed(name string) (PoolStrategy, error) {
	switch name {
	case "", StrategyLeastLoaded:
		return &LeastLoadedStrategy{}, nil
	case StrategyRoundRobin:
		return &RoundRobinStrategy{}, nil
	case StrategyRandom:
		return &RandomStrategy{}, nil
	default:
		return nil, fmt.Errorf("%s: %w", name, ErrUnknownStrategy)
	}
}

// StrategyName reports the name of a strategy this package built, so a
// decision can record what actually selected the credential.
func StrategyName(strategy PoolStrategy) string {
	switch strategy.(type) {
	case *LeastLoadedStrategy:
		return StrategyLeastLoaded
	case *RoundRobinStrategy:
		return StrategyRoundRobin
	case *RandomStrategy:
		return StrategyRandom
	default:
		return ""
	}
}
