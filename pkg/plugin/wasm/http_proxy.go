package wasm

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/http/httpproxy"
)

// pluginProxyFor resolves the conventional HTTP proxy for a plugin destination.
func pluginProxyFor(target *url.URL) (*url.URL, error) {
	proxyURL, err := httpproxy.FromEnvironment().ProxyFunc()(target)
	if err != nil {
		return nil, errHTTPUnavailable
	}
	if proxyURL != nil && !validPluginProxyURL(proxyURL) {
		return nil, errHTTPUnavailable
	}
	return proxyURL, nil
}

// validPluginProxyURL reports whether a deployment proxy URL can safely carry pinned CONNECT requests.
func validPluginProxyURL(proxyURL *url.URL) bool {
	validScheme := proxyURL.Scheme == "http" || proxyURL.Scheme == "https"
	validPath := proxyURL.Path == "" || proxyURL.Path == "/"

	return proxyURL.Hostname() != "" && validScheme && validPath && proxyURL.RawQuery == "" && proxyURL.Fragment == ""
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
