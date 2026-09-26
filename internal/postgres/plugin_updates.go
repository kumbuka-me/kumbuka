package postgres

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// ClaimPluginUpdateAnnouncements records newly seen plugin releases and returns only releases not announced before.
func (s *Store) ClaimPluginUpdateAnnouncements(ctx context.Context, updates []domain.PluginUpdateNotice) ([]domain.PluginUpdateNotice, error) {
	if len(updates) == 0 {
		return nil, nil
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	pending := make([]domain.PluginUpdateNotice, 0, len(updates))
	for _, update := range updates {
		if !validPluginUpdateNotice(update) {
			continue
		}

		var inserted string
		err := tx.QueryRow(ctx, `
INSERT INTO plugin_update_announcements(plugin_id,version)
VALUES($1,$2)
ON CONFLICT DO NOTHING
RETURNING plugin_id`, update.ID, update.AvailableVersion).Scan(&inserted)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		pending = append(pending, update)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return pending, nil
}

// EnabledAdministratorIDs returns enabled administrator account identifiers in stable order.
func (s *Store) EnabledAdministratorIDs(ctx context.Context) ([]int64, error) {
	rows, err := s.pool.Query(ctx, `
SELECT id
FROM users
WHERE enabled AND role='admin'
ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// validPluginUpdateNotice reports whether an update contains both a plugin identifier and available version.
func validPluginUpdateNotice(update domain.PluginUpdateNotice) bool {
	return strings.TrimSpace(update.ID) != "" && strings.TrimSpace(update.AvailableVersion) != ""
}
