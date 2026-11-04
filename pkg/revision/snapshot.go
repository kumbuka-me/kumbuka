package revision

import (
	"maps"
	"slices"
	"time"
)

// PageSnapshot contains editable page metadata at one committed revision.
// Derived render artifacts and access-independent counters are intentionally excluded.
type PageSnapshot struct {
	// Slug is the canonical page path.
	Slug string `json:"slug"`
	// Title is the page display title.
	Title string `json:"title"`
	// Icon is the page navigation icon identifier.
	Icon string `json:"icon"`
	// Language is the selected content-search language.
	Language string `json:"language"`
	// Tags is the normalized tag set.
	Tags []string `json:"tags"`
	// GroupIDs identifies explicit collaborating groups.
	GroupIDs []int64 `json:"group_ids"`
	// Status is the page lifecycle status.
	Status string `json:"status"`
	// OwnerGroupID identifies the documentation owner.
	OwnerGroupID int64 `json:"owner_group_id"`
	// ReviewIntervalDays configures the documentation review cadence.
	ReviewIntervalDays int `json:"review_interval_days"`
	// DeprecatedTarget points to a replacement page.
	DeprecatedTarget string `json:"deprecated_target"`
	// Properties is the editable page property map.
	Properties map[string]string `json:"properties"`
	// ReviewedAt records the review timestamp for history, but is not restored.
	ReviewedAt *time.Time `json:"reviewed_at"`
}

// ChangedFields lists changed metadata field names without revealing field values.
// A nil previous snapshot represents a newly created page.
func ChangedFields(previous *PageSnapshot, current PageSnapshot) []string {
	if previous == nil {
		return nil
	}
	changed := make([]string, 0, 11)
	if previous.Slug != current.Slug {
		changed = append(changed, "path")
	}
	if previous.Title != current.Title {
		changed = append(changed, "title")
	}
	if previous.Icon != current.Icon {
		changed = append(changed, "icon")
	}
	if previous.Language != current.Language {
		changed = append(changed, "language")
	}
	if !slices.Equal(previous.Tags, current.Tags) {
		changed = append(changed, "tags")
	}
	if !slices.Equal(previous.GroupIDs, current.GroupIDs) {
		changed = append(changed, "collaborators")
	}
	if previous.Status != current.Status {
		changed = append(changed, "status")
	}
	if previous.OwnerGroupID != current.OwnerGroupID {
		changed = append(changed, "owner")
	}
	if previous.ReviewIntervalDays != current.ReviewIntervalDays {
		changed = append(changed, "review interval")
	}
	if previous.DeprecatedTarget != current.DeprecatedTarget {
		changed = append(changed, "replacement page")
	}
	if !maps.Equal(previous.Properties, current.Properties) {
		changed = append(changed, "properties")
	}
	if !equalReviewTime(previous.ReviewedAt, current.ReviewedAt) {
		changed = append(changed, "reviewed at")
	}
	return changed
}

// equalReviewTime compares optional review dates independently of pointer identity.
func equalReviewTime(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}
