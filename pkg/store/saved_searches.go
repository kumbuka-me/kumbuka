package store

import (
	"context"
	"strings"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// SavedSearches returns a user's named searches.
func (s *Store) SavedSearches(ctx context.Context, userID int64) ([]domain.SavedSearch, error) {
	rows, err := s.pool.Query(ctx, `
SELECT id,name,query,pinned
FROM saved_searches
WHERE user_id=$1
ORDER BY pinned DESC,lower(name),id`, userID)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var items []domain.SavedSearch

	for rows.Next() {
		var item domain.SavedSearch
		if err := rows.Scan(&item.ID, &item.Name, &item.Query, &item.Pinned); err != nil {
			return nil, err
		}

		items = append(items, item)
	}

	return items, rows.Err()
}

// SaveSavedSearch creates or updates a named search.
func (s *Store) SaveSavedSearch(ctx context.Context, userID, id int64, name, query string, pinned bool) error {
	name = strings.TrimSpace(name)
	query = strings.TrimSpace(query)
	if name == "" {
		return domain.NewValidationError("name", "A saved search name is required.")
	}
	if query == "" {
		return domain.NewValidationError("query", "A search query is required.")
	}
	if id == 0 {
		_, err := s.pool.Exec(ctx, `
INSERT INTO saved_searches(user_id,name,query,pinned)
VALUES($1,$2,$3,$4)`, userID, name, query, pinned)

		return mutationError(err)
	}

	tag, err := s.pool.Exec(ctx, `
UPDATE saved_searches
SET name=$3,query=$4,pinned=$5
WHERE id=$1 AND user_id=$2`, id, userID, name, query, pinned)
	if err == nil && tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}

	return mutationError(err)
}

// DeleteSavedSearch deletes one named search owned by a user.
func (s *Store) DeleteSavedSearch(ctx context.Context, userID, id int64) error {
	tag, err := s.pool.Exec(ctx, `
DELETE FROM saved_searches
WHERE id=$1 AND user_id=$2`, id, userID)
	if err == nil && tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}

	return err
}
