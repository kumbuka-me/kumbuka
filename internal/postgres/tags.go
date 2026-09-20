package postgres

import (
	"context"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// TagInfos returns all stored tags with page usage counts.
func (s *Store) TagInfos(ctx context.Context) ([]domain.TagInfo, error) {
	rows, err := s.pool.Query(ctx, `
SELECT t.id,t.name,count(tp.id)
FROM tags t
LEFT JOIN page_tags pt ON pt.tag_id=t.id
LEFT JOIN pages tp ON tp.id=pt.page_id AND tp.deleted_at IS NULL
GROUP BY t.id
ORDER BY lower(t.name),t.id`)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var tags []domain.TagInfo

	for rows.Next() {
		var tag domain.TagInfo
		if err := rows.Scan(&tag.ID, &tag.Name, &tag.PageCount); err != nil {
			return nil, err
		}

		tags = append(tags, tag)
	}

	return tags, rows.Err()
}

// DeleteTag removes a tag and its page associations.
func (s *Store) DeleteTag(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `
DELETE FROM tags
WHERE id=$1`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}

	return err
}
