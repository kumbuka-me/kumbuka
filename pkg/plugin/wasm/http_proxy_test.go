package wasm

import (
	"io"
	"net"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidatePluginConnectResponseRejectsNonSuccessStatus(t *testing.T) {
	t.Parallel()

	client, server := net.Pipe()
	defer func() { _ = client.Close() }()
	written := make(chan struct{})
	go func() {
		defer close(written)
		defer func() { _ = server.Close() }()
		_, _ = io.WriteString(server, "HTTP/1.1 407 Proxy Authentication Required\r\nContent-Length: 0\r\n\r\n")
	}()

	err := validatePluginConnectResponse(client, pluginConnectRequest(&url.URL{}, "192.0.2.10:443"))

	require.ErrorIs(t, err, errHTTPUnavailable)
	<-written
}

func TestValidatePluginConnectResponseRejectsBufferedTunnelBytes(t *testing.T) {
	t.Parallel()

	client, server := net.Pipe()
	defer func() { _ = client.Close() }()
	written := make(chan struct{})
	go func() {
		defer close(written)
		defer func() { _ = server.Close() }()
		_, _ = io.WriteString(server, "HTTP/1.1 200 Connection Established\r\n\r\nunexpected")
	}()

	err := validatePluginConnectResponse(client, pluginConnectRequest(&url.URL{}, "192.0.2.10:443"))

	require.ErrorIs(t, err, errHTTPUnavailable)
	<-written
}

func TestValidatePluginConnectResponseRejectsOversizedHeaders(t *testing.T) {
	t.Parallel()

	client, server := net.Pipe()
	defer func() { _ = client.Close() }()
	written := make(chan struct{})
	go func() {
		defer close(written)
		defer func() { _ = server.Close() }()
		_, _ = io.WriteString(
			server,
			"HTTP/1.1 200 Connection Established\r\nX-Huge: "+strings.Repeat("a", maxPluginConnectResponseBytes)+"\r\n\r\n",
		)
	}()

	err := validatePluginConnectResponse(client, pluginConnectRequest(&url.URL{}, "192.0.2.10:443"))
	_ = client.Close()

	require.ErrorIs(t, err, errHTTPUnavailable)
	<-written
}
