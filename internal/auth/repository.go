package auth

import (
	"context"
	"time"

	"github.com/kumbuka-me/kumbuka/internal/domain"
)

// bearerRepository resolves API tokens to users.
type bearerRepository interface {
	UserByToken(context.Context, string) (domain.User, error)
}

// noneRepository provisions the fixed administrator used by no-auth mode.
type noneRepository interface {
	EnsureAdministrator(context.Context, string, string, string) (domain.User, error)
}

// trustedProxyRepository resolves identities asserted by a trusted proxy.
type trustedProxyRepository interface {
	TrustedProxyUser(context.Context, string, string, string) (domain.User, error)
	SetExternalAdminStatus(context.Context, int64, string, bool) error
}

// localRepository persists local credentials and browser sessions.
type localRepository interface {
	CreateInitialLocalAdministrator(context.Context, string, string, string, string) (domain.User, error)
	CreateLocalSession(context.Context, int64, string, time.Time) error
	DeleteLocalSession(context.Context, string) error
	LocalCredential(context.Context, string) (user domain.User, passwordHash string, err error)
	LocalUserBySession(context.Context, string) (domain.User, error)
	SetLocalCredential(context.Context, int64, string) error
}

// oidcRepository persists OIDC identities and synchronized group memberships.
type oidcRepository interface {
	LoginOIDCUser(context.Context, string, string, string, string, string) (domain.User, error)
	OIDCUser(context.Context, string, string) (domain.User, error)
	SyncOIDCGroups(context.Context, int64, []string, []domain.OIDCGroupMapping, bool) error
	SetExternalAdminStatus(context.Context, int64, string, bool) error
}

// browserRepository collects the persistence capabilities used by dynamic browser authentication.
type browserRepository interface {
	localRepository
	noneRepository
	oidcRepository
	trustedProxyRepository
	ApplicationSettings(context.Context) (domain.ApplicationSettings, error)
	HasLocalAdministratorCredential(context.Context) (bool, error)
	OIDCGroupMappings(context.Context) ([]domain.OIDCGroupMapping, error)
	SetupRequired(context.Context) (bool, error)
}
