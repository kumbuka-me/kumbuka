package plugin

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/kumbuka-me/kumbuka/pkg/pluginversion"
	"github.com/kumbuka-me/sdk/pluginpackage"
)

// BuiltinPackage identifies an offline candidate, never an active installation.
type BuiltinPackage struct {
	// ID is the plugin identifier.
	ID string
	// Version is the bundled plugin version.
	Version string
	// Digest identifies the exact immutable package contents.
	Digest [32]byte
	// ArchiveName is the embedded package archive filename.
	ArchiveName string
}

// Distribution inventories offline candidates without reading package bytes.
type Distribution interface {
	Catalog() []BuiltinPackage
	Package(context.Context, string) ([]byte, error)
}

// ArchiveDistribution supplies caller-owned packages for isolated renderers.
// Server startup uses the generated embedded catalog instead.
type ArchiveDistribution struct {
	// catalog describes the bundled plugins available from the distribution.
	catalog []BuiltinPackage
	// packages stores bundled plugin archive bytes by plugin identifier.
	packages map[string][]byte
}

// NewArchiveDistribution validates caller-owned archives and constructs an in-memory distribution.
func NewArchiveDistribution(archives [][]byte) (*ArchiveDistribution, error) {
	d := &ArchiveDistribution{packages: make(map[string][]byte)}
	for _, archive := range archives {
		pkg, err := pluginpackage.Read(archive)
		if err != nil {
			return nil, err
		}
		id := pkg.Manifest().ID
		if _, ok := d.packages[id]; ok {
			return nil, fmt.Errorf("duplicate builtin plugin %s", id)
		}
		d.catalog = append(d.catalog, BuiltinPackage{ID: id, Version: pkg.Manifest().Version, Digest: pkg.Digest()})
		d.packages[id] = archive
	}
	return d, nil
}

// Catalog returns a copy of the bundled plugin catalog.
func (d *ArchiveDistribution) Catalog() []BuiltinPackage {
	return slices.Clone(d.catalog)
}

// Package returns the archive bytes for the requested bundled plugin.
func (d *ArchiveDistribution) Package(_ context.Context, id string) ([]byte, error) {
	data, ok := d.packages[id]
	if !ok {
		return nil, fmt.Errorf("builtin plugin %s is unavailable", id)
	}
	return data, nil
}

// validateBuiltin verifies that archive contents match catalog metadata.
func validateBuiltin(info BuiltinPackage, archive []byte) (*pluginpackage.Package, error) {
	pkg, err := pluginpackage.Read(archive)
	if err != nil {
		return nil, err
	}
	if pkg.Manifest().ID != info.ID || pkg.Manifest().Version != info.Version || pkg.Digest() != info.Digest {
		return nil, errors.New("builtin package does not match catalog")
	}
	return pkg, nil
}

// describe enriches loaded plugin metadata with bundled-update information.
func (m *Manager) describe(metadata LoadedPlugin) LoadedPlugin {
	metadata = cloneLoaded(metadata)
	if m.distribution != nil {
		for _, info := range m.distribution.Catalog() {
			if info.ID != metadata.Manifest.ID {
				continue
			}
			metadata.Builtin = &info
			metadata.BuiltinUpdateAvailable = pluginversion.Newer(info.Version, metadata.Manifest.Version)
			metadata.BuiltinMismatch = info.Version == metadata.Manifest.Version && info.Digest != metadata.Digest
			break
		}
	}
	return metadata
}

// BuiltinArchive reads and verifies one explicitly selected offline update.
func (m *Manager) BuiltinArchive(ctx context.Context, id, version string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.distribution != nil {
		for _, info := range m.distribution.Catalog() {
			if info.ID != id || info.Version != version {
				continue
			}
			archive, err := m.distribution.Package(ctx, id)
			if err != nil {
				return nil, err
			}
			if _, err := validateBuiltin(info, archive); err != nil {
				return nil, err
			}
			return archive, nil
		}
	}
	return nil, errors.New("builtin package is unavailable")
}
