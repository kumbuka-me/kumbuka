// Package wasm adapts untrusted WASI reactors to Kumbuka's module contracts.
package wasm

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/sdk"
	"github.com/kumbuka-me/sdk/pluginpackage"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

// Limits bounds individual sandbox calls, guest memory, and wire payloads. Each page is 64 KiB. Zero fields use conservative defaults.
type Limits struct {
	// MemoryPages is the maximum guest memory in 64 KiB WebAssembly pages.
	MemoryPages uint32
	// CallTimeout bounds one guest invocation.
	CallTimeout time.Duration
	// LoadTimeout bounds compilation before module initialization.
	LoadTimeout time.Duration
	// InitTimeout bounds one module initialization.
	InitTimeout time.Duration
	// WireBytes bounds request, response, and assembled output payloads.
	WireBytes int
	// Parts bounds the number of render fragments returned by one guest call.
	Parts int
}

// defaults fills unset resource limits with conservative runtime defaults.
func (l Limits) defaults() Limits {
	if l.MemoryPages == 0 {
		l.MemoryPages = 1024
	}
	if l.CallTimeout <= 0 {
		l.CallTimeout = 2 * time.Second
	}
	if l.LoadTimeout <= 0 {
		l.LoadTimeout = 60 * time.Second
	}
	if l.InitTimeout <= 0 {
		l.InitTimeout = 2 * time.Second
	}
	if l.WireBytes <= 0 {
		l.WireBytes = 4 << 20
	}
	if l.Parts <= 0 {
		l.Parts = 256
	}
	return l
}

// The compilation cache shares machine code, never registries, guest memory,
// request state, or capabilities. Prefer wazero's persistent cache so unchanged
// guests do not have to be compiled again after Kumbuka restarts.
var (
	compilationCacheOnce sync.Once
	compilationCache     wazero.CompilationCache
)

// sharedCompilationCache returns one process-wide cache backed by the user's normal cache directory when available. Read-only or unusual environments fall back to the in-memory cache instead of preventing Kumbuka from starting.
func sharedCompilationCache() wazero.CompilationCache {
	compilationCacheOnce.Do(func() {
		if root, err := os.UserCacheDir(); err == nil {
			cache, cacheErr := wazero.NewCompilationCacheWithDir(filepath.Join(root, "kumbuka", "wasm"))
			if cacheErr == nil {
				compilationCache = cache
				return
			}
		}

		compilationCache = wazero.NewCompilationCache()
	})

	return compilationCache
}

// Wazero's cache does not single-flight concurrent compilations. Serialize cache
// misses so parallel startup cannot compile the same binary repeatedly. Cache
// hits bypass this gate; rendering and guest state are never shared here.
var compilationGate = make(chan struct{}, 1)

// InvocationObserver receives bounded metadata for completed executable plugin calls.
type InvocationObserver interface {
	// ObservePluginInvocation records one completed executable plugin call.
	ObservePluginInvocation(pluginID, moduleID, stage string, duration time.Duration, err error)
}

// UserDirectory provides privacy-safe user lookup capabilities to plugins.
type UserDirectory interface {
	// Search returns enabled users matching one bounded query.
	Search(context.Context, sdk.UserQuery) ([]sdk.User, error)
	// ResolveMention resolves one canonical or case-insensitive mention.
	ResolveMention(context.Context, sdk.UserMention) (sdk.User, error)
}

// Runtime owns the wazero engine and trusted host policy used for plugin instances.
type Runtime struct {
	// engine owns compiled modules and instantiated WASM guests.
	engine wazero.Runtime
	// limits contains effective runtime resource bounds.
	limits Limits
	// permissions contains host capabilities explicitly granted by application policy.
	permissions map[string]bool
	// storage provides persistent plugin state storage.
	storage plugin.Storage
	// users provides the privacy-safe Kumbuka user directory.
	users UserDirectory
	// secrets decrypts manifest-declared plugin secret configuration for its owning guest.
	secrets plugin.SecretCodec
	// httpAuthorizer decides whether the current invocation may perform outbound network I/O.
	httpAuthorizer func(context.Context) bool
	// httpActive bounds concurrent outbound plugin requests across the runtime.
	httpActive chan struct{}
	// logger receives debug-only plugin initialization timings when configured.
	logger *slog.Logger
	// invocationObserver receives completed executable plugin call measurements.
	invocationObserver InvocationObserver
	// interpreter forces wazero's interpreter instead of AOT compilation.
	interpreter bool
}

// Option configures trusted application policy, equally for every source.
type Option func(*Runtime)

// WithPermissions grants host capabilities that packages may declare and request.
func WithPermissions(permissions ...string) Option {
	return func(r *Runtime) {
		for _, p := range permissions {
			r.permissions[p] = true
		}
	}
}

// WithStorage provides namespaced persistent settings and data storage to plugins.
func WithStorage(storage plugin.Storage) Option { return func(r *Runtime) { r.storage = storage } }

// WithUserDirectory provides privacy-safe user lookup capabilities to plugins.
func WithUserDirectory(users UserDirectory) Option { return func(r *Runtime) { r.users = users } }

// WithSecretCodec provides encryption for manifest-declared secret configuration fields.
func WithSecretCodec(codec plugin.SecretCodec) Option { return func(r *Runtime) { r.secrets = codec } }

// WithHTTPAuthorizer enables outbound HTTP only for invocation contexts accepted by authorize.
func WithHTTPAuthorizer(authorize func(context.Context) bool) Option {
	return func(r *Runtime) { r.httpAuthorizer = authorize }
}

// WithLogger enables runtime diagnostics such as per-plugin initialization timings. Timing messages use DEBUG level, so normal application logging remains unchanged.
func WithLogger(logger *slog.Logger) Option { return func(r *Runtime) { r.logger = logger } }

// WithInvocationObserver records completed executable plugin calls without coupling the runtime to a metrics implementation.
func WithInvocationObserver(observer InvocationObserver) Option {
	return func(r *Runtime) { r.invocationObserver = observer }
}

// WithInterpreter uses wazero's interpreter instead of AOT compilation. It is useful for tests that exercise guest behavior without benchmarking compilation.
func WithInterpreter() Option { return func(r *Runtime) { r.interpreter = true } }

// New creates a WASM plugin runtime with bounded resources and explicit host policy.
func New(ctx context.Context, limits Limits, options ...Option) (*Runtime, error) {
	limits = limits.defaults()

	if !validLimits(limits) {
		return nil, errors.New("invalid WASM runtime limits")
	}

	r := &Runtime{limits: limits, permissions: make(map[string]bool), httpActive: make(chan struct{}, 16)}
	for _, option := range options {
		option(r)
	}

	config := wazero.NewRuntimeConfig()
	if r.interpreter {
		config = wazero.NewRuntimeConfigInterpreter()
	} else {
		config = config.WithCompilationCache(sharedCompilationCache())
	}
	config = config.WithMemoryLimitPages(limits.MemoryPages).WithCloseOnContextDone(true)
	engine := wazero.NewRuntimeWithConfig(ctx, config)
	// No filesystem preopens, environment, process arguments, sockets, or host
	// streams are configured. WASI descriptors cannot access Kumbuka's resources.
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, engine); err != nil {
		_ = engine.Close(ctx)
		return nil, err
	}

	r.engine = engine

	if _, err := engine.NewHostModuleBuilder("kumbuka_v1").NewFunctionBuilder().WithFunc(r.hostCall).Export("call").Instantiate(ctx); err != nil {
		_ = engine.Close(ctx)
		return nil, err
	}

	return r, nil
}

// validLimits reports whether effective runtime limits stay within hard safety ceilings.
func validLimits(limits Limits) bool {
	return limits.MemoryPages <= 65536 && limits.WireBytes <= 16<<20 && limits.Parts <= 4096
}

// Load validates policy and returns one isolated plugin instance. Declarative packages never compile or instantiate WASM.
func (r *Runtime) Load(ctx context.Context, pkg *pluginpackage.Package) (loaded plugin.Instance, err error) {
	manifest := pkg.Manifest()
	started := time.Now()
	if r.logger != nil {
		defer func() {
			attributes := []any{
				"event", "plugin_initialized",
				"plugin_id", manifest.ID,
				"plugin_name", manifest.Name,
				"wasm", manifest.RequiresWASM(),
				"duration", time.Since(started),
			}
			if err != nil {
				attributes[1] = "plugin_initialization_failed"
				attributes = append(attributes, "error", err)
				r.logger.Debug("plugin initialization failed", attributes...)
				return
			}
			r.logger.Debug("plugin initialized", attributes...)
		}()
	}

	for _, permission := range manifest.Permissions {
		if !r.permissions[permission] {
			return nil, fmt.Errorf("plugin permission not granted: %s", permission)
		}
	}

	instance := &Instance{runtime: r, manifest: manifest, gate: make(chan struct{}, 1)}
	if !manifest.RequiresWASM() {
		return instance, nil
	}

	ctx, cancel := context.WithTimeout(ctx, r.limits.LoadTimeout)
	defer cancel()

	binary := pkg.WASM()
	compiled, err := r.compile(ctx, binary)
	if err != nil {
		return nil, fmt.Errorf("compile WASM: %w", err)
	}

	instance.compiled = compiled
	initializeCtx, stop := context.WithTimeout(ctx, r.limits.InitTimeout)
	defer stop()

	if err := instance.instantiate(initializeCtx); err != nil {
		_ = compiled.Close(ctx)
		return nil, err
	}

	return instance, nil
}

// Close releases resources held by the receiver.
func (r *Runtime) Close(ctx context.Context) error { return r.engine.Close(ctx) }

// validateABI verifies the guest exports and API version required by Kumbuka.
func validateABI(compiled wazero.CompiledModule) error {
	if len(compiled.ImportedMemories()) != 0 {
		return errors.New("imported WASM memory is not allowed")
	}

	for _, imported := range compiled.ImportedFunctions() {
		namespace, name, _ := imported.Import()
		if !validImport(namespace, name, imported) {
			return fmt.Errorf("unsupported WASM import %s.%s", namespace, name)
		}
	}
	if compiled.ExportedMemories()["memory"] == nil {
		return errors.New("plugin must export memory")
	}

	signatures := []struct {
		// name is the required ABI export.
		name string
		// params lists the export's parameter types in call order.
		params []api.ValueType
		// results lists the export's return types in result order.
		results []api.ValueType
	}{
		{"_initialize", nil, nil},
		{"kumbuka_api_version", nil, []api.ValueType{api.ValueTypeI32}},
		{"kumbuka_alloc", []api.ValueType{api.ValueTypeI32}, []api.ValueType{api.ValueTypeI32}},
		{"kumbuka_transform", []api.ValueType{api.ValueTypeI32, api.ValueTypeI32}, []api.ValueType{api.ValueTypeI64}},
	}

	for _, signature := range signatures {
		function := compiled.ExportedFunctions()[signature.name]
		if !matchesSignature(function, signature.params, signature.results) {
			return fmt.Errorf("missing or incompatible WASM export %s", signature.name)
		}
	}

	return nil
}

// instantiate creates one isolated reactor from validated compiled code.
func (i *Instance) instantiate(ctx context.Context) error {
	// Anonymous instances cannot be imported by another plugin. Only _initialize
	// is invoked, and it shares the load/call deadline and memory limit.
	// Widgets compare stored timestamps with time.Now, so the WASI wall clock
	// must reflect real time rather than wazero's default simulated epoch.
	config := wazero.NewModuleConfig().WithName("").WithStartFunctions("_initialize").WithSysWalltime()
	module, err := i.runtime.engine.InstantiateModule(ctx, i.compiled.CompiledModule, config)
	if err != nil {
		return fmt.Errorf("initialize WASM: %w", err)
	}

	version, err := module.ExportedFunction("kumbuka_api_version").Call(ctx)
	if err != nil || !compatibleAPIVersion(version) {
		_ = module.Close(context.Background())
		return errors.New("incompatible WASM plugin API version")
	}

	i.module = module

	return nil
}

// validImport reports whether a guest import belongs to WASI or Kumbuka's explicit host ABI.
func validImport(namespace, name string, function api.FunctionDefinition) bool {
	return namespace == wasi_snapshot_preview1.ModuleName || validHostImport(namespace, name, function)
}

// matchesSignature reports whether a WASM function has the expected parameter and result types.
func matchesSignature(function api.FunctionDefinition, params, results []api.ValueType) bool {
	return function != nil && slices.Equal(function.ParamTypes(), params) && slices.Equal(function.ResultTypes(), results)
}

// compatibleAPIVersion reports whether the guest returned exactly Kumbuka's supported ABI version.
func compatibleAPIVersion(version []uint64) bool {
	return len(version) == 1 && version[0] == sdk.Version
}

// validHostImport reports whether an imported function is part of Kumbuka's allowed host ABI.
func validHostImport(namespace, name string, function api.FunctionDefinition) bool {
	if namespace != "kumbuka_v1" || name != "call" {
		return false
	}

	params := []api.ValueType{api.ValueTypeI32, api.ValueTypeI32, api.ValueTypeI32, api.ValueTypeI32}
	results := []api.ValueType{api.ValueTypeI32}
	return matchesSignature(function, params, results)
}
