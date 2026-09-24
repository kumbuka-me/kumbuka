package wasm_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/kumbuka/pkg/plugin/wasm"
	"github.com/kumbuka-me/sdk"
	"github.com/kumbuka-me/sdk/pluginpackage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// memoryStorage groups the state and data associated with memory storage.
type memoryStorage struct {
	// mu protects concurrent access to the receiver state.
	mu sync.Mutex
	// values indexes the state associated with values.
	values map[string][]byte
}

// ReadPluginValue reads plugin value.
func (s *memoryStorage) ReadPluginValue(_ context.Context, id string, namespace plugin.StorageNamespace, key string) ([]byte, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.values[id+"/"+string(namespace)+"/"+key]
	return bytes.Clone(value), ok, nil
}

// ListPluginValues lists matching plugin values.
func (s *memoryStorage) ListPluginValues(_ context.Context, id string, namespace plugin.StorageNamespace, prefix string) (map[string][]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make(map[string][]byte)
	wanted := id + "/" + string(namespace) + "/" + prefix
	for key, value := range s.values {
		if strings.HasPrefix(key, wanted) {
			result[strings.TrimPrefix(key, id+"/"+string(namespace)+"/")] = bytes.Clone(value)
		}
	}
	return result, nil
}

// WritePluginValue stores one plugin value.
func (s *memoryStorage) WritePluginValue(_ context.Context, id string, namespace plugin.StorageNamespace, key string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[id+"/"+string(namespace)+"/"+key] = bytes.Clone(value)
	return nil
}

// ReplacePluginValue atomically moves one test plugin value while rejecting collisions.
func (s *memoryStorage) ReplacePluginValue(_ context.Context, id string, namespace plugin.StorageNamespace, oldKey, newKey string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	oldPath := id + "/" + string(namespace) + "/" + oldKey
	newPath := id + "/" + string(namespace) + "/" + newKey
	if _, ok := s.values[oldPath]; !ok {
		return plugin.ErrPluginValueNotFound
	}
	if _, ok := s.values[newPath]; ok {
		return plugin.ErrPluginValueAlreadyExists
	}
	delete(s.values, oldPath)
	s.values[newPath] = bytes.Clone(value)
	return nil
}

// DeletePluginValue removes one plugin value.
func (s *memoryStorage) DeletePluginValue(_ context.Context, id string, namespace plugin.StorageNamespace, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.values, id+"/"+string(namespace)+"/"+key)
	return nil
}

// capabilityPackage handles the capability package operation.
func capabilityPackage(t *testing.T, id string, permissions []string) *pluginpackage.Package {
	t.Helper()
	binary, err := fixtureWASM()
	require.NoError(t, err)
	grants, _ := json.Marshal(permissions)
	manifest := fmt.Sprintf("api_version: 1\nid: %s\nname: Fixture\nversion: 1.0.0\nmodules:\n  - type: renderer-extension\n    id: fixture\n    stage: preprocess\npermissions: %s\n", id, grants)
	var output bytes.Buffer
	archive := zip.NewWriter(&output)
	for name, data := range map[string][]byte{"README.md": []byte("# Fixture\n"), "plugin.yaml": []byte(manifest), "plugin.wasm": binary} {
		file, err := archive.Create(name)
		require.NoError(t, err)
		_, err = file.Write(data)
		require.NoError(t, err)
	}
	require.NoError(t, archive.Close())
	pkg, err := pluginpackage.Read(output.Bytes())
	require.NoError(t, err)
	return pkg
}

// hostRequest handles the host request operation.
func hostRequest(t *testing.T, instance plugin.Instance, ctx plugin.Context, method string, params any) string {
	t.Helper()
	data, err := json.Marshal(params)
	require.NoError(t, err)
	request, err := json.Marshal(sdk.CapabilityRequest{Method: method, Params: data})
	require.NoError(t, err)
	result, err := instance.Contributions().Preprocessors[0].Preprocess(ctx, "host:"+string(request))
	require.NoError(t, err)
	return result
}

// TestCapabilityPermissionsAndStorageIsolation verifies capability permissions and storage isolation behavior.
func TestCapabilityPermissionsAndStorageIsolation(t *testing.T) {
	ctx := context.Background()
	permissions := []string{"pages:read", "storage:read", "storage:write", "settings:read", "settings:write"}
	storage := &memoryStorage{values: make(map[string][]byte)}
	runtime, err := wasm.New(ctx, wasm.Limits{}, wasm.WithPermissions(permissions...), wasm.WithStorage(storage))
	require.NoError(t, err)
	defer func() { _ = runtime.Close(ctx) }()
	instances := make([]plugin.Instance, 0, 2)
	for _, id := range []string{"io.one", "io.two"} {
		instance, err := runtime.Load(ctx, capabilityPackage(t, id, permissions))
		require.NoError(t, err)
		defer func() { _ = instance.Close(ctx) }()
		instances = append(instances, instance)
	}
	scope := plugin.Context{Context: ctx}
	for _, source := range []string{`host-raw:{"method":"plugin.storage.read","plugin_id":"io.two"}`, `host-raw:{"method":"log"} {}`} {
		got, err := instances[0].Contributions().Preprocessors[0].Preprocess(scope, source)
		require.NoError(t, err)
		assert.Contains(t, got, "invalid capability request")
	}
	got, err := instances[0].Contributions().Preprocessors[0].Preprocess(scope, "host-invalid-buffer")
	require.NoError(t, err)
	assert.Equal(t, "rejected", got)
	assert.Empty(t, storage.values)
	assert.Equal(t, "null", hostRequest(t, instances[0], scope, "plugin.storage.write", sdk.StorageValue{Key: "key", Value: []byte("one")}))
	assert.Contains(t, hostRequest(t, instances[0], scope, "plugin.storage.read", sdk.StorageValue{Key: "key"}), `"Found":true`)
	assert.Contains(t, hostRequest(t, instances[1], scope, "plugin.storage.read", sdk.StorageValue{Key: "key"}), `"Found":false`)
	assert.Contains(t, hostRequest(t, instances[0], scope, "plugin.settings.read", sdk.StorageValue{Key: "key"}), `"Found":false`)
	assert.Contains(t, hostRequest(t, instances[0], scope, "plugin.storage.read", map[string]any{"Key": "key", "plugin_id": "io.two"}), "invalid storage")
	assert.Contains(t, hostRequest(t, instances[0], scope, "network", nil), "denied")
	assert.Contains(t, hostRequest(t, instances[0], scope, "pages.get", nil), "unavailable")
	ungranted, err := wasm.New(ctx, wasm.Limits{})
	require.NoError(t, err)
	defer func() { _ = ungranted.Close(ctx) }()
	_, err = ungranted.Load(ctx, capabilityPackage(t, "io.denied", permissions))
	require.ErrorContains(t, err, "not granted")
	undeclared, err := runtime.Load(ctx, capabilityPackage(t, "io.undeclared", nil))
	require.NoError(t, err)
	defer func() { _ = undeclared.Close(ctx) }()
	assert.Contains(t, hostRequest(t, undeclared, scope, "plugin.storage.write", sdk.StorageValue{Key: "key"}), "denied")
	assert.Contains(t, hostRequest(t, undeclared, scope, "pages.get", nil), "denied")
}

// TestCapabilitiesUseCurrentRequestAndRecoverFromHostPanic verifies capabilities use current request and recover from host panic behavior.
func TestCapabilitiesUseCurrentRequestAndRecoverFromHostPanic(t *testing.T) {
	ctx := context.Background()
	runtime, err := wasm.New(ctx, wasm.Limits{}, wasm.WithPermissions("pages:read"), wasm.WithInterpreter())
	require.NoError(t, err)
	defer func() { _ = runtime.Close(ctx) }()
	instance, err := runtime.Load(ctx, capabilityPackage(t, "io.scope", []string{"pages:read"}))
	require.NoError(t, err)
	defer func() { _ = instance.Close(ctx) }()
	var workers sync.WaitGroup
	for _, viewer := range []string{"alice", "bob"} {
		workers.Go(func() {
			scope := plugin.Context{Context: ctx, Capabilities: map[string]plugin.Capability{"pages.get": func(context.Context, json.RawMessage) (any, error) { return viewer, nil }}}
			assert.Equal(t, `"`+viewer+`"`, hostRequest(t, instance, scope, "pages.get", nil))
		})
	}
	workers.Wait()
	scope := plugin.Context{Context: ctx, Capabilities: map[string]plugin.Capability{"pages.get": func(context.Context, json.RawMessage) (any, error) { panic("broken adapter") }}}
	assert.Contains(t, hostRequest(t, instance, scope, "pages.get", nil), "panicked")
	scope.Capabilities["pages.get"] = func(context.Context, json.RawMessage) (any, error) { return "healthy", nil }
	assert.Equal(t, `"healthy"`, hostRequest(t, instance, scope, "pages.get", nil))
}

// testSecretCodec provides deterministic encryption for host-capability tests.
type testSecretCodec struct{}

// Configured reports that test encryption is available.
func (testSecretCodec) Configured() bool { return true }

// Encrypt prefixes plaintext with the deterministic test marker.
func (testSecretCodec) Encrypt(value string) (string, error) { return "enc:" + value, nil }

// Decrypt removes the deterministic test marker.
func (testSecretCodec) Decrypt(value string) (string, error) {
	plain, ok := strings.CutPrefix(value, "enc:")
	if !ok {
		return "", errors.New("invalid ciphertext")
	}
	return plain, nil
}

// resourceCapabilityPackage builds a fixture package with one structured secret resource.
func resourceCapabilityPackage(t *testing.T, id string, permissions []string) *pluginpackage.Package {
	t.Helper()
	binary, err := fixtureWASM()
	require.NoError(t, err)
	grants, _ := json.Marshal(permissions)
	manifest := fmt.Sprintf(`api_version: 1
id: %s
name: Fixture
version: 1.0.0
modules:
  - type: renderer-extension
    id: fixture
    stage: preprocess
  - type: admin-resource
    id: sources
    name: Sources
    fields:
      - id: name
        name: Name
        type: text
        required: true
        key: true
      - id: token
        name: Token
        type: secret
permissions: %s
`, id, grants)
	var output bytes.Buffer
	archive := zip.NewWriter(&output)
	for name, data := range map[string][]byte{"README.md": []byte("# Fixture\n"), "plugin.yaml": []byte(manifest), "plugin.wasm": binary} {
		file, createErr := archive.Create(name)
		require.NoError(t, createErr)
		_, writeErr := file.Write(data)
		require.NoError(t, writeErr)
	}
	require.NoError(t, archive.Close())
	pkg, err := pluginpackage.Read(output.Bytes())
	require.NoError(t, err)
	return pkg
}

// TestPluginResourcesRequirePermissionAndRevealSecrets verifies structured resources stay scoped and decrypt only for their owner.
func TestPluginResourcesRequirePermissionAndRevealSecrets(t *testing.T) {
	ctx := context.Background()
	storage := &memoryStorage{values: map[string][]byte{
		`io.resources/settings/r:sources:docs`: []byte(`{"name":"docs","token":"enc:secret"}`),
	}}
	runtime, err := wasm.New(ctx, wasm.Limits{}, wasm.WithInterpreter(), wasm.WithPermissions("settings:read"), wasm.WithStorage(storage), wasm.WithSecretCodec(testSecretCodec{}))
	require.NoError(t, err)
	defer func() { _ = runtime.Close(ctx) }()

	denied, err := runtime.Load(ctx, resourceCapabilityPackage(t, "io.denied-resources", nil))
	require.NoError(t, err)
	assert.Contains(t, hostRequest(t, denied, plugin.Context{Context: ctx}, "plugin.resources.get", sdk.PluginResourceRequest{Resource: "sources", Key: "docs"}), "capability denied")
	require.NoError(t, denied.Close(ctx))

	allowed, err := runtime.Load(ctx, resourceCapabilityPackage(t, "io.resources", []string{"settings:read"}))
	require.NoError(t, err)
	defer func() { _ = allowed.Close(ctx) }()
	result := hostRequest(t, allowed, plugin.Context{Context: ctx}, "plugin.resources.get", sdk.PluginResourceRequest{Resource: "sources", Key: "docs"})
	assert.Contains(t, result, `"token":"secret"`)
	assert.NotContains(t, result, "enc:secret")
}

// TestHTTPRequiresNetworkPermissions verifies generic HTTP and its risky options require explicit manifest grants.
func TestHTTPRequiresNetworkPermissions(t *testing.T) {
	ctx := context.Background()
	runtime, err := wasm.New(
		ctx,
		wasm.Limits{},
		wasm.WithInterpreter(),
		wasm.WithPermissions("network:http", "network:private", "network:insecure-tls"),
		wasm.WithHTTPAuthorizer(func(context.Context) bool { return true }),
	)
	require.NoError(t, err)
	defer func() { _ = runtime.Close(ctx) }()

	denied, err := runtime.Load(ctx, capabilityPackage(t, "io.http-denied", nil))
	require.NoError(t, err)
	assert.Contains(t, hostRequest(t, denied, plugin.Context{Context: ctx}, "http.do", sdk.HTTPRequest{Method: "GET", URL: "https://example.com"}), "capability denied")
	require.NoError(t, denied.Close(ctx))

	basic, err := runtime.Load(ctx, capabilityPackage(t, "io.http-basic", []string{"network:http"}))
	require.NoError(t, err)
	defer func() { _ = basic.Close(ctx) }()
	privateResult := hostRequest(t, basic, plugin.Context{Context: ctx}, "http.do", sdk.HTTPRequest{Method: "GET", URL: "http://10.0.0.1", AllowedPrivateIPs: []string{"10.0.0.1"}})
	assert.Contains(t, privateResult, "capability denied")
	insecureResult := hostRequest(t, basic, plugin.Context{Context: ctx}, "http.do", sdk.HTTPRequest{Method: "GET", URL: "https://example.com", InsecureSkipVerify: true})
	assert.Contains(t, insecureResult, "capability denied")
}

// settingsCapabilityPackage builds a fixture package with one typed singleton settings group.
func settingsCapabilityPackage(t *testing.T, id string, permissions []string) *pluginpackage.Package {
	t.Helper()
	binary, err := fixtureWASM()
	require.NoError(t, err)
	grants, _ := json.Marshal(permissions)
	manifest := fmt.Sprintf(`api_version: 1
id: %s
name: Fixture
version: 1.0.0
modules:
  - type: renderer-extension
    id: fixture
    stage: preprocess
  - type: settings
    id: appearance
    name: Appearance
    fields:
      - id: position
        name: Position
        type: select
        required: true
        default: right
        options: [left, right]
      - id: token
        name: Token
        type: secret
permissions: %s
`, id, grants)
	var output bytes.Buffer
	archive := zip.NewWriter(&output)
	for name, data := range map[string][]byte{"README.md": []byte("# Fixture\n"), "plugin.yaml": []byte(manifest), "plugin.wasm": binary} {
		file, createErr := archive.Create(name)
		require.NoError(t, createErr)
		_, writeErr := file.Write(data)
		require.NoError(t, writeErr)
	}
	require.NoError(t, archive.Close())
	pkg, err := pluginpackage.Read(output.Bytes())
	require.NoError(t, err)
	return pkg
}

// TestTypedSettingsReadDefaultsAndStoredSecrets verifies declared settings are host-managed and returned through the normal Settings API.
func TestTypedSettingsReadDefaultsAndStoredSecrets(t *testing.T) {
	ctx := context.Background()
	storage := &memoryStorage{values: map[string][]byte{
		`io.settings/settings/setting:appearance`: []byte(`{"position":"left","token":"enc:secret"}`),
	}}
	runtime, err := wasm.New(
		ctx,
		wasm.Limits{},
		wasm.WithInterpreter(),
		wasm.WithPermissions("settings:read", "settings:write"),
		wasm.WithStorage(storage),
		wasm.WithSecretCodec(testSecretCodec{}),
	)
	require.NoError(t, err)
	defer func() { _ = runtime.Close(ctx) }()

	instance, err := runtime.Load(ctx, settingsCapabilityPackage(t, "io.settings", []string{"settings:read", "settings:write"}))
	require.NoError(t, err)
	defer func() { _ = instance.Close(ctx) }()
	scope := plugin.Context{Context: ctx}

	position := hostRequest(t, instance, scope, "plugin.settings.read", sdk.StorageValue{Key: "appearance.position"})
	assert.Contains(t, position, `"Value":"bGVmdA=="`)
	secret := hostRequest(t, instance, scope, "plugin.settings.read", sdk.StorageValue{Key: "appearance.token"})
	assert.Contains(t, secret, `"Value":"c2VjcmV0"`)
	assert.NotContains(t, secret, "enc:secret")

	write := hostRequest(t, instance, scope, "plugin.settings.write", sdk.StorageValue{Key: "appearance.position", Value: []byte("right")})
	assert.Contains(t, write, "administrator managed")
	raw := hostRequest(t, instance, scope, "plugin.settings.read", sdk.StorageValue{Key: "setting:appearance"})
	assert.Contains(t, raw, "administrator managed")
}

// TestTypedSettingsReadManifestDefault verifies declared settings return their manifest default before first save.
func TestTypedSettingsReadManifestDefault(t *testing.T) {
	ctx := context.Background()
	storage := &memoryStorage{values: make(map[string][]byte)}
	runtime, err := wasm.New(ctx, wasm.Limits{}, wasm.WithInterpreter(), wasm.WithPermissions("settings:read"), wasm.WithStorage(storage))
	require.NoError(t, err)
	defer func() { _ = runtime.Close(ctx) }()

	instance, err := runtime.Load(ctx, settingsCapabilityPackage(t, "io.defaults", []string{"settings:read"}))
	require.NoError(t, err)
	defer func() { _ = instance.Close(ctx) }()

	result := hostRequest(t, instance, plugin.Context{Context: ctx}, "plugin.settings.read", sdk.StorageValue{Key: "appearance.position"})
	assert.Contains(t, result, `"Value":"cmlnaHQ="`)
	assert.Contains(t, result, `"Found":true`)
}
