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

func TestAccountUpdateNormalizesAdminManagedProfile(t *testing.T) {
	t.Parallel()

	repo := &accountRepositoryStub{}
	input := accountInput()
	username := "  renamed-user  "
	email := "  renamed@example.test  "
	displayName := "  Renamed User  "
	input.Username = &username
	input.Email = &email
	input.DisplayName = &displayName

	err := NewUsers(repo, nil, slog.Default()).UpdateAccount(context.Background(), input)

	require.NoError(t, err)
	require.NotNil(t, repo.update.Username)
	require.NotNil(t, repo.update.Email)
	require.NotNil(t, repo.update.DisplayName)
	assert.Equal(t, "renamed-user", *repo.update.Username)
	assert.Equal(t, "renamed@example.test", *repo.update.Email)
	assert.Equal(t, "Renamed User", *repo.update.DisplayName)
}

func TestAccountUpdateUsesUsernameAsEmptyDisplayNameFallback(t *testing.T) {
	t.Parallel()

	repo := &accountRepositoryStub{}
	input := accountInput()
	username := "renamed-user"
	email := ""
	displayName := "   "
	input.Username = &username
	input.Email = &email
	input.DisplayName = &displayName

	err := NewUsers(repo, nil, slog.Default()).UpdateAccount(context.Background(), input)

	require.NoError(t, err)
	require.NotNil(t, repo.update.DisplayName)
	assert.Equal(t, "renamed-user", *repo.update.DisplayName)
}

func TestAccountUpdateRejectsInvalidAdminManagedProfile(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name        string
		username    string
		email       string
		displayName string
		field       string
	}{
		{name: "empty username", username: " ", email: "user@example.test", displayName: "User", field: "username"},
		{name: "invalid email", username: "user", email: "not-an-email", displayName: "User", field: "email"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			repo := &accountRepositoryStub{}
			input := accountInput()
			input.Username = &tc.username
			input.Email = &tc.email
			input.DisplayName = &tc.displayName

			err := NewUsers(repo, nil, slog.Default()).UpdateAccount(context.Background(), input)

			validation, ok := errors.AsType[*domain.ValidationError](err)
			require.True(t, ok)
			assert.Equal(t, tc.field, validation.Fields[0].Field)
			assert.Zero(t, repo.calls)
		})
	}
}

func TestAccountUpdateRejectsProfileChangeFromNonAdministrator(t *testing.T) {
	t.Parallel()

	repo := &accountRepositoryStub{}
	input := accountInput()
	input.Actor = domain.User{ID: 9, Role: domain.UserRoleEditor}
	username := "renamed-user"
	email := "renamed@example.test"
	displayName := "Renamed User"
	input.Username = &username
	input.Email = &email
	input.DisplayName = &displayName

	err := NewUsers(repo, nil, slog.Default()).UpdateAccount(context.Background(), input)

	require.ErrorIs(t, err, domain.ErrForbidden)
	assert.Zero(t, repo.calls)
}
