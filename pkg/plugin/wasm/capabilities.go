package wasm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"slices"
	"strings"

	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/sdk"
	"github.com/tetratelabs/wazero/api"
)

// callerKey identifies the current guest invocation in context.
type callerKey struct {
}

// invocationState binds host capabilities and call limits to one guest invocation.
type invocationState struct {
	// instance owns the active executable plugin instance.
	instance *Instance
	// remaining limits host capability calls for one guest invocation.
	remaining int
}

// capabilitiesKey stores render-scoped capabilities in context without collisions.
type capabilitiesKey struct {
}

// decode strictly decodes one capability request payload.
func decode(data []byte, result any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()

	if err := d.Decode(result); err != nil {
		return err
	}

	var extra any
	if d.Decode(&extra) != io.EOF {
		return errors.New("expected one JSON value")
	}

	return nil
}

// hostCall validates guest buffers and dispatches one capability request.
func (r *Runtime) hostCall(ctx context.Context, module api.Module, pointer, length, output, capacity uint32) uint32 {
	// Validate both buffers before executing a side effect. Never let a plugin
	// probe host addresses or cause a write followed by an invalid-buffer retry.
	if !validHostCallBuffers(length, capacity, r.limits.WireBytes) {
		return 0
	}

	input, ok := module.Memory().Read(pointer, length)
	if !ok {
		return 0
	}
	if _, ok := module.Memory().Read(output, capacity); !ok {
		return 0
	}

	response := sdk.CapabilityResponse{}
	value, err := plugin.Guard("host capability", func() (any, error) {
		var request sdk.CapabilityRequest
		if err := decode(input, &request); err != nil {
			return nil, errors.New("invalid capability request")
		}
		return r.dispatch(ctx, module, request)
	})
	if err != nil {
		response.Error = err.Error()
	} else {
		response.Value, err = json.Marshal(value)
		if err != nil {
			response.Error = "cannot encode capability result"
		}
	}
	encoded, err := json.Marshal(response)
	if err != nil || len(encoded) > int(capacity) {
		encoded = []byte(`{"error":"capability response exceeds size limit"}`)
	}
	if len(encoded) > int(capacity) || !module.Memory().Write(output, encoded) {
		return 0
	}

	return uint32(len(encoded))
}

// dispatch executes an allowed capability method for the current plugin.
func (r *Runtime) dispatch(ctx context.Context, module api.Module, request sdk.CapabilityRequest) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	state, ok := ctx.Value(callerKey{}).(*invocationState)
	if !ok || state.instance.module != module {
		return nil, errors.New("capabilities unavailable outside invocation")
	}
	if state.remaining <= 0 {
		return nil, errors.New("host call quota exceeded")
	}

	state.remaining--
	caller := state.instance
	if !r.capabilityAllowed(caller, request.Method) {
		return nil, errors.New("capability denied")
	}

	if request.Method == "http.do" {
		return r.httpCall(ctx, caller, request.Params)
	}
	if request.Method == "plugin.resources.get" || request.Method == "plugin.resources.list" {
		return r.resourceCall(ctx, caller, request)
	}
	if strings.HasPrefix(request.Method, "plugin.") {
		return r.storageCall(ctx, caller, request)
	}
	if request.Method == "log" {
		var message sdk.LogMessage
		if err := decode(request.Params, &message); err != nil || !validLogMessage(message) {
			return nil, errors.New("invalid log message")
		}
		slog.InfoContext(ctx, "plugin message", "plugin_id", caller.manifest.ID, "message", message.Message)
		return nil, nil
	}
	capabilities, _ := ctx.Value(capabilitiesKey{}).(map[string]plugin.Capability)
	capability := capabilities[request.Method]
	if capability == nil {
		return nil, errors.New("capability unavailable in this context")
	}

	return capability(ctx, request.Params)
}

// validHostCallBuffers reports whether guest request and response buffers stay within wire limits.
func validHostCallBuffers(length, capacity uint32, wireBytes int) bool {
	return length > 0 && uint64(length) <= uint64(wireBytes) && capacity >= 256 && uint64(capacity) <= uint64(wireBytes)
}

// capabilityAllowed reports whether a method is known and granted to the calling plugin.
func (r *Runtime) capabilityAllowed(caller *Instance, method string) bool {
	permission, known := sdk.PermissionFor(method)
	if !known {
		return false
	}
	if permission == "" {
		return true
	}

	return r.permissionGranted(caller, permission)
}

// permissionGranted reports whether both host policy and the plugin manifest grant a permission.
func (r *Runtime) permissionGranted(caller *Instance, permission string) bool {
	return r.permissions[permission] && slices.Contains(caller.manifest.Permissions, permission)
}

// resourceCall reads manifest-declared structured settings for the calling plugin.
func (r *Runtime) resourceCall(ctx context.Context, caller *Instance, request sdk.CapabilityRequest) (any, error) {
	if r.storage == nil {
		return nil, errors.New("plugin storage unavailable")
	}

	if request.Method == "plugin.resources.get" {
		var query sdk.PluginResourceRequest
		if err := decode(request.Params, &query); err != nil {
			return nil, errors.New("invalid plugin resource request")
		}
		resource := manifestModule(caller.manifest, query.Resource)
		if resource.Type != "admin-resource" {
			return nil, errors.New("plugin resource is not declared")
		}
		record, found, err := plugin.ReadResourceRecord(ctx, r.storage, caller.manifest.ID, resource, query.Key)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, errors.New("plugin resource record not found")
		}
		record, err = plugin.RevealResourceSecrets(record, resource, r.secrets)
		if err != nil {
			return nil, err
		}
		return sdk.PluginResourceRecord{Key: record.Key, Values: record.Values}, nil
	}

	var query sdk.PluginResourceListRequest
	if err := decode(request.Params, &query); err != nil {
		return nil, errors.New("invalid plugin resource request")
	}
	resource := manifestModule(caller.manifest, query.Resource)
	if resource.Type != "admin-resource" {
		return nil, errors.New("plugin resource is not declared")
	}
	records, err := plugin.ReadResourceRecords(ctx, r.storage, caller.manifest.ID, resource)
	if err != nil {
		return nil, err
	}
	result := make([]sdk.PluginResourceRecord, 0, len(records))
	for _, record := range records {
		revealed, revealErr := plugin.RevealResourceSecrets(record, resource, r.secrets)
		if revealErr != nil {
			return nil, revealErr
		}
		result = append(result, sdk.PluginResourceRecord{Key: revealed.Key, Values: revealed.Values})
	}
	return result, nil
}

// validLogMessage reports whether a plugin log message stays within the wire contract.
func validLogMessage(message sdk.LogMessage) bool {
	return len(message.Message) <= 4096
}

// storageCall executes namespaced plugin settings or data storage operations.
func (r *Runtime) storageCall(ctx context.Context, caller *Instance, request sdk.CapabilityRequest) (any, error) {
	if r.storage == nil {
		return nil, errors.New("plugin storage unavailable")
	}

	var value sdk.StorageValue
	if err := decode(request.Params, &value); err != nil || !validStorageValue(value) {
		return nil, errors.New("invalid storage value")
	}

	id := caller.manifest.ID
	settingsCall := strings.HasPrefix(request.Method, "plugin.settings.")
	if settingsCall && strings.HasSuffix(request.Method, ".read") {
		data, declared, err := plugin.ReadDeclaredSetting(ctx, r.storage, id, caller.manifest, value.Key, r.secrets)
		if declared {
			return sdk.StoredValue{Value: data, Found: err == nil}, err
		}
	}
	if settingsCall && (plugin.DeclaredSetting(caller.manifest, value.Key) || plugin.ReservedSettingStorageKey(value.Key)) {
		return nil, errors.New("manifest-declared settings are administrator managed")
	}

	namespace := "data"
	if settingsCall {
		namespace = "settings"
	}
	if strings.HasSuffix(request.Method, ".read") {
		data, found, err := r.storage.ReadPluginValue(ctx, id, namespace, value.Key)
		if len(data) > 64<<10 {
			return nil, errors.New("stored value exceeds size limit")
		}
		return sdk.StoredValue{Value: data, Found: found}, err
	}

	return nil, r.storage.WritePluginValue(ctx, id, namespace, value.Key, value.Value)
}

// validStorageValue reports whether a plugin storage request uses bounded key and value data.
func validStorageValue(value sdk.StorageValue) bool {
	return len(value.Key) > 0 && len(value.Key) <= 256 && !strings.ContainsRune(value.Key, 0) && len(value.Value) <= 64<<10
}
