// Package portable defines Kumbuka's versioned, instance-portable archive format.
package portable

const (
	// Format identifies a Kumbuka portable archive manifest.
	Format = "kumbuka"
	// Version is the newest portable archive format understood by this build.
	Version = 1
	// ManifestPath is the required root-level archive manifest.
	ManifestPath = "manifest.json"
)

// Manifest inventories every portable page and referenced file in an archive.
type Manifest struct {
	// Format identifies the archive producer and must equal Format.
	Format string `json:"format"`
	// Version identifies the portable archive schema version.
	Version int `json:"version"`
	// Pages contains every exported page in deterministic slug order.
	Pages []PageEntry `json:"pages"`
	// Media contains referenced uploaded images included in the archive.
	Media []ResourceEntry `json:"media,omitempty"`
	// Attachments contains referenced uploaded files included in the archive.
	Attachments []ResourceEntry `json:"attachments,omitempty"`
}

// PageEntry points from a page slug to its Markdown and metadata archive entries.
type PageEntry struct {
	// Slug is the canonical Kumbuka page path.
	Slug string `json:"slug"`
	// Markdown is the archive-relative path containing the page source.
	Markdown string `json:"markdown"`
	// Metadata is the archive-relative path containing portable page metadata.
	Metadata string `json:"metadata"`
}

// ResourceEntry identifies one binary resource bundled with the archive.
type ResourceEntry struct {
	// Path is the archive-relative path containing the resource bytes.
	Path string `json:"path"`
	// Filename is the portable original filename used when recreating the resource.
	Filename string `json:"filename"`
}

// PageMetadata contains page settings that remain meaningful across Kumbuka instances.
type PageMetadata struct {
	// Slug is the canonical Kumbuka page path and must match the manifest entry.
	Slug string `json:"slug"`
	// Title is the human-readable page title.
	Title string `json:"title"`
	// Icon is the optional icon identifier displayed beside the page title.
	Icon string `json:"icon,omitempty"`
	// Language optionally overrides the instance-wide content language.
	Language string `json:"language,omitempty"`
	// Tags contains normalized page tags.
	Tags []string `json:"tags,omitempty"`
	// Groups contains collaboration group names assigned to the page.
	Groups []string `json:"groups,omitempty"`
	// Status is the page lifecycle state.
	Status string `json:"status"`
	// OwnerGroup is the portable collaboration group name responsible for the page.
	OwnerGroup string `json:"owner_group,omitempty"`
	// ReviewIntervalDays configures the page's documentation review cadence.
	ReviewIntervalDays int `json:"review_interval_days,omitempty"`
	// DeprecatedTarget points a deprecated page at its replacement page path.
	DeprecatedTarget string `json:"deprecated_target,omitempty"`
	// Properties contains searchable structured page metadata.
	Properties map[string]string `json:"properties,omitempty"`
}

// NewManifest creates an empty manifest for the current portable archive format.
func NewManifest() Manifest {
	return Manifest{Format: Format, Version: Version, Pages: []PageEntry{}}
}
