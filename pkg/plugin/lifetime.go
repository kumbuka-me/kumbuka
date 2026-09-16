package plugin

import (
	"fmt"
	"slices"
	"sync"

	"github.com/kumbuka-me/sdk/pluginpackage"
)

// A lifetime belongs to one contribution version. Render-plan leases retain it,
// so retirement cannot close a reactor while a render still uses it.
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

// AcquireRenderPlan pins the immutable render-only registry view for one complete
// render without cloning contribution metadata on the hot path.
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

func acquireRenderPlanLifetimes(plan *RenderPlan) {
	for _, life := range plan.lifetimes {
		life.acquire()
	}
}

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

// transition validates a candidate before committing persistence or publication.
// Replacement keeps contribution order and is never observable as remove/add.
func (r *Registry) transition(id string, replacement *Entry, replace bool, commit func() error) (*lifetime, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	index := slices.IndexFunc(r.entries, func(e Entry) bool { return e.Descriptor.ID == id })
	if replacement != nil && (index >= 0) != replace {
		if index >= 0 {
			return nil, fmt.Errorf("plugin %s is already registered", id)
		}
		return nil, fmt.Errorf("plugin %s is no longer registered", id)
	}
	candidate := &Registry{entries: slices.Clone(r.entries), renderGeneration: r.renderGeneration}
	if replacement == nil {
		if err := candidate.Unregister(id); err != nil {
			return nil, err
		}
	} else {
		if index >= 0 {
			candidate.entries = slices.Delete(candidate.entries, index, index+1)
		}
		if err := candidate.Register(replacement.Descriptor, replacement.Contributions); err != nil {
			return nil, err
		}
		if index >= 0 {
			added := candidate.entries[len(candidate.entries)-1]
			candidate.entries = candidate.entries[:len(candidate.entries)-1]
			candidate.entries = slices.Insert(candidate.entries, index, added)
			candidate.renderPlan = buildRenderPlan(candidate.entries, candidate.renderGeneration)
		}
	}
	catalog := make(map[string]managedPlugin, len(candidate.entries))
	for _, entry := range candidate.entries {
		catalog[entry.Descriptor.ID] = managedPlugin{metadata: LoadedPlugin{Enabled: true, Manifest: pluginpackage.Manifest{Requires: entry.Descriptor.Requires}}}
	}
	if _, err := dependencyOrder(catalog); err != nil {
		return nil, err
	}
	if commit != nil {
		if err := commit(); err != nil {
			return nil, err
		}
	}
	var old *lifetime
	if index >= 0 {
		old = r.entries[index].lifetime
	}
	r.entries = candidate.entries
	r.renderGeneration = candidate.renderGeneration
	r.renderPlan = candidate.renderPlan
	return old, nil
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

// detach removes all manager-owned versions together during shutdown. Normal
// lifecycle transitions still enforce dependency rules.
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
