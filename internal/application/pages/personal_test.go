package pages

import (
	"context"
	"errors"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// personalRepositoryStub provides controllable personal repository behavior for tests.
type personalRepositoryStub struct {
	personalRepository
	// slug records the slug observed by the test double.
	slug string
	// userID records the user ID observed by the test double.
	userID int64
	// scope configures or records the scope value used by the fixture.
	scope domain.PageWatchScope
}

func (s *personalRepositoryStub) GetPage(_ context.Context, slug string) (domain.Page, error) {
	s.slug = slug
	return domain.Page{Slug: slug}, nil
}

func (s *personalRepositoryStub) SetPageWatch(_ context.Context, slug string, userID int64, scope domain.PageWatchScope) error {
	s.slug = slug
	s.userID = userID
	s.scope = scope
	return nil
}

func TestSetPageWatchValidatesScopeBeforePersistence(t *testing.T) {
	t.Parallel()

	repository := &personalRepositoryStub{}
	err := NewPersonal(repository, &viewAccessFake{allowed: true}).SetPageWatchFor(
		context.Background(),
		domain.User{ID: 42},
		"guide",
		"children",
	)

	validation, ok := errors.AsType[*domain.ValidationError](err)
	require.True(t, ok)
	require.Len(t, validation.Fields, 1)
	assert.Equal(t, "scope", validation.Fields[0].Field)
	assert.Empty(t, repository.slug)
}

func TestSetPageWatchNormalizesApplicationInput(t *testing.T) {
	t.Parallel()

	repository := &personalRepositoryStub{}
	err := NewPersonal(repository, &viewAccessFake{allowed: true}).SetPageWatchFor(
		context.Background(),
		domain.User{ID: 42},
		" /guide/ ",
		" subtree ",
	)

	require.NoError(t, err)
	assert.Equal(t, "guide", repository.slug)
	assert.Equal(t, int64(42), repository.userID)
	assert.Equal(t, domain.PageWatchScopeSubtree, repository.scope)
}
