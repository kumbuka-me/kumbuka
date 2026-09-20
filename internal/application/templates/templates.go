package templates

import (
	"context"
	"strings"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/icons"
	md "github.com/kumbuka-me/kumbuka/pkg/markdown"
)

// PageTemplateInput contains transport-independent page-blueprint settings.
type PageTemplateInput struct {
	// Name is the name of page template input.
	Name string
	// Description describes page template input.
	Description string
	// Markdown stores the markdown value used by page template input.
	Markdown string
	// PathPrefix stores the path prefix value used by page template input.
	PathPrefix string
	// Icon names the icon used for page template input.
	Icon string
	// Tags contains the tags associated with page template input.
	Tags []string
	// Status is the current status of page template input.
	Status string
	// OwnerGroupID identifies the owner group associated with page template input.
	OwnerGroupID int64
	// ReviewIntervalDays stores the review interval days value used by page template input.
	ReviewIntervalDays int
	// Properties maps keys to properties values used by page template input.
	Properties map[string]string
	// Fields contains the fields associated with page template input.
	Fields []domain.PageTemplateField
}

// templateRepository contains reusable page template operations.
type templateRepository interface {
	PageTemplates(context.Context) ([]domain.PageTemplate, error)
	PageTemplate(context.Context, int64) (domain.PageTemplate, error)
	CreatePageTemplate(context.Context, domain.PageTemplate) (domain.PageTemplate, error)
	UpdatePageTemplate(context.Context, int64, domain.PageTemplate) error
	DeletePageTemplate(context.Context, int64) error
}

// Templates exposes reusable page-blueprint use cases.
type Templates struct {
	// repository provides the persistence operations required by templates.
	repository templateRepository
	// iconCatalog stores the icon catalog value used by templates.
	iconCatalog *icons.Catalog
}

// NewTemplates constructs the reusable page template service.
func NewTemplates(repository templateRepository) *Templates {
	return &Templates{repository: repository, iconCatalog: icons.Builtin()}
}

// WithIconCatalog uses the active plugin-aware icon catalog for validation.
func (s *Templates) WithIconCatalog(catalog *icons.Catalog) *Templates {
	if catalog == nil {
		catalog = icons.Builtin()
	}
	s.iconCatalog = catalog
	return s
}

// PageTemplates returns all reusable page blueprints.
func (s *Templates) PageTemplates(ctx context.Context) ([]domain.PageTemplate, error) {
	return s.repository.PageTemplates(ctx)
}

// PageTemplate returns a reusable page blueprint by identifier.
func (s *Templates) PageTemplate(ctx context.Context, id int64) (domain.PageTemplate, error) {
	return s.repository.PageTemplate(ctx, id)
}

// CreatePageTemplate creates a reusable page blueprint.
func (s *Templates) CreatePageTemplate(ctx context.Context, input PageTemplateInput) (domain.PageTemplate, error) {
	item, err := validatePageTemplateWithCatalog(input, s.iconCatalog)
	if err != nil {
		return domain.PageTemplate{}, err
	}
	return s.repository.CreatePageTemplate(ctx, item)
}

// UpdatePageTemplate replaces a reusable page blueprint.
func (s *Templates) UpdatePageTemplate(ctx context.Context, id int64, input PageTemplateInput) error {
	item, err := validatePageTemplateWithCatalog(input, s.iconCatalog)
	if err != nil {
		return err
	}
	return s.repository.UpdatePageTemplate(ctx, id, item)
}

// DeletePageTemplate removes a reusable page template.
func (s *Templates) DeletePageTemplate(ctx context.Context, id int64) error {
	return s.repository.DeletePageTemplate(ctx, id)
}

// validatePageTemplateWithCatalog validates a page template against the active icon catalog.
func validatePageTemplateWithCatalog(input PageTemplateInput, catalog *icons.Catalog) (domain.PageTemplate, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.PathPrefix = md.Slug(input.PathPrefix)
	input.Icon = strings.TrimSpace(input.Icon)
	input.Status = defaultPageTemplateStatus(input.Status)

	fields, fieldProblems := normalizeTemplateFields(input.Fields)
	validation := validatePageTemplateSettings(input, catalog)
	validation.Fields = append(validation.Fields, fieldProblems...)

	if len(validation.Fields) > 0 {
		return domain.PageTemplate{}, validation
	}

	return domain.PageTemplate{
		Name:               input.Name,
		Description:        input.Description,
		Markdown:           input.Markdown,
		PathPrefix:         input.PathPrefix,
		Icon:               input.Icon,
		Tags:               normalizeTags(input.Tags),
		Status:             input.Status,
		OwnerGroupID:       input.OwnerGroupID,
		ReviewIntervalDays: input.ReviewIntervalDays,
		Properties:         normalizeTemplateProperties(input.Properties),
		Fields:             fields,
	}, nil
}

// defaultPageTemplateStatus applies the verified default to an unspecified blueprint status.
func defaultPageTemplateStatus(status string) string {
	if status == "" {
		return "verified"
	}

	return status
}

// validatePageTemplateSettings validates blueprint settings outside prompted fields.
func validatePageTemplateSettings(input PageTemplateInput, catalogs ...*icons.Catalog) *domain.ValidationError {
	catalog := icons.Builtin()
	if len(catalogs) > 0 && catalogs[0] != nil {
		catalog = catalogs[0]
	}
	validation := &domain.ValidationError{}

	if input.Name == "" {
		validation.Fields = append(validation.Fields, domain.FieldError{Field: "name", Message: "A template name is required."})
	}
	if !catalog.IsIcon(input.Icon) {
		validation.Fields = append(validation.Fields, domain.FieldError{Field: "icon", Message: "Choose an icon from the available icon catalog."})
	}
	if !domain.ValidPageStatus(input.Status) {
		validation.Fields = append(validation.Fields, domain.FieldError{Field: "status", Message: "Choose a valid default page status."})
	}
	if input.OwnerGroupID < 0 || !domain.ValidReviewIntervalDays(input.ReviewIntervalDays) {
		validation.Fields = append(validation.Fields, domain.FieldError{Field: "review_interval_days", Message: "Choose valid review defaults."})
	}

	return validation
}

// normalizeTemplateFields normalizes prompted fields and returns field-specific problems.
func normalizeTemplateFields(values []domain.PageTemplateField) ([]domain.PageTemplateField, []domain.FieldError) {
	fields := make([]domain.PageTemplateField, 0, len(values))
	problems := make([]domain.FieldError, 0)
	seen := map[string]bool{}

	for _, field := range values {
		field.Name = strings.TrimSpace(field.Name)
		field.Label = strings.TrimSpace(field.Label)
		field.Default = strings.TrimSpace(field.Default)

		if field.Name == "" && field.Label == "" {
			continue
		}
		if !validTemplateFieldName(field.Name) {
			problems = append(problems, domain.FieldError{Field: "fields", Message: "Field names may contain letters, numbers, underscores, and hyphens."})
			continue
		}

		key := strings.ToLower(field.Name)
		if seen[key] {
			problems = append(problems, domain.FieldError{Field: "fields", Message: "Template field names must be unique."})
			continue
		}
		seen[key] = true

		if field.Label == "" {
			field.Label = field.Name
		}

		fields = append(fields, field)
	}

	return fields, problems
}

// normalizeTemplateProperties trims property names and values and drops empty names.
func normalizeTemplateProperties(values map[string]string) map[string]string {
	properties := make(map[string]string, len(values))

	for key, value := range values {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}

		properties[key] = strings.TrimSpace(value)
	}

	return properties
}

// validTemplateFieldName reports whether a blueprint field name is portable.
func validTemplateFieldName(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' ||
			character == '_' || character == '-' {
			continue
		}
		return false
	}
	return true
}

// normalizeTags canonicalizes and deduplicates blueprint tags.
func normalizeTags(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}
