package store

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// NotifyPluginUpdates creates one deduplicated inbox notification for every enabled administrator.
func (s *Store) NotifyPluginUpdates(ctx context.Context, updates []domain.PluginUpdateNotice) error {
	if len(updates) == 0 {
		return nil
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var administrators bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS(
  SELECT 1
  FROM users
  WHERE enabled AND role='admin'
)`).Scan(&administrators); err != nil {
		return err
	}
	if !administrators {
		return tx.Commit(ctx)
	}

	pending := make([]domain.PluginUpdateNotice, 0, len(updates))
	for _, update := range updates {
		if strings.TrimSpace(update.ID) == "" || strings.TrimSpace(update.AvailableVersion) == "" {
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
			return err
		}
		pending = append(pending, update)
	}
	if len(pending) == 0 {
		return tx.Commit(ctx)
	}

	sort.Slice(pending, func(i, j int) bool {
		return strings.ToLower(pending[i].Name) < strings.ToLower(pending[j].Name)
	})
	title, body := pluginUpdateNotification(pending)
	tag, err := tx.Exec(ctx, `
INSERT INTO notifications(user_id,kind,title,body,url)
SELECT id,'plugin-update',$1,$2,'/admin/plugins'
FROM users
WHERE enabled AND role='admin'`, title, body)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return nil
	}

	return tx.Commit(ctx)
}

// pluginUpdateNotification formats one compact administrator notification for newly announced releases.
func pluginUpdateNotification(updates []domain.PluginUpdateNotice) (string, string) {
	if len(updates) == 1 {
		update := updates[0]
		return "Plugin update available", fmt.Sprintf(
			"%s %s is available; currently %s.",
			update.Name,
			update.AvailableVersion,
			update.CurrentVersion,
		)
	}

	entries := make([]string, 0, len(updates))
	for _, update := range updates {
		entries = append(entries, fmt.Sprintf(
			"%s %s -> %s",
			update.Name,
			update.CurrentVersion,
			update.AvailableVersion,
		))
	}

	return fmt.Sprintf("%d plugin updates available", len(updates)), strings.Join(entries, "; ")
}
