package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	appauthentication "github.com/kumbuka-me/kumbuka/internal/application/authentication"
	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
	"github.com/kumbuka-me/kumbuka/internal/route"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"golang.org/x/net/http/httpguts"
)

// browserAuthenticator resolves the effective browser authentication mode while reusing deployment-fixed adapters.
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
	// oidcLogin applies application-level OIDC login policy.
	oidcLogin oidcLoginService
	// trustedProxyLogin applies application-level trusted-proxy registration policy.
	trustedProxyLogin trustedProxyLoginService
	// localLoginEnabled exposes local recovery login alongside another effective mode.
	localLoginEnabled bool
	// setupRequired reports process-local first-run setup state without request-time database access.
	setupRequired func() bool
	// setupCompleted marks process-local setup complete after the bootstrap transaction commits.
	setupCompleted func()
	// settings points at the immutable process-resident effective authentication settings.
	settings atomic.Pointer[authenticationSettingsSnapshot]

	// mu protects the cached OIDC integration and its settings key.
	mu sync.Mutex
	// oidcKey fingerprints the settings used to build the cached OIDC integration.
	oidcKey string
	// oidc is the cached OIDC integration for oidcKey.
	oidc *OIDC
}

// authenticationSettingsSnapshot owns one immutable effective authentication configuration.
type authenticationSettingsSnapshot struct {
	// value is cloned before publication and never mutated afterwards.
	value domain.AuthenticationSettings
	// authenticator is the reusable adapter for value when it has already been constructed.
	authenticator Authenticator
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
		oidcLogin:         appauthentication.NewOIDC(repository, config.AllowUserRegistrationOverride),
		trustedProxyLogin: appauthentication.NewTrustedProxy(repository, config.AllowUserRegistrationOverride),
		localLoginEnabled: config.LocalLoginEnabled,
		setupRequired:     config.SetupRequired,
		setupCompleted:    config.SetupCompleted,
	}
	browser.local = NewLocal(repository, config.OIDC.PublicURL).WithSetupCompleted(browser.completeSetup)

	// Load and validate the effective authentication settings once. Steady-state
	// requests use the process-resident snapshot until an administrator saves a
	// replacement configuration.
	settings, err := browser.loadSettings(ctx)
	if err != nil {
		return BrowserAuth{}, err
	}
	if err := browser.validate(ctx, settings); err != nil {
		return BrowserAuth{}, err
	}
	browser.applySettings(settings)

	return BrowserAuth{
		Authenticator:     browser,
		Login:             http.HandlerFunc(browser.login),
		Callback:          http.HandlerFunc(browser.callback),
		Validate:          browser.validate,
		ApplySettings:     browser.applySettings,
		Local:             browser.local,
		LocalLoginAllowed: browser.localLoginAllowed,
	}, nil
}

// Authenticate resolves a user with the currently configured browser authentication mode.
func (b *browserAuthenticator) Authenticate(r *http.Request) (domain.User, error) {
	if b.requiresSetup() {
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

	if b.localLoginEnabled && b.modeOverride != domain.AuthModeLocal {
		user, err := b.local.Authenticate(r)
		if err == nil {
			return user, nil
		}
		if !errors.Is(err, ErrUnauthenticated) {
			return domain.User{}, err
		}
	}

	if snapshot := b.settings.Load(); snapshot != nil {
		if snapshot.authenticator != nil {
			return snapshot.authenticator.Authenticate(r)
		}

		authenticator, err := b.authenticatorForSettings(r.Context(), snapshot.value)
		if err != nil {
			return domain.User{}, err
		}
		b.settings.CompareAndSwap(snapshot, &authenticationSettingsSnapshot{
			value:         snapshot.value,
			authenticator: authenticator,
		})
		return authenticator.Authenticate(r)
	}

	settings, err := b.loadSettings(r.Context())
	if err != nil {
		return domain.User{}, err
	}
	authenticator, err := b.authenticatorForSettings(r.Context(), settings)
	if err != nil {
		return domain.User{}, err
	}

	return authenticator.Authenticate(r)
}

// login starts the configured interactive flow or redirects home for non-interactive modes.
func (b *browserAuthenticator) login(w http.ResponseWriter, r *http.Request) {
	if b.requiresSetup() {
		route.Redirect(w, r, "/setup", http.StatusFound)
		return
	}

	settings, err := b.currentSettings(r.Context())
	if err != nil {
		httpresponse.Problem(w, http.StatusInternalServerError, "The request could not be processed.")
		return
	}
	switch settings.Mode {
	case domain.AuthModeLocal:
		next := r.URL.Query().Get("next")
		target := "/auth/local"

		if next != "" {
			target += "?next=" + url.QueryEscape(next)
		}

		route.Redirect(w, r, target, http.StatusFound)
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
	snapshot := b.settings.Load()
	settings, err := b.currentSettings(r.Context())
	if err != nil {
		httpresponse.Problem(w, http.StatusInternalServerError, "The request could not be processed.")
		return
	}
	if settings.Mode != domain.AuthModeOIDC {
		httpresponse.Problem(w, http.StatusBadRequest, "OIDC authentication is not enabled.")
		return
	}
	settings, err = b.refreshOIDCGroupMappings(r.Context(), settings)
	if err != nil {
		httpresponse.Problem(w, http.StatusInternalServerError, "The request could not be processed.")
		return
	}

	oidcAuth, err := b.oidcFor(r.Context(), settings)
	if err != nil {
		httpresponse.Problem(w, http.StatusInternalServerError, "The request could not be processed.")
		return
	}
	if snapshot != nil {
		b.settings.CompareAndSwap(snapshot, &authenticationSettingsSnapshot{
			value:         cloneAuthenticationSettings(settings),
			authenticator: oidcAuth,
		})
	}

	oidcAuth.callback(w, r)
}

// validate checks persisted authentication settings before administrators activate them.
func (b *browserAuthenticator) validate(ctx context.Context, settings domain.AuthenticationSettings) error {
	if b.requiresSetup() {
		return nil
	}

	if err := b.validateSettings(settings); err != nil {
		return err
	}

	if settings.Mode == domain.AuthModeLocal {
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

	if settings.Mode == domain.AuthModeOIDC {
		_, err := b.oidcFor(ctx, settings)
		return err
	}

	return nil
}

// requiresSetup reports process-local first-run setup state without database I/O.
func (b *browserAuthenticator) requiresSetup() bool {
	return b.setupRequired != nil && b.setupRequired()
}

// completeSetup switches a freshly bootstrapped process to its post-setup authentication state.
func (b *browserAuthenticator) completeSetup() {
	if b.setupCompleted != nil {
		b.setupCompleted()
	}
	if b.modeOverride == "" {
		b.applySettings(domain.AuthenticationSettings{Mode: domain.AuthModeLocal})
	}
}

// currentSettings returns the process-resident effective authentication settings.
func (b *browserAuthenticator) currentSettings(ctx context.Context) (domain.AuthenticationSettings, error) {
	if snapshot := b.settings.Load(); snapshot != nil {
		return snapshot.value, nil
	}

	// Directly constructed authenticators in focused tests may not have gone
	// through ConfigureBrowserAuth. Production instances always take the cached
	// branch above after startup initialization.
	return b.loadSettings(ctx)
}

// loadSettings reads persisted authentication settings at startup and overlays deployment-managed fields.
func (b *browserAuthenticator) loadSettings(ctx context.Context) (domain.AuthenticationSettings, error) {
	// Deployment-managed modes that do not depend on persisted OIDC group
	// configuration need no settings query even during startup.
	switch b.modeOverride {
	case domain.AuthModeNone, domain.AuthModeLocal:
		return domain.AuthenticationSettings{Mode: b.modeOverride}, nil
	case domain.AuthModeTrustedProxy:
		return domain.AuthenticationSettings{
			Mode:                      domain.AuthModeTrustedProxy,
			TrustedUsernameHeaders:    slices.Clone(b.trustedProxy.Username),
			TrustedEmailHeaders:       slices.Clone(b.trustedProxy.Email),
			TrustedDisplayNameHeaders: slices.Clone(b.trustedProxy.DisplayName),
			TrustedGroupHeaders:       slices.Clone(b.trustedProxy.Groups),
			TrustedAdminGroup:         b.trustedProxy.AdminGroup,
		}, nil
	}

	settings, err := b.repository.ApplicationSettings(ctx)
	if err != nil {
		return domain.AuthenticationSettings{}, err
	}

	authentication := settings.Authentication

	switch b.modeOverride {
	case "":
	case domain.AuthModeOIDC:
		authentication.Mode = domain.AuthModeOIDC
		authentication.OIDCIssuer = b.oidcConfig.Issuer
		authentication.OIDCClientID = b.oidcConfig.ClientID
		authentication.OIDCGroupClaim = b.oidcConfig.GroupClaim
		authentication.OIDCAdminGroup = b.oidcConfig.AdminGroup
	default:
		return domain.AuthenticationSettings{}, fmt.Errorf("unsupported auth mode %q", b.modeOverride)
	}

	if oidcGroupMappingsEnabled(authentication) {
		authentication.OIDCGroupMappings, err = b.repository.OIDCGroupMappings(ctx)
		if err != nil {
			return domain.AuthenticationSettings{}, err
		}
	}

	return cloneAuthenticationSettings(authentication), nil
}

// refreshOIDCGroupMappings reloads mutable group references only during an interactive OIDC callback.
func (b *browserAuthenticator) refreshOIDCGroupMappings(
	ctx context.Context,
	settings domain.AuthenticationSettings,
) (domain.AuthenticationSettings, error) {
	if !oidcGroupMappingsEnabled(settings) {
		return settings, nil
	}

	mappings, err := b.repository.OIDCGroupMappings(ctx)
	if err != nil {
		return domain.AuthenticationSettings{}, err
	}
	settings.OIDCGroupMappings = mappings
	return settings, nil
}

// applySettings replaces the effective authentication snapshot after persisted settings are saved.
func (b *browserAuthenticator) applySettings(settings domain.AuthenticationSettings) {
	settings = cloneAuthenticationSettings(settings)
	b.settings.Store(&authenticationSettingsSnapshot{
		value:         settings,
		authenticator: b.cachedAuthenticatorForSettings(settings),
	})
}

// cachedAuthenticatorForSettings returns an already-constructible steady-state authenticator without external I/O.
func (b *browserAuthenticator) cachedAuthenticatorForSettings(settings domain.AuthenticationSettings) Authenticator {
	switch settings.Mode {
	case domain.AuthModeNone:
		return b.none
	case domain.AuthModeLocal:
		return b.local
	case domain.AuthModeTrustedProxy:
		return NewTrustedProxy(b.trustedProxyLogin, TrustedProxyHeaders{
			Username:    settings.TrustedUsernameHeaders,
			Email:       settings.TrustedEmailHeaders,
			DisplayName: settings.TrustedDisplayNameHeaders,
			Groups:      settings.TrustedGroupHeaders,
			AdminGroup:  settings.TrustedAdminGroup,
		})
	case domain.AuthModeOIDC:
		key := oidcSettingsKey(settings)
		b.mu.Lock()
		defer b.mu.Unlock()
		if b.oidc != nil && b.oidcKey == key {
			return b.oidc
		}
	}
	return nil
}

// cloneAuthenticationSettings prevents request handlers from sharing mutable slice backing arrays with configuration writers.
func cloneAuthenticationSettings(settings domain.AuthenticationSettings) domain.AuthenticationSettings {
	settings.TrustedUsernameHeaders = slices.Clone(settings.TrustedUsernameHeaders)
	settings.TrustedEmailHeaders = slices.Clone(settings.TrustedEmailHeaders)
	settings.TrustedDisplayNameHeaders = slices.Clone(settings.TrustedDisplayNameHeaders)
	settings.TrustedGroupHeaders = slices.Clone(settings.TrustedGroupHeaders)
	settings.OIDCGroupMappings = slices.Clone(settings.OIDCGroupMappings)
	return settings
}

// authenticatorForSettings creates the authenticator for one resolved configuration.
func (b *browserAuthenticator) authenticatorForSettings(
	ctx context.Context,
	settings domain.AuthenticationSettings,
) (Authenticator, error) {
	if err := b.validateSettings(settings); err != nil {
		return nil, err
	}

	switch settings.Mode {
	case domain.AuthModeNone:
		return b.none, nil
	case domain.AuthModeLocal:
		return b.local, nil
	case domain.AuthModeTrustedProxy:
		return NewTrustedProxy(b.trustedProxyLogin, TrustedProxyHeaders{
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

	switch settings.Mode {
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

// oidcGroupMappingsEnabled reports whether persisted authentication settings require OIDC group mappings.
func oidcGroupMappingsEnabled(settings domain.AuthenticationSettings) bool {
	return settings.Mode == domain.AuthModeOIDC && settings.OIDCGroupSync
}

// trustedAdminGroupMissingHeaders reports whether administrator elevation is configured without any trusted group source.
func trustedAdminGroupMissingHeaders(settings domain.AuthenticationSettings) bool {
	return strings.TrimSpace(settings.TrustedAdminGroup) != "" && len(settings.TrustedGroupHeaders) == 0
}

// validateTrustedProxySettings appends all static trusted-proxy configuration failures.
func validateTrustedProxySettings(settings domain.AuthenticationSettings, validation *domain.ValidationError) {
	if len(settings.TrustedUsernameHeaders) == 0 {
		appendAuthenticationProblem(validation, "trusted_username_headers", "Configure at least one username header.")
	}
	if trustedAdminGroupMissingHeaders(settings) {
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

	return settings.Mode == domain.AuthModeLocal || b.localLoginEnabled, nil
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
	}, b.repository, b.oidcLogin)
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
