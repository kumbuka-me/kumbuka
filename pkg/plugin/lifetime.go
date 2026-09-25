package plugin

import (
	"fmt"
	"slices"
	"sync"

	"github.com/kumbuka-me/sdk/pluginpackage"
)

// lifetime tracks one contribution version so retirement cannot close a reactor while a render still uses it.
type lifetime struct {
	// mu protects concurrent access to the receiver state.
	mu sync.Mutex
	// refs counts active executable leases using this contribution version.
	refs int
	// retired prevents this contribution version from accepting new ownership.
	retired bool
	// done closes once retirement has no active render references.
	done chan struct{}
}

// newLifetime creates a lifetime with an open retirement signal.
func newLifetime() *lifetime { return &lifetime{done: make(chan struct{})} }

// acquire retains one active reference to this contribution version.
func (l *lifetime) acquire() { l.mu.Lock(); defer l.mu.Unlock(); l.refs++ }

// release drops one active reference and completes retirement when drained.
func (l *lifetime) release() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.refs--
	if l.retired && l.refs == 0 {
		close(l.done)
	}
}

// retire marks this contribution version retired and returns its drain signal.
func (l *lifetime) retire() <-chan struct{} {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.retired = true
	if l.refs == 0 {
		close(l.done)
	}
	return l.done
}

// AcquireRenderPlan pins the immutable render-only registry view for one complete render without cloning contribution metadata on the hot path.
func (r *Registry) AcquireRenderPlan() (*RenderPlan, func()) {
	r.mu.RLock()
	if r.renderPlan == nil {
		r.mu.RUnlock()
		r.mu.Lock()
		if r.renderPlan == nil {
			r.rebuildRenderPlanLocked()
		}
		plan := r.renderPlan
		acquireRenderPlanLifetimes(plan)
		r.mu.Unlock()
		return plan, releaseRenderPlan(plan)
	}
	plan := r.renderPlan
	acquireRenderPlanLifetimes(plan)
	r.mu.RUnlock()
	return plan, releaseRenderPlan(plan)
}

// acquireRenderPlanLifetimes acquires every plugin lifetime referenced by a render plan.
func acquireRenderPlanLifetimes(plan *RenderPlan) {
	for _, life := range plan.lifetimes {
		life.acquire()
	}
}

// releaseRenderPlan releases every plugin lifetime held by a render plan lease.
func releaseRenderPlan(plan *RenderPlan) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			for _, life := range plan.lifetimes {
				life.release()
			}
		})
	}
}

// AcquireEntry pins one active plugin entry while a non-render operation invokes it.
func (r *Registry) AcquireEntry(id string) (Entry, func(), bool) {
	r.mu.RLock()
	index := slices.IndexFunc(r.entries, func(entry Entry) bool { return entry.Descriptor.ID == id })
	if index < 0 {
		r.mu.RUnlock()
		return Entry{}, func() {}, false
	}

	entry := cloneEntry(r.entries[index])
	entry.lifetime.acquire()
	r.mu.RUnlock()

	var once sync.Once
	return entry, func() { once.Do(entry.lifetime.release) }, true
}

// AcquireEntries pins a deterministic snapshot of every active plugin for a non-render operation.
func (r *Registry) AcquireEntries() ([]Entry, func()) {
	r.mu.RLock()
	entries := make([]Entry, len(r.entries))
	for index, entry := range r.entries {
		entries[index] = cloneEntry(entry)
		entries[index].lifetime.acquire()
	}
	r.mu.RUnlock()

	var once sync.Once
	return entries, func() {
		once.Do(func() {
			for _, entry := range entries {
				entry.lifetime.release()
			}
		})
	}
}

// transition validates a candidate before committing persistence or publication. Replacement keeps contribution order and is never observable as remove/add.
func (r *Registry) transition(id string, replacement *Entry, replace bool, commit func() error) (*lifetime, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	index := slices.IndexFunc(r.entries, func(entry Entry) bool { return entry.Descriptor.ID == id })
	if err := validateTransitionPresence(id, replacement, replace, index); err != nil {
		return nil, err
	}
	candidate, err := r.transitionCandidate(id, replacement, index)
	if err != nil {
		return nil, err
	}
	if err := validateTransitionDependencies(candidate.entries); err != nil {
		return nil, err
	}
	if commit != nil {
		if err := commit(); err != nil {
			return nil, err
		}
	}

	old := transitionLifetime(r.entries, index)
	r.entries = candidate.entries
	r.renderGeneration = candidate.renderGeneration
	r.renderPlan = candidate.renderPlan
	return old, nil
}

// validateTransitionPresence verifies create-versus-replace expectations against the active registry.
func validateTransitionPresence(id string, replacement *Entry, replace bool, index int) error {
	if replacement == nil || (index >= 0) == replace {
		return nil
	}
	if index >= 0 {
		return fmt.Errorf("plugin %s is already registered", id)
	}
	return fmt.Errorf("plugin %s is no longer registered", id)
}

// transitionCandidate applies one registry mutation to an isolated candidate snapshot.
func (r *Registry) transitionCandidate(id string, replacement *Entry, index int) (*Registry, error) {
	candidate := &Registry{entries: slices.Clone(r.entries), renderGeneration: r.renderGeneration}
	if replacement == nil {
		if err := candidate.Unregister(id); err != nil {
			return nil, err
		}
		return candidate, nil
	}

	if index >= 0 {
		candidate.entries = slices.Delete(candidate.entries, index, index+1)
	}
	if err := candidate.Register(replacement.Descriptor, replacement.Contributions); err != nil {
		return nil, err
	}
	if index >= 0 {
		moveNewestEntry(candidate, index)
	}
	return candidate, nil
}

// moveNewestEntry restores the replaced plugin's contribution position.
func moveNewestEntry(candidate *Registry, index int) {
	added := candidate.entries[len(candidate.entries)-1]
	candidate.entries = candidate.entries[:len(candidate.entries)-1]
	candidate.entries = slices.Insert(candidate.entries, index, added)
	candidate.renderPlan = buildRenderPlan(candidate.entries, candidate.renderGeneration)
}

// validateTransitionDependencies verifies the candidate registry's dependency graph.
func validateTransitionDependencies(entries []Entry) error {
	catalog := make(map[string]managedPlugin, len(entries))
	for _, entry := range entries {
		catalog[entry.Descriptor.ID] = managedPlugin{metadata: LoadedPlugin{Enabled: true, Manifest: pluginpackage.Manifest{Requires: entry.Descriptor.Requires}}}
	}
	_, err := dependencyOrder(catalog)
	return err
}

// transitionLifetime returns the lifetime replaced or removed at index.
func transitionLifetime(entries []Entry, index int) *lifetime {
	if index < 0 {
		return nil
	}
	return entries[index].lifetime
}

// initialize replaces the registry contents during atomic startup publication.
func (r *Registry) initialize(entries []Entry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.entries) != 0 {
		return fmt.Errorf("registry is not empty")
	}
	r.publishEntriesLocked(entries)
	return nil
}

// detach removes all manager-owned versions together during shutdown. Normal lifecycle transitions still enforce dependency rules.
func (r *Registry) detach(ids map[string]managedPlugin) map[string]*lifetime {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make(map[string]*lifetime)
	entries := slices.Clone(r.entries)
	entries = slices.DeleteFunc(entries, func(entry Entry) bool {
		if _, owned := ids[entry.Descriptor.ID]; !owned {
			return false
		}
		result[entry.Descriptor.ID] = entry.lifetime
		return true
	})
	if len(entries) != len(r.entries) {
		r.publishEntriesLocked(entries)
	}
	return result
}
