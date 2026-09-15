package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/kumbuka-me/kumbuka/internal/domain"
)

const webhookSelect = `
SELECT id,name,url,events,body_template,retry_enabled,retry_count,retry_backoff_ms,retry_max_backoff_ms,retry_jitter,enabled,created_at,updated_at
FROM webhooks`

type webhookRow interface {
	Scan(...any) error
}

type webhookHeaderQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

// Webhooks returns all outgoing webhook configurations in name order.
func (s *Store) Webhooks(ctx context.Context) ([]domain.Webhook, error) {
	rows, err := s.pool.Query(ctx, webhookSelect+`
ORDER BY lower(name),id`)
	if err != nil {
		return nil, err
	}

	var result []domain.Webhook

	for rows.Next() {
		item, err := scanWebhook(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}

		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	for index := range result {
		headers, err := webhookHeaders(ctx, s.pool, result[index].ID)
		if err != nil {
			return nil, err
		}
		result[index].Headers = headers
	}

	return result, nil
}

// Webhook returns one outgoing webhook by identifier.
func (s *Store) Webhook(ctx context.Context, id int64) (domain.Webhook, error) {
	item, err := scanWebhook(s.pool.QueryRow(ctx, webhookSelect+`
WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Webhook{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Webhook{}, err
	}

	item.Headers, err = webhookHeaders(ctx, s.pool, item.ID)
	return item, err
}

// SaveWebhook creates or replaces one outgoing webhook.
func (s *Store) SaveWebhook(ctx context.Context, id int64, item domain.Webhook) (domain.Webhook, error) {
	if id == 0 {
		return s.createWebhook(ctx, item)
	}

	return s.updateWebhook(ctx, id, item)
}

// createWebhook inserts one outgoing webhook and returns the persisted row.
func (s *Store) createWebhook(ctx context.Context, item domain.Webhook) (domain.Webhook, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.Webhook{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	row := tx.QueryRow(ctx, `
INSERT INTO webhooks(
  name,url,events,body_template,retry_enabled,retry_count,retry_backoff_ms,retry_max_backoff_ms,retry_jitter,enabled
)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
RETURNING id,name,url,events,body_template,retry_enabled,retry_count,retry_backoff_ms,retry_max_backoff_ms,retry_jitter,enabled,created_at,updated_at`,
		item.Name,
		item.URL,
		item.Events,
		item.BodyTemplate,
		item.RetryEnabled,
		item.RetryCount,
		item.RetryBackoff.Milliseconds(),
		item.RetryMaxBackoff.Milliseconds(),
		item.RetryJitter,
		item.Enabled,
	)

	saved, err := scanWebhook(row)
	if err != nil {
		return domain.Webhook{}, mutationError(err)
	}
	if err := replaceWebhookHeaders(ctx, tx, saved.ID, item.Headers); err != nil {
		return domain.Webhook{}, mutationError(err)
	}
	if saved.Headers, err = webhookHeaders(ctx, tx, saved.ID); err != nil {
		return domain.Webhook{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Webhook{}, mutationError(err)
	}

	return saved, nil
}

// updateWebhook replaces one outgoing webhook and returns the persisted row.
func (s *Store) updateWebhook(ctx context.Context, id int64, item domain.Webhook) (domain.Webhook, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.Webhook{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	row := tx.QueryRow(ctx, `
UPDATE webhooks
SET name=$2,
    url=$3,
    events=$4,
    body_template=$5,
    retry_enabled=$6,
    retry_count=$7,
    retry_backoff_ms=$8,
    retry_max_backoff_ms=$9,
    retry_jitter=$10,
    enabled=$11,
    updated_at=now()
WHERE id=$1
RETURNING id,name,url,events,body_template,retry_enabled,retry_count,retry_backoff_ms,retry_max_backoff_ms,retry_jitter,enabled,created_at,updated_at`,
		id,
		item.Name,
		item.URL,
		item.Events,
		item.BodyTemplate,
		item.RetryEnabled,
		item.RetryCount,
		item.RetryBackoff.Milliseconds(),
		item.RetryMaxBackoff.Milliseconds(),
		item.RetryJitter,
		item.Enabled,
	)

	saved, err := scanWebhook(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Webhook{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Webhook{}, mutationError(err)
	}
	if err := replaceWebhookHeaders(ctx, tx, saved.ID, item.Headers); err != nil {
		return domain.Webhook{}, mutationError(err)
	}
	if saved.Headers, err = webhookHeaders(ctx, tx, saved.ID); err != nil {
		return domain.Webhook{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Webhook{}, mutationError(err)
	}

	return saved, nil
}

// scanWebhook decodes one outgoing webhook row.
func scanWebhook(row webhookRow) (domain.Webhook, error) {
	var (
		item         domain.Webhook
		backoffMS    int64
		maxBackoffMS int64
	)

	err := row.Scan(
		&item.ID,
		&item.Name,
		&item.URL,
		&item.Events,
		&item.BodyTemplate,
		&item.RetryEnabled,
		&item.RetryCount,
		&backoffMS,
		&maxBackoffMS,
		&item.RetryJitter,
		&item.Enabled,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	item.RetryBackoff = time.Duration(backoffMS) * time.Millisecond
	item.RetryMaxBackoff = time.Duration(maxBackoffMS) * time.Millisecond

	return item, err
}

// webhookHeaders returns request headers configured for one webhook.
func webhookHeaders(ctx context.Context, querier webhookHeaderQuerier, webhookID int64) ([]domain.WebhookHeader, error) {
	rows, err := querier.Query(ctx, `
SELECT id,name,value,sensitive
FROM webhook_headers
WHERE webhook_id=$1
ORDER BY lower(name),id`, webhookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []domain.WebhookHeader
	for rows.Next() {
		var header domain.WebhookHeader
		if err := rows.Scan(&header.ID, &header.Name, &header.Value, &header.Sensitive); err != nil {
			return nil, err
		}
		result = append(result, header)
	}

	return result, rows.Err()
}

// replaceWebhookHeaders replaces all request headers belonging to one webhook.
func replaceWebhookHeaders(ctx context.Context, tx pgx.Tx, webhookID int64, headers []domain.WebhookHeader) error {
	if _, err := tx.Exec(ctx, `DELETE FROM webhook_headers WHERE webhook_id=$1`, webhookID); err != nil {
		return err
	}

	for _, header := range headers {
		if _, err := tx.Exec(ctx, `
INSERT INTO webhook_headers(webhook_id,name,value,sensitive)
VALUES($1,$2,$3,$4)`, webhookID, header.Name, header.Value, header.Sensitive); err != nil {
			return err
		}
	}

	return nil
}

// DeleteWebhook removes one outgoing webhook.
func (s *Store) DeleteWebhook(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM webhooks WHERE id=$1`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}

	return err
}

// AddWebhookDelivery records the outcome of one outgoing delivery.
func (s *Store) AddWebhookDelivery(ctx context.Context, webhookID int64, event string, statusCode, attempts int, message string) error {
	_, err := s.pool.Exec(ctx, `
INSERT INTO webhook_deliveries(webhook_id,event,status_code,attempts,error)
VALUES($1,$2,$3,$4,$5)`, webhookID, event, statusCode, attempts, message)

	return err
}

// WebhookDeliveries returns the newest outgoing delivery outcomes.
func (s *Store) WebhookDeliveries(ctx context.Context, limit int) ([]domain.WebhookDelivery, error) {
	rows, err := s.pool.Query(ctx, `
SELECT d.id,d.webhook_id,w.name,d.event,d.status_code,d.attempts,d.error,d.created_at
FROM webhook_deliveries d
JOIN webhooks w ON w.id=d.webhook_id
ORDER BY d.created_at DESC,d.id DESC
LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []domain.WebhookDelivery

	for rows.Next() {
		var item domain.WebhookDelivery

		if err := rows.Scan(
			&item.ID,
			&item.WebhookID,
			&item.WebhookName,
			&item.Event,
			&item.StatusCode,
			&item.Attempts,
			&item.Error,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}

		result = append(result, item)
	}

	return result, rows.Err()
}
