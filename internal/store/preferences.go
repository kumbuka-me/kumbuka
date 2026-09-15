package store

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/kumbuka-me/kumbuka/internal/domain"
)

// Preferences returns a user's stored preferences or defaults when none exist yet.
func (s *Store) Preferences(ctx context.Context, userID int64) (domain.UserPreferences, error) {
	preferences := domain.DefaultUserPreferences()
	err := s.pool.QueryRow(ctx, `
SELECT
  theme,
  show_page_contents,
  navigation_style,
  navigation_density,
  typography_size,
  sidebar_width,
  show_navigation_guides,
  remember_navigation_state,
  show_navigation_page_counts,
  expanded_navigation,
  hidden_plugin_widgets
FROM user_preferences
WHERE user_id=$1`, userID).Scan(
		&preferences.Theme,
		&preferences.ShowPageContents,
		&preferences.NavigationStyle,
		&preferences.NavigationDensity,
		&preferences.TypographySize,
		&preferences.SidebarWidth,
		&preferences.ShowNavigationGuides,
		&preferences.RememberNavigationState,
		&preferences.ShowNavigationPageCounts,
		&preferences.ExpandedNavigation,
		&preferences.HiddenPluginWidgets,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return preferences, nil
	}

	return preferences, err
}

// SavePreferences creates or updates all presentation preferences for a user.
func (s *Store) SavePreferences(ctx context.Context, userID int64, preferences domain.UserPreferences) error {
	_, err := s.pool.Exec(ctx, `
INSERT INTO user_preferences(
  user_id,
  theme,
  show_page_contents,
  navigation_style,
  navigation_density,
  typography_size,
  sidebar_width,
  show_navigation_guides,
  remember_navigation_state,
  show_navigation_page_counts,
  expanded_navigation,
  hidden_plugin_widgets,
  updated_at
)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,now())
ON CONFLICT(user_id) DO UPDATE
SET theme=EXCLUDED.theme,
    show_page_contents=EXCLUDED.show_page_contents,
    navigation_style=EXCLUDED.navigation_style,
    navigation_density=EXCLUDED.navigation_density,
    typography_size=EXCLUDED.typography_size,
    sidebar_width=EXCLUDED.sidebar_width,
    show_navigation_guides=EXCLUDED.show_navigation_guides,
    remember_navigation_state=EXCLUDED.remember_navigation_state,
    show_navigation_page_counts=EXCLUDED.show_navigation_page_counts,
    expanded_navigation=EXCLUDED.expanded_navigation,
    hidden_plugin_widgets=EXCLUDED.hidden_plugin_widgets,
    updated_at=now()`, userID, preferences.Theme, preferences.ShowPageContents, preferences.NavigationStyle, preferences.NavigationDensity, preferences.TypographySize, preferences.SidebarWidth, preferences.ShowNavigationGuides, preferences.RememberNavigationState, preferences.ShowNavigationPageCounts, normalizeNavigationPaths(preferences.ExpandedNavigation), preferences.HiddenPluginWidgets)
	return err
}

// SetShowPageContents updates only the page-contents preference and preserves every other user preference.
func (s *Store) SetShowPageContents(ctx context.Context, userID int64, show bool) error {
	_, err := s.pool.Exec(ctx, `
INSERT INTO user_preferences(user_id,show_page_contents,updated_at)
VALUES($1,$2,now())
ON CONFLICT(user_id) DO UPDATE
SET show_page_contents=EXCLUDED.show_page_contents,
    updated_at=now()`, userID, show)
	return err
}

// SetExpandedNavigation stores the user's explicitly expanded navigation folders.
func (s *Store) SetExpandedNavigation(ctx context.Context, userID int64, expanded []string) error {
	_, err := s.pool.Exec(ctx, `
INSERT INTO user_preferences(user_id,expanded_navigation,updated_at)
VALUES($1,$2,now())
ON CONFLICT(user_id) DO UPDATE
SET expanded_navigation=EXCLUDED.expanded_navigation,
    updated_at=now()`, userID, normalizeNavigationPaths(expanded))
	return err
}

// SetSidebarWidth stores a validated desktop sidebar width without changing other preferences.
func (s *Store) SetSidebarWidth(ctx context.Context, userID int64, width int) error {
	if !domain.ValidSidebarWidth(width) {
		return domain.NewValidationError("sidebar_width", "Sidebar width is out of range.")
	}

	_, err := s.pool.Exec(ctx, `
INSERT INTO user_preferences(user_id,sidebar_width,updated_at)
VALUES($1,$2,now())
ON CONFLICT(user_id) DO UPDATE
SET sidebar_width=EXCLUDED.sidebar_width,
    updated_at=now()`, userID, width)

	return err
}

// normalizeNavigationPaths trims and deduplicates persisted folder slugs while preserving order.
func normalizeNavigationPaths(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))

	for _, value := range values {
		value = strings.Trim(strings.TrimSpace(value), "/")
		if value == "" || seen[value] {
			continue
		}

		seen[value] = true
		result = append(result, value)
	}

	return result
}
