package endpoint

import (
	"context"
	"net/http"
	"time"

	apppages "github.com/kumbuka-me/kumbuka/internal/application/pages"
	appsettings "github.com/kumbuka-me/kumbuka/internal/application/settings"
	apptemplates "github.com/kumbuka-me/kumbuka/internal/application/templates"
	appusers "github.com/kumbuka-me/kumbuka/internal/application/users"
	appwebhooks "github.com/kumbuka-me/kumbuka/internal/application/webhooks"
	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/revision"
)

type viewDataService interface {
	Load(*http.Request, *webview.Views, string) (webview.Layout, error)
}

type administrationService interface {
	Stats(context.Context) (domain.AdminStats, error)
	TagInfos(context.Context) ([]domain.TagInfo, error)
	DeleteTag(context.Context, int64) error
	DocumentationHealth(context.Context, time.Time) (domain.DocumentationHealth, error)
	AuditEvents(context.Context, int) ([]domain.AuditEvent, error)
}

// Actor-scoped catalog interfaces expose reads that apply resource authorization inside the application layer.
type visiblePageService interface {
	GetPageFor(context.Context, domain.User, string) (domain.Page, error)
}

type visiblePageContentService interface {
	pageContentService
	visiblePageService
}

type visiblePageLookupService interface {
	visiblePageService
	GetPageOrAliasFor(context.Context, domain.User, string) (domain.Page, string, error)
}

type pageEditorQuery interface {
	Load(context.Context, domain.User, string, int64) (apppages.EditorResult, error)
}

type pageEditorSave interface {
	Execute(context.Context, apppages.EditorSaveInput) (domain.Page, error)
}

type visiblePageListService interface {
	ListPagesFor(context.Context, domain.User, int) ([]domain.Page, error)
}

type visiblePageSearchService interface {
	SearchFor(context.Context, domain.User, string, int) ([]domain.Page, error)
}

type visiblePageRevisionService interface {
	RevisionsFor(context.Context, domain.User, string) ([]revision.Revision, error)
}

type visiblePageInventoryService interface {
	PageInventoryFor(context.Context, domain.User) ([]domain.Page, error)
}

type visiblePageActions interface {
	SetFavoriteFor(context.Context, domain.User, string, bool) error
	SetPageWatchFor(context.Context, domain.User, string, string) error
}

type accessibleCatalogService interface {
	Accessible(domain.User) apppages.AccessibleCatalog
}

type scopedPageCatalogService interface {
	pageReportCatalogService
	visiblePageService
	accessibleCatalogService
}

// Catalog interfaces are intentionally consumer-oriented instead of mirroring
// every method exposed by apppages.Catalog.
type pageContentService interface {
	GetPage(context.Context, string) (domain.Page, error)
}

type pageLookupService interface {
	pageContentService
	ResolvePageAlias(context.Context, string) (string, error)
}

type pageListService interface {
	ListPages(context.Context, int) ([]domain.Page, error)
}

type pageSearchService interface {
	Search(context.Context, string, int) ([]domain.Page, error)
}

type pageReportCatalogService interface {
	pageContentService
	pageSearchService
}

type homeCatalogService interface {
	pageListService
	Favorites(context.Context, int64) ([]domain.Page, error)
	RecentViewed(context.Context, int64, int) ([]domain.Page, error)
	RecentEdited(context.Context, int64, int) ([]domain.RecentEdit, error)
	Popular(context.Context, int) ([]domain.Page, error)
}

type homeQueryService interface {
	Lists(domain.User) apppages.HomeLists
}

type draftListService interface {
	List(context.Context, int64, int) ([]domain.PageDraft, error)
}

type editorDraftService interface {
	Draft(context.Context, int64, string) (domain.PageDraft, error)
	Save(context.Context, apppages.PageDraftSaveInput) (domain.PageDraft, error)
	Delete(context.Context, int64, string) error
}

type sidebarCatalogService interface {
	Favorites(context.Context, int64) ([]domain.Page, error)
	RecentViewed(context.Context, int64, int) ([]domain.Page, error)
}

type pageViewCatalogService interface {
	pageLookupService
	pageSearchService
	pageWatchReader
	accessibleCatalogService
	IsFavorite(context.Context, string, int64) (bool, error)
	PageLinks(context.Context, string) ([]domain.PageLink, error)
	PageComments(context.Context, string) ([]domain.PageComment, error)
	SavePageRender(context.Context, int64, time.Time, domain.PageRender) error
}

type favoriteService interface {
	SetFavorite(context.Context, string, int64, bool) error
}

type pageWatchReader interface {
	PageWatch(context.Context, string, int64) (domain.PageWatch, error)
}

type pageWatchService interface {
	pageWatchReader
	SetPageWatch(context.Context, string, int64, string) error
}

type pagePresenceService interface {
	PageEditors(context.Context, string, domain.User) ([]domain.PageEditorPresence, error)
	TouchPageEditor(context.Context, string, domain.User) error
	LeavePageEditor(context.Context, string, domain.User) error
}

type pageRevisionService interface {
	Revisions(context.Context, string) ([]revision.Revision, error)
}

type pageTagService interface {
	Tags(context.Context) ([]string, error)
}

type pageAliasService interface {
	PageAliases(context.Context) (map[string]string, error)
}

type pageInventoryService interface {
	PageInventory(context.Context) ([]domain.Page, error)
}

type pagePermalinkService interface {
	PageSlugByID(context.Context, int64) (string, error)
}

type groupReader interface {
	Groups(context.Context) ([]domain.Group, error)
	AssignableGroups(context.Context, domain.User) ([]domain.Group, error)
	GroupMembers(context.Context, int64) ([]domain.User, error)
}

type groupWriter interface {
	CreateGroup(context.Context, string) (domain.Group, error)
	DeleteGroup(context.Context, int64) error
	AddGroupMember(context.Context, int64, int64) error
	RemoveGroupMember(context.Context, int64, int64) error
}

type pageAccessAdmin interface {
	PageAccessRules(context.Context) ([]domain.PageAccessRule, error)
	SavePageAccessRule(context.Context, string, int64, string) error
	DeletePageAccessRule(context.Context, int64) error
}

// Knowledge interfaces expose graph and saved-search operations used by handlers.
type knowledgeGraphService interface {
	KnowledgeGraph(context.Context, int) (domain.KnowledgeGraph, error)
	KnowledgeGraphFor(context.Context, domain.User, int) (domain.KnowledgeGraph, error)
}

type savedSearchService interface {
	SaveSavedSearch(context.Context, int64, int64, string, string, bool) error
	DeleteSavedSearch(context.Context, int64, int64) error
}

type notificationReader interface {
	Notifications(context.Context, int64, int) (notifications []domain.Notification, unread int, err error)
}

type notificationService interface {
	notificationReader
	MarkNotificationRead(context.Context, int64, int64) error
	MarkAllNotificationsRead(context.Context, int64) error
	OpenNotification(context.Context, int64, int64) (string, error)
}

type webhookAdminService interface {
	Webhooks(context.Context) ([]domain.Webhook, error)
	WebhookDeliveries(context.Context, int) ([]domain.WebhookDelivery, error)
	SaveWebhook(context.Context, int64, appwebhooks.WebhookInput) (domain.Webhook, error)
	DeleteWebhook(context.Context, int64) error
	TestWebhook(context.Context, int64) error
	RevealWebhookHeader(context.Context, int64, int64) (string, error)
}

// Media readers and writers are separated so read-only exports and downloads
// do not receive upload/delete capabilities.
type imageContentService interface {
	ImageContent(context.Context, int64) (domain.ImageData, error)
}

type imageListService interface {
	Images(context.Context) ([]domain.Image, error)
	SearchImages(context.Context, string, int, int) ([]domain.Image, error)
}

type userImageService interface {
	ImagesByUser(context.Context, int64) ([]domain.Image, error)
	SearchImagesByUser(context.Context, int64, string, int, int) ([]domain.Image, error)
}

type imageService interface {
	imageContentService
	Images(context.Context) ([]domain.Image, error)
	ImagesByUser(context.Context, int64) ([]domain.Image, error)
	SearchImages(context.Context, string, int, int) ([]domain.Image, error)
	SearchImagesByUser(context.Context, int64, string, int, int) ([]domain.Image, error)
	UploadImage(context.Context, string, []byte, domain.User) (domain.Image, error)
	DeleteImage(context.Context, int64, domain.User) error
}

type attachmentService interface {
	Attachments(context.Context) ([]domain.Attachment, error)
	AttachmentContent(context.Context, int64) (domain.AttachmentData, error)
	UploadAttachment(context.Context, string, []byte, domain.User) (domain.Attachment, error)
	DeleteAttachment(context.Context, int64, domain.User) error
}

type navigationService interface {
	NavigationPages(context.Context) ([]domain.Page, error)
	VisiblePages(context.Context, domain.User) ([]domain.Page, error)
	NavigationItems(context.Context) ([]domain.NavigationItem, error)
	NavigationIcons(context.Context) (map[string]string, error)
	SetNavigationIcon(context.Context, string, string) error
}

// Page mutation interfaces follow the individual workflows rather than
// exposing the complete Pages service to every write handler.
type pageWriterService interface {
	Save(context.Context, apppages.PageSaveInput) (domain.Page, error)
	Delete(context.Context, string, domain.User) error
}

type pageMoveService interface {
	Move(context.Context, string, string, domain.MovePageOptions, domain.User) error
}

type pageReviewService interface {
	Review(context.Context, string, domain.User) error
}

type pageApprovalService interface {
	PageReviewRequest(context.Context, string) (domain.PageReviewRequest, error)
	ReviewGroups(context.Context) ([]domain.Group, error)
	CanReview(context.Context, string, domain.User) (bool, error)
	CanManageReview(domain.PageReviewRequest, domain.User) bool
	RequestReview(context.Context, apppages.PageReviewRequestInput) (domain.PageReviewRequest, error)
	UpdateReview(context.Context, apppages.PageReviewUpdateInput) (domain.PageReviewRequest, error)
	CancelReview(context.Context, int64, string, domain.User) error
	DecideReview(context.Context, apppages.PageReviewDecisionInput) error
	ReviewDetail(context.Context, int64, string, domain.User) (apppages.PageReviewDetail, error)
	AddReviewComment(context.Context, apppages.PageReviewCommentInput) (domain.PageReviewComment, error)
	ApplyReviewSuggestion(context.Context, int64, string, int64, domain.User) (domain.Page, error)
	ApplyAllReviewSuggestions(context.Context, int64, string, domain.User) (domain.Page, error)
}

type pageRevisionWriter interface {
	RestoreRevision(context.Context, string, int, domain.User) (domain.Page, error)
}

type pageDiscussionWriter interface {
	AddComment(context.Context, string, int64, string, string, string, domain.User) (domain.PageComment, error)
	AddSuggestion(context.Context, string, string, string, string, domain.User) (domain.PageComment, error)
	ApplyCommentSuggestion(context.Context, string, int64, domain.User) (domain.Page, error)
	ResolveComment(context.Context, string, int64, bool, domain.User) error
}

type pageImportService interface {
	Import(context.Context, []apppages.ImportedPage, string, domain.User) (int, error)
}

type pageBulkService interface {
	Bulk(context.Context, apppages.BulkPageInput) error
}

type preferenceService interface {
	Preferences(context.Context, int64) (domain.UserPreferences, error)
	SavePreferences(context.Context, int64, domain.UserPreferences) error
	SetShowPageContents(context.Context, int64, bool) error
	SetExpandedNavigation(context.Context, int64, []string) error
	SetSidebarWidth(context.Context, int64, int) error
}

type recycleBinService interface {
	DeletedPages(context.Context) ([]domain.DeletedPage, error)
	RestorePage(context.Context, string) error
	PermanentlyDeletePage(context.Context, string) error
}

type settingsService interface {
	ApplicationSettings(context.Context) (domain.ApplicationSettings, error)
	PDFHeaders(context.Context) ([]domain.PDFHeader, error)
	PDFRequestHeaders(context.Context) ([]domain.PDFHeader, error)
	ResolvePDFRequestHeaders(context.Context, []appsettings.PDFHeaderInput) ([]domain.PDFHeader, error)
	RevealPDFHeader(context.Context, int64) (string, error)
	SaveApplicationSettings(context.Context, domain.ApplicationSettings, int64) error
	SavePDFSettings(context.Context, string, []appsettings.PDFHeaderInput, int64) error
	SaveAuthenticationSettings(context.Context, domain.AuthenticationSettings, int64) error
	RecordLocalPasswordUpdated(context.Context, domain.User)
}

type sharingService interface {
	CreatePageShareLink(context.Context, string, domain.User) (apppages.IssuedPageShareLink, error)
	PageShareLink(context.Context, string) (domain.PageShareLink, error)
}

type systemService interface {
	Ping(context.Context) error
	SetupRequired(context.Context) (bool, error)
	RecordSetupCompleted(context.Context, domain.User)
}

type templateService interface {
	PageTemplates(context.Context) ([]domain.PageTemplate, error)
	PageTemplate(context.Context, int64) (domain.PageTemplate, error)
	CreatePageTemplate(context.Context, apptemplates.PageTemplateInput) (domain.PageTemplate, error)
	UpdatePageTemplate(context.Context, int64, apptemplates.PageTemplateInput) error
	DeletePageTemplate(context.Context, int64) error
}

type tokenService interface {
	Tokens(context.Context) ([]domain.APIToken, error)
	UserTokens(context.Context, int64) ([]domain.APIToken, error)
	CreateToken(context.Context, string, int64, int64, *time.Time) (domain.IssuedToken, error)
	DeleteUserToken(context.Context, int64, int64) error
	DeleteToken(context.Context, int64) error
}

// User administration is split into directory, account management, and OIDC
// identity capabilities.
type userDirectoryService interface {
	User(context.Context, int64) (domain.User, error)
	SearchUsers(context.Context, string, int) ([]domain.User, error)
}

type userManagementService interface {
	Users(context.Context) ([]domain.AdminUser, error)
	UserGroups(context.Context, int64) ([]domain.Group, error)
	UpdateUser(context.Context, int64, string, bool, []int64, *bool) error
	RevokeUserSessions(context.Context, int64, int64) error
}

type oidcIdentityService interface {
	OIDCIdentities(context.Context) ([]domain.OIDCIdentity, error)
	OIDCGroupMappings(context.Context) ([]domain.OIDCGroupMapping, error)
	PendingOIDCIdentities(context.Context) ([]domain.PendingOIDCIdentity, error)
	ApprovePendingOIDCIdentity(context.Context, int64, int64) (domain.User, error)
	LinkPendingOIDCIdentity(context.Context, int64, int64, int64) (domain.User, error)
	SetPendingOIDCIdentityRejected(context.Context, int64, bool, int64) error
	RemoveOIDCIdentity(context.Context, int64, string, string, int64) error
	HasLocalCredential(context.Context, int64) (bool, error)
}

type adminUserOverviewService interface {
	userManagementService
	oidcIdentityService
}

// userAccountWriter exposes the complete account mutation without credential or settings capabilities.
type userAccountWriter interface {
	UpdateAccount(context.Context, appusers.UserUpdateInput) error
}
