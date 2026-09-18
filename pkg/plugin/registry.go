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
	defer func() {
		if recover() != nil {
			err = fmt.Errorf("plugin %s panicked during registration", descriptor.ID)
		}
	}()
	r.mu.Lock()
	defer r.mu.Unlock()

	if !validID.MatchString(descriptor.ID) || descriptor.Name == "" {
		return fmt.Errorf("invalid plugin descriptor %q", descriptor.ID)
	}
	active := make(map[string]bool)
	names := make(map[string]bool)
	activeHighlighter := ""
	for _, entry := range r.entries {
		active[entry.Descriptor.ID] = true
		if len(entry.Contributions.CodeHighlighters) != 0 {
			activeHighlighter = entry.Descriptor.ID
		}
		for _, macro := range entry.Contributions.Macros {
			names[macro.Name()] = true
		}
	}

	if active[descriptor.ID] {
		return fmt.Errorf("plugin %s is already registered", descriptor.ID)
	}
	for _, dependency := range descriptor.Requires {
		if !active[dependency] {
			return fmt.Errorf("plugin %s requires active plugin %s", descriptor.ID, dependency)
		}
	}
	for _, macro := range modules.Macros {
		if macro == nil || !validID.MatchString(macro.Name()) {
			return fmt.Errorf("invalid macro in plugin %s", descriptor.ID)
		}
		if names[macro.Name()] {
			return fmt.Errorf("macro %s is already registered", macro.Name())
		}
		names[macro.Name()] = true
	}
	for _, module := range modules.ContentPreprocessors {
		if module == nil {
			return fmt.Errorf("nil content preprocessor in %s", descriptor.ID)
		}
	}
	for _, module := range modules.Preprocessors {
		if module == nil {
			return fmt.Errorf("nil preprocessor in %s", descriptor.ID)
		}
	}
	for _, module := range modules.MarkdownExtensions {
		if module == nil {
			return fmt.Errorf("nil Markdown extension in %s", descriptor.ID)
		}
	}
	for _, module := range modules.Postprocessors {
		if module == nil {
			return fmt.Errorf("nil postprocessor in %s", descriptor.ID)
		}
	}
	if len(modules.CodeHighlighters) > 1 {
		return fmt.Errorf("plugin %s contributes more than one code highlighter", descriptor.ID)
	}
	if len(modules.CodeHighlighters) == 1 {
		if modules.CodeHighlighters[0].Highlighter == nil {
			return fmt.Errorf("nil code highlighter in %s", descriptor.ID)
		}
		if activeHighlighter != "" {
			return fmt.Errorf("code highlighter is already provided by plugin %s", activeHighlighter)
		}
	}

	if err := validateIDs(modules); err != nil {
		return err
	}

	entries := append(slices.Clone(r.entries), cloneEntry(Entry{Descriptor: descriptor, Contributions: modules, lifetime: newLifetime()}))
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

// validateIDs validates unique IDs for metadata-only contribution modules.
func validateIDs(c Contributions) error {
	seen := make(map[string]bool)

	check := func(kind, id string) error {
		key := kind + ":" + id
		if !validID.MatchString(id) || seen[key] {
			return fmt.Errorf("invalid or duplicate %s ID %q", kind, id)
		}
		seen[key] = true
		return nil
	}

	for _, m := range c.CodeHighlighters {
		if err := check("code-highlighter", m.ID); err != nil {
			return err
		}
	}
	for _, m := range c.Widgets {
		if err := check("widget", m.ID); err != nil {
			return err
		}
	}
	for _, m := range c.Exporters {
		if m.Exporter == nil {
			return fmt.Errorf("nil exporter %q", m.ID)
		}
		if err := check("exporter", m.ID); err != nil {
			return err
		}
	}
	for _, m := range c.BrowserModules {
		if err := check("browser", m.ID); err != nil {
			return err
		}
	}
	for _, m := range c.EditorExtensions {
		if err := check("editor", m.ID); err != nil {
			return err
		}
	}
	for _, m := range c.AdminActions {
		if m.Action == nil {
			return fmt.Errorf("nil admin action %q", m.ID)
		}
		if err := check("admin-action", m.ID); err != nil {
			return err
		}
	}
	for _, m := range c.AdminResources {
		if err := check("admin-resource", m.ID); err != nil {
			return err
		}
	}
	for _, m := range c.EditorCompletions {
		if err := check("editor-completion", m.ID); err != nil {
			return err
		}
	}
	for _, m := range c.EditorInserts {
		if err := check("editor-insert", m.ID); err != nil {
			return err
		}
	}
	for _, m := range c.SettingsModules {
		if err := check("settings", m.ID); err != nil {
			return err
		}
	}
	for _, m := range c.ContentStyles {
		if err := check("content-style", m.ID); err != nil {
			return err
		}
	}
	for _, m := range c.RenderPolicies {
		if err := check("render-policy", m.ID); err != nil {
			return err
		}
	}

	return nil
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
