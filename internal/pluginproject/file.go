// Package pluginproject manages reproducible plugin dependencies for static Kumbuka projects.
package pluginproject

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

const (
	// DefaultFile is the conventional project plugin dependency file.
	DefaultFile = ".kumbukaplugins"
	fileFormat  = 1
)

var (
	pluginIDPattern   = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,127}$`)
	repositoryPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	tagPrefixPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)
	assetPattern      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	versionPattern    = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
)

// File is the versioned .kumbukaplugins project dependency manifest.
type File struct {
	Format  int          `toml:"format"`
	Plugins []Dependency `toml:"plugin"`
}

// Dependency pins one GitHub Release plugin package.
type Dependency struct {
	ID         string `toml:"id"`
	Repository string `toml:"repository"`
	TagPrefix  string `toml:"tag_prefix"`
	Asset      string `toml:"asset"`
	Version    string `toml:"version"`
}

// Load reads a project dependency file. A missing file is an empty project.
func Load(filename string) (File, error) {
	data, err := os.ReadFile(filename)
	if errors.Is(err, os.ErrNotExist) {
		return File{Format: fileFormat}, nil
	}
	if err != nil {
		return File{}, err
	}

	file := File{Format: fileFormat}
	decoder := toml.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&file); err != nil {
		return File{}, fmt.Errorf("parse %s: %w", filename, err)
	}
	if file.Format != fileFormat {
		return File{}, fmt.Errorf("unsupported %s format %d", filename, file.Format)
	}

	seen := make(map[string]bool, len(file.Plugins))
	for index := range file.Plugins {
		dependency, err := NormalizeDependency(file.Plugins[index])
		if err != nil {
			return File{}, fmt.Errorf("plugin %d: %w", index+1, err)
		}
		if seen[dependency.ID] {
			return File{}, fmt.Errorf("duplicate plugin %s", dependency.ID)
		}
		seen[dependency.ID] = true
		file.Plugins[index] = dependency
	}

	slices.SortFunc(file.Plugins, func(left, right Dependency) int {
		return strings.Compare(left.ID, right.ID)
	})

	return file, nil
}

// Save writes a canonical dependency file atomically.
func Save(filename string, file File) error {
	file.Format = fileFormat
	seen := make(map[string]bool, len(file.Plugins))
	for index := range file.Plugins {
		dependency, err := NormalizeDependency(file.Plugins[index])
		if err != nil {
			return fmt.Errorf("plugin %d: %w", index+1, err)
		}
		if seen[dependency.ID] {
			return fmt.Errorf("duplicate plugin %s", dependency.ID)
		}
		seen[dependency.ID] = true
		file.Plugins[index] = dependency
	}
	slices.SortFunc(file.Plugins, func(left, right Dependency) int {
		return strings.Compare(left.ID, right.ID)
	})

	data, err := toml.Marshal(file)
	if err != nil {
		return err
	}

	directory := filepath.Dir(filename)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".kumbukaplugins-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer func() { _ = os.Remove(temporaryName) }()

	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}

	return os.Rename(temporaryName, filename)
}

// NormalizeDependency validates and canonicalizes one dependency declaration.
func NormalizeDependency(dependency Dependency) (Dependency, error) {
	dependency.ID = strings.TrimSpace(dependency.ID)
	dependency.Repository = strings.TrimSuffix(strings.TrimSpace(dependency.Repository), ".git")
	dependency.TagPrefix = strings.TrimSpace(dependency.TagPrefix)
	dependency.Asset = strings.TrimSpace(dependency.Asset)
	dependency.Version = strings.TrimSpace(strings.TrimPrefix(dependency.Version, "v"))

	if dependency.TagPrefix == "" {
		dependency.TagPrefix = "v"
	}
	if dependency.Asset == "" && repositoryPattern.MatchString(dependency.Repository) {
		dependency.Asset = path.Base(dependency.Repository)
	}

	switch {
	case !pluginIDPattern.MatchString(dependency.ID):
		return Dependency{}, fmt.Errorf("invalid plugin ID %q", dependency.ID)
	case !validRepository(dependency.Repository):
		return Dependency{}, fmt.Errorf("repository must be a GitHub owner/repository value")
	case !tagPrefixPattern.MatchString(dependency.TagPrefix) || strings.Contains(dependency.TagPrefix, "..") || strings.Contains(dependency.TagPrefix, "//"):
		return Dependency{}, fmt.Errorf("invalid tag_prefix %q", dependency.TagPrefix)
	case !assetPattern.MatchString(dependency.Asset):
		return Dependency{}, fmt.Errorf("invalid asset %q", dependency.Asset)
	case !versionPattern.MatchString(dependency.Version):
		return Dependency{}, fmt.Errorf("invalid plugin version %q", dependency.Version)
	}

	return dependency, nil
}

func validRepository(repository string) bool {
	if !repositoryPattern.MatchString(repository) {
		return false
	}
	owner, name, _ := strings.Cut(repository, "/")
	return owner != "." && owner != ".." && name != "." && name != ".."
}

// Add appends one dependency to the project file.
func Add(filename string, dependency Dependency) error {
	file, err := Load(filename)
	if err != nil {
		return err
	}
	dependency, err = NormalizeDependency(dependency)
	if err != nil {
		return err
	}
	for _, current := range file.Plugins {
		if current.ID == dependency.ID {
			return fmt.Errorf("plugin %s is already declared", dependency.ID)
		}
	}
	file.Plugins = append(file.Plugins, dependency)
	return Save(filename, file)
}

// Remove deletes one dependency from the project file.
func Remove(filename, id string) error {
	file, err := Load(filename)
	if err != nil {
		return err
	}
	id = strings.TrimSpace(id)
	index := slices.IndexFunc(file.Plugins, func(dependency Dependency) bool { return dependency.ID == id })
	if index < 0 {
		return fmt.Errorf("plugin %s is not declared", id)
	}
	file.Plugins = slices.Delete(file.Plugins, index, index+1)
	return Save(filename, file)
}
