package users

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// accountRepositoryStub provides controllable account repository behavior for tests.
type accountRepositoryStub struct {
	// userRepository is embedded to provide the default interface behavior for this fixture.
	userRepository
	// update configures or records the update value used by the fixture.
	update domain.UserAccountUpdate
	// calls counts calls observed by the test double.
	calls int
	// mode configures or records the mode value used by the fixture.
	mode domain.AuthMode
	// failure configures or records the failure value used by the fixture.
	failure error
	// profile is the current profile returned to profile mutations.
	profile domain.UserProfile
	// audits records emitted audit actions.
	audits []string
	// relinked records a trusted-proxy identity mutation.
	relinked string
}

// passwordServiceStub provides controllable password service behavior for tests.
type passwordServiceStub struct {
	// problem configures or records the problem value used by the fixture.
	problem string
	// hash configures or records the hash value used by the fixture.
	hash string
	// err configures the error returned by the test double.
	err error
}

func (s passwordServiceStub) Problem(string) string       { return s.problem }
func (s passwordServiceStub) Hash(string) (string, error) { return s.hash, s.err }

func (s *accountRepositoryStub) ApplicationSettings(context.Context) (domain.ApplicationSettings, error) {
	return domain.ApplicationSettings{Authentication: domain.AuthenticationSettings{Mode: s.mode}}, s.failure
}
func (s *accountRepositoryStub) UpdateUserAccount(_ context.Context, input domain.UserAccountUpdate) error {
	s.calls++
	s.update = input
	return s.failure
}
func (s *accountRepositoryStub) UserProfile(context.Context, int64) (domain.UserProfile, error) {
	profile := s.profile
	if profile.UserID == 0 {
		profile.UserID = 7
		profile.Source = domain.ProfileSourceLocal
	}
	return profile, s.failure
}
func (s *accountRepositoryStub) LogAudit(_ context.Context, _ int64, action, _, _, _ string) error {
	s.audits = append(s.audits, action)
	return nil
}
func (s *accountRepositoryStub) RelinkTrustedProxyIdentity(_ context.Context, _ int64, username string) error {
	s.relinked = username
	return s.failure
}
func accountInput() UserUpdateInput {
	return UserUpdateInput{UserID: 7, Actor: domain.User{ID: 7, Role: "admin"}, Role: "admin", Enabled: true}
}
func TestAccountUpdateValidatesBeforeWriting(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*UserUpdateInput)
		field  string
	}{
		{"self demotion", func(i *UserUpdateInput) { i.Role = "editor" }, "role"},
		{"self disable", func(i *UserUpdateInput) { i.Enabled = false }, "account_enabled"},
		{"password", func(i *UserUpdateInput) { i.Password = "short" }, "local_password"},
		{"local mode toggle", func(i *UserUpdateInput) { i.UpdateLocalCredential = true }, "local_credential_enabled"},
		{"runtime override", func(i *UserUpdateInput) { i.UpdateLocalCredential = true; i.AuthModeOverride = "local" }, "local_credential_enabled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &accountRepositoryStub{mode: "local"}
			if tc.name == "runtime override" {
				repo.mode = "oidc"
			}
			input := accountInput()
			tc.change(&input)
			passwords := passwordServiceStub{hash: "hashed-password"}
			if tc.name == "password" {
				passwords.problem = "Use at least 12 characters."
			}
			err := NewUsers(repo, passwords, slog.Default()).UpdateAccount(context.Background(), input)
			validation, ok := errors.AsType[*domain.ValidationError](err)
			require.True(t, ok)
			assert.Equal(t, tc.field, validation.Fields[0].Field)
			assert.Zero(t, repo.calls)
		})
	}
}
func TestAccountUpdateHashesPasswordAndWritesOnce(t *testing.T) {
	for _, toggle := range []bool{false, true} {
		repo := &accountRepositoryStub{mode: "oidc"}
		input := accountInput()
		input.Password = "a-long-password-123"
		input.UpdateLocalCredential = toggle
		require.NoError(t, NewUsers(repo, passwordServiceStub{hash: "hashed-password"}, slog.Default()).UpdateAccount(context.Background(), input))
		assert.Equal(t, 1, repo.calls)
		assert.Equal(t, "hashed-password", repo.update.PasswordHash)
		if toggle {
			require.NotNil(t, repo.update.LocalCredentialEnabled)
			assert.True(t, *repo.update.LocalCredentialEnabled)
		} else {
			assert.Nil(t, repo.update.LocalCredentialEnabled)
		}
	}
}
func TestAccountUpdatePreservesPersistenceFailure(t *testing.T) {
	failure := errors.New("database unavailable")
	repo := &accountRepositoryStub{failure: failure}
	require.ErrorIs(t, NewUsers(repo, nil, slog.Default()).UpdateAccount(context.Background(), accountInput()), failure)
	assert.Equal(t, 1, repo.calls)
}

func TestUpdateUserUsesAccountMutationBoundary(t *testing.T) {
	t.Parallel()

	enabled := true
	repo := &accountRepositoryStub{}
	err := NewUsers(repo, nil, slog.Default()).UpdateUser(
		context.Background(),
		7,
		"editor",
		true,
		[]int64{2, 5},
		&enabled,
	)

	require.NoError(t, err)
	assert.Equal(t, 1, repo.calls)
	assert.Equal(t, domain.UserAccountUpdate{
		UserID:                 7,
		Role:                   "editor",
		Enabled:                true,
		GroupIDs:               []int64{2, 5},
		LocalCredentialEnabled: &enabled,
	}, repo.update)
}

func TestAccountUpdatePreservesAuthenticationSettingsFailure(t *testing.T) {
	t.Parallel()

	failure := errors.New("settings unavailable")
	repo := &accountRepositoryStub{failure: failure}
	input := accountInput()
	input.UpdateLocalCredential = true

	err := NewUsers(repo, passwordServiceStub{}, slog.Default()).UpdateAccount(context.Background(), input)

	require.ErrorIs(t, err, failure)
	assert.Zero(t, repo.calls)
}

func TestAccountUpdatePreservesPasswordHashFailure(t *testing.T) {
	t.Parallel()

	failure := errors.New("hash unavailable")
	repo := &accountRepositoryStub{}
	input := accountInput()
	input.Password = "a-long-password-123"

	err := NewUsers(repo, passwordServiceStub{err: failure}, slog.Default()).UpdateAccount(context.Background(), input)

	require.ErrorIs(t, err, failure)
	assert.Zero(t, repo.calls)
}

func TestAccountUpdateOverridesOnlyChangedOIDCField(t *testing.T) {
	t.Parallel()
	repo := &accountRepositoryStub{profile: domain.UserProfile{
		UserID: 7, Source: domain.ProfileSourceOIDC, Username: "provider-user",
		Email: "provider@example.test", DisplayName: "Provider User",
	}}
	input := accountInput()
	username, email, displayName := "provider-user", "local@example.test", "Provider User"
	input.Username, input.Email, input.DisplayName = &username, &email, &displayName

	require.NoError(t, NewUsers(repo, nil, slog.Default()).UpdateAccount(context.Background(), input))
	assert.Nil(t, repo.update.Username)
	require.NotNil(t, repo.update.Email)
	assert.Equal(t, "local@example.test", *repo.update.Email)
	assert.Nil(t, repo.update.DisplayName)
	assert.Equal(t, []string{"user.profile_overridden"}, repo.audits)
}

func TestAccountUpdateRevertsOIDCField(t *testing.T) {
	t.Parallel()
	repo := &accountRepositoryStub{profile: domain.UserProfile{
		UserID: 7, Source: domain.ProfileSourceOIDC, Username: "alice",
		Email: "local@example.test", ProviderEmail: "provider@example.test", EmailOverridden: true,
	}}
	input := accountInput()
	input.RevertEmail = true

	require.NoError(t, NewUsers(repo, nil, slog.Default()).UpdateAccount(context.Background(), input))
	assert.True(t, repo.update.RevertEmail)
	assert.Equal(t, []string{"user.profile_override_reverted"}, repo.audits)
}

func TestAccountUpdateRejectsGenericTrustedProxyUsername(t *testing.T) {
	t.Parallel()
	repo := &accountRepositoryStub{profile: domain.UserProfile{
		UserID: 7, Source: domain.ProfileSourceTrustedProxy, Username: "proxy-user",
	}}
	input := accountInput()
	username := "renamed"
	input.Username = &username

	err := NewUsers(repo, nil, slog.Default()).UpdateAccount(context.Background(), input)
	validation, ok := errors.AsType[*domain.ValidationError](err)
	require.True(t, ok)
	assert.Equal(t, "username", validation.Fields[0].Field)
	assert.Zero(t, repo.calls)
}

func TestRelinkTrustedProxyIdentityAuditsChange(t *testing.T) {
	t.Parallel()
	repo := &accountRepositoryStub{}
	actor := domain.User{ID: 3, Role: domain.UserRoleAdmin}

	require.NoError(t, NewUsers(repo, nil, slog.Default()).RelinkTrustedProxyIdentity(context.Background(), 7, " proxy-new ", actor))
	assert.Equal(t, "proxy-new", repo.relinked)
	assert.Equal(t, []string{"identity.trusted_proxy_relinked"}, repo.audits)
}
