// Tests of the login rules: codes, sessions, refresh tokens and the creation limit.

package accounts

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"servers-lobby/internal/store"
)

// testAddress is a public address, so the creation limit applies to it.
const testAddress = "203.0.113.5"

// loopbackAddress is never limited.
const loopbackAddress = "127.0.0.1"

// newTestService returns a Service on a new database file, with the limit
// given, and a clock the test moves forward by setting *clock.
func newTestService(t *testing.T, limit int) (*Service, *time.Time) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "lobby.db"))
	if err != nil {
		t.Fatalf("store.Open returned error: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	clock := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	service := New(db, limit)
	service.now = func() time.Time { return clock }
	return service, &clock
}

// loginWithNewAccount issues a code from address and uses it, returning the login.
func loginWithNewAccount(t *testing.T, service *Service, address string) Login {
	t.Helper()
	code, err := service.IssueLoginCode(address)
	if err != nil {
		t.Fatalf("IssueLoginCode returned error: %v", err)
	}
	login, err := service.CheckLoginCode(code, address)
	if err != nil {
		t.Fatalf("CheckLoginCode returned error: %v", err)
	}
	return login
}

func TestFirstLoginCreatesAccountWithDefaultName(t *testing.T) {
	service, _ := newTestService(t, 0)
	login := loginWithNewAccount(t, service, testAddress)

	if login.UserID != 1000 {
		t.Errorf("UserID = %d, want 1000", login.UserID)
	}
	if login.DisplayName != "Player1000" {
		t.Errorf("DisplayName = %q, want Player1000", login.DisplayName)
	}
	if !isTokenFormat(login.SessionToken) || !isTokenFormat(login.RefreshToken) {
		t.Errorf("tokens do not have the token shape")
	}
	if userID, ok := service.UserForSession(login.SessionToken); !ok || userID != 1000 {
		t.Errorf("UserForSession = %d, %v; want 1000, true", userID, ok)
	}
}

func TestLaterLoginByRefreshTokenReturnsSameAccount(t *testing.T) {
	service, _ := newTestService(t, 0)
	first := loginWithNewAccount(t, service, testAddress)

	later, err := service.LoginWithRefreshToken(first.RefreshToken)
	if err != nil {
		t.Fatalf("LoginWithRefreshToken returned error: %v", err)
	}
	if later.UserID != first.UserID || later.DisplayName != first.DisplayName {
		t.Errorf("later login = %d %q, want %d %q", later.UserID, later.DisplayName, first.UserID, first.DisplayName)
	}
	if later.RefreshToken != first.RefreshToken {
		t.Errorf("refresh token changed on login by token")
	}
	if later.SessionToken == first.SessionToken {
		t.Errorf("login by token gave the same session token")
	}
}

func TestRefreshGivesNewSessionAndKeepsOldUntilExpiry(t *testing.T) {
	service, clock := newTestService(t, 0)
	login := loginWithNewAccount(t, service, testAddress)

	refreshed, err := service.RefreshSession(login.RefreshToken)
	if err != nil {
		t.Fatalf("RefreshSession returned error: %v", err)
	}
	if refreshed == login.SessionToken {
		t.Fatalf("RefreshSession returned the old session token")
	}
	*clock = clock.Add(14 * time.Minute)
	if _, ok := service.UserForSession(login.SessionToken); !ok {
		t.Errorf("old session expired before 15 minutes")
	}
	if userID, ok := service.UserForSession(refreshed); !ok || userID != login.UserID {
		t.Errorf("new session = %d, %v; want %d, true", userID, ok, login.UserID)
	}
}

func TestExpiredSessionIsRefused(t *testing.T) {
	service, clock := newTestService(t, 0)
	login := loginWithNewAccount(t, service, testAddress)

	*clock = clock.Add(sessionLifetime)
	if _, ok := service.UserForSession(login.SessionToken); ok {
		t.Errorf("session still valid at 15 minutes")
	}
}

func TestUnknownTokensAreRefused(t *testing.T) {
	service, _ := newTestService(t, 0)
	loginWithNewAccount(t, service, testAddress)
	unknownToken := "00000000000000000000000000000000000000000000000000000000000000ff"

	if _, err := service.LoginWithRefreshToken(unknownToken); !errors.Is(err, ErrUnknownRefreshToken) {
		t.Errorf("LoginWithRefreshToken unknown token error = %v, want ErrUnknownRefreshToken", err)
	}
	if _, err := service.RefreshSession(unknownToken); !errors.Is(err, ErrUnknownRefreshToken) {
		t.Errorf("RefreshSession unknown token error = %v, want ErrUnknownRefreshToken", err)
	}
	if _, ok := service.UserForSession(unknownToken); ok {
		t.Errorf("UserForSession accepted an unknown session token")
	}
}

func TestMalformedTokensAreRefusedWithoutLookup(t *testing.T) {
	service, _ := newTestService(t, 0)
	for _, token := range []string{"", "short", "ABCDEF" + strings.Repeat("0", 58), strings.Repeat("g", 64), strings.Repeat("0", 65)} {
		if _, err := service.LoginWithRefreshToken(token); !errors.Is(err, ErrUnknownRefreshToken) {
			t.Errorf("LoginWithRefreshToken(%q) error = %v, want ErrUnknownRefreshToken", token, err)
		}
		if _, ok := service.UserForSession(token); ok {
			t.Errorf("UserForSession(%q) accepted a malformed token", token)
		}
	}
}

func TestDeletedRefreshTokenIsRefused(t *testing.T) {
	service, _ := newTestService(t, 0)
	login := loginWithNewAccount(t, service, testAddress)

	if err := service.DeleteRefreshToken(login.UserID); err != nil {
		t.Fatalf("DeleteRefreshToken returned error: %v", err)
	}
	if _, err := service.LoginWithRefreshToken(login.RefreshToken); !errors.Is(err, ErrUnknownRefreshToken) {
		t.Errorf("login with deleted token error = %v, want ErrUnknownRefreshToken", err)
	}
}

func TestLoginCodeWorksOnce(t *testing.T) {
	service, _ := newTestService(t, 0)
	code, err := service.IssueLoginCode(testAddress)
	if err != nil {
		t.Fatalf("IssueLoginCode returned error: %v", err)
	}
	if _, err := service.CheckLoginCode(code, testAddress); err != nil {
		t.Fatalf("first CheckLoginCode returned error: %v", err)
	}
	if _, err := service.CheckLoginCode(code, testAddress); !errors.Is(err, ErrUnknownLoginCode) {
		t.Errorf("second CheckLoginCode error = %v, want ErrUnknownLoginCode", err)
	}
}

func TestLoginCodeExpiresAfterFiveMinutes(t *testing.T) {
	service, clock := newTestService(t, 0)
	code, err := service.IssueLoginCode(testAddress)
	if err != nil {
		t.Fatalf("IssueLoginCode returned error: %v", err)
	}
	*clock = clock.Add(loginCodeLifetime)
	if _, err := service.CheckLoginCode(code, testAddress); !errors.Is(err, ErrUnknownLoginCode) {
		t.Errorf("expired code error = %v, want ErrUnknownLoginCode", err)
	}
}

func TestUnknownLoginCodeIsRefused(t *testing.T) {
	service, _ := newTestService(t, 0)
	if _, err := service.CheckLoginCode("never-issued", testAddress); !errors.Is(err, ErrUnknownLoginCode) {
		t.Errorf("unknown code error = %v, want ErrUnknownLoginCode", err)
	}
}

func TestAtMostTwentyUnusedLoginCodesPerAddress(t *testing.T) {
	service, _ := newTestService(t, 0)
	for issued := 0; issued < maxUnusedLoginCodesPerAddress; issued++ {
		if _, err := service.IssueLoginCode(testAddress); err != nil {
			t.Fatalf("code %d returned error: %v", issued+1, err)
		}
	}
	if _, err := service.IssueLoginCode(testAddress); !errors.Is(err, ErrTooManyLoginCodes) {
		t.Errorf("21st code error = %v, want ErrTooManyLoginCodes", err)
	}
	if _, err := service.IssueLoginCode("198.51.100.9"); err != nil {
		t.Errorf("other address refused: %v", err)
	}
}

func TestCreationLimitPerAddressPerHour(t *testing.T) {
	service, clock := newTestService(t, 2)
	loginWithNewAccount(t, service, testAddress)
	loginWithNewAccount(t, service, testAddress)

	code, err := service.IssueLoginCode(testAddress)
	if err != nil {
		t.Fatalf("IssueLoginCode returned error: %v", err)
	}
	if _, err := service.CheckLoginCode(code, testAddress); !errors.Is(err, ErrAccountLimit) {
		t.Fatalf("third account error = %v, want ErrAccountLimit", err)
	}
	// The refused code is not used up: it is still known, so it is refused for
	// the limit again and not as unknown.
	if _, err := service.CheckLoginCode(code, testAddress); !errors.Is(err, ErrAccountLimit) {
		t.Errorf("refused code second try error = %v, want ErrAccountLimit", err)
	}
	*clock = clock.Add(accountCreationWindow)
	loginWithNewAccount(t, service, testAddress)
}

func TestCreationLimitDoesNotApplyToLoopbackOrWhenZero(t *testing.T) {
	limited, _ := newTestService(t, 1)
	loginWithNewAccount(t, limited, loopbackAddress)
	loginWithNewAccount(t, limited, loopbackAddress)

	unlimited, _ := newTestService(t, 0)
	loginWithNewAccount(t, unlimited, testAddress)
	loginWithNewAccount(t, unlimited, testAddress)
}

func TestRestartKeepsAccountAndNameButNotSessions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lobby.db")
	first, err := store.Open(path)
	if err != nil {
		t.Fatalf("store.Open returned error: %v", err)
	}
	firstService := New(first, 0)
	login := loginWithNewAccount(t, firstService, testAddress)
	if err := first.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	second, err := store.Open(path)
	if err != nil {
		t.Fatalf("second store.Open returned error: %v", err)
	}
	defer second.Close()
	restarted := New(second, 0)
	if _, ok := restarted.UserForSession(login.SessionToken); ok {
		t.Errorf("session survived a restart")
	}
	again, err := restarted.LoginWithRefreshToken(login.RefreshToken)
	if err != nil {
		t.Fatalf("login after restart returned error: %v", err)
	}
	if again.UserID != login.UserID || again.DisplayName != login.DisplayName {
		t.Errorf("after restart = %d %q, want %d %q", again.UserID, again.DisplayName, login.UserID, login.DisplayName)
	}
}
