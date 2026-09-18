package externalfiles

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/http/httpproxy"
)

// proxyFor uses Go's conventional uppercase/lowercase proxy and NO_PROXY rules.
// A proxy is deployment-trusted infrastructure, never selected by page content.
func proxyFor(target *url.URL) (*url.URL, error) {
	proxyURL, err := httpproxy.FromEnvironment().ProxyFunc()(target)
	if err != nil {
		return nil, errUnavailable
	}
	if proxyURL != nil && (proxyURL.Hostname() == "" || (proxyURL.Scheme != "http" && proxyURL.Scheme != "https") || proxyURL.RawQuery != "" || proxyURL.Fragment != "" || (proxyURL.Path != "" && proxyURL.Path != "/")) {
		return nil, errors.New("external files require an HTTP or HTTPS proxy")
	}
	return proxyURL, nil
}

// tunnel connects through the administrator's proxy to an already-validated
// numeric destination. This prevents the proxy from resolving the provider name
// again to a different private address (DNS rebinding). Origin TLS runs above it.
func tunnel(ctx context.Context, proxyURL *url.URL, target string, roots *x509.CertPool) (net.Conn, error) {
	port := proxyURL.Port()
	if port == "" {
		port = "80"
		if proxyURL.Scheme == "https" {
			port = "443"
		}
	}
	dialer := net.Dialer{Timeout: 3 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(proxyURL.Hostname(), port))
	if err != nil {
		return nil, errUnavailable
	}
	success := false
	defer func() {
		if !success {
			_ = conn.Close()
		}
	}()
	rawConn := conn
	stop := context.AfterFunc(ctx, func() { _ = rawConn.Close() })
	defer stop()
	deadline := time.Now().Add(5 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = conn.SetDeadline(deadline)
	if proxyURL.Scheme == "https" {
		// The provider's insecure switch never disables verification of a TLS proxy.
		connTLS := tls.Client(conn, &tls.Config{ServerName: proxyURL.Hostname(), MinVersion: tls.VersionTLS12, RootCAs: roots})
		if err := connTLS.HandshakeContext(ctx); err != nil {
			return nil, errUnavailable
		}
		conn = connTLS
	}
	request := &http.Request{Method: http.MethodConnect, URL: &url.URL{Opaque: target}, Host: target, Header: make(http.Header)}
	if proxyURL.User != nil {
		password, _ := proxyURL.User.Password()
		request.Header.Set("Proxy-Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(proxyURL.User.Username()+":"+password)))
	}
	if err := request.Write(conn); err != nil {
		return nil, errUnavailable
	}
	// Read a bounded CONNECT header without consuming bytes from the tunnel.
	reader := bufio.NewReaderSize(conn, 16<<10)
	total := 0
	for {
		lineBytes, err := reader.ReadSlice('\n')
		line := string(lineBytes)
		total += len(line)
		if err != nil || total > 16<<10 {
			return nil, errUnavailable
		}
		if total == len(line) {
			// Parse the status with the standard parser using only a synthetic header terminator.
			response, e := http.ReadResponse(bufio.NewReader(strings.NewReader(line+"\r\n")), request)
			if e != nil || response.StatusCode != http.StatusOK {
				return nil, errUnavailable
			}
		}
		if line == "\r\n" {
			break
		}
	}
	if reader.Buffered() != 0 {
		return nil, errUnavailable
	}
	if !stop() {
		return nil, errUnavailable
	}
	_ = conn.SetDeadline(time.Time{})
	success = true
	return conn, nil
}
