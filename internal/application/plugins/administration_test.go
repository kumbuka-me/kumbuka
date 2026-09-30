package plugins

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/sdk/pluginpackage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdminStatusWithoutCatalog(t *testing.T) {
	t.Parallel()

	assert.Equal(t, PluginUpdateStatus{}, (&Admin{}).Status())
}

// administrationLifecycleStub provides deterministic lifecycle behavior without constructing a WASM runtime.
type administrationLifecycleStub struct {
	mu       sync.Mutex
	plugins  []plugin.LoadedPlugin
	installs []string
	upgrades []string
	failures map[string]error
}

func (s *administrationLifecycleStub) Plugins() []plugin.LoadedPlugin {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]plugin.LoadedPlugin(nil), s.plugins...)
}
func (*administrationLifecycleStub) IsRequired(string) bool { return false }
func (s *administrationLifecycleStub) Install(_ context.Context, archive []byte) (plugin.LoadedPlugin, error) {
	pkg, err := pluginpackage.Read(archive)
	if err != nil {
		return plugin.LoadedPlugin{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item := plugin.LoadedPlugin{Manifest: pkg.Manifest(), Enabled: true}
	s.plugins = append(s.plugins, item)
	s.installs = append(s.installs, item.Manifest.ID)
	return item, nil
}
func (s *administrationLifecycleStub) Upgrade(_ context.Context, id string, archive []byte) (plugin.LoadedPlugin, error) {
	pkg, err := pluginpackage.Read(archive)
	if err != nil {
		return plugin.LoadedPlugin{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.failures[id]; err != nil {
		return plugin.LoadedPlugin{}, err
	}
	for index := range s.plugins {
		if s.plugins[index].Manifest.ID != id {
			continue
		}
		s.plugins[index].Manifest = pkg.Manifest()
		s.upgrades = append(s.upgrades, id)
		return s.plugins[index], nil
	}
	return plugin.LoadedPlugin{}, errors.New("plugin is not installed")
}
func (*administrationLifecycleStub) Enable(context.Context, string) error    { return nil }
func (*administrationLifecycleStub) Disable(context.Context, string) error   { return nil }
func (*administrationLifecycleStub) Uninstall(context.Context, string) error { return nil }

// administrationCatalogStub provides release bytes and independent download failures.
type administrationCatalogStub struct {
	mu        sync.Mutex
	updates   map[string]domain.PluginRelease
	archives  map[string][]byte
	failures  map[string]error
	downloads []string
	download  func(context.Context, string, string) ([]byte, error)
}

func (*administrationCatalogStub) Refresh(context.Context) error { return nil }
func (s *administrationCatalogStub) Available() (map[string]domain.PluginRelease, error) {
	return s.updates, nil
}
func (s *administrationCatalogStub) Download(ctx context.Context, id, version string) ([]byte, error) {
	s.mu.Lock()
	s.downloads = append(s.downloads, id+"@"+version)
	download := s.download
	failure := s.failures[id]
	archive := s.archives[id]
	s.mu.Unlock()
	if download != nil {
		return download(ctx, id, version)
	}
	if failure != nil {
		return nil, failure
	}
	return archive, nil
}
func (*administrationCatalogStub) Status() PluginUpdateStatus { return PluginUpdateStatus{} }

func TestInstallAlwaysRequiresExactPermissionReview(t *testing.T) {
	t.Parallel()

	const id = "io.example.install-review"
	archive := administrationPluginArchive(t, id, "1.0.0", []string{"pages:read", "pages:write"})
	manager := &administrationLifecycleStub{}
	admin := NewAdmin(manager, nil)

	_, err := admin.Install(context.Background(), archive)
	var approval *PermissionApprovalRequiredError
	require.ErrorAs(t, err, &approval)
	assert.Equal(t, "install", approval.Operation)
	assert.Equal(t, id, approval.PluginID)
	assert.Equal(t, "1.0.0", approval.Version)
	assert.Equal(t, []string{"pages:read", "pages:write"}, approval.Permissions)
	assert.Equal(t, approval.Permissions, approval.AddedPermissions)
	assert.Empty(t, approval.RemovedPermissions)
	assert.Empty(t, manager.installs)

	_, err = admin.Install(context.Background(), archive, PermissionApproval{
		Operation:   "install",
		PluginID:    id,
		Version:     "1.0.0",
		Digest:      approval.Digest,
		Permissions: []string{"pages:write"},
	})
	require.ErrorAs(t, err, &approval, "partial permission approval must not install the package")
	assert.Empty(t, manager.installs)

	item, err := admin.Install(context.Background(), archive, PermissionApproval{
		Operation:   "install",
		PluginID:    id,
		Version:     "1.0.0",
		Digest:      approval.Digest,
		Permissions: []string{"pages:write", "pages:read"},
	})
	require.NoError(t, err)
	assert.Equal(t, id, item.Manifest.ID)
	assert.Equal(t, []string{id}, manager.installs)
}

func TestInstallWithoutPermissionsStillRequiresConfirmation(t *testing.T) {
	t.Parallel()

	const id = "io.example.install-no-permissions"
	archive := administrationPluginArchive(t, id, "1.0.0", nil)
	manager := &administrationLifecycleStub{}
	admin := NewAdmin(manager, nil)

	_, err := admin.Install(context.Background(), archive)
	var approval *PermissionApprovalRequiredError
	require.ErrorAs(t, err, &approval)
	assert.Empty(t, approval.Permissions)
	assert.Empty(t, manager.installs)

	_, err = admin.Install(context.Background(), archive, PermissionApproval{
		Operation: "upgrade",
		PluginID:  id,
		Version:   "1.0.0",
		Digest:    approval.Digest,
	})
	require.ErrorAs(t, err, &approval, "approval for a different lifecycle operation must not install the package")
	assert.Empty(t, manager.installs)

	_, err = admin.Install(context.Background(), archive, PermissionApproval{
		Operation: "install",
		PluginID:  id,
		Version:   "1.0.0",
		Digest:    approval.Digest,
	})
	require.NoError(t, err)
	assert.Equal(t, []string{id}, manager.installs)
}

func TestCatalogUpdateRequiresApprovalForAnyPermissionChange(t *testing.T) {
	t.Parallel()

	const id = "io.example.permissions"
	manager := &administrationLifecycleStub{plugins: []plugin.LoadedPlugin{{
		Manifest: pluginpackage.Manifest{ID: id, Version: "1.0.0", Permissions: []string{"pages:read", "storage:read"}},
		Enabled:  true,
	}}}
	archive := administrationPluginArchive(t, id, "1.1.0", []string{"pages:read", "pages:write"})
	catalog := &administrationCatalogStub{
		updates:  map[string]domain.PluginRelease{id: {Version: "1.1.0"}},
		archives: map[string][]byte{id: archive},
	}
	admin := NewAdmin(manager, catalog)

	err := admin.Update(context.Background(), id)

	var approval *PermissionApprovalRequiredError
	require.ErrorAs(t, err, &approval)
	assert.Equal(t, "update", approval.Operation)
	assert.Equal(t, id, approval.PluginID)
	assert.Equal(t, "1.1.0", approval.Version)
	assert.Equal(t, []string{"pages:read", "pages:write"}, approval.Permissions)
	assert.Equal(t, []string{"pages:write"}, approval.AddedPermissions)
	assert.Equal(t, []string{"storage:read"}, approval.RemovedPermissions)
	assert.Empty(t, manager.upgrades)

	err = admin.Update(context.Background(), id, UpdateApproval{
		Operation:   "update",
		PluginID:    id,
		Version:     "1.1.0",
		Digest:      "wrong-digest",
		Permissions: approval.Permissions,
	})
	require.ErrorAs(t, err, &approval)
	assert.Empty(t, manager.upgrades, "approval for different package bytes must not activate the release")

	require.NoError(t, admin.Update(context.Background(), id, UpdateApproval{
		Operation:   "update",
		PluginID:    id,
		Version:     "1.1.0",
		Digest:      pluginArchiveDigest(archive),
		Permissions: []string{"pages:write", "pages:read"},
	}))
	assert.Equal(t, []string{id}, manager.upgrades)
	assert.Equal(t, "1.1.0", manager.plugins[0].Manifest.Version)
}

func TestCatalogUpdateWithExistingPermissionsNeedsNoExtraApproval(t *testing.T) {
	t.Parallel()

	const id = "io.example.same-permissions"
	manager := &administrationLifecycleStub{plugins: []plugin.LoadedPlugin{{
		Manifest: pluginpackage.Manifest{ID: id, Version: "1.0.0", Permissions: []string{"pages:read", "pages:write"}},
	}}}
	catalog := &administrationCatalogStub{
		updates: map[string]domain.PluginRelease{id: {Version: "1.1.0"}},
		archives: map[string][]byte{id: administrationPluginArchive(
			t,
			id,
			"1.1.0",
			[]string{"pages:write", "pages:read"},
		)},
	}

	require.NoError(t, NewAdmin(manager, catalog).Update(context.Background(), id))
	assert.Equal(t, []string{id}, manager.upgrades)
}

func TestManualUpgradeRequiresApprovalWhenPermissionsChange(t *testing.T) {
	t.Parallel()

	const id = "io.example.manual-upgrade"
	manager := &administrationLifecycleStub{plugins: []plugin.LoadedPlugin{{
		Manifest: pluginpackage.Manifest{ID: id, Version: "1.0.0", Permissions: []string{"pages:read"}},
	}}}
	archive := administrationPluginArchive(t, id, "1.1.0", nil)
	admin := NewAdmin(manager, nil)

	err := admin.Upgrade(context.Background(), id, archive)
	var approval *PermissionApprovalRequiredError
	require.ErrorAs(t, err, &approval)
	assert.Equal(t, "upgrade", approval.Operation)
	assert.Empty(t, approval.Permissions)
	assert.Equal(t, []string{"pages:read"}, approval.RemovedPermissions)
	assert.Empty(t, manager.upgrades)

	require.NoError(t, admin.Upgrade(context.Background(), id, archive, PermissionApproval{
		Operation: "upgrade",
		PluginID:  id,
		Version:   "1.1.0",
		Digest:    approval.Digest,
	}))
	assert.Equal(t, []string{id}, manager.upgrades)
}

func TestUpdateAllContinuesPastApprovalAndDownloadFailures(t *testing.T) {
	t.Parallel()

	const (
		approvalID = "io.example.a-approval"
		failureID  = "io.example.b-failure"
		successID  = "io.example.c-success"
	)
	manager := &administrationLifecycleStub{plugins: []plugin.LoadedPlugin{
		{Manifest: pluginpackage.Manifest{ID: approvalID, Version: "1.0.0"}},
		{Manifest: pluginpackage.Manifest{ID: failureID, Version: "1.0.0"}},
		{Manifest: pluginpackage.Manifest{ID: successID, Version: "1.0.0"}},
	}}
	catalog := &administrationCatalogStub{
		updates: map[string]domain.PluginRelease{
			approvalID: {Version: "1.1.0"},
			failureID:  {Version: "1.1.0"},
			successID:  {Version: "1.1.0"},
		},
		archives: map[string][]byte{
			approvalID: administrationPluginArchive(t, approvalID, "1.1.0", []string{"pages:write"}),
			successID:  administrationPluginArchive(t, successID, "1.1.0", nil),
		},
		failures: map[string]error{failureID: errors.New("catalog download failed")},
	}

	result, err := NewAdmin(manager, catalog).UpdateAll(context.Background())

	require.NoError(t, err)
	assert.Equal(t, []string{successID}, result.Updated)
	require.Len(t, result.Failed, 2)
	assert.Equal(t, approvalID, result.Failed[0].PluginID)
	var approval *PermissionApprovalRequiredError
	require.ErrorAs(t, result.Failed[0].Err, &approval)
	assert.Equal(t, failureID, result.Failed[1].PluginID)
	assert.ErrorContains(t, result.Failed[1].Err, "catalog download failed")
	assert.ElementsMatch(t, []string{approvalID + "@1.1.0", failureID + "@1.1.0", successID + "@1.1.0"}, catalog.downloads)
	assert.ElementsMatch(t, []string{successID}, manager.upgrades)
}

func TestUpdateAllUsesFourConcurrentWorkers(t *testing.T) {
	t.Parallel()

	const pluginCount = 7
	manager := &administrationLifecycleStub{}
	catalog := &administrationCatalogStub{
		updates:  make(map[string]domain.PluginRelease, pluginCount),
		archives: make(map[string][]byte, pluginCount),
	}
	started := make(chan string, pluginCount)
	release := make(chan struct{})
	catalog.download = func(ctx context.Context, id, _ string) ([]byte, error) {
		started <- id
		select {
		case <-release:
			return catalog.archives[id], nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	for index := range pluginCount {
		id := fmt.Sprintf("io.example.parallel-%02d", index)
		manager.plugins = append(manager.plugins, plugin.LoadedPlugin{Manifest: pluginpackage.Manifest{ID: id, Version: "1.0.0"}})
		catalog.updates[id] = domain.PluginRelease{Version: "1.1.0"}
		catalog.archives[id] = administrationPluginArchive(t, id, "1.1.0", nil)
	}

	type updateResult struct {
		result UpdateAllResult
		err    error
	}
	done := make(chan updateResult, 1)
	go func() {
		result, err := NewAdmin(manager, catalog).UpdateAll(context.Background())
		done <- updateResult{result: result, err: err}
	}()

	for range pluginUpdateConcurrency {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("bulk update did not start four downloads concurrently")
		}
	}
	select {
	case id := <-started:
		t.Fatalf("bulk update exceeded the four-worker limit with %s", id)
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	select {
	case got := <-done:
		require.NoError(t, got.err)
		require.Len(t, got.result.Updated, pluginCount)
		assert.Empty(t, got.result.Failed)
	case <-time.After(3 * time.Second):
		t.Fatal("bulk update did not finish after releasing downloads")
	}
}

// administrationPluginArchive creates a minimal valid declarative plugin package for update-policy tests.
func administrationPluginArchive(t *testing.T, id, version string, permissions []string) []byte {
	t.Helper()

	var permissionYAML string
	if len(permissions) == 0 {
		permissionYAML = "permissions: []\n"
	} else {
		permissionYAML = "permissions:\n"
		for _, permission := range permissions {
			permissionYAML += "  - " + permission + "\n"
		}
	}

	manifest := fmt.Sprintf(
		"api_version: 1\nid: %s\nname: Administration fixture\nversion: %s\nmodules:\n  - type: markdown-syntax\n    id: syntax\n    syntax: strikethrough\n%s",
		id,
		version,
		permissionYAML,
	)

	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, content := range map[string]string{
		"plugin.yaml": manifest,
		"README.md":   "# Administration fixture\n",
	} {
		entry, err := writer.Create(name)
		require.NoError(t, err)
		_, err = entry.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	return buffer.Bytes()
}

// offlineLifecycle exposes embedded package reads independently of remote transport.
type offlineLifecycle struct {
	*administrationLifecycleStub
	archives map[string][]byte
}

func (s *offlineLifecycle) BuiltinArchive(_ context.Context, id, version string) ([]byte, error) {
	data, ok := s.archives[id]
	if !ok {
		return nil, errors.New("missing builtin")
	}
	return data, nil
}
func TestUpdateResolutionAcrossSources(t *testing.T) {
	for _, tc := range []struct {
		name, builtin, remote string
		want                  string
		offline               bool
	}{
		{"builtin only", "1.10.0", "", "1.10.0", true},
		{"remote only", "", "1.11.0", "1.11.0", false},
		{"remote newer", "1.10.0", "1.11.0", "1.11.0", false},
		{"builtin newer", "1.12.0", "1.11.0", "1.12.0", true},
		{"tie prefers offline", "1.11.0", "1.11.0", "1.11.0", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			item := plugin.LoadedPlugin{Manifest: pluginpackage.Manifest{ID: "io.fixture", Version: "1.9.0"}}
			if tc.builtin != "" {
				item.Builtin = &plugin.BuiltinPackage{ID: "io.fixture", Version: tc.builtin}
				item.BuiltinUpdateAvailable = true
			}
			manager := &administrationLifecycleStub{plugins: []plugin.LoadedPlugin{item}}
			var catalog Catalog
			if tc.remote != "" {
				catalog = &administrationCatalogStub{updates: map[string]domain.PluginRelease{"io.fixture": {Version: tc.remote}}}
			}
			releases, err := NewAdmin(manager, catalog).Available()
			require.NoError(t, err)
			assert.Equal(t, tc.want, releases["io.fixture"].Version)
			assert.Equal(t, tc.offline, releases["io.fixture"].Builtin)
		})
	}
}
func TestOfflineUpdateAndUpdateAllContinueAfterFailure(t *testing.T) {
	ctx := context.Background()
	manager := &offlineLifecycle{administrationLifecycleStub: &administrationLifecycleStub{failures: map[string]error{"io.bad": errors.New("initialization failed")}}, archives: make(map[string][]byte)}
	for _, id := range []string{"io.bad", "io.good"} {
		manager.plugins = append(manager.plugins, plugin.LoadedPlugin{Manifest: pluginpackage.Manifest{ID: id, Version: "1.0.0"}, Builtin: &plugin.BuiltinPackage{ID: id, Version: "1.1.0"}, BuiltinUpdateAvailable: true})
		manager.archives[id] = administrationPluginArchive(t, id, "1.1.0", nil)
	}
	admin := NewAdmin(manager, nil)
	result, err := admin.UpdateAll(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"io.good"}, result.Updated)
	require.Len(t, result.Failed, 1)
	assert.Equal(t, "io.bad", result.Failed[0].PluginID)
	// Explicit offline selection ignores a newer remote release entirely.
	remote := &administrationCatalogStub{updates: map[string]domain.PluginRelease{"io.good": {Version: "9.0.0"}}}
	require.NoError(t, NewAdmin(manager, remote).UpdateBuiltin(ctx, "io.good"))
	assert.Empty(t, remote.downloads)
}
