package postgres

import (
	"strings"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/revision"
	"github.com/stretchr/testify/assert"
)

func TestRevisionAuditSummaryHidesPropertyValues(t *testing.T) {
	t.Parallel()
	before := revision.PageSnapshot{Title: "Before", Properties: map[string]string{"credential": "private_old"}}
	after := revision.PageSnapshot{Title: "After", Properties: map[string]string{"credential": "private_new"}}
	summary := pageRevisionAuditSummary(false, before, after, "page.updated")
	assert.Contains(t, summary, "title: Before → After")
	assert.Contains(t, summary, "properties")
	assert.False(t, strings.Contains(summary, "private_old"))
	assert.False(t, strings.Contains(summary, "private_new"))
}

func TestRevisionSummaryDescribesOnlyRealChanges(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "Page created", pageRevisionSummary(true, nil, "page.created"))
	assert.Equal(t, "Changed: tags", pageRevisionSummary(false, []string{"tags"}, "page.updated"))
	assert.Equal(t, "Markdown changed · Changed: title", pageRevisionSummary(true, []string{"title"}, "page.updated"))
}
