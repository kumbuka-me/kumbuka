package service

import (
	"context"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// preferenceRepository contains per-user preference operations.
type preferenceRepository interface {
	Preferences(context.Context, int64) (domain.UserPreferences, error)
	SavePreferences(context.Context, int64, domain.UserPreferences) error
	SetShowPageContents(context.Context, int64, bool) error
	SetExpandedNavigation(context.Context, int64, []string) error
	SetSidebarWidth(context.Context, int64, int) error
}

// Preferences exposes per-user display preference use cases.
type Preferences struct{ repository preferenceRepository }

// NewPreferences constructs the user preference service.
func NewPreferences(repository preferenceRepository) *Preferences {
	return &Preferences{repository: repository}
}

// Preferences returns the saved preferences for a user.
func (s *Preferences) Preferences(ctx context.Context, userID int64) (domain.UserPreferences, error) {
	return s.repository.Preferences(ctx, userID)
}

// SavePreferences replaces the saved preferences for a user.
func (s *Preferences) SavePreferences(
	ctx context.Context,
	userID int64,
	preferences domain.UserPreferences,
) error {
	if !domain.ValidNavigationStyle(preferences.NavigationStyle) {
		return domain.NewValidationError("navigation_style", "Choose a valid navigation style.")
	}
	if !domain.ValidNavigationDensity(preferences.NavigationDensity) {
		return domain.NewValidationError("navigation_density", "Choose a valid navigation density.")
	}
	if preferences.TypographySize != "" && !domain.ValidTypographySize(preferences.TypographySize) {
		return domain.NewValidationError("typography_size", "Choose a valid typography size.")
	}
	if !domain.ValidSidebarWidth(preferences.SidebarWidth) {
		return domain.NewValidationError("sidebar_width", "Sidebar width is out of range.")
	}
	return s.repository.SavePreferences(ctx, userID, preferences)
}

// SetShowPageContents updates a user's page-contents visibility preference.
func (s *Preferences) SetShowPageContents(ctx context.Context, userID int64, show bool) error {
	return s.repository.SetShowPageContents(ctx, userID, show)
}

// SetExpandedNavigation updates the navigation paths expanded by a user.
func (s *Preferences) SetExpandedNavigation(
	ctx context.Context,
	userID int64,
	expanded []string,
) error {
	return s.repository.SetExpandedNavigation(ctx, userID, expanded)
}

// SetSidebarWidth updates a user's preferred sidebar width.
func (s *Preferences) SetSidebarWidth(ctx context.Context, userID int64, width int) error {
	if !domain.ValidSidebarWidth(width) {
		return domain.NewValidationError("sidebar_width", "Sidebar width is out of range.")
	}
	return s.repository.SetSidebarWidth(ctx, userID, width)
}
