package wasm

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sdk "github.com/kumbuka-me/sdk"
)

// clearPluginHTTPEnv removes ambient proxy and certificate settings from one test.
func clearPluginHTTPEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "no_proxy", "REQUEST_METHOD", "SSL_CERT_FILE", "SSL_CERT_DIR"} {
		t.Setenv(key, "")
	}
}

// TestPluginHTTPNetworkPolicy verifies special-use destinations stay blocked unless an exact private address is allowed.
func TestPluginHTTPNetworkPolicy(t *testing.T) {
	for _, raw := range []string{"127.0.0.1", "0.0.0.0", "169.254.169.254", "10.0.0.1", "172.16.0.1", "192.168.1.1", "100.100.100.200", "192.0.2.1", "198.18.0.1", "224.0.0.1", "::1", "fe80::1", "fc00::1", "64:ff9b::7f00:1", "2002:7f00:1::", "2001:db8::1"} {
		if allowedHTTPIP(netip.MustParseAddr(raw), nil) {
			t.Errorf("allowed %s", raw)
		}
	}
	if !allowedHTTPIP(netip.MustParseAddr("10.0.0.1"), []string{"10.0.0.1"}) || allowedHTTPIP(netip.MustParseAddr("10.0.0.2"), []string{"10.0.0.1"}) {
		t.Fatal("private IP exception is not exact")
	}
	for _, raw := range []string{"127.0.0.1", "169.254.169.254", "10.0.0.0/8", "::ffff:10.0.0.1"} {
		if validHTTPPrivateIP(raw) {
			t.Errorf("accepted invalid private exception %s", raw)
		}
	}
}

// TestPluginHTTPRequestValidation verifies the generic HTTP wire contract rejects unsafe request metadata.
func TestPluginHTTPRequestValidation(t *testing.T) {
	valid := sdk.HTTPRequest{Method: http.MethodGet, URL: "https://example.com/api", Headers: map[string]string{"Accept": "application/json"}}
	if !validHTTPRequest(valid) {
		t.Fatal("valid request rejected")
	}
	for _, request := range []sdk.HTTPRequest{
		{Method: "TRACE", URL: valid.URL},
		{Method: http.MethodGet, URL: "file:///etc/passwd"},
		{Method: http.MethodGet, URL: "https://user:secret@example.com"},
		{Method: http.MethodGet, URL: "https://example.com/#fragment"},
		{Method: http.MethodGet, URL: valid.URL, Headers: map[string]string{"Host": "attacker.example"}},
		{Method: http.MethodGet, URL: "http://example.com", InsecureSkipVerify: true},
		{Method: http.MethodGet, URL: valid.URL, AllowedPrivateIPs: []string{"127.0.0.1"}},
	} {
		if validHTTPRequest(request) {
			t.Fatalf("unsafe request accepted: %+v", request)
		}
	}
}

// TestPluginHTTPConventionalProxyEnvironment verifies standard proxy variables are honored without accepting unsupported proxy schemes.
func TestPluginHTTPConventionalProxyEnvironment(t *testing.T) {
	clearPluginHTTPEnv(t)
	target, _ := url.Parse("https://git.example/api/v4")
	for _, key := range []string{"HTTPS_PROXY", "https_proxy"} {
		t.Setenv(key, "http://proxy.example:3128")
		got, err := pluginProxyFor(target)
		if err != nil || got == nil || got.Host != "proxy.example:3128" {
			t.Fatalf("%s: %v %v", key, got, err)
		}
		t.Setenv(key, "")
	}

	t.Setenv("HTTPS_PROXY", "http://proxy.example:3128")
	t.Setenv("NO_PROXY", "git.example")
	got, err := pluginProxyFor(target)
	if err != nil || got != nil {
		t.Fatal("NO_PROXY ignored")
	}

	t.Setenv("NO_PROXY", "")
	t.Setenv("HTTPS_PROXY", "socks5://proxy.example")
	if _, err := pluginProxyFor(target); err == nil {
		t.Fatal("unsupported proxy silently accepted")
	}
}

// TestPluginHTTPProxyTunnelPinsDestination verifies proxy CONNECT receives a validated numeric target and separate proxy credentials.
func TestPluginHTTPProxyTunnelPinsDestination(t *testing.T) {
	clearPluginHTTPEnv(t)
	seen := make(chan string, 1)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect || r.Header.Get("Authorization") != "" || r.Header.Get("Proxy-Authorization") != "Basic dXNlcjpwYXNz" {
			t.Error("invalid proxy request")
		}
		seen <- r.Host
		connection, buffer, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = connection.Close() }()
		_, _ = buffer.WriteString("HTTP/1.1 200 Connection established\r\n\r\n")
		_ = buffer.Flush()
	}))
	defer proxy.Close()

	proxyURL, _ := url.Parse(proxy.URL)
	proxyURL.User = url.UserPassword("user", "pass")
	t.Setenv("HTTPS_PROXY", proxyURL.String())
	client, err := secureHTTPClient(sdk.HTTPRequest{Method: http.MethodGet, URL: "https://example.com"})
	if err != nil {
		t.Fatal(err)
	}
	connection, err := client.Transport.(*http.Transport).DialContext(context.Background(), "tcp", "93.184.216.34:443")
	if err != nil {
		t.Fatal(err)
	}
	_ = connection.Close()
	if host := <-seen; host != "93.184.216.34:443" {
		t.Fatalf("proxy received unpinned destination %s", host)
	}
	if _, err = client.Transport.(*http.Transport).DialContext(context.Background(), "tcp", "169.254.169.254:443"); err == nil {
		t.Fatal("proxy bypassed special-use destination block")
	}
}

// TestPluginHTTPTLSOptions verifies plugin HTTP honors the explicit TLS override and conventional custom certificate roots.
func TestPluginHTTPTLSOptions(t *testing.T) {
	clearPluginHTTPEnv(t)
	for _, enabled := range []bool{false, true} {
		client, err := secureHTTPClient(sdk.HTTPRequest{Method: http.MethodGet, URL: "https://example.com", InsecureSkipVerify: enabled})
		if err != nil {
			t.Fatal(err)
		}
		if client.Transport.(*http.Transport).TLSClientConfig.InsecureSkipVerify != enabled {
			t.Fatal("TLS option ignored")
		}
	}

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = fmt.Fprint(w, "ok") }))
	defer server.Close()
	file := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(file, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SSL_CERT_FILE", file)
	roots, err := pluginCertificateRoots()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = server.Certificate().Verify(x509.VerifyOptions{Roots: roots}); err != nil {
		t.Fatal("custom CA not trusted", err)
	}
	t.Setenv("SSL_CERT_FILE", file+".missing")
	if _, err = pluginCertificateRoots(); err == nil {
		t.Fatal("missing custom CA silently ignored")
	}
}

// TestPluginHTTPProxyHeaderBound verifies oversized CONNECT responses are rejected.
func TestPluginHTTPProxyHeaderBound(t *testing.T) {
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		connection, buffer, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer func() { _ = connection.Close() }()
		_, _ = buffer.WriteString("HTTP/1.1 200 OK\r\nX-Huge: " + strings.Repeat("a", 32<<10) + "\r\n\r\n")
		_ = buffer.Flush()
	}))
	defer proxy.Close()
	proxyURL, _ := url.Parse(proxy.URL)
	if connection, err := pluginProxyTunnel(context.Background(), proxyURL, "93.184.216.34:443", nil); err == nil {
		_ = connection.Close()
		t.Fatal("oversized CONNECT response accepted")
	}
}
