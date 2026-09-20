package users

import (
	"context"
	"errors"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

type accountRepositoryStub struct {
	userRepository
	update  domain.UserAccountUpdate
	calls   int
	mode    string
	failure error
}

func (s *accountRepositoryStub) ApplicationSettings(context.Context) (domain.ApplicationSettings, error) {
	return domain.ApplicationSettings{Authentication: domain.AuthenticationSettings{Mode: s.mode}}, s.failure
}
func (s *accountRepositoryStub) UpdateUserAccount(_ context.Context, input domain.UserAccountUpdate) error {
	s.calls++
	s.update = input
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
			err := NewUsers(repo).UpdateAccount(context.Background(), input)
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
		require.NoError(t, NewUsers(repo).UpdateAccount(context.Background(), input))
		assert.Equal(t, 1, repo.calls)
		require.NoError(t, bcrypt.CompareHashAndPassword([]byte(repo.update.PasswordHash), []byte(input.Password)))
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
	require.ErrorIs(t, NewUsers(repo).UpdateAccount(context.Background(), accountInput()), failure)
	assert.Equal(t, 1, repo.calls)
}

func TestUpdateUserUsesAccountMutationBoundary(t *testing.T) {
	t.Parallel()

	enabled := true
	repo := &accountRepositoryStub{}
	err := NewUsers(repo).UpdateUser(
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
