// Package accounts holds the login rules of the lobby service (SERVICE.md section 4):
// login codes, session tokens, refresh tokens and the account creation limit.
//
// Login codes and session tokens are held in memory and are lost on restart.
// Accounts and refresh tokens are stored through the store package.
package accounts

import (
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"servers-lobby/internal/store"
)

// namePrefix starts the default name of an account (SERVICE.md section 4).
const namePrefix = "Player"

// Errors returned by the login rules. The API maps them to the answers of
// SERVICE.md section 4. Their text never contains a token or a code.
var (
	ErrUnknownLoginCode    = errors.New("login code is unknown, used or expired")
	ErrTooManyLoginCodes   = errors.New("too many unused login codes for this address")
	ErrAccountLimit        = errors.New("account creation limit reached for this address")
	ErrUnknownRefreshToken = errors.New("refresh token is unknown")
)

// Login is the result of a successful login: the account and its tokens.
type Login struct {
	UserID       int64
	DisplayName  string
	SessionToken string
	RefreshToken string
}

// Service holds the login state and the database it writes accounts to.
// It is safe for concurrent use.
type Service struct {
	db                        *store.Store
	accountsPerAddressPerHour int
	now                       func() time.Time

	mu         sync.Mutex
	loginCodes map[string]loginCode
	sessions   map[string]session
	creations  map[string][]time.Time
}

// New returns a Service that stores accounts in db. accountsPerAddressPerHour
// is the account creation limit; 0 means no limit (SERVICE.md section 4).
func New(db *store.Store, accountsPerAddressPerHour int) *Service {
	return &Service{
		db:                        db,
		accountsPerAddressPerHour: accountsPerAddressPerHour,
		now:                       time.Now,
		loginCodes:                make(map[string]loginCode),
		sessions:                  make(map[string]session),
		creations:                 make(map[string][]time.Time),
	}
}

// CheckLoginCode uses a login code from address. A known, unused and unexpired
// code creates a new account and is used up. It returns ErrUnknownLoginCode
// for any other code and ErrAccountLimit when the address may not create an
// account now; in that case the code is left unused.
func (service *Service) CheckLoginCode(code string, address string) (Login, error) {
	sessionToken, err := newSessionToken()
	if err != nil {
		return Login{}, err
	}
	refreshToken, err := newRefreshToken()
	if err != nil {
		return Login{}, err
	}

	service.mu.Lock()
	defer service.mu.Unlock()
	now := service.now()
	entry, found := service.loginCodes[code]
	if !found || !now.Before(entry.expiresAt) {
		return Login{}, ErrUnknownLoginCode
	}
	if !service.creationAllowed(address, now) {
		return Login{}, ErrAccountLimit
	}
	userID, err := service.db.CreateAccount(hashToken(refreshToken), defaultDisplayName)
	if err != nil {
		return Login{}, fmt.Errorf("create account: %w", err)
	}
	delete(service.loginCodes, code)
	service.recordCreation(address, now)
	service.storeSession(sessionToken, userID, now)
	return Login{
		UserID:       userID,
		DisplayName:  defaultDisplayName(userID),
		SessionToken: sessionToken,
		RefreshToken: refreshToken,
	}, nil
}

// LoginWithRefreshToken logs in the account that owns a refresh token. The
// refresh token in the result is the one given. It returns
// ErrUnknownRefreshToken when no account has the token.
func (service *Service) LoginWithRefreshToken(refreshToken string) (Login, error) {
	userID, err := service.userForRefreshToken(refreshToken)
	if err != nil {
		return Login{}, err
	}
	name, found, err := service.db.DisplayName(userID)
	if err != nil {
		return Login{}, fmt.Errorf("read name of account %d: %w", userID, err)
	}
	if !found {
		return Login{}, fmt.Errorf("account %d of a refresh token has no row", userID)
	}
	sessionToken, err := service.startSession(userID)
	if err != nil {
		return Login{}, err
	}
	return Login{
		UserID:       userID,
		DisplayName:  name,
		SessionToken: sessionToken,
		RefreshToken: refreshToken,
	}, nil
}

// RefreshSession returns a new session token for the account that owns a
// refresh token. The old session token stays valid until it expires. It
// returns ErrUnknownRefreshToken when no account has the token.
func (service *Service) RefreshSession(refreshToken string) (string, error) {
	userID, err := service.userForRefreshToken(refreshToken)
	if err != nil {
		return "", err
	}
	return service.startSession(userID)
}

// DeleteRefreshToken removes the refresh token of an account, so the next
// start of the client creates a new account (SERVICE.md section 4).
func (service *Service) DeleteRefreshToken(userID int64) error {
	return service.db.DeleteRefreshToken(userID)
}

// UserForSession returns the account of a session token. The bool is false
// when the token is unknown, has the wrong shape, or has expired.
func (service *Service) UserForSession(sessionToken string) (int64, bool) {
	if !isTokenFormat(sessionToken) {
		return 0, false
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	entry, found := service.sessions[sessionToken]
	if !found || !service.now().Before(entry.expiresAt) {
		return 0, false
	}
	return entry.userID, true
}

// startSession issues and stores a new session token for userID.
func (service *Service) startSession(userID int64) (string, error) {
	sessionToken, err := newSessionToken()
	if err != nil {
		return "", err
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	service.storeSession(sessionToken, userID, service.now())
	return sessionToken, nil
}

// userForRefreshToken looks up the account of a refresh token by its hash.
// A token of the wrong shape is unknown without a database lookup.
func (service *Service) userForRefreshToken(refreshToken string) (int64, error) {
	if !isTokenFormat(refreshToken) {
		return 0, ErrUnknownRefreshToken
	}
	userID, found, err := service.db.UserIDForTokenHash(hashToken(refreshToken))
	if err != nil {
		return 0, fmt.Errorf("look up refresh token: %w", err)
	}
	if !found {
		return 0, ErrUnknownRefreshToken
	}
	return userID, nil
}

// defaultDisplayName is the name of a new account: Player followed by the user id.
func defaultDisplayName(userID int64) string {
	return namePrefix + strconv.FormatInt(userID, 10)
}
