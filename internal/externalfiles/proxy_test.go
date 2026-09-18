package externalfiles

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func clearProxyEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "no_proxy", "REQUEST_METHOD", "SSL_CERT_FILE", "SSL_CERT_DIR"} {
		t.Setenv(key, "")
	}
}
func TestConventionalProxyEnvironment(t *testing.T) {
	clearProxyEnv(t)
	target, _ := url.Parse("https://git.example/api/v4")
	for _, key := range []string{"HTTPS_PROXY", "https_proxy"} {
		t.Setenv(key, "http://proxy.example:3128")
		got, err := proxyFor(target)
		if err != nil || got == nil || got.Host != "proxy.example:3128" {
			t.Fatalf("%s %v %v", key, got, err)
		}
		t.Setenv(key, "")
	}
	t.Setenv("HTTPS_PROXY", "http://proxy.example:3128")
	t.Setenv("NO_PROXY", "git.example")
	got, err := proxyFor(target)
	if err != nil || got != nil {
		t.Fatal("NO_PROXY ignored")
	}
	t.Setenv("NO_PROXY", "")
	t.Setenv("HTTPS_PROXY", "socks5://proxy.example")
	if _, err := proxyFor(target); err == nil {
		t.Fatal("unsupported proxy silently bypassed")
	}
	httpTarget, _ := url.Parse("http://example.com")
	t.Setenv("HTTP_PROXY", "http://proxy.example:3128")
	got, err = proxyFor(httpTarget)
	if err != nil || got == nil {
		t.Fatal("HTTP_PROXY ignored")
	}
}
func TestProxyTunnelPinsDestinationAndSeparatesCredentials(t *testing.T) {
	clearProxyEnv(t)
	seen := make(chan string, 1)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "CONNECT" || r.Header.Get("Authorization") != "" || r.Header.Get("Proxy-Authorization") != "Basic dXNlcjpwYXNz" {
			t.Error("invalid proxy request")
		}
		seen <- r.Host
		c, b, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = c.Close() }()
		_, _ = b.WriteString("HTTP/1.1 200 Connection established\r\n\r\n")
		_ = b.Flush()
	}))
	defer proxy.Close()
	u, _ := url.Parse(proxy.URL)
	u.User = url.UserPassword("user", "pass")
	t.Setenv("HTTPS_PROXY", u.String())
	c, err := secureClient(source(), false)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := c.Transport.(*http.Transport).DialContext(context.Background(), "tcp", "93.184.216.34:443")
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	if host := <-seen; host != "93.184.216.34:443" {
		t.Fatalf("proxy resolved hostname instead of pinned IP: %s", host)
	}
	if _, err = c.Transport.(*http.Transport).DialContext(context.Background(), "tcp", "169.254.169.254:443"); err == nil {
		t.Fatal("proxy bypassed metadata block")
	}
}
func TestTLSOverrideAndCustomCA(t *testing.T) {
	clearProxyEnv(t)
	for _, enabled := range []bool{false, true} {
		c, err := secureClient(source(), enabled)
		if err != nil {
			t.Fatal(err)
		}
		if c.Transport.(*http.Transport).TLSClientConfig.InsecureSkipVerify != enabled {
			t.Fatal("TLS option ignored")
		}
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = fmt.Fprint(w, "ok") }))
	defer server.Close()
	file := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(file, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SSL_CERT_FILE", file)
	roots, err := certificateRoots()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = server.Certificate().Verify(x509.VerifyOptions{Roots: roots}); err != nil {
		t.Fatal("CA not trusted", err)
	}
	t.Setenv("SSL_CERT_FILE", file+"missing")
	if _, err = certificateRoots(); err == nil {
		t.Fatal("bad CA setting ignored")
	}
}
func TestProxyHeaderBound(t *testing.T) {
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, b, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer func() { _ = c.Close() }()
		_, _ = b.WriteString("HTTP/1.1 200 OK\r\nX-Huge: " + strings.Repeat("a", 32<<10) + "\r\n\r\n")
		_ = b.Flush()
	}))
	defer proxy.Close()
	u, _ := url.Parse(proxy.URL)
	if c, err := tunnel(context.Background(), u, "93.184.216.34:443", nil); err == nil {
		_ = c.Close()
		t.Fatal("oversized CONNECT response accepted")
	}
}

func TestHTTPSProxyRequiresTrustedCertificate(t *testing.T) {
	clearProxyEnv(t)
	proxy := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, b, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer func() { _ = c.Close() }()
		_, _ = b.WriteString("HTTP/1.1 200 OK\r\n\r\n")
		_ = b.Flush()
	}))
	defer proxy.Close()
	u, _ := url.Parse(proxy.URL)
	if c, err := tunnel(context.Background(), u, "93.184.216.34:443", x509.NewCertPool()); err == nil {
		_ = c.Close()
		t.Fatal("untrusted HTTPS proxy accepted")
	}
	roots := x509.NewCertPool()
	roots.AddCert(proxy.Certificate())
	c, err := tunnel(context.Background(), u, "93.184.216.34:443", roots)
	if err != nil {
		t.Fatal("trusted HTTPS proxy failed", err)
	}
	_ = c.Close()
}
