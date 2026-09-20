package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/pluginusage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A narrow row fake checks the projection order without a running database.
type pageContractRow struct {
	status      string
	pluginUsage json.RawMessage
}

func (r pageContractRow) Scan(destinations ...any) error {
	if len(destinations) != 14 {
		return fmt.Errorf("page projection has %d fields, want 14", len(destinations))
	}
	status, ok := destinations[12].(*string)
	if !ok {
		return fmt.Errorf("page status destination is %T, want *string", destinations[12])
	}
	*status = r.status
	usage, ok := destinations[13].(*json.RawMessage)
	if !ok {
		return fmt.Errorf("page plugin usage destination is %T, want *json.RawMessage", destinations[13])
	}
	*usage = slices.Clone(r.pluginUsage)
	return nil
}

func TestScanPagePreservesLifecycleStatus(t *testing.T) {
	t.Parallel()
	require.Contains(t, pageSelect, ",p.status", "common page SELECT must include status")

	t.Run("draft", func(t *testing.T) {
		t.Parallel()

		page, err := scanPage(pageContractRow{status: "draft"})
		require.NoError(t, err)
		assert.Equal(t, "draft", page.Status)
	})

	t.Run("verified", func(t *testing.T) {
		t.Parallel()

		page, err := scanPage(pageContractRow{status: "verified"})
		require.NoError(t, err)
		assert.Equal(t, "verified", page.Status)
	})

	t.Run("deprecated", func(t *testing.T) {
		t.Parallel()

		page, err := scanPage(pageContractRow{status: "deprecated"})
		require.NoError(t, err)
		assert.Equal(t, "deprecated", page.Status)
	})

	t.Run("archived", func(t *testing.T) {
		t.Parallel()

		page, err := scanPage(pageContractRow{status: "archived"})
		require.NoError(t, err)
		assert.Equal(t, "archived", page.Status)
	})
}

func TestScanPagePreservesPluginUsage(t *testing.T) {
	t.Parallel()
	record := json.RawMessage(`{"version":1,"fingerprint":"abc","modules":[{"plugin_id":"io.example","module_id":"example","values":["value"]}]}`)

	page, err := scanPage(pageContractRow{status: "verified", pluginUsage: record})

	require.NoError(t, err)
	require.NotNil(t, page.PluginUsage)
	assert.Equal(t, "abc", page.PluginUsage.Fingerprint)
	require.Len(t, page.PluginUsage.Modules, 1)
	assert.Equal(t, "io.example", page.PluginUsage.Modules[0].PluginID)
	assert.Equal(t, []string{"value"}, page.PluginUsage.Modules[0].Values)
}

type propertyContractTx struct {
	pgx.Tx
	inserted [][]any
}

func (tx *propertyContractTx) Exec(_ context.Context, query string, args ...any) (pgconn.CommandTag, error) {
	if strings.Contains(query, "INSERT INTO page_properties") {
		tx.inserted = append(tx.inserted, slices.Clone(args))
	}
	return pgconn.CommandTag{}, nil
}

func TestPagePropertyKeyNormalizationPreservesValue(t *testing.T) {
	t.Parallel()
	tx := &propertyContractTx{}
	properties := map[string]string{" Owner ": " Platform ", "environment": " prod ", "blank": " ", " ": "ignored"}

	require.NoError(t, replacePageProperties(context.Background(), tx, 7, properties))
	assert.Equal(t, [][]any{{int64(7), "Owner", "Platform"}, {int64(7), "environment", "prod"}}, tx.inserted)
	assert.Equal(t, " Platform ", properties[" Owner "], "normalization must not mutate the caller's properties")
}

// This test exercises the actual SQL projections in an isolated PostgreSQL schema.
func TestPageLifecycleQueryContracts(t *testing.T) {
	dsn := integrationDatabase(t)
	ctx := context.Background()
	database, err := Open(ctx, dsn, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	defer database.Close()
	actor, err := database.EnsureAdministrator(ctx, "contract-admin", "", "Contract Admin")
	require.NoError(t, err)
	statuses := []string{"draft", "verified", "deprecated", "archived"}
	for _, status := range statuses {
		slug := "contract/" + status
		metadata := domain.PageMetadata{Status: status}
		if status == "verified" {
			metadata.PluginUsage = &pluginusage.Index{
				Version:     pluginusage.Version,
				Fingerprint: "contract",
				SourceHash:  "source",
				Modules:     []pluginusage.Module{{PluginID: "io.example", ModuleID: "example", Values: []string{"value"}}},
			}
		}
		_, err := database.SavePage(ctx, "", slug, "Contract "+status, "", "", "Lifecycle contract", "Created", nil, nil, nil,
			metadata, map[string]string{" Owner ": " Platform "}, domain.PageRender{}, actor)
		require.NoError(t, err)
		require.NoError(t, database.SetFavorite(ctx, slug, actor.ID, true))
		require.NoError(t, database.RecordView(ctx, slug, actor.ID))
	}
	t.Run("list", func(t *testing.T) {
		pages, err := database.ListPages(ctx, 100)
		require.NoError(t, err)
		require.Len(t, pages, len(statuses))
		for _, page := range pages {
			assert.Equal(t, strings.TrimPrefix(page.Slug, "contract/"), page.Status, "page %s", page.Slug)
		}
	})

	t.Run("search", func(t *testing.T) {
		pages, err := database.Search(ctx, "Contract", 100)
		require.NoError(t, err)
		require.Len(t, pages, len(statuses))
		for _, page := range pages {
			assert.Equal(t, strings.TrimPrefix(page.Slug, "contract/"), page.Status, "page %s", page.Slug)
		}
	})

	t.Run("favorites", func(t *testing.T) {
		pages, err := database.Favorites(ctx, actor.ID)
		require.NoError(t, err)
		require.Len(t, pages, len(statuses))
		for _, page := range pages {
			assert.Equal(t, strings.TrimPrefix(page.Slug, "contract/"), page.Status, "page %s", page.Slug)
		}
	})

	t.Run("recently viewed", func(t *testing.T) {
		pages, err := database.RecentViewed(ctx, actor.ID, 100)
		require.NoError(t, err)
		require.Len(t, pages, len(statuses))
		for _, page := range pages {
			assert.Equal(t, strings.TrimPrefix(page.Slug, "contract/"), page.Status, "page %s", page.Slug)
		}
	})

	t.Run("popular", func(t *testing.T) {
		pages, err := database.Popular(ctx, 100)
		require.NoError(t, err)
		require.Len(t, pages, len(statuses))
		for _, page := range pages {
			assert.Equal(t, strings.TrimPrefix(page.Slug, "contract/"), page.Status, "page %s", page.Slug)
		}
	})

	t.Run("page properties", func(t *testing.T) {
		page, err := database.GetPage(ctx, "contract/verified")
		require.NoError(t, err)
		assert.Equal(t, []domain.PageProperty{{Key: "Owner", Value: "Platform"}}, page.Properties)
		require.NotNil(t, page.PluginUsage)
		assert.Equal(t, "contract", page.PluginUsage.Fingerprint)
		assert.Equal(t, []string{"value"}, page.PluginUsage.Modules[0].Values)
	})

	t.Run("render artifact", func(t *testing.T) {
		page, err := database.GetPage(ctx, "contract/verified")
		require.NoError(t, err)
		render := domain.PageRender{
			HTML:        "<h1 id=\"cached\">Cached</h1>",
			Contents:    []domain.PageHeading{{Level: 1, ID: "cached", Title: "Cached"}},
			Fingerprint: "render-v1",
		}
		require.NoError(t, database.SavePageRender(ctx, page.ID, page.UpdatedAt, render))

		page, err = database.GetPage(ctx, page.Slug)
		require.NoError(t, err)
		assert.Equal(t, render.HTML, page.Render.HTML)
		assert.Equal(t, render.Contents, page.Render.Contents)
		assert.Equal(t, render.Fingerprint, page.Render.Fingerprint)
	})
}

func TestScanPageIgnoresInvalidDerivedPluginUsage(t *testing.T) {
	t.Parallel()

	page, err := scanPage(pageContractRow{status: "verified", pluginUsage: json.RawMessage(`{"version":"invalid"}`)})

	require.NoError(t, err)
	assert.Nil(t, page.PluginUsage)
}
