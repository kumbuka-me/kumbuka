package users

import (
	"context"
	"fmt"
	"net/mail"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/application/audit"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/utils"
)

// UserUpdateInput describes the administrator's intended account change. Password confirmation belongs to the form; password policy belongs to this operation.
type UserUpdateInput struct {
	// UserID identifies the account being changed.
	UserID int64
	// Actor is the administrator requesting the account change.
	Actor domain.User
	// Username optionally replaces the administrator-managed username.
	Username *string
	// Email optionally replaces the administrator-managed email address.
	Email *string
	// DisplayName optionally replaces the administrator-managed display name.
	DisplayName *string
	// RevertUsername restores the latest provider-managed username.
	RevertUsername bool
	// RevertEmail restores the latest provider-managed email.
	RevertEmail bool
	// RevertDisplayName restores the latest provider-managed display name.
	RevertDisplayName bool
	// Role is the requested account role.
	Role domain.UserRole
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
	AuthModeOverride domain.AuthMode
}

// changesProfile reports whether the request edits a profile value or restores provider ownership.
func (input UserUpdateInput) changesProfile() bool {
	return input.Username != nil || input.Email != nil || input.DisplayName != nil ||
		input.RevertUsername || input.RevertEmail || input.RevertDisplayName
}

// UpdateAccount validates the complete operation before submitting one atomic mutation.
func (s *Users) UpdateAccount(ctx context.Context, input UserUpdateInput) error {
	if err := validateAccountUpdate(input); err != nil {
		return err
	}

	update := domain.UserAccountUpdate{
		UserID:   input.UserID,
		Role:     input.Role,
		Enabled:  input.Enabled,
		GroupIDs: input.GroupIDs,
	}
	profileChanged := input.changesProfile()
	var profile domain.UserProfile
	if profileChanged {
		var err error
		profile, err = s.repository.UserProfile(ctx, input.UserID)
		if err != nil {
			return fmt.Errorf("load account profile: %w", err)
		}
	}
	if err := prepareProfileUpdate(input, profile, &update); err != nil {
		return err
	}
	if err := s.prepareLocalCredentialUpdate(ctx, input, &update); err != nil {
		return err
	}
	if err := s.preparePasswordUpdate(input.Password, &update); err != nil {
		return err
	}
	if err := s.repository.UpdateUserAccount(ctx, update); err != nil {
		return fmt.Errorf("update account: %w", err)
	}
	if profileChanged {
		s.recordProfileAudit(ctx, input.Actor.ID, profile, update)
	}
	return nil
}

// validateAccountUpdate validates authorization, identity, role, self-protection, and group membership inputs.
func validateAccountUpdate(input UserUpdateInput) error {
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
	return nil
}

// prepareProfileUpdate validates and normalizes an administrator-managed account profile.
func prepareProfileUpdate(input UserUpdateInput, current domain.UserProfile, update *domain.UserAccountUpdate) error {
	if !input.changesProfile() {
		return nil
	}
	if err := validateProfileUpdateOwnership(input, current); err != nil {
		return err
	}
	if err := prepareUsernameUpdate(input, current, update); err != nil {
		return err
	}
	if err := prepareEmailUpdate(input, current, update); err != nil {
		return err
	}
	prepareDisplayNameUpdate(input, current, update)

	update.RevertUsername = input.RevertUsername
	update.RevertEmail = input.RevertEmail
	update.RevertDisplayName = input.RevertDisplayName
	return nil
}

// validateProfileUpdateOwnership rejects profile operations that conflict with the active identity source.
func validateProfileUpdateOwnership(input UserUpdateInput, current domain.UserProfile) error {
	if current.Source == domain.ProfileSourceTrustedProxy && (input.Username != nil || input.RevertUsername) {
		return domain.NewValidationError("username", "Use the trusted-proxy relink action to change this identity.")
	}
	if current.Source == domain.ProfileSourceLocal && revertsProviderProfile(input) {
		return domain.NewValidationError("profile", "Local profile fields are not provider-managed.")
	}
	return nil
}

// revertsProviderProfile reports whether the update restores any provider-managed profile field.
func revertsProviderProfile(input UserUpdateInput) bool {
	return input.RevertUsername || input.RevertEmail || input.RevertDisplayName
}

// prepareUsernameUpdate validates and records a locally managed username replacement.
func prepareUsernameUpdate(input UserUpdateInput, current domain.UserProfile, update *domain.UserAccountUpdate) error {
	if input.Username == nil || input.RevertUsername {
		return nil
	}

	username := strings.TrimSpace(*input.Username)
	if username == "" {
		return domain.NewValidationError("username", "Username is required.")
	}
	if len([]rune(username)) > 128 {
		return domain.NewValidationError("username", "Use at most 128 characters.")
	}
	if username != current.Username {
		update.Username = utils.ToPtr(username)
	}
	return nil
}

// prepareEmailUpdate validates and records a locally managed email replacement.
func prepareEmailUpdate(input UserUpdateInput, current domain.UserProfile, update *domain.UserAccountUpdate) error {
	if input.Email == nil || input.RevertEmail {
		return nil
	}

	email := strings.TrimSpace(*input.Email)
	if email != "" && !validAccountEmail(email) {
		return domain.NewValidationError("email", "Enter a valid email address.")
	}
	if email != current.Email {
		update.Email = utils.ToPtr(email)
	}
	return nil
}

// prepareDisplayNameUpdate records a locally managed display name using the effective username as its blank fallback.
func prepareDisplayNameUpdate(input UserUpdateInput, current domain.UserProfile, update *domain.UserAccountUpdate) {
	if input.DisplayName == nil || input.RevertDisplayName {
		return
	}

	displayName := strings.TrimSpace(*input.DisplayName)
	if displayName == "" {
		displayName = current.Username
		if update.Username != nil {
			displayName = *update.Username
		}
	}
	if displayName != current.DisplayName {
		update.DisplayName = utils.ToPtr(displayName)
	}
}

// recordProfileAudit records each field whose ownership or local value changed.
func (s *Users) recordProfileAudit(ctx context.Context, actorID int64, current domain.UserProfile, update domain.UserAccountUpdate) {
	fields := []struct {
		// name is the profile field label included in the audit detail.
		name string
		// value is the new local value, or nil when no local replacement was requested.
		value *string
		// reverted reports whether provider ownership was restored.
		reverted bool
	}{
		{"username", update.Username, update.RevertUsername},
		{"email", update.Email, update.RevertEmail},
		{"display name", update.DisplayName, update.RevertDisplayName},
	}
	for _, field := range fields {
		action, detail := "user.profile_updated", "Updated local "+field.name
		if field.reverted {
			action, detail = "user.profile_override_reverted", "Restored provider-managed "+field.name
		} else if field.value != nil && current.Source != domain.ProfileSourceLocal {
			action, detail = "user.profile_overridden", "Locally overrode provider-managed "+field.name
		} else if field.value == nil {
			continue
		}
		audit.Record(ctx, s.logger, s.repository, actorID, action, "user", fmt.Sprint(current.UserID), detail)
	}
}

// validAccountEmail reports whether value is one plain mailbox address.
func validAccountEmail(value string) bool {
	address, err := mail.ParseAddress(value)
	return err == nil && address.Address == value
}

// prepareLocalCredentialUpdate resolves authentication mode and applies the requested recovery-credential state.
func (s *Users) prepareLocalCredentialUpdate(ctx context.Context, input UserUpdateInput, update *domain.UserAccountUpdate) error {
	if !input.UpdateLocalCredential {
		return nil
	}
	mode, err := s.accountAuthenticationMode(ctx, input.AuthModeOverride)
	if err != nil {
		return err
	}
	if !domain.IsExternalAuthMode(mode) {
		return domain.NewValidationError(
			"local_credential_enabled",
			"Local recovery credentials can only be enabled or disabled while external authentication is active.")
	}
	update.LocalCredentialEnabled = utils.ToPtr(input.Password != "" || input.LocalCredentialEnabled)
	return nil
}

// accountAuthenticationMode returns the deployment override or the persisted authentication mode.
func (s *Users) accountAuthenticationMode(ctx context.Context, override domain.AuthMode) (domain.AuthMode, error) {
	if override != "" {
		return override, nil
	}
	settings, err := s.repository.ApplicationSettings(ctx)
	if err != nil {
		return "", fmt.Errorf("load authentication settings for account update: %w", err)
	}
	return settings.Authentication.Mode, nil
}

// preparePasswordUpdate validates and hashes an optional local recovery password.
func (s *Users) preparePasswordUpdate(password string, update *domain.UserAccountUpdate) error {
	if password == "" {
		return nil
	}
	if s.passwords == nil {
		return fmt.Errorf("hash account password: password service is not configured")
	}
	if problem := s.passwords.Problem(password); problem != "" {
		return domain.NewValidationError("local_password", problem)
	}
	hash, err := s.passwords.Hash(password)
	if err != nil {
		return fmt.Errorf("hash account password: %w", err)
	}
	update.PasswordHash = hash
	return nil
}
