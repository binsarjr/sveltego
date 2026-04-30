// Package pgx provides a native pgx v5 implementation of auth.Storage for
// PostgreSQL. It uses pgxpool for connection pooling and pgx's binary protocol
// for lower per-query overhead compared to the generic database/sql adapter.
package pgx

import (
	"context"
	"errors"
	"fmt"
	"time"

	pgxv5 "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/binsarjr/sveltego/auth"
)

// Store implements auth.Storage using a pgxpool.Pool. Construct via New.
type Store struct {
	pool *pgxpool.Pool
}

// New returns a *Store backed by pool. The caller is responsible for applying
// Schema (from auth/storage/sql) to the database before the first use.
func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Ensure Store satisfies auth.Storage at compile time.
var _ auth.Storage = (*Store)(nil)

// isConflict returns true when err is a Postgres uniqueness violation (SQLSTATE 23505).
func isConflict(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// --- User ---

// CreateUser persists u. Returns ErrConflict if the email is already taken.
func (s *Store) CreateUser(ctx context.Context, u *auth.User) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO auth_users
			(id, email, email_verified, name, image, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		u.ID, u.Email, u.EmailVerified, u.Name, u.Image,
		u.CreatedAt.UTC(), u.UpdatedAt.UTC(),
	)
	if isConflict(err) {
		return fmt.Errorf("auth: %w", auth.ErrConflict)
	}
	return err
}

// UserByID returns the user with the given id. Returns ErrNotFound if absent.
func (s *Store) UserByID(ctx context.Context, id string) (*auth.User, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, email, email_verified, name, image, created_at, updated_at
		FROM auth_users WHERE id = $1`, id)
	if err != nil {
		return nil, err
	}
	return pgxv5.CollectOneRow(rows, scanUser)
}

// UserByEmail returns the user with the given email. Returns ErrNotFound if absent.
func (s *Store) UserByEmail(ctx context.Context, email string) (*auth.User, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, email, email_verified, name, image, created_at, updated_at
		FROM auth_users WHERE email = $1`, email)
	if err != nil {
		return nil, err
	}
	return pgxv5.CollectOneRow(rows, scanUser)
}

func scanUser(row pgxv5.CollectableRow) (*auth.User, error) {
	var u auth.User
	err := row.Scan(
		&u.ID, &u.Email, &u.EmailVerified, &u.Name, &u.Image,
		&u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgxv5.ErrNoRows) {
			return nil, fmt.Errorf("auth: %w", auth.ErrNotFound)
		}
		return nil, err
	}
	u.CreatedAt = u.CreatedAt.UTC()
	u.UpdatedAt = u.UpdatedAt.UTC()
	return &u, nil
}

// UpdateUser persists changes to an existing user. Returns ErrNotFound if absent.
func (s *Store) UpdateUser(ctx context.Context, u *auth.User) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE auth_users
		SET email = $1, email_verified = $2, name = $3, image = $4, updated_at = $5
		WHERE id = $6`,
		u.Email, u.EmailVerified, u.Name, u.Image,
		u.UpdatedAt.UTC(), u.ID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("auth: %w", auth.ErrNotFound)
	}
	return nil
}

// DeleteUser removes the user and cascades to sessions, accounts, and
// verifications inside a single transaction. Returns ErrNotFound if absent.
func (s *Store) DeleteUser(ctx context.Context, id string) error {
	tx, err := s.pool.BeginTx(ctx, pgxv5.TxOptions{IsoLevel: pgxv5.ReadCommitted})
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	var dummy int
	err = tx.QueryRow(ctx, `SELECT 1 FROM auth_users WHERE id = $1`, id).Scan(&dummy)
	if errors.Is(err, pgxv5.ErrNoRows) {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("auth: %w", auth.ErrNotFound)
	}
	if err != nil {
		return err
	}

	for _, q := range []string{
		`DELETE FROM auth_sessions      WHERE user_id = $1`,
		`DELETE FROM auth_accounts      WHERE user_id = $1`,
		`DELETE FROM auth_verifications WHERE user_id = $1`,
		`DELETE FROM auth_users         WHERE id      = $1`,
	} {
		if _, err = tx.Exec(ctx, q, id); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

// --- Session ---

// CreateSession persists sess.
func (s *Store) CreateSession(ctx context.Context, sess *auth.Session) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO auth_sessions
			(id, user_id, token, expires_at, fresh_until, ip_address, user_agent, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		sess.ID, sess.UserID, sess.Token,
		sess.ExpiresAt.UTC(), sess.FreshUntil.UTC(),
		sess.IPAddress, sess.UserAgent,
		sess.CreatedAt.UTC(), sess.UpdatedAt.UTC(),
	)
	return err
}

// SessionByToken returns the session for the given token. Returns ErrNotFound
// if absent, or ErrSessionExpired if the session is past its ExpiresAt.
func (s *Store) SessionByToken(ctx context.Context, token string) (*auth.Session, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, token, expires_at, fresh_until,
		       ip_address, user_agent, created_at, updated_at
		FROM auth_sessions WHERE token = $1`, token)
	if err != nil {
		return nil, err
	}

	sess, err := pgxv5.CollectOneRow(rows, func(row pgxv5.CollectableRow) (*auth.Session, error) {
		var s auth.Session
		if scanErr := row.Scan(
			&s.ID, &s.UserID, &s.Token,
			&s.ExpiresAt, &s.FreshUntil,
			&s.IPAddress, &s.UserAgent,
			&s.CreatedAt, &s.UpdatedAt,
		); scanErr != nil {
			if errors.Is(scanErr, pgxv5.ErrNoRows) {
				return nil, fmt.Errorf("auth: %w", auth.ErrNotFound)
			}
			return nil, scanErr
		}
		s.ExpiresAt = s.ExpiresAt.UTC()
		s.FreshUntil = s.FreshUntil.UTC()
		s.CreatedAt = s.CreatedAt.UTC()
		s.UpdatedAt = s.UpdatedAt.UTC()
		return &s, nil
	})
	if err != nil {
		if errors.Is(err, pgxv5.ErrNoRows) {
			return nil, fmt.Errorf("auth: %w", auth.ErrNotFound)
		}
		return nil, err
	}

	if time.Now().After(sess.ExpiresAt) {
		return nil, fmt.Errorf("auth: %w", auth.ErrSessionExpired)
	}
	return sess, nil
}

// RefreshSession extends the expiry of the session identified by token.
// Returns ErrNotFound if absent.
func (s *Store) RefreshSession(ctx context.Context, token string, newExpiry time.Time) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE auth_sessions SET expires_at = $1, updated_at = $2 WHERE token = $3`,
		newExpiry.UTC(), time.Now().UTC(), token,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("auth: %w", auth.ErrNotFound)
	}
	return nil
}

// RevokeSession deletes the session identified by token. Idempotent.
func (s *Store) RevokeSession(ctx context.Context, token string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM auth_sessions WHERE token = $1`, token)
	return err
}

// RevokeAllSessions deletes every session belonging to userID. Idempotent.
func (s *Store) RevokeAllSessions(ctx context.Context, userID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM auth_sessions WHERE user_id = $1`, userID)
	return err
}

// --- Account ---

// CreateAccount persists a. Returns ErrConflict if the (provider, provider_account_id) pair is already taken.
func (s *Store) CreateAccount(ctx context.Context, a *auth.Account) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO auth_accounts
			(id, user_id, provider, provider_account_id,
			 access_token, refresh_token, expires_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		a.ID, a.UserID, a.Provider, a.ProviderAccountID,
		a.AccessToken, a.RefreshToken,
		timePtrUTC(a.ExpiresAt),
		a.CreatedAt.UTC(), a.UpdatedAt.UTC(),
	)
	if isConflict(err) {
		return fmt.Errorf("auth: %w", auth.ErrConflict)
	}
	return err
}

// AccountsByUser returns all accounts for userID.
// Returns an empty slice (not ErrNotFound) when the user has no accounts.
func (s *Store) AccountsByUser(ctx context.Context, userID string) ([]*auth.Account, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, provider, provider_account_id,
		       access_token, refresh_token, expires_at, created_at, updated_at
		FROM auth_accounts WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	accs, err := pgxv5.CollectRows(rows, func(row pgxv5.CollectableRow) (*auth.Account, error) {
		var a auth.Account
		if scanErr := row.Scan(
			&a.ID, &a.UserID, &a.Provider, &a.ProviderAccountID,
			&a.AccessToken, &a.RefreshToken,
			&a.ExpiresAt,
			&a.CreatedAt, &a.UpdatedAt,
		); scanErr != nil {
			return nil, scanErr
		}
		a.CreatedAt = a.CreatedAt.UTC()
		a.UpdatedAt = a.UpdatedAt.UTC()
		if a.ExpiresAt != nil {
			t := a.ExpiresAt.UTC()
			a.ExpiresAt = &t
		}
		return &a, nil
	})
	if err != nil {
		return nil, err
	}
	if accs == nil {
		accs = []*auth.Account{}
	}
	return accs, nil
}

// LinkAccount is an alias of CreateAccount documenting provider-linking intent.
func (s *Store) LinkAccount(ctx context.Context, a *auth.Account) error {
	return s.CreateAccount(ctx, a)
}

// UnlinkAccount removes the account with the given accountID.
// Returns ErrNotFound if absent.
func (s *Store) UnlinkAccount(ctx context.Context, accountID string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM auth_accounts WHERE id = $1`, accountID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("auth: %w", auth.ErrNotFound)
	}
	return nil
}

// --- Verification ---

// CreateVerification persists v.
func (s *Store) CreateVerification(ctx context.Context, v *auth.Verification) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO auth_verifications
			(id, user_id, kind, token, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		v.ID, v.UserID, v.Kind, v.Token,
		v.ExpiresAt.UTC(), v.CreatedAt.UTC(),
	)
	return err
}

// VerificationByCode returns the Verification identified by code (Token).
// Returns ErrNotFound if absent.
func (s *Store) VerificationByCode(ctx context.Context, code string) (*auth.Verification, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, kind, token, expires_at, created_at
		FROM auth_verifications WHERE token = $1`, code)
	if err != nil {
		return nil, err
	}

	v, err := pgxv5.CollectOneRow(rows, func(row pgxv5.CollectableRow) (*auth.Verification, error) {
		var ver auth.Verification
		if scanErr := row.Scan(
			&ver.ID, &ver.UserID, &ver.Kind, &ver.Token,
			&ver.ExpiresAt, &ver.CreatedAt,
		); scanErr != nil {
			if errors.Is(scanErr, pgxv5.ErrNoRows) {
				return nil, fmt.Errorf("auth: %w", auth.ErrNotFound)
			}
			return nil, scanErr
		}
		ver.ExpiresAt = ver.ExpiresAt.UTC()
		ver.CreatedAt = ver.CreatedAt.UTC()
		return &ver, nil
	})
	if err != nil {
		if errors.Is(err, pgxv5.ErrNoRows) {
			return nil, fmt.Errorf("auth: %w", auth.ErrNotFound)
		}
		return nil, err
	}
	return v, nil
}

// ConsumeVerification deletes the Verification identified by code.
// Returns ErrNotFound if absent.
func (s *Store) ConsumeVerification(ctx context.Context, code string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM auth_verifications WHERE token = $1`, code)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("auth: %w", auth.ErrNotFound)
	}
	return nil
}

// timePtrUTC converts *time.Time to UTC or nil for nullable timestamp columns.
func timePtrUTC(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}
