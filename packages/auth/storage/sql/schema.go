// Package sql provides a database/sql-based implementation of auth.Storage
// with dialect support for Postgres, MySQL, and SQLite.
package sql

import "fmt"

// Dialect identifies the database engine driving the SQL adapter.
// It controls placeholder syntax, type mappings, and conflict handling.
type Dialect int

const (
	// Postgres targets PostgreSQL 12+ using $N placeholders and ON CONFLICT DO NOTHING.
	Postgres Dialect = iota
	// MySQL targets MySQL 8+ using ? placeholders and INSERT IGNORE.
	MySQL
	// SQLite targets SQLite 3 using ? placeholders and ON CONFLICT DO NOTHING.
	SQLite
)

// Schema returns the DDL required to set up the auth tables for the given
// dialect. Users should apply this once via their migration tool or at
// application startup. Running it a second time is safe: all statements use
// IF NOT EXISTS guards.
func Schema(d Dialect) string {
	switch d {
	case Postgres:
		return postgresSchema
	case MySQL:
		return mysqlSchema
	case SQLite:
		return sqliteSchema
	default:
		panic(fmt.Sprintf("auth/storage/sql: unknown dialect %d", d))
	}
}

const postgresSchema = `
CREATE TABLE IF NOT EXISTS auth_users (
    id              TEXT        NOT NULL PRIMARY KEY,
    email           TEXT        NOT NULL,
    email_verified  BOOLEAN     NOT NULL DEFAULT FALSE,
    name            TEXT        NOT NULL DEFAULT '',
    image           TEXT        NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS auth_users_email_uq ON auth_users (email);

CREATE TABLE IF NOT EXISTS auth_sessions (
    id          TEXT        NOT NULL PRIMARY KEY,
    user_id     TEXT        NOT NULL,
    token       TEXT        NOT NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    fresh_until TIMESTAMPTZ NOT NULL,
    ip_address  TEXT        NOT NULL DEFAULT '',
    user_agent  TEXT        NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS auth_sessions_token_uq ON auth_sessions (token);
CREATE INDEX IF NOT EXISTS auth_sessions_user_id_idx  ON auth_sessions (user_id);
CREATE INDEX IF NOT EXISTS auth_sessions_expires_idx  ON auth_sessions (expires_at);

CREATE TABLE IF NOT EXISTS auth_accounts (
    id                  TEXT        NOT NULL PRIMARY KEY,
    user_id             TEXT        NOT NULL,
    provider            TEXT        NOT NULL,
    provider_account_id TEXT        NOT NULL,
    access_token        TEXT        NOT NULL DEFAULT '',
    refresh_token       TEXT        NOT NULL DEFAULT '',
    expires_at          TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL,
    updated_at          TIMESTAMPTZ NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS auth_accounts_provider_uq ON auth_accounts (provider, provider_account_id);
CREATE INDEX IF NOT EXISTS auth_accounts_user_id_idx ON auth_accounts (user_id);

CREATE TABLE IF NOT EXISTS auth_verifications (
    id         TEXT        NOT NULL PRIMARY KEY,
    user_id    TEXT        NOT NULL,
    kind       TEXT        NOT NULL,
    token      TEXT        NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS auth_verifications_token_idx   ON auth_verifications (token);
CREATE INDEX IF NOT EXISTS auth_verifications_expires_idx ON auth_verifications (expires_at);
`

const mysqlSchema = `
CREATE TABLE IF NOT EXISTS auth_users (
    id              VARCHAR(255) NOT NULL PRIMARY KEY,
    email           VARCHAR(255) NOT NULL,
    email_verified  TINYINT(1)   NOT NULL DEFAULT 0,
    name            VARCHAR(255) NOT NULL DEFAULT '',
    image           TEXT         NOT NULL DEFAULT '',
    created_at      DATETIME(6)  NOT NULL,
    updated_at      DATETIME(6)  NOT NULL,
    UNIQUE KEY auth_users_email_uq (email)
);

CREATE TABLE IF NOT EXISTS auth_sessions (
    id          VARCHAR(255) NOT NULL PRIMARY KEY,
    user_id     VARCHAR(255) NOT NULL,
    token       VARCHAR(255) NOT NULL,
    expires_at  DATETIME(6)  NOT NULL,
    fresh_until DATETIME(6)  NOT NULL,
    ip_address  VARCHAR(255) NOT NULL DEFAULT '',
    user_agent  TEXT         NOT NULL DEFAULT '',
    created_at  DATETIME(6)  NOT NULL,
    updated_at  DATETIME(6)  NOT NULL,
    UNIQUE KEY auth_sessions_token_uq (token),
    KEY auth_sessions_user_id_idx (user_id),
    KEY auth_sessions_expires_idx (expires_at)
);

CREATE TABLE IF NOT EXISTS auth_accounts (
    id                  VARCHAR(255) NOT NULL PRIMARY KEY,
    user_id             VARCHAR(255) NOT NULL,
    provider            VARCHAR(255) NOT NULL,
    provider_account_id VARCHAR(255) NOT NULL,
    access_token        TEXT         NOT NULL DEFAULT '',
    refresh_token       TEXT         NOT NULL DEFAULT '',
    expires_at          DATETIME(6),
    created_at          DATETIME(6)  NOT NULL,
    updated_at          DATETIME(6)  NOT NULL,
    UNIQUE KEY auth_accounts_provider_uq (provider, provider_account_id),
    KEY auth_accounts_user_id_idx (user_id)
);

CREATE TABLE IF NOT EXISTS auth_verifications (
    id         VARCHAR(255) NOT NULL PRIMARY KEY,
    user_id    VARCHAR(255) NOT NULL,
    kind       VARCHAR(255) NOT NULL,
    token      VARCHAR(255) NOT NULL,
    expires_at DATETIME(6)  NOT NULL,
    created_at DATETIME(6)  NOT NULL,
    KEY auth_verifications_token_idx (token),
    KEY auth_verifications_expires_idx (expires_at)
);
`

const sqliteSchema = `
CREATE TABLE IF NOT EXISTS auth_users (
    id              TEXT    NOT NULL PRIMARY KEY,
    email           TEXT    NOT NULL UNIQUE,
    email_verified  INTEGER NOT NULL DEFAULT 0,
    name            TEXT    NOT NULL DEFAULT '',
    image           TEXT    NOT NULL DEFAULT '',
    created_at      TEXT    NOT NULL,
    updated_at      TEXT    NOT NULL
);
CREATE INDEX IF NOT EXISTS auth_users_email_idx ON auth_users (email);

CREATE TABLE IF NOT EXISTS auth_sessions (
    id          TEXT    NOT NULL PRIMARY KEY,
    user_id     TEXT    NOT NULL,
    token       TEXT    NOT NULL UNIQUE,
    expires_at  TEXT    NOT NULL,
    fresh_until TEXT    NOT NULL,
    ip_address  TEXT    NOT NULL DEFAULT '',
    user_agent  TEXT    NOT NULL DEFAULT '',
    created_at  TEXT    NOT NULL,
    updated_at  TEXT    NOT NULL
);
CREATE INDEX IF NOT EXISTS auth_sessions_user_id_idx  ON auth_sessions (user_id);
CREATE INDEX IF NOT EXISTS auth_sessions_expires_idx  ON auth_sessions (expires_at);

CREATE TABLE IF NOT EXISTS auth_accounts (
    id                  TEXT NOT NULL PRIMARY KEY,
    user_id             TEXT NOT NULL,
    provider            TEXT NOT NULL,
    provider_account_id TEXT NOT NULL,
    access_token        TEXT NOT NULL DEFAULT '',
    refresh_token       TEXT NOT NULL DEFAULT '',
    expires_at          TEXT,
    created_at          TEXT NOT NULL,
    updated_at          TEXT NOT NULL,
    UNIQUE (provider, provider_account_id)
);
CREATE INDEX IF NOT EXISTS auth_accounts_user_id_idx ON auth_accounts (user_id);

CREATE TABLE IF NOT EXISTS auth_verifications (
    id         TEXT NOT NULL PRIMARY KEY,
    user_id    TEXT NOT NULL,
    kind       TEXT NOT NULL,
    token      TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS auth_verifications_token_idx   ON auth_verifications (token);
CREATE INDEX IF NOT EXISTS auth_verifications_expires_idx ON auth_verifications (expires_at);
`
