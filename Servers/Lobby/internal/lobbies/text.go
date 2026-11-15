// This file holds the cleaning of player text that the lobby and the
// WebSocket share (SERVICE.md section 3).

package lobbies

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// CleanText prepares player text (SERVICE.md section 3): control characters
// are removed, the ends are trimmed, and at most maxCharacters characters are
// kept. Invalid UTF-8 has already been replaced when the JSON was decoded.
func CleanText(text string, maxCharacters int) string {
	withoutControls := strings.Map(func(character rune) rune {
		if unicode.IsControl(character) {
			return -1
		}
		return character
	}, text)
	trimmed := strings.TrimSpace(withoutControls)
	if utf8.RuneCountInString(trimmed) <= maxCharacters {
		return trimmed
	}
	kept := []rune(trimmed)[:maxCharacters]
	return string(kept)
}
