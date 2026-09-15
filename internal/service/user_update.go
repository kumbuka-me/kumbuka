package service

import (
	"context"
	"fmt"
	"github.com/kumbuka-me/kumbuka/internal/auth"
	"github.com/kumbuka-me/kumbuka/internal/domain"
)

// UserUpdateInput describes the administrator's intended account change.
// Password confirmation belongs to the form; password policy belongs to this operation.
type UserUpdateInput struct {
	UserID                 int64
	Actor                  domain.User
	Role                   string
	Enabled                bool
	GroupIDs               []int64
	Password               string
	UpdateLocalCredential  bool
	LocalCredentialEnabled bool
	AuthModeOverride       string
}

// UpdateAccount validates the complete operation before submitting one atomic mutation.
func (s *Users) UpdateAccount(ctx context.Context, input UserUpdateInput) error {
	if input.Actor.Role != "admin" {
		return domain.ErrForbidden
	}
	if input.UserID <= 0 {
		return newValidationError("user_id", "Choose a valid user.")
	}
	if !domain.ValidUserRole(input.Role) {
		return newValidationError("role", "Choose a valid user role.")
	}
	if input.UserID == input.Actor.ID {
		if input.Role != "admin" {
			return newValidationError("role", "You cannot remove your own administrator role.")
		}
		if !input.Enabled {
			return newValidationError("account_enabled", "You cannot disable your own account.")
		}
	}
	for _, id := range input.GroupIDs {
		if id <= 0 {
			return newValidationError("group_id", "Choose a valid group.")
		}
	}
	update := domain.UserAccountUpdate{UserID: input.UserID, Role: input.Role, Enabled: input.Enabled, GroupIDs: input.GroupIDs}
	if input.UpdateLocalCredential {
		mode := input.AuthModeOverride
		if mode == "" {
			settings, err := s.repository.ApplicationSettings(ctx)
			if err != nil {
				return fmt.Errorf("load authentication settings for account update: %w", err)
			}
			mode = settings.Authentication.Mode
		}
		if auth.AuthMode(mode) != auth.AuthModeOIDC && auth.AuthMode(mode) != auth.AuthModeTrustedProxy {
			return newValidationError("local_credential_enabled", "Local recovery credentials can only be enabled or disabled while external authentication is active.")
		}
		enabled := input.Password != "" || input.LocalCredentialEnabled
		update.LocalCredentialEnabled = &enabled
	}
	if input.Password != "" {
		if problem := auth.LocalPasswordProblem(input.Password); problem != "" {
			return newValidationError("local_password", problem)
		}
		hash, err := auth.HashLocalPassword(input.Password)
		if err != nil {
			return fmt.Errorf("hash account password: %w", err)
		}
		update.PasswordHash = hash
	}
	if err := s.repository.UpdateUserAccount(ctx, update); err != nil {
		return fmt.Errorf("update account: %w", err)
	}
	return nil
}
