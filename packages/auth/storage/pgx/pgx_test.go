//go:build integration

// Package pgx_test verifies pgx.Store against the shared compliance suite.
//
// Required environment variables:
//
//	AUTH_TEST_POSTGRES_DSN  e.g. "postgres://user:pass@localhost:5432/testdb?sslmode=disable"
//
// Run with:
//
//	go test -tags integration -race ./packages/auth/storage/pgx/...
package pgx_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/binsarjr/sveltego/auth"
	"github.com/binsarjr/sveltego/auth/storage/adaptertest"
	authpgx "github.com/binsarjr/sveltego/auth/storage/pgx"
	authsql "github.com/binsarjr/sveltego/auth/storage/sql"
)

func TestStore_Pgx_AdapterCompliance(t *testing.T) {
	dsn := os.Getenv("AUTH_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("AUTH_TEST_POSTGRES_DSN not set; skipping pgx integration tests")
	}

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)

	// Apply schema (reusing the sql adapter's Postgres DDL).
	conn, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatalf("pool.Acquire: %v", err)
	}
	_, err = conn.Exec(context.Background(), authsql.Schema(authsql.Postgres))
	conn.Release()
	if err != nil {
		t.Fatalf("apply postgres schema: %v", err)
	}

	adaptertest.Run(t, func() auth.Storage {
		return authpgx.New(pool)
	})
}
