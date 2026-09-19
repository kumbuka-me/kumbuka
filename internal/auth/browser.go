package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/kumbuka-me/kumbuka/internal/httpresponse"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"golang.org/x/net/http/httpguts"
)

// browserAuthenticator resolves the database-managed browser authentication mode per request.
type browserAuthenticator struct {
	// repository loads authentication settings and persists authenticated identities.
	repository browserRepository
	// modeOverride forces one deployment-managed authentication mode when non-empty.
	modeOverride domain.AuthMode
	// trustedProxy contains deployment-managed trusted-proxy header overrides.
	trustedProxy TrustedProxyHeaders
	// oidcConfig contains deployment-managed OIDC secrets and callback configuration.
	oidcConfig OIDCConfig
	// none authenticates the built-in administrator when authentication is disabled.
	none *None
	// local authenticates Kumbuka-managed browser sessions.
	local *Local
	// localLoginEnabled exposes local recovery login alongside another effective mode.
	localLoginEnabled bool

	// mu protects the cached OIDC integration and its settings key.
	mu sync.Mutex
	// oidcKey fingerprints the settings used to build the cached OIDC integration.
	oidcKey string
	// oidc is the cached OIDC integration for oidcKey.
	oidc *OIDC
}

// ConfigureBrowserAuth constructs database-managed browser authentication.
func ConfigureBrowserAuth(
	ctx context.Context,
	config BrowserConfig,
	repository browserRepository,
) (BrowserAuth, error) {
	browser := &browserAuthenticator{
		repository:        repository,
		modeOverride:      config.ModeOverride,
		trustedProxy:      config.TrustedProxy,
		oidcConfig:        config.OIDC,
		none:              NewNone(repository),
		local:             NewLocal(repository, config.OIDC.PublicURL),
		localLoginEnabled: config.LocalLoginEnabled,
	}

	// Validate the effective startup mode so broken OIDC or local recovery
	// configuration fails early.
	settings, err := browser.currentSettings(ctx)
	if err != nil {
		return BrowserAuth{}, err
	}
	if err := browser.validate(ctx, settings); err != nil {
		return BrowserAuth{}, err
	}

	return BrowserAuth{
		Authenticator:     browser,
		Login:             http.HandlerFunc(browser.login),
		Callback:          http.HandlerFunc(browser.callback),
		Validate:          browser.validate,
		Local:             browser.local,
		LocalLoginAllowed: browser.localLoginAllowed,
	}, nil
}

// Authenticate resolves a user with the currently configured browser authentication mode.
func (b *browserAuthenticator) Authenticate(r *http.Request) (domain.User, error) {
	settings, err := b.currentSettings(r.Context())
	if err != nil {
		return domain.User{}, err
	}

	setupRequired, err := b.setupRequired(r.Context())
	if err != nil {
		return domain.User{}, err
	}
	if setupRequired {
		return domain.User{}, ErrUnauthenticated
	}

	// A bootstrap session is minted only by the one-time setup flow. It keeps
	// the initial local administrator signed in long enough to configure or
	// link the deployment-managed authentication method without exposing local
	// sign-in as an additional login path.
	if b.modeOverride != "" {
		user, err := b.local.authenticateBootstrapSession(r)
		if err == nil {
			return user, nil
		}
		if !errors.Is(err, ErrUnauthenticated) {
			return domain.User{}, err
		}
	}

	if b.localLoginEnabled && domain.AuthMode(settings.Mode) != domain.AuthModeLocal {
		user, err := b.local.Authenticate(r)
		if err == nil {
			return user, nil
		}
		if !errors.Is(err, ErrUnauthenticated) {
			return domain.User{}, err
		}
	}

	authenticator, err := b.authenticatorForSettings(r.Context(), settings)
	if err != nil {
		return domain.User{}, err
	}

	return authenticator.Authenticate(r)
}

// login starts the configured interactive flow or redirects home for non-interactive modes.
func (b *browserAuthenticator) login(w http.ResponseWriter, r *http.Request) {
	settings, err := b.currentSettings(r.Context())
	if err != nil {
		httpresponse.Problem(w, http.StatusInternalServerError, "The request could not be processed.")
		return
	}

	setupRequired, err := b.setupRequired(r.Context())
	if err != nil {
		httpresponse.Problem(w, http.StatusInternalServerError, "The request could not be processed.")
		return
	}
	if setupRequired {
		http.Redirect(w, r, "/setup", http.StatusFound)
		return
	}
	switch domain.AuthMode(settings.Mode) {
	case domain.AuthModeLocal:
		next := r.URL.Query().Get("next")
		target := "/auth/local"

		if next != "" {
			target += "?next=" + url.QueryEscape(next)
		}

		http.Redirect(w, r, target, http.StatusFound)
		return
	case domain.AuthModeOIDC:
	case domain.AuthModeNone, domain.AuthModeTrustedProxy:
		LoginUnavailable().ServeHTTP(w, r)
		return
	default:
		httpresponse.Problem(w, http.StatusInternalServerError, "The request could not be processed.")
		return
	}

	oidcAuth, err := b.oidcFor(r.Context(), settings)
	if err != nil {
		httpresponse.Problem(w, http.StatusInternalServerError, "The request could not be processed.")
		return
	}

	oidcAuth.login(w, r)
}

// callback completes OIDC only while OIDC is the effective authentication mode.
func (b *browserAuthenticator) callback(w http.ResponseWriter, r *http.Request) {
	settings, err := b.currentSettings(r.Context())
	if err != nil {
		httpresponse.Problem(w, http.StatusInternalServerError, "The request could not be processed.")
		return
	}
	if domain.AuthMode(settings.Mode) != domain.AuthModeOIDC {
		httpresponse.Problem(w, http.StatusBadRequest, "OIDC authentication is not enabled.")
		return
	}

	oidcAuth, err := b.oidcFor(r.Context(), settings)
	if err != nil {
		httpresponse.Problem(w, http.StatusInternalServerError, "The request could not be processed.")
		return
	}

	oidcAuth.callback(w, r)
}

// validate checks persisted authentication settings before administrators activate them.
func (b *browserAuthenticator) validate(ctx context.Context, settings domain.AuthenticationSettings) error {
	setupRequired, err := b.setupRequired(ctx)
	if err != nil {
		return err
	}
	if setupRequired {
		return nil
	}

	if err := b.validateSettings(settings); err != nil {
		return err
	}

	if domain.AuthMode(settings.Mode) == domain.AuthModeLocal {
		configured, err := b.repository.HasLocalAdministratorCredential(ctx)
		if err != nil {
			return err
		}
		if !configured {
			return domain.NewValidationError(
				"auth_mode",
				"Local authentication requires an administrator with a local password.",
			)
		}
	}

	if domain.AuthMode(settings.Mode) == domain.AuthModeOIDC {
		_, err := b.oidcFor(ctx, settings)
		return err
	}

	return nil
}

// setupRequired reports whether first-run setup should bypass normal authentication validation.
func (b *browserAuthenticator) setupRequired(ctx context.Context) (bool, error) {
	return b.repository.SetupRequired(ctx)
}

// currentSettings reads database-managed settings and overlays deployment-managed authentication fields.
func (b *browserAuthenticator) currentSettings(ctx context.Context) (domain.AuthenticationSettings, error) {
	// None and local overrides need no provider-specific database settings.
	switch b.modeOverride {
	case domain.AuthModeNone, domain.AuthModeLocal:
		return domain.AuthenticationSettings{Mode: string(b.modeOverride)}, nil
	}

	settings, err := b.repository.ApplicationSettings(ctx)
	if err != nil {
		return domain.AuthenticationSettings{}, err
	}

	authentication := settings.Authentication

	switch b.modeOverride {
	case "":
	case domain.AuthModeTrustedProxy:
		authentication.Mode = string(domain.AuthModeTrustedProxy)
		authentication.TrustedUsernameHeaders = b.trustedProxy.Username
		authentication.TrustedEmailHeaders = b.trustedProxy.Email
		authentication.TrustedDisplayNameHeaders = b.trustedProxy.DisplayName
		authentication.TrustedGroupHeaders = b.trustedProxy.Groups
		authentication.TrustedAdminGroup = b.trustedProxy.AdminGroup
	case domain.AuthModeOIDC:
		authentication.Mode = string(domain.AuthModeOIDC)
		authentication.OIDCIssuer = b.oidcConfig.Issuer
		authentication.OIDCClientID = b.oidcConfig.ClientID
		authentication.OIDCGroupClaim = b.oidcConfig.GroupClaim
		authentication.OIDCAdminGroup = b.oidcConfig.AdminGroup
	default:
		return domain.AuthenticationSettings{}, fmt.Errorf("unsupported auth mode %q", b.modeOverride)
	}

	if domain.AuthMode(authentication.Mode) == domain.AuthModeOIDC && authentication.OIDCGroupSync {
		authentication.OIDCGroupMappings, err = b.repository.OIDCGroupMappings(ctx)
		if err != nil {
			return domain.AuthenticationSettings{}, err
		}
	}

	return authentication, nil
}

// authenticatorForSettings creates the authenticator for one resolved configuration.
func (b *browserAuthenticator) authenticatorForSettings(
	ctx context.Context,
	settings domain.AuthenticationSettings,
) (Authenticator, error) {
	if err := b.validateSettings(settings); err != nil {
		return nil, err
	}

	switch domain.AuthMode(settings.Mode) {
	case domain.AuthModeNone:
		return b.none, nil
	case domain.AuthModeLocal:
		return b.local, nil
	case domain.AuthModeTrustedProxy:
		return NewTrustedProxy(b.repository, TrustedProxyHeaders{
			Username:    settings.TrustedUsernameHeaders,
			Email:       settings.TrustedEmailHeaders,
			DisplayName: settings.TrustedDisplayNameHeaders,
			Groups:      settings.TrustedGroupHeaders,
			AdminGroup:  settings.TrustedAdminGroup,
		}), nil
	case domain.AuthModeOIDC:
		return b.oidcFor(ctx, settings)
	default:
		return nil, fmt.Errorf("unsupported auth mode %q", settings.Mode)
	}
}

// validateSettings checks configuration that does not require contacting an OIDC provider.
func (b *browserAuthenticator) validateSettings(settings domain.AuthenticationSettings) error {
	validation := &domain.ValidationError{}

	switch domain.AuthMode(settings.Mode) {
	case domain.AuthModeNone, domain.AuthModeLocal:
		return nil
	case domain.AuthModeTrustedProxy:
		validateTrustedProxySettings(settings, validation)
	case domain.AuthModeOIDC:
		b.validateOIDCSettings(settings, validation)
	default:
		return fmt.Errorf("unsupported auth mode %q", settings.Mode)
	}

	if len(validation.Fields) == 0 {
		return nil
	}

	return validation
}

// validateTrustedProxySettings appends all static trusted-proxy configuration failures.
func validateTrustedProxySettings(settings domain.AuthenticationSettings, validation *domain.ValidationError) {
	if len(settings.TrustedUsernameHeaders) == 0 {
		appendAuthenticationProblem(validation, "trusted_username_headers", "Configure at least one username header.")
	}
	if strings.TrimSpace(settings.TrustedAdminGroup) != "" && len(settings.TrustedGroupHeaders) == 0 {
		appendAuthenticationProblem(
			validation,
			"trusted_group_headers",
			"Configure at least one group header for external administrator elevation.",
		)
	}

	validateAuthenticationHeaders(validation, "trusted_username_headers", settings.TrustedUsernameHeaders)
	validateAuthenticationHeaders(validation, "trusted_email_headers", settings.TrustedEmailHeaders)
	validateAuthenticationHeaders(validation, "trusted_display_name_headers", settings.TrustedDisplayNameHeaders)
	validateAuthenticationHeaders(validation, "trusted_group_headers", settings.TrustedGroupHeaders)
}

// validateAuthenticationHeaders appends one problem when an active trusted header list contains an invalid name.
func validateAuthenticationHeaders(validation *domain.ValidationError, field string, headers []string) {
	for _, header := range headers {
		if httpguts.ValidHeaderFieldName(header) {
			continue
		}

		appendAuthenticationProblem(validation, field, "Use valid HTTP header names separated by commas.")
		return
	}
}

// validateOIDCSettings appends all static OIDC configuration failures.
func (b *browserAuthenticator) validateOIDCSettings(settings domain.AuthenticationSettings, validation *domain.ValidationError) {
	if strings.TrimSpace(settings.OIDCIssuer) == "" {
		appendAuthenticationProblem(validation, "oidc_issuer", "OIDC issuer is required.")
	}
	if strings.TrimSpace(settings.OIDCClientID) == "" {
		appendAuthenticationProblem(validation, "oidc_client_id", "OIDC client ID is required.")
	}

	usesGroups := settings.OIDCGroupSync || strings.TrimSpace(settings.OIDCAdminGroup) != ""
	if usesGroups && strings.TrimSpace(settings.OIDCGroupClaim) == "" {
		appendAuthenticationProblem(
			validation,
			"oidc_group_claim",
			"Configure the OIDC claim containing group memberships.",
		)
	}
	validateOIDCGroupMappings(settings.OIDCGroupMappings, validation)

	if b.oidcConfig.ClientSecret == "" {
		appendAuthenticationProblem(
			validation,
			"oidc_client_secret",
			"Configure KUMBUKA__OIDC_CLIENT_SECRET before enabling OIDC.",
		)
	}
	if len(b.oidcConfig.SessionSecret) < 32 {
		appendAuthenticationProblem(
			validation,
			"oidc_session_secret",
			"Configure KUMBUKA__OIDC_SESSION_SECRET with at least 32 characters before enabling OIDC.",
		)
	}
}

// validateOIDCGroupMappings appends at most one mapping problem for invalid or duplicate external groups.
func validateOIDCGroupMappings(mappings []domain.OIDCGroupMapping, validation *domain.ValidationError) {
	seen := make(map[string]struct{}, len(mappings))

	for _, mapping := range mappings {
		group := strings.TrimSpace(mapping.OIDCGroup)
		if group == "" || mapping.GroupID <= 0 {
			appendAuthenticationProblem(
				validation,
				"oidc_group_mapping",
				"Choose a Kumbuka group for every OIDC group mapping.",
			)
			return
		}
		if _, duplicate := seen[group]; duplicate {
			appendAuthenticationProblem(
				validation,
				"oidc_group_mapping",
				"Each OIDC group may only be mapped once.",
			)
			return
		}

		seen[group] = struct{}{}
	}
}

// appendAuthenticationProblem adds one safe field-level authentication validation problem.
func appendAuthenticationProblem(validation *domain.ValidationError, field, message string) {
	validation.Fields = append(validation.Fields, domain.FieldError{Field: field, Message: message})
}

// localLoginAllowed reports whether the local sign-in endpoint is active for the effective mode.
func (b *browserAuthenticator) localLoginAllowed(ctx context.Context) (bool, error) {
	settings, err := b.currentSettings(ctx)
	if err != nil {
		return false, err
	}

	return domain.AuthMode(settings.Mode) == domain.AuthModeLocal || b.localLoginEnabled, nil
}

// oidcFor returns a cached OIDC integration for the supplied public settings.
func (b *browserAuthenticator) oidcFor(ctx context.Context, settings domain.AuthenticationSettings) (*OIDC, error) {
	key := oidcSettingsKey(settings)

	b.mu.Lock()
	if b.oidc != nil && b.oidcKey == key {
		configured := b.oidc

		b.mu.Unlock()
		return configured, nil
	}

	b.mu.Unlock()

	configured, err := NewOIDC(ctx, OIDCConfig{
		ClientID:            strings.TrimSpace(settings.OIDCClientID),
		ClientSecret:        b.oidcConfig.ClientSecret,
		Issuer:              strings.TrimSpace(settings.OIDCIssuer),
		SessionSecret:       b.oidcConfig.SessionSecret,
		PublicURL:           b.oidcConfig.PublicURL,
		GroupClaim:          strings.TrimSpace(settings.OIDCGroupClaim),
		GroupSync:           settings.OIDCGroupSync,
		GroupsAuthoritative: settings.OIDCGroupsAuthoritative,
		GroupMappings:       settings.OIDCGroupMappings,
		AdminGroup:          strings.TrimSpace(settings.OIDCAdminGroup),
	}, b.repository)
	if err != nil {
		return nil, err
	}

	b.mu.Lock()

	b.oidcKey = key
	b.oidc = configured

	b.mu.Unlock()
	return configured, nil
}

// oidcSettingsKey fingerprints public OIDC settings that affect the cached integration.
func oidcSettingsKey(settings domain.AuthenticationSettings) string {
	var key strings.Builder
	_, _ = fmt.Fprintf(
		&key,
		"%s\x00%s\x00%s\x00%t\x00%t\x00%s",
		strings.TrimSpace(settings.OIDCIssuer),
		strings.TrimSpace(settings.OIDCClientID),
		strings.TrimSpace(settings.OIDCGroupClaim),
		settings.OIDCGroupSync,
		settings.OIDCGroupsAuthoritative,
		strings.TrimSpace(settings.OIDCAdminGroup),
	)

	for _, mapping := range settings.OIDCGroupMappings {
		_, _ = fmt.Fprintf(&key, "\x00%s=%d", strings.TrimSpace(mapping.OIDCGroup), mapping.GroupID)
	}

	return key.String()
}
