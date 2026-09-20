//go:build wasip1 && wasm

package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
	"unsafe"
)

var input, output []byte

// These local wire types intentionally avoid importing the public SDK package.
// The SDK owns the production WASM exports, while this fixture owns raw exports
// so runtime tests can exercise malformed pointers, malformed JSON, and traps.
type renderRequest struct {
	// Source records the source associated with render request.
	Source string `json:"source"`
}

// renderResult contains the result produced by render result.
type renderResult struct {
	// Parts contains the parts associated with render result.
	Parts []renderPart `json:"parts,omitempty"`
}

// renderPart groups data used by render part.
type renderPart struct {
	// Text stores the text value used by render part.
	Text string `json:"text,omitempty"`
	// Markdown stores the markdown value used by render part.
	Markdown *string `json:"markdown,omitempty"`
}

// capabilityRequest contains the request payload for capability request.
type capabilityRequest struct {
	// Method is the method associated with capability request.
	Method string `json:"method"`
	// Params stores the params value used by capability request.
	Params json.RawMessage `json:"params,omitempty"`
}

// capabilityResponse contains the response payload for capability response.
type capabilityResponse struct {
	// Value contains the value represented by capability response.
	Value json.RawMessage `json:"value,omitempty"`
	// Error stores the error value used by capability response.
	Error string `json:"error,omitempty"`
}

// main runs the package entry point.
func main() {}

// version returns the raw test fixture ABI version.
//
//go:wasmexport kumbuka_api_version
func version() uint32 { return 1 }

// alloc reserves guest memory for one host-provided request.
//
//go:wasmexport kumbuka_alloc
func alloc(n uint32) uint32 {
	input = make([]byte, n)
	return uint32(uintptr(unsafe.Pointer(&input[0])))
}

// transform exercises the requested test behavior and returns a packed response.
//
//go:wasmexport kumbuka_transform
func transform(pointer, length uint32) uint64 {
	var request renderRequest
	_ = json.Unmarshal(input, &request)
	result := renderResult{Parts: []renderPart{{Text: request.Source}}}
	switch {
	case request.Source == "walltime":
		result.Parts[0].Text = time.Now().UTC().Format(time.RFC3339Nano)
	case strings.HasPrefix(request.Source, "host-raw:"):
		data := []byte(strings.TrimPrefix(request.Source, "host-raw:"))
		response := make([]byte, 4096)
		size := rawHost(uint32(uintptr(unsafe.Pointer(&data[0]))), uint32(len(data)), uint32(uintptr(unsafe.Pointer(&response[0]))), uint32(len(response)))
		result.Parts[0].Text = string(response[:size])
	case request.Source == "host-invalid-buffer":
		data := []byte(`{"method":"plugin.storage.write","params":{"Key":"invalid","Value":"YmFk"}}`)
		size := rawHost(uint32(uintptr(unsafe.Pointer(&data[0]))), uint32(len(data)), 0xffffffff, 4096)
		if size != 0 {
			panic("invalid buffer accepted")
		}
		result.Parts[0].Text = "rejected"

	case strings.HasPrefix(request.Source, "host:"):
		var call capabilityRequest
		_ = json.Unmarshal([]byte(strings.TrimPrefix(request.Source, "host:")), &call)
		var value json.RawMessage
		if err := callHost(call.Method, call.Params, &value); err != nil {
			result.Parts[0].Text = err.Error()
		} else {
			result.Parts[0].Text = string(value)
		}

	case request.Source == "loop":
		for {
		}
	case request.Source == "trap":
		panic("guest failed")
	case request.Source == "bad-pointer":
		return uint64(100)<<32 | 0xffffffff
	case request.Source == "oversized":
		return uint64(0xffffffff) << 32
	case request.Source == "malformed":
		output = []byte("{")
		return address()
	case request.Source == "trailing":
		output = []byte(`{} {}`)
		return address()
	case request.Source == "grow":
		output = make([]byte, 128<<20)
		return address()
	case request.Source == "unknown-field":
		output = []byte(`{"trusted_html":"<script>bad</script>"}`)
		return address()
	case request.Source == "recursive":
		value := "recursive"
		result.Parts = []renderPart{{Markdown: &value}}
	case request.Source == "unsafe":
		result.Parts = []renderPart{{Text: `<div>safe</div><script>bad()</script><a href="javascript:bad()">link</a>`}}
	case request.Source == "environment":
		result.Parts[0].Text = strings.Join(os.Environ(), ",")
	case strings.HasPrefix(request.Source, "read:"):
		_, err := os.ReadFile(strings.TrimPrefix(request.Source, "read:"))
		result.Parts[0].Text = denied(err)
	case strings.HasPrefix(request.Source, "write:"):
		err := os.WriteFile(strings.TrimPrefix(request.Source, "write:"), []byte("modified"), 0600)
		result.Parts[0].Text = denied(err)
	case strings.HasPrefix(request.Source, "network:"):
		connection, err := net.DialTimeout("tcp", strings.TrimPrefix(request.Source, "network:"), 20*time.Millisecond)
		if connection != nil {
			_ = connection.Close()
		}
		result.Parts[0].Text = denied(err)
	case request.Source == "process":
		result.Parts[0].Text = denied(exec.Command("/bin/sh", "-c", "exit 0").Run())
	}
	output, _ = json.Marshal(result)
	return address()
}

// callHost exercises the public low-level capability envelope against the raw
// imported host ABI. The SDK intentionally keeps its generic transport internal;
// this reactor owns a copy because these tests validate the ABI boundary itself.
func callHost(method string, params, result any) error {
	data, err := json.Marshal(params)
	if err != nil {
		return err
	}
	request, err := json.Marshal(capabilityRequest{Method: method, Params: data})
	if err != nil {
		return err
	}
	response := make([]byte, 4<<20)
	length := rawHost(
		uint32(uintptr(unsafe.Pointer(&request[0]))),
		uint32(len(request)),
		uint32(uintptr(unsafe.Pointer(&response[0]))),
		uint32(len(response)),
	)
	runtime.KeepAlive(request)
	if length == 0 || uint64(length) > uint64(len(response)) {
		return fmt.Errorf("invalid host response")
	}
	var envelope capabilityResponse
	if err := json.Unmarshal(response[:length], &envelope); err != nil {
		return err
	}
	if envelope.Error != "" {
		return fmt.Errorf("host: %s", envelope.Error)
	}
	if result == nil {
		return nil
	}
	return json.Unmarshal(envelope.Value, result)
}

// denied records whether an intentionally forbidden host call was rejected.
func denied(err error) string {
	if err != nil {
		return "denied"
	}
	return "ALLOWED"
}

// address returns the linear-memory offset of a byte slice.
func address() uint64 { return uint64(len(output))<<32 | uint64(uintptr(unsafe.Pointer(&output[0]))) }

// rawHost invokes the imported Kumbuka host-call ABI directly.
//
//go:wasmimport kumbuka_v1 call
func rawHost(pointer, length, output, capacity uint32) uint32
