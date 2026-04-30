//go:build integration

package sql_test

import (
	"database/sql"
	"os"
	"testing"

	"github.com/binsarjr/sveltego/auth"
	"github.com/binsarjr/sveltego/auth/storage/adaptertest"
	authsql "github.com/binsarjr/sveltego/auth/storage/sql"
)

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

func TestStore_Postgres_AdapterCompliance(t *testing.T) {
	adaptertest.Run(t, integrationPostgres(t))
}

func TestStore_MySQL_AdapterCompliance(t *testing.T) {
	adaptertest.Run(t, integrationMySQL(t))
}
