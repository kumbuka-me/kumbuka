package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSearchTokensPreserveQuotedSegments(t *testing.T) {
	t.Parallel()

	got := searchTokens(`group:"Platform Team" tag:kubernetes "postgres restore"`)
	want := []string{`group:"Platform Team"`, "tag:kubernetes", `"postgres restore"`}

	assert.Equal(t, want, got)
}

func TestParseSearchQueryPreservesQuotedFreeText(t *testing.T) {
	t.Parallel()

	parsed := parseSearchQuery(`"postgres restore" group:"Platform Team"`)

	assert.Equal(t, `"postgres restore"`, parsed.text)
	assert.Equal(t, map[string][]string{"group": {"Platform Team"}}, parsed.filters)
}

func TestSplitSearchFilterDecodesEscapedQuotedValue(t *testing.T) {
	t.Parallel()

	key, value, ok := splitSearchFilter(`group:"Platform \"Blue\" Team"`)

	assert.True(t, ok)
	assert.Equal(t, "group", key)
	assert.Equal(t, `Platform "Blue" Team`, value)
}

func TestSearchQueryBuilderAppliesFiltersInStableOrder(t *testing.T) {
	t.Parallel()

	parsed := parseSearchQuery(`restore group:"Platform Team" tag:Kubernetes title:Runbook namespace:ops author:Alice status:DRAFT owner:SRE property:"tier=gold"`)
	assert.Equal(t, "restore", parsed.text)
	assert.Equal(t, map[string][]string{
		"group":     {"Platform Team"},
		"tag":       {"Kubernetes"},
		"title":     {"Runbook"},
		"namespace": {"ops"},
		"author":    {"Alice"},
		"status":    {"DRAFT"},
		"owner":     {"SRE"},
		"property":  {"tier=gold"},
	}, parsed.filters)

	builder := newSearchQueryBuilder(parsed.text)
	builder.applyFilters(parsed.filters)
	query := builder.sql(25, 50)

	assert.Equal(t, queryArgs{
		"restore",
		"kubernetes",
		"platform team",
		"%Runbook%",
		"ops/%",
		"%Alice%",
		"draft",
		"sre",
		"tier",
		"%gold%",
		25,
		50,
	}, builder.args)
	assert.Contains(t, query, "p.search_vector @@ websearch_to_tsquery('english', $1)")
	assert.Contains(t, query, "xt.name=$2")
	assert.Contains(t, query, "lower(g.name)=$3")
	assert.Contains(t, query, "p.title ILIKE $4")
	assert.Contains(t, query, "p.slug ILIKE $5")
	assert.Contains(t, query, "u.username ILIKE $6")
	assert.Contains(t, query, "lower(p.status)=$7")
	assert.Contains(t, query, "lower(og.name)=$8")
	assert.Contains(t, query, "lower(pp.key)=$9 AND pp.value ILIKE $10")
	assert.Contains(t, query, "LIMIT $11 OFFSET $12")
}

func TestParseSearchQueryKeepsUnknownFiltersAsText(t *testing.T) {
	t.Parallel()

	parsed := parseSearchQuery(`kind:runbook plain`)

	assert.Equal(t, "kind:runbook plain", parsed.text)
	assert.Empty(t, parsed.filters)
}
