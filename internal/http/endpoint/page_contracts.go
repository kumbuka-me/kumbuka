package endpoint

import (
	"context"
	"time"

	apppages "github.com/kumbuka-me/kumbuka/internal/application/pages"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/revision"
)

// visiblePageContentService combines raw page content with actor-authorized lookup for exports.
type visiblePageContentService interface {
	GetPage(context.Context, string) (domain.Page, error)
	GetPageFor(context.Context, domain.User, string) (domain.Page, error)
}

// visiblePageLookupService resolves direct and aliased API page reads with actor authorization.
type visiblePageLookupService interface {
	GetPageFor(context.Context, domain.User, string) (domain.Page, error)
	GetPageOrAliasFor(context.Context, domain.User, string) (domain.Page, string, error)
}

// pageEditorQuery loads application data for the create/edit page workflow.
type pageEditorQuery interface {
	Load(context.Context, domain.User, string, int64) (apppages.EditorResult, error)
}

// pageEditorSave executes one browser-editor save workflow.
type pageEditorSave interface {
	Execute(context.Context, apppages.EditorSaveInput) (domain.Page, error)
}

// visiblePageListService returns only list rows visible to the actor.
type visiblePageListService interface {
	ListPagesFor(context.Context, domain.User, int) ([]domain.Page, error)
}

// visiblePageSearchService returns only search results visible to the actor.
type visiblePageSearchService interface {
	SearchFor(context.Context, domain.User, string, int) ([]domain.Page, error)
}

// visiblePageRevisionService returns revision history only for a visible page.
type visiblePageRevisionService interface {
	RevisionsFor(context.Context, domain.User, string) ([]revision.Revision, error)
}

// visiblePageInventoryService returns only inventory rows visible to the actor.
type visiblePageInventoryService interface {
	PageInventoryFor(context.Context, domain.User) ([]domain.Page, error)
}

// visiblePageActions mutates actor-owned page preferences after application authorization.
type visiblePageActions interface {
	SetFavoriteFor(context.Context, domain.User, string, bool) error
	SetPageWatchFor(context.Context, domain.User, string, domain.PageWatchScope) error
}

// pageReportService supplies authorized plugin/report capabilities for one actor.
type pageReportService interface {
	GetPage(context.Context, string) (domain.Page, error)
	Search(context.Context, string, int) ([]domain.Page, error)
	GetPageFor(context.Context, domain.User, string) (domain.Page, error)
	Accessible(domain.User) apppages.AccessibleCatalog
}

// pageContentService supplies raw page content to administrator and portable workflows.
type pageContentService interface {
	GetPage(context.Context, string) (domain.Page, error)
}

// homeQueryService binds dashboard list capabilities to one actor.
type homeQueryService interface {
	Lists(domain.User) apppages.HomeLists
}

// editorDraftService owns private editor autosaves.
type editorDraftService interface {
	Draft(context.Context, int64, string) (domain.PageDraft, error)
	Save(context.Context, apppages.PageDraftSaveInput) (domain.PageDraft, error)
	Delete(context.Context, int64, string) error
}

// pageRenderArtifactStore persists reusable rendered-page artifacts.
type pageRenderArtifactStore interface {
	SavePageRender(context.Context, int64, time.Time, domain.PageRender) error
}

// pagePresenceService owns active editor presence for one page.
type pagePresenceService interface {
	PageEditors(context.Context, string, domain.User) ([]domain.PageEditorPresence, error)
	TouchPageEditor(context.Context, string, domain.User) error
	LeavePageEditor(context.Context, string, domain.User) error
}

// pageTagService lists actor-visible tags exposed through the page API.
type pageTagService interface {
	TagsFor(context.Context, domain.User) ([]string, error)
}

// pageAliasService lists actor-visible aliases used by editor catalog endpoints.
type pageAliasService interface {
	PageAliasesFor(context.Context, domain.User) (map[string]string, error)
}

// pageInventoryService supplies the unfiltered administrator page inventory.
type pageInventoryService interface {
	PageInventory(context.Context) ([]domain.Page, error)
}

// pagePermalinkService resolves immutable page IDs to their current slugs.
type pagePermalinkService interface {
	PageSlugByID(context.Context, int64) (string, error)
}

// pageWriterService exposes the core page create/update/delete mutations used by HTTP endpoints.
type pageWriterService interface {
	Save(context.Context, apppages.PageSaveInput) (domain.Page, error)
	Delete(context.Context, string, domain.User) error
}

// pageMoveService moves a page subtree after application authorization.
type pageMoveService interface {
	Move(context.Context, string, string, domain.MovePageOptions, domain.User) error
}

// pageReviewService records a manual page review.
type pageReviewService interface {
	Review(context.Context, string, domain.User) error
}

// pageApprovalService owns the page approval workflow.
type pageApprovalService interface {
	PageReviewRequest(context.Context, string) (domain.PageReviewRequest, error)
	ReviewGroups(context.Context) ([]domain.Group, error)
	CanReview(context.Context, string, domain.User) (bool, error)
	CanManageReview(domain.PageReviewRequest, domain.User) bool
	RequestReview(context.Context, apppages.PageReviewRequestInput) (domain.PageReviewRequest, error)
	UpdateReview(context.Context, apppages.PageReviewUpdateInput) (domain.PageReviewRequest, error)
	CancelReview(context.Context, int64, string, domain.User) error
	DecideReview(context.Context, apppages.PageReviewDecisionInput) error
}

// pageReviewDiscussionService owns review comments and applicable suggestions.
type pageReviewDiscussionService interface {
	ReviewDetail(context.Context, int64, string, domain.User) (apppages.PageReviewDetail, error)
	AddReviewComment(context.Context, apppages.PageReviewCommentInput) (domain.PageReviewComment, error)
	ApplyReviewSuggestion(context.Context, int64, string, int64, domain.User) (domain.Page, error)
	ApplyAllReviewSuggestions(context.Context, int64, string, domain.User) (domain.Page, error)
}

// pageRevisionWriter restores one historical page revision.
type pageRevisionWriter interface {
	RestoreRevision(context.Context, string, int, domain.User) (domain.Page, error)
}

// pageDiscussionWriter owns page comments and inline suggestions.
type pageDiscussionWriter interface {
	AddComment(context.Context, string, int64, string, string, string, domain.User) (domain.PageComment, error)
	AddSuggestion(context.Context, string, string, string, string, domain.User) (domain.PageComment, error)
	ApplyCommentSuggestion(context.Context, string, int64, domain.User) (domain.Page, error)
	ResolveComment(context.Context, string, int64, bool, domain.User) error
}

// pageImportService restores validated portable pages.
type pageImportService interface {
	Import(context.Context, []apppages.ImportedPage, string, domain.User) (int, error)
}

// pageBulkService executes administrator bulk page mutations.
type pageBulkService interface {
	Bulk(context.Context, apppages.BulkPageInput) error
}
