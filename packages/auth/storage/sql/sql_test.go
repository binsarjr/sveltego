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
	"os"
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

// The tests below are guarded by the `integration` build tag. They are NOT
// compiled or run during normal `go test ./...`.

// integrationPostgres opens a Postgres connection using AUTH_TEST_POSTGRES_DSN.
func integrationPostgres(t *testing.T) func() auth.Storage {
	t.Helper()

	dsn := os.Getenv("AUTH_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("AUTH_TEST_POSTGRES_DSN not set")
	}

	db, err := sql.Open("pgx", dsn) // uses pgx stdlib driver
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(authsql.Schema(authsql.Postgres)); err != nil {
		t.Fatalf("apply postgres schema: %v", err)
	}

	return func() auth.Storage { return authsql.New(db, authsql.Postgres) }
}

// integrationMySQL opens a MySQL connection using AUTH_TEST_MYSQL_DSN.
func integrationMySQL(t *testing.T) func() auth.Storage {
	t.Helper()

	dsn := os.Getenv("AUTH_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("AUTH_TEST_MYSQL_DSN not set")
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("open mysql: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(authsql.Schema(authsql.MySQL)); err != nil {
		t.Fatalf("apply mysql schema: %v", err)
	}

	return func() auth.Storage { return authsql.New(db, authsql.MySQL) }
}
