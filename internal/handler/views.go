package handler

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/httpresponse"
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
	"shared_page",
	"home",
	"page",
	"not_found",
	"edit",
	"search",
	"settings",
	"admin",
	"admin_configuration",
	"admin_branding",
	"admin_plugins",
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
	"admin_navigation",
	"admin_bin",
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
}

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
}

// NewViews parses each page template with the shared layout and partials once at startup.
func NewViews(
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

// IconCatalog returns the catalog used by this view set and icon picker.
func (v *Views) IconCatalog() *icons.Catalog { return v.iconCatalog }

// render executes a page layout into a buffer before writing the HTTP response.
func render(views *Views, w http.ResponseWriter, page string, data ViewData) {
	renderTemplate(views, w, page, "layout", data)
}

// renderStatus executes a page layout with an explicit HTTP status.
func renderStatus(views *Views, w http.ResponseWriter, status int, page string, data ViewData) {
	renderTemplateStatus(views, w, status, page, "layout", data)
}

// renderPublic executes the minimal unauthenticated page layout.
func renderPublic(views *Views, w http.ResponseWriter, page string, data ViewData) {
	renderTemplate(views, w, page, "public-layout", data)
}

// renderFragment executes one named fragment from a parsed page template set.
func renderFragment(views *Views, w http.ResponseWriter, page, name string, data ViewData) {
	renderTemplate(views, w, page, name, data)
}

// renderTemplate executes a named template with a successful HTTP status.
func renderTemplate(views *Views, w http.ResponseWriter, page, name string, data ViewData) {
	renderTemplateStatus(views, w, http.StatusOK, page, name, data)
}

// renderTemplateStatus executes a named ViewData template with an explicit HTTP status.
func renderTemplateStatus(views *Views, w http.ResponseWriter, status int, page, name string, data ViewData) {
	renderTemplateDataStatus(views, w, status, page, name, data)
}

// renderTemplateDataStatus executes a named template with arbitrary view data and an explicit HTTP status.
func renderTemplateDataStatus(views *Views, w http.ResponseWriter, status int, page, name string, data any) {
	pageTemplate, ok := views.templates[page]
	if !ok {
		httpresponse.InternalServerError(views.logger.With("operation", "render_template", "page", page, "template", name), w, fmt.Errorf("page template %q not found", page))
		return
	}

	var output bytes.Buffer
	if err := pageTemplate.ExecuteTemplate(&output, name, data); err != nil {
		httpresponse.InternalServerError(views.logger.With("operation", "render_template", "page", page, "template", name), w, err)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)

	if _, err := w.Write(output.Bytes()); err != nil {
		views.logger.Error(
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

// renderTemplateHTML renders a trusted template fragment for insertion into rendered Markdown.
func renderTemplateHTML(views *Views, page, name string, data ViewData) (template.HTML, error) {
	pageTemplate, ok := views.templates[page]
	if !ok {
		return "", fmt.Errorf("page template %q not found", page)
	}

	var output bytes.Buffer
	if err := pageTemplate.ExecuteTemplate(&output, name, data); err != nil {
		return "", fmt.Errorf("render %s template %s: %w", page, name, err)
	}

	return template.HTML(output.String()), nil
}
