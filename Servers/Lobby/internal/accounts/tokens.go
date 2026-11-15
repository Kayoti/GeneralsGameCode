// This file holds the random values and the token checks of the accounts package.

package accounts

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// Sizes of the random values, in bytes (SERVICE.md section 4). A login code
// has its own size: the documents give none, and 16 bytes is enough for a code
// that lives five minutes.
const (
	loginCodeBytes = 16
	tokenBytes     = 32
)

// tokenHexLength is the length of a session or refresh token in hexadecimal.
const tokenHexLength = 2 * tokenBytes

// randomHex returns byteCount random bytes in hexadecimal.
func randomHex(byteCount int) (string, error) {
	buffer := make([]byte, byteCount)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}
	return hex.EncodeToString(buffer), nil
}

// newRefreshToken returns a random refresh token in hexadecimal.
func newRefreshToken() (string, error) {
	token, err := randomHex(tokenBytes)
	if err != nil {
		return "", fmt.Errorf("make refresh token: %w", err)
	}
	return token, nil
}

// hashToken returns the SHA-256 of a token in hexadecimal. Refresh tokens are
// stored only in this form (SERVICE.md section 4).
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// isTokenFormat reports whether text has the shape of a session or refresh
// token: tokenHexLength lowercase hexadecimal digits. Anything else is
// refused before it is looked up.
func isTokenFormat(text string) bool {
	if len(text) != tokenHexLength {
		return false
	}
	for _, character := range text {
		isDigit := character >= '0' && character <= '9'
		isLetter := character >= 'a' && character <= 'f'
		if !isDigit && !isLetter {
			return false
		}
	}
	return true
}
