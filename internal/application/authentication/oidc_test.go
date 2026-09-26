package authentication

import (
	"context"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type oidcRepositoryStub struct {
	settings            domain.ApplicationSettings
	registrationEnabled bool
	issuer              string
	subject             string
	username            string
	email               string
	displayName         string
	existing            domain.User
}

func (s *oidcRepositoryStub) ApplicationSettings(context.Context) (domain.ApplicationSettings, error) {
	return s.settings, nil
}

func (s *oidcRepositoryStub) OIDCUser(context.Context, string, string) (domain.User, error) {
	if s.existing.ID == 0 {
		return domain.User{}, domain.ErrNotFound
	}
	return s.existing, nil
}

func (s *oidcRepositoryStub) ResolveOIDCLogin(_ context.Context, issuer, subject, username, email, displayName string, registrationEnabled bool) (domain.User, error) {
	s.issuer = issuer
	s.subject = subject
	s.username = username
	s.email = email
	s.displayName = displayName
	s.registrationEnabled = registrationEnabled
	return domain.User{ID: 7, Username: username, Enabled: true}, nil
}

func TestOIDCLoginOwnsNormalizationAndRegistrationPolicy(t *testing.T) {
	t.Parallel()
	repository := &oidcRepositoryStub{settings: domain.ApplicationSettings{AllowUserRegistration: true}}

	user, err := NewOIDC(repository, nil).Login(context.Background(), " https://issuer.example ", " subject ", " alice ", " alice@example.test ", " Alice ")

	require.NoError(t, err)
	assert.Equal(t, int64(7), user.ID)
	assert.Equal(t, "https://issuer.example", repository.issuer)
	assert.Equal(t, "subject", repository.subject)
	assert.Equal(t, "alice", repository.username)
	assert.True(t, repository.registrationEnabled)
}

func TestOIDCLoginUsesDeploymentRegistrationOverride(t *testing.T) {
	t.Parallel()
	repository := &oidcRepositoryStub{settings: domain.ApplicationSettings{AllowUserRegistration: true}}
	disabled := false

	_, err := NewOIDC(repository, &disabled).Login(context.Background(), "issuer", "subject", "alice", "", "")

	require.NoError(t, err)
	assert.False(t, repository.registrationEnabled)
}

func TestOIDCLoginRejectsIncompleteIdentityBeforePersistence(t *testing.T) {
	t.Parallel()
	repository := &oidcRepositoryStub{}

	_, err := NewOIDC(repository, nil).Login(context.Background(), "issuer", "subject", " ", "", "")

	require.Error(t, err)
	assert.Empty(t, repository.issuer)
}
