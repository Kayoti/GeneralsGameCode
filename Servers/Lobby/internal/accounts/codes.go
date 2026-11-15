// This file holds the login codes of SERVICE.md section 4.

package accounts

import (
	"fmt"
	"time"
)

// loginCodeLifetime is how long a login code is valid (SERVICE.md section 4).
const loginCodeLifetime = 5 * time.Minute

// maxUnusedLoginCodesPerAddress is how many login codes one address may hold
// at once (SERVICE.md section 4).
const maxUnusedLoginCodesPerAddress = 20

// loginCode is an issued code that has not been used yet.
type loginCode struct {
	address   string
	expiresAt time.Time
}

// IssueLoginCode returns a new login code for address. It returns
// ErrTooManyLoginCodes when the address already holds
// maxUnusedLoginCodesPerAddress unused codes.
func (service *Service) IssueLoginCode(address string) (string, error) {
	code, err := newLoginCode()
	if err != nil {
		return "", err
	}

	service.mu.Lock()
	defer service.mu.Unlock()
	now := service.now()
	service.pruneLoginCodes(now)
	if service.unusedLoginCodes(address) >= maxUnusedLoginCodesPerAddress {
		return "", ErrTooManyLoginCodes
	}
	service.loginCodes[code] = loginCode{address: address, expiresAt: now.Add(loginCodeLifetime)}
	return code, nil
}

// pruneLoginCodes removes the codes that have expired. The caller holds the lock.
func (service *Service) pruneLoginCodes(now time.Time) {
	for code, entry := range service.loginCodes {
		if !now.Before(entry.expiresAt) {
			delete(service.loginCodes, code)
		}
	}
}

// unusedLoginCodes counts the codes held for address. A used code is removed
// when it is used, so every code held is unused. The caller holds the lock.
func (service *Service) unusedLoginCodes(address string) int {
	count := 0
	for _, entry := range service.loginCodes {
		if entry.address == address {
			count++
		}
	}
	return count
}

// newLoginCode returns a random login code in hexadecimal.
func newLoginCode() (string, error) {
	code, err := randomHex(loginCodeBytes)
	if err != nil {
		return "", fmt.Errorf("make login code: %w", err)
	}
	return code, nil
}
