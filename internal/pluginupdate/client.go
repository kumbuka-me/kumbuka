// Package pluginupdate discovers and downloads first-party Kumbuka plugin updates.
package pluginupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kumbuka-me/sdk"
	"github.com/kumbuka-me/sdk/pluginpackage"
)

const (
	// DefaultCatalogURL is the canonical first-party plugin update catalog.
	DefaultCatalogURL = "https://kumbuka.me/plugins/catalog.json"

	catalogSchemaVersion = 1
	maxCatalogBytes      = 2 << 20
	defaultFailureTTL    = time.Minute
	defaultHTTPTimeout   = 15 * time.Second
)

// Release describes one downloadable plugin release from the update catalog.
type Release struct {
	// Version is the strict MAJOR.MINOR.PATCH plugin version.
	Version string `json:"version"`
	// APIVersion is the Kumbuka plugin API version required by the package.
	APIVersion int `json:"api_version"`
	// ReleasedAt records when GitHub published the release.
	ReleasedAt time.Time `json:"released_at"`
	// PackageURL is the HTTPS URL of the versioned .kumbukaplugin asset.
	PackageURL string `json:"package_url"`
	// SHA256 is the lowercase hexadecimal SHA-256 digest of the package asset.
	SHA256 string `json:"sha256"`
}

// catalogPlugin groups all published versions for one stable plugin ID.
type catalogPlugin struct {
	// ID is the stable plugin identifier.
	ID string `json:"id"`
	// Name is the human-readable plugin name.
	Name string `json:"name"`
	// Provider is the package provider supplied by the plugin manifest.
	Provider string `json:"provider"`
	// Versions contains published releases, normally newest first.
	Versions []Release `json:"versions"`
}

// catalog is the versioned wire representation served by kumbuka.me.
type catalog struct {
	// SchemaVersion identifies the catalog wire format.
	SchemaVersion int `json:"schema_version"`
	// Plugins contains first-party plugin release histories.
	Plugins []catalogPlugin `json:"plugins"`
}

// semanticVersion is a strict three-component plugin version.
type semanticVersion struct {
	// major is the compatibility-breaking version component.
	major uint64
	// minor is the feature version component.
	minor uint64
	// patch is the patch version component.
	patch uint64
}

// Client caches the public catalog and downloads verified plugin packages through temporary storage.
type Client struct {
	// catalogURL is the canonical catalog endpoint queried by this client.
	catalogURL string
	// httpClient performs bounded catalog and package HTTP requests.
	httpClient *http.Client
	// tempDir receives transient plugin downloads before verification.
	tempDir string
	// cacheTTL controls how long one successful catalog response can be reused.
	cacheTTL time.Duration
	// now supplies wall-clock time for cache decisions.
	now func() time.Time

	// mu protects the cached catalog and its timestamp.
	mu sync.Mutex
	// cached contains the last successfully decoded catalog.
	cached catalog
	// cachedAt records when cached was fetched.
	cachedAt time.Time
	// lastFailure is the most recent catalog refresh error, cached briefly to avoid repeated slow failures.
	lastFailure error
	// failedAt records when lastFailure occurred.
	failedAt time.Time
}

// New creates a first-party plugin update client for catalogURL and the configured successful refresh interval.
func New(catalogURL string, cacheTTL time.Duration) *Client {
	return &Client{
		catalogURL: strings.TrimSpace(catalogURL),
		httpClient: &http.Client{Timeout: defaultHTTPTimeout},
		tempDir:    os.TempDir(),
		cacheTTL:   cacheTTL,
		now:        time.Now,
	}
}

// Updates returns the newest compatible release newer than each installed plugin version.
func (c *Client) Updates(ctx context.Context, installed map[string]string) (map[string]Release, error) {
	updates := make(map[string]Release)
	if c == nil || c.catalogURL == "" || c.cacheTTL <= 0 || len(installed) == 0 {
		return updates, nil
	}

	available, err := c.loadCatalog(ctx)
	if err != nil {
		return nil, err
	}

	for _, item := range available.Plugins {
		currentText, ok := installed[item.ID]
		if !ok {
			continue
		}
		current, ok := parseVersion(currentText)
		if !ok {
			continue
		}

		var selected Release
		var selectedVersion semanticVersion
		found := false
		for _, release := range item.Versions {
			if int64(release.APIVersion) != int64(sdk.Version) {
				continue
			}
			candidate, valid := parseVersion(release.Version)
			if !valid || compareVersion(candidate, current) <= 0 {
				continue
			}
			if problem := releaseProblem(release); problem != "" {
				continue
			}
			if !found || compareVersion(candidate, selectedVersion) > 0 {
				selected = release
				selectedVersion = candidate
				found = true
			}
		}
		if found {
			updates[item.ID] = selected
		}
	}

	return updates, nil
}

// Download retrieves release into temporary storage, verifies it, and returns validated package bytes.
func (c *Client) Download(ctx context.Context, pluginID string, release Release) ([]byte, error) {
	if c == nil {
		return nil, errors.New("plugin update client is unavailable")
	}
	if int64(release.APIVersion) != int64(sdk.Version) {
		return nil, fmt.Errorf("plugin release API version %d is incompatible", release.APIVersion)
	}
	if problem := releaseProblem(release); problem != "" {
		return nil, errors.New(problem)
	}

	packageURL, err := url.Parse(release.PackageURL)
	if err != nil || !allowedPackageURL(packageURL) {
		return nil, errors.New("plugin package URL is not an allowed first-party release URL")
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, packageURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create plugin package request: %w", err)
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("download plugin package: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download plugin package: unexpected HTTP status %d", response.StatusCode)
	}
	if response.ContentLength > pluginpackage.MaxArchiveBytes {
		return nil, errors.New("downloaded plugin package exceeds the 16 MiB limit")
	}

	file, err := os.CreateTemp(c.tempDir, "kumbuka-plugin-*.kumbukaplugin")
	if err != nil {
		return nil, fmt.Errorf("create temporary plugin package: %w", err)
	}
	path := file.Name()
	defer os.Remove(path)

	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(response.Body, pluginpackage.MaxArchiveBytes+1))
	closeErr := file.Close()
	if copyErr != nil {
		return nil, fmt.Errorf("download plugin package: %w", copyErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close temporary plugin package: %w", closeErr)
	}
	if written > pluginpackage.MaxArchiveBytes {
		return nil, errors.New("downloaded plugin package exceeds the 16 MiB limit")
	}

	actualHash := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(actualHash, release.SHA256) {
		return nil, errors.New("downloaded plugin package checksum does not match the catalog")
	}

	archive, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read temporary plugin package: %w", err)
	}
	pkg, err := pluginpackage.Read(archive)
	if err != nil {
		return nil, fmt.Errorf("validate downloaded plugin package: %w", err)
	}
	if pkg.Manifest().ID != pluginID {
		return nil, errors.New("downloaded plugin package ID does not match the requested plugin")
	}
	if pkg.Manifest().Version != release.Version {
		return nil, errors.New("downloaded plugin package version does not match the catalog")
	}

	return archive, nil
}

// loadCatalog returns a cached catalog or refreshes it from the configured endpoint.
func (c *Client) loadCatalog(ctx context.Context) (catalog, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.now()
	if !c.cachedAt.IsZero() && now.Sub(c.cachedAt) < c.cacheTTL {
		return c.cached, nil
	}
	if c.lastFailure != nil && !c.failedAt.IsZero() && now.Sub(c.failedAt) < defaultFailureTTL {
		return catalog{}, c.lastFailure
	}

	loaded, err := c.fetchCatalog(ctx)
	if err != nil {
		c.lastFailure = err
		c.failedAt = now
		return catalog{}, err
	}
	c.cached = loaded
	c.cachedAt = now
	c.lastFailure = nil
	c.failedAt = time.Time{}
	return loaded, nil
}

// fetchCatalog downloads and decodes one bounded catalog response.
func (c *Client) fetchCatalog(ctx context.Context) (catalog, error) {
	catalogURL, err := url.Parse(c.catalogURL)
	if err != nil || !allowedCatalogURL(catalogURL) {
		return catalog{}, errors.New("plugin catalog URL must use HTTPS")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, catalogURL.String(), nil)
	if err != nil {
		return catalog{}, fmt.Errorf("create plugin catalog request: %w", err)
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return catalog{}, fmt.Errorf("download plugin catalog: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return catalog{}, fmt.Errorf("download plugin catalog: unexpected HTTP status %d", response.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, maxCatalogBytes+1))
	if err != nil {
		return catalog{}, fmt.Errorf("read plugin catalog: %w", err)
	}
	if len(body) > maxCatalogBytes {
		return catalog{}, errors.New("plugin catalog exceeds the 2 MiB limit")
	}

	var decoded catalog
	if err := json.Unmarshal(body, &decoded); err != nil {
		return catalog{}, fmt.Errorf("decode plugin catalog: %w", err)
	}
	if decoded.SchemaVersion != catalogSchemaVersion {
		return catalog{}, fmt.Errorf("unsupported plugin catalog schema version %d", decoded.SchemaVersion)
	}

	return decoded, nil
}

// releaseProblem returns a validation problem for update metadata that must not be used.
func releaseProblem(release Release) string {
	if _, ok := parseVersion(release.Version); !ok {
		return "plugin release version is invalid"
	}
	if release.APIVersion <= 0 {
		return "plugin release API version is invalid"
	}
	if release.ReleasedAt.IsZero() {
		return "plugin release publication date is invalid"
	}
	if len(release.SHA256) != sha256.Size*2 {
		return "plugin release checksum is invalid"
	}
	if _, err := hex.DecodeString(release.SHA256); err != nil {
		return "plugin release checksum is invalid"
	}
	packageURL, err := url.Parse(strings.TrimSpace(release.PackageURL))
	if err != nil || !allowedPackageURL(packageURL) {
		return "plugin release package URL is invalid"
	}
	return ""
}

// allowedCatalogURL reports whether a catalog URL is HTTPS or a loopback test endpoint.
func allowedCatalogURL(value *url.URL) bool {
	if value == nil || value.Host == "" || value.User != nil {
		return false
	}
	if value.Scheme == "https" {
		return true
	}
	return value.Scheme == "http" && loopbackHost(value.Hostname())
}

// allowedPackageURL reports whether a package URL points at the first-party GitHub releases or loopback tests.
func allowedPackageURL(value *url.URL) bool {
	if value == nil || value.Host == "" || value.User != nil {
		return false
	}
	if value.Scheme == "http" && loopbackHost(value.Hostname()) {
		return true
	}
	if value.Scheme != "https" || !strings.EqualFold(value.Hostname(), "github.com") {
		return false
	}
	return strings.HasPrefix(value.EscapedPath(), "/kumbuka-me/plugins/releases/download/")
}

// loopbackHost reports whether host resolves syntactically to a loopback-only test target.
func loopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}

// parseVersion parses the strict MAJOR.MINOR.PATCH versions used by first-party plugins.
func parseVersion(value string) (semanticVersion, bool) {
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return semanticVersion{}, false
	}
	values := make([]uint64, 3)
	for index, part := range parts {
		if part == "" || (len(part) > 1 && part[0] == '0') {
			return semanticVersion{}, false
		}
		parsed, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return semanticVersion{}, false
		}
		values[index] = parsed
	}
	return semanticVersion{major: values[0], minor: values[1], patch: values[2]}, true
}

// compareVersion compares left and right and returns -1, 0, or 1.
func compareVersion(left, right semanticVersion) int {
	for _, pair := range [][2]uint64{{left.major, right.major}, {left.minor, right.minor}, {left.patch, right.patch}} {
		if pair[0] < pair[1] {
			return -1
		}
		if pair[0] > pair[1] {
			return 1
		}
	}
	return 0
}
