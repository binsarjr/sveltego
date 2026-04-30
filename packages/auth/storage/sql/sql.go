package sql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/binsarjr/sveltego/auth"
)

// Store implements auth.Storage using a *sql.DB. Construct via New.
type Store struct {
	db      *sql.DB
	dialect Dialect
}

// New returns a *Store that wraps db and speaks the given dialect.
// The caller is responsible for calling Schema(dialect) to create tables
// before the first use.
func New(db *sql.DB, dialect Dialect) *Store {
	return &Store{db: db, dialect: dialect}
}

// Ensure Store satisfies auth.Storage at compile time.
var _ auth.Storage = (*Store)(nil)

// ph returns the dialect-appropriate positional placeholder for argument n
// (1-indexed). Postgres uses $1, $2, …; MySQL and SQLite use ?.
func (s *Store) ph(n int) string {
	if s.dialect == Postgres {
		return fmt.Sprintf("$%d", n)
	}
	return "?"
}

// placeholders builds a comma-separated list of n positional placeholders.
func (s *Store) placeholders(n int) string {
	parts := make([]string, n)
	for i := range n {
		parts[i] = s.ph(i + 1)
	}
	return strings.Join(parts, ", ")
}

// isConflict reports whether err represents a uniqueness violation on any
// supported driver.
func (s *Store) isConflict(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	switch s.dialect {
	case Postgres:
		// lib/pq: pq: duplicate key value violates unique constraint "..."
		// pgdriver (pgx stdlib): ERROR: duplicate key value (SQLSTATE 23505)
		return strings.Contains(msg, "23505") ||
			strings.Contains(msg, "duplicate key") ||
			strings.Contains(msg, "unique constraint")
	case MySQL:
		// Error 1062: Duplicate entry '...' for key '...'
		return strings.Contains(msg, "1062") ||
			strings.Contains(msg, "Duplicate entry")
	case SQLite:
		// UNIQUE constraint failed: ...
		return strings.Contains(msg, "UNIQUE constraint failed")
	}
	return false
}

// timeVal converts a time.Time to a value suitable for the current dialect.
// Postgres and MySQL drivers handle time.Time natively; SQLite stores RFC3339.
func (s *Store) timeVal(t time.Time) interface{} {
	if s.dialect == SQLite {
		return t.UTC().Format(time.RFC3339Nano)
	}
	return t.UTC()
}

// timePtrVal converts *time.Time for storage.
func (s *Store) timePtrVal(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return s.timeVal(*t)
}

// scanTime reads a time value from a column. SQLite stores RFC3339 strings;
// Postgres/MySQL return time.Time directly.
func scanTime(v interface{}) (time.Time, error) {
	switch t := v.(type) {
	case time.Time:
		return t.UTC(), nil
	case string:
		parsed, err := time.Parse(time.RFC3339Nano, t)
		if err != nil {
			parsed, err = time.Parse(time.RFC3339, t)
		}
		return parsed.UTC(), err
	case []byte:
		parsed, err := time.Parse(time.RFC3339Nano, string(t))
		if err != nil {
			parsed, err = time.Parse(time.RFC3339, string(t))
		}
		return parsed.UTC(), err
	default:
		return time.Time{}, fmt.Errorf("auth/storage/sql: unexpected time type %T", v)
	}
}

// scanTimePtr reads a nullable time value from a column.
func scanTimePtr(v interface{}) (*time.Time, error) {
	if v == nil {
		return nil, nil
	}
	t, err := scanTime(v)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// --- User ---

// CreateUser persists u. Returns ErrConflict if the email is already taken.
func (s *Store) CreateUser(ctx context.Context, u *auth.User) error {
	q := fmt.Sprintf(`INSERT INTO auth_users
		(id, email, email_verified, name, image, created_at, updated_at)
		VALUES (%s)`,
		s.placeholders(7))

	emailVerified := 0
	if u.EmailVerified {
		emailVerified = 1
	}

	_, err := s.db.ExecContext(ctx, q,
		u.ID, u.Email, emailVerified, u.Name, u.Image,
		s.timeVal(u.CreatedAt), s.timeVal(u.UpdatedAt),
	)
	if s.isConflict(err) {
		return fmt.Errorf("auth: %w", auth.ErrConflict)
	}
	return err
}

// UserByID returns the user with the given id. Returns ErrNotFound if absent.
func (s *Store) UserByID(ctx context.Context, id string) (*auth.User, error) {
	q := fmt.Sprintf(`SELECT id, email, email_verified, name, image, created_at, updated_at
		FROM auth_users WHERE id = %s`, s.ph(1))

	row := s.db.QueryRowContext(ctx, q, id)
	return s.scanUser(row)
}

// UserByEmail returns the user with the given email. Returns ErrNotFound if absent.
func (s *Store) UserByEmail(ctx context.Context, email string) (*auth.User, error) {
	q := fmt.Sprintf(`SELECT id, email, email_verified, name, image, created_at, updated_at
		FROM auth_users WHERE email = %s`, s.ph(1))

	row := s.db.QueryRowContext(ctx, q, email)
	return s.scanUser(row)
}

func (s *Store) scanUser(row *sql.Row) (*auth.User, error) {
	var u auth.User
	var emailVerified int
	var createdRaw, updatedRaw interface{}

	err := row.Scan(
		&u.ID, &u.Email, &emailVerified, &u.Name, &u.Image,
		&createdRaw, &updatedRaw,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("auth: %w", auth.ErrNotFound)
	}
	if err != nil {
		return nil, err
	}

	u.EmailVerified = emailVerified != 0
	if u.CreatedAt, err = scanTime(createdRaw); err != nil {
		return nil, err
	}
	if u.UpdatedAt, err = scanTime(updatedRaw); err != nil {
		return nil, err
	}
	return &u, nil
}

// UpdateUser persists changes to an existing user. Returns ErrNotFound if absent.
func (s *Store) UpdateUser(ctx context.Context, u *auth.User) error {
	q := fmt.Sprintf(`UPDATE auth_users
		SET email = %s, email_verified = %s, name = %s, image = %s, updated_at = %s
		WHERE id = %s`,
		s.ph(1), s.ph(2), s.ph(3), s.ph(4), s.ph(5), s.ph(6))

	emailVerified := 0
	if u.EmailVerified {
		emailVerified = 1
	}

	res, err := s.db.ExecContext(ctx, q,
		u.Email, emailVerified, u.Name, u.Image,
		s.timeVal(u.UpdatedAt), u.ID,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("auth: %w", auth.ErrNotFound)
	}
	return nil
}

// DeleteUser removes the user and cascades to sessions, accounts, and
// verifications inside a single transaction. Returns ErrNotFound if absent.
func (s *Store) DeleteUser(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	// Verify the user exists first.
	checkQ := fmt.Sprintf(`SELECT 1 FROM auth_users WHERE id = %s`, s.ph(1))
	var dummy int
	err = tx.QueryRowContext(ctx, checkQ, id).Scan(&dummy)
	if errors.Is(err, sql.ErrNoRows) {
		_ = tx.Rollback()
		return fmt.Errorf("auth: %w", auth.ErrNotFound)
	}
	if err != nil {
		return err
	}

	for _, delQ := range []string{
		fmt.Sprintf(`DELETE FROM auth_sessions      WHERE user_id = %s`, s.ph(1)),
		fmt.Sprintf(`DELETE FROM auth_accounts      WHERE user_id = %s`, s.ph(1)),
		fmt.Sprintf(`DELETE FROM auth_verifications WHERE user_id = %s`, s.ph(1)),
		fmt.Sprintf(`DELETE FROM auth_users         WHERE id      = %s`, s.ph(1)),
	} {
		if _, err = tx.ExecContext(ctx, delQ, id); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// --- Session ---

// CreateSession persists s.
func (s *Store) CreateSession(ctx context.Context, sess *auth.Session) error {
	q := fmt.Sprintf(`INSERT INTO auth_sessions
		(id, user_id, token, expires_at, fresh_until, ip_address, user_agent, created_at, updated_at)
		VALUES (%s)`,
		s.placeholders(9))

	_, err := s.db.ExecContext(ctx, q,
		sess.ID, sess.UserID, sess.Token,
		s.timeVal(sess.ExpiresAt), s.timeVal(sess.FreshUntil),
		sess.IPAddress, sess.UserAgent,
		s.timeVal(sess.CreatedAt), s.timeVal(sess.UpdatedAt),
	)
	return err
}

// SessionByToken returns the session for the given token. Returns ErrNotFound
// if absent, or ErrSessionExpired if the session is past its ExpiresAt.
func (s *Store) SessionByToken(ctx context.Context, token string) (*auth.Session, error) {
	q := fmt.Sprintf(`SELECT id, user_id, token, expires_at, fresh_until,
		ip_address, user_agent, created_at, updated_at
		FROM auth_sessions WHERE token = %s`, s.ph(1))

	row := s.db.QueryRowContext(ctx, q, token)

	var sess auth.Session
	var expiresRaw, freshRaw, createdRaw, updatedRaw interface{}

	err := row.Scan(
		&sess.ID, &sess.UserID, &sess.Token,
		&expiresRaw, &freshRaw,
		&sess.IPAddress, &sess.UserAgent,
		&createdRaw, &updatedRaw,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("auth: %w", auth.ErrNotFound)
	}
	if err != nil {
		return nil, err
	}

	if sess.ExpiresAt, err = scanTime(expiresRaw); err != nil {
		return nil, err
	}
	if sess.FreshUntil, err = scanTime(freshRaw); err != nil {
		return nil, err
	}
	if sess.CreatedAt, err = scanTime(createdRaw); err != nil {
		return nil, err
	}
	if sess.UpdatedAt, err = scanTime(updatedRaw); err != nil {
		return nil, err
	}

	if time.Now().After(sess.ExpiresAt) {
		return nil, fmt.Errorf("auth: %w", auth.ErrSessionExpired)
	}
	return &sess, nil
}

// RefreshSession extends the expiry of the session identified by token.
// Returns ErrNotFound if absent.
func (s *Store) RefreshSession(ctx context.Context, token string, newExpiry time.Time) error {
	q := fmt.Sprintf(`UPDATE auth_sessions SET expires_at = %s, updated_at = %s WHERE token = %s`,
		s.ph(1), s.ph(2), s.ph(3))

	res, err := s.db.ExecContext(ctx, q, s.timeVal(newExpiry), s.timeVal(time.Now()), token)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("auth: %w", auth.ErrNotFound)
	}
	return nil
}

// RevokeSession deletes the session identified by token. Idempotent.
func (s *Store) RevokeSession(ctx context.Context, token string) error {
	q := fmt.Sprintf(`DELETE FROM auth_sessions WHERE token = %s`, s.ph(1))
	_, err := s.db.ExecContext(ctx, q, token)
	return err
}

// RevokeAllSessions deletes every session belonging to userID. Idempotent.
func (s *Store) RevokeAllSessions(ctx context.Context, userID string) error {
	q := fmt.Sprintf(`DELETE FROM auth_sessions WHERE user_id = %s`, s.ph(1))
	_, err := s.db.ExecContext(ctx, q, userID)
	return err
}

// --- Account ---

// CreateAccount persists a.
func (s *Store) CreateAccount(ctx context.Context, a *auth.Account) error {
	q := fmt.Sprintf(`INSERT INTO auth_accounts
		(id, user_id, provider, provider_account_id, access_token, refresh_token, expires_at, created_at, updated_at)
		VALUES (%s)`,
		s.placeholders(9))

	_, err := s.db.ExecContext(ctx, q,
		a.ID, a.UserID, a.Provider, a.ProviderAccountID,
		a.AccessToken, a.RefreshToken,
		s.timePtrVal(a.ExpiresAt),
		s.timeVal(a.CreatedAt), s.timeVal(a.UpdatedAt),
	)
	if s.isConflict(err) {
		return fmt.Errorf("auth: %w", auth.ErrConflict)
	}
	return err
}

// AccountsByUser returns all accounts for the given userID.
// Returns an empty slice (not ErrNotFound) when the user has no accounts.
func (s *Store) AccountsByUser(ctx context.Context, userID string) ([]*auth.Account, error) {
	q := fmt.Sprintf(`SELECT id, user_id, provider, provider_account_id,
		access_token, refresh_token, expires_at, created_at, updated_at
		FROM auth_accounts WHERE user_id = %s`, s.ph(1))

	rows, err := s.db.QueryContext(ctx, q, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*auth.Account
	for rows.Next() {
		a, err := s.scanAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if out == nil {
		out = []*auth.Account{}
	}
	return out, nil
}

func (s *Store) scanAccount(rows *sql.Rows) (*auth.Account, error) {
	var a auth.Account
	var expiresRaw, createdRaw, updatedRaw interface{}

	err := rows.Scan(
		&a.ID, &a.UserID, &a.Provider, &a.ProviderAccountID,
		&a.AccessToken, &a.RefreshToken,
		&expiresRaw, &createdRaw, &updatedRaw,
	)
	if err != nil {
		return nil, err
	}

	a.ExpiresAt, err = scanTimePtr(expiresRaw)
	if err != nil {
		return nil, err
	}
	if a.CreatedAt, err = scanTime(createdRaw); err != nil {
		return nil, err
	}
	if a.UpdatedAt, err = scanTime(updatedRaw); err != nil {
		return nil, err
	}
	return &a, nil
}

// LinkAccount is an alias of CreateAccount documenting provider-linking intent.
func (s *Store) LinkAccount(ctx context.Context, a *auth.Account) error {
	return s.CreateAccount(ctx, a)
}

// UnlinkAccount removes the account with the given accountID.
// Returns ErrNotFound if absent.
func (s *Store) UnlinkAccount(ctx context.Context, accountID string) error {
	q := fmt.Sprintf(`DELETE FROM auth_accounts WHERE id = %s`, s.ph(1))
	res, err := s.db.ExecContext(ctx, q, accountID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("auth: %w", auth.ErrNotFound)
	}
	return nil
}

// --- Verification ---

// CreateVerification persists v.
func (s *Store) CreateVerification(ctx context.Context, v *auth.Verification) error {
	q := fmt.Sprintf(`INSERT INTO auth_verifications
		(id, user_id, kind, token, expires_at, created_at)
		VALUES (%s)`,
		s.placeholders(6))

	_, err := s.db.ExecContext(ctx, q,
		v.ID, v.UserID, v.Kind, v.Token,
		s.timeVal(v.ExpiresAt), s.timeVal(v.CreatedAt),
	)
	return err
}

// VerificationByCode returns the Verification identified by code (Token).
// Returns ErrNotFound if absent.
func (s *Store) VerificationByCode(ctx context.Context, code string) (*auth.Verification, error) {
	q := fmt.Sprintf(`SELECT id, user_id, kind, token, expires_at, created_at
		FROM auth_verifications WHERE token = %s`, s.ph(1))

	row := s.db.QueryRowContext(ctx, q, code)

	var v auth.Verification
	var expiresRaw, createdRaw interface{}

	err := row.Scan(&v.ID, &v.UserID, &v.Kind, &v.Token, &expiresRaw, &createdRaw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("auth: %w", auth.ErrNotFound)
	}
	if err != nil {
		return nil, err
	}

	if v.ExpiresAt, err = scanTime(expiresRaw); err != nil {
		return nil, err
	}
	if v.CreatedAt, err = scanTime(createdRaw); err != nil {
		return nil, err
	}
	return &v, nil
}

// ConsumeVerification deletes the Verification identified by code.
// Returns ErrNotFound if absent.
func (s *Store) ConsumeVerification(ctx context.Context, code string) error {
	q := fmt.Sprintf(`DELETE FROM auth_verifications WHERE token = %s`, s.ph(1))
	res, err := s.db.ExecContext(ctx, q, code)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("auth: %w", auth.ErrNotFound)
	}
	return nil
}
