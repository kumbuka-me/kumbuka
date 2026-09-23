package wasm

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/kumbuka-me/sdk"
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

// httpCall validates and performs one generic outbound HTTP request for a plugin.
func (r *Runtime) httpCall(ctx context.Context, caller *Instance, raw []byte) (any, error) {
	if r.httpAuthorizer == nil || !r.httpAuthorizer(ctx) {
		return nil, errHTTPUnavailable
	}

	request, err := r.decodeHTTPRequest(caller, raw)
	if err != nil {
		return nil, err
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
	httpRequest, err := newPluginHTTPRequest(execution, request)
	if err != nil {
		return nil, errHTTPUnavailable
	}
	response, err := client.Do(httpRequest)
	if err != nil {
		return nil, errHTTPUnavailable
	}
	defer func() { _ = response.Body.Close() }()
	return readPluginHTTPResponse(response)
}

// decodeHTTPRequest validates the wire request and capability-gated network options.
func (r *Runtime) decodeHTTPRequest(caller *Instance, raw []byte) (sdk.HTTPRequest, error) {
	var request sdk.HTTPRequest
	if err := decode(raw, &request); err != nil || !validHTTPRequest(request) {
		return sdk.HTTPRequest{}, errHTTPUnavailable
	}
	if len(request.AllowedPrivateIPs) > 0 && !r.permissionGranted(caller, "network:private") {
		return sdk.HTTPRequest{}, errors.New("capability denied")
	}
	if request.InsecureSkipVerify && !r.permissionGranted(caller, "network:insecure-tls") {
		return sdk.HTTPRequest{}, errors.New("capability denied")
	}
	return request, nil
}

// newPluginHTTPRequest constructs one bounded outbound request with the default plugin user agent.
func newPluginHTTPRequest(ctx context.Context, request sdk.HTTPRequest) (*http.Request, error) {
	httpRequest, err := http.NewRequestWithContext(ctx, request.Method, request.URL, bytes.NewReader(request.Body))
	if err != nil {
		return nil, err
	}
	for name, value := range request.Headers {
		httpRequest.Header.Set(name, value)
	}
	if httpRequest.Header.Get("User-Agent") == "" {
		httpRequest.Header.Set("User-Agent", pluginHTTPUserAgent)
	}
	return httpRequest, nil
}

// readPluginHTTPResponse enforces response body and header bounds before exposing data to the guest.
func readPluginHTTPResponse(response *http.Response) (sdk.HTTPResponse, error) {
	if response.ContentLength > maxHTTPResponseBody {
		return sdk.HTTPResponse{}, errHTTPUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxHTTPResponseBody+1))
	if err != nil || len(body) > maxHTTPResponseBody {
		return sdk.HTTPResponse{}, errHTTPUnavailable
	}
	headers, ok := boundedHTTPHeaders(response.Header)
	if !ok {
		return sdk.HTTPResponse{}, errHTTPUnavailable
	}
	return sdk.HTTPResponse{StatusCode: response.StatusCode, Headers: headers, Body: body}, nil
}
