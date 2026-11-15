// Tests of the account and refresh token queries.

package store

import (
	"fmt"
	"path/filepath"
	"testing"
)

// playerName is the name function the tests use, as in the accounts package.
func playerName(userID int64) string {
	return fmt.Sprintf("Player%d", userID)
}

func TestFirstAccountGetsFirstUserID(t *testing.T) {
	store := openTestStore(t, filepath.Join(t.TempDir(), "lobby.db"))
	defer store.Close()

	userID, err := store.CreateAccount("hash-a", playerName)
	if err != nil {
		t.Fatalf("CreateAccount returned error: %v", err)
	}
	if userID != firstUserID {
		t.Errorf("user id = %d, want %d", userID, firstUserID)
	}
}

func TestSecondAccountGetsNextUserID(t *testing.T) {
	store := openTestStore(t, filepath.Join(t.TempDir(), "lobby.db"))
	defer store.Close()

	if _, err := store.CreateAccount("hash-a", playerName); err != nil {
		t.Fatalf("first CreateAccount returned error: %v", err)
	}
	userID, err := store.CreateAccount("hash-b", playerName)
	if err != nil {
		t.Fatalf("second CreateAccount returned error: %v", err)
	}
	if userID != firstUserID+1 {
		t.Errorf("user id = %d, want %d", userID, firstUserID+1)
	}
}

func TestCreatedAccountHasItsName(t *testing.T) {
	store := openTestStore(t, filepath.Join(t.TempDir(), "lobby.db"))
	defer store.Close()

	userID, err := store.CreateAccount("hash-a", playerName)
	if err != nil {
		t.Fatalf("CreateAccount returned error: %v", err)
	}
	name, found, err := store.DisplayName(userID)
	if err != nil {
		t.Fatalf("DisplayName returned error: %v", err)
	}
	if !found || name != "Player1000" {
		t.Errorf("DisplayName = %q, %v; want Player1000, true", name, found)
	}
}

func TestDisplayNameOfUnknownAccountIsNotFound(t *testing.T) {
	store := openTestStore(t, filepath.Join(t.TempDir(), "lobby.db"))
	defer store.Close()

	if _, found, err := store.DisplayName(999999); err != nil || found {
		t.Errorf("DisplayName = found %v, err %v; want not found, no error", found, err)
	}
}

func TestRefreshTokenHashFindsItsAccount(t *testing.T) {
	store := openTestStore(t, filepath.Join(t.TempDir(), "lobby.db"))
	defer store.Close()

	userID, err := store.CreateAccount("hash-a", playerName)
	if err != nil {
		t.Fatalf("CreateAccount returned error: %v", err)
	}
	found, ok, err := store.UserIDForTokenHash("hash-a")
	if err != nil || !ok || found != userID {
		t.Errorf("UserIDForTokenHash = %d, %v, %v; want %d, true, nil", found, ok, err, userID)
	}
	if _, ok, err := store.UserIDForTokenHash("hash-unknown"); err != nil || ok {
		t.Errorf("unknown hash found = %v, err %v; want not found, no error", ok, err)
	}
}

func TestDeleteRefreshTokenKeepsAccount(t *testing.T) {
	store := openTestStore(t, filepath.Join(t.TempDir(), "lobby.db"))
	defer store.Close()

	userID, err := store.CreateAccount("hash-a", playerName)
	if err != nil {
		t.Fatalf("CreateAccount returned error: %v", err)
	}
	if err := store.DeleteRefreshToken(userID); err != nil {
		t.Fatalf("DeleteRefreshToken returned error: %v", err)
	}
	if _, ok, err := store.UserIDForTokenHash("hash-a"); err != nil || ok {
		t.Errorf("deleted token still found = %v, err %v", ok, err)
	}
	if _, found, err := store.DisplayName(userID); err != nil || !found {
		t.Errorf("account after token delete: found %v, err %v; want found", found, err)
	}
}

func TestAccountSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lobby.db")
	first := openTestStore(t, path)
	userID, err := first.CreateAccount("hash-a", playerName)
	if err != nil {
		t.Fatalf("CreateAccount returned error: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	second := openTestStore(t, path)
	defer second.Close()
	found, ok, err := second.UserIDForTokenHash("hash-a")
	if err != nil || !ok || found != userID {
		t.Errorf("after restart UserIDForTokenHash = %d, %v, %v; want %d, true, nil", found, ok, err, userID)
	}
	nextID, err := second.CreateAccount("hash-b", playerName)
	if err != nil {
		t.Fatalf("CreateAccount after restart returned error: %v", err)
	}
	if nextID != userID+1 {
		t.Errorf("user id after restart = %d, want %d", nextID, userID+1)
	}
}
