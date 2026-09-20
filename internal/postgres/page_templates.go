package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

const pageTemplateSelect = `
SELECT id,name,description,markdown_content,path_prefix,icon,tags,status,
       coalesce(owner_group_id,0),review_interval_days,properties,fields
FROM page_templates`

// PageTemplates returns reusable blueprints in name order.
func (s *Store) PageTemplates(ctx context.Context) ([]domain.PageTemplate, error) {
	rows, err := s.pool.Query(ctx, pageTemplateSelect+`
ORDER BY lower(name),id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var templates []domain.PageTemplate
	for rows.Next() {
		item, err := scanPageTemplate(rows)
		if err != nil {
			return nil, err
		}
		templates = append(templates, item)
	}
	return templates, rows.Err()
}

// PageTemplate returns one reusable page blueprint.
func (s *Store) PageTemplate(ctx context.Context, id int64) (domain.PageTemplate, error) {
	item, err := scanPageTemplate(s.pool.QueryRow(ctx, pageTemplateSelect+`
WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.PageTemplate{}, domain.ErrNotFound
	}
	return item, err
}

// CreatePageTemplate creates a reusable page blueprint.
func (s *Store) CreatePageTemplate(ctx context.Context, item domain.PageTemplate) (domain.PageTemplate, error) {
	properties, fields, err := templateJSON(item)
	if err != nil {
		return domain.PageTemplate{}, err
	}

	row := s.pool.QueryRow(ctx, `
INSERT INTO page_templates(
  name,description,markdown_content,path_prefix,icon,tags,status,
  owner_group_id,review_interval_days,properties,fields
)
VALUES($1,$2,$3,$4,$5,$6,$7,nullif($8,0),$9,$10,$11)
RETURNING id,name,description,markdown_content,path_prefix,icon,tags,status,
          coalesce(owner_group_id,0),review_interval_days,properties,fields`,
		item.Name, item.Description, item.Markdown, item.PathPrefix, item.Icon, item.Tags,
		item.Status, item.OwnerGroupID, item.ReviewIntervalDays, properties, fields,
	)
	created, err := scanPageTemplate(row)
	return created, mutationError(err)
}

// UpdatePageTemplate updates a reusable page blueprint.
func (s *Store) UpdatePageTemplate(ctx context.Context, id int64, item domain.PageTemplate) error {
	properties, fields, err := templateJSON(item)
	if err != nil {
		return err
	}

	tag, err := s.pool.Exec(ctx, `
UPDATE page_templates
SET name=$2,description=$3,markdown_content=$4,path_prefix=$5,icon=$6,tags=$7,status=$8,
    owner_group_id=nullif($9,0),review_interval_days=$10,properties=$11,fields=$12,updated_at=now()
WHERE id=$1`, id, item.Name, item.Description, item.Markdown, item.PathPrefix, item.Icon, item.Tags,
		item.Status, item.OwnerGroupID, item.ReviewIntervalDays, properties, fields)
	if err == nil && tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return mutationError(err)
}

// DeletePageTemplate removes a reusable page template.
func (s *Store) DeletePageTemplate(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `
DELETE FROM page_templates
WHERE id=$1`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return err
}

type templateRow interface {
	Scan(...any) error
}

// scanPageTemplate decodes one page-blueprint row and its JSON metadata.
func scanPageTemplate(row templateRow) (domain.PageTemplate, error) {
	var item domain.PageTemplate
	var properties, fields []byte

	if err := row.Scan(
		&item.ID, &item.Name, &item.Description, &item.Markdown, &item.PathPrefix, &item.Icon,
		&item.Tags, &item.Status, &item.OwnerGroupID, &item.ReviewIntervalDays, &properties, &fields,
	); err != nil {
		return domain.PageTemplate{}, err
	}

	item.Properties = map[string]string{}

	if err := decodeTemplateJSON(properties, &item.Properties); err != nil {
		return domain.PageTemplate{}, err
	}
	if err := decodeTemplateJSON(fields, &item.Fields); err != nil {
		return domain.PageTemplate{}, err
	}

	return item, nil
}

// decodeTemplateJSON ignores empty database values and decodes populated JSON values.
func decodeTemplateJSON(data []byte, target any) error {
	if len(data) == 0 {
		return nil
	}

	return json.Unmarshal(data, target)
}

// templateJSON encodes blueprint properties and prompted fields for persistence.
func templateJSON(item domain.PageTemplate) ([]byte, []byte, error) {
	properties, err := json.Marshal(item.Properties)
	if err != nil {
		return nil, nil, err
	}
	fields, err := json.Marshal(item.Fields)
	if err != nil {
		return nil, nil, err
	}
	return properties, fields, nil
}
