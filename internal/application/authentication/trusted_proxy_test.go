package authentication

import (
	"context"
	"errors"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type trustedProxyRepositoryStub struct {
	settings    domain.ApplicationSettings
	existing    domain.User
	refreshErr  error
	created     domain.User
	createCalls int
	username    string
}

func (s *trustedProxyRepositoryStub) ApplicationSettings(context.Context) (domain.ApplicationSettings, error) {
	return s.settings, nil
}

func (s *trustedProxyRepositoryStub) RefreshTrustedProxyUser(_ context.Context, username, _, _ string) (domain.User, error) {
	s.username = username
	if s.refreshErr != nil {
		return domain.User{}, s.refreshErr
	}
	if s.existing.ID == 0 {
		return domain.User{}, domain.ErrNotFound
	}
	return s.existing, nil
}

func (s *trustedProxyRepositoryStub) CreateTrustedProxyUser(_ context.Context, username, _, _ string) (domain.User, error) {
	s.createCalls++
	s.username = username
	return s.created, nil
}

func TestTrustedProxyLoginRefreshesExistingUserWithoutRegistrationLookup(t *testing.T) {
	t.Parallel()

	repository := &trustedProxyRepositoryStub{existing: domain.User{ID: 7, Username: "alice"}}
	user, err := NewTrustedProxy(repository, nil).Login(context.Background(), " alice ", "", "")

	require.NoError(t, err)
	assert.Equal(t, int64(7), user.ID)
	assert.Equal(t, "alice", repository.username)
	assert.Zero(t, repository.createCalls)
}

func TestTrustedProxyLoginHonorsPersistedRegistrationPolicy(t *testing.T) {
	t.Parallel()

	repository := &trustedProxyRepositoryStub{
		settings: domain.ApplicationSettings{AllowUserRegistration: true},
		created:  domain.User{ID: 9, Username: "alice"},
	}
	user, err := NewTrustedProxy(repository, nil).Login(context.Background(), "alice", "", "")

	require.NoError(t, err)
	assert.Equal(t, int64(9), user.ID)
	assert.Equal(t, 1, repository.createCalls)
}

func TestTrustedProxyLoginHonorsDeploymentRegistrationOverride(t *testing.T) {
	t.Parallel()

	disabled := false
	repository := &trustedProxyRepositoryStub{
		settings: domain.ApplicationSettings{AllowUserRegistration: true},
	}
	_, err := NewTrustedProxy(repository, &disabled).Login(context.Background(), "alice", "", "")

	assert.ErrorIs(t, err, domain.ErrRegistrationDisabled)
	assert.Zero(t, repository.createCalls)
}

func TestTrustedProxyLoginPreservesRefreshFailure(t *testing.T) {
	t.Parallel()

	failure := errors.New("database unavailable")
	repository := &trustedProxyRepositoryStub{refreshErr: failure}
	_, err := NewTrustedProxy(repository, nil).Login(context.Background(), "alice", "", "")

	assert.ErrorIs(t, err, failure)
	assert.Zero(t, repository.createCalls)
}
