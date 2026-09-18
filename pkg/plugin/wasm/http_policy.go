package wasm

import (
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strings"

	"github.com/kumbuka-me/sdk"
	"golang.org/x/net/http/httpguts"
)

var blockedHTTPPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"), netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"), netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"), netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"), netip.MustParsePrefix("2002::/16"), netip.MustParsePrefix("3fff::/20"),
}

// validHTTPRequest validates bounded request metadata before any network side effect.
func validHTTPRequest(request sdk.HTTPRequest) bool {
	if !slices.Contains([]string{http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions}, request.Method) {
		return false
	}
	if len(request.URL) == 0 || len(request.URL) > maxHTTPDestination || len(request.Body) > maxHTTPRequestBody || len(request.Headers) > maxHTTPHeaderCount || len(request.AllowedPrivateIPs) > maxHTTPPrivateIPs {
		return false
	}
	parsed, err := url.ParseRequestURI(request.URL)
	if err != nil || !parsed.IsAbs() || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" || strings.ContainsAny(parsed.Host, "%\\") {
		return false
	}
	if request.InsecureSkipVerify && parsed.Scheme != "https" {
		return false
	}
	for _, raw := range request.AllowedPrivateIPs {
		if !validHTTPPrivateIP(raw) {
			return false
		}
	}
	for name, value := range request.Headers {
		if !validPluginHTTPHeader(name, value) {
			return false
		}
	}
	return true
}

// validPluginHTTPHeader accepts application headers while rejecting transport-controlled fields.
func validPluginHTTPHeader(name, value string) bool {
	if !httpguts.ValidHeaderFieldName(name) || !httpguts.ValidHeaderFieldValue(value) || len(value) > maxHTTPHeaderValue {
		return false
	}
	switch strings.ToLower(name) {
	case "host", "content-length", "transfer-encoding", "connection", "proxy-authorization", "proxy-connection", "trailer", "upgrade":
		return false
	default:
		return true
	}
}

// validHTTPPrivateIP reports whether raw is one exact RFC1918 or IPv6 ULA address.
func validHTTPPrivateIP(raw string) bool {
	address, err := netip.ParseAddr(raw)
	return err == nil && address.Zone() == "" && address.IsPrivate() && !address.Is4In6()
}

// allowedHTTPIP applies the host SSRF policy to one resolved destination address.
func allowedHTTPIP(address netip.Addr, private []string) bool {
	if address.Zone() != "" || address.Is4In6() {
		return false
	}
	if address.IsPrivate() {
		for _, raw := range private {
			allowed, err := netip.ParseAddr(raw)
			if err == nil && allowed == address {
				return true
			}
		}
		return false
	}
	if !address.IsGlobalUnicast() || address.IsLoopback() || address.IsLinkLocalUnicast() {
		return false
	}
	if address.Is6() && !netip.MustParsePrefix("2000::/3").Contains(address) {
		return false
	}
	for _, prefix := range blockedHTTPPrefixes {
		if prefix.Contains(address) {
			return false
		}
	}
	return true
}

// boundedHTTPHeaders copies response headers within the public wire limits.
func boundedHTTPHeaders(source http.Header) (map[string][]string, bool) {
	result := make(map[string][]string)
	total := 0
	for name, values := range source {
		if len(result) >= maxHTTPHeaderCount {
			return nil, false
		}
		switch strings.ToLower(name) {
		case "connection", "transfer-encoding", "trailer", "upgrade", "proxy-authenticate", "keep-alive":
			continue
		}
		copied := append([]string(nil), values...)
		for _, value := range copied {
			total += len(name) + len(value)
			if total > maxHTTPHeaderBytes || len(value) > maxHTTPHeaderValue {
				return nil, false
			}
		}
		result[name] = copied
	}
	return result, true
}
