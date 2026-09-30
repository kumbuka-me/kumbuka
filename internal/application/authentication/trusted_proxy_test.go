package authentication

import (
	"context"
	"errors"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// trustedProxyRepositoryStub provides controllable trusted-proxy persistence behavior for application tests.
type trustedProxyRepositoryStub struct {
	// settings is the application settings returned by the fixture.
	settings domain.ApplicationSettings
	// existing is the existing account returned by the fixture.
	existing domain.User
	// refreshErr is the error returned by the trusted-proxy refresh operation.
	refreshErr error
	// created is the account returned by the trusted-proxy create operation.
	created domain.User
	// createCalls counts trusted-proxy account creation attempts.
	createCalls int
	// username captures the username supplied to the exercised operation.
	username string
}

func (s *trustedProxyRepositoryStub) ApplicationSettings(context.Context) (domain.ApplicationSettings, error) {
	return s.settings, nil
}

func (s *trustedProxyRepositoryStub) RefreshTrustedProxyUser(_ context.Context, username, _, _ string, _, _ bool) (domain.User, error) {
	s.username = username
	if s.refreshErr != nil {
		return domain.User{}, s.refreshErr
	}
	if s.existing.ID == 0 {
		return domain.User{}, domain.ErrNotFound
	}
	return s.existing, nil
}

func (s *trustedProxyRepositoryStub) CreateTrustedProxyUser(_ context.Context, username, _, _ string, _, _ bool) (domain.User, error) {
	s.createCalls++
	s.username = username
	return s.created, nil
}

func TestTrustedProxyLoginRefreshesExistingUserWithoutRegistrationLookup(t *testing.T) {
	t.Parallel()

	repository := &trustedProxyRepositoryStub{existing: domain.User{ID: 7, Username: "alice"}}
	user, err := NewTrustedProxy(repository, nil).Login(context.Background(), " alice ", "", "", false, false)

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
	user, err := NewTrustedProxy(repository, nil).Login(context.Background(), "alice", "", "", false, false)

	require.NoError(t, err)
	assert.Equal(t, int64(9), user.ID)
	assert.Equal(t, 1, repository.createCalls)
}

func TestTrustedProxyLoginHonorsDeploymentRegistrationOverride(t *testing.T) {
	t.Parallel()

	repository := &trustedProxyRepositoryStub{
		settings: domain.ApplicationSettings{AllowUserRegistration: true},
	}
	_, err := NewTrustedProxy(repository, utils.ToPtr(false)).Login(context.Background(), "alice", "", "", false, false)

	assert.ErrorIs(t, err, domain.ErrRegistrationDisabled)
	assert.Zero(t, repository.createCalls)
}

func TestTrustedProxyLoginPreservesRefreshFailure(t *testing.T) {
	t.Parallel()

	failure := errors.New("database unavailable")
	repository := &trustedProxyRepositoryStub{refreshErr: failure}
	_, err := NewTrustedProxy(repository, nil).Login(context.Background(), "alice", "", "", false, false)

	assert.ErrorIs(t, err, failure)
	assert.Zero(t, repository.createCalls)
}
