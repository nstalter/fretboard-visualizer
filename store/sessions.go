package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// CreateSession stores a session under tokenHash and purges expired ones while it is at it.
func (s *Store) CreateSession(ctx context.Context, tokenHash, sub, email string, expires time.Time) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, now()); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx,
			`INSERT INTO sessions (token_hash, sub, email, expires_at) VALUES (?, ?, ?, ?)`,
			tokenHash, sub, email, expires.Unix())
		return err
	})
}

// GetSession reports ok=false when the session is missing or expired.
func (s *Store) GetSession(ctx context.Context, tokenHash string) (sub, email string, ok bool, err error) {
	err = s.db.QueryRowContext(ctx,
		`SELECT sub, email FROM sessions WHERE token_hash = ? AND expires_at > ?`,
		tokenHash, now()).Scan(&sub, &email)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	return sub, email, true, nil
}

func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
	return err
}
