package webview

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/icons"
	"github.com/kumbuka-me/kumbuka/pkg/themes"
)

const brandLogoHTML = template.HTML(`<img class="kumbuka-logo" src="/brand/logo" alt="" />`)

var sharedTemplateFiles = []string{
	"templates/layout.gohtml",
	"templates/public_layout.gohtml",
	"templates/header.gohtml",
	"templates/sidebar.gohtml",
	"templates/admin_sidebar.gohtml",
	"templates/account_menu.gohtml",
	"templates/dialogs.gohtml",
	"templates/navigation.gohtml",
	"templates/lists.gohtml",
	"templates/page_contents.gohtml",
	"templates/revisions.gohtml",
}

var pageTemplateNames = []string{
	"login",
	"setup",
	"home",
	"page",
	"review",
	"not_found",
	"edit",
	"search",
	"settings",
	"admin",
	"admin_configuration",
	"admin_branding",
	"admin_plugins",
	"admin_plugin_settings",
	"admin_health",
	"admin_templates",
	"admin_permissions",
	"admin_webhooks",
	"admin_audit",
	"admin_pages",
	"admin_import",
	"graph",
	"admin_users",
	"admin_groups",
	"admin_tags",
	"admin_tokens",
	"admin_exports",
	"admin_images",
	"admin_attachments",
	"admin_navigation",
	"admin_bin",
}

// ManagedConfigurationItem contains one deployment-owned configuration value safe to show to administrators.
type ManagedConfigurationItem struct {
	// Name is the human-readable configuration label.
	Name string
	// Value is the redacted or administrator-safe effective value.
	Value string
	// Source identifies the effective deployment source and setting name.
	Source string
}

// ManagedConfigurationGroup groups related deployment-owned configuration values.
type ManagedConfigurationGroup struct {
	// Name is the human-readable group heading.
	Name string
	// Items contains the deployment-owned settings in this group.
	Items []ManagedConfigurationItem
}

// RuntimeInfo contains non-secret runtime configuration safe to show to administrators.
type RuntimeInfo struct {
	// ListenAddress is the configured HTTP listen address.
	ListenAddress string
	// PublicURL is the externally visible Kumbuka URL.
	PublicURL string
	// PDFURL is the optional deployment-level PDF endpoint override.
	PDFURL string
	// ReadOnly reports whether deployment configuration blocks state-changing application requests.
	ReadOnly bool
	// UserRegistrationOverrideConfigured reports whether registration is managed by deployment configuration.
	UserRegistrationOverrideConfigured bool
	// AllowUserRegistrationOverride is the deployment-managed registration value when configured.
	AllowUserRegistrationOverride bool
	// AuthModeOverride is the optional deployment-level recovery override.
	AuthModeOverride string
	// OIDCIssuerOverride is the deployment-managed issuer used by the OIDC runtime override.
	OIDCIssuerOverride string
	// OIDCClientIDOverride is the deployment-managed client ID used by the OIDC runtime override.
	OIDCClientIDOverride string
	// TrustedUsernameHeadersOverride contains deployment-managed username headers for the trusted-proxy runtime override.
	TrustedUsernameHeadersOverride []string
	// TrustedEmailHeadersOverride contains deployment-managed email headers for the trusted-proxy runtime override.
	TrustedEmailHeadersOverride []string
	// TrustedDisplayNameHeadersOverride contains deployment-managed display-name headers for the trusted-proxy runtime override.
	TrustedDisplayNameHeadersOverride []string
	// TrustedGroupHeadersOverride contains deployment-managed group headers for the trusted-proxy runtime override.
	TrustedGroupHeadersOverride []string
	// TrustedAdminGroupOverride is the deployment-managed administrator group for the trusted-proxy runtime override.
	TrustedAdminGroupOverride string
	// OIDCGroupClaimOverride is the deployment-managed group claim used by the OIDC runtime override.
	OIDCGroupClaimOverride string
	// OIDCAdminGroupOverride is the deployment-managed administrator group used by the OIDC runtime override.
	OIDCAdminGroupOverride string
	// OIDCClientSecretConfigured reports whether the OIDC client secret is available.
	OIDCClientSecretConfigured bool
	// OIDCSessionSecretConfigured reports whether a valid OIDC session secret is available.
	OIDCSessionSecretConfigured bool
	// EncryptionKeyConfigured reports whether sensitive persisted settings can be encrypted.
	EncryptionKeyConfigured bool
	// LocalLoginEnabled reports whether the deployment exposes break-glass local login.
	LocalLoginEnabled bool
	// ThemeDirectory is the optional external theme directory.
	ThemeDirectory string
	// PluginUpdateCheckInterval is the deployment-configured catalog refresh interval or Disabled.
	PluginUpdateCheckInterval string
	// ManagedConfiguration groups deployment-owned configuration with safe effective values and sources.
	ManagedConfiguration []ManagedConfigurationGroup
}

// RenderErrorHandler maps a template-rendering failure onto an HTTP response.
// The HTTP adapter installs the production handler at composition time so webview
// does not depend on transport packages above it.
type RenderErrorHandler func(*slog.Logger, http.ResponseWriter, error)

// Views contains the shared server-rendered HTML dependencies.
type Views struct {
	// templates maps page names to parsed template sets with the shared layout and partials.
	templates map[string]*template.Template
	// logger records template rendering failures.
	logger *slog.Logger
	// pageTimingLogger records opt-in page handler timing diagnostics.
	pageTimingLogger *slog.Logger
	// version is the application version exposed in rendered pages.
	version string
	// commit is the application commit exposed for diagnostics.
	commit string
	// themes contains the themes available to the browser.
	themes []themes.Theme
	// runtime contains non-secret runtime configuration shown to administrators.
	runtime RuntimeInfo
	// assetVersion fingerprints embedded browser assets for cache-safe URLs.
	assetVersion string
	// iconCatalog combines built-in icons with resources from enabled plugins.
	iconCatalog *icons.Catalog
	// renderError maps template failures to transport responses without importing the HTTP adapter.
	renderError RenderErrorHandler
}

// New parses each page template with the shared layout and partials once at startup.
func New(
	appFS fs.FS,
	logger *slog.Logger,
	version, commit string,
	availableThemes []themes.Theme,
	runtime RuntimeInfo,
	iconCatalog ...*icons.Catalog,
) (*Views, error) {
	catalog := icons.Builtin()
	if len(iconCatalog) > 0 && iconCatalog[0] != nil {
		catalog = iconCatalog[0]
	}
	assetVersion, err := fingerprintAssets(appFS)
	if err != nil {
		return nil, fmt.Errorf("fingerprint web assets: %w", err)
	}

	funcs := template.FuncMap{
		"join":               strings.Join,
		"timeago":            timeAgo,
		"filesize":           fileSize,
		"hasgroup":           hasGroup,
		"hasgroupid":         hasGroupID,
		"hasstring":          slices.Contains[[]string, string],
		"templateproperties": templatePropertiesText,
		"templatefields":     templateFieldsText,
		"blueprintcontext":   pageTemplateContext,
		"blankblueprint":     blankPageTemplate,
		"webhookcontext":     webhookContext,
		"externalhover":      domain.ExternalLinkHoverTitle,
		"externalhovereffect": func(link domain.ExternalLink) string {
			return domain.EffectiveExternalLinkHoverEffect(link.HoverEffect)
		},
		"icon": catalog.SVG,
		"logo": func() template.HTML {
			return brandLogoHTML
		},
	}
	templates := make(map[string]*template.Template, len(pageTemplateNames))

	for _, name := range pageTemplateNames {
		files := make([]string, 0, len(sharedTemplateFiles)+1)
		files = append(files, sharedTemplateFiles...)
		files = append(files, "templates/"+name+".gohtml")

		parsed, err := template.New(name).Funcs(funcs).ParseFS(appFS, files...)
		if err != nil {
			return nil, fmt.Errorf("parse %s template: %w", name, err)
		}

		templates[name] = parsed
	}

	return &Views{
		templates:    templates,
		logger:       logger,
		version:      version,
		commit:       commit,
		themes:       availableThemes,
		runtime:      runtime,
		assetVersion: assetVersion,
		iconCatalog:  catalog,
	}, nil
}

// WithRenderErrorHandler installs transport-specific handling for template failures.
func (v *Views) WithRenderErrorHandler(handler RenderErrorHandler) *Views {
	v.renderError = handler
	return v
}

// IconCatalog returns the catalog used by this view set and icon picker.
func (v *Views) IconCatalog() *icons.Catalog { return v.iconCatalog }

// Logger returns the logger associated with server-rendered responses.
func (v *Views) Logger() *slog.Logger { return v.logger }

// Version returns the application version exposed to templates.
func (v *Views) Version() string { return v.version }

// Commit returns the application commit exposed to templates.
func (v *Views) Commit() string { return v.commit }

// Themes returns the themes available to browser views.
func (v *Views) Themes() []themes.Theme { return v.themes }

// Runtime returns non-secret runtime configuration exposed to administrators.
func (v *Views) Runtime() RuntimeInfo { return v.runtime }

// AssetVersion returns the fingerprint used for cache-safe embedded assets.
func (v *Views) AssetVersion() string { return v.assetVersion }

// Render executes a page layout into a buffer before writing the HTTP response.
func (v *Views) Render(w http.ResponseWriter, page string, data Screen) {
	v.RenderDataStatus(w, http.StatusOK, page, "layout", data)
}

// RenderStatus executes a page layout with an explicit HTTP status.
func (v *Views) RenderStatus(w http.ResponseWriter, status int, page string, data Screen) {
	v.RenderDataStatus(w, status, page, "layout", data)
}

// RenderPublic executes the minimal unauthenticated page layout.
func (v *Views) RenderPublic(w http.ResponseWriter, page string, data Screen) {
	v.RenderPublicStatus(w, http.StatusOK, page, data)
}

// RenderPublicStatus executes the minimal unauthenticated page layout with an explicit HTTP status.
func (v *Views) RenderPublicStatus(w http.ResponseWriter, status int, page string, data Screen) {
	v.RenderDataStatus(w, status, page, "public-layout", data)
}

// RenderFragment executes one named fragment from a parsed page template set.
func (v *Views) RenderFragment(w http.ResponseWriter, page, name string, data Screen) {
	v.RenderDataStatus(w, http.StatusOK, page, name, data)
}

// RenderDataStatus executes a named template with a typed view model and an explicit HTTP status.
func (v *Views) RenderDataStatus(w http.ResponseWriter, status int, page, name string, data Screen) {
	pageTemplate, ok := v.templates[page]
	if !ok {
		v.handleRenderError(w, page, name, fmt.Errorf("page template %q not found", page))
		return
	}

	var output bytes.Buffer
	if err := pageTemplate.ExecuteTemplate(&output, name, data); err != nil {
		v.handleRenderError(w, page, name, err)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)

	if _, err := w.Write(output.Bytes()); err != nil {
		v.logger.Error(
			"write template response",
			"event",
			"template_write_failed",
			"page",
			page,
			"template",
			name,
			"error",
			err,
		)
	}
}

// handleRenderError delegates HTTP error mapping to the adapter installed by the composition root.
func (v *Views) handleRenderError(w http.ResponseWriter, page, name string, err error) {
	logger := v.logger.With("operation", "render_template", "page", page, "template", name)
	if v.renderError != nil {
		v.renderError(logger, w, err)
		return
	}

	logger.Error("render template", "event", "template_render_failed", "error", err)
}

// RenderHTML renders a trusted template fragment for insertion into rendered Markdown.
func (v *Views) RenderHTML(page, name string, data Screen) (template.HTML, error) {
	pageTemplate, ok := v.templates[page]
	if !ok {
		return "", fmt.Errorf("page template %q not found", page)
	}

	var output bytes.Buffer
	if err := pageTemplate.ExecuteTemplate(&output, name, data); err != nil {
		return "", fmt.Errorf("render %s template %s: %w", page, name, err)
	}

	return template.HTML(output.String()), nil
}
