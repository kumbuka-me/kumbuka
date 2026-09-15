package httpresponse

import "strings"

// IsLocalPath accepts root-relative redirects without browser URL normalization hazards.
func IsLocalPath(value string) bool {
	if !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") {
		return false
	}
	for _, character := range value {
		// Browsers treat backslashes as slashes and strip some control characters.
		if character == '\\' || character <= 0x1f || character == 0x7f {
			return false
		}
	}
	return true
}
