package plugin

import (
	"context"
	"errors"
	"fmt"
)

// ContentChangeScope constructs mutation-scoped capabilities for one active plugin.
type ContentChangeScope func(Descriptor) Context

// ContentChangeTargets returns active plugins that currently own committed-content hooks.
func (m *Manager) ContentChangeTargets() []Descriptor {
	if m == nil || m.registry == nil {
		return nil
	}
	entries, release := m.registry.AcquireEntries()
	defer release()

	targets := make([]Descriptor, 0, len(entries))
	for _, entry := range entries {
		if len(entry.Contributions.ContentChanges) != 0 {
			targets = append(targets, entry.Descriptor)
		}
	}
	return targets
}

// ContentChanged invokes every active committed-content hook while pinning plugin lifetimes.
func (m *Manager) ContentChanged(
	ctx context.Context,
	request ContentChangeRequest,
	scopeFor ContentChangeScope,
) error {
	if m == nil || m.registry == nil {
		return nil
	}
	var combined error
	entries, release := m.registry.AcquireEntries()
	defer release()
	for _, entry := range entries {
		combined = errors.Join(combined, contentChangedEntry(ctx, entry, request, scopeFor))
	}
	return combined
}

// ContentChangedFor invokes committed-content hooks for one active plugin and ignores plugins no longer active.
func (m *Manager) ContentChangedFor(
	ctx context.Context,
	pluginID string,
	request ContentChangeRequest,
	scopeFor ContentChangeScope,
) error {
	if m == nil || m.registry == nil {
		return nil
	}
	entry, release, ok := m.registry.AcquireEntry(pluginID)
	if !ok {
		return nil
	}
	defer release()
	return contentChangedEntry(ctx, entry, request, scopeFor)
}

// contentChangedEntry invokes every committed-content hook contributed by one leased plugin entry.
func contentChangedEntry(
	ctx context.Context,
	entry Entry,
	request ContentChangeRequest,
	scopeFor ContentChangeScope,
) error {
	var combined error
	for _, module := range entry.Contributions.ContentChanges {
		scope := Context{}
		if scopeFor != nil {
			scope = scopeFor(entry.Descriptor)
		}
		scope.Context = ctx
		_, err := Guard(entry.Descriptor.ID, func() (struct{}, error) {
			return struct{}{}, module.Handler.Changed(scope, request)
		})
		if err != nil {
			combined = errors.Join(combined, fmt.Errorf("plugin %s content change %s: %w", entry.Descriptor.ID, module.ID, err))
		}
	}
	return combined
}
