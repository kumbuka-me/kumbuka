package plugin

import (
	"bytes"
	"context"
	"sort"
	"sync"
)

// Record is durable installation state. Bundled records contain no archive:
// their distribution bytes always come from the running Kumbuka binary.
type Record struct {
	// ID identifies the associated object.
	ID string
	// Source records whether the durable state refers to bundled or installed bytes.
	Source Source
	// Enabled records whether the plugin should be active after startup.
	Enabled bool
	// Package contains installed archive bytes; bundled records leave it empty.
	Package []byte
}

// Store persists plugin installation and enabled-state records.
type Store interface {
	// ListPlugins returns all durable plugin records.
	ListPlugins(context.Context) ([]Record, error)
	// SavePlugin creates or replaces one durable plugin record.
	SavePlugin(context.Context, Record) error
	// DeletePlugin removes one durable plugin record.
	DeletePlugin(context.Context, string) error
}
type ManagerOption func(*Manager)

// WithStore configures durable plugin installation state for a manager.
func WithStore(store Store) ManagerOption { return func(m *Manager) { m.store = store } }

// WithStorage configures namespaced persistent plugin settings and data.
func WithStorage(storage Storage) ManagerOption { return func(m *Manager) { m.values = storage } }

// memoryStore provides process-local installation persistence for isolated renderers and tests.
type memoryStore struct {
	// mu protects concurrent access to the receiver state.
	mu sync.Mutex
	// records indexes cloned durable records by plugin ID.
	records map[string]Record
}

// ListPlugins returns all durable plugin records.
func (s *memoryStore) ListPlugins(context.Context) ([]Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]Record, 0, len(s.records))
	for _, record := range s.records {
		result = append(result, cloneRecord(record))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

// SavePlugin creates or replaces one durable plugin record.
func (s *memoryStore) SavePlugin(_ context.Context, record Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[record.ID] = cloneRecord(record)
	return nil
}

// DeletePlugin removes one durable plugin record.
func (s *memoryStore) DeletePlugin(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.records, id)
	return nil
}

// cloneRecord copies archive bytes so callers cannot mutate stored state.
func cloneRecord(record Record) Record { record.Package = bytes.Clone(record.Package); return record }

// WithRequiredPlugins protects operator-selected plugin IDs from disable/removal.
// A package cannot make itself required by declaring manifest metadata.
func WithRequiredPlugins(ids ...string) ManagerOption {
	required := make(map[string]bool, len(ids))
	for _, id := range ids {
		required[id] = true
	}
	return func(m *Manager) { m.required = required }
}

// IsRequired reports trusted operator policy for a plugin ID.
func (m *Manager) IsRequired(id string) bool { m.mu.Lock(); defer m.mu.Unlock(); return m.required[id] }
