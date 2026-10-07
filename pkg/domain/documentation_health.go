package domain

// BrokenWikiLink describes a wiki link whose target page does not exist.
type BrokenWikiLink struct {
	// SourceSlug is the source slug associated with broken wiki link.
	SourceSlug string
	// SourceTitle is the source title associated with broken wiki link.
	SourceTitle string
	// TargetSlug is the target slug associated with broken wiki link.
	TargetSlug string
}

// DocumentationHealth groups page-quality findings for administrators.
type DocumentationHealth struct {
	// BrokenLinks contains the broken links associated with documentation health.
	BrokenLinks []BrokenWikiLink
	// OrphanPages contains the orphan pages associated with documentation health.
	OrphanPages []Page
	// UntaggedPages contains the untagged pages associated with documentation health.
	UntaggedPages []Page
	// UniconedPages contains the uniconed pages associated with documentation health.
	UniconedPages []Page
	// StalePages contains the stale pages associated with documentation health.
	StalePages []Page
	// ReviewDue contains the review due associated with documentation health.
	ReviewDue []Page
	// DraftPages contains the draft pages associated with documentation health.
	DraftPages []Page
	// Deprecated contains the deprecated associated with documentation health.
	Deprecated []Page
}
