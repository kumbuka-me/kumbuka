package searchquery

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParsePreservesQuotedFreeTextAndFilters(t *testing.T) {
	t.Parallel()

	parsed := Parse(`"postgres restore" group:"Platform Team" tag:kubernetes`)

	assert.Equal(t, `"postgres restore"`, parsed.Text)
	assert.Equal(t, map[string][]string{
		"group": {"Platform Team"},
		"tag":   {"kubernetes"},
	}, parsed.Filters)
}

func TestParseDecodesEscapedQuotedFilterValue(t *testing.T) {
	t.Parallel()

	parsed := Parse(`group:"Platform \"Blue\" Team"`)

	assert.Equal(t, map[string][]string{"group": {`Platform "Blue" Team`}}, parsed.Filters)
}

func TestParseKeepsUnknownFiltersAsFreeText(t *testing.T) {
	t.Parallel()

	parsed := Parse(`kind:runbook plain`)

	assert.Equal(t, "kind:runbook plain", parsed.Text)
	assert.Empty(t, parsed.Filters)
}
