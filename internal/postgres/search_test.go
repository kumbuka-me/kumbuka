package postgres

import (
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/searchquery"
	"github.com/stretchr/testify/assert"
)

func TestSearchQueryBuilderAppliesFiltersInStableOrder(t *testing.T) {
	t.Parallel()

	parsed := searchquery.Parse(`restore group:"Platform Team" tag:Kubernetes title:Runbook namespace:ops author:Alice status:DRAFT owner:SRE property:"tier=gold"`)
	builder := newSearchQueryBuilder(parsed.Text)
	builder.applyFilters(parsed.Filters)
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
