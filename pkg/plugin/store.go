package plugin

import (
	"bytes"
	"cmp"
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/kumbuka-me/sdk/pluginpackage"
)

// Record is validated installation metadata. Inventory never includes package bytes.
type Record struct {
	ID       string
	Enabled  bool
	Manifest pluginpackage.Manifest
	Digest   [32]byte
	README   string
}

// Store separates inventory and lifecycle state from package storage.
type Store interface {
	ListPlugins(context.Context) ([]Record, error)
	PluginPackage(context.Context, string) ([]byte, error)
	// SeedPlugin inserts a missing installation atomically; an existing row is never changed.
	// Callers must reload inventory afterward to observe a concurrent winner.
	SeedPlugin(context.Context, Record, []byte) error
	SavePlugin(context.Context, Record, []byte) error
	SetPluginEnabled(context.Context, string, bool) error
	DeletePlugin(context.Context, string) error
}

// SecretCodec encrypts and decrypts manifest-declared plugin secret fields.
type SecretCodec interface {
	// Configured reports whether encryption and decryption are available.
	Configured() bool
	// Encrypt protects one plaintext setting for persistence.
	Encrypt(string) (string, error)
	// Decrypt reveals one persisted setting to its owning plugin.
	Decrypt(string) (string, error)
}

// ManagerOption configures one trusted manager dependency or operator policy.
type ManagerOption func(*Manager)

// WithStore configures durable plugin installation state for a manager.
func WithStore(store Store) ManagerOption { return func(m *Manager) { m.store = store } }

// WithStorage configures namespaced persistent plugin settings and data.
func WithStorage(storage Storage) ManagerOption { return func(m *Manager) { m.values = storage } }

// WithSecretCodec configures encryption for manifest-declared plugin secrets.
func WithSecretCodec(codec SecretCodec) ManagerOption { return func(m *Manager) { m.secrets = codec } }

// memoryStore provides process-local installation persistence for isolated renderers and tests.
type memoryStore struct {
	// mu protects concurrent access to the receiver state.
	mu sync.Mutex
	// records indexes cloned durable records by plugin ID.
	records  map[string]Record
	packages map[string][]byte
}

// ListPlugins returns all durable plugin records.
func (s *memoryStore) ListPlugins(context.Context) ([]Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]Record, 0, len(s.records))
	for _, record := range s.records {
		result = append(result, cloneRecord(record))
	}
	slices.SortFunc(result, func(left, right Record) int {
		return cmp.Compare(left.ID, right.ID)
	})
	return result, nil
}

// SavePlugin creates or replaces one durable plugin record.
func (s *memoryStore) SavePlugin(_ context.Context, record Record, archive []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.packages == nil {
		s.packages = make(map[string][]byte)
	}
	s.packages[record.ID] = bytes.Clone(archive)
	s.records[record.ID] = cloneRecord(record)
	return nil
}

// SeedPlugin atomically inserts a missing installation without replacing its state.
func (s *memoryStore) SeedPlugin(_ context.Context, record Record, archive []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.records[record.ID]; exists {
		return nil
	}
	if s.packages == nil {
		s.packages = make(map[string][]byte)
	}
	s.packages[record.ID] = bytes.Clone(archive)
	s.records[record.ID] = cloneRecord(record)
	return nil
}

// DeletePlugin removes one durable plugin record.
func (s *memoryStore) DeletePlugin(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.records, id)
	delete(s.packages, id)
	return nil
}

// cloneRecord copies mutable metadata so callers cannot mutate stored state.
func cloneRecord(record Record) Record {
	record.Manifest = cloneLoaded(LoadedPlugin{Manifest: record.Manifest}).Manifest
	return record
}

func (s *memoryStore) PluginPackage(_ context.Context, id string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, ok := s.packages[id]
	if !ok {
		return nil, fmt.Errorf("plugin %s is not installed", id)
	}
	return bytes.Clone(data), nil
}
func (s *memoryStore) SetPluginEnabled(_ context.Context, id string, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.records[id]
	if !ok {
		return fmt.Errorf("plugin %s is not installed", id)
	}
	record.Enabled = enabled
	s.records[id] = record
	return nil
}

// WithRequiredPlugins protects operator-selected plugin IDs from disable/removal. A package cannot make itself required by declaring manifest metadata.
func WithRequiredPlugins(ids ...string) ManagerOption {
	required := make(map[string]bool, len(ids))
	for _, id := range ids {
		required[id] = true
	}
	return func(m *Manager) { m.required = required }
}

// IsRequired reports trusted operator policy for a plugin ID.
func (m *Manager) IsRequired(id string) bool { m.mu.Lock(); defer m.mu.Unlock(); return m.required[id] }
