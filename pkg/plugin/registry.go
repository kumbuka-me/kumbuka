package plugin

import (
	"fmt"
	"regexp"
	"slices"
	"sync"
)

// Registry owns active contributions. Its zero value is ready for use.
// Registration is atomic; renderers lease immutable render plans rather than hold
// registry locks while invoking modules. A removed module may finish an already-started render.
type Registry struct {
	// mu protects concurrent access to the receiver state.
	mu sync.RWMutex
	// entries contains active plugin contributions in deterministic order.
	entries []Entry
	// renderPlan is the immutable render-only view for the current lifecycle generation.
	renderPlan *RenderPlan
	// renderGeneration increments whenever active contributions change.
	renderGeneration uint64
}

// Entry associates an immutable contribution set with its owner.
type Entry struct {
	// lifetime tracks render leases for this contribution version.
	lifetime *lifetime
	// Descriptor identifies the plugin that owns these contributions.
	Descriptor Descriptor
	// Contributions contains the plugin modules published atomically with this entry.
	Contributions Contributions
}

// Snapshot is an isolated view of active modules in deterministic order.
// Use Acquire for executable snapshots that must survive lifecycle changes.
// Callbacks are shared and must be concurrency safe; all metadata slices are copied.
type Snapshot struct {
	// Entries contains cloned active entries in deterministic registry order.
	Entries []Entry
}

var validID = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// Register publishes all contributions or none. Required plugins must already
// be active, so load order is explicit and dependency cycles cannot be introduced.
func (r *Registry) Register(descriptor Descriptor, modules Contributions) (err error) {
	defer recoverRegistrationPanic(&err, descriptor.ID)

	r.mu.Lock()
	defer r.mu.Unlock()

	if err := validateRegistration(descriptor, modules, r.entries); err != nil {
		return err
	}

	entry := Entry{
		Descriptor:    descriptor,
		Contributions: modules,
		lifetime:      newLifetime(),
	}
	entries := append(slices.Clone(r.entries), cloneEntry(entry))
	r.publishEntriesLocked(entries)

	return nil
}

// Unregister removes every contribution owned by id. Dependents must be removed
// first. Existing executable leases stay valid; future renders see the removal.
func (r *Registry) Unregister(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	index := slices.IndexFunc(r.entries, func(entry Entry) bool { return entry.Descriptor.ID == id })
	if index < 0 {
		return fmt.Errorf("plugin %s is not registered", id)
	}

	for _, entry := range r.entries {
		if slices.Contains(entry.Descriptor.Requires, id) {
			return fmt.Errorf("plugin %s requires %s", entry.Descriptor.ID, id)
		}
	}

	entries := slices.Delete(slices.Clone(r.entries), index, index+1)
	r.publishEntriesLocked(entries)
	return nil
}

// Snapshot returns an inspection snapshot of the current registry state.
func (r *Registry) Snapshot() Snapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()

	snapshot := Snapshot{Entries: make([]Entry, len(r.entries))}
	for index, entry := range r.entries {
		snapshot.Entries[index] = cloneEntry(entry)
	}

	return snapshot
}

// cloneEntry copies contribution metadata slices while sharing immutable callbacks.
func cloneEntry(entry Entry) Entry {
	entry.Descriptor.Requires = slices.Clone(entry.Descriptor.Requires)
	c := &entry.Contributions
	c.ContentPreprocessors = slices.Clone(c.ContentPreprocessors)
	c.Preprocessors = slices.Clone(c.Preprocessors)
	c.MarkdownExtensions = slices.Clone(c.MarkdownExtensions)
	c.CodeHighlighters = slices.Clone(c.CodeHighlighters)
	c.Postprocessors = slices.Clone(c.Postprocessors)
	c.Macros = slices.Clone(c.Macros)
	c.Widgets = slices.Clone(c.Widgets)
	c.Exporters = slices.Clone(c.Exporters)
	c.BrowserModules = slices.Clone(c.BrowserModules)
	c.EditorExtensions = slices.Clone(c.EditorExtensions)
	c.AdminActions = slices.Clone(c.AdminActions)
	c.AdminResources = slices.Clone(c.AdminResources)
	c.EditorCompletions = slices.Clone(c.EditorCompletions)
	c.EditorInserts = slices.Clone(c.EditorInserts)
	c.SettingsModules = slices.Clone(c.SettingsModules)
	c.ContentStyles = slices.Clone(c.ContentStyles)
	c.RenderPolicies = slices.Clone(c.RenderPolicies)
	for i := range c.SettingsModules {
		c.SettingsModules[i].Requires = slices.Clone(c.SettingsModules[i].Requires)
	}

	return entry
}

// CodeHighlighter returns the single active highlighter contribution, when present.
func (s Snapshot) CodeHighlighter() (string, CodeHighlighterModule, bool) {
	for _, entry := range s.Entries {
		if len(entry.Contributions.CodeHighlighters) != 0 {
			return entry.Descriptor.ID, entry.Contributions.CodeHighlighters[0], true
		}
	}

	return "", CodeHighlighterModule{}, false
}
