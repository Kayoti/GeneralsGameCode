// Package store holds the lobby service's database (SERVICE.md section 1).
//
// The service keeps accounts and tokens in SQLite. Lobbies, rooms and
// connections live in memory and are not stored here. All SQL uses parameters.
package store

import (
	"database/sql"
	"errors"
	"fmt"

	// Registers the "sqlite" database driver. The driver is pure Go (SERVICE.md section 1).
	_ "modernc.org/sqlite"
)

// driverName is the name the modernc.org/sqlite package registers its driver under.
const driverName = "sqlite"

// schemaVersion is the version of the tables defined in schemaStatements.
// A database written with another version is refused at start.
const schemaVersion = 1

// schemaStatements is the one place where the tables are defined. Every table
// is created with IF NOT EXISTS, so a restart does not change an existing database.
var schemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS schema_version (
		version INTEGER NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS accounts (
		user_id INTEGER PRIMARY KEY,
		display_name TEXT NOT NULL
	)`,
	// One refresh token per account. Only its SHA-256 is stored (SERVICE.md section 4).
	`CREATE TABLE IF NOT EXISTS refresh_tokens (
		user_id INTEGER PRIMARY KEY REFERENCES accounts(user_id),
		token_hash TEXT NOT NULL UNIQUE
	)`,
}

// Store is an open database.
type Store struct {
	db *sql.DB
}

// Open opens the SQLite file at path, creates the tables that do not exist
// yet, and checks the schema version. A file written by another version of
// the service is refused.
func Open(path string) (*Store, error) {
	db, err := sql.Open(driverName, path)
	if err != nil {
		return nil, fmt.Errorf("open database %q: %w", path, err)
	}
	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("prepare database %q: %w", path, err)
	}
	return store, nil
}

// Close closes the database.
func (store *Store) Close() error {
	return store.db.Close()
}

// migrate creates the tables and stores the schema version when the database
// is new, then checks that the stored version is the one this code knows.
func (store *Store) migrate() error {
	for _, statement := range schemaStatements {
		if _, err := store.db.Exec(statement); err != nil {
			return fmt.Errorf("create table: %w", err)
		}
	}
	if _, err := store.db.Exec(
		`INSERT INTO schema_version (version)
		 SELECT ? WHERE NOT EXISTS (SELECT 1 FROM schema_version)`,
		schemaVersion,
	); err != nil {
		return fmt.Errorf("write schema version: %w", err)
	}
	return store.checkVersion()
}

func (store *Store) checkVersion() error {
	var stored int
	err := store.db.QueryRow(`SELECT version FROM schema_version`).Scan(&stored)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("schema version row is missing")
	}
	if err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if stored != schemaVersion {
		return fmt.Errorf("schema version is %d, this service needs %d", stored, schemaVersion)
	}
	return nil
}
