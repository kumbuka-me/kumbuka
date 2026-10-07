package domain

import "time"

// GraphNode is one page in the wiki relationship graph.
type GraphNode struct {
	// ID is the stable persisted page identity.
	ID int64 `json:"id"`
	// Slug is the normalized page path associated with graph node.
	Slug string `json:"slug"`
	// Title is the title associated with graph node.
	Title string `json:"title"`
	// Status is the current status of graph node.
	Status PageStatus `json:"status"`
}

// GraphEdge is one wiki-link relationship between pages.
type GraphEdge struct {
	// Source records the source associated with graph edge.
	Source string `json:"source"`
	// Target is the destination page slug.
	Target string `json:"target"`
}

// KnowledgeGraph contains graph nodes and link edges.
type KnowledgeGraph struct {
	// Nodes contains the nodes associated with knowledge graph.
	Nodes []GraphNode `json:"nodes"`
	// Edges contains the edges associated with knowledge graph.
	Edges []GraphEdge `json:"edges"`
}

// RecentEdit describes a page recently edited by one user.
type RecentEdit struct {
	// Page embeds page behavior in recent edit.
	Page
	// RevisionMessage contains the revision message for recent edit.
	RevisionMessage string
}

// PageDraft is a private, autosaved editor state owned by one user.
type PageDraft struct {
	// ID identifies page draft.
	ID int64 `json:"id"`
	// Key is the lookup key for page draft.
	Key string `json:"key"`
	// PageID identifies the page associated with page draft.
	PageID int64 `json:"page_id,omitempty"`
	// BaseRevision is the page revision from which editing began.
	BaseRevision int `json:"base_revision"`
	// CurrentRevision is the latest persisted page revision.
	CurrentRevision int `json:"current_revision"`
	// Stale reports whether the page changed after drafting began.
	Stale bool `json:"stale"`
	// Title is the title associated with page draft.
	Title string `json:"title"`
	// Slug is the normalized page path associated with page draft.
	Slug string `json:"slug"`
	// PageSlug is the page slug associated with page draft.
	PageSlug string `json:"page_slug,omitempty"`
	// Values contains the values represented by page draft.
	Values map[string][]string `json:"values,omitempty"`
	// CreatedAt records the created at timestamp for page draft.
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt records the updated at timestamp for page draft.
	UpdatedAt time.Time `json:"updated_at"`
}
