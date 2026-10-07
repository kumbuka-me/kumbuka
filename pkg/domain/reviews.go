package domain

import "time"

// PageReviewRequest tracks lightweight documentation approval for one revision.
type PageReviewRequest struct {
	// ID identifies page review request.
	ID int64
	// PageSlug is the page slug associated with page review request.
	PageSlug string
	// RevisionNumber is the immutable page revision under review.
	RevisionNumber int
	// RequestedBy identifies the user who requested review.
	RequestedBy int64
	// RequestedByName is the requested by name associated with page review request.
	RequestedByName string
	// ReviewerGroupID identifies the reviewer group associated with page review request.
	ReviewerGroupID int64
	// ReviewerGroupName is the reviewer group name associated with page review request.
	ReviewerGroupName string
	// Reviewers contains the reviewers associated with page review request.
	Reviewers []User
	// ReviewedBy identifies the user who completed the review.
	ReviewedBy int64
	// ReviewedByName is the reviewed by name associated with page review request.
	ReviewedByName string
	// Status is the current status of page review request.
	Status PageReviewStatus
	// Note is the message supplied when review was requested.
	Note string
	// DecisionNote explains the reviewer decision.
	DecisionNote string
	// PreviousStatus is the previous status associated with page review request.
	PreviousStatus PageReviewStatus
	// CreatedAt records the created at timestamp for page review request.
	CreatedAt time.Time
	// UpdatedAt records the updated at timestamp for page review request.
	UpdatedAt time.Time
}

// PageReviewComment is line-anchored feedback attached to one immutable review revision.
type PageReviewComment struct {
	// ID identifies the review comment.
	ID int64
	// ReviewRequestID identifies the review request that owns the comment.
	ReviewRequestID int64
	// AuthorID identifies the user who created the comment.
	AuthorID int64
	// Author is the display name of the comment author.
	Author string
	// Side selects the previous or reviewed side of the revision diff.
	Side PageReviewCommentSide
	// StartLine is the first one-based source line covered by the comment.
	StartLine int
	// EndLine is the last one-based source line covered by the comment.
	EndLine int
	// Body contains the human discussion text associated with the line range.
	Body string
	// IsSuggestion reports whether Replacement proposes an applicable source change.
	IsSuggestion bool
	// Original stores the exact reviewed Markdown range used for conflict detection.
	Original string
	// Replacement stores the Markdown proposed by a suggestion.
	Replacement string
	// AppliedBy identifies the user who applied the suggestion, or zero while unapplied.
	AppliedBy int64
	// AppliedByName is the display name of the user who applied the suggestion.
	AppliedByName string
	// AppliedAt records when a suggestion was applied; nil means it remains unapplied.
	AppliedAt *time.Time
	// CreatedAt records when the review comment was created.
	CreatedAt time.Time
}
