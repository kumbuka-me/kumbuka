// Package pluginupdate discovers and downloads first-party Kumbuka plugin updates.
package pluginupdate

import (
	"bytes"
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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/sdk"
	"github.com/kumbuka-me/sdk/pluginpackage"
)

const (
	// DefaultCatalogURL is the canonical first-party plugin update catalog.
	DefaultCatalogURL = "https://kumbuka.me/plugins/catalog.json"

	catalogSchemaVersion = 1
	maxCatalogBytes      = 2 << 20
	defaultHTTPTimeout   = 15 * time.Second
)

// ErrCatalogUnavailable reports that no successful catalog refresh has completed yet.
var ErrCatalogUnavailable = errors.New("plugin update catalog is unavailable")

// release is one catalog-internal downloadable plugin release.
type release struct {
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
	Versions []release `json:"versions"`
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

// Client fetches, caches, and downloads first-party plugin update metadata and packages.
type Client struct {
	// catalogURL is the canonical catalog endpoint queried by this client.
	catalogURL string
	// httpClient performs bounded catalog and package HTTP requests.
	httpClient *http.Client
	// mu protects the cached catalog and ready state.
	mu sync.RWMutex
	// cached contains the last successfully decoded catalog.
	cached catalog
	// ready reports whether at least one successful refresh completed.
	ready bool
}

// New creates a first-party plugin update client for catalogURL using standard proxy environment settings.
func New(catalogURL string) *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyFromEnvironment

	return &Client{
		catalogURL: strings.TrimSpace(catalogURL),
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   defaultHTTPTimeout,
		},
	}
}

// Refresh fetches the catalog immediately and atomically replaces the cached successful copy.
func (c *Client) Refresh(ctx context.Context) error {
	if c == nil || c.catalogURL == "" {
		return ErrCatalogUnavailable
	}

	loaded, err := c.fetchCatalog(ctx)
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.cached = loaded
	c.ready = true
	c.mu.Unlock()

	return nil
}

// Updates returns the newest compatible release newer than each installed plugin version from the cached catalog.
func (c *Client) Updates(installed map[string]string) (map[string]domain.PluginRelease, error) {
	available, err := c.cachedCatalog()
	if err != nil {
		return nil, err
	}

	updates := make(map[string]domain.PluginRelease)
	for _, item := range available.Plugins {
		currentText, installed := installed[item.ID]
		if !installed {
			continue
		}
		selected, found := newestCompatibleRelease(item.Versions, currentText)
		if found {
			updates[item.ID] = domain.PluginRelease{Version: selected.Version, ReleasedAt: selected.ReleasedAt}
		}
	}
	return updates, nil
}

// cachedCatalog returns the last successfully refreshed catalog.
func (c *Client) cachedCatalog() (catalog, error) {
	if c == nil {
		return catalog{}, ErrCatalogUnavailable
	}
	c.mu.RLock()
	available := c.cached
	ready := c.ready
	c.mu.RUnlock()
	if !ready {
		return catalog{}, ErrCatalogUnavailable
	}
	return available, nil
}

// newestCompatibleRelease selects the newest valid release newer than currentText.
func newestCompatibleRelease(releases []release, currentText string) (release, bool) {
	current, ok := parseVersion(currentText)
	if !ok {
		return release{}, false
	}

	var selected release
	var selectedVersion semanticVersion
	found := false
	for _, candidateRelease := range releases {
		candidate, valid := eligibleUpdateVersion(candidateRelease, current)
		if !valid {
			continue
		}
		if !found || compareVersion(candidate, selectedVersion) > 0 {
			selected = candidateRelease
			selectedVersion = candidate
			found = true
		}
	}
	return selected, found
}

// eligibleUpdateVersion validates compatibility, version ordering, and release metadata.
func eligibleUpdateVersion(candidateRelease release, current semanticVersion) (semanticVersion, bool) {
	if int64(candidateRelease.APIVersion) != int64(sdk.Version) || releaseProblem(candidateRelease) != "" {
		return semanticVersion{}, false
	}
	candidate, valid := parseVersion(candidateRelease.Version)
	return candidate, valid && compareVersion(candidate, current) > 0
}

// Download retrieves one bounded package in memory, verifies it, and returns validated package bytes.
func (c *Client) Download(ctx context.Context, pluginID, version string) ([]byte, error) {
	if c == nil {
		return nil, errors.New("plugin update client is unavailable")
	}
	release, err := c.release(pluginID, version)
	if err != nil {
		return nil, err
	}
	packageURL, err := validatedPackageURL(release.PackageURL)
	if err != nil {
		return nil, err
	}

	response, err := c.downloadPackageResponse(ctx, packageURL)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close() // nolint:errcheck

	data, err := readVerifiedPackage(response, release.SHA256)
	if err != nil {
		return nil, err
	}
	if err := validateDownloadedPackage(data, pluginID, release.Version); err != nil {
		return nil, err
	}
	return bytes.Clone(data), nil
}

// validatedPackageURL parses and enforces the first-party package URL policy.
func validatedPackageURL(value string) (*url.URL, error) {
	packageURL, err := url.Parse(value)
	if err != nil || !allowedPackageURL(packageURL) {
		return nil, errors.New("plugin package URL is not an allowed first-party release URL")
	}
	return packageURL, nil
}

// downloadPackageResponse performs the package request and validates response metadata.
func (c *Client) downloadPackageResponse(ctx context.Context, packageURL *url.URL) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, packageURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create plugin package request: %w", err)
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("download plugin package: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close() // nolint:errcheck
		return nil, fmt.Errorf("download plugin package: unexpected HTTP status %d", response.StatusCode)
	}
	if response.ContentLength > pluginpackage.MaxArchiveBytes {
		response.Body.Close() // nolint:errcheck
		return nil, errors.New("downloaded plugin package exceeds the 16 MiB limit")
	}
	return response, nil
}

// readVerifiedPackage reads the bounded response and verifies its catalog checksum.
func readVerifiedPackage(response *http.Response, expectedHash string) ([]byte, error) {
	var archive bytes.Buffer
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(&archive, hash), io.LimitReader(response.Body, pluginpackage.MaxArchiveBytes+1))
	if err != nil {
		return nil, fmt.Errorf("download plugin package: %w", err)
	}
	if written > pluginpackage.MaxArchiveBytes {
		return nil, errors.New("downloaded plugin package exceeds the 16 MiB limit")
	}
	if !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), expectedHash) {
		return nil, errors.New("downloaded plugin package checksum does not match the catalog")
	}
	return archive.Bytes(), nil
}

// validateDownloadedPackage verifies the package manifest identity and version.
func validateDownloadedPackage(data []byte, pluginID, version string) error {
	pkg, err := pluginpackage.Read(data)
	if err != nil {
		return fmt.Errorf("validate downloaded plugin package: %w", err)
	}
	if pkg.Manifest().ID != pluginID {
		return errors.New("downloaded plugin package ID does not match the requested plugin")
	}
	if pkg.Manifest().Version != version {
		return errors.New("downloaded plugin package version does not match the catalog")
	}
	return nil
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
	defer response.Body.Close() // nolint:errcheck
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

// release returns one validated cached release selected by plugin ID and exact version.
func (c *Client) release(pluginID, version string) (release, error) {
	c.mu.RLock()
	available := c.cached
	ready := c.ready
	c.mu.RUnlock()
	if !ready {
		return release{}, ErrCatalogUnavailable
	}

	for _, item := range available.Plugins {
		if item.ID != pluginID {
			continue
		}
		for _, candidate := range item.Versions {
			if candidate.Version != version {
				continue
			}
			if int64(candidate.APIVersion) != int64(sdk.Version) {
				return release{}, fmt.Errorf("plugin release API version %d is incompatible", candidate.APIVersion)
			}
			if problem := releaseProblem(candidate); problem != "" {
				return release{}, errors.New(problem)
			}
			return candidate, nil
		}
	}

	return release{}, errors.New("plugin release is not available in the cached catalog")
}

// releaseProblem returns a validation problem for update metadata that must not be used.
func releaseProblem(release release) string {
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
	if !validUpdateURLAuthority(value) {
		return false
	}
	if value.Scheme == "https" {
		return true
	}
	return value.Scheme == "http" && loopbackHost(value.Hostname())
}

// allowedPackageURL reports whether a package URL points at the first-party GitHub releases or loopback tests.
func allowedPackageURL(value *url.URL) bool {
	if !validUpdateURLAuthority(value) {
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

// validUpdateURLAuthority reports whether an update URL has a host and contains no user credentials.
func validUpdateURLAuthority(value *url.URL) bool {
	return value != nil && value.Host != "" && value.User == nil
}

// validSemanticVersionPart reports whether a numeric version component is non-empty and has no leading zeroes.
func validSemanticVersionPart(part string) bool {
	return part != "" && (len(part) == 1 || part[0] != '0')
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
		if !validSemanticVersionPart(part) {
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
