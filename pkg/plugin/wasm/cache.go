package wasm

import (
	"context"
	"crypto/sha256"
	"sync"

	"github.com/tetratelabs/wazero"
)

// Cache immutable decoded/compiled modules, not just machine code. Repeated
// CompileModule calls otherwise decode multi-megabyte Go binaries each time.
// Leases keep evicted modules alive until their last instance closes. All
// instances still own separate linear memory and request capabilities.
// Keep enough entries for the complete bundled set plus upgrade/test headroom.
const retainedCodeLimit = 32

var retainedCode []*codeEntry
var codeMu sync.Mutex

// codeEntry owns one cached compiled module and its active lease count.
type codeEntry struct {
	// digest identifies the validated plugin package content.
	digest [32]byte
	// pages records the guest memory limit used to compile this entry.
	pages uint32
	// module holds the active WebAssembly module instance.
	module wazero.CompiledModule
	// refs counts active instances leasing this compiled module.
	refs int
	// retired marks an evicted entry waiting for active leases to close.
	retired bool
}

// compiledLease keeps cached compiled code alive while an instance uses it.
type compiledLease struct {
	// CompiledModule embeds compiled module behavior in compiled lease.
	wazero.CompiledModule
	// entry points to the cached compiled module being leased.
	entry *codeEntry
	// once guarantees that a compiled lease is released at most once.
	once sync.Once
}

// Close releases resources held by the receiver.
func (l *compiledLease) Close(ctx context.Context) error {
	var err error
	l.once.Do(func() {
		if l.entry == nil {
			err = l.CompiledModule.Close(ctx)
			return
		}

		module := releaseRetiredCode(l.entry)
		if module != nil {
			err = module.Close(ctx)
		}
	})
	return err
}

// releaseRetiredCode releases one cache lease and returns code that became
// unreferenced while retired. Resource cleanup deliberately happens after the
// caller leaves the global cache lock.
func releaseRetiredCode(entry *codeEntry) wazero.CompiledModule {
	codeMu.Lock()
	defer codeMu.Unlock()

	entry.refs--
	if entry.retired && entry.refs == 0 {
		return entry.module
	}

	return nil
}

// compile prepares guest code. Compiler mode reuses immutable compiled modules
// across runtimes and single-flights cache misses; interpreter modules stay
// runtime-local because interpreter runtimes do not share a compilation engine.
func (r *Runtime) compile(ctx context.Context, binary []byte) (*compiledLease, error) {
	// Interpreter runtimes don't share an engine through wazero's compilation
	// cache, so their prepared modules stay owned by the creating runtime.
	if r.interpreter {
		module, err := r.engine.CompileModule(ctx, binary)
		if err != nil {
			return nil, err
		}
		if err := validateABI(module); err != nil {
			_ = module.Close(ctx)
			return nil, err
		}
		return &compiledLease{CompiledModule: module}, nil
	}

	digest := sha256.Sum256(binary)
	if lease := retainedLease(digest, r.limits.MemoryPages); lease != nil {
		return lease, nil
	}

	select {
	case compilationGate <- struct{}{}:
		defer func() { <-compilationGate }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Another runtime may have populated the cache while this caller waited.
	if lease := retainedLease(digest, r.limits.MemoryPages); lease != nil {
		return lease, nil
	}

	module, err := r.engine.CompileModule(ctx, binary)
	if err != nil {
		return nil, err
	}
	if err := validateABI(module); err != nil {
		_ = module.Close(ctx)
		return nil, err
	}

	entry, evicted := retainCompiledCode(digest, r.limits.MemoryPages, module)
	if evicted != nil {
		_ = evicted.Close(context.Background())
	}

	return &compiledLease{CompiledModule: module, entry: entry}, nil
}

// retainCompiledCode publishes compiled code and returns an unreferenced
// eviction for cleanup after the cache lock is released.
func retainCompiledCode(digest [32]byte, pages uint32, module wazero.CompiledModule) (*codeEntry, wazero.CompiledModule) {
	codeMu.Lock()
	defer codeMu.Unlock()

	var evicted wazero.CompiledModule
	if len(retainedCode) >= retainedCodeLimit {
		entry := retainedCode[0]
		entry.retired = true
		if entry.refs == 0 {
			evicted = entry.module
		}
		retainedCode = retainedCode[1:]
	}

	entry := &codeEntry{digest: digest, pages: pages, module: module, refs: 1}
	retainedCode = append(retainedCode, entry)

	return entry, evicted
}

// retainedLease returns a lease for cached code and promotes it to the most
// recently used position. The caller receives its own reference count.
func retainedLease(digest [32]byte, pages uint32) *compiledLease {
	codeMu.Lock()
	defer codeMu.Unlock()

	for index, entry := range retainedCode {
		if entry.digest != digest || entry.pages != pages {
			continue
		}
		copy(retainedCode[index:], retainedCode[index+1:])
		retainedCode[len(retainedCode)-1] = entry
		entry.refs++
		return &compiledLease{CompiledModule: entry.module, entry: entry}
	}

	return nil
}
