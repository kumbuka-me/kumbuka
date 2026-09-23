package wasm

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"net/url"
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
	connection, err := connectPluginProxy(ctx, proxyURL, roots)
	if err != nil {
		return nil, err
	}
	success := false
	defer func() {
		if !success {
			_ = connection.Close()
		}
	}()

	stop, err := armPluginProxyConnection(ctx, connection)
	if err != nil {
		return nil, err
	}
	defer stop()

	request := pluginConnectRequest(proxyURL, target)
	if err := request.Write(connection); err != nil {
		return nil, errHTTPUnavailable
	}
	if err := validatePluginConnectResponse(connection, request); err != nil {
		return nil, err
	}
	if !stop() {
		return nil, errHTTPUnavailable
	}
	_ = connection.SetDeadline(time.Time{})
	success = true
	return connection, nil
}

// connectPluginProxy dials the proxy endpoint and performs TLS when the proxy uses HTTPS.
func connectPluginProxy(ctx context.Context, proxyURL *url.URL, roots *x509.CertPool) (net.Conn, error) {
	port := proxyURL.Port()
	if port == "" {
		port = "80"
		if proxyURL.Scheme == "https" {
			port = "443"
		}
	}
	connection, err := (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "tcp", net.JoinHostPort(proxyURL.Hostname(), port))
	if err != nil {
		return nil, errHTTPUnavailable
	}
	if proxyURL.Scheme != "https" {
		return connection, nil
	}

	tlsConnection := tls.Client(connection, &tls.Config{ServerName: proxyURL.Hostname(), MinVersion: tls.VersionTLS12, RootCAs: roots})
	if err := tlsConnection.HandshakeContext(ctx); err != nil {
		_ = connection.Close()
		return nil, errHTTPUnavailable
	}
	return tlsConnection, nil
}

// armPluginProxyConnection binds the tunnel setup to the request context and a short I/O deadline.
func armPluginProxyConnection(ctx context.Context, connection net.Conn) (func() bool, error) {
	stop := context.AfterFunc(ctx, func() { _ = connection.Close() })
	deadline := time.Now().Add(5 * time.Second)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	if err := connection.SetDeadline(deadline); err != nil {
		stop()
		return nil, errHTTPUnavailable
	}
	return stop, nil
}

// pluginConnectRequest creates an authenticated CONNECT request for the validated target.
func pluginConnectRequest(proxyURL *url.URL, target string) *http.Request {
	request := &http.Request{Method: http.MethodConnect, URL: &url.URL{Opaque: target}, Host: target, Header: make(http.Header)}
	if proxyURL.User == nil {
		return request
	}
	password, _ := proxyURL.User.Password()
	request.Header.Set("Proxy-Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(proxyURL.User.Username()+":"+password)))
	return request
}

const maxPluginConnectResponseBytes = 16 << 10

// validatePluginConnectResponse reads a bounded proxy response and requires an empty buffered tail.
func validatePluginConnectResponse(connection net.Conn, request *http.Request) error {
	limited := io.LimitReader(connection, maxPluginConnectResponseBytes+1)
	reader := bufio.NewReaderSize(limited, maxPluginConnectResponseBytes)
	response, err := http.ReadResponse(reader, request)
	if err != nil || response.StatusCode != http.StatusOK {
		return errHTTPUnavailable
	}
	if response.Body != nil {
		_ = response.Body.Close()
	}
	if reader.Buffered() != 0 {
		return errHTTPUnavailable
	}
	return nil
}
