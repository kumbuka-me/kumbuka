package response

import httpkitresponse "github.com/containeroo/httpkit/response"

// IsLocalPath accepts root-relative redirects without browser URL normalization hazards.
func IsLocalPath(value string) bool {
	return httpkitresponse.IsLocalPath(value)
}
