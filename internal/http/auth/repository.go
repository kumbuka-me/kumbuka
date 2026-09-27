package auth

import (
	"context"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// bearerRepository resolves API tokens to users.
type bearerRepository interface {
	UserByToken(context.Context, string) (domain.User, error)
}

// noneRepository provisions the fixed administrator used by no-auth mode.
type noneRepository interface {
	EnsureAdministrator(context.Context, string, string, string) (domain.User, error)
}

// trustedProxyLoginService resolves trusted-proxy profiles through application registration policy.
type trustedProxyLoginService interface {
	Login(context.Context, string, string, string) (domain.User, error)
}

// trustedProxyRepository persists authorization state for trusted-proxy identities.
type trustedProxyRepository interface {
	SetExternalAdminStatus(context.Context, int64, domain.AuthMode, bool) error
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

// oidcLoginService resolves verified OIDC profiles through application policy.
type oidcLoginService interface {
	Login(context.Context, string, string, string, string, string) (domain.User, error)
}

// oidcRepository persists OIDC identities and synchronized group memberships.
type oidcRepository interface {
	OIDCUser(context.Context, string, string) (domain.User, error)
	SyncOIDCGroups(context.Context, int64, []string, []domain.OIDCGroupMapping, bool) error
	SetExternalAdminStatus(context.Context, int64, domain.AuthMode, bool) error
}

// browserRepository collects the persistence capabilities used by dynamic browser authentication.
type browserRepository interface {
	localRepository
	noneRepository
	oidcRepository
	trustedProxyRepository
	ApplicationSettings(context.Context) (domain.ApplicationSettings, error)
	ResolveOIDCLogin(context.Context, string, string, string, string, string, bool) (domain.User, error)
	RefreshTrustedProxyUser(context.Context, string, string, string) (domain.User, error)
	CreateTrustedProxyUser(context.Context, string, string, string) (domain.User, error)
	HasLocalAdministratorCredential(context.Context) (bool, error)
	OIDCGroupMappings(context.Context) ([]domain.OIDCGroupMapping, error)
	SetupRequired(context.Context) (bool, error)
}
