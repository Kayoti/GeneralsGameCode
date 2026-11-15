// This file holds the display name rules of SERVICE.md section 4 (name change).

package accounts

import (
	"fmt"
	"strings"
)

// Limits of a display name, in characters (SERVICE.md section 4).
const (
	minNameLength = 3
	maxNameLength = 20
)

// NameRefusal is returned when a requested display name breaks a rule. Reason
// is a sentence for the player, such as "names have 3 to 20 characters".
type NameRefusal struct {
	Reason string
}

func (refusal *NameRefusal) Error() string {
	return "display name refused: " + refusal.Reason
}

// DisplayName returns the stored display name of an account. The bool is
// false when no account has the id.
func (service *Service) DisplayName(userID int64) (string, bool, error) {
	return service.db.DisplayName(userID)
}

// ChangeDisplayName sets the display name of an account after checking the
// rules of SERVICE.md section 4. It returns a *NameRefusal when a rule is
// broken. Any other error is a failure of the service.
func (service *Service) ChangeDisplayName(userID int64, name string) error {
	if refusal := checkNameRules(userID, name); refusal != nil {
		return refusal
	}
	service.nameMu.Lock()
	defer service.nameMu.Unlock()
	renamed, err := service.db.RenameAccount(userID, name)
	if err != nil {
		return err
	}
	if !renamed {
		return &NameRefusal{Reason: "that name is already taken"}
	}
	return nil
}

// checkNameRules returns the first rule that name breaks, or nil. The only
// name of the form Player followed by digits that is allowed is the account's
// own default name.
func checkNameRules(userID int64, name string) *NameRefusal {
	if len(name) < minNameLength || len(name) > maxNameLength {
		return &NameRefusal{Reason: fmt.Sprintf("names have %d to %d characters", minNameLength, maxNameLength)}
	}
	for _, character := range name {
		if !isNameCharacter(character) {
			return &NameRefusal{Reason: "names may only use letters, digits, spaces, _ and -"}
		}
	}
	if strings.HasPrefix(name, " ") || strings.HasSuffix(name, " ") {
		return &NameRefusal{Reason: "names cannot start or end with a space"}
	}
	if isDefaultNameForm(name) && name != defaultDisplayName(userID) {
		return &NameRefusal{Reason: "names of the form " + namePrefix + " followed by digits are reserved"}
	}
	return nil
}

// isNameCharacter reports whether a character may appear in a display name.
// Letters are the ASCII letters, so the case-insensitive uniqueness check of
// the database is exact.
func isNameCharacter(character rune) bool {
	switch {
	case character >= 'a' && character <= 'z', character >= 'A' && character <= 'Z':
		return true
	case character >= '0' && character <= '9':
		return true
	}
	return character == ' ' || character == '_' || character == '-'
}

// isDefaultNameForm reports whether name is namePrefix followed only by digits.
func isDefaultNameForm(name string) bool {
	digits, found := strings.CutPrefix(name, namePrefix)
	return found && allDigits(digits)
}

// allDigits reports whether text is not empty and holds only the digits 0 to 9.
func allDigits(text string) bool {
	for _, character := range text {
		if character < '0' || character > '9' {
			return false
		}
	}
	return text != ""
}
