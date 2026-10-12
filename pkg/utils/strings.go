package utils

import "strings"

// SplitCommaSeparatedValues trims and drops empty comma-separated values.
// Example: "one, two, ,three" becomes ["one", "two", "three"].
func SplitCommaSeparatedValues(value string) []string {
	values := make([]string, 0)
	for part := range strings.SplitSeq(value, ",") {
		if part = strings.TrimSpace(part); part != "" {
			values = append(values, part)
		}
	}
	return values
}
