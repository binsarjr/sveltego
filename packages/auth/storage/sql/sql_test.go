// Package sql_test verifies the sql.Store adapter against the shared compliance
// suite. Tests run against an in-memory SQLite database (via modernc.org/sqlite)
// so no external services are required for `go test ./...`.
//
// Integration tests against live Postgres / MySQL databases require the
// following environment variables and the `integration` build tag:
//
//	AUTH_TEST_POSTGRES_DSN  e.g. "postgres://user:pass@localhost:5432/testdb?sslmode=disable"
//	AUTH_TEST_MYSQL_DSN     e.g. "user:pass@tcp(localhost:3306)/testdb"
//	AUTH_TEST_SQLITE_FILE   optional path; defaults to ":memory:" when blank
package sql_test

import (
	"database/sql"
	"testing"

	"github.com/binsarjr/sveltego/auth"
	"github.com/binsarjr/sveltego/auth/storage/adaptertest"
	authsql "github.com/binsarjr/sveltego/auth/storage/sql"

	_ "modernc.org/sqlite"
)

// sqliteFactory opens a fresh in-memory SQLite database, applies the schema,
// and returns an auth.Storage backed by it. Each call creates a new isolated
// database so the adaptertest suite gets clean state per sub-test.
func sqliteFactory(t *testing.T) auth.Storage {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Skipf("modernc.org/sqlite unavailable: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(authsql.Schema(authsql.SQLite)); err != nil {
		t.Fatalf("apply SQLite schema: %v", err)
	}
	return authsql.New(db, authsql.SQLite)
}

func TestStore_SQLite_AdapterCompliance(t *testing.T) {
	adaptertest.Run(t, func() auth.Storage { return sqliteFactory(t) })
}

// Integration tests (Postgres, MySQL) live in sql_integration_test.go
// and require the `integration` build tag plus environment variables.
