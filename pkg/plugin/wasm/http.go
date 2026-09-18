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
