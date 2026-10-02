// Package pluginversion implements the strict MAJOR.MINOR.PATCH release format.
package pluginversion

import (
	"cmp"
	"strconv"
	"strings"
)

// Version is a parsed strict MAJOR.MINOR.PATCH plugin version.
type Version struct {
	// major is the semantic-version major component.
	major uint64
	// minor is the semantic-version minor component.
	minor uint64
	// patch is the semantic-version patch component.
	patch uint64
}

// Parse parses the strict MAJOR.MINOR.PATCH versions used by first-party plugins.
func Parse(value string) (Version, bool) {
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return Version{}, false
	}

	var values [3]uint64
	for index, part := range parts {
		value, ok := parsePart(part)
		if !ok {
			return Version{}, false
		}
		values[index] = value
	}

	return Version{
		major: values[0],
		minor: values[1],
		patch: values[2],
	}, true
}

// parsePart parses a single part of a version.
func parsePart(part string) (uint64, bool) {
	if len(part) > 1 && part[0] == '0' {
		return 0, false
	}

	value, err := strconv.ParseUint(part, 10, 64)
	return value, err == nil
}

// Compare compares left and right and returns -1, 0, or 1.
func Compare(left, right Version) int {
	if result := cmp.Compare(left.major, right.major); result != 0 {
		return result
	}
	if result := cmp.Compare(left.minor, right.minor); result != 0 {
		return result
	}
	return cmp.Compare(left.patch, right.patch)
}

// Newer reports whether both versions are valid and candidate is newer.
func Newer(candidate, installed string) bool {
	candidateVersion, ok := Parse(candidate)
	if !ok {
		return false
	}

	installedVersion, ok := Parse(installed)
	return ok && Compare(candidateVersion, installedVersion) > 0
}
