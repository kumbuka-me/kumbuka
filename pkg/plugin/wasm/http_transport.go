package wasm

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"time"

	"github.com/kumbuka-me/sdk"
)

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
