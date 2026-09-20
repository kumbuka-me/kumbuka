package pages

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/pluginusage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type pageSaveRepositoryStub struct {
	pageContentRepository
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

type pageContentPreparerStub struct {
	usage  pluginusage.Index
	render domain.PageRender
}

func (s pageContentPreparerStub) Prepare(context.Context, string) (*pluginusage.Index, domain.PageRender, error) {
	usage := s.usage
	return &usage, s.render, nil
}

type denyingPageAccess struct{}

func (denyingPageAccess) CanView(context.Context, domain.User, string) (bool, error) {
	return false, nil
}
func (denyingPageAccess) CanEdit(context.Context, domain.User, string) (bool, error) {
	return false, nil
}
func (denyingPageAccess) FilterPages(context.Context, domain.User, []domain.Page) ([]domain.Page, error) {
	return nil, nil
}

func TestPageUseCasesEnforceResourceAccessBeforePersistence(t *testing.T) {
	t.Parallel()

	actor := domain.User{ID: 7, Role: domain.UserRoleEditor}
	_, err := NewLookup(nil, denyingPageAccess{}).GetPageFor(context.Background(), actor, "private")
	require.ErrorIs(t, err, domain.ErrNotFound)

	_, err = NewMutations(nil, denyingPageAccess{}, nil, slog.Default()).Save(context.Background(), PageSaveInput{
		Slug:   "private",
		Title:  "Private",
		Status: "draft",
		Actor:  actor,
	})
	require.ErrorIs(t, err, domain.ErrForbidden)
}

func TestPagesOptionalDependenciesAreSafe(t *testing.T) {
	t.Parallel()

	pages := NewMutations(nil, nil, nil, nil)

	assert.NotNil(t, pages.effects.logger)
	assert.Nil(t, pages.content)
	assert.Nil(t, pages.icons)
}

func TestSavePersistsDerivedPluginUsage(t *testing.T) {
	t.Parallel()

	repository := &pageSaveRepositoryStub{}
	want := pluginusage.Index{
		Version:     pluginusage.Version,
		Fingerprint: "render-plan",
		Modules:     []pluginusage.Module{{PluginID: "me.kumbuka.variables", ModuleID: "variables", Values: []string{"environment"}}},
	}
	pages := NewMutations(repository, nil, nil, slog.Default()).WithContentPreparer(pageContentPreparerStub{usage: want})

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

func TestSaveValidatesPageBeforePersistence(t *testing.T) {
	t.Parallel()

	pages := NewMutations(nil, nil, nil, slog.Default())
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

	pages := NewMutations(nil, nil, nil, slog.Default())
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
		_, err := NewMutations(repository, nil, nil, slog.Default()).save(context.Background(), PageSaveInput{
			Title:  "Generated Page Path",
			Status: "verified",
		})

		require.NoError(t, err)
		assert.Equal(t, "generated-page-path", repository.slug)
	})

	t.Run("keeps an explicit path for a new page", func(t *testing.T) {
		t.Parallel()

		repository := &pageSaveRepositoryStub{}
		_, err := NewMutations(repository, nil, nil, slog.Default()).save(context.Background(), PageSaveInput{
			Slug:   "custom/path",
			Title:  "Generated Page Path",
			Status: "verified",
		})

		require.NoError(t, err)
		assert.Equal(t, "custom/path", repository.slug)
	})

	t.Run("rejects a path made only of slashes", func(t *testing.T) {
		t.Parallel()

		_, err := NewMutations(nil, nil, nil, slog.Default()).Save(context.Background(), PageSaveInput{
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

		_, err := NewMutations(nil, nil, nil, slog.Default()).Save(context.Background(), PageSaveInput{
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

		_, err := NewMutations(nil, nil, nil, slog.Default()).Save(context.Background(), PageSaveInput{
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

	_, err := NewMutations(nil, nil, nil, slog.Default()).Save(context.Background(), PageSaveInput{Slug: "explicit-path", Title: "Explicit title"})
	validation, ok := errors.AsType[*domain.ValidationError](err)

	require.True(t, ok)
	require.Len(t, validation.Fields, 1)
	assert.Equal(t, "status", validation.Fields[0].Field)
}

func TestBulkValidatesInputsBeforePersistence(t *testing.T) {
	t.Parallel()

	t.Run("pages", func(t *testing.T) {
		t.Parallel()

		err := NewBulk(nil, nil, nil, slog.Default()).Bulk(context.Background(), BulkPageInput{})
		validation, ok := errors.AsType[*domain.ValidationError](err)

		require.True(t, ok)
		assert.Equal(t, "pages", validation.Fields[0].Field)
	})

	t.Run("group", func(t *testing.T) {
		t.Parallel()

		err := NewBulk(nil, nil, nil, slog.Default()).Bulk(context.Background(), BulkPageInput{Action: "group", Slugs: []string{"guide"}})
		validation, ok := errors.AsType[*domain.ValidationError](err)

		require.True(t, ok)
		assert.Equal(t, "group_id", validation.Fields[0].Field)
	})

	t.Run("status", func(t *testing.T) {
		t.Parallel()

		err := NewBulk(nil, nil, nil, slog.Default()).Bulk(context.Background(), BulkPageInput{Action: "status", Slugs: []string{"guide"}, Status: "invalid"})
		validation, ok := errors.AsType[*domain.ValidationError](err)

		require.True(t, ok)
		assert.Equal(t, "status", validation.Fields[0].Field)
	})

	t.Run("tag", func(t *testing.T) {
		t.Parallel()

		err := NewBulk(nil, nil, nil, slog.Default()).Bulk(context.Background(), BulkPageInput{Action: "tag", Slugs: []string{"guide"}})
		validation, ok := errors.AsType[*domain.ValidationError](err)

		require.True(t, ok)
		assert.Equal(t, "tag", validation.Fields[0].Field)
	})

	t.Run("move target", func(t *testing.T) {
		t.Parallel()

		err := NewBulk(nil, nil, nil, slog.Default()).Bulk(context.Background(), BulkPageInput{Action: "move", Slugs: []string{"guide"}})
		validation, ok := errors.AsType[*domain.ValidationError](err)

		require.True(t, ok)
		assert.Equal(t, "target", validation.Fields[0].Field)
	})

	t.Run("action", func(t *testing.T) {
		t.Parallel()

		err := NewBulk(nil, nil, nil, slog.Default()).Bulk(context.Background(), BulkPageInput{Action: "invalid", Slugs: []string{"guide"}})
		validation, ok := errors.AsType[*domain.ValidationError](err)

		require.True(t, ok)
		assert.Equal(t, "action", validation.Fields[0].Field)
	})
}

type bulkMoveRepositoryStub struct {
	bulkRepository
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
	pages := NewBulk(repository, nil, nil, slog.Default())
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
