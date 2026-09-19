package service

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	md "github.com/kumbuka-me/kumbuka/pkg/markdown"
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/kumbuka/pkg/pluginusage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type pageSaveRepositoryStub struct {
	pageRepository
	slug     string
	metadata domain.PageMetadata
	render   domain.PageRender
}

func (r *pageSaveRepositoryStub) SavePage(
	_ context.Context,
	_, slug, title, _, _, _, _ string,
	_, _ []string,
	_ []int64,
	metadata domain.PageMetadata,
	_ map[string]string,
	render domain.PageRender,
	_ domain.User,
) (domain.Page, error) {
	r.slug = slug
	r.metadata = metadata
	r.render = render

	return domain.Page{Slug: slug, Title: title}, nil
}

func (r *pageSaveRepositoryStub) ApplicationSettings(context.Context) (domain.ApplicationSettings, error) {
	return domain.ApplicationSettings{}, nil
}

type pageUsageAnalyzerStub struct{ index pluginusage.Index }

func (s pageUsageAnalyzerStub) AnalyzeUsage(string) pluginusage.Index { return s.index }

func TestPagesOptionalDependenciesAreSafe(t *testing.T) {
	t.Parallel()

	pages := NewPages(nil, nil).WithRenderer(nil)

	assert.NotNil(t, pages.logger)
	assert.Nil(t, pages.renderer)
	assert.Nil(t, pages.usageAnalyzer)
}

func TestSavePersistsDerivedPluginUsage(t *testing.T) {
	t.Parallel()

	repository := &pageSaveRepositoryStub{}
	want := pluginusage.Index{
		Version:     pluginusage.Version,
		Fingerprint: "render-plan",
		Modules:     []pluginusage.Module{{PluginID: "me.kumbuka.variables", ModuleID: "variables", Values: []string{"environment"}}},
	}
	pages := NewPages(repository, slog.Default()).WithUsageAnalyzer(pageUsageAnalyzerStub{index: want})

	_, err := pages.save(context.Background(), PageSaveInput{
		Slug:     "guide",
		Title:    "Guide",
		Markdown: "{{var:environment}}",
		Status:   "verified",
	})

	require.NoError(t, err)
	require.NotNil(t, repository.metadata.PluginUsage)
	assert.Equal(t, want, *repository.metadata.PluginUsage)
}

func TestSaveMaterializesStableMarkdown(t *testing.T) {
	t.Parallel()
	repository := &pageSaveRepositoryStub{}
	renderer := md.NewWithRegistry(&plugin.Registry{})
	renderer.SetArtifactBuild("test", "abc")
	pages := NewPages(repository, slog.Default()).WithRenderer(renderer)

	_, err := pages.save(context.Background(), PageSaveInput{
		Slug: "guide", Title: "Guide", Markdown: "# Guide\n\nStatic.", Status: "verified",
	})

	require.NoError(t, err)
	assert.Contains(t, repository.render.HTML, `<h1 id="guide">Guide</h1>`)
	assert.NotEmpty(t, repository.render.Fingerprint)
	require.Len(t, repository.render.Contents, 1)
	assert.Equal(t, "guide", repository.render.Contents[0].ID)
}

func TestSaveLeavesDynamicMarkdownUnmaterialized(t *testing.T) {
	t.Parallel()
	repository := &pageSaveRepositoryStub{}
	renderer := md.NewWithRegistry(&plugin.Registry{})
	renderer.SetArtifactBuild("test", "abc")
	pages := NewPages(repository, slog.Default()).WithRenderer(renderer)

	_, err := pages.save(context.Background(), PageSaveInput{
		Slug: "guide", Title: "Guide", Markdown: "{{var:environment}}", Status: "verified",
	})

	require.NoError(t, err)
	assert.Empty(t, repository.render.Fingerprint)
	assert.Empty(t, repository.render.HTML)
}

func TestSaveValidatesPageBeforePersistence(t *testing.T) {
	t.Parallel()

	pages := NewPages(nil, slog.Default())
	_, err := pages.Save(context.Background(), PageSaveInput{
		Icon:               "not-an-icon",
		Language:           "klingon",
		Status:             "unknown",
		OwnerGroupID:       -1,
		ReviewIntervalDays: 3651,
	})

	validation, ok := errors.AsType[*domain.ValidationError](err)

	require.True(t, ok)
	assert.Equal(t, []domain.FieldError{
		{Field: "slug", Message: "A page path is required."},
		{Field: "title", Message: "Title is required."},
		{Field: "icon", Message: "Choose an icon from the available icon catalog."},
		{Field: "language", Message: "Choose a supported content language."},
		{Field: "status", Message: "Choose valid page workflow settings."},
	}, validation.Fields)
}

func TestMoveValidatesDestinationBeforePersistence(t *testing.T) {
	t.Parallel()

	pages := NewPages(nil, slog.Default())
	err := pages.Move(context.Background(), "guide", "", domain.MovePageOptions{}, domain.User{})

	validation, ok := errors.AsType[*domain.ValidationError](err)

	require.True(t, ok)
	assert.Equal(t, "slug", validation.Fields[0].Field)
}

func TestSaveSlugResolution(t *testing.T) {
	t.Parallel()

	t.Run("derives path from title for a new page", func(t *testing.T) {
		t.Parallel()

		repository := &pageSaveRepositoryStub{}
		_, err := NewPages(repository, slog.Default()).save(context.Background(), PageSaveInput{
			Title:  "Generated Page Path",
			Status: "verified",
		})

		require.NoError(t, err)
		assert.Equal(t, "generated-page-path", repository.slug)
	})

	t.Run("keeps an explicit path for a new page", func(t *testing.T) {
		t.Parallel()

		repository := &pageSaveRepositoryStub{}
		_, err := NewPages(repository, slog.Default()).save(context.Background(), PageSaveInput{
			Slug:   "custom/path",
			Title:  "Generated Page Path",
			Status: "verified",
		})

		require.NoError(t, err)
		assert.Equal(t, "custom/path", repository.slug)
	})

	t.Run("rejects a path made only of slashes", func(t *testing.T) {
		t.Parallel()

		_, err := NewPages(nil, slog.Default()).Save(context.Background(), PageSaveInput{
			Slug:   "/////",
			Title:  "Invalid path",
			Status: "verified",
		})
		validation, ok := errors.AsType[*domain.ValidationError](err)

		require.True(t, ok)
		require.Len(t, validation.Fields, 1)
		assert.Equal(t, "slug", validation.Fields[0].Field)
		assert.Equal(t, "Use a page path without leading, trailing, or repeated slashes.", validation.Fields[0].Message)
	})

	t.Run("rejects repeated slashes inside a path", func(t *testing.T) {
		t.Parallel()

		_, err := NewPages(nil, slog.Default()).Save(context.Background(), PageSaveInput{
			Slug:   "platform//database",
			Title:  "Invalid path",
			Status: "verified",
		})
		validation, ok := errors.AsType[*domain.ValidationError](err)

		require.True(t, ok)
		require.Len(t, validation.Fields, 1)
		assert.Equal(t, "slug", validation.Fields[0].Field)
	})

	t.Run("requires an explicit path when editing an existing page", func(t *testing.T) {
		t.Parallel()

		_, err := NewPages(nil, slog.Default()).Save(context.Background(), PageSaveInput{
			PreviousSlug: "existing-page",
			Title:        "Renamed title",
			Status:       "verified",
		})
		validation, ok := errors.AsType[*domain.ValidationError](err)

		require.True(t, ok)
		require.Len(t, validation.Fields, 1)
		assert.Equal(t, "slug", validation.Fields[0].Field)
	})
}

func TestSaveRequiresExplicitStatus(t *testing.T) {
	t.Parallel()

	_, err := NewPages(nil, slog.Default()).Save(context.Background(), PageSaveInput{Slug: "explicit-path", Title: "Explicit title"})
	validation, ok := errors.AsType[*domain.ValidationError](err)

	require.True(t, ok)
	require.Len(t, validation.Fields, 1)
	assert.Equal(t, "status", validation.Fields[0].Field)
}

func TestBulkValidatesInputsBeforePersistence(t *testing.T) {
	t.Parallel()

	t.Run("pages", func(t *testing.T) {
		t.Parallel()

		err := NewPages(nil, slog.Default()).Bulk(context.Background(), BulkPageInput{})
		validation, ok := errors.AsType[*domain.ValidationError](err)

		require.True(t, ok)
		assert.Equal(t, "pages", validation.Fields[0].Field)
	})

	t.Run("group", func(t *testing.T) {
		t.Parallel()

		err := NewPages(nil, slog.Default()).Bulk(context.Background(), BulkPageInput{Action: "group", Slugs: []string{"guide"}})
		validation, ok := errors.AsType[*domain.ValidationError](err)

		require.True(t, ok)
		assert.Equal(t, "group_id", validation.Fields[0].Field)
	})

	t.Run("status", func(t *testing.T) {
		t.Parallel()

		err := NewPages(nil, slog.Default()).Bulk(context.Background(), BulkPageInput{Action: "status", Slugs: []string{"guide"}, Status: "invalid"})
		validation, ok := errors.AsType[*domain.ValidationError](err)

		require.True(t, ok)
		assert.Equal(t, "status", validation.Fields[0].Field)
	})

	t.Run("tag", func(t *testing.T) {
		t.Parallel()

		err := NewPages(nil, slog.Default()).Bulk(context.Background(), BulkPageInput{Action: "tag", Slugs: []string{"guide"}})
		validation, ok := errors.AsType[*domain.ValidationError](err)

		require.True(t, ok)
		assert.Equal(t, "tag", validation.Fields[0].Field)
	})

	t.Run("move target", func(t *testing.T) {
		t.Parallel()

		err := NewPages(nil, slog.Default()).Bulk(context.Background(), BulkPageInput{Action: "move", Slugs: []string{"guide"}})
		validation, ok := errors.AsType[*domain.ValidationError](err)

		require.True(t, ok)
		assert.Equal(t, "target", validation.Fields[0].Field)
	})

	t.Run("action", func(t *testing.T) {
		t.Parallel()

		err := NewPages(nil, slog.Default()).Bulk(context.Background(), BulkPageInput{Action: "invalid", Slugs: []string{"guide"}})
		validation, ok := errors.AsType[*domain.ValidationError](err)

		require.True(t, ok)
		assert.Equal(t, "action", validation.Fields[0].Field)
	})
}

type bulkMoveRepositoryStub struct {
	pageRepository
	calls  int
	slugs  []string
	target string
}

func (r *bulkMoveRepositoryStub) BulkMovePages(_ context.Context, slugs []string, target string, _ domain.User) error {
	r.calls++
	r.slugs = append([]string(nil), slugs...)
	r.target = target
	return nil
}

func (r *bulkMoveRepositoryStub) LogAudit(context.Context, int64, string, string, string, string) error {
	return nil
}

func TestBulkMoveDelegatesAsSinglePersistenceOperation(t *testing.T) {
	t.Parallel()

	repository := &bulkMoveRepositoryStub{}
	pages := NewPages(repository, slog.Default())
	slugs := []string{"guide/first", "guide/second"}

	err := pages.Bulk(context.Background(), BulkPageInput{
		Action: "move",
		Slugs:  slugs,
		Target: "archive",
		Actor:  domain.User{ID: 7},
	})

	require.NoError(t, err)
	assert.Equal(t, 1, repository.calls)
	assert.Equal(t, slugs, repository.slugs)
	assert.Equal(t, "archive", repository.target)
}
