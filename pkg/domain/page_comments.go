package domain

import "time"

// PageCommentSuggestion describes an applicable Markdown replacement attached to an inline discussion.
type PageCommentSuggestion struct {
	// RevisionNumber identifies the exact page revision from which the suggestion was created.
	RevisionNumber int `json:"revision_number"`
	// StartByte is the zero-based inclusive byte offset of Original in the revision Markdown.
	StartByte int `json:"start_byte"`
	// EndByte is the zero-based exclusive byte offset of Original in the revision Markdown.
	EndByte int `json:"end_byte"`
	// Original stores the exact Markdown source range used for conflict detection.
	Original string `json:"original"`
	// Replacement stores the Markdown proposed for the selected source range.
	Replacement string `json:"replacement"`
	// AppliedBy identifies the user who applied the suggestion, or zero while unapplied.
	AppliedBy int64 `json:"applied_by,omitempty"`
	// AppliedByName is the display name of the user who applied the suggestion.
	AppliedByName string `json:"applied_by_name,omitempty"`
	// AppliedAt records when the suggestion was applied; nil means it remains unapplied.
	AppliedAt *time.Time `json:"applied_at,omitempty"`
}

// PageComment is a discussion item optionally anchored to selected page text.
type PageComment struct {
	// ID identifies page comment.
	ID int64 `json:"id"`
	// PageID identifies the page associated with page comment.
	PageID int64 `json:"page_id"`
	// AuthorID identifies the user who created the comment, or zero if the account was deleted.
	AuthorID int64 `json:"-"`
	// ParentID identifies the comment this item replies to.
	ParentID int64 `json:"parent_id,omitempty"`
	// ParentAuthor is the display name of the replied-to comment author.
	ParentAuthor string `json:"parent_author,omitempty"`
	// ParentBody contains the replied-to comment body for compact context.
	ParentBody string `json:"parent_body,omitempty"`
	// Author is the comment author display name.
	Author string `json:"author"`
	// Anchor stores the selected rendered text associated with the discussion.
	Anchor string `json:"anchor"`
	// Quote contains an optional excerpt explicitly quoted by the reply author.
	Quote string `json:"quote,omitempty"`
	// Body contains the comment text.
	Body string `json:"body"`
	// Suggestion contains an applicable Markdown replacement for anchored root comments.
	Suggestion *PageCommentSuggestion `json:"suggestion,omitempty"`
	// Resolved reports whether the discussion has been closed.
	Resolved *time.Time `json:"resolved_at,omitempty"`
	// CreatedAt records the created at timestamp for page comment.
	CreatedAt time.Time `json:"created_at"`
}
