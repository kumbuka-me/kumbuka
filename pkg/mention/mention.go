// Package mention finds Kumbuka @username references in authored text.
package mention

import (
	"strings"

	"github.com/kumbuka-me/kumbuka/pkg/ascii"
)

// Range identifies one @username reference by byte offsets in the source.
type Range struct {
	// Start is the byte offset of the leading @ character.
	Start int
	// End is the byte offset immediately after the username.
	End int
	// Username is the normalized lower-case username without the leading @.
	Username string
}

// Ranges returns mention references in source order. A mention begins at the
// start of text or after a non-word character. Names contain ASCII letters,
// digits, underscores, dots, and hyphens. Plugin-style {{...}} declarations
// are ignored so option values such as task assignees are not treated as page
// mentions.
func Ranges(text string) []Range {
	var result []Range
	consumed := 0
	for offset := 0; offset < len(text); {
		next := strings.IndexByte(text[offset:], '@')
		if next < 0 {
			break
		}
		start := offset + next
		if end, inside := macroEnd(text, start); inside {
			offset = end
			continue
		}

		// The preceding boundary must not belong to an already-consumed mention.
		validBoundary := start == 0 || start > consumed && !wordByte(text[start-1])
		offset = start + 1
		if !validBoundary {
			continue
		}

		end := offset
		for end < len(text) && nameByte(text[end]) {
			end++
		}
		if end == offset {
			continue
		}

		result = append(result, Range{
			Start:    start,
			End:      end,
			Username: strings.ToLower(text[offset:end]),
		})
		offset = end
		consumed = end
	}
	return result
}

// Usernames returns distinct normalized usernames in mention order.
func Usernames(text string) []string {
	ranges := Ranges(text)
	usernames := make([]string, 0, len(ranges))
	seen := make(map[string]bool, len(ranges))
	for _, item := range ranges {
		if seen[item.Username] {
			continue
		}
		seen[item.Username] = true
		usernames = append(usernames, item.Username)
	}
	return usernames
}

// macroEnd reports whether an at sign is inside a plugin-style {{...}}
// declaration and where scanning should resume.
func macroEnd(text string, at int) (int, bool) {
	open := strings.LastIndex(text[:at], "{{")
	if open < 0 || strings.LastIndex(text[:at], "}}") > open {
		return 0, false
	}
	closeOffset := strings.Index(text[at:], "}}")
	if closeOffset < 0 {
		return len(text), true
	}
	return at + closeOffset + 2, true
}

// nameByte reports whether value can occur inside a username mention.
func nameByte(value byte) bool {
	return wordByte(value) || value == '.' || value == '-'
}

// wordByte defines the ASCII word characters used at mention boundaries.
func wordByte(value byte) bool {
	return ascii.IsAlphanumeric(rune(value)) || value == '_'
}
