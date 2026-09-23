package postgres

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// isSearchFilter reports whether a token prefix is a supported field filter.
func isSearchFilter(key string) bool {
	switch key {
	case "tag", "group", "title", "namespace", "author", "status", "owner", "property":
		return true
	default:
		return false
	}
}

// Search supports free text and field filters for taxonomy, ownership, lifecycle, and structured properties.
func (s *Store) Search(ctx context.Context, query string, limit int) ([]domain.Page, error) {
	return s.SearchPage(ctx, query, limit, 0)
}

// SearchPage returns one deterministic window of filtered search results.
func (s *Store) SearchPage(ctx context.Context, query string, limit, offset int) ([]domain.Page, error) {
	parsed := parseSearchQuery(query)
	builder := newSearchQueryBuilder(parsed.text)
	builder.applyFilters(parsed.filters)

	rows, err := s.pool.Query(ctx, builder.sql(limit, offset), builder.args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanSearchPages(rows)
}

// parsedSearchQuery separates free-text terms from supported field filters.
type parsedSearchQuery struct {
	// text preserves free-text websearch syntax after supported filters are removed.
	text string
	// filters groups supported filter values by their normalized filter name.
	filters map[string][]string
}

// parseSearchQuery classifies search tokens without changing their original value semantics.
func parseSearchQuery(query string) parsedSearchQuery {
	var textTerms []string
	filters := make(map[string][]string)
	for _, token := range searchTokens(query) {
		key, value, ok := splitSearchFilter(token)
		if ok {
			filters[key] = append(filters[key], value)
			continue
		}

		textTerms = append(textTerms, token)
	}
	return parsedSearchQuery{text: strings.Join(textTerms, " "), filters: filters}
}

// splitSearchFilter recognizes supported field filters and decodes a quoted filter value.
func splitSearchFilter(token string) (string, string, bool) {
	key, value, found := strings.Cut(token, ":")
	key = strings.ToLower(key)
	if !found || value == "" || !isSearchFilter(key) {
		return "", "", false
	}

	return key, unquoteSearchValue(value), true
}

// unquoteSearchValue removes matching filter-value quotes while preserving escaped characters.
func unquoteSearchValue(value string) string {
	if !hasMatchingSearchValueQuotes(value) {
		return value
	}

	var decoded strings.Builder
	escaped := false
	for _, character := range value[1 : len(value)-1] {
		if escaped {
			decoded.WriteRune(character)
			escaped = false
			continue
		}
		if character == '\\' {
			escaped = true
			continue
		}
		decoded.WriteRune(character)
	}
	if escaped {
		decoded.WriteRune('\\')
	}

	return decoded.String()
}

// hasMatchingSearchValueQuotes reports whether a filter value is enclosed by the same supported quote character.
func hasMatchingSearchValueQuotes(value string) bool {
	if len(value) < 2 {
		return false
	}

	quote := value[0]
	return (quote == '\'' || quote == '"') && value[len(value)-1] == quote
}

// searchQueryBuilder owns SQL predicates, ranking, and positional arguments for page search.
type searchQueryBuilder struct {
	// args owns positional SQL arguments in append order.
	args queryArgs
	// where contains predicates joined into the final WHERE clause.
	where []string
	// rank is the SQL ranking expression used for selection and ordering.
	rank string
}

// newSearchQueryBuilder creates a page-search builder with optional full-text ranking.
func newSearchQueryBuilder(text string) *searchQueryBuilder {
	builder := &searchQueryBuilder{where: []string{"p.deleted_at IS NULL"}, rank: "0::real"}
	if text == "" {
		return builder
	}
	parameter := builder.args.add(text)
	builder.where = append(builder.where, "p.search_vector @@ websearch_to_tsquery('english', "+parameter+")")
	builder.rank = "ts_rank(p.search_vector, websearch_to_tsquery('english', " + parameter + "))"
	return builder
}

// applyFilters appends all supported field filters to the query in stable filter-kind order.
func (b *searchQueryBuilder) applyFilters(filters map[string][]string) {
	for _, value := range filters["tag"] {
		p := b.args.add(strings.ToLower(value))
		b.where = append(b.where, "EXISTS (SELECT 1 FROM page_tags x JOIN tags xt ON xt.id=x.tag_id WHERE x.page_id=p.id AND xt.name="+p+")")
	}
	for _, value := range filters["group"] {
		p := b.args.add(strings.ToLower(value))
		b.where = append(b.where, "EXISTS (SELECT 1 FROM page_groups pg JOIN wiki_groups g ON g.id=pg.group_id WHERE pg.page_id=p.id AND lower(g.name)="+p+")")
	}
	b.applyLikeFilters(filters["title"], "p.title ILIKE ", "%", "%")
	b.applyLikeFilters(filters["namespace"], "p.slug ILIKE ", "", "/%")
	for _, value := range filters["author"] {
		p := b.args.add("%" + value + "%")
		b.where = append(b.where, "(u.username ILIKE "+p+" OR u.display_name ILIKE "+p+")")
	}
	for _, value := range filters["status"] {
		p := b.args.add(strings.ToLower(value))
		b.where = append(b.where, "lower(p.status)="+p)
	}
	for _, value := range filters["owner"] {
		p := b.args.add(strings.ToLower(value))
		b.where = append(b.where, "EXISTS (SELECT 1 FROM wiki_groups og WHERE og.id=p.owner_group_id AND lower(og.name)="+p+")")
	}
	for _, value := range filters["property"] {
		b.applyPropertyFilter(value)
	}
}

// applyLikeFilters appends simple ILIKE filters using a caller-provided value shape.
func (b *searchQueryBuilder) applyLikeFilters(values []string, expression, prefix, suffix string) {
	for _, value := range values {
		b.where = append(b.where, expression+b.args.add(prefix+value+suffix))
	}
}

// applyPropertyFilter appends one structured property key/value predicate when syntactically valid.
func (b *searchQueryBuilder) applyPropertyFilter(value string) {
	key, propertyValue, ok := strings.Cut(value, "=")
	key = strings.TrimSpace(key)
	if !ok || key == "" {
		return
	}
	keyParam := b.args.add(strings.ToLower(key))
	valueParam := b.args.add("%" + strings.TrimSpace(propertyValue) + "%")
	b.where = append(b.where, "EXISTS (SELECT 1 FROM page_properties pp WHERE pp.page_id=p.id AND lower(pp.key)="+keyParam+" AND pp.value ILIKE "+valueParam+")")
}

// sql renders the final search statement and appends pagination arguments.
func (b *searchQueryBuilder) sql(limit, offset int) string {
	limitParam := b.args.add(limit)
	offsetParam := b.args.add(offset)
	return `
SELECT p.id,p.slug,p.title,coalesce(max(ni.icon),''),p.markdown_content,coalesce(p.created_by,0),coalesce(p.updated_by,0),coalesce(u.display_name,u.username,''),p.created_at,p.updated_at,p.view_count,coalesce(array_agg(t.name ORDER BY t.name) FILTER (WHERE t.name IS NOT NULL),'{}'),p.status,` + b.rank + `
FROM pages p
LEFT JOIN navigation_icons ni ON ni.path=p.slug
LEFT JOIN users u ON u.id=p.updated_by
LEFT JOIN page_tags pt ON pt.page_id=p.id
LEFT JOIN tags t ON t.id=pt.tag_id
WHERE ` + strings.Join(b.where, " AND ") + `
GROUP BY p.id,u.id
ORDER BY ` + b.rank + ` DESC,p.updated_at DESC,p.id DESC
LIMIT ` + limitParam + ` OFFSET ` + offsetParam
}

// scanSearchPages decodes page-search rows into domain pages.
func scanSearchPages(rows pgx.Rows) ([]domain.Page, error) {
	var pages []domain.Page
	for rows.Next() {
		var page domain.Page
		if err := rows.Scan(&page.ID, &page.Slug, &page.Title, &page.Icon, &page.Markdown, &page.CreatedBy, &page.UpdatedBy, &page.Author, &page.CreatedAt, &page.UpdatedAt, &page.ViewCount, &page.Tags, &page.Status, &page.Rank); err != nil {
			return nil, err
		}
		pages = append(pages, page)
	}
	return pages, rows.Err()
}

// RecentViewed returns a user's recently viewed pages.
func (s *Store) RecentViewed(ctx context.Context, userID int64, limit int) ([]domain.Page, error) {
	rows, err := s.pool.Query(
		ctx,
		pageSelect+`
JOIN page_views v ON v.page_id=p.id AND v.user_id=$1
WHERE p.deleted_at IS NULL
GROUP BY p.id,u.id,v.viewed_at
ORDER BY v.viewed_at DESC
LIMIT $2`,
		userID,
		limit,
	)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	return collectPages(rows)
}

// Popular returns the most-viewed pages.
func (s *Store) Popular(ctx context.Context, limit int) ([]domain.Page, error) {
	rows, err := s.pool.Query(
		ctx,
		pageSelect+`
WHERE p.deleted_at IS NULL
GROUP BY p.id,u.id
ORDER BY p.view_count DESC,p.updated_at DESC
LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	return collectPages(rows)
}

// searchTokens splits a search query while preserving quoted segments and their delimiters.
func searchTokens(query string) []string {
	var tokens []string
	var current strings.Builder
	var quote rune
	escaped := false

	for _, r := range query {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}
		if quote != 0 {
			current.WriteRune(r)
			if r == '\\' {
				escaped = true
				continue
			}
			if r == quote {
				quote = 0
			}
			continue
		}
		if r == '"' || r == '\'' {
			quote = r
			current.WriteRune(r)
			continue
		}
		if unicode.IsSpace(r) {
			tokens = flushSearchToken(tokens, &current)
			continue
		}

		current.WriteRune(r)
	}
	if escaped {
		current.WriteRune('\\')
	}

	tokens = flushSearchToken(tokens, &current)

	return tokens
}

// queryArgs owns SQL parameters and returns their positional placeholder.
type queryArgs []any

// add appends one search argument and returns its SQL placeholder.
func (a *queryArgs) add(value any) string {
	*a = append(*a, value)

	return fmt.Sprintf("$%d", len(*a))
}

// flushSearchToken appends the current token and resets its builder.
func flushSearchToken(tokens []string, current *strings.Builder) []string {
	if current.Len() == 0 {
		return tokens
	}

	tokens = append(tokens, current.String())
	current.Reset()

	return tokens
}
