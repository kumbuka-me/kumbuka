package wasm

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/kumbuka-me/sdk"
	"golang.org/x/net/http/httpguts"
	"golang.org/x/net/http/httpproxy"
)

const (
	maxHTTPRequestBody  = 1 << 20
	maxHTTPResponseBody = 1 << 20
	maxHTTPHeaderCount  = 64
	maxHTTPHeaderBytes  = 64 << 10
	maxHTTPPrivateIPs   = 16
	maxHTTPDestination  = 4096
	maxHTTPHeaderValue  = 8 << 10
	pluginHTTPUserAgent = "Kumbuka-Plugin/1"
)

var errHTTPUnavailable = errors.New("plugin HTTP request unavailable")

var blockedHTTPPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"), netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"), netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"), netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"), netip.MustParsePrefix("2002::/16"), netip.MustParsePrefix("3fff::/20"),
}

// httpCall validates and performs one generic outbound HTTP request for a plugin.
func (r *Runtime) httpCall(ctx context.Context, caller *Instance, raw []byte) (any, error) {
	if r.httpAuthorizer == nil || !r.httpAuthorizer(ctx) {
		return nil, errHTTPUnavailable
	}

	var request sdk.HTTPRequest
	if err := decode(raw, &request); err != nil || !validHTTPRequest(request) {
		return nil, errHTTPUnavailable
	}
	if len(request.AllowedPrivateIPs) > 0 && !r.permissionGranted(caller, "network:private") {
		return nil, errors.New("capability denied")
	}
	if request.InsecureSkipVerify && !r.permissionGranted(caller, "network:insecure-tls") {
		return nil, errors.New("capability denied")
	}

	select {
	case r.httpActive <- struct{}{}:
		defer func() { <-r.httpActive }()
	default:
		return nil, errHTTPUnavailable
	}

	client, err := secureHTTPClient(request)
	if err != nil {
		return nil, errHTTPUnavailable
	}
	defer client.CloseIdleConnections()

	execution, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	httpRequest, err := http.NewRequestWithContext(execution, request.Method, request.URL, bytes.NewReader(request.Body))
	if err != nil {
		return nil, errHTTPUnavailable
	}
	for name, value := range request.Headers {
		httpRequest.Header.Set(name, value)
	}
	if httpRequest.Header.Get("User-Agent") == "" {
		httpRequest.Header.Set("User-Agent", pluginHTTPUserAgent)
	}

	response, err := client.Do(httpRequest)
	if err != nil {
		return nil, errHTTPUnavailable
	}
	defer func() { _ = response.Body.Close() }()
	if response.ContentLength > maxHTTPResponseBody {
		return nil, errHTTPUnavailable
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, maxHTTPResponseBody+1))
	if err != nil || len(body) > maxHTTPResponseBody {
		return nil, errHTTPUnavailable
	}
	headers, ok := boundedHTTPHeaders(response.Header)
	if !ok {
		return nil, errHTTPUnavailable
	}
	return sdk.HTTPResponse{StatusCode: response.StatusCode, Headers: headers, Body: body}, nil
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

// secureHTTPClient creates a no-redirect client that pins validated DNS answers to actual dials.
func secureHTTPClient(request sdk.HTTPRequest) (*http.Client, error) {
	roots, err := pluginCertificateRoots()
	if err != nil {
		return nil, errHTTPUnavailable
	}
	target, err := url.Parse(request.URL)
	if err != nil {
		return nil, errHTTPUnavailable
	}
	proxyURL, err := pluginProxyFor(target)
	if err != nil {
		return nil, errHTTPUnavailable
	}

	transport := &http.Transport{
		TLSClientConfig:        &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, InsecureSkipVerify: request.InsecureSkipVerify},
		Proxy:                  nil,
		DisableCompression:     true,
		DisableKeepAlives:      true,
		MaxResponseHeaderBytes: maxHTTPHeaderBytes,
		TLSHandshakeTimeout:    3 * time.Second,
		ResponseHeaderTimeout:  4 * time.Second,
	}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, errHTTPUnavailable
		}
		addresses, err := resolveHTTPHost(ctx, host)
		if err != nil || len(addresses) == 0 {
			return nil, errHTTPUnavailable
		}
		for _, address := range addresses {
			if !allowedHTTPIP(address, request.AllowedPrivateIPs) {
				return nil, errHTTPUnavailable
			}
		}

		dialer := net.Dialer{Timeout: 3 * time.Second}
		for _, resolved := range addresses {
			destination := net.JoinHostPort(resolved.String(), port)
			var connection net.Conn
			var dialErr error
			if proxyURL != nil {
				connection, dialErr = pluginProxyTunnel(ctx, proxyURL, destination, roots)
			} else {
				connection, dialErr = dialer.DialContext(ctx, network, destination)
			}
			if dialErr == nil {
				return connection, nil
			}
		}
		return nil, errHTTPUnavailable
	}

	return &http.Client{
		Transport: transport,
		Timeout:   8 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}, nil
}

// resolveHTTPHost resolves a hostname or returns a validated literal address without a second lookup.
func resolveHTTPHost(ctx context.Context, host string) ([]netip.Addr, error) {
	if address, err := netip.ParseAddr(host); err == nil {
		return []netip.Addr{address.Unmap()}, nil
	}
	addresses, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	result := make([]netip.Addr, 0, len(addresses))
	for _, address := range addresses {
		result = append(result, address.Unmap())
	}
	return result, nil
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

// pluginCertificateRoots honors conventional CA environment variables for plugin HTTP.
func pluginCertificateRoots() (*x509.CertPool, error) {
	file, dirs := os.Getenv("SSL_CERT_FILE"), os.Getenv("SSL_CERT_DIR")
	if file == "" && dirs == "" {
		return nil, nil
	}
	roots, err := x509.SystemCertPool()
	if err != nil {
		roots = x509.NewCertPool()
	}
	budget := int64(8 << 20)
	add := func(name string) (bool, error) {
		input, err := os.Open(name)
		if err != nil {
			return false, errHTTPUnavailable
		}
		defer func() { _ = input.Close() }()
		info, err := input.Stat()
		if err != nil || !info.Mode().IsRegular() || info.Size() > budget {
			return false, errHTTPUnavailable
		}
		data, err := io.ReadAll(io.LimitReader(input, budget+1))
		if err != nil || int64(len(data)) > budget {
			return false, errHTTPUnavailable
		}
		budget -= int64(len(data))
		return roots.AppendCertsFromPEM(data), nil
	}
	if file != "" {
		ok, err := add(file)
		if err != nil || !ok {
			return nil, errHTTPUnavailable
		}
	}
	for _, directory := range filepath.SplitList(dirs) {
		if directory == "" {
			continue
		}
		entries, err := os.ReadDir(directory)
		if err != nil || len(entries) > 512 {
			return nil, errHTTPUnavailable
		}
		added := false
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			ok, err := add(filepath.Join(directory, entry.Name()))
			if err != nil {
				return nil, err
			}
			added = added || ok
		}
		if !added {
			return nil, errHTTPUnavailable
		}
	}
	return roots, nil
}

// pluginProxyFor resolves the conventional HTTP proxy for a plugin destination.
func pluginProxyFor(target *url.URL) (*url.URL, error) {
	proxyURL, err := httpproxy.FromEnvironment().ProxyFunc()(target)
	if err != nil {
		return nil, errHTTPUnavailable
	}
	if proxyURL != nil && (proxyURL.Hostname() == "" || (proxyURL.Scheme != "http" && proxyURL.Scheme != "https") || proxyURL.RawQuery != "" || proxyURL.Fragment != "" || (proxyURL.Path != "" && proxyURL.Path != "/")) {
		return nil, errHTTPUnavailable
	}
	return proxyURL, nil
}

// pluginProxyTunnel connects through a deployment-trusted proxy to a validated numeric destination.
func pluginProxyTunnel(ctx context.Context, proxyURL *url.URL, target string, roots *x509.CertPool) (net.Conn, error) {
	port := proxyURL.Port()
	if port == "" {
		port = "80"
		if proxyURL.Scheme == "https" {
			port = "443"
		}
	}

	dialer := net.Dialer{Timeout: 3 * time.Second}
	connection, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(proxyURL.Hostname(), port))
	if err != nil {
		return nil, errHTTPUnavailable
	}
	success := false
	defer func() {
		if !success {
			_ = connection.Close()
		}
	}()

	rawConnection := connection
	stop := context.AfterFunc(ctx, func() { _ = rawConnection.Close() })
	defer stop()
	deadline := time.Now().Add(5 * time.Second)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	_ = connection.SetDeadline(deadline)

	if proxyURL.Scheme == "https" {
		tlsConnection := tls.Client(connection, &tls.Config{ServerName: proxyURL.Hostname(), MinVersion: tls.VersionTLS12, RootCAs: roots})
		if err := tlsConnection.HandshakeContext(ctx); err != nil {
			return nil, errHTTPUnavailable
		}
		connection = tlsConnection
	}

	request := &http.Request{Method: http.MethodConnect, URL: &url.URL{Opaque: target}, Host: target, Header: make(http.Header)}
	if proxyURL.User != nil {
		password, _ := proxyURL.User.Password()
		request.Header.Set("Proxy-Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(proxyURL.User.Username()+":"+password)))
	}
	if err := request.Write(connection); err != nil {
		return nil, errHTTPUnavailable
	}

	reader := bufio.NewReaderSize(connection, 16<<10)
	total := 0
	for {
		lineBytes, err := reader.ReadSlice('\n')
		line := string(lineBytes)
		total += len(line)
		if err != nil || total > 16<<10 {
			return nil, errHTTPUnavailable
		}
		if total == len(line) {
			response, parseErr := http.ReadResponse(bufio.NewReader(strings.NewReader(line+"\r\n")), request)
			if parseErr != nil || response.StatusCode != http.StatusOK {
				return nil, errHTTPUnavailable
			}
		}
		if line == "\r\n" {
			break
		}
	}
	if reader.Buffered() != 0 || !stop() {
		return nil, errHTTPUnavailable
	}
	_ = connection.SetDeadline(time.Time{})
	success = true
	return connection, nil
}

// permissionGranted reports whether application policy and the plugin manifest grant permission.
func (r *Runtime) permissionGranted(caller *Instance, permission string) bool {
	return r.permissions[permission] && slices.Contains(caller.manifest.Permissions, permission)
}
