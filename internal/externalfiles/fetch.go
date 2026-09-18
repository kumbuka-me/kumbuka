package externalfiles

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	sdk "github.com/kumbuka-me/sdk"
)

// Exclude non-public and special-use destinations even if DNS resolves them.
var blocked = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"), netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"), netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"), netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"), netip.MustParsePrefix("2002::/16"), netip.MustParsePrefix("3fff::/20"),
}

func validPrivateIP(raw string) bool {
	a, e := netip.ParseAddr(raw)
	return e == nil && a.Zone() == "" && a.IsPrivate() && !a.Is4In6()
}
func allowedIP(a netip.Addr, private []string) bool {
	if a.Zone() != "" || a.Is4In6() {
		return false
	}
	if a.IsPrivate() {
		for _, raw := range private {
			if p, e := netip.ParseAddr(raw); e == nil && p == a {
				return true
			}
		}
		return false
	}
	if !a.IsGlobalUnicast() || a.IsLoopback() || a.IsLinkLocalUnicast() {
		return false
	}
	if a.Is6() && !netip.MustParsePrefix("2000::/3").Contains(a) {
		return false
	}
	for _, p := range blocked {
		if p.Contains(a) {
			return false
		}
	}
	return true
}

// secureClient validates every DNS answer and dials only a validated numeric
// address, including through a proxy tunnel. Redirects are never followed.
// TLS verifies the original hostname unless the deployment explicitly opts out.
func secureClient(v Source, insecure bool) (*http.Client, error) {
	roots, err := certificateRoots()
	if err != nil {
		return nil, unavailable
	}
	target, err := url.Parse(v.Endpoint)
	if err != nil {
		return nil, unavailable
	}
	proxyURL, err := proxyFor(target)
	if err != nil {
		return nil, unavailable
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, InsecureSkipVerify: insecure}, Proxy: nil, DisableCompression: true, DisableKeepAlives: true, MaxResponseHeaderBytes: 16 << 10, TLSHandshakeTimeout: 3 * time.Second, ResponseHeaderTimeout: 4 * time.Second}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, unavailable
		}
		ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if err != nil || len(ips) == 0 {
			return nil, unavailable
		}
		for _, ip := range ips {
			if !allowedIP(ip.Unmap(), v.PrivateIPs) {
				return nil, unavailable
			}
		}
		dialer := net.Dialer{Timeout: 3 * time.Second}
		for _, ip := range ips {
			var c net.Conn
			var e error
			destination := net.JoinHostPort(ip.Unmap().String(), port)
			if proxyURL != nil {
				c, e = tunnel(ctx, proxyURL, destination, roots)
			} else {
				c, e = dialer.DialContext(ctx, "tcp", destination)
			}
			if e == nil {
				return c, nil
			}
		}
		return nil, unavailable
	}
	return &http.Client{Transport: transport, Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, nil
}

func fileURL(v Source, path string) string {
	base := strings.TrimRight(v.Endpoint, "/")
	if v.Provider == "gitlab" {
		return base + "/projects/" + url.PathEscape(v.Repository) + "/repository/files/" + url.PathEscape(path) + "/raw?ref=" + url.QueryEscape(v.Ref) + "&lfs=false"
	}
	segments := strings.Split(v.Repository+"/contents/"+path, "/")
	for i := range segments {
		segments[i] = url.PathEscape(segments[i])
	}
	return base + "/repos/" + strings.Join(segments, "/") + "?ref=" + url.QueryEscape(v.Ref)
}

func fetchFile(ctx context.Context, v Source, path string, insecure bool) (string, error) {
	client, err := secureClient(v, insecure)
	if err != nil {
		return "", unavailable
	}
	defer client.CloseIdleConnections()
	return fetchWithClient(ctx, client, v, path)
}

func fetchWithClient(ctx context.Context, client *http.Client, v Source, path string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL(v, path), nil)
	if err != nil {
		return "", unavailable
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Kumbuka-External-Files")
	if v.Provider == "github" {
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		if v.Token != "" {
			req.Header.Set("Authorization", "Bearer "+v.Token)
		}
	} else if v.Token != "" {
		req.Header.Set("PRIVATE-TOKEN", v.Token)
	}
	response, err := client.Do(req)
	if err != nil {
		return "", unavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Encoding") != "" {
		return "", unavailable
	}
	limit := int64(maxFile)
	if v.Provider == "github" {
		limit = 2 * maxFile
	}
	if response.ContentLength > limit {
		return "", unavailable
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil || int64(len(body)) > limit {
		return "", unavailable
	}
	if v.Provider == "github" {
		var file struct {
			Type, Encoding, Content string
			Size                    int
		}
		if json.Unmarshal(body, &file) != nil || file.Type != "file" || file.Encoding != "base64" || file.Size > maxFile {
			return "", unavailable
		}
		body, err = base64.StdEncoding.DecodeString(file.Content)
		if err != nil {
			return "", unavailable
		}
	}
	content := string(body)
	if !validContent(content) {
		return "", unavailable
	}
	return content, nil
}

func validContent(content string) bool {
	if len(content) > maxFile || !utf8.ValidString(content) {
		return false
	}
	for _, r := range content {
		if (unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t') || unicode.In(r, unicode.Cf) {
			return false
		}
	}
	return true
}

func selectLines(content string, q sdk.ExternalFileRequest) (sdk.ExternalFile, error) {
	if !validContent(content) {
		return sdk.ExternalFile{}, unavailable
	}
	content = strings.ReplaceAll(content, "\r\n", "\n")
	// A terminal newline terminates the last line; it does not create another one.
	content = strings.TrimSuffix(content, "\n")
	lines := strings.Split(content, "\n")
	if len(lines) > 10000 {
		return sdk.ExternalFile{}, unavailable
	}
	start, end := q.Start, q.End
	if start == 0 && end == 0 {
		start = 1
		end = len(lines)
	}
	if start < 1 || end < start || end > len(lines) {
		return sdk.ExternalFile{}, errors.New("external file line range is out of bounds")
	}
	return sdk.ExternalFile{Content: strings.Join(lines[start-1:end], "\n"), Start: start}, nil
}
