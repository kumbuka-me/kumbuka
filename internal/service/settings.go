package service

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/secrets"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/icons"
	"golang.org/x/net/http/httpguts"
)

var reservedPDFHeaderNames = map[string]struct{}{
	"Accept":            {},
	"Connection":        {},
	"Content-Encoding":  {},
	"Content-Length":    {},
	"Content-Type":      {},
	"Host":              {},
	"Keep-Alive":        {},
	"Proxy-Connection":  {},
	"Te":                {},
	"Trailer":           {},
	"Transfer-Encoding": {},
	"Upgrade":           {},
}

// PDFHeaderInput contains one administrator-supplied PDF request header.
type PDFHeaderInput struct {
	// ID identifies PDF header input.
	ID int64
	// Name is the name of PDF header input.
	Name string
	// Value contains the value represented by PDF header input.
	Value string
	// Sensitive reports whether sensitive applies to PDF header input.
	Sensitive bool
}

// settingsRepository contains persisted application configuration operations.
type settingsRepository interface {
	auditRepository
	ApplicationSettings(context.Context) (domain.ApplicationSettings, error)
	BrandLogo(context.Context) (string, []byte, error)
	ClearBrandLogo(context.Context) error
	PDFHeaders(context.Context) ([]domain.PDFHeader, error)
	SaveApplicationSettings(context.Context, domain.ApplicationSettings) error
	SaveAuthenticationSettings(context.Context, domain.AuthenticationSettings) error
	SaveBrandLogo(context.Context, string, []byte) error
	SavePDFSettings(context.Context, string, []domain.PDFHeader) error
}

// Settings exposes persisted application configuration use cases.
type Settings struct {
	// repository provides the persistence operations required by settings.
	repository settingsRepository
	// secrets stores the secrets value used by settings.
	secrets *secrets.Cipher
	// iconCatalog stores the icon catalog value used by settings.
	iconCatalog *icons.Catalog
	// logger reports failures from best-effort audit side effects.
	logger *slog.Logger
}

// NewSettings constructs the application settings service.
func NewSettings(repository settingsRepository, secretCipher *secrets.Cipher) *Settings {
	return &Settings{repository: repository, secrets: secretCipher, iconCatalog: icons.Builtin(), logger: serviceLogger(nil)}
}

// WithLogger uses logger for best-effort service side-effect failures.
func (s *Settings) WithLogger(logger *slog.Logger) *Settings {
	s.logger = serviceLogger(logger)
	return s
}

// WithIconCatalog uses the active plugin-aware icon catalog for validation.
func (s *Settings) WithIconCatalog(catalog *icons.Catalog) *Settings {
	if catalog == nil {
		catalog = icons.Builtin()
	}
	s.iconCatalog = catalog
	return s
}

// ApplicationSettings returns the current application configuration.
func (s *Settings) ApplicationSettings(ctx context.Context) (domain.ApplicationSettings, error) {
	return s.repository.ApplicationSettings(ctx)
}

// PDFHeaders returns PDF request headers without exposing persisted sensitive values.
func (s *Settings) PDFHeaders(ctx context.Context) ([]domain.PDFHeader, error) {
	headers, err := s.repository.PDFHeaders(ctx)
	if err != nil {
		return nil, err
	}

	for index := range headers {
		if !headers[index].Sensitive {
			continue
		}

		headers[index].Configured = headers[index].Value != ""
		headers[index].Value = ""
	}

	return headers, nil
}

// PDFRequestHeaders returns persisted PDF headers with sensitive values decrypted for one outbound request.
func (s *Settings) PDFRequestHeaders(ctx context.Context) ([]domain.PDFHeader, error) {
	headers, err := s.repository.PDFHeaders(ctx)
	if err != nil {
		return nil, err
	}

	return s.decryptPDFHeaders(headers)
}

// ResolvePDFRequestHeaders resolves unsaved administrator form input for a PDF service test.
func (s *Settings) ResolvePDFRequestHeaders(ctx context.Context, inputs []PDFHeaderInput) ([]domain.PDFHeader, error) {
	existing, err := s.repository.PDFHeaders(ctx)
	if err != nil {
		return nil, err
	}

	headers, err := s.resolvePDFHeaderInputs(inputs, existing)
	if err == nil {
		return headers, nil
	}
	if errors.Is(err, secrets.ErrNotConfigured) {
		return nil, &domain.ValidationError{
			Fields: []domain.FieldError{{
				Field:   "pdf_headers",
				Message: "Configure KUMBUKA__ENCRYPTION_KEY before testing stored sensitive PDF headers.",
			}},
			Cause: err,
		}
	}

	return nil, err
}

// RevealPDFHeader returns one persisted header value for an explicit administrator reveal action.
func (s *Settings) RevealPDFHeader(ctx context.Context, id int64) (string, error) {
	headers, err := s.repository.PDFHeaders(ctx)
	if err != nil {
		return "", err
	}

	for _, header := range headers {
		if header.ID != id {
			continue
		}
		if !header.Sensitive {
			return header.Value, nil
		}

		value, err := s.decryptPDFHeader(header)
		if err == nil {
			return value, nil
		}
		if errors.Is(err, secrets.ErrNotConfigured) {
			return "", &domain.ValidationError{
				Fields: []domain.FieldError{{
					Field:   "pdf_headers",
					Message: "Configure KUMBUKA__ENCRYPTION_KEY before revealing sensitive PDF headers.",
				}},
				Cause: err,
			}
		}

		return "", err
	}

	return "", domain.ErrNotFound
}

// SaveApplicationSettings persists application settings and records the change.
func (s *Settings) SaveApplicationSettings(
	ctx context.Context,
	settings domain.ApplicationSettings,
	actorID int64,
) error {
	if !domain.ValidTypographySize(settings.Rendering.DefaultTypographySize) {
		return domain.NewValidationError("default_typography_size", "Choose a valid default typography size.")
	}
	if !domain.ValidRobotsPolicy(settings.RobotsPolicy) {
		return domain.NewValidationError("robots_policy", "Choose a valid robots.txt policy.")
	}

	externalLinks, err := normalizeExternalLinksWithCatalog(settings.ExternalLinks, s.iconCatalog)
	if err != nil {
		return err
	}

	settings.ExternalLinks = externalLinks

	if err := s.repository.SaveApplicationSettings(ctx, settings); err != nil {
		return err
	}

	recordAuditEvent(
		ctx, s.logger, s.repository,
		actorID,
		"settings.application_updated",
		"settings",
		"application",
		"Updated application settings",
	)

	return nil
}

// normalizeExternalLinksWithCatalog normalizes external links and validates their icons against the active catalog.
func normalizeExternalLinksWithCatalog(links []domain.ExternalLink, catalog *icons.Catalog) ([]domain.ExternalLink, error) {
	normalized := make([]domain.ExternalLink, 0, len(links))

	for _, link := range links {
		link.Label = strings.TrimSpace(link.Label)
		link.URL = strings.TrimSpace(link.URL)
		link.Icon = strings.TrimSpace(link.Icon)
		link.Description = strings.TrimSpace(link.Description)
		link.HoverEffect = strings.TrimSpace(link.HoverEffect)
		link.HoverText = strings.TrimSpace(link.HoverText)

		if link == (domain.ExternalLink{}) {
			continue
		}
		if link.Label == "" {
			return nil, domain.NewValidationError("external_links", "Enter a label for every external link.")
		}
		if !validExternalLinkURL(link.URL) {
			return nil, domain.NewValidationError("external_links", "Enter a valid HTTP or HTTPS URL for every external link.")
		}
		if link.Icon != "" && !catalog.IsIcon(link.Icon) {
			return nil, domain.NewValidationError("external_links", "Choose external link icons from the available icon catalog.")
		}
		if !domain.ValidExternalLinkHoverEffect(link.HoverEffect) {
			return nil, domain.NewValidationError("external_links", "Choose a valid hover effect for every external link.")
		}
		link.HoverEffect = domain.EffectiveExternalLinkHoverEffect(link.HoverEffect)

		normalized = append(normalized, link)
	}

	return normalized, nil
}

// validExternalLinkURL reports whether value is an absolute HTTP or HTTPS URL.
func validExternalLinkURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Host == "" {
		return false
	}

	return strings.EqualFold(parsed.Scheme, "http") || strings.EqualFold(parsed.Scheme, "https")
}

// SavePDFSettings persists the PDF endpoint and request headers and records the change.
func (s *Settings) SavePDFSettings(
	ctx context.Context,
	pdfURL string,
	inputs []PDFHeaderInput,
	actorID int64,
) error {
	existing, err := s.repository.PDFHeaders(ctx)
	if err != nil {
		return err
	}

	resolved, err := s.resolvePDFHeaderInputs(inputs, existing)
	if err != nil {
		return pdfHeaderEncryptionError(err)
	}

	stored, err := s.preparePDFHeadersForStorage(resolved)
	if err != nil {
		return err
	}

	if err := s.repository.SavePDFSettings(ctx, pdfURL, stored); err != nil {
		return err
	}

	recordAuditEvent(
		ctx, s.logger, s.repository,
		actorID,
		"settings.pdf_updated",
		"settings",
		"pdf",
		"Updated PDF service settings",
	)

	return nil
}

// preparePDFHeadersForStorage encrypts sensitive header values and strips persisted row state.
func (s *Settings) preparePDFHeadersForStorage(headers []domain.PDFHeader) ([]domain.PDFHeader, error) {
	stored := make([]domain.PDFHeader, 0, len(headers))

	for _, header := range headers {
		prepared, err := s.preparePDFHeaderForStorage(header)
		if err != nil {
			return nil, err
		}

		stored = append(stored, prepared)
	}

	return stored, nil
}

// preparePDFHeaderForStorage encrypts one sensitive value and resets database-managed fields.
func (s *Settings) preparePDFHeaderForStorage(header domain.PDFHeader) (domain.PDFHeader, error) {
	header.ID = 0
	header.Configured = false

	if !header.Sensitive {
		return header, nil
	}

	value, err := s.secrets.Encrypt(header.Value)
	if err != nil {
		return domain.PDFHeader{}, pdfHeaderEncryptionError(err)
	}

	header.Value = value

	return header, nil
}

// pdfHeaderEncryptionError converts a missing deployment key into an actionable validation error.
func pdfHeaderEncryptionError(err error) error {
	if !errors.Is(err, secrets.ErrNotConfigured) {
		return err
	}

	return &domain.ValidationError{
		Fields: []domain.FieldError{{
			Field:   "pdf_headers",
			Message: "Configure KUMBUKA__ENCRYPTION_KEY before saving sensitive PDF headers.",
		}},
		Cause: err,
	}
}

// resolvePDFHeaderInputs validates request-header form input and resolves masked stored values.
func (s *Settings) resolvePDFHeaderInputs(inputs []PDFHeaderInput, existing []domain.PDFHeader) ([]domain.PDFHeader, error) {
	existingByID := make(map[int64]domain.PDFHeader, len(existing))
	for _, header := range existing {
		existingByID[header.ID] = header
	}

	seenNames := make(map[string]struct{}, len(inputs))
	seenIDs := make(map[int64]struct{}, len(inputs))
	resolved := make([]domain.PDFHeader, 0, len(inputs))

	for _, input := range inputs {
		name, err := normalizePDFHeaderName(input.Name)
		if err != nil {
			return nil, err
		}
		nameKey := strings.ToLower(name)
		if _, exists := seenNames[nameKey]; exists {
			return nil, domain.NewValidationError("pdf_headers", "PDF header names must be unique.")
		}
		seenNames[nameKey] = struct{}{}

		previous, err := existingPDFHeader(input.ID, existingByID, seenIDs)
		if err != nil {
			return nil, err
		}

		value := input.Value
		if strings.TrimSpace(value) == "" && previous.Sensitive {
			value, err = s.decryptPDFHeader(previous)
			if err != nil {
				return nil, err
			}
		}
		if strings.TrimSpace(value) == "" {
			return nil, domain.NewValidationError("pdf_headers", "Enter a value for every PDF request header.")
		}
		if !httpguts.ValidHeaderFieldValue(value) {
			return nil, domain.NewValidationError("pdf_headers", "PDF header values must be valid HTTP header values.")
		}

		resolved = append(resolved, domain.PDFHeader{
			ID:        input.ID,
			Name:      name,
			Value:     value,
			Sensitive: input.Sensitive,
		})
	}

	return resolved, nil
}

// existingPDFHeader resolves an existing row and rejects duplicate submitted identifiers.
func existingPDFHeader(
	id int64,
	existing map[int64]domain.PDFHeader,
	seen map[int64]struct{},
) (domain.PDFHeader, error) {
	if id == 0 {
		return domain.PDFHeader{}, nil
	}
	if _, exists := seen[id]; exists {
		return domain.PDFHeader{}, domain.NewValidationError("pdf_headers", "PDF header rows must be unique.")
	}
	seen[id] = struct{}{}

	header, exists := existing[id]
	if !exists {
		return domain.PDFHeader{}, domain.NewValidationError("pdf_headers", "One PDF header no longer exists. Reload the page and try again.")
	}

	return header, nil
}

// decryptPDFHeaders decrypts sensitive values while leaving ordinary headers unchanged.
func (s *Settings) decryptPDFHeaders(headers []domain.PDFHeader) ([]domain.PDFHeader, error) {
	resolved := make([]domain.PDFHeader, 0, len(headers))
	for _, header := range headers {
		if header.Sensitive {
			value, err := s.decryptPDFHeader(header)
			if err != nil {
				return nil, err
			}
			header.Value = value
		}

		resolved = append(resolved, header)
	}

	return resolved, nil
}

// decryptPDFHeader decrypts one sensitive stored value.
func (s *Settings) decryptPDFHeader(header domain.PDFHeader) (string, error) {
	if !header.Sensitive {
		return header.Value, nil
	}
	if s.secrets == nil {
		return "", secrets.ErrNotConfigured
	}

	return s.secrets.Decrypt(header.Value)
}

// normalizePDFHeaderName validates one configurable request-header name while preserving its display casing.
func normalizePDFHeaderName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", domain.NewValidationError("pdf_headers", "Enter a name for every PDF request header.")
	}
	if !httpguts.ValidHeaderFieldName(name) {
		return "", domain.NewValidationError("pdf_headers", "PDF header names must be valid HTTP header names.")
	}

	canonicalName := http.CanonicalHeaderKey(name)
	if _, forbidden := reservedPDFHeaderNames[canonicalName]; forbidden {
		return "", domain.NewValidationError("pdf_headers", canonicalName+" cannot be configured as a PDF request header.")
	}

	return name, nil
}

// SaveAuthenticationSettings persists authentication settings and records the change.
func (s *Settings) SaveAuthenticationSettings(
	ctx context.Context,
	settings domain.AuthenticationSettings,
	actorID int64,
) error {
	if err := s.repository.SaveAuthenticationSettings(ctx, settings); err != nil {
		return err
	}

	recordAuditEvent(
		ctx, s.logger, s.repository,
		actorID,
		"settings.authentication_updated",
		"settings",
		"authentication",
		"Updated authentication settings",
	)

	return nil
}

// RecordLocalPasswordUpdated records a local recovery password change.
func (s *Settings) RecordLocalPasswordUpdated(ctx context.Context, actor domain.User) {
	recordAuditEvent(
		ctx, s.logger, s.repository,
		actor.ID,
		"settings.local_password_updated",
		"user",
		actor.Username,
		"Updated local recovery password",
	)
}
