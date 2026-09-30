// Package pluginversion implements the strict MAJOR.MINOR.PATCH release format.
package pluginversion

import (
	"strconv"
	"strings"
)

type Version struct{ major, minor, patch uint64 }

// Parse parses the strict MAJOR.MINOR.PATCH versions used by first-party plugins.
func Parse(value string) (Version, bool) {
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return Version{}, false
	}
	values := make([]uint64, 3)
	for index, part := range parts {
		if part == "" || (len(part)>1 && part[0]=='0') || strings.IndexFunc(part,func(r rune) bool { return r<'0' || r>'9' })>=0 {
			return Version{}, false
		}
		parsed, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return Version{}, false
		}
		values[index] = parsed
	}
	return Version{major: values[0], minor: values[1], patch: values[2]}, true
}

// Compare compares left and right and returns -1, 0, or 1.
func Compare(left, right Version) int {
	for _, pair := range [][2]uint64{{left.major, right.major}, {left.minor, right.minor}, {left.patch, right.patch}} {
		if pair[0] < pair[1] {
			return -1
		}
		if pair[0] > pair[1] {
			return 1
		}
	}
	return 0
}

// Newer reports whether both versions are valid and candidate is newer.
func Newer(candidate, installed string) bool {
	a, ok := Parse(candidate)
	if !ok {
		return false
	}
	b, ok := Parse(installed)
	return ok && Compare(a, b) > 0
}
