package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/kumbuka-me/kumbuka/internal/auth"
	"github.com/kumbuka-me/kumbuka/internal/httpresponse"
	"github.com/kumbuka-me/kumbuka/internal/service"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/icons"
	"golang.org/x/net/http/httpguts"
)

// contentLanguageOption describes one supported application/page language.
type contentLanguageOption struct {
	// Code is the persisted BCP 47 language tag.
	Code string
	// Label is the administrator-facing language name.
	Label string
}

// contentLanguageOptions contains the languages supported by page search and presentation.
var contentLanguageOptions = []contentLanguageOption{
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

// Administration renders the administrator overview.
func Administration(
	viewDataUseCases viewDataService,
	administrationUseCases administrationService,
	views *Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := administrationData(r, viewDataUseCases, views, "Administration", "overview")
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}

		stats, err := administrationUseCases.Stats(r.Context())
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}

		data.AdminStats = stats

		render(views, w, "admin", data)
	}
}

// AdminConfiguration renders runtime and application-wide configuration.
func AdminConfiguration(
	viewDataUseCases viewDataService,
	groupUseCases groupReader,
	userUseCases oidcIdentityService,
	settingsUseCases settingsService,
	views *Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := administrationData(r, viewDataUseCases, views, "Configuration", "configuration")
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}

		groups, err := groupUseCases.Groups(r.Context())
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}

		data.Groups = groups
		data.ApplicationSettings.Authentication.OIDCGroupMappings, err = userUseCases.OIDCGroupMappings(r.Context())
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}

		data.PDFHeaders, err = settingsUseCases.PDFHeaders(r.Context())
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}
		data.ContentLanguages = contentLanguageOptions

		render(views, w, "admin_configuration", data)
	}
}

// isContentLanguage reports whether a configured content language is exposed by the admin UI.
func isContentLanguage(value string) bool {
	return slices.ContainsFunc(contentLanguageOptions, func(option contentLanguageOption) bool {
		return option.Code == value
	})
}

// AdminDocumentationHealth renders actionable wiki documentation-quality findings.
func AdminDocumentationHealth(
	viewDataUseCases viewDataService,
	administrationUseCases administrationService,
	views *Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := administrationData(r, viewDataUseCases, views, "Documentation health", "health")
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}

		health, err := administrationUseCases.DocumentationHealth(r.Context(), time.Now().AddDate(0, -6, 0))
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}

		data.DocumentationHealth = health

		render(views, w, "admin_health", data)
	}
}

// AdminAudit renders recent application audit events.
func AdminAudit(
	viewDataUseCases viewDataService,
	administrationUseCases administrationService,
	views *Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := administrationData(r, viewDataUseCases, views, "Audit log", "audit")
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}

		events, err := administrationUseCases.AuditEvents(r.Context(), 500)
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}

		data.AuditEvents = events

		render(views, w, "admin_audit", data)
	}
}

// AdminUsers renders user roles and group memberships.
func AdminUsers(
	viewDataUseCases viewDataService,
	userUseCases adminUserOverviewService,
	groupUseCases groupReader,
	views *Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := administrationData(r, viewDataUseCases, views, "Users", "users")
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}

		users, err := userUseCases.Users(r.Context())
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}

		groups, err := groupUseCases.Groups(r.Context())
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}

		identities, err := userUseCases.OIDCIdentities(r.Context())
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}

		identitiesByUser := make(map[int64][]domain.OIDCIdentity)

		for _, identity := range identities {
			identitiesByUser[identity.UserID] = append(identitiesByUser[identity.UserID], identity)
		}
		for index := range users {
			users[index].OIDCIdentities = identitiesByUser[users[index].User.ID]
		}

		pendingIdentities, err := userUseCases.PendingOIDCIdentities(r.Context())
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}

		identityCount := len(identities)
		data.AdminUsers = users
		data.Groups = groups
		data.PendingOIDCIdentities = pendingIdentities
		data.OIDCIdentityCount = identityCount

		render(views, w, "admin_users", data)
	}
}

// AdminGroups renders group management.
func AdminGroups(
	viewDataUseCases viewDataService,
	groupUseCases groupReader,
	views *Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := administrationData(r, viewDataUseCases, views, "Groups", "groups")
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}

		groups, err := groupUseCases.Groups(r.Context())
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}

		data.Groups = groups

		render(views, w, "admin_groups", data)
	}
}

// AdminPageTemplates renders reusable page-template management.
func AdminPageTemplates(
	viewDataUseCases viewDataService,
	templateUseCases templateService,
	groupUseCases groupReader,
	views *Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := administrationData(r, viewDataUseCases, views, "Page templates", "templates")
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}

		templates, err := templateUseCases.PageTemplates(r.Context())
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}

		groups, err := groupUseCases.Groups(r.Context())
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}

		data.PageTemplates = templates
		data.Groups = groups
		data.PageStatuses = domain.PageStatuses()

		render(views, w, "admin_templates", data)
	}
}

// CreateAdminPageTemplate creates a reusable Markdown page template.
func CreateAdminPageTemplate(templateUseCases templateService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid template form.")
			return
		}
		if _, err := templateUseCases.CreatePageTemplate(r.Context(), pageTemplateInputFromForm(r)); err != nil {
			writeAdminProblem(logger, w, err, "Page template")
			return
		}

		http.Redirect(w, r, "/admin/templates", http.StatusSeeOther)
	}
}

// UpdateAdminPageTemplate updates one reusable Markdown page template.
func UpdateAdminPageTemplate(templateUseCases templateService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid template identifier.")
			return
		}
		if err := r.ParseForm(); err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid template form.")
			return
		}
		if err := templateUseCases.UpdatePageTemplate(r.Context(), id, pageTemplateInputFromForm(r)); err != nil {
			writeAdminProblem(logger, w, err, "Page template")
			return
		}

		http.Redirect(w, r, "/admin/templates", http.StatusSeeOther)
	}
}

// pageTemplateInputFromForm translates the blueprint form into a service input.
func pageTemplateInputFromForm(r *http.Request) service.PageTemplateInput {
	ownerGroupID, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("owner_group_id")), 10, 64)
	reviewIntervalDays, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("review_interval_days")))

	return service.PageTemplateInput{
		Name:               r.FormValue("name"),
		Description:        r.FormValue("description"),
		Markdown:           r.FormValue("markdown"),
		PathPrefix:         r.FormValue("path_prefix"),
		Icon:               r.FormValue("icon"),
		Tags:               splitTags(r.FormValue("tags")),
		Status:             r.FormValue("status"),
		OwnerGroupID:       ownerGroupID,
		ReviewIntervalDays: reviewIntervalDays,
		Properties:         parseBlueprintProperties(r.FormValue("properties")),
		Fields:             parseBlueprintFields(r.FormValue("fields")),
	}
}

// parseBlueprintProperties parses one key=value blueprint property per line.
func parseBlueprintProperties(value string) map[string]string {
	properties := map[string]string{}

	for line := range strings.SplitSeq(value, "\n") {
		key, content, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)

		if !ok || key == "" {
			continue
		}

		properties[key] = strings.TrimSpace(content)
	}

	return properties
}

// parseBlueprintFields parses the compact blueprint field definition format.
func parseBlueprintFields(value string) []domain.PageTemplateField {
	fields := make([]domain.PageTemplateField, 0)

	for line := range strings.SplitSeq(value, "\n") {
		field, ok := parseBlueprintField(line)
		if !ok {
			continue
		}

		fields = append(fields, field)
	}

	return fields
}

// parseBlueprintField parses one name|label|default|required blueprint field line.
func parseBlueprintField(value string) (domain.PageTemplateField, bool) {
	parts := strings.Split(value, "|")
	name := formValueAt(parts, 0)
	if name == "" {
		return domain.PageTemplateField{}, false
	}

	return domain.PageTemplateField{
		Name:     name,
		Label:    formValueAt(parts, 1),
		Default:  formValueAt(parts, 2),
		Required: strings.EqualFold(formValueAt(parts, 3), "required"),
	}, true
}

// DeleteAdminPageTemplate deletes one reusable page template.
func DeleteAdminPageTemplate(templateUseCases templateService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid template identifier.")
			return
		}
		if err := templateUseCases.DeletePageTemplate(r.Context(), id); err != nil {
			writeAdminProblem(logger, w, err, "Page template")
			return
		}

		http.Redirect(w, r, "/admin/templates", http.StatusSeeOther)
	}
}

// AdminNavigation renders icon configuration for every navigation path.
func AdminNavigation(
	viewDataUseCases viewDataService,
	navigationUseCases navigationService,
	views *Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := administrationData(r, viewDataUseCases, views, "Navigation", "navigation")
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}

		items, err := navigationUseCases.NavigationItems(r.Context())
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}

		data.AdminNavigation = items

		render(views, w, "admin_navigation", data)
	}
}

// SearchIcons serves icon picker results to page editors and administrators.
func SearchIcons(catalog *icons.Catalog) http.HandlerFunc {
	if catalog == nil {
		catalog = icons.Builtin()
	}
	type result struct {
		Name   string `json:"name"`
		Label  string `json:"label"`
		Source string `json:"source"`
		SVG    string `json:"svg"`
	}
	type response struct {
		Items   []result `json:"items"`
		HasMore bool     `json:"has_more"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		offset := 0

		if value := r.URL.Query().Get("offset"); value != "" {
			var err error
			offset, err = strconv.Atoi(value)
			if err != nil || offset < 0 {
				httpresponse.Problem(w, http.StatusBadRequest, "Icon search offset must be a non-negative integer.")
				return
			}
		}

		options, hasMore := catalog.SearchPage(r.URL.Query().Get("q"), offset, 80)
		results := make([]result, 0, len(options))

		for _, option := range options {
			results = append(
				results,
				result{
					Name:   option.Name,
					Label:  option.Label,
					Source: option.Source,
					SVG:    string(catalog.SVG(option.Name, 22)),
				},
			)
		}

		httpresponse.Respond(w, http.StatusOK, response{Items: results, HasMore: hasMore})
	}
}

// SaveAdminNavigationIcon stores the selected icon for one navigation path.
func SaveAdminNavigationIcon(navigationUseCases navigationService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid navigation form.")
			return
		}

		path := strings.TrimSpace(r.FormValue("path"))
		icon := strings.TrimSpace(r.FormValue("icon"))
		if err := navigationUseCases.SetNavigationIcon(r.Context(), path, icon); err != nil {
			writeAdminProblem(logger, w, err, "Navigation path")
			return
		}

		http.Redirect(w, r, "/admin/navigation", http.StatusSeeOther)
	}
}

// AdminTags renders tag management and usage counts.
func AdminTags(
	viewDataUseCases viewDataService,
	administrationUseCases administrationService,
	views *Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := administrationData(r, viewDataUseCases, views, "Tags", "tags")
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}

		tags, err := administrationUseCases.TagInfos(r.Context())
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}

		data.AdminTags = tags

		render(views, w, "admin_tags", data)
	}
}

// AdminTokens renders administrator-managed personal access tokens.
func AdminTokens(
	viewDataUseCases viewDataService,
	userUseCases userManagementService,
	tokenUseCases tokenService,
	views *Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := administrationData(r, viewDataUseCases, views, "Access tokens", "tokens")
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}

		users, err := userUseCases.Users(r.Context())
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}

		tokens, err := tokenUseCases.Tokens(r.Context())
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}

		data.AdminUsers = users
		data.AdminTokens = tokens

		render(views, w, "admin_tokens", data)
	}
}

// AdminExports renders page export controls.
func AdminExports(
	viewDataUseCases viewDataService,
	navigationUseCases navigationService,
	views *Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := administrationData(r, viewDataUseCases, views, "Exports", "exports")
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}

		pages, err := navigationUseCases.NavigationPages(r.Context())
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}

		data.AdminPages = pages

		render(views, w, "admin_exports", data)
	}
}

// AdminImages renders all uploaded images and their reference counts.
func AdminImages(
	viewDataUseCases viewDataService,
	mediaUseCases imageListService,
	views *Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := administrationData(r, viewDataUseCases, views, "Images", "images")
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}

		data.ImageQuery = strings.TrimSpace(r.URL.Query().Get("image_q"))
		images, err := mediaUseCases.SearchImages(
			r.Context(),
			data.ImageQuery,
			managedImagePageSize+1,
			0,
		)
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}

		data.Images, data.ImagesHasMore = managedImageItems(images)

		render(views, w, "admin_images", data)
	}
}

// SaveAdminAuthentication updates database-managed browser authentication settings.
func SaveAdminAuthentication(
	settingsUseCases settingsService,
	browserAuth auth.BrowserAuth,
	views *Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		admin := currentUser(r)
		if err := r.ParseForm(); err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid authentication form.")
			return
		}

		settings := authenticationSettingsFromForm(r)
		if views.runtime.AuthModeOverride != "" {
			current, err := settingsUseCases.ApplicationSettings(r.Context())
			if err != nil {
				httpresponse.InternalServerError(views.logger, w, err)
				return
			}

			settings = preserveRuntimeManagedAuthenticationSettings(settings, current.Authentication, views.runtime)
		}

		effective := effectiveAuthenticationSettings(settings, views.runtime)

		problems := authenticationSettingsProblems(effective, views.runtime)
		if len(problems) > 0 {
			httpresponse.Problem(w, http.StatusUnprocessableEntity, "Authentication validation failed.", problems...)
			return
		}

		if err := browserAuth.Validate(r.Context(), effective); err != nil {
			if tryWriteValidationProblem(w, err, "Authentication validation failed.") {
				return
			}

			views.logger.Warn(
				"authentication settings rejected",
				"event", "authentication_settings_rejected",
				"mode", effective.Mode,
				"error", err,
			)

			field := "auth_mode"
			message := "The authentication configuration could not be verified."

			if effective.Mode == string(auth.AuthModeOIDC) {
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
			writeAdminProblem(views.logger, w, err, "Authentication settings")
			return
		}

		http.Redirect(w, r, "/admin/configuration", http.StatusSeeOther)
	}
}

// preserveRuntimeManagedAuthenticationSettings keeps persisted values that cannot be changed while deployment overrides are active.
func preserveRuntimeManagedAuthenticationSettings(
	settings, current domain.AuthenticationSettings,
	runtime RuntimeInfo,
) domain.AuthenticationSettings {
	if runtime.AuthModeOverride == "" {
		return settings
	}

	settings.Mode = current.Mode

	switch auth.AuthMode(runtime.AuthModeOverride) {
	case auth.AuthModeOIDC:
		settings.OIDCIssuer = current.OIDCIssuer
		settings.OIDCClientID = current.OIDCClientID
	case auth.AuthModeTrustedProxy:
		settings.TrustedUsernameHeaders = current.TrustedUsernameHeaders
		settings.TrustedEmailHeaders = current.TrustedEmailHeaders
		settings.TrustedDisplayNameHeaders = current.TrustedDisplayNameHeaders
	}

	return settings
}

// effectiveAuthenticationSettings overlays deployment-managed values for validation and runtime behavior.
func effectiveAuthenticationSettings(settings domain.AuthenticationSettings, runtime RuntimeInfo) domain.AuthenticationSettings {
	if runtime.AuthModeOverride == "" {
		return settings
	}

	settings.Mode = runtime.AuthModeOverride

	switch auth.AuthMode(runtime.AuthModeOverride) {
	case auth.AuthModeOIDC:
		settings.OIDCIssuer = runtime.OIDCIssuerOverride
		settings.OIDCClientID = runtime.OIDCClientIDOverride
	case auth.AuthModeTrustedProxy:
		settings.TrustedUsernameHeaders = runtime.TrustedUsernameHeadersOverride
		settings.TrustedEmailHeaders = runtime.TrustedEmailHeadersOverride
		settings.TrustedDisplayNameHeaders = runtime.TrustedDisplayNameHeadersOverride
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

// authenticationSettingsProblems returns field-level validation errors for browser authentication settings.
func authenticationSettingsProblems(
	settings domain.AuthenticationSettings,
	runtime RuntimeInfo,
) []httpresponse.FieldProblem {
	var problems []httpresponse.FieldProblem

	switch auth.AuthMode(settings.Mode) {
	case auth.AuthModeNone:
	case auth.AuthModeLocal:
	case auth.AuthModeTrustedProxy:
		if len(settings.TrustedUsernameHeaders) == 0 {
			problems = append(problems, httpresponse.NewFieldProblem(
				"trusted_username_headers",
				"Configure at least one username header.",
			))
		}
		if settings.TrustedAdminGroup != "" && len(settings.TrustedGroupHeaders) == 0 {
			problems = append(problems, httpresponse.NewFieldProblem("trusted_group_headers", "Configure at least one group header for external administrator elevation."))
		}
	case auth.AuthModeOIDC:
		if settings.OIDCIssuer == "" {
			problems = append(problems, httpresponse.NewFieldProblem("oidc_issuer", "OIDC issuer is required."))
		}
		if settings.OIDCClientID == "" {
			problems = append(problems, httpresponse.NewFieldProblem("oidc_client_id", "OIDC client ID is required."))
		}
		if !runtime.OIDCClientSecretConfigured {
			problems = append(problems, httpresponse.NewFieldProblem(
				"oidc_client_secret",
				"Configure KUMBUKA__OIDC_CLIENT_SECRET before enabling OIDC.",
			))
		}
		if !runtime.OIDCSessionSecretConfigured {
			problems = append(problems, httpresponse.NewFieldProblem(
				"oidc_session_secret",
				"Configure KUMBUKA__OIDC_SESSION_SECRET with at least 32 characters before enabling OIDC.",
			))
		}
		usesOIDCGroups := settings.OIDCGroupSync || settings.OIDCAdminGroup != ""
		if usesOIDCGroups && settings.OIDCGroupClaim == "" {
			problems = append(problems, httpresponse.NewFieldProblem(
				"oidc_group_claim",
				"Configure the OIDC claim containing group memberships.",
			))
		}

		seenMappings := map[string]bool{}

		for _, mapping := range settings.OIDCGroupMappings {
			if mapping.OIDCGroup == "" || mapping.GroupID <= 0 {
				problems = append(problems, httpresponse.NewFieldProblem(
					"oidc_group_mapping",
					"Choose a Kumbuka group for every OIDC group mapping.",
				))
				break
			}
			if seenMappings[mapping.OIDCGroup] {
				problems = append(problems, httpresponse.NewFieldProblem(
					"oidc_group_mapping",
					"Each OIDC group may only be mapped once.",
				))
				break
			}

			seenMappings[mapping.OIDCGroup] = true
		}
	default:
		problems = append(problems, httpresponse.NewFieldProblem("auth_mode", "Choose a supported authentication mode."))
	}

	for field, headers := range map[string][]string{
		"trusted_username_headers":     settings.TrustedUsernameHeaders,
		"trusted_email_headers":        settings.TrustedEmailHeaders,
		"trusted_display_name_headers": settings.TrustedDisplayNameHeaders,
		"trusted_group_headers":        settings.TrustedGroupHeaders,
	} {
		for _, header := range headers {
			if !httpguts.ValidHeaderFieldName(header) {
				problems = append(problems, httpresponse.NewFieldProblem(field, "Use valid HTTP header names separated by commas."))
				break
			}
		}
	}

	return problems
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
func SaveAdminSettings(settingsUseCases settingsService, views *Views, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		admin := currentUser(r)
		if err := r.ParseForm(); err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid settings form.")
			return
		}

		settings := applicationSettingsFromForm(r)
		if views.runtime.UserRegistrationOverrideConfigured {
			current, err := settingsUseCases.ApplicationSettings(r.Context())
			if err != nil {
				httpresponse.InternalServerError(views.logger, w, err)
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

// UpdateAdminUser updates one user's role, group memberships, and optional recovery login state.
func UpdateAdminUser(
	userUseCases userAccountWriter,
	views *Views,
	logger *slog.Logger,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		admin := currentUser(r)
		userID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || userID <= 0 {
			httpresponse.Problem(w,
				http.StatusBadRequest,
				"User validation failed.",
				httpresponse.NewFieldProblem("user_id", "Choose a valid user."),
			)
			return
		}
		if err := r.ParseForm(); err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid user form.")
			return
		}

		role := r.FormValue("role")
		enabled := r.FormValue("account_enabled") == "on"

		groupIDs := make([]int64, 0, len(r.Form["group_id"]))

		for _, value := range r.Form["group_id"] {
			groupID, err := strconv.ParseInt(value, 10, 64)
			if err != nil || groupID <= 0 {
				httpresponse.Problem(w,
					http.StatusBadRequest,
					"Group validation failed.",
					httpresponse.NewFieldProblem("group_id", "Choose a valid group."),
				)
				return
			}

			groupIDs = append(groupIDs, groupID)
		}

		password := r.FormValue("local_password")
		updateLocalCredential := r.FormValue("update_local_credential") == "true"
		if problems := localPasswordValidationProblems(
			password,
			r.FormValue("local_password_confirm"),
			"local_password",
			"local_password_confirm",
			false,
		); len(problems) > 0 {
			httpresponse.Problem(w, http.StatusUnprocessableEntity, "Local login validation failed.", problems...)
			return
		}

		if err := userUseCases.UpdateAccount(r.Context(), service.UserUpdateInput{
			UserID: userID, Actor: admin, Role: role, Enabled: enabled, GroupIDs: groupIDs,
			Password: password, UpdateLocalCredential: updateLocalCredential,
			LocalCredentialEnabled: r.FormValue("local_credential_enabled") == "on",
			AuthModeOverride:       views.runtime.AuthModeOverride,
		}); err != nil {
			writeAdminProblem(logger, w, err, "User")
			return
		}

		http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
	}
}

// RevokeAdminUserSessions signs an account out of local and OIDC browser sessions.
func RevokeAdminUserSessions(userUseCases userManagementService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		admin := currentUser(r)
		userID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || userID <= 0 {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid user.")
			return
		}
		if err := userUseCases.RevokeUserSessions(r.Context(), userID, admin.ID); err != nil {
			writeAdminProblem(logger, w, err, "User sessions")
			return
		}

		http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
	}
}

// ApprovePendingOIDCIdentity creates a Kumbuka account for one verified identity request.
func ApprovePendingOIDCIdentity(userUseCases oidcIdentityService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		admin := currentUser(r)
		pendingID, err := pendingOIDCIdentityID(r)
		if err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid identity request.")
			return
		}

		_, err = userUseCases.ApprovePendingOIDCIdentity(r.Context(), pendingID, admin.ID)
		if err != nil {
			writeAdminProblem(logger, w, err, "Identity request")
			return
		}

		http.Redirect(w, r, "/admin/users#pending-identities", http.StatusSeeOther)
	}
}

// LinkPendingOIDCIdentity replaces an existing user's issuer binding with a verified identity request.
func LinkPendingOIDCIdentity(userUseCases oidcIdentityService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		admin := currentUser(r)
		pendingID, err := pendingOIDCIdentityID(r)
		if err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid identity request.")
			return
		}
		if err := r.ParseForm(); err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid identity link form.")
			return
		}

		userID, err := strconv.ParseInt(r.FormValue("user_id"), 10, 64)
		if err != nil || userID <= 0 {
			httpresponse.Problem(w,
				http.StatusUnprocessableEntity,
				"Identity link validation failed.",
				httpresponse.NewFieldProblem("user_id", "Choose an existing Kumbuka user."),
			)
			return
		}

		_, err = userUseCases.LinkPendingOIDCIdentity(r.Context(), pendingID, userID, admin.ID)
		if err != nil {
			writeAdminProblem(logger, w, err, "Identity request")
			return
		}

		http.Redirect(w, r, "/admin/users#pending-identities", http.StatusSeeOther)
	}
}

// RejectPendingOIDCIdentity blocks one verified identity request until an administrator reopens it.
func RejectPendingOIDCIdentity(userUseCases oidcIdentityService, logger *slog.Logger) http.HandlerFunc {
	return pendingOIDCIdentityStatusHandler(userUseCases, logger, true)
}

// ReopenPendingOIDCIdentity returns a rejected identity request to the pending queue.
func ReopenPendingOIDCIdentity(userUseCases oidcIdentityService, logger *slog.Logger) http.HandlerFunc {
	return pendingOIDCIdentityStatusHandler(userUseCases, logger, false)
}

// pendingOIDCIdentityStatusHandler updates one administrator decision.
func pendingOIDCIdentityStatusHandler(
	userUseCases oidcIdentityService,
	logger *slog.Logger,
	rejected bool,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		admin := currentUser(r)
		pendingID, err := pendingOIDCIdentityID(r)
		if err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid identity request.")
			return
		}
		if err := userUseCases.SetPendingOIDCIdentityRejected(
			r.Context(),
			pendingID,
			rejected,
			admin.ID,
		); err != nil {
			writeAdminProblem(logger, w, err, "Identity request")
			return
		}

		http.Redirect(w, r, "/admin/users#pending-identities", http.StatusSeeOther)
	}
}

// pendingOIDCIdentityID parses the pending identity identifier from the route.
func pendingOIDCIdentityID(r *http.Request) (pendingID int64, err error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid pending OIDC identity")
	}

	return id, nil
}

// RemoveAdminOIDCIdentity disconnects one active OIDC binding from a Kumbuka account.
func RemoveAdminOIDCIdentity(userUseCases oidcIdentityService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		admin := currentUser(r)
		userID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || userID <= 0 {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid user identifier.")
			return
		}
		if err := r.ParseForm(); err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid identity form.")
			return
		}

		issuer := strings.TrimSpace(r.FormValue("issuer"))
		subject := strings.TrimSpace(r.FormValue("subject"))
		if issuer == "" || subject == "" {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid OIDC identity.")
			return
		}
		if err := userUseCases.RemoveOIDCIdentity(
			r.Context(),
			userID,
			issuer,
			subject,
			admin.ID,
		); err != nil {
			writeAdminProblem(logger, w, err, "OIDC identity")
			return
		}

		http.Redirect(w, r, "/admin/users#oidc-identities", http.StatusSeeOther)
	}
}

// CreateAdminGroup creates a new user group.
func CreateAdminGroup(groupUseCases groupWriter, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid group form.")
			return
		}
		if _, err := groupUseCases.CreateGroup(r.Context(), r.FormValue("name")); err != nil {
			writeAdminProblem(logger, w, err, "Group")
			return
		}

		http.Redirect(w, r, "/admin/groups", http.StatusSeeOther)
	}
}

// DeleteAdminGroup deletes one user group.
func DeleteAdminGroup(groupUseCases groupWriter, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			httpresponse.Problem(w,
				http.StatusBadRequest,
				"Group validation failed.",
				httpresponse.NewFieldProblem("group_id", "Choose a valid group."),
			)
			return
		}
		if err := groupUseCases.DeleteGroup(r.Context(), id); err != nil {
			writeAdminProblem(logger, w, err, "Group")
			return
		}

		http.Redirect(w, r, "/admin/groups", http.StatusSeeOther)
	}
}

// DeleteAdminTag removes a tag and all page associations for it.
func DeleteAdminTag(administrationUseCases administrationService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid tag identifier.")
			return
		}
		if err := administrationUseCases.DeleteTag(r.Context(), id); err != nil {
			writeAdminProblem(logger, w, err, "Tag")
			return
		}

		http.Redirect(w, r, "/admin/tags", http.StatusSeeOther)
	}
}

// administrationData builds common view data for administrator-only pages.
func administrationData(
	r *http.Request,
	viewDataUseCases viewDataService,
	views *Views,
	title, section string,
) (ViewData, error) {
	data, err := viewDataUseCases.Load(r, views, title)
	if err != nil {
		return ViewData{}, err
	}

	data.AdminSection = section
	data.Navigation = nil

	return data, nil
}

// hasGroup reports whether a group name appears in a user's group list.
func hasGroup(groups []string, name string) bool {
	return slices.ContainsFunc(groups, func(group string) bool {
		return strings.EqualFold(group, name)
	})
}

// hasGroupID reports whether a group identifier appears in a page group list.
func hasGroupID(groups []domain.Group, id int64) bool {
	return slices.ContainsFunc(groups, func(group domain.Group) bool {
		return group.ID == id
	})
}

// writeAdminProblem translates expected administration errors into HTTP problems.
func writeAdminProblem(logger *slog.Logger, w http.ResponseWriter, err error, object string) {
	if tryWriteValidationProblem(w, err, object+" validation failed.") {
		return
	}
	switch {
	case errors.Is(err, domain.ErrNotFound):
		httpresponse.Problem(w, http.StatusNotFound, object+" not found.")
	case errors.Is(err, domain.ErrAlreadyExists):
		httpresponse.Problem(w, http.StatusConflict, object+" already exists.")
	case errors.Is(err, domain.ErrForbidden):
		httpresponse.Problem(w, http.StatusForbidden, object+" operation is not permitted.")
	default:
		httpresponse.InternalServerError(logger, w, err)
	}
}

// AdminBin renders pages that have been moved to the recycle bin.
func AdminBin(
	viewDataUseCases viewDataService,
	recycleBinUseCases recycleBinService,
	views *Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := administrationData(r, viewDataUseCases, views, "Recycle bin", "bin")
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}

		pages, err := recycleBinUseCases.DeletedPages(r.Context())
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}

		data.DeletedPages = pages

		render(views, w, "admin_bin", data)
	}
}

// RestoreAdminPage restores one page from the recycle bin.
func RestoreAdminPage(recycleBinUseCases recycleBinService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := strings.TrimSpace(r.PathValue("slug"))
		if slug == "" {
			httpresponse.Problem(w,
				http.StatusBadRequest,
				"A page path is required.",
				httpresponse.NewFieldProblem("slug", "Choose a page to restore."),
			)
			return
		}
		if err := recycleBinUseCases.RestorePage(r.Context(), slug); err != nil {
			writeAdminProblem(logger, w, err, "Page")
			return
		}

		http.Redirect(w, r, "/admin/bin", http.StatusSeeOther)
	}
}

// PermanentlyDeleteAdminPage removes one page from the recycle bin permanently.
func PermanentlyDeleteAdminPage(recycleBinUseCases recycleBinService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := strings.TrimSpace(r.PathValue("slug"))
		if slug == "" {
			httpresponse.Problem(w,
				http.StatusBadRequest,
				"A page path is required.",
				httpresponse.NewFieldProblem("slug", "Choose a page to delete permanently."),
			)
			return
		}
		if err := recycleBinUseCases.PermanentlyDeletePage(r.Context(), slug); err != nil {
			writeAdminProblem(logger, w, err, "Page")
			return
		}

		http.Redirect(w, r, "/admin/bin", http.StatusSeeOther)
	}
}

// SearchAdminUsers returns user matches for live administrator pickers.
func SearchAdminUsers(userUseCases userDirectoryService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := strings.TrimSpace(r.URL.Query().Get("q"))
		if len([]rune(query)) < 2 {
			httpresponse.Respond(w, http.StatusOK, []domain.User{})
			return
		}

		users, err := userUseCases.SearchUsers(r.Context(), query, 20)
		if err != nil {
			httpresponse.InternalServerError(logger, w, err)
			return
		}

		httpresponse.Respond(w, http.StatusOK, jsonSlice(users))
	}
}

// AdminGroupMembers returns the current members of one group.
func AdminGroupMembers(groupUseCases groupReader, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		groupID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || groupID <= 0 {
			httpresponse.Problem(w,
				http.StatusBadRequest,
				"Invalid group identifier.",
				httpresponse.NewFieldProblem("group_id", "Choose a valid group."),
			)
			return
		}

		members, err := groupUseCases.GroupMembers(r.Context(), groupID)
		if err != nil {
			httpresponse.InternalServerError(logger, w, err)
			return
		}

		httpresponse.Respond(w, http.StatusOK, jsonSlice(members))
	}
}

// groupMemberRequest contains the request payload for group member request.
type groupMemberRequest struct {
	// UserID identifies the user associated with group member request.
	UserID int64 `json:"user_id"`
}

// AddAdminGroupMember assigns one user to a group.
func AddAdminGroupMember(
	groupUseCases groupWriter,
	userUseCases userDirectoryService,
	logger *slog.Logger,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		groupID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || groupID <= 0 {
			httpresponse.Problem(w,
				http.StatusBadRequest,
				"Invalid group identifier.",
				httpresponse.NewFieldProblem("group_id", "Choose a valid group."),
			)
			return
		}

		request, err := decode[groupMemberRequest](w, r)
		if err != nil {
			if tryWriteRequestProblem(w, http.StatusBadRequest, "Invalid member request.", "request", err) {
				return
			}

			httpresponse.InternalServerError(logger, w, err)
			return
		}
		if request.UserID <= 0 {
			httpresponse.Problem(w,
				http.StatusUnprocessableEntity,
				"A user is required.",
				httpresponse.NewFieldProblem("user_id", "Choose a person from the suggestions."),
			)
			return
		}
		if err := groupUseCases.AddGroupMember(r.Context(), groupID, request.UserID); err != nil {
			writeAdminProblem(logger, w, err, "Group or user")
			return
		}

		user, err := userUseCases.User(r.Context(), request.UserID)
		if err != nil {
			writeAdminProblem(logger, w, err, "User")
			return
		}

		httpresponse.Respond(w, http.StatusCreated, user)
	}
}

// RemoveAdminGroupMember removes one user from a group.
func RemoveAdminGroupMember(groupUseCases groupWriter, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		groupID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || groupID <= 0 {
			httpresponse.Problem(w,
				http.StatusBadRequest,
				"Invalid group identifier.",
				httpresponse.NewFieldProblem("group_id", "Choose a valid group."),
			)
			return
		}

		userID, err := strconv.ParseInt(r.PathValue("userID"), 10, 64)
		if err != nil || userID <= 0 {
			httpresponse.Problem(w,
				http.StatusBadRequest,
				"Invalid user identifier.",
				httpresponse.NewFieldProblem("user_id", "Choose a valid person."),
			)
			return
		}
		if err := groupUseCases.RemoveGroupMember(r.Context(), groupID, userID); err != nil {
			writeAdminProblem(logger, w, err, "Group membership")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// AdminPageAccess renders inherited page-path access rules.
func AdminPageAccess(
	viewDataUseCases viewDataService,
	accessUseCases pageAccessAdmin,
	groupUseCases groupReader,
	views *Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := administrationData(r, viewDataUseCases, views, "Page access", "permissions")
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}
		data.PageAccessRules, err = accessUseCases.PageAccessRules(r.Context())
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}
		data.Groups, err = groupUseCases.Groups(r.Context())
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}
		render(views, w, "admin_permissions", data)
	}
}

// SaveAdminPageAccess creates or replaces one inherited path rule.
func SaveAdminPageAccess(accessUseCases pageAccessAdmin, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid access form.")
			return
		}
		groupID, err := strconv.ParseInt(r.FormValue("group_id"), 10, 64)
		if err != nil {
			httpresponse.Problem(w, http.StatusUnprocessableEntity, "Page access validation failed.", httpresponse.NewFieldProblem("group_id", "Choose a group."))
			return
		}
		if err := accessUseCases.SavePageAccessRule(r.Context(), r.FormValue("path"), groupID, r.FormValue("access")); err != nil {
			writeAdminProblem(logger, w, err, "Page access rule")
			return
		}
		http.Redirect(w, r, "/admin/permissions", http.StatusSeeOther)
	}
}

// DeleteAdminPageAccess removes one inherited path rule.
func DeleteAdminPageAccess(accessUseCases pageAccessAdmin, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid access rule identifier.")
			return
		}
		if err := accessUseCases.DeletePageAccessRule(r.Context(), id); err != nil {
			writeAdminProblem(logger, w, err, "Page access rule")
			return
		}
		http.Redirect(w, r, "/admin/permissions", http.StatusSeeOther)
	}
}

// AdminWebhooks renders outgoing webhook configuration and recent deliveries.
func AdminWebhooks(viewDataUseCases viewDataService, webhookUseCases webhookAdminService, views *Views) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := administrationData(r, viewDataUseCases, views, "Webhooks", "webhooks")
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}
		data.Webhooks, err = webhookUseCases.Webhooks(r.Context())
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}
		data.WebhookDeliveries, err = webhookUseCases.WebhookDeliveries(r.Context(), 50)
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}
		data.WebhookEvents = service.WebhookEvents()
		data.WebhookDraft = service.DefaultWebhook()
		render(views, w, "admin_webhooks", data)
	}
}

// SaveAdminWebhook creates or updates one outgoing webhook.
func SaveAdminWebhook(webhookUseCases webhookAdminService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid webhook form.")
			return
		}

		id, err := optionalPositivePathID(r.PathValue("id"))
		if err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid webhook identifier.")
			return
		}

		headers, err := webhookHeadersFromForm(r)
		if err != nil {
			if tryWriteRequestProblem(w, http.StatusBadRequest, "Invalid webhook form.", "headers", err) {
				return
			}
			httpresponse.InternalServerError(logger, w, err)
			return
		}

		retryCount, retryBackoff, retryMaxBackoff, err := webhookRetryFromForm(r)
		if err != nil {
			if tryWriteRequestProblem(w, http.StatusBadRequest, "Invalid webhook form.", "retry", err) {
				return
			}
			httpresponse.InternalServerError(logger, w, err)
			return
		}

		_, err = webhookUseCases.SaveWebhook(r.Context(), id, service.WebhookInput{
			Name:            r.FormValue("name"),
			URL:             r.FormValue("url"),
			Events:          r.Form["event"],
			BodyTemplate:    r.FormValue("body_template"),
			Headers:         headers,
			RetryEnabled:    r.FormValue("retry_enabled") == "on",
			RetryCount:      retryCount,
			RetryBackoff:    retryBackoff,
			RetryMaxBackoff: retryMaxBackoff,
			RetryJitter:     r.FormValue("retry_jitter") == "on",
			Enabled:         r.FormValue("enabled") == "on",
		})
		if err != nil {
			writeAdminProblem(logger, w, err, "Webhook")
			return
		}
		http.Redirect(w, r, "/admin/webhooks", http.StatusSeeOther)
	}
}

// optionalPositivePathID parses an optional positive identifier from a route value.
func optionalPositivePathID(raw string) (int64, error) {
	if raw == "" {
		return 0, nil
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("identifier must be a positive integer")
	}
	return id, nil
}

// webhookRetryFromForm parses persisted duration-based retry settings.
func webhookRetryFromForm(r *http.Request) (int, time.Duration, time.Duration, error) {
	retryCount, err := strconv.Atoi(strings.TrimSpace(r.FormValue("retry_count")))
	if err != nil {
		return 0, 0, 0, newRequestError("retry_count", "Retries must be a number.", err)
	}
	retryBackoff, err := time.ParseDuration(strings.TrimSpace(r.FormValue("retry_backoff")))
	if err != nil {
		return 0, 0, 0, newRequestError("retry_backoff", "Initial backoff must be a duration such as 1s.", err)
	}
	retryMaxBackoff, err := time.ParseDuration(strings.TrimSpace(r.FormValue("retry_max_backoff")))
	if err != nil {
		return 0, 0, 0, newRequestError("retry_max_backoff", "Maximum backoff must be a duration such as 30s.", err)
	}

	return retryCount, retryBackoff, retryMaxBackoff, nil
}

// webhookHeadersFromForm parses dynamic webhook request-header rows.
func webhookHeadersFromForm(r *http.Request) ([]service.WebhookHeaderInput, error) {
	rows := r.Form["webhook_header_row"]
	if len(rows) == 0 {
		return nil, nil
	}

	seen := make(map[string]struct{}, len(rows))
	headers := make([]service.WebhookHeaderInput, 0, len(rows))
	for _, row := range rows {
		if !validWebhookHeaderRow(row) {
			return nil, newRequestError("headers", "The webhook header form is invalid.", nil)
		}
		if _, exists := seen[row]; exists {
			return nil, newRequestError("headers", "The webhook header form contains a duplicate row.", nil)
		}
		seen[row] = struct{}{}

		prefix := "webhook_header_" + row + "_"
		id := int64(0)
		if rawID := strings.TrimSpace(r.FormValue(prefix + "id")); rawID != "" {
			parsed, err := strconv.ParseInt(rawID, 10, 64)
			if err != nil || parsed <= 0 {
				return nil, newRequestError("headers", "The webhook header form is invalid.", err)
			}
			id = parsed
		}

		headers = append(headers, service.WebhookHeaderInput{
			ID:        id,
			Name:      r.FormValue(prefix + "name"),
			Value:     r.FormValue(prefix + "value"),
			Sensitive: r.FormValue(prefix+"sensitive") == "on",
		})
	}

	return headers, nil
}

// validWebhookHeaderRow restricts dynamic form keys to a small identifier alphabet.
func validWebhookHeaderRow(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, character := range value {
		if character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' ||
			character == '-' || character == '_' {
			continue
		}
		return false
	}
	return true
}

// DeleteAdminWebhook removes one configured outgoing webhook.
func DeleteAdminWebhook(webhookUseCases webhookAdminService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid webhook identifier.")
			return
		}
		if err := webhookUseCases.DeleteWebhook(r.Context(), id); err != nil {
			writeAdminProblem(logger, w, err, "Webhook")
			return
		}
		http.Redirect(w, r, "/admin/webhooks", http.StatusSeeOther)
	}
}

// TestAdminWebhook sends a diagnostic delivery to one configured webhook.
func TestAdminWebhook(webhookUseCases webhookAdminService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid webhook identifier.")
			return
		}
		if err := webhookUseCases.TestWebhook(r.Context(), id); err != nil {
			httpresponse.InternalServerError(logger, w, err)
			return
		}
		http.Redirect(w, r, "/admin/webhooks", http.StatusSeeOther)
	}
}

// RevealAdminWebhookHeader decrypts one sensitive webhook header after an explicit administrator action.
func RevealAdminWebhookHeader(webhookUseCases webhookAdminService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, no-store")
		webhookID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || webhookID <= 0 {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid webhook identifier.")
			return
		}
		headerID, err := strconv.ParseInt(r.PathValue("headerID"), 10, 64)
		if err != nil || headerID <= 0 {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid webhook header identifier.")
			return
		}

		value, err := webhookUseCases.RevealWebhookHeader(r.Context(), webhookID, headerID)
		if err != nil {
			writeAdminProblem(logger, w, err, "Webhook header")
			return
		}
		httpresponse.Respond(w, http.StatusOK, map[string]string{"value": value})
	}
}
