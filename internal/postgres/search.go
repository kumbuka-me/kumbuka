package postgres

import (
	"context"
	"fmt"
	"strings"
	"unicode"

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
	var textTerms []string
	filters := map[string][]string{}

	for _, token := range searchTokens(query) {
		key, value, found := strings.Cut(token, ":")
		key = strings.ToLower(key)

		if found && value != "" && isSearchFilter(key) {
			filters[key] = append(filters[key], value)
		} else {
			textTerms = append(textTerms, token)
		}
	}

	args := queryArgs{}
	where := []string{"p.deleted_at IS NULL"}

	text := strings.Join(textTerms, " ")
	rank := "0::real"

	if text != "" {
		p := args.add(text)
		where = append(where, "p.search_vector @@ websearch_to_tsquery('english', "+p+")")
		rank = "ts_rank(p.search_vector, websearch_to_tsquery('english', " + p + "))"
	}

	for _, v := range filters["tag"] {
		p := args.add(strings.ToLower(v))
		where = append(
			where,
			"EXISTS (SELECT 1 FROM page_tags x JOIN tags xt ON xt.id=x.tag_id WHERE x.page_id=p.id AND xt.name="+p+")",
		)
	}
	for _, v := range filters["group"] {
		p := args.add(strings.ToLower(v))
		where = append(
			where,
			"EXISTS (SELECT 1 FROM page_groups pg JOIN wiki_groups g ON g.id=pg.group_id WHERE pg.page_id=p.id AND lower(g.name)="+p+")",
		)
	}
	for _, v := range filters["title"] {
		p := args.add("%" + v + "%")
		where = append(where, "p.title ILIKE "+p)
	}
	for _, v := range filters["namespace"] {
		p := args.add(v + "/%")
		where = append(where, "p.slug ILIKE "+p)
	}
	for _, v := range filters["author"] {
		p := args.add("%" + v + "%")
		where = append(where, "(u.username ILIKE "+p+" OR u.display_name ILIKE "+p+")")
	}

	for _, v := range filters["status"] {
		p := args.add(strings.ToLower(v))
		where = append(where, "lower(p.status)="+p)
	}
	for _, v := range filters["owner"] {
		p := args.add(strings.ToLower(v))
		where = append(where, "EXISTS (SELECT 1 FROM wiki_groups og WHERE og.id=p.owner_group_id AND lower(og.name)="+p+")")
	}
	for _, v := range filters["property"] {
		key, value, ok := strings.Cut(v, "=")
		if !ok || strings.TrimSpace(key) == "" {
			continue
		}

		keyParam := args.add(strings.ToLower(strings.TrimSpace(key)))
		valueParam := args.add("%" + strings.TrimSpace(value) + "%")
		where = append(where, "EXISTS (SELECT 1 FROM page_properties pp WHERE pp.page_id=p.id AND lower(pp.key)="+keyParam+" AND pp.value ILIKE "+valueParam+")")
	}

	limitParam := args.add(limit)
	sql := `
SELECT p.id,p.slug,p.title,coalesce(max(ni.icon),''),p.markdown_content,coalesce(p.created_by,0),coalesce(p.updated_by,0),coalesce(u.display_name,u.username,''),p.created_at,p.updated_at,p.view_count,coalesce(array_agg(t.name ORDER BY t.name) FILTER (WHERE t.name IS NOT NULL),'{}'),p.status,` + rank + `
FROM pages p
LEFT JOIN navigation_icons ni ON ni.path=p.slug
LEFT JOIN users u ON u.id=p.updated_by
LEFT JOIN page_tags pt ON pt.page_id=p.id
LEFT JOIN tags t ON t.id=pt.tag_id
WHERE ` + strings.Join(where, " AND ") + `
GROUP BY p.id,u.id
ORDER BY ` + rank + ` DESC,p.updated_at DESC
LIMIT ` + limitParam
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var out []domain.Page

	for rows.Next() {
		var p domain.Page
		if err := rows.Scan(&p.ID, &p.Slug, &p.Title, &p.Icon, &p.Markdown, &p.CreatedBy, &p.UpdatedBy, &p.Author, &p.CreatedAt, &p.UpdatedAt, &p.ViewCount, &p.Tags, &p.Status, &p.Rank); err != nil {
			return nil, err
		}

		out = append(out, p)
	}

	return out, rows.Err()
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

// searchTokens splits a search query while preserving whitespace inside quoted filter values.
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
		if r == '\\' && quote != 0 {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				current.WriteRune(r)
			}
			continue
		}
		if r == '"' || r == '\'' {
			quote = r
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
