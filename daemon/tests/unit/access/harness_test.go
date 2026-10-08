package access_test

import (
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	appaccess "github.com/jonaskahn/relo/internal/application/access"
	"github.com/jonaskahn/relo/tests/testkit"
)

// harness wires the client-key lifecycle over a real database and a clock the
// test moves by hand.
type harness struct {
	t     *testing.T
	keys  *appaccess.Keys
	clock *testkit.FakeClock
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	clock := testkit.NewFakeClock(time.Unix(1_700_000_000, 0))
	db := testkit.OpenTestDB(t)
	return &harness{t: t, clock: clock, keys: appaccess.NewKeys(sqlite.NewAccessKeyStore(db), clock)}
}
