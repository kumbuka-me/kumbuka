package endpoint

import (
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/http/auth"
	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// contentLanguageOptions contains the languages supported by page search and presentation.
var contentLanguageOptions = []webview.ContentLanguageOption{
	{Code: "en", Label: "English"},
	{Code: "en-US", Label: "English (United States)"},
	{Code: "en-GB", Label: "English (United Kingdom)"},
	{Code: "de", Label: "German"},
	{Code: "de-CH", Label: "German (Switzerland)"},
	{Code: "de-AT", Label: "German (Austria)"},
	{Code: "fr", Label: "French"},
	{Code: "fr-CH", Label: "French (Switzerland)"},
	{Code: "it", Label: "Italian"},
	{Code: "it-CH", Label: "Italian (Switzerland)"},
	{Code: "es", Label: "Spanish"},
	{Code: "nl", Label: "Dutch"},
	{Code: "pt", Label: "Portuguese"},
}

// AdminConfiguration renders runtime and application-wide configuration.
func AdminConfiguration(
	viewDataUseCases viewDataService,
	groupUseCases groupReader,
	userUseCases oidcIdentityService,
	settingsUseCases settingsService,
	views *webview.Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := administrationData(r, viewDataUseCases, views, "Configuration", "configuration")
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		groups, err := groupUseCases.Groups(r.Context())
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		data.Groups = groups
		data.ApplicationSettings.Authentication.OIDCGroupMappings, err = userUseCases.OIDCGroupMappings(r.Context())
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		data.PDFHeaders, err = settingsUseCases.PDFHeaders(r.Context())
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}
		data.ContentLanguages = contentLanguageOptions

		views.Render(w, "admin_configuration", data)
	}
}

// isContentLanguage reports whether a configured content language is exposed by the admin UI.
func isContentLanguage(value string) bool {
	return slices.ContainsFunc(contentLanguageOptions, func(option webview.ContentLanguageOption) bool {
		return option.Code == value
	})
}

// SaveAdminAuthentication updates database-managed browser authentication settings.
func SaveAdminAuthentication(
	settingsUseCases settingsService,
	browserAuth auth.BrowserAuth,
	views *webview.Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		admin := currentUser(r)
		if err := r.ParseForm(); err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid authentication form.")
			return
		}

		settings := authenticationSettingsFromForm(r)
		if views.Runtime().AuthModeOverride != "" {
			current, err := settingsUseCases.ApplicationSettings(r.Context())
			if err != nil {
				httpresponse.InternalServerError(views.Logger(), w, err)
				return
			}

			settings = preserveRuntimeManagedAuthenticationSettings(settings, current.Authentication, views.Runtime())
		}

		effective := effectiveAuthenticationSettings(settings, views.Runtime())

		if err := browserAuth.Validate(r.Context(), effective); err != nil {
			if tryWriteValidationProblem(w, err, "Authentication validation failed.") {
				return
			}

			views.Logger().Warn(
				"authentication settings rejected",
				"event", "authentication_settings_rejected",
				"mode", effective.Mode,
				"error", err,
			)

			field := "auth_mode"
			message := "The authentication configuration could not be verified."

			if effective.Mode == string(domain.AuthModeOIDC) {
				field = "oidc_issuer"
				message = "OIDC provider discovery failed. Check the issuer and server connectivity."
			}

			httpresponse.Problem(w,
				http.StatusUnprocessableEntity,
				"Authentication validation failed.",
				httpresponse.NewFieldProblem(field, message),
			)
			return
		}

		if err := settingsUseCases.SaveAuthenticationSettings(r.Context(), settings, admin.ID); err != nil {
			writeAdminProblem(views.Logger(), w, err, "Authentication settings")
			return
		}

		http.Redirect(w, r, "/admin/configuration", http.StatusSeeOther)
	}
}

// preserveRuntimeManagedAuthenticationSettings keeps persisted values that cannot be changed while deployment overrides are active.
func preserveRuntimeManagedAuthenticationSettings(
	settings, current domain.AuthenticationSettings,
	runtime webview.RuntimeInfo,
) domain.AuthenticationSettings {
	if runtime.AuthModeOverride == "" {
		return settings
	}

	settings.Mode = current.Mode

	switch domain.AuthMode(runtime.AuthModeOverride) {
	case domain.AuthModeOIDC:
		settings.OIDCIssuer = current.OIDCIssuer
		settings.OIDCClientID = current.OIDCClientID
		settings.OIDCGroupClaim = current.OIDCGroupClaim
		settings.OIDCAdminGroup = current.OIDCAdminGroup
	case domain.AuthModeTrustedProxy:
		settings.TrustedUsernameHeaders = current.TrustedUsernameHeaders
		settings.TrustedEmailHeaders = current.TrustedEmailHeaders
		settings.TrustedDisplayNameHeaders = current.TrustedDisplayNameHeaders
		settings.TrustedGroupHeaders = current.TrustedGroupHeaders
		settings.TrustedAdminGroup = current.TrustedAdminGroup
	}

	return settings
}

// effectiveAuthenticationSettings overlays deployment-managed values for validation and runtime behavior.
func effectiveAuthenticationSettings(settings domain.AuthenticationSettings, runtime webview.RuntimeInfo) domain.AuthenticationSettings {
	if runtime.AuthModeOverride == "" {
		return settings
	}

	settings.Mode = runtime.AuthModeOverride

	switch domain.AuthMode(runtime.AuthModeOverride) {
	case domain.AuthModeOIDC:
		settings.OIDCIssuer = runtime.OIDCIssuerOverride
		settings.OIDCClientID = runtime.OIDCClientIDOverride
		settings.OIDCGroupClaim = runtime.OIDCGroupClaimOverride
		settings.OIDCAdminGroup = runtime.OIDCAdminGroupOverride
	case domain.AuthModeTrustedProxy:
		settings.TrustedUsernameHeaders = runtime.TrustedUsernameHeadersOverride
		settings.TrustedEmailHeaders = runtime.TrustedEmailHeadersOverride
		settings.TrustedDisplayNameHeaders = runtime.TrustedDisplayNameHeadersOverride
		settings.TrustedGroupHeaders = runtime.TrustedGroupHeadersOverride
		settings.TrustedAdminGroup = runtime.TrustedAdminGroupOverride
	}

	return settings
}

// authenticationSettingsFromForm parses non-secret browser authentication settings.
func authenticationSettingsFromForm(r *http.Request) domain.AuthenticationSettings {
	mappings := make([]domain.OIDCGroupMapping, 0, len(r.Form["oidc_group_source"]))

	for index, source := range r.Form["oidc_group_source"] {
		source = strings.TrimSpace(source)
		if source == "" || index >= len(r.Form["oidc_group_id"]) {
			continue
		}

		groupID, _ := strconv.ParseInt(strings.TrimSpace(r.Form["oidc_group_id"][index]), 10, 64)
		mappings = append(mappings, domain.OIDCGroupMapping{OIDCGroup: source, GroupID: groupID})
	}

	return domain.AuthenticationSettings{
		Mode:                      strings.TrimSpace(r.FormValue("auth_mode")),
		OIDCIssuer:                strings.TrimSpace(r.FormValue("oidc_issuer")),
		OIDCClientID:              strings.TrimSpace(r.FormValue("oidc_client_id")),
		OIDCGroupClaim:            strings.TrimSpace(r.FormValue("oidc_group_claim")),
		OIDCGroupSync:             r.FormValue("oidc_group_sync") == "on",
		OIDCGroupsAuthoritative:   r.FormValue("oidc_groups_authoritative") == "on",
		OIDCGroupMappings:         mappings,
		OIDCAdminGroup:            strings.TrimSpace(r.FormValue("oidc_admin_group")),
		TrustedUsernameHeaders:    splitHeaderNames(r.FormValue("trusted_username_headers")),
		TrustedEmailHeaders:       splitHeaderNames(r.FormValue("trusted_email_headers")),
		TrustedDisplayNameHeaders: splitHeaderNames(r.FormValue("trusted_display_name_headers")),
		TrustedGroupHeaders:       splitHeaderNames(r.FormValue("trusted_group_headers")),
		TrustedAdminGroup:         strings.TrimSpace(r.FormValue("trusted_admin_group")),
	}
}

// splitHeaderNames normalizes a comma-separated ordered header list.
func splitHeaderNames(value string) []string {
	seen := map[string]bool{}
	headers := make([]string, 0)

	for value := range strings.FieldsFuncSeq(value, func(r rune) bool { return r == ',' || r == '\n' }) {
		header := strings.TrimSpace(value)
		key := strings.ToLower(header)
		if header == "" || seen[key] {
			continue
		}

		seen[key] = true
		headers = append(headers, header)
	}

	return headers
}

// SaveAdminSettings updates mutable application-wide settings.
func SaveAdminSettings(settingsUseCases settingsService, views *webview.Views, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		admin := currentUser(r)
		if err := r.ParseForm(); err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid settings form.")
			return
		}

		settings := applicationSettingsFromForm(r)
		if views.Runtime().UserRegistrationOverrideConfigured {
			current, err := settingsUseCases.ApplicationSettings(r.Context())
			if err != nil {
				httpresponse.InternalServerError(views.Logger(), w, err)
				return
			}

			settings.AllowUserRegistration = current.AllowUserRegistration
		}
		if !isContentLanguage(settings.ContentLanguage) {
			httpresponse.Problem(w,
				http.StatusUnprocessableEntity,
				"Settings validation failed.",
				httpresponse.NewFieldProblem("content_language", "Choose a supported content language."),
			)
			return
		}
		if err := settingsUseCases.SaveApplicationSettings(r.Context(), settings, admin.ID); err != nil {
			writeAdminProblem(logger, w, err, "Settings")
			return
		}

		http.Redirect(w, r, "/admin/configuration", http.StatusSeeOther)
	}
}

// applicationSettingsFromForm parses mutable application-wide settings from a form.
func applicationSettingsFromForm(r *http.Request) domain.ApplicationSettings {
	return domain.ApplicationSettings{
		AllowUserRegistration: r.FormValue("allow_user_registration") == "on",
		DiscussionsEnabled:    r.FormValue("discussions_enabled") == "on",
		ContentLanguage:       strings.TrimSpace(r.FormValue("content_language")),
		ExternalLinks:         externalLinksFromForm(r),
		RobotsPolicy:          strings.TrimSpace(r.FormValue("robots_policy")),
		Rendering: domain.RenderingSettings{
			DefaultTypographySize: strings.TrimSpace(r.FormValue("default_typography_size")),
		},
	}
}

// externalLinksFromForm parses ordered external-link rows from the application settings form.
func externalLinksFromForm(r *http.Request) []domain.ExternalLink {
	labels := r.Form["external_link_label"]
	urls := r.Form["external_link_url"]
	icons := r.Form["external_link_icon"]
	descriptions := r.Form["external_link_description"]
	hoverEffects := r.Form["external_link_hover_effect"]
	hoverTexts := r.Form["external_link_hover_text"]
	count := max(len(labels), len(urls), len(icons), len(descriptions), len(hoverEffects), len(hoverTexts))
	links := make([]domain.ExternalLink, 0, count)

	for index := range count {
		link := domain.ExternalLink{
			Label:       formValueAt(labels, index),
			URL:         formValueAt(urls, index),
			Icon:        formValueAt(icons, index),
			Description: formValueAt(descriptions, index),
			HoverEffect: formValueAt(hoverEffects, index),
			HoverText:   formValueAt(hoverTexts, index),
		}
		if link == (domain.ExternalLink{}) {
			continue
		}

		links = append(links, link)
	}

	return links
}

// formValueAt returns one trimmed repeated-form value when present.
func formValueAt(values []string, index int) string {
	if index >= len(values) {
		return ""
	}

	return strings.TrimSpace(values[index])
}
