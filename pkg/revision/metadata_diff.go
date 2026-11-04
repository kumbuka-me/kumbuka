package revision

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
)

// MetadataDifference describes one nonsecret metadata field's old and new values.
type MetadataDifference struct {
	// Name labels the edited metadata field.
	Name string `json:"name"`
	// Before contains the previous display value, or an omission marker for sensitive fields.
	Before string `json:"before"`
	// After contains the updated display value, or an omission marker for sensitive fields.
	After string `json:"after"`
}

// CompareSnapshots produces safe display values for fields changed between two recorded metadata states.
// Arbitrary property values are never included in rendered differences or audit summaries.
func CompareSnapshots(before, after PageSnapshot) []MetadataDifference {
	result := make([]MetadataDifference, 0, 12)
	add := func(label, old, current string) {
		if old != current {
			result = append(result, MetadataDifference{Name: label, Before: displayValue(old), After: displayValue(current)})
		}
	}
	add("path", before.Slug, after.Slug)
	add("title", before.Title, after.Title)
	add("icon", before.Icon, after.Icon)
	add("language", before.Language, after.Language)
	if !slices.Equal(before.Tags, after.Tags) {
		add("tags", strings.Join(before.Tags, ", "), strings.Join(after.Tags, ", "))
	}
	if !slices.Equal(before.GroupIDs, after.GroupIDs) {
		add("collaborators", ids(before.GroupIDs), ids(after.GroupIDs))
	}
	add("status", before.Status, after.Status)
	add("owner", strconv.FormatInt(before.OwnerGroupID, 10), strconv.FormatInt(after.OwnerGroupID, 10))
	add("review interval", strconv.Itoa(before.ReviewIntervalDays), strconv.Itoa(after.ReviewIntervalDays))
	add("replacement page", before.DeprecatedTarget, after.DeprecatedTarget)
	if !maps.Equal(before.Properties, after.Properties) {
		result = append(result, MetadataDifference{Name: "properties", Before: "(values hidden)", After: "(values hidden)"})
	}
	oldDate, newDate := "", ""
	if before.ReviewedAt != nil {
		oldDate = before.ReviewedAt.Format("2006-01-02 15:04")
	}
	if after.ReviewedAt != nil {
		newDate = after.ReviewedAt.Format("2006-01-02 15:04")
	}
	if !equalReviewTime(before.ReviewedAt, after.ReviewedAt) {
		result = append(result, MetadataDifference{Name: "reviewed at", Before: displayValue(oldDate), After: displayValue(newDate)})
	}
	return result
}

// ids renders sorted metadata group identifiers without consulting mutable group names.
func ids(numbers []int64) string {
	labels := make([]string, 0, len(numbers))
	for _, id := range numbers {
		labels = append(labels, fmt.Sprint(id))
	}
	return strings.Join(labels, ", ")
}

// displayValue clips untrusted metadata to a safe, concise label.
func displayValue(value string) string {
	if value == "" {
		return "(none)"
	}
	runes := []rune(value)
	if len(runes) > 120 {
		return string(runes[:120]) + "…"
	}
	return value
}
