package pluginproject

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/kumbuka-me/sdk/pluginpackage"
)

// Resolved contains validated package bytes for one project dependency.
type Resolved struct {
	Dependency Dependency
	Manifest   pluginpackage.Manifest
	Archive    []byte
}

// Resolver loads project dependencies from embedded packages, a user cache, or GitHub Releases.
type Resolver struct {
	Bundled  fs.FS
	CacheDir string
	Client   *http.Client
	baseURL  string
}

// NewResolver constructs the default project resolver.
func NewResolver(bundled fs.FS) (*Resolver, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("resolve plugin cache: %w", err)
	}
	return &Resolver{
		Bundled:  bundled,
		CacheDir: filepath.Join(cache, "kumbuka", "plugins"),
		Client:   &http.Client{Timeout: 30 * time.Second},
		baseURL:  "https://github.com",
	}, nil
}

// Resolve validates and materializes every declared dependency without loading its WASM runtime.
func (r *Resolver) Resolve(ctx context.Context, dependencies []Dependency) ([]Resolved, error) {
	result := make([]Resolved, 0, len(dependencies))
	for _, dependency := range dependencies {
		var err error
		dependency, err = NormalizeDependency(dependency)
		if err != nil {
			return nil, err
		}
		if item, found, err := resolveBundled(r.Bundled, dependency); err != nil {
			return nil, err
		} else if found {
			result = append(result, item)
			continue
		}

		item, err := r.resolveRemote(ctx, dependency)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}

	slices.SortFunc(result, func(left, right Resolved) int {
		return strings.Compare(left.Manifest.ID, right.Manifest.ID)
	})
	return result, nil
}

func resolveBundled(bundled fs.FS, dependency Dependency) (Resolved, bool, error) {
	if bundled == nil {
		return Resolved{}, false, nil
	}
	archive, err := fs.ReadFile(bundled, dependency.Asset+".kumbukaplugin")
	if errors.Is(err, fs.ErrNotExist) {
		return Resolved{}, false, nil
	}
	if err != nil {
		return Resolved{}, false, err
	}
	item, err := resolvedPackage(dependency, archive)
	if err != nil {
		// An embedded package with the requested asset name but a different
		// identity/version is not this dependency; resolve the pinned release.
		return Resolved{}, false, nil
	}
	return item, true, nil
}

func (r *Resolver) resolveRemote(ctx context.Context, dependency Dependency) (Resolved, error) {
	cacheKey := sha256.Sum256([]byte(dependency.Repository + "\n" + dependency.TagPrefix + "\n" + dependency.Asset))
	cacheFile := filepath.Join(r.CacheDir, hex.EncodeToString(cacheKey[:8]), dependency.ID, dependency.Version, "plugin.kumbukaplugin")
	if archive, err := os.ReadFile(cacheFile); err == nil {
		if item, err := resolvedPackage(dependency, archive); err == nil {
			return item, nil
		}
		_ = os.Remove(cacheFile)
	} else if !errors.Is(err, os.ErrNotExist) {
		return Resolved{}, err
	}

	archive, err := r.download(ctx, dependency)
	if err != nil {
		return Resolved{}, err
	}
	item, err := resolvedPackage(dependency, archive)
	if err != nil {
		return Resolved{}, err
	}
	if err := writeCacheFile(cacheFile, archive); err != nil {
		return Resolved{}, err
	}
	return item, nil
}

func resolvedPackage(dependency Dependency, archive []byte) (Resolved, error) {
	pkg, err := pluginpackage.Read(archive)
	if err != nil {
		return Resolved{}, err
	}
	manifest := pkg.Manifest()
	if manifest.ID != dependency.ID {
		return Resolved{}, fmt.Errorf("plugin package ID %s does not match declared ID %s", manifest.ID, dependency.ID)
	}
	if manifest.Version != dependency.Version {
		return Resolved{}, fmt.Errorf("plugin %s package version %s does not match declared version %s", dependency.ID, manifest.Version, dependency.Version)
	}
	return Resolved{Dependency: dependency, Manifest: manifest, Archive: slices.Clone(archive)}, nil
}

func (r *Resolver) download(ctx context.Context, dependency Dependency) ([]byte, error) {
	filename := dependency.Asset + "-" + dependency.Version + ".kumbukaplugin"
	tag := dependency.TagPrefix + dependency.Version
	base := strings.TrimRight(r.baseURL, "/") + "/" + dependency.Repository + "/releases/download/" + tag + "/" + filename

	archive, err := r.get(ctx, base, pluginpackage.MaxArchiveBytes)
	if err != nil {
		return nil, fmt.Errorf("download plugin %s: %w", dependency.ID, err)
	}
	checksum, err := r.get(ctx, base+".sha256", 4096)
	if err != nil {
		return nil, fmt.Errorf("download checksum for %s: %w", dependency.ID, err)
	}
	fields := strings.Fields(string(checksum))
	if len(fields) == 0 {
		return nil, fmt.Errorf("empty checksum for plugin %s", dependency.ID)
	}
	expected, err := hex.DecodeString(fields[0])
	if err != nil || len(expected) != sha256.Size {
		return nil, fmt.Errorf("invalid checksum for plugin %s", dependency.ID)
	}
	actual := sha256.Sum256(archive)
	if !slices.Equal(expected, actual[:]) {
		return nil, fmt.Errorf("checksum mismatch for plugin %s", dependency.ID)
	}
	return archive, nil
}

func (r *Resolver) get(ctx context.Context, url string, limit int) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	client := r.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("%s", response.Status)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, int64(limit)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > limit {
		return nil, fmt.Errorf("response exceeds size limit")
	}
	return data, nil
}

func writeCacheFile(filename string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(filename), ".plugin-*")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer func() { _ = os.Remove(name) }()
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
	return os.Rename(name, filename)
}
