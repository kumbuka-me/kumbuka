package users

import (
	"context"
	"fmt"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// UserUpdateInput describes the administrator's intended account change.
// Password confirmation belongs to the form; password policy belongs to this operation.
type UserUpdateInput struct {
	// UserID identifies the account being changed.
	UserID int64
	// Actor is the administrator requesting the account change.
	Actor domain.User
	// Role is the requested account role.
	Role string
	// Enabled is the requested account-enabled state.
	Enabled bool
	// GroupIDs replaces the account group memberships.
	GroupIDs []int64
	// Password optionally replaces the local recovery password.
	Password string
	// UpdateLocalCredential reports whether the recovery-credential enabled state should change.
	UpdateLocalCredential bool
	// LocalCredentialEnabled is the requested recovery-credential state when no new password is supplied.
	LocalCredentialEnabled bool
	// AuthModeOverride is the deployment-managed authentication mode, when configured.
	AuthModeOverride string
}

// UpdateAccount validates the complete operation before submitting one atomic mutation.
func (s *Users) UpdateAccount(ctx context.Context, input UserUpdateInput) error {
	if !input.Actor.IsAdministrator() {
		return domain.ErrForbidden
	}
	if input.UserID <= 0 {
		return domain.NewValidationError("user_id", "Choose a valid user.")
	}
	if !domain.ValidUserRole(input.Role) {
		return domain.NewValidationError("role", "Choose a valid user role.")
	}
	if input.UserID == input.Actor.ID {
		if input.Role != domain.UserRoleAdmin {
			return domain.NewValidationError("role", "You cannot remove your own administrator role.")
		}
		if !input.Enabled {
			return domain.NewValidationError("account_enabled", "You cannot disable your own account.")
		}
	}
	for _, id := range input.GroupIDs {
		if id <= 0 {
			return domain.NewValidationError("group_id", "Choose a valid group.")
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
		if !domain.IsExternalAuthMode(domain.AuthMode(mode)) {
			return domain.NewValidationError("local_credential_enabled", "Local recovery credentials can only be enabled or disabled while external authentication is active.")
		}
		enabled := input.Password != "" || input.LocalCredentialEnabled
		update.LocalCredentialEnabled = &enabled
	}
	if input.Password != "" {
		if s.passwords == nil {
			return fmt.Errorf("hash account password: password service is not configured")
		}
		if problem := s.passwords.Problem(input.Password); problem != "" {
			return domain.NewValidationError("local_password", problem)
		}
		hash, err := s.passwords.Hash(input.Password)
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
