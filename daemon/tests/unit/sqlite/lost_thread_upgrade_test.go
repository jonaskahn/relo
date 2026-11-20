package storage_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/tests/testkit"
)

const lostThreadMessage = "No thread state was found for the requested previous_message_id."
const lostThreadBody = `{"type":"error","error":{"type":"not_found_error","message":"No thread state was found for the requested previous_message_id.","details":{"error_code":"thread_not_found"}}}`

func beforeLostThreadRepair(t *testing.T) (*sqlite.DB, sqlite.Migration) {
	t.Helper()
	logger, _ := testkit.TestLogger(t)
	db, err := sqlite.OpenDB(config.DatabasePath(testkit.TempHome(t)), logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	migrations, err := sqlite.LoadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	execLostThreadSQL(t, db, "CREATE TABLE schema_migrations (version INTEGER NOT NULL PRIMARY KEY, applied_at INTEGER NOT NULL, checksum TEXT NOT NULL) STRICT")
	for _, m := range migrations {
		if m.Version == 42 {
			return db, m
		}
		if err := sqlite.ApplyMigration(db, m); err != nil {
			t.Fatal(err)
		}
	}
	t.Fatal("lost-thread repair migration missing")
	return nil, sqlite.Migration{}
}

func execLostThreadSQL(t *testing.T, db *sqlite.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.SQL().Exec(query, args...); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

func seedLostThreadDenials(t *testing.T, db *sqlite.DB) {
	t.Helper()
	execLostThreadSQL(t, db, `INSERT INTO providers (id,template_id,origin,label,auth,created_at,updated_at) VALUES ('different-connection','claude','signin','Synthetic','api_key',1,1)`)
	execLostThreadSQL(t, db, `INSERT INTO models (provider_id,model_id,source,enabled,available,updated_at) VALUES ('different-connection','different-model','listing',1,1,1)`)
	execLostThreadSQL(t, db, `INSERT INTO model_groups (id,label,strategy,enabled,listed,created_at,updated_at) VALUES ('kept','Kept','priority',1,1,1,1)`)
	execLostThreadSQL(t, db, `INSERT INTO model_group_members (group_id,position,provider_id,model_id,weight,enabled) VALUES ('kept',0,'different-connection','different-model',1,1)`)
	execLostThreadSQL(t, db, `INSERT INTO usage_events (id,request_id,timestamp,provider,model,status,duration_ms) VALUES (1,'synthetic',1000,'different-connection','different-model',404,10)`)
	for i, id := range []string{"first", "second"} {
		execLostThreadSQL(t, db, `INSERT INTO credentials (id,provider_id,kind,status,created_at,updated_at) VALUES (?,'different-connection','api_key','active',1,1)`, id)
		execLostThreadSQL(t, db, `INSERT INTO credential_models VALUES (?,'different-model','learned',0,?)`, id, 1020+i)
		execLostThreadSQL(t, db, `INSERT INTO usage_attempts (event_id,ordinal,provider,model,credential_id,status,error_code,duration_ms) VALUES (1,?,'different-connection','different-model',?,404,?,10)`, i, id, lostThreadMessage)
		execLostThreadSQL(t, db, `INSERT INTO usage_captures (event_id,ordinal,kind,status,body,body_bytes,truncated,created_at) VALUES (1,?,'provider_response',404,?,?,0,1030)`, i, []byte(lostThreadBody), len(lostThreadBody))
	}
}

func lostThreadUnchangedState(t *testing.T, db *sqlite.DB) map[string]string {
	t.Helper()
	snapshot := map[string]string{}
	for _, table := range []string{"credentials", "providers", "models", "model_groups", "model_group_members", "credential_rosters", "usage_events", "usage_attempts", "usage_captures"} {
		rows, err := db.SQL().Query("SELECT * FROM " + table + " ORDER BY 1")
		if err != nil {
			t.Fatal(err)
		}
		columns, err := rows.Columns()
		if err != nil {
			t.Fatal(err)
		}
		var state [][]any
		for rows.Next() {
			values := make([]any, len(columns))
			pointers := make([]any, len(columns))
			for i := range values {
				pointers[i] = &values[i]
			}
			if err := rows.Scan(pointers...); err != nil {
				t.Fatal(err)
			}
			state = append(state, values)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		_ = rows.Close()
		snapshot[table] = fmt.Sprint(state)
	}
	return snapshot
}

func TestLostThreadUpgradeRequiresRetainedEvidence(t *testing.T) {
	cases := []struct {
		name, mutation string
		removed        bool
	}{
		{"two accounts beyond request duration", "", true},
		{"genuine model 404", "UPDATE usage_attempts SET error_code='model not found'", false},
		{"genuine 403", "UPDATE usage_attempts SET status=403", false},
		{"listing source", "UPDATE credential_models SET source='listing'", false},
		{"positive learned observation", "UPDATE credential_models SET available=1", false},
		{"other template", "UPDATE providers SET template_id='anthropic'", false},
		{"known roster", "INSERT INTO credential_rosters SELECT id,'listing',0,1 FROM credentials", false},
		{"missing capture", "DELETE FROM usage_captures", false},
		{"missing attempt", "DELETE FROM usage_attempts", false},
		{"missing identity", "UPDATE usage_attempts SET credential_id=NULL", false},
		{"incomplete unflagged body", "UPDATE usage_captures SET body_bytes=body_bytes+1", false},
		{"ambiguous missing identity", "INSERT INTO usage_attempts (event_id,ordinal,provider,model,status,duration_ms) VALUES (1,10,'different-connection','different-model',404,1)", false},
		{"ambiguous ordinal across accounts", "UPDATE usage_attempts SET ordinal=0; UPDATE usage_captures SET ordinal=0", false},
		{"wrong credential", "UPDATE usage_attempts SET credential_id='someone-else'", false},
		{"wrong connection", "UPDATE usage_attempts SET provider='elsewhere'", false},
		{"wrong model", "UPDATE usage_attempts SET model='elsewhere'", false},
		{"wrong ordinal", "UPDATE usage_captures SET ordinal=10+ordinal", false},
		{"client response is not evidence", "UPDATE usage_captures SET kind='agent_response'", false},
		{"capture status mismatch", "UPDATE usage_captures SET status=200", false},
		{"truncated JSON", "UPDATE usage_captures SET truncated=1", false},
		{"malformed JSON", "UPDATE usage_captures SET body=CAST('{broken' AS BLOB)", false},
		{"missing error code", "UPDATE usage_captures SET body=CAST('{}' AS BLOB)", false},
		{"wrong structured code", "UPDATE usage_captures SET body=CAST(replace(CAST(body AS TEXT),'thread_not_found','model_not_found') AS BLOB)", false},
		{"wrong captured message", "UPDATE usage_captures SET body=CAST(replace(CAST(body AS TEXT),'No thread state was found','model not found') AS BLOB)", false},
		{"before request", "UPDATE credential_models SET observed_at=999", false},
		{"after persistence", "UPDATE credential_models SET observed_at=1031", false},
		{"duplicate capture", "INSERT INTO usage_captures (event_id,ordinal,kind,status,body,truncated,created_at) SELECT event_id,ordinal,kind,status,body,truncated,created_at FROM usage_captures", false},
		{"ambiguous overlapping attempt", "INSERT INTO usage_attempts (event_id,ordinal,provider,model,credential_id,status,error_code,duration_ms) SELECT event_id,ordinal,provider,model,credential_id,status,error_code,duration_ms FROM usage_attempts", false},
		{"unresolved overlapping capture", "INSERT INTO usage_attempts (event_id,ordinal,provider,model,credential_id,status,duration_ms) SELECT event_id,ordinal+10,provider,model,credential_id,404,1 FROM usage_attempts", false},
		{"later model refusal", "INSERT INTO usage_events (id,timestamp,provider,model,status,duration_ms) VALUES (2,1040,'different-connection','different-model',404,1); INSERT INTO usage_attempts (event_id,ordinal,provider,model,credential_id,status,error_code,duration_ms) SELECT 2,ordinal,provider,model,credential_id,404,'model not found',1 FROM usage_attempts WHERE event_id=1", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, repair := beforeLostThreadRepair(t)
			seedLostThreadDenials(t, db)
			if tc.mutation != "" {
				execLostThreadSQL(t, db, tc.mutation)
			}
			before := lostThreadUnchangedState(t, db)
			if err := sqlite.Migrate(db); err != nil {
				t.Fatal(err)
			}
			rows, _, err := sqlite.NewCredentialModelRepo(db).List(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			want := 2
			if tc.removed {
				want = 0
			}
			if len(rows) != want {
				t.Fatalf("observations=%+v, want %d", rows, want)
			}
			if after := lostThreadUnchangedState(t, db); !reflect.DeepEqual(before, after) {
				t.Fatal("repair changed unrelated state or usage history")
			}
			if err := sqlite.Migrate(db); err != nil {
				t.Fatal(err)
			}
			result, err := db.SQL().Exec(repair.SQL)
			if err != nil {
				t.Fatal(err)
			}
			count, err := result.RowsAffected()
			if err != nil || count != 0 {
				t.Fatalf("repeated repair affected %d rows: %v", count, err)
			}
		})
	}
}

func TestLostThreadUpgradeEmptyDatabase(t *testing.T) {
	db, _ := beforeLostThreadRepair(t)
	if err := sqlite.Migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.Migrate(db); err != nil {
		t.Fatal(err)
	}
}

func TestLostThreadUpgradeRestoresSelectionAcrossRestart(t *testing.T) {
	db, _ := beforeLostThreadRepair(t)
	seedLostThreadDenials(t, db)
	load := func(database *sqlite.DB) *account.Manager {
		t.Helper()
		pools := account.NewManagerWithOptions(sqlite.NewCredentialStore(database), testkit.SecretStore(nil),
			account.ManagerOptions{ModelRepo: sqlite.NewCredentialModelStore(database)})
		if err := pools.LoadFromDB(context.Background()); err != nil {
			t.Fatal(err)
		}
		return pools
	}
	selection := account.Selection{Models: []string{"different-model"}}
	if _, err := load(db).GetPool("different-connection").Select(selection); !errors.Is(err, account.ErrNoModelAccount) {
		t.Fatalf("poisoned selection = %v, want no model account", err)
	}
	if err := sqlite.Migrate(db); err != nil {
		t.Fatal(err)
	}
	if _, err := load(db).GetPool("different-connection").Select(selection); err != nil {
		t.Fatal(err)
	}
	var path string
	var sequence int
	var name string
	if err := db.SQL().QueryRow("PRAGMA database_list").Scan(&sequence, &name, &path); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	logger, _ := testkit.TestLogger(t)
	reopened, err := sqlite.OpenDB(path, logger)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	if err := sqlite.Migrate(reopened); err != nil {
		t.Fatal(err)
	}
	if _, err := load(reopened).GetPool("different-connection").Select(selection); err != nil {
		t.Fatal(err)
	}
}
