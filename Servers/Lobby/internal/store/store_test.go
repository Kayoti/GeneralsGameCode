// Tests of the database opening and the schema version check.

package store

import (
	"path/filepath"
	"strings"
	"testing"
)

func openTestStore(t *testing.T, path string) *Store {
	t.Helper()
	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open(%q) returned error: %v", path, err)
	}
	return store
}

func TestOpenCreatesVersionRow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lobby.db")
	store := openTestStore(t, path)
	defer store.Close()

	var stored int
	if err := store.db.QueryRow(`SELECT version FROM schema_version`).Scan(&stored); err != nil {
		t.Fatalf("reading version row: %v", err)
	}
	if stored != schemaVersion {
		t.Errorf("stored version = %d, want %d", stored, schemaVersion)
	}
}

func TestReopenKeepsOneVersionRow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lobby.db")
	first := openTestStore(t, path)
	if err := first.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	second := openTestStore(t, path)
	defer second.Close()

	var rows int
	if err := second.db.QueryRow(`SELECT COUNT(*) FROM schema_version`).Scan(&rows); err != nil {
		t.Fatalf("counting version rows: %v", err)
	}
	if rows != 1 {
		t.Errorf("version rows = %d, want 1", rows)
	}
}

func TestOpenRefusesOtherSchemaVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lobby.db")
	store := openTestStore(t, path)
	if _, err := store.db.Exec(`UPDATE schema_version SET version = ?`, schemaVersion+1); err != nil {
		t.Fatalf("changing version row: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	reopened, err := Open(path)
	if err == nil {
		reopened.Close()
		t.Fatal("expected Open to refuse a database with another schema version")
	}
	if !strings.Contains(err.Error(), "schema version") {
		t.Errorf("error does not name the schema version: %v", err)
	}
}
