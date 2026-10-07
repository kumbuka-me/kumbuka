package domain

// PageTemplateField is one author-supplied value used by a page blueprint.
type PageTemplateField struct {
	// Name is the name of page template field.
	Name string `json:"name"`
	// Label is the display label for page template field.
	Label string `json:"label"`
	// Default is the field value used when the author leaves it empty.
	Default string `json:"default,omitempty"`
	// Required prevents creating a page without a field value.
	Required bool `json:"required,omitempty"`
}

// PageTemplate is a reusable page blueprint offered when creating a page.
type PageTemplate struct {
	// ID identifies page template.
	ID int64
	// Name is the name of page template.
	Name string
	// Description describes page template.
	Description string
	// Markdown is the template body before field substitution.
	Markdown string
	// PathPrefix is prepended to slugs created from the template.
	PathPrefix string
	// Icon names the icon used for page template.
	Icon string
	// Tags contains the tags associated with page template.
	Tags []string
	// Status is the current status of page template.
	Status PageStatus
	// OwnerGroupID identifies the owner group associated with page template.
	OwnerGroupID int64
	// ReviewIntervalDays is copied to pages created from the template.
	ReviewIntervalDays int
	// Properties maps keys to properties values used by page template.
	Properties map[string]string
	// Fields contains the fields associated with page template.
	Fields []PageTemplateField
}
