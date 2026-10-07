package pages

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/pluginusage"
	"github.com/kumbuka-me/kumbuka/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pageSaveRepositoryStub provides controllable page save repository behavior for tests.
type pageSaveRepositoryStub struct {
	// pageContentRepository is embedded to provide the default interface behavior for this fixture.
	pageContentRepository
	// slug records the slug observed by the test double.
	slug string
	// metadata configures the metadata used by the fixture.
	metadata domain.PageMetadata
	// render configures the render used by the fixture.
	render domain.PageRender
	// previous configures the page returned before an edit.
	previous domain.Page
	// saveErr configures a persistence failure.
	saveErr error
}

func (r *pageSaveRepositoryStub) SavePage(
	_ context.Context,
	_, slug, title, _, _, markdown, _ string,
	_, _ []string,
	_ []int64,
	metadata domain.PageMetadata,
	_ map[string]string,
	render domain.PageRender,
	_ domain.User,
) (domain.Page, error) {
	if r.saveErr != nil {
		return domain.Page{}, r.saveErr
	}

	r.slug = slug
	r.metadata = metadata
	r.render = render

	return domain.Page{ID: 7, Slug: slug, Title: title, Markdown: markdown}, nil
}

// navigationIconCacheStub records cache invalidations triggered by page mutations.
type navigationIconCacheStub struct {
	// calls counts cache invalidations observed by the test double.
	calls int
}

func (s *navigationIconCacheStub) InvalidateIcons() { s.calls++ }

func (r *pageSaveRepositoryStub) GetPage(context.Context, string) (domain.Page, error) {
	return r.previous, nil
}

func (r *pageSaveRepositoryStub) ApplicationSettings(context.Context) (domain.ApplicationSettings, error) {
	return domain.ApplicationSettings{}, nil
}

// pageContentPreparerStub provides controllable page content preparer behavior for tests.
type pageContentPreparerStub struct {
	// usage configures the usage used by the fixture.
	usage pluginusage.Index
	// render configures the render used by the fixture.
	render domain.PageRender
}

// pageContentChangeSinkStub records committed page Markdown mutations.
type pageContentChangeSinkStub struct {
	// changes contains observed post-commit mutations.
	changes []PageContentChange
	// err is the configured post-commit hook failure.
	err error
}

func (s *pageContentChangeSinkStub) ContentChanged(_ context.Context, change PageContentChange) error {
	s.changes = append(s.changes, change)
	return s.err
}

func (s pageContentPreparerStub) Prepare(context.Context, string) (*pluginusage.Index, domain.PageRender, error) {
	return utils.ToPtr(s.usage), s.render, nil
}

// denyingPageAccess provides test state for denying page access behavior.
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

	logger := slog.Default()
	pages := NewMutations(nil, nil, nil, logger)

	assert.Same(t, logger, pages.effects.logger)
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

func TestSaveInvalidatesNavigationIconsOnlyAfterPersistenceSucceeds(t *testing.T) {
	t.Parallel()

	cache := &navigationIconCacheStub{}
	repository := &pageSaveRepositoryStub{}
	mutations := NewMutations(repository, nil, nil, slog.Default()).WithNavigationIconInvalidator(cache)

	_, err := mutations.save(context.Background(), PageSaveInput{
		Slug:   "guide",
		Title:  "Guide",
		Status: domain.PageStatusVerified,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, cache.calls)

	repository.saveErr = errors.New("save failed")
	_, err = mutations.save(context.Background(), PageSaveInput{
		Slug:   "guide",
		Title:  "Guide",
		Status: domain.PageStatusVerified,
	})
	require.Error(t, err)
	assert.Equal(t, 1, cache.calls)
}

func TestSaveEmitsCommittedContentChange(t *testing.T) {
	t.Parallel()
	repository := &pageSaveRepositoryStub{previous: domain.Page{Slug: "guide", Markdown: "old"}}
	sink := &pageContentChangeSinkStub{}
	actor := domain.User{ID: 7}
	pages := NewMutations(repository, nil, nil, slog.Default()).WithContentChangeSink(sink)

	page, err := pages.Save(context.Background(), PageSaveInput{
		PreviousSlug: "guide",
		Slug:         "guide",
		Title:        "Guide",
		Markdown:     "new",
		Status:       domain.PageStatusVerified,
		Actor:        actor,
	})

	require.NoError(t, err)
	require.Len(t, sink.changes, 1)
	assert.Equal(t, page, sink.changes[0].Page)
	assert.Equal(t, "old", sink.changes[0].PreviousMarkdown)
	assert.Equal(t, "new", sink.changes[0].Markdown)
	assert.Equal(t, actor, sink.changes[0].Actor)
}

func TestSaveSkipsContentHookWhenMarkdownIsUnchanged(t *testing.T) {
	t.Parallel()
	repository := &pageSaveRepositoryStub{previous: domain.Page{Slug: "guide", Markdown: "same"}}
	sink := &pageContentChangeSinkStub{}
	_, err := NewMutations(repository, nil, nil, slog.Default()).WithContentChangeSink(sink).Save(
		context.Background(),
		PageSaveInput{PreviousSlug: "guide", Slug: "guide", Title: "Guide", Markdown: "same", Status: domain.PageStatusVerified},
	)

	require.NoError(t, err)
	assert.Empty(t, sink.changes)
}

func TestSaveKeepsCommittedPageWhenContentHookFails(t *testing.T) {
	t.Parallel()
	repository := &pageSaveRepositoryStub{}
	sink := &pageContentChangeSinkStub{err: errors.New("hook failed")}
	page, err := NewMutations(repository, nil, nil, slog.Default()).WithContentChangeSink(sink).Save(
		context.Background(),
		PageSaveInput{Slug: "guide", Title: "Guide", Markdown: "new", Status: domain.PageStatusVerified},
	)

	require.NoError(t, err)
	assert.Equal(t, "guide", page.Slug)
	require.Len(t, sink.changes, 1)
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

// pageMoveRepositoryStub records one committed page move.
type pageMoveRepositoryStub struct {
	pageContentRepository
	// moved reports whether MovePage completed.
	moved bool
}

func (s *pageMoveRepositoryStub) MovePage(context.Context, string, string, domain.MovePageOptions, domain.User) error {
	s.moved = true
	return nil
}

func (s *pageMoveRepositoryStub) GetPage(_ context.Context, slug string) (domain.Page, error) {
	return domain.Page{ID: 7, Slug: slug, Title: "Guide"}, nil
}

func TestMoveInvalidatesNavigationIconsAfterPersistence(t *testing.T) {
	t.Parallel()

	repository := &pageMoveRepositoryStub{}
	cache := &navigationIconCacheStub{}
	mutations := NewMutations(repository, nil, nil, slog.Default()).WithNavigationIconInvalidator(cache)

	err := mutations.Move(
		context.Background(),
		"guide",
		"archive/guide",
		domain.MovePageOptions{},
		domain.User{ID: 7},
	)

	require.NoError(t, err)
	assert.True(t, repository.moved)
	assert.Equal(t, 1, cache.calls)
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

// bulkMoveRepositoryStub provides controllable bulk move repository behavior for tests.
type bulkMoveRepositoryStub struct {
	// bulkRepository is embedded to provide the default interface behavior for this fixture.
	bulkRepository
	// calls counts calls observed by the test double.
	calls int
	// slugs records the slugs observed by the test double.
	slugs []string
	// target configures or records the target value used by the fixture.
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

func TestBulkMoveInvalidatesNavigationIcons(t *testing.T) {
	t.Parallel()

	repository := &bulkMoveRepositoryStub{}
	cache := &navigationIconCacheStub{}
	mutations := NewMutations(nil, nil, nil, slog.Default()).WithNavigationIconInvalidator(cache)
	pages := NewBulk(repository, mutations, nil, slog.Default())

	err := pages.Bulk(context.Background(), BulkPageInput{
		Action: BulkPageActionMove,
		Slugs:  []string{"guide"},
		Target: "archive",
		Actor:  domain.User{ID: 7},
	})

	require.NoError(t, err)
	assert.Equal(t, 1, cache.calls)
}
