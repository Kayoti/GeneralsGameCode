// This file holds the account and refresh token queries (SERVICE.md section 4).

package store

import (
	"database/sql"
	"errors"
	"fmt"
)

// firstUserID is the user id of the first account (SERVICE.md section 3).
const firstUserID = 1000

// CreateAccount stores a new account and its refresh token in one transaction.
// The user id is one more than the largest id stored, or firstUserID when
// there is no account yet. displayNameFor returns the name for the id that was
// chosen. tokenHash is the SHA-256 of the refresh token, in hexadecimal.
// The caller must not run two creations at once, because the id is chosen
// before it is stored.
func (store *Store) CreateAccount(tokenHash string, displayNameFor func(userID int64) string) (int64, error) {
	tx, err := store.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin account transaction: %w", err)
	}
	defer tx.Rollback()

	var userID int64
	if err := tx.QueryRow(
		`SELECT COALESCE(MAX(user_id) + 1, ?) FROM accounts`,
		firstUserID,
	).Scan(&userID); err != nil {
		return 0, fmt.Errorf("choose next user id: %w", err)
	}
	if _, err := tx.Exec(
		`INSERT INTO accounts (user_id, display_name) VALUES (?, ?)`,
		userID, displayNameFor(userID),
	); err != nil {
		return 0, fmt.Errorf("insert account %d: %w", userID, err)
	}
	if _, err := tx.Exec(
		`INSERT INTO refresh_tokens (user_id, token_hash) VALUES (?, ?)`,
		userID, tokenHash,
	); err != nil {
		return 0, fmt.Errorf("insert refresh token of account %d: %w", userID, err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit account %d: %w", userID, err)
	}
	return userID, nil
}

// DisplayName returns the display name of an account. The bool is false when
// no account has the id.
func (store *Store) DisplayName(userID int64) (string, bool, error) {
	var name string
	err := store.db.QueryRow(
		`SELECT display_name FROM accounts WHERE user_id = ?`,
		userID,
	).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read name of account %d: %w", userID, err)
	}
	return name, true, nil
}

// UserIDForTokenHash returns the account that owns a refresh token, given its
// SHA-256 in hexadecimal. The bool is false when no account has that token.
func (store *Store) UserIDForTokenHash(tokenHash string) (int64, bool, error) {
	var userID int64
	err := store.db.QueryRow(
		`SELECT user_id FROM refresh_tokens WHERE token_hash = ?`,
		tokenHash,
	).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("look up refresh token: %w", err)
	}
	return userID, true, nil
}

// DeleteRefreshToken removes the refresh token of an account. The account row
// is kept (SERVICE.md section 4).
func (store *Store) DeleteRefreshToken(userID int64) error {
	if _, err := store.db.Exec(
		`DELETE FROM refresh_tokens WHERE user_id = ?`,
		userID,
	); err != nil {
		return fmt.Errorf("delete refresh token of account %d: %w", userID, err)
	}
	return nil
}
