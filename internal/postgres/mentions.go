package postgres

import (
	"strings"

	"github.com/kumbuka-me/kumbuka/pkg/ascii"
)

// mentionedUsernames extracts distinct, lower-case mentions in source order. A mention begins at the start of text or after a non-word character. Names contain ASCII letters, digits, underscores, dots and hyphens.
func mentionedUsernames(text string) []string {
	var usernames []string
	seen := map[string]bool{}
	consumed := 0
	for offset := 0; offset < len(text); {
		next := strings.IndexByte(text[offset:], '@')
		if next < 0 {
			break
		}
		start := offset + next
		if end, inside := mentionMacroEnd(text, start); inside {
			offset = end
			continue
		}
		// The preceding boundary must not belong to an already-consumed mention.
		validBoundary := start == 0 || start > consumed && !mentionWordByte(text[start-1])
		offset = start + 1
		if !validBoundary {
			continue
		}
		end := offset
		for end < len(text) && mentionNameByte(text[end]) {
			end++
		}
		if end == offset {
			continue
		}
		username := strings.ToLower(text[offset:end])
		if !seen[username] {
			seen[username] = true
			usernames = append(usernames, username)
		}
		offset = end
		consumed = end
	}
	return usernames
}

// mentionMacroEnd reports whether an at sign is inside a plugin-style {{...}} declaration and where scanning should resume.
func mentionMacroEnd(text string, at int) (int, bool) {
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

// mentionNameByte reports whether value can occur inside a username mention.
func mentionNameByte(value byte) bool {
	return mentionWordByte(value) || value == '.' || value == '-'
}

// mentionWordByte defines the ASCII word characters used at mention boundaries.
func mentionWordByte(value byte) bool {
	return ascii.IsAlphanumeric(rune(value)) || value == '_'
}
