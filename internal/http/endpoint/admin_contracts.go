package endpoint

import (
	"context"
	"time"

	apptemplates "github.com/kumbuka-me/kumbuka/internal/application/templates"
	appusers "github.com/kumbuka-me/kumbuka/internal/application/users"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// adminOverviewService supplies the persisted summary shown on the administration overview.
type adminOverviewService interface {
	Stats(context.Context) (domain.AdminStats, error)
	AttachmentCount(context.Context) (int64, error)
}

// administrationService supplies documentation-health, tag, and audit administration.
type administrationService interface {
	TagInfos(context.Context) ([]domain.TagInfo, error)
	DeleteTag(context.Context, int64) error
	DocumentationHealth(context.Context, time.Time) (domain.DocumentationHealth, error)
	AuditEvents(context.Context, int) ([]domain.AuditEvent, error)
}

// groupReader supplies group data used by editor and administration endpoints.
type groupReader interface {
	Groups(context.Context) ([]domain.Group, error)
	AssignableGroups(context.Context, domain.User) ([]domain.Group, error)
	GroupMembers(context.Context, int64) ([]domain.User, error)
}

// groupWriter mutates collaboration groups and memberships.
type groupWriter interface {
	CreateGroup(context.Context, string) (domain.Group, error)
	DeleteGroup(context.Context, int64) error
	AddGroupMember(context.Context, int64, int64) error
	RemoveGroupMember(context.Context, int64, int64) error
}

// pageAccessAdmin manages inherited page access rules.
type pageAccessAdmin interface {
	PageAccessRules(context.Context) ([]domain.PageAccessRule, error)
	SavePageAccessRule(context.Context, string, int64, domain.PageAccessLevel) error
	DeletePageAccessRule(context.Context, int64) error
}

// recycleBinService owns administrator deleted-page lifecycle operations.
type recycleBinService interface {
	DeletedPages(context.Context) ([]domain.DeletedPage, error)
	RestorePage(context.Context, string) error
	PermanentlyDeletePage(context.Context, string) error
}

// templateService owns reusable page-template administration.
type templateService interface {
	PageTemplates(context.Context) ([]domain.PageTemplate, error)
	PageTemplate(context.Context, int64) (domain.PageTemplate, error)
	CreatePageTemplate(context.Context, apptemplates.PageTemplateInput) (domain.PageTemplate, error)
	UpdatePageTemplate(context.Context, int64, apptemplates.PageTemplateInput) error
	DeletePageTemplate(context.Context, int64) error
}

// tokenService owns personal and administrator API token operations.
type tokenService interface {
	Tokens(context.Context) ([]domain.APIToken, error)
	UserTokens(context.Context, int64) ([]domain.APIToken, error)
	CreateToken(context.Context, string, int64, int64, *time.Time) (domain.IssuedToken, error)
	DeleteUserToken(context.Context, int64, int64) error
	DeleteToken(context.Context, int64) error
}

// userDirectoryService supplies user lookup and search without account mutation capabilities.
type userDirectoryService interface {
	User(context.Context, int64) (domain.User, error)
	SearchUsers(context.Context, string, int) ([]domain.User, error)
}

// userManagementService owns administrator account state and group membership updates.
type userManagementService interface {
	Users(context.Context) ([]domain.AdminUser, error)
	UserGroups(context.Context, int64) ([]domain.Group, error)
	UpdateUser(context.Context, int64, domain.UserRole, bool, []int64, *bool) error
	RevokeUserSessions(context.Context, int64, int64) error
}

// oidcIdentityService owns external identity approval, mapping, and removal operations.
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

// adminUserOverviewService combines account and OIDC data used by the administrator user screen.
type adminUserOverviewService interface {
	userManagementService
	oidcIdentityService
}

// userAccountWriter exposes the complete account mutation without credential or settings capabilities.
type userAccountWriter interface {
	UpdateAccount(context.Context, appusers.UserUpdateInput) error
}
