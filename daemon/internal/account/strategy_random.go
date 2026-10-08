// Random strategy: uniform credential selection.
package account

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"math/big"
)

// ErrNoRandomness reports a random source that could not produce a
// selection.
var ErrNoRandomness = errors.New("credential selection could not read a random source")

// RandomStrategy picks a candidate uniformly at random, which keeps
// requests away from any predictable account.
type RandomStrategy struct {
	Source io.Reader
}

// Select returns a randomly chosen candidate.
func (s *RandomStrategy) Select(candidates []PoolEntry, _ Selection) (PoolEntry, error) {
	if len(candidates) == 0 {
		return PoolEntry{}, ErrNoCredentials
	}
	index, err := rand.Int(s.reader(), big.NewInt(int64(len(candidates))))
	if err != nil {
		return PoolEntry{}, fmt.Errorf("pick a credential: %w: %v", ErrNoRandomness, err)
	}
	return candidates[index.Int64()], nil
}

func (s *RandomStrategy) reader() io.Reader {
	if s.Source == nil {
		return rand.Reader
	}
	return s.Source
}
